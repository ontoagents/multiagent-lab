// Package facade 统一 MCP facade（方案 04 §4.4）：
// tools/list 固定返回 6 个 onto_* 工具（Q-14 契约 4→5→6：REQ-151 增 sparql_query，REQ-215 增 list_concepts）；
// tools/call 按 ontology_id 路由到所属 running 方案，将工具语义翻译为 SPARQL 在
// Oxigraph 执行。Agent 侧无感运行时差异。
package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
	"github.com/ontoagents/ontoagents/pkg/sparqlguard"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/manager"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

type Facade struct {
	Store    *store.Store
	Endpoint func(profileID string) (string, error) // Manager.ProcEndpoint
	HTTP     *http.Client
	// Manager REQ-435/M231：运行面生命周期与引擎运维（nil=运行面 12 件工具不注册——构造兼容；
	// 生产装配在 runtimed main 与 rest.Server 共用同一实例）。
	Manager *manager.Manager
	// TraceSparql 翻译透视开关（REQ-94，§4.8.2）：开启时把工具翻译出的 SPARQL
	// 与执行耗时/结果数落 trace_log，供前端“翻译透视”视图学习用。
	TraceSparql bool
	// ownerCache 伴生图归属解析缓存（REQ-216：agent/会话 → 绑定伴生本体 id）。
	// REQ-216 增量④：TTL 失效（ownerCacheTTL）——agent 换绑本体后 facade 查询最迟一个
	// TTL 收敛，不再 stale 到进程重启。
	ownerMu    sync.Mutex
	ownerCache map[string]ownerEntry
}

// ownerEntry 归属缓存行（值 + 写入时刻，TTL 失效）。
type ownerEntry struct {
	oid string
	at  time.Time
}

// ownerCacheTTL 归属缓存有效期（换绑收敛上限；解析是每查询一次平台 HTTP，TTL 不宜过短；var 便于测试收紧）。
// OwnerCacheTTL 归属缓存有效期的导出形态（REQ-384：换绑收敛窗口透出——观测面
// route-ambiguities 返回该值；ownerCacheTTL 为内部 var 便于测试收紧）。
const OwnerCacheTTL = 60 * time.Second

var ownerCacheTTL = OwnerCacheTTL

func New(st *store.Store, endpoint func(string) (string, error)) *Facade {
	return &Facade{Store: st, Endpoint: endpoint, HTTP: &http.Client{Timeout: 15 * time.Second}, TraceSparql: true, ownerCache: map[string]ownerEntry{}}
}

// QueryOntology REQ-382/M178：按本体路由执行只读 SPARQL（REST 面用——cqbench 执行级对齐
// 的执行通道；与 MCP sparql_query 同一 RunningByOntology 路由语义，default 图无伴生限定）。
// 返回 bindings 原始结果集；路由失败/执行失败返回错误。
func (f *Facade) QueryOntology(ctx context.Context, ontologyID, query string) ([]map[string]any, error) {
	if err := sparqlguard.Vet(query); err != nil {
		return nil, err
	}
	p, err := f.Store.RunningByOntology(ontologyID)
	if err != nil || p == nil {
		return nil, fmt.Errorf("本体 %s 未挂载到任何运行中的方案", ontologyID)
	}
	ep, err := f.Endpoint(p.ID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return f.query(cctx, ep, query)
}

// Mount 挂载到 /mcp（Streamable HTTP，沿用 Q-14）。
func (f *Facade) Mount() *server.StreamableHTTPServer {
	s := server.NewMCPServer("ontology-facade", "1.0.0", server.WithToolCapabilities(false))
	f.addTools(s)
	return server.NewStreamableHTTPServer(s)
}

// requiredString 必填字符串入参声明（各工具共用）。
func requiredString(name, desc string) mcp.ToolOption {
	return mcp.WithString(name, mcp.Required(), mcp.Description(desc))
}

func (f *Facade) addTools(s *server.MCPServer) {
	oid := requiredString
	s.AddTool(mcp.NewTool("get_concept",
		mcp.WithDescription("查询本体中某个概念（类）的定义：标签、描述、父概念。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "概念名（如 计算节点）"),
	), f.handleConcept())

	s.AddTool(mcp.NewTool("get_instance",
		mcp.WithDescription("查询本体中某个实例（个体）的详情：所属概念、属性、关系。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "实例名"),
	), f.handleInstance())

	s.AddTool(mcp.NewTool("list_instances",
		mcp.WithDescription("列出本体中某概念下的全部实例名。"),
		oid("ontology_id", "本体仓库 id"),
		oid("concept", "概念名"),
	), f.handleListInstances())

	s.AddTool(mcp.NewTool("neighbors",
		mcp.WithDescription("查询某实例的关系邻居：返回 [(关系名, 目标实例名)]。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "实例名"),
	), f.handleNeighbors())

	// REQ-215：指引不再预列概念全量清单（防 instruction 膨胀），概念发现运行时按需拉取。
	s.AddTool(mcp.NewTool("list_concepts",
		mcp.WithDescription("列出本体中全部概念（类）名。指引只给本体身份与规模，概念清单用本工具按需获取。"),
		oid("ontology_id", "本体仓库 id"),
		mcp.WithNumber("limit", mcp.Description("可选：最多返回的概念名个数（缺省全部，硬上限 500 防工具结果过大）")),
	), f.handleListConcepts())

	// REQ-415/M211：可回答性预检（99 号 D1 借鉴）——确定性预检不引入 LLM：
	// question 分词与概念标签 containment 匹配，覆盖度→{answerable, matched_concepts, suggestion}。
	// 治「agent 盲跑后才发现本体不含该域」；check_readiness 不另立（healthz/loaded_status 已对位）。
	s.AddTool(mcp.NewTool("check_answerable",
		mcp.WithDescription("预检本体能否回答某问题（确定性关键词/标签匹配，零 LLM）：返回 answerable、matched_concepts（命中概念）、missing（分域零命中清单——concepts/relations/synonyms 各域缺失的提问词，REQ-545）与 suggestion（建议调用序）。提问前先调用可避免盲跑。"),
		oid("ontology_id", "本体仓库 id"),
		oid("question", "用户问题原文"),
	), f.handleCheckAnswerable())

	f.addSparqlQueryTool(s)
	// REQ-368/M166：伴生图时序化查询面（snapshot/evolution/evidence 三模式）——只读挂载即并入。
	f.addCompanionHistoryTool(s)
	// REQ-366/M164：T2/T3/T5 十三件扩展工具（spec CRUD/校验/CQ 基准）——无条件注册，
	// MCP 标准面对外全量可枚举；内部智能体启用门控在平台 backend 装配层（agent.onto_ops）。
	f.addOpsTools(s)
	// REQ-435/M231：运行面三组 12 件（Manager 未接=不注册）
	f.addRuntimeTools(s)
}

// ---- SPARQL 执行与 URI 约定 ----

// query 对方案 SPARQL 端点执行 SELECT，返回 bindings（JSON 结果集）。
func (f *Facade) query(ctx context.Context, endpoint, sparql string) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(sparql))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SPARQL 执行失败: %s", resp.Status)
	}
	var res struct {
		Results struct {
			Bindings []map[string]any `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Results.Bindings, nil
}

// route 按 ontology_id 路由到 running 方案（返回方案与端点）。
func (f *Facade) route(ontologyID string) (p *store.Profile, endpoint string, errResult *mcp.CallToolResult) {
	p, err := f.Store.RunningByOntology(ontologyID)
	if err != nil || p == nil {
		return nil, "", mcp.NewToolResultErrorf("ONTOLOGY_SERVICE_UNAVAILABLE: 本体 %s 未挂载到任何运行中的方案（REQ-70 按方案隔离）", ontologyID)
	}
	ep, err := f.Endpoint(p.ID)
	if err != nil {
		return nil, "", mcp.NewToolResultErrorf("ONTOLOGY_SERVICE_UNAVAILABLE: %v", err)
	}
	return p, ep, nil
}

// exec 执行翻译后的 SPARQL 并记录翻译透视（REQ-94）。执行失败同样留痕（ok=false）。
func (f *Facade) exec(ctx context.Context, tool, ontologyID, profileID, endpoint, sparql string) ([]map[string]any, error) {
	start := time.Now()
	bindings, err := f.query(ctx, endpoint, sparql)
	if f.TraceSparql {
		tr := &store.Trace{Tool: tool, ProfileID: profileID, OntologyID: ontologyID,
			Sparql: sparql, TookMS: time.Since(start).Milliseconds(), ResultCount: len(bindings), Ok: err == nil}
		if err != nil {
			tr.Error = err.Error()
		}
		_ = f.Store.SaveTrace(tr) // 透视落库失败不影响查询本身
	}
	return bindings, err
}

func uriOf(ontologyID, kind, name string) string {
	switch kind {
	case "concept":
		return pkgspec.ConceptURI(ontologyID, name)
	case "relation":
		return pkgspec.RelationURI(ontologyID, name)
	case "instance":
		return pkgspec.InstanceURI(ontologyID, name)
	}
	return pkgspec.URIPrefix(ontologyID) + kind + ":" + pkgspec.Sanitize(name)
}

func lit(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprint(v)
	}
	if s, ok := m["value"].(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// nameFromURI 从 urn:o:{oid}:{kind}:{name} 反解 name。
func nameFromURI(u string) string {
	parts := strings.SplitN(u, ":", 5)
	if len(parts) == 5 {
		return parts[4]
	}
	return u
}

func jsonResult(v any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b))
}

// ---- 4 固定工具 handler（第 5 工具 sparql_query 见 sparql.go；第 6 工具 list_concepts 见 handleListConcepts；
// oxigraph 翻译：get_concept/get_instance→SELECT、list_instances→rdf:type、neighbors→属性路径）----

func (f *Facade) handleConcept() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		// REQ-482/M282：方案组作用域——概念按 rdfs:label 跨方案内全部本体匹配（URI 不再
		// 钉死单本体命名空间），每命中标注来源本体（URI 反解）；单命中保持既有扁平形状并增
		// ontology_id/profile_id，多命中返回 matches（§2.67 语义完备①③）。
		q := fmt.Sprintf(`PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX owl: <http://www.w3.org/2002/07/owl#>
SELECT ?c ?label ?comment ?parent WHERE {
  ?c rdf:type owl:Class ; rdfs:label %s .
  OPTIONAL { ?c rdfs:comment ?comment }
  OPTIONAL { ?c rdfs:subClassOf ?parent }
}`, sparqlString(name))
		bindings, err := f.exec(ctx, "get_concept", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		type cHit struct {
			src        string
			uri        string
			definition string
			parents    []string
			seen       map[string]bool
		}
		order := []*cHit{}
		byURI := map[string]*cHit{}
		for _, b := range bindings {
			if b["c"] == nil {
				continue
			}
			cu := lit(b["c"])
			h := byURI[cu]
			if h == nil {
				h = &cHit{src: hitSource(cu, oid), uri: cu, seen: map[string]bool{}}
				byURI[cu] = h
				order = append(order, h)
			}
			if h.definition == "" && b["comment"] != nil {
				h.definition = lit(b["comment"])
			}
			if b["parent"] != nil {
				pu := lit(b["parent"])
				if !h.seen[pu] {
					h.seen[pu] = true
					h.parents = append(h.parents, nameFromURI(pu))
				}
			}
		}
		if len(order) == 0 {
			return mcp.NewToolResultErrorf("概念 %q 不存在于本体 %s（方案组内 %d 个本体）", name, oid, len(p.LoadedOntologyIDs())), nil
		}
		hitOut := func(h *cHit) map[string]any {
			if h.parents == nil {
				h.parents = []string{}
			}
			m := map[string]any{"ontology_id": h.src, "name": name, "uri": h.uri, "label": name, "parents": h.parents}
			if h.definition != "" {
				m["definition"] = h.definition
			}
			return m
		}
		if len(order) == 1 {
			out := hitOut(order[0])
			out["profile_id"] = p.ID
			return jsonResult(out), nil
		}
		matches := make([]map[string]any, 0, len(order))
		for _, h := range order {
			matches = append(matches, hitOut(h))
		}
		return jsonResult(map[string]any{
			"count": len(matches), "matches": matches, "profile_id": p.ID,
			"note": "同名概念命中多个本体（方案组语义）——已标注来源本体，可用 GRAPH <urn:onto:{oid}> 深查",
		}), nil
	}
}

func (f *Facade) handleInstance() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		// REQ-482/M282：方案组作用域——实例按 rdfs:label 跨方案内全部本体匹配并标注来源。
		q := fmt.Sprintf(`SELECT ?i ?p ?o WHERE { ?i rdfs:label %s . ?i ?p ?o }`, sparqlString(name))
		bindings, err := f.exec(ctx, "get_instance", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		build := func(src, iu string, rows []map[string]any) map[string]any {
			out := map[string]any{"ontology_id": src, "name": name, "uri": iu,
				"attributes": map[string]any{}, "relations": []any{}}
			pfx := "urn:o:" + pkgspec.Sanitize(src) + ":"
			for _, b := range rows {
				pu, ov := lit(b["p"]), b["o"]
				switch {
				case strings.HasSuffix(pu, "22-rdf-syntax-ns#type"):
					out["concept"] = nameFromURI(lit(ov))
				case strings.HasPrefix(pu, pfx+"attr:"):
					out["attributes"].(map[string]any)[nameFromURI(pu)] = lit(ov)
				case strings.HasPrefix(pu, pfx+"relation:"):
					out["relations"] = append(out["relations"].([]any), map[string]string{"rel": nameFromURI(pu), "target": nameFromURI(lit(ov))})
				}
			}
			return out
		}
		order := []string{}
		byURI := map[string][]map[string]any{}
		for _, b := range bindings {
			if b["i"] == nil {
				continue
			}
			iu := lit(b["i"])
			if _, ok := byURI[iu]; !ok {
				order = append(order, iu)
			}
			byURI[iu] = append(byURI[iu], b)
		}
		if len(order) == 0 {
			return mcp.NewToolResultErrorf("实例 %q 不存在于本体 %s（方案组内 %d 个本体）", name, oid, len(p.LoadedOntologyIDs())), nil
		}
		if len(order) == 1 {
			out := build(hitSource(order[0], oid), order[0], byURI[order[0]])
			out["profile_id"] = p.ID
			return jsonResult(out), nil
		}
		matches := make([]map[string]any, 0, len(order))
		for _, iu := range order {
			matches = append(matches, build(hitSource(iu, oid), iu, byURI[iu]))
		}
		return jsonResult(map[string]any{
			"count": len(matches), "matches": matches, "profile_id": p.ID,
			"note": "同名实例命中多个本体（方案组语义）——已标注来源本体",
		}), nil
	}
}

func (f *Facade) handleListInstances() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		concept, _ := args["concept"].(string)
		if oid == "" || concept == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 concept 必填"), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		// REQ-482/M282：方案组作用域——概念按 label 跨本体匹配，逐命中本体列实例。
		q := fmt.Sprintf(`PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
SELECT ?c ?i WHERE { ?c rdf:type owl:Class ; rdfs:label %s . ?i rdf:type ?c } ORDER BY ?i`, sparqlString(concept))
		bindings, err := f.exec(ctx, "list_instances", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		type cHit struct {
			src       string
			uri       string
			instances []string
		}
		order := []*cHit{}
		byURI := map[string]*cHit{}
		for _, b := range bindings {
			if b["c"] == nil || b["i"] == nil {
				continue
			}
			cu := lit(b["c"])
			h := byURI[cu]
			if h == nil {
				h = &cHit{src: hitSource(cu, oid), uri: cu}
				byURI[cu] = h
				order = append(order, h)
			}
			h.instances = append(h.instances, nameFromURI(lit(b["i"])))
		}
		if len(order) == 0 {
			return jsonResult(map[string]any{"concept": concept, "count": 0, "instances": []string{}, "profile_id": p.ID}), nil
		}
		hitOut := func(h *cHit) map[string]any {
			return map[string]any{"ontology_id": h.src, "concept": concept, "count": len(h.instances), "instances": h.instances}
		}
		if len(order) == 1 {
			out := hitOut(order[0])
			out["profile_id"] = p.ID
			return jsonResult(out), nil
		}
		matches := make([]map[string]any, 0, len(order))
		for _, h := range order {
			matches = append(matches, hitOut(h))
		}
		return jsonResult(map[string]any{"count": len(matches), "matches": matches, "profile_id": p.ID}), nil
	}
}

func (f *Facade) handleNeighbors() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		// REQ-482/M282：方案组作用域——实例按 label 跨本体匹配，逐命中列关系邻居。
		q := fmt.Sprintf(`SELECT ?i ?p ?o WHERE { ?i rdfs:label %s . ?i ?p ?o . FILTER(isIRI(?o)) FILTER(STRSTARTS(STR(?p), "urn:o:")) }`, sparqlString(name))
		bindings, err := f.exec(ctx, "neighbors", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		type iHit struct {
			src       string
			uri       string
			neighbors []any
		}
		order := []*iHit{}
		byURI := map[string]*iHit{}
		for _, b := range bindings {
			if b["i"] == nil {
				continue
			}
			iu := lit(b["i"])
			h := byURI[iu]
			if h == nil {
				h = &iHit{src: hitSource(iu, oid), uri: iu}
				byURI[iu] = h
				order = append(order, h)
			}
			pu := lit(b["p"])
			if !strings.Contains(pu, ":relation:") {
				continue
			}
			h.neighbors = append(h.neighbors, map[string]string{"rel": nameFromURI(pu), "target": nameFromURI(lit(b["o"]))})
		}
		if len(order) == 0 {
			return jsonResult(map[string]any{"name": name, "neighbors": []any{}, "profile_id": p.ID}), nil
		}
		hitOut := func(h *iHit) map[string]any {
			if h.neighbors == nil {
				h.neighbors = []any{}
			}
			return map[string]any{"ontology_id": h.src, "name": name, "neighbors": h.neighbors}
		}
		if len(order) == 1 {
			out := hitOut(order[0])
			out["profile_id"] = p.ID
			return jsonResult(out), nil
		}
		matches := make([]map[string]any, 0, len(order))
		for _, h := range order {
			matches = append(matches, hitOut(h))
		}
		return jsonResult(map[string]any{"count": len(matches), "matches": matches, "profile_id": p.ID}), nil
	}
}

// handleListConcepts REQ-215 第 6 工具：概念名清单运行时按需拉取
// （guide 紧凑化后指引不含概念枚举，模型先 list_concepts 探明概念再按名查询）。
func (f *Facade) handleListConcepts() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填"), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		q := `PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX owl: <http://www.w3.org/2002/07/owl#>
SELECT DISTINCT ?label WHERE { ?c rdf:type owl:Class ; rdfs:label ?label } ORDER BY ?label`
		bindings, err := f.exec(ctx, "list_concepts", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		all := []string{}
		seen := map[string]bool{}
		for _, b := range bindings {
			if b["label"] == nil {
				continue
			}
			n := lit(b["label"])
			if !seen[n] {
				seen[n] = true
				all = append(all, n)
			}
		}
		limit := 0
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		names := all
		truncated := false
		const hardCap = 500
		if (limit > 0 && len(names) > limit) || len(names) > hardCap {
			cut := limit
			if cut <= 0 || cut > hardCap {
				cut = hardCap
			}
			names = names[:cut]
			truncated = true
		}
		out := map[string]any{"ontology_id": oid, "count": len(names), "concepts": names, "scope": scopeBlock(p)}
		if truncated {
			out["truncated"] = true
		}
		return jsonResult(out), nil
	}
}
