package companion

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// REQ-170/M28 薄本体 SPARQL 生成（纯函数，便于零依赖单测）。
// 数据面：种子 5 骨架类（bot: 前缀）+ agent named graph 隔离（REQ-211：一 agent 一图，
// 该 agent 全部会话与参与的项目会话共享；会话维度仅保留在候选 provenance 与抽取游标）
// + 失效化而非删除。
// 实体 URI 规则：http://eino-lab/e/{slug(label)}——同名 slug 归并为同一实体（薄版口径）。
// 关系边 = bot:Relation 实例节点（bot:subject/bot:object/bot:relName），
// 矛盾（同主体+同关系名+新目标）→ 旧边节点 bot:invalidAt 标记，保留可查历史。
// ---------------------------------------------------------------------------

const (
	// BotNS 薄本体词表命名空间（方案 §五）。
	BotNS = "http://eino-lab/ontology/thin/"
	// GraphNS 伴生图命名空间：GRAPH <…/graph/agt-{agentID}>（REQ-211 前为 conv-{convID}，
	// 存量会话图由启动迁移 ADD TO 聚合后 DROP）。
	GraphNS = "http://eino-lab/graph/"
	// EntityNS 实体命名空间。
	EntityNS = "http://eino-lab/e/"
	// MsgNS 消息溯源命名空间。
	MsgNS = "http://eino-lab/msg/"
)

// GraphURI 伴生图 URI（REQ-216：换绑本体伴生子图——一绑定本体一图，多 agent 绑同一本体
// 共享沉淀〔隔离边界 agent→本体升维〕；宿主方案引擎同时承载本体 default graph 与伴生
// named graph，互不干扰）。REQ-216 前为 agt-{agentID}（REQ-211）→ conv-{convID}（M28），
// 两代旧图由启动迁移收敛（conv 迁移已完成；agt 图跨实例复制见 MigrateAgentGraphsToOntology）。
func GraphURI(ontologyID string) string { return fmt.Sprintf("%sont-%s", GraphNS, ontologyID) }

// LegacyAgentGraphURI 旧 agent 图 URI（REQ-216 启动迁移源：:9199 独立实例 agt-{id} 图
// SPARQL 层复制到方案引擎 ont-{id} 后 DROP）。
func LegacyAgentGraphURI(agentID string) string { return fmt.Sprintf("%sagt-%s", GraphNS, agentID) }

// LegacyConvGraphURI 旧会话图 URI（REQ-211 启动迁移源：ADD TO agent 图后 DROP）。
func LegacyConvGraphURI(convID string) string { return fmt.Sprintf("%sconv-%s", GraphNS, convID) }

// AddGraph SPARQL 1.1 图管理：源图全量并入目标图（源图不存在按空图处理，幂等安全）。
func AddGraph(fromURI, toURI string) string {
	return fmt.Sprintf("ADD <%s> TO <%s>", fromURI, toURI)
}

// EntityURI 实体 URI（slug 归并）。
func EntityURI(label string) string { return EntityNS + Slug(label) }

// MessageURI 消息溯源 URI。
func MessageURI(msgID string) string { return MsgNS + msgID }

// Slug 实体标签 → URI 片段（安全字符保留，其余转下划线；空串落占位）。
func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '_' || r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff: // CJK 统一表意文字：URI 保留（oxigraph/IRI 合法）
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "unnamed"
	}
	return out
}

// turtleEscape Turtle 字面量转义（反斜杠/引号/换行/回车/制表）。
func turtleEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`,
	)
	return r.Replace(s)
}

// xsdTime RFC3339 → xsd 时间字面量值。
func xsdTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// SeedSchema 种子 5 骨架类（方案 §五，一次性预置；幂等 INSERT）。
// 薄版 schema 冻结：类型发现 P3 前不开放（报告 R1 漂移风险的结构性免疫）。
func SeedSchema() string {
	return `PREFIX bot: <` + BotNS + `>
PREFIX owl: <http://www.w3.org/2002/07/owl#>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
INSERT DATA {
  bot:Concept a owl:Class ; rdfs:label "概念" .
  bot:Relation a owl:Class ; rdfs:label "关系陈述" .
  bot:Event a owl:Class ; rdfs:label "事件" .
  bot:Source a owl:Class ; rdfs:label "来源" .
  bot:Agent a owl:Class ; rdfs:label "参与智能体" .
}`
}

// nodeKind map 候选 kind → 种子类。
func nodeKind(kind string) string {
	switch kind {
	case "relation":
		return "bot:Relation"
	case "event":
		return "bot:Event"
	default:
		return "bot:Concept"
	}
}

// ValidFromOf REQ-368/M166：timeScope 宽松解析为有效性时间起点（vt，xsd:date）——
// "2024"→2024-01-01；"2024-03"/"2024-Q1"→2024-03-01；"2024-03-05"原样；其余（自然语言/区间
// 形态）不可解析=空串=不落 bot:validFrom（诚实缺省，vt 与 tt 语义分离见包注）。
func ValidFromOf(timeScope string) string {
	ts := strings.TrimSpace(timeScope)
	if ts == "" {
		return ""
	}
	// 完整日期
	if len(ts) == 10 && ts[4] == '-' && ts[7] == '-' {
		if _, err := time.Parse("2006-01-02", ts); err == nil {
			return ts
		}
	}
	// 年-月
	if len(ts) == 7 && ts[4] == '-' {
		if _, err := time.Parse("2006-01", ts); err == nil {
			return ts + "-01"
		}
	}
	// 季度（Q1~Q4，大小写不敏感）
	if len(ts) == 7 && (ts[5] == 'Q' || ts[5] == 'q') && ts[6] >= '1' && ts[6] <= '4' {
		if _, err := time.Parse("2006", ts[:4]); err == nil {
			return fmt.Sprintf("%s-%02d-01", ts[:4], (ts[6]-'1')*3+1)
		}
	}
	// 纯年份
	if len(ts) == 4 {
		if _, err := time.Parse("2006", ts); err == nil {
			return ts + "-01-01"
		}
	}
	return ""
}

// InsertNodeTriples 概念/事件入图（同名 slug 归并；自带溯源三件套；timeScope 非空落 bot:timeScope，
// REQ-368：可解析为日期时另落 bot:validFrom=vt 有效性时间起点）。
func InsertNodeTriples(graphID, candID, kind, label, definition, timeScope string, confidence float64, msgID string, at time.Time) string {
	g := GraphURI(graphID)
	e := EntityURI(label)
	var b strings.Builder
	b.WriteString("PREFIX bot: <" + BotNS + ">\nPREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nPREFIX prov: <http://www.w3.org/ns/prov#>\n")
	fmt.Fprintf(&b, "INSERT DATA {\n  GRAPH <%s> {\n", g)
	fmt.Fprintf(&b, "    <%s> a %s ;\n      rdfs:label %q ;\n", e, nodeKind(kind), turtleEscape(label))
	if definition != "" {
		fmt.Fprintf(&b, "      bot:definition %q ;\n", turtleEscape(definition))
	}
	if timeScope != "" {
		fmt.Fprintf(&b, "      bot:timeScope %q ;\n", turtleEscape(timeScope))
	}
	if vf := ValidFromOf(timeScope); vf != "" {
		fmt.Fprintf(&b, "      bot:validFrom %q ;\n", vf)
	}
	fmt.Fprintf(&b, "      bot:confidence %.2f ;\n      bot:extractedFrom <%s> ;\n      prov:generatedAtTime %q ;\n      prov:wasGeneratedBy <%sactivity-%s> .\n",
		confidence, MessageURI(msgID), xsdTime(at), EntityNS, candID)
	b.WriteString("  }\n}")
	return b.String()
}

// InsertRelationTriples 关系边入图：边 = bot:Relation 实例节点（subject/object/relName）。
// 冲突语义：同 subject + 同 relName + 不同 object 的旧边由调用方先发 InvalidateEdge。
func InsertRelationTriples(graphID, candID, relName, sourceLabel, targetLabel, definition string, confidence float64, msgID string, at time.Time) string {
	g := GraphURI(graphID)
	edge := EdgeURI(candID)
	var b strings.Builder
	b.WriteString("PREFIX bot: <" + BotNS + ">\nPREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nPREFIX prov: <http://www.w3.org/ns/prov#>\n")
	fmt.Fprintf(&b, "INSERT DATA {\n  GRAPH <%s> {\n", g)
	fmt.Fprintf(&b, "    <%s> a bot:Relation ;\n      rdfs:label %q ;\n      bot:subject <%s> ;\n      bot:object <%s> ;\n      bot:relName %q ;\n",
		edge, turtleEscape(relName), EntityURI(sourceLabel), EntityURI(targetLabel), turtleEscape(relName))
	if definition != "" {
		fmt.Fprintf(&b, "      bot:definition %q ;\n", turtleEscape(definition))
	}
	// REQ-227①：首建即计数（确认次数语义——印证聚合在其上递增）
	fmt.Fprintf(&b, "      bot:confidence %.2f ;\n      bot:confirmCount 1 ;\n      bot:extractedFrom <%s> ;\n      prov:generatedAtTime %q ;\n      prov:wasGeneratedBy <%sactivity-%s> .\n",
		confidence, MessageURI(msgID), xsdTime(at), EntityNS, candID)
	// 边两端实体不存在则薄建（label 锚定，同名归并）
	fmt.Fprintf(&b, "    <%s> a bot:Concept ; rdfs:label %q .\n", EntityURI(sourceLabel), turtleEscape(sourceLabel))
	fmt.Fprintf(&b, "    <%s> a bot:Concept ; rdfs:label %q .\n", EntityURI(targetLabel), turtleEscape(targetLabel))
	b.WriteString("  }\n}")
	return b.String()
}

// EdgeURI 关系边节点 URI。
func EdgeURI(candID string) string { return EntityNS + "edge-" + candID }

// MergeNodeData REQ-298：已存在实体的印证式合写——仅当缺失 definition 时补齐
//（不重复写 confidence/generatedAtTime/wasGeneratedBy 多值，治内容清单组合行爆炸的根源）。
func MergeNodeData(graphID, candID, kind, label, definition string, confidence float64, msgID string, at time.Time) string {
	g := GraphURI(graphID)
	e := EntityURI(label)
	if definition != "" {
		return fmt.Sprintf(`PREFIX bot: <%s>
INSERT DATA {
  GRAPH <%s> {
    <%s> bot:definition %q .
  }
}`, BotNS, g, e, turtleEscape(definition))
	}
	return fmt.Sprintf(`PREFIX bot: <%s>
INSERT DATA { GRAPH <%s> { } }`, BotNS, g)
}

// NodeExists 实体存在性检查（Ask——入图侧收敛判定）。
func NodeExists(graphID, label string) string {
	return fmt.Sprintf(`PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
ASK { GRAPH <%s> { <%s> rdfs:label [] } }`, GraphURI(graphID), EntityURI(label))
}

// FindActiveEdge 查同主体+同关系名且未失效的旧边（矛盾检测前置）。
func FindActiveEdge(graphID, sourceLabel, relName string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
SELECT ?edge WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject <%s> ; bot:relName %q .
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
  }
} LIMIT 1`, BotNS, GraphURI(graphID), EntityURI(sourceLabel), turtleEscape(relName))
}

// FindActiveEdgeWithObject REQ-227①：活跃旧边连同客体 URI（印证聚合的同事实判定——
// object 相同=同一事实被再次确认走聚合计数；不同=矛盾走失效化既有路径）。
func FindActiveEdgeWithObject(graphID, sourceLabel, relName string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
SELECT ?edge ?o WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject <%s> ; bot:relName %q ; bot:object ?o .
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
  }
} LIMIT 1`, BotNS, GraphURI(graphID), EntityURI(sourceLabel), turtleEscape(relName))
}

// AggregateConfirmCountRead 读边当前印证计数（无计数=0）。
func AggregateConfirmCountRead(edgeURI string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
SELECT ?c WHERE { GRAPH ?g { <%s> bot:confirmCount ?c } } LIMIT 1`, BotNS, edgeURI)
}

// AggregateConfirmCountWrite REQ-227①：同事实再确认——confirmCount 覆盖写（读改写两步，
// DELETE DATA 需具体旧值不支持变量；伴生写路径低频且同本体确认经宿主引擎串行）+ 新 activity
// 挂多值 provenance（每次确认的候选活动全部可溯，图内保持单边）。oldCount=0 时省 DELETE。
func AggregateConfirmCountWrite(graphID, edgeURI, candID string, oldCount, newCount int, at time.Time) string {
	del := ""
	if oldCount > 0 {
		del = fmt.Sprintf("DELETE DATA { GRAPH <%s> { <%s> bot:confirmCount %d . } };\n", GraphURI(graphID), edgeURI, oldCount)
	}
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <http://www.w3.org/ns/prov#>
%sINSERT DATA {
  GRAPH <%s> {
    <%s> bot:confirmCount %d ;
      prov:wasGeneratedBy <%sactivity-%s> ;
      bot:lastConfirmedAt %q .
  }
}`, BotNS, del, GraphURI(graphID), edgeURI, newCount, EntityNS, candID, xsdTime(at))
}

// InvalidateEdge 旧边失效化（bot:invalidAt 标记而非删除，保留可查历史）。
func InvalidateEdge(graphID, edgeURI string, at time.Time) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
INSERT DATA {
  GRAPH <%s> {
    <%s> bot:invalidAt %q .
  }
}`, BotNS, GraphURI(graphID), edgeURI, xsdTime(at))
}

// SelectGraphTriples 会话图全量读取（确认后回显/冒烟核对用）。
func SelectGraphTriples(graphID string) string {
	return fmt.Sprintf(`PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?s ?p ?o WHERE {
  GRAPH <%s> { ?s ?p ?o . FILTER(?p != rdfs:label) }
} ORDER BY ?s LIMIT 500`, GraphURI(graphID))
}

// SelectGraphAllTriples 子图全量三元组（无过滤、高上限——快照导出/迁移复制用；高限防失控）。
func SelectGraphAllTriples(graphID string) string {
	return fmt.Sprintf(`SELECT ?s ?p ?o WHERE {
  GRAPH <%s> { ?s ?p ?o }
} ORDER BY ?s LIMIT 200000`, GraphURI(graphID))
}

// CountGraphTriples 子图三元组计数（快照回灌的重建检测探针）。
func CountGraphTriples(graphID string) string {
	return fmt.Sprintf(`SELECT (COUNT(*) AS ?n) WHERE { GRAPH <%s> { ?s ?p ?o } }`, GraphURI(graphID))
}

// MarkAutoConfirmed REQ-187：自动入图溯源标记（bot:autoConfirmed——区分于人工确认）。
func MarkAutoConfirmed(graphID, candID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
INSERT DATA {
  GRAPH <%s> {
    <%s> bot:autoConfirmed true .
  }
}`, BotNS, GraphURI(graphID), EdgeURI(candID))
}

// SelectNodes 会话图节点（概念/事件实体，含定义/置信度/入图时间——REQ-154 成长可视化数据源）。
func SelectNodes(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX prov: <http://www.w3.org/ns/prov#>
SELECT ?kind ?label (SAMPLE(?d) AS ?def) (MAX(?c) AS ?conf) (SAMPLE(?t) AS ?tscope) (MIN(?at) AS ?at) WHERE {
  GRAPH <%s> {
    ?s a ?kind ; rdfs:label ?label .
    FILTER(?kind IN (bot:Concept, bot:Event))
    OPTIONAL { ?s bot:definition ?d }
    OPTIONAL { ?s bot:confidence ?c }
    OPTIONAL { ?s bot:timeScope ?t }
    OPTIONAL { ?s prov:generatedAtTime ?at }
  }
} GROUP BY ?s ?kind ?label ORDER BY ?at LIMIT 300`, BotNS, GraphURI(graphID))
}

// SelectEdges 会话图活跃关系边（两端标签 + 关系名 + 入图时间；失效边不返回）。
func SelectEdges(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX prov: <http://www.w3.org/ns/prov#>
SELECT ?src ?rel ?dst ?at ?count WHERE {
  GRAPH <%s> {
    ?e a bot:Relation ; bot:relName ?rel ; bot:subject ?s ; bot:object ?o ; prov:generatedAtTime ?at .
    ?s rdfs:label ?src .
    ?o rdfs:label ?dst .
    OPTIONAL { ?e bot:confirmCount ?count }
    FILTER NOT EXISTS { ?e bot:invalidAt ?any }
  }
} ORDER BY ?at LIMIT 300`, BotNS, GraphURI(graphID))
}

// SelectLabels 伴生图概念实体标签清单（状态回显 + KG 检索源匹配 + REQ-194① 对齐清单；
// 限定 bot:Concept——bot:Relation 边节点同样带 rdfs:label（关系名），不属实体）。
// REQ-286 B3：别名平铺——bot:alias 一并返回（召回/对齐面含别名，「K8s」等变体可匹配）。
// REQ-287④ LIMIT·缓存治理：300→500（词法/FTS 回填/对齐面共用此清单；上限即治理口径的显式化）。
func SelectLabels(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?label WHERE {
  GRAPH <%s> {
    { ?s a bot:Concept ; rdfs:label ?label }
    UNION
    { ?s a bot:Concept ; bot:alias ?label }
  }
} ORDER BY ?label LIMIT 500`, BotNS, GraphURI(graphID))
}

// SelectEntityDefs REQ-287②④：实体 label→definition 全量投影（定义文本向量与 FTS 回填语料；
// 定义是区分同义实体的关键信号——隐式召回失败的对症语料）。LIMIT 500 与 SelectLabels 同口径。
func SelectEntityDefs(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?label (SAMPLE(?d) AS ?def) WHERE {
  GRAPH <%s> {
    ?s a bot:Concept ; rdfs:label ?label .
    OPTIONAL { ?s bot:definition ?d }
  }
} GROUP BY ?s ?label ORDER BY ?label LIMIT 500`, BotNS, GraphURI(graphID))
}

// AddAlias REQ-286 B3：实体别名挂接（bot:alias 三元组——变体不删不丢，同实体多叫法）。
func AddAlias(graphID, entityLabel, alias string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
INSERT DATA {
  GRAPH <%s> {
    <%s> bot:alias %q ; a bot:Concept .
  }
}`, BotNS, GraphURI(graphID), EntityURI(entityLabel), turtleEscape(alias))
}

// SelectAliasOwner REQ-286 B3：别名→主实体标签解析（召回命中别名后回溯主实体）。
func SelectAliasOwner(graphID, alias string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?label WHERE {
  GRAPH <%s> { ?s bot:alias %q ; rdfs:label ?label }
} LIMIT 1`, BotNS, GraphURI(graphID), turtleEscape(alias))
}

// SelectAliasPairs REQ-313：别名全量投影（owner label→alias，画像语料聚合用——
// 单查询零新表，查询时聚合口径；LIMIT 500 与 SelectLabels 同口径）。
func SelectAliasPairs(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?label ?alias WHERE {
  GRAPH <%s> { ?s a bot:Concept ; bot:alias ?alias ; rdfs:label ?label }
} ORDER BY ?label LIMIT 500`, BotNS, GraphURI(graphID))
}

// RenameEntity REQ-286 C1：实体重命名（迁移式）——返回顺序操作序列（多次独立 Update 执行；
// oxigraph 对 `;` 串联多操作更新执行不完整——真机实证只落首段，必须拆步）。
// 步骤：①新实体（label+类型）+旧名转别名 ②边主体位重定向 ③边客体位重定向 ④旧主体三元组清理。
func RenameEntity(graphID, oldLabel, newLabel string) []string {
	oldURI, newURI := EntityURI(oldLabel), EntityURI(newLabel)
	g := GraphURI(graphID)
	step1 := fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
INSERT DATA { GRAPH <%s> { <%s> rdfs:label %q ; a bot:Concept . <%s> bot:alias %q . } }`,
		BotNS, g, newURI, turtleEscape(newLabel), newURI, turtleEscape(oldLabel))
	step2 := fmt.Sprintf(`PREFIX bot: <%s>
DELETE { GRAPH <%s> { ?edge bot:subject <%s> } }
INSERT { GRAPH <%s> { ?edge bot:subject <%s> } }
WHERE { GRAPH <%s> { ?edge a bot:Relation ; bot:subject <%s> } }`,
		BotNS, g, oldURI, g, newURI, g, oldURI)
	step3 := fmt.Sprintf(`PREFIX bot: <%s>
DELETE { GRAPH <%s> { ?edge bot:object <%s> } }
INSERT { GRAPH <%s> { ?edge bot:object <%s> } }
WHERE { GRAPH <%s> { ?edge a bot:Relation ; bot:object <%s> } }`,
		BotNS, g, oldURI, g, newURI, g, oldURI)
	step4 := fmt.Sprintf(`DELETE { GRAPH <%s> { <%s> ?p ?o } }
WHERE { GRAPH <%s> { <%s> ?p ?o } }`, g, oldURI, g, oldURI)
	return []string{step1, step2, step3, step4}
}

// DeleteEntity REQ-286 C1：删除实体（顺序步骤：边主体位整边删→边客体位整边删→主体三元组删）。
func DeleteEntity(graphID, label string) []string {
	e := EntityURI(label)
	g := GraphURI(graphID)
	step1 := fmt.Sprintf(`PREFIX bot: <%s>
DELETE { GRAPH <%s> { ?e ?p ?o } }
WHERE { GRAPH <%s> { ?e a bot:Relation ; ?p ?o . ?e bot:subject <%s> } }`, BotNS, g, g, e)
	step2 := fmt.Sprintf(`PREFIX bot: <%s>
DELETE { GRAPH <%s> { ?e ?p ?o } }
WHERE { GRAPH <%s> { ?e a bot:Relation ; ?p ?o . ?e bot:object <%s> } }`, BotNS, g, g, e)
	step3 := fmt.Sprintf(`DELETE { GRAPH <%s> { <%s> ?p ?o } }
WHERE { GRAPH <%s> { <%s> ?p ?o } }`, g, e, g, e)
	return []string{step1, step2, step3}
}

// SelectEdgeByURI REQ-286 C1：按边 URI 取边（存在性与主体/客体标签，编辑前校验）。
func SelectEdgeByURI(graphID, edgeURI string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?src ?rel ?dst WHERE {
  GRAPH <%s> {
    <%s> a bot:Relation ; bot:relName ?rel ; bot:subject ?s ; bot:object ?o .
    ?s rdfs:label ?src . ?o rdfs:label ?dst .
  }
} LIMIT 1`, BotNS, GraphURI(graphID), edgeURI)
}

// DeleteEdgeByURI REQ-286 C1：删除关系边（整边删除，含 confirmCount 等附属）。
func DeleteEdgeByURI(graphID, edgeURI string) string {
	return fmt.Sprintf(`DELETE { GRAPH <%s> { <%s> ?p ?o } }
WHERE { GRAPH <%s> { <%s> ?p ?o } }`, GraphURI(graphID), edgeURI, GraphURI(graphID), edgeURI)
}

// SelectRelationEdges 活跃关系边含边 URI（前端关系表行操作定位用；REQ-286 C1）。
// 与 SelectEdges 同形状增 ?edge 列——前端只增不改兼容。
func SelectRelationEdges(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX prov: <http://www.w3.org/ns/prov#>
SELECT ?edge ?src ?rel ?dst (MIN(?at) AS ?at) (SUM(?cc) AS ?count) WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:relName ?rel ; bot:subject ?s ; bot:object ?o ; prov:generatedAtTime ?at .
    ?s rdfs:label ?src .
    ?o rdfs:label ?dst .
    OPTIONAL { ?edge bot:confirmCount ?cc }
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
  }
} GROUP BY ?edge ?src ?rel ?dst ORDER BY ?at LIMIT 300`, BotNS, GraphURI(graphID))
}

// SelectRelationsAll REQ-299：全量关系边含失效边（诊断数据面——invalidAt 标记可选返回）。
func SelectRelationsAll(graphID string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX prov: <http://www.w3.org/ns/prov#>
SELECT ?edge ?src ?rel ?dst ?at ?cc ?c ?inv WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:relName ?rel ; bot:subject ?s ; bot:object ?o ; prov:generatedAtTime ?at .
    ?s rdfs:label ?src .
    ?o rdfs:label ?dst .
    OPTIONAL { ?edge bot:confirmCount ?cc }
    OPTIONAL { ?edge bot:confidence ?c }
    OPTIONAL { ?edge bot:invalidAt ?iv }
    BIND(BOUND(?iv) AS ?inv)
  }
} ORDER BY ?at LIMIT 500`, BotNS, GraphURI(graphID))
}

// DropGraph 伴生图整体摘除（低侵入三原则③；REQ-211 起作用域=智能体）。
func DropGraph(graphID string) string {
	return fmt.Sprintf(`DROP SILENT GRAPH <%s>`, GraphURI(graphID))
}

// DropGraphByURI 按显式 URI 摘除（REQ-211 迁移专用：旧 conv 图清理——GraphURI 已换轨，
// 迁移不能走 DropGraph（会生成 agt- URI 落空））。
func DropGraphByURI(uri string) string {
	return fmt.Sprintf(`DROP SILENT GRAPH <%s>`, uri)
}

// SelectEntityInfo 实体定义/置信度/时点（KG 检索源并入：命中实体详情，OPTIONAL 兼容薄建实体）。
func SelectEntityInfo(graphID, label string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?def ?conf ?tscope WHERE {
  GRAPH <%s> {
    <%s> rdfs:label %q .
    OPTIONAL { <%s> bot:definition ?def }
    OPTIONAL { <%s> bot:confidence ?conf }
    OPTIONAL { <%s> bot:timeScope ?tscope }
  }
} LIMIT 1`, BotNS, GraphURI(graphID), EntityURI(label), turtleEscape(label), EntityURI(label), EntityURI(label), EntityURI(label))
}

// SelectEntityEdges 实体的活跃关系边（双向：作为主体或客体；失效边不召回）。
func SelectEntityEdges(graphID, label string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?relName ?otherLabel ?dir WHERE {
  GRAPH <%s> {
    {
      ?edge bot:subject <%s> ; bot:relName ?relName ; bot:object ?other .
      BIND("out" AS ?dir)
    } UNION {
      ?edge bot:object <%s> ; bot:relName ?relName ; bot:subject ?other .
      BIND("in" AS ?dir)
    }
    ?other rdfs:label ?otherLabel .
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
  }
} ORDER BY ?relName LIMIT 20`, BotNS, GraphURI(graphID), EntityURI(label), EntityURI(label))
}

// SelectEntityNeighborhood 实体 2 跳邻域（REQ-194②召回增强）：
//
//	hop=1 直接边（双向，与 SelectEntityEdges 同语义）；
//	hop=2 经中间实体的链式边（E —rel1→ m —rel2→ o2 出向链 / o2 —rel2→ m —rel1→ E 入向链），
//	relName 以 CONCAT 拼链式可读文本（如「引发→HPA 调整·依赖」），dir 取首边方向。
//
// 失效边两跳均过滤；自环（m=E / 终点=E）排除；每实体边总量由调用方限流（≤8）。
func SelectEntityNeighborhood(graphID, label string) string {
	e := EntityURI(label)
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT DISTINCT ?relName ?otherLabel ?dir ?hop WHERE {
  GRAPH <%s> {
    {
      ?e1 bot:subject <%s> ; bot:relName ?r1 ; bot:object ?o1 .
      ?o1 rdfs:label ?otherLabel .
      FILTER NOT EXISTS { ?e1 bot:invalidAt ?ix1 }
      BIND(?r1 AS ?relName) BIND("out" AS ?dir) BIND(1 AS ?hop)
    } UNION {
      ?e1 bot:object <%s> ; bot:relName ?r1 ; bot:subject ?s1 .
      ?s1 rdfs:label ?otherLabel .
      FILTER NOT EXISTS { ?e1 bot:invalidAt ?ix2 }
      BIND(?r1 AS ?relName) BIND("in" AS ?dir) BIND(1 AS ?hop)
    } UNION {
      ?e1 bot:subject <%s> ; bot:relName ?r1 ; bot:object ?m .
      ?m rdfs:label ?mLabel .
      ?e2 bot:subject ?m ; bot:relName ?r2 ; bot:object ?o2 .
      ?o2 rdfs:label ?otherLabel .
      FILTER NOT EXISTS { ?e1 bot:invalidAt ?ix3 } FILTER NOT EXISTS { ?e2 bot:invalidAt ?iy1 }
      FILTER(?m != <%s>) FILTER(?o2 != <%s>)
      BIND(CONCAT(?r1, "→", ?mLabel, "·", ?r2) AS ?relName)
      BIND("out" AS ?dir) BIND(2 AS ?hop)
    } UNION {
      ?e2 bot:subject ?x2 ; bot:relName ?r2 ; bot:object ?m .
      ?m rdfs:label ?mLabel .
      ?e1 bot:subject ?m ; bot:relName ?r1 ; bot:object <%s> .
      ?x2 rdfs:label ?otherLabel .
      FILTER NOT EXISTS { ?e1 bot:invalidAt ?ix4 } FILTER NOT EXISTS { ?e2 bot:invalidAt ?iy2 }
      FILTER(?m != <%s>) FILTER(?x2 != <%s>)
      BIND(CONCAT(?r2, "→", ?mLabel, "·", ?r1) AS ?relName)
      BIND("in" AS ?dir) BIND(2 AS ?hop)
    }
  }
} ORDER BY ?hop LIMIT 20`,
		BotNS, GraphURI(graphID), e, e, e, e, e, e, e, e)
}

// SelectSubjectActiveEdges 实体作为主体的全部活跃边（REQ-194⑤语义矛盾检测数据面：
// 新断言与同主体既有断言拼 prompt 交 LLM 二分类；含边 URI 供冲突失效化定位）。
func SelectSubjectActiveEdges(graphID, subjectLabel string) string {
	return fmt.Sprintf(`PREFIX bot: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?edge ?relName ?objLabel WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ; bot:subject <%s> ; bot:relName ?relName ; bot:object ?o .
    ?o rdfs:label ?objLabel .
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
  }
} ORDER BY ?relName LIMIT 30`, BotNS, GraphURI(graphID), EntityURI(subjectLabel))
}
