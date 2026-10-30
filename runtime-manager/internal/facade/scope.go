package facade

// REQ-482/M282 查询面组作用域化与消费归因——方案组语义的工具面承载：
// 结构化工具按名跨方案内全部本体匹配 + 每命中标注来源本体（URI 反解 urn:o:{oid}:）；
// sparql_query/list_concepts/check_answerable 响应携组作用域块（union 默认图从
// 「侧漏」转正为文档化组语义，§2.67 批④）。

import (
	"strings"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

// oidFromURI urn:o:{oid}:{kind}:{name} → {oid}（非该形态=空串）。
func oidFromURI(u string) string {
	parts := strings.SplitN(u, ":", 5)
	if len(parts) == 5 && parts[0] == "urn" && parts[1] == "o" {
		return parts[2]
	}
	return ""
}

// sparqlString SPARQL 字符串字面量（转义反斜杠与双引号——label 匹配注入用）。
func sparqlString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// scopeBlock 方案组作用域块（agent 可见方案内本体清单与 GRAPH 限定线索）。
// raw_graphs=urn:onto:{oid}（REQ-320 per-graph 装载）；inferred_graph=装载期物化推断图
//（REQ-481，oxigraph regular 方案存在；fuseki 查询期推理无独立推断图——命名规则同 manager）。
func scopeBlock(p *store.Profile) map[string]any {
	ids := p.LoadedOntologyIDs()
	graphs := make([]string, 0, len(ids))
	for _, id := range ids {
		graphs = append(graphs, "urn:onto:"+id)
	}
	inferred := "urn:inferred:assembly"
	if len(ids) == 1 {
		inferred = "urn:inferred:" + ids[0]
	}
	return map[string]any{
		"profile_id":     p.ID,
		"ontologies":     ids,
		"raw_graphs":     graphs,
		"inferred_graph": inferred,
		"scope_note": "default 图=方案内全部本体 raw 图（+推断图）的并置 union（方案组语义，REQ-482）；" +
			"逐行来源可用 GRAPH <urn:onto:{oid}> 限定复查",
	}
}

// hitSource 命中行来源标注（ontology_id 反解失败时回落查询入参 oid——IRI 形态异常的诚实兜底）。
func hitSource(u, fallback string) string {
	if id := oidFromURI(u); id != "" {
		return id
	}
	return fallback
}
