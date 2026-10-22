package facade

// REQ-368/M166 伴生图时序化查询面（96 号规划档主线 A·O-3）——companion_history 工具：
// 对伴生子图（ont-{oid} named graph）提供三类时序查询：
//   snapshot  某日快照重放——day 当日结束时仍有效的实体与关系断言
//             （tt=prov:generatedAtTime ≤ 当日末 且未被 bot:invalidAt 失效）；
//   evolution 某区间演化追踪——[from,to] 内新增断言（tt 落区间）+失效边（bot:invalidAt 落区间）；
//   evidence  Evidence 溯源——断言级证据链（confidence/bot:extractedFrom 源消息/prov:generatedAtTime/
//             bot:validFrom 有效性起点），定位粒度=源对话消息（候选抽取自单条消息）。
//
// 时序模型（双时序，REQ-280 收编）：tt=事务时间（入库时刻，prov:generatedAtTime，既有件）；
// vt=有效性时间（bot:validFrom，REQ-368 起自 timeScope 可解析日期落点；边的 vt 起点即 tt，
// 终点=bot:invalidAt 既有失效语义）。批次快照粒度（不做实时流式，REQ-368 不做清单）。
//
// 词表 IRI 与 backend/internal/companion 同源（跨 Go module 不互引，双实现各自测试锁定——
// 沿 REQ-151/236 白名单双实现先例）。工具无条件注册、挂载即并入（只读查询面，沿原 6 查询
// 工具语义，不入 backend OntoOperationTools 门控清单）。

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// 伴生薄本体词表（与 backend/internal/companion/sparql.go 常量同值）。
const (
	histBotNS    = "http://eino-lab/ontology/thin/"
	histProvNS   = "http://www.w3.org/ns/prov#"
	histXSDNS    = "http://www.w3.org/2001/XMLSchema#"
	histEntityNS = "http://eino-lab/e/"
	histMsgNS    = "http://eino-lab/msg/"
	histGraphNS  = "http://eino-lab/graph/"
)

// companionGraphURI 伴生图 URI（与 backend/internal/companion GraphURI 同值：ont-{oid}）。
func companionGraphURI(ontologyID string) string { return histGraphNS + "ont-" + ontologyID }

// histDayRE 日期形态（YYYY-MM-DD）校验。
var histDayRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// histDayEnd 日期 → 当日末 xsd:dateTime 字面量（快照截止点）。
func histDayEnd(day string) string { return day + "T23:59:59Z" }

// histDayStart 日期 → 当日起点 xsd:dateTime 字面量。
func histDayStart(day string) string { return day + "T00:00:00Z" }

func (f *Facade) addCompanionHistoryTool(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("companion_history",
		mcp.WithDescription("伴生图时序化查询（历史版本可读，REQ-368）：snapshot=某日快照重放（当日末仍有效的实体与关系）；evolution=某区间演化追踪（新增断言+失效边）；evidence=断言级证据溯源（置信度/源消息/入库时间/有效性起点，定位粒度=源对话消息）。伴生图=对话沉淀的动态知识（挂本体伴生子图，REQ-216）。"),
		mcp.WithString("ontology_id", mcp.Description("伴生本体仓库 id（与 agent_id/conversation_id 二选一，直传优先）")),
		mcp.WithString("agent_id", mcp.Description("智能体 id（解析其绑定本体的伴生子图）")),
		mcp.WithString("conversation_id", mcp.Description("会话 id（解析所属智能体绑定的伴生子图）")),
		mcp.WithString("mode", mcp.Required(), mcp.Description("查询模式：snapshot | evolution | evidence")),
		mcp.WithString("day", mcp.Description("snapshot 必填：快照日期（YYYY-MM-DD，重放当日末状态）")),
		mcp.WithString("from", mcp.Description("evolution 必填：区间起点（YYYY-MM-DD）")),
		mcp.WithString("to", mcp.Description("evolution 必填：区间终点（YYYY-MM-DD，含当日）")),
		mcp.WithString("entity", mcp.Description("evidence 可选：按实体/关系名标签过滤")),
		mcp.WithNumber("limit", mcp.Description("返回条数上限（可选，默认 100，最大 500）")),
	), f.handleCompanionHistory())
}

func (f *Facade) handleCompanionHistory() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		agentID, _ := args["agent_id"].(string)
		convID, _ := args["conversation_id"].(string)
		mode, _ := args["mode"].(string)
		day, _ := args["day"].(string)
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		entity, _ := args["entity"].(string)
		limit := 100
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 500 {
				limit = 500
			}
		}
		// 本体解析：ontology_id 直传优先；否则 agent/conv 解析绑定伴生本体（复用 sparql 通道解析）
		if strings.TrimSpace(oid) == "" {
			resolved, errRes := f.companionOntology(agentID, convID)
			if errRes != nil {
				return errRes, nil
			}
			oid = resolved
		}
		// 路由到宿主 running 方案引擎（伴生图=ont-{oid} named graph，宿主同 sparql companion 分支）
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		g := companionGraphURI(oid)
		switch mode {
		case "snapshot":
			return f.companionSnapshot(ctx, oid, p.ID, ep, g, day, limit)
		case "evolution":
			return f.companionEvolution(ctx, oid, p.ID, ep, g, from, to, limit)
		case "evidence":
			return f.companionEvidence(ctx, oid, p.ID, ep, g, entity, limit)
		default:
			return mcp.NewToolResultErrorf("mode 仅支持 snapshot | evolution | evidence，得到 %q", mode), nil
		}
	}
}

// histEsc Turtle 字面量转义（引号/反斜杠；标签来自 LLM 抽取文本）。
func histEsc(s string) string {
	r := strings.NewReplacer("\\", "\\\\", `"`, `\"`)
	return r.Replace(s)
}

// companionSnapshot 某日快照重放：实体（kind!=bot:Relation）+ 有效边（tt≤当日末且未在当日末前失效）。
func (f *Facade) companionSnapshot(ctx context.Context, oid, profileID, ep, g, day string, limit int) (*mcp.CallToolResult, error) {
	if !histDayRE.MatchString(strings.TrimSpace(day)) {
		return mcp.NewToolResultErrorf("snapshot 需 day（YYYY-MM-DD），得到 %q", day), nil
	}
	dayEnd := histDayEnd(strings.TrimSpace(day))
	entQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <%s>
SELECT ?kind ?label ?tt WHERE {
  GRAPH <%s> {
    ?e a ?kind ; rdfs:label ?label ; prov:generatedAtTime ?tt .
    FILTER(?kind != bot:Relation)
    FILTER(?tt <= "%s"^^xsd:dateTime)
  }
} ORDER BY ?label LIMIT %d`, histBotNS, histProvNS, histXSDNS, g, dayEnd, limit)
	entBindings, err := f.query(ctx, ep, entQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	edgeQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <%s>
SELECT ?src ?rel ?dst WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject ?s ; bot:object ?o ; bot:relName ?rel ; prov:generatedAtTime ?tt .
    ?s rdfs:label ?src . ?o rdfs:label ?dst .
    FILTER(?tt <= "%s"^^xsd:dateTime)
    FILTER NOT EXISTS { ?edge bot:invalidAt ?inv . FILTER(?inv <= "%s"^^xsd:dateTime) }
  }
} ORDER BY ?src LIMIT %d`, histBotNS, histProvNS, histXSDNS, g, dayEnd, dayEnd, limit)
	edgeBindings, err := f.query(ctx, ep, edgeQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	entities := make([]map[string]string, 0, len(entBindings))
	for _, b := range entBindings {
		entities = append(entities, map[string]string{
			"kind":  histLocal(lit(b["kind"])),
			"label": lit(b["label"]),
			"tt":    lit(b["tt"]),
		})
	}
	edges := make([]map[string]string, 0, len(edgeBindings))
	for _, b := range edgeBindings {
		edges = append(edges, map[string]string{"source": lit(b["src"]), "rel": lit(b["rel"]), "target": lit(b["dst"])})
	}
	return jsonResult(map[string]any{
		"mode": "snapshot", "ontology_id": oid, "graph": g, "day": strings.TrimSpace(day),
		"entities": entities, "edges": edges,
		"counts": map[string]int{"entities": len(entities), "edges": len(edges)},
		"note":   "快照=当日末仍有效断言重放（批次粒度，REQ-368）；vt 起点 bot:validFrom 随 evidence 模式可查",
	}), nil
}

// companionEvolution 区间演化：新增实体/边（tt 落区间）+ 失效边（invalidAt 落区间）。
func (f *Facade) companionEvolution(ctx context.Context, oid, profileID, ep, g, from, to string, limit int) (*mcp.CallToolResult, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if !histDayRE.MatchString(from) || !histDayRE.MatchString(to) {
		return mcp.NewToolResultErrorf("evolution 需 from/to（YYYY-MM-DD），得到 %q/%q", from, to), nil
	}
	lo, hi := histDayStart(from), histDayEnd(to)
	entQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <%s>
SELECT ?kind ?label ?tt WHERE {
  GRAPH <%s> {
    ?e a ?kind ; rdfs:label ?label ; prov:generatedAtTime ?tt .
    FILTER(?kind != bot:Relation)
    FILTER(?tt >= "%s"^^xsd:dateTime && ?tt <= "%s"^^xsd:dateTime)
  }
} ORDER BY ?tt LIMIT %d`, histBotNS, histProvNS, histXSDNS, g, lo, hi, limit)
	entBindings, err := f.query(ctx, ep, entQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	edgeQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <%s>
SELECT ?src ?rel ?dst ?tt WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject ?s ; bot:object ?o ; bot:relName ?rel ; prov:generatedAtTime ?tt .
    ?s rdfs:label ?src . ?o rdfs:label ?dst .
    FILTER(?tt >= "%s"^^xsd:dateTime && ?tt <= "%s"^^xsd:dateTime)
  }
} ORDER BY ?tt LIMIT %d`, histBotNS, histProvNS, histXSDNS, g, lo, hi, limit)
	edgeBindings, err := f.query(ctx, ep, edgeQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	invQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <%s>
SELECT ?src ?rel ?dst ?inv WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject ?s ; bot:object ?o ; bot:relName ?rel ; bot:invalidAt ?inv .
    ?s rdfs:label ?src . ?o rdfs:label ?dst .
    FILTER(?inv >= "%s"^^xsd:dateTime && ?inv <= "%s"^^xsd:dateTime)
  }
} ORDER BY ?inv LIMIT %d`, histBotNS, histXSDNS, g, lo, hi, limit)
	invBindings, err := f.query(ctx, ep, invQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	addedEntities := make([]map[string]string, 0, len(entBindings))
	for _, b := range entBindings {
		addedEntities = append(addedEntities, map[string]string{"kind": histLocal(lit(b["kind"])), "label": lit(b["label"]), "tt": lit(b["tt"])})
	}
	addedEdges := make([]map[string]string, 0, len(edgeBindings))
	for _, b := range edgeBindings {
		addedEdges = append(addedEdges, map[string]string{"source": lit(b["src"]), "rel": lit(b["rel"]), "target": lit(b["dst"]), "tt": lit(b["tt"])})
	}
	invalidated := make([]map[string]string, 0, len(invBindings))
	for _, b := range invBindings {
		invalidated = append(invalidated, map[string]string{"source": lit(b["src"]), "rel": lit(b["rel"]), "target": lit(b["dst"]), "invalidated_at": lit(b["inv"])})
	}
	return jsonResult(map[string]any{
		"mode": "evolution", "ontology_id": oid, "graph": g, "from": from, "to": to,
		"added":       map[string]any{"entities": addedEntities, "edges": addedEdges},
		"invalidated": invalidated,
		"counts":      map[string]int{"added_entities": len(addedEntities), "added_edges": len(addedEdges), "invalidated": len(invalidated)},
		"note":        "演化追踪=区间内新增断言+失效边（批次粒度，REQ-368）",
	}), nil
}

// companionEvidence 断言级证据链：置信度/源消息/入库时间/有效性起点（实体与边两类断言）。
func (f *Facade) companionEvidence(ctx context.Context, oid, profileID, ep, g, entity string, limit int) (*mcp.CallToolResult, error) {
	entityFilter := ""
	if e := strings.TrimSpace(entity); e != "" {
		entityFilter = fmt.Sprintf("\n    FILTER(?label = %q)", histEsc(e))
	}
	nodeQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?kind ?label ?conf ?msg ?tt ?vf ?et WHERE {
  GRAPH <%s> {
    ?e a ?kind ; rdfs:label ?label ; bot:confidence ?conf ; bot:extractedFrom ?msg ; prov:generatedAtTime ?tt .
    FILTER(?kind != bot:Relation)
    OPTIONAL { ?e bot:validFrom ?vf }
    OPTIONAL { ?e bot:evidenceText ?et }%s
  }
} ORDER BY DESC(?tt) LIMIT %d`, histBotNS, histProvNS, g, entityFilter, limit)
	nodeBindings, err := f.query(ctx, ep, nodeQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	edgeQ := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?src ?rel ?dst ?conf ?msg ?tt ?inv ?et WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject ?s ; bot:object ?o ; bot:relName ?rel ; bot:confidence ?conf ; bot:extractedFrom ?msg ; prov:generatedAtTime ?tt .
    ?s rdfs:label ?src . ?o rdfs:label ?dst .
    OPTIONAL { ?edge bot:invalidAt ?inv }
    OPTIONAL { ?edge bot:evidenceText ?et }
    %s
  }
} ORDER BY DESC(?tt) LIMIT %d`, histBotNS, histProvNS, g, edgeEntityFilter(entity), limit)
	edgeBindings, err := f.query(ctx, ep, edgeQ)
	if err != nil {
		return mcp.NewToolResultErrorf("查询失败: %v", err), nil
	}
	assertions := make([]map[string]string, 0, len(nodeBindings)+len(edgeBindings))
	for _, b := range nodeBindings {
		a := map[string]string{
			"type": "node", "name": lit(b["label"]), "kind": histLocal(lit(b["kind"])),
			"confidence": lit(b["conf"]), "source_msg": histLocal(lit(b["msg"])), "generated_at": lit(b["tt"]),
		}
		if b["vf"] != nil {
			a["valid_from"] = lit(b["vf"])
		}
		if b["et"] != nil {
			a["evidence_text"] = lit(b["et"])
		}
		assertions = append(assertions, a)
	}
	for _, b := range edgeBindings {
		a := map[string]string{
			"type": "edge", "name": lit(b["rel"]), "source": lit(b["src"]), "target": lit(b["dst"]),
			"confidence": lit(b["conf"]), "source_msg": histLocal(lit(b["msg"])), "generated_at": lit(b["tt"]),
		}
		if b["inv"] != nil {
			a["invalidated_at"] = lit(b["inv"])
		}
		if b["et"] != nil {
			a["evidence_text"] = lit(b["et"])
		}
		assertions = append(assertions, a)
	}
	return jsonResult(map[string]any{
		"mode": "evidence", "ontology_id": oid, "graph": g,
		"entity": strings.TrimSpace(entity), "assertions": assertions,
		"counts": map[string]int{"assertions": len(assertions)},
		"note":   "证据定位粒度=源对话消息（候选抽取自单条消息，REQ-368/359）；source_msg=消息 id（urn 的 msg 段）",
	}), nil
}

// edgeEntityFilter 边证据查询的标签过滤（关系名或端点标签任一命中）。
func edgeEntityFilter(entity string) string {
	if e := strings.TrimSpace(entity); e != "" {
		return fmt.Sprintf("FILTER(?rel = %q || ?src = %q || ?dst = %q)", histEsc(e), histEsc(e), histEsc(e))
	}
	return ""
}

// histLocal URI 取本地段（urn/URL 末段——msg id、kind 名等展示用）。
func histLocal(u string) string {
	if i := strings.LastIndexAny(u, "/#:"); i >= 0 && i+1 < len(u) {
		return u[i+1:]
	}
	return u
}
