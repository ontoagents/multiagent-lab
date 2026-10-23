// rest_nl2sparql.go NL2SPARQL 受控生成（REQ-333/M133，79 号建议项 A 消费纵深）：
// 自然语言问题 → LLM 生成 SELECT（本体 schema 上下文，沿 REQ-255 cqSparql 的 IRI 模板口径）
// → 只读白名单校验（facade ValidateReadonlySelect 同口径的本地词法实现——生成侧预检省往返，
// 执行侧 runtime-manager 代理同口径权威兜底）→ 失败错误回喂修复 ≤2 轮 → 交付查询。
// 执行与透视由前端携 X-Onto-NL 走既有工作台代理（runtime-manager 落 trace_log 翻译对照）。
// 诚实边界：SELECT-only 不扩写（UPDATE/DELETE 永不入白名单）；生成失败降级返回可懂报错
// 不虚构结果；复杂查询走 SPARQL 工作台（REQ-92）人工路径不变。
package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
	"github.com/xiaoyao/eino-multiagent-lab/pkg/sparqlguard"
)

// ---- 只读白名单（REQ-380/M176 单源化 pkg/sparqlguard——原「facade 同口径本地实现」收口）----

// vetReadonlySelect 词法级只读校验（薄包装；统一 SELECT|ASK 口径——生成面仍只产 SELECT）。
func vetReadonlySelect(query string) error { return sparqlguard.Vet(query) }


// ---- 生成端点 ----

// nl2sparql POST /api/ontology/nl2sparql {ontology_id, question} → {query, rounds, history}。
// 生成 prompt 沿 REQ-255 cqSparql 的 IRI 模板与 schema 清单口径；白名单失败错误回喂 ≤2 轮。
func (s *Server) nl2sparql(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "模型通道未配置（LLM Creator 未装配）"})
		return
	}
	var req struct {
		OntologyID string `json:"ontology_id"`
		Question   string `json:"question"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.OntologyID) == "" || strings.TrimSpace(req.Question) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 与 question 必填"})
		return
	}
	raw, _, err := s.Store.GetArtifact(req.OntologyID, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	sp := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec_json 解析失败: " + err.Error()})
		return
	}
	id := req.OntologyID
	prompt := buildNL2SparqlPrompt(id, sp, req.Question, nil)
	const maxRounds = 3 // 首轮 + 修复 ≤2
	history := make([]string, 0, maxRounds)
	for round := 1; round <= maxRounds; round++ {
		reply, _, err := s.LLM.RawChat(r.Context(), prompt)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "NL2SPARQL 生成失败（模型通道）: " + err.Error()})
			return
		}
		query := extractSparqlReply(reply)
		history = append(history, query)
		if verr := vetReadonlySelect(query); verr != nil {
			if round == maxRounds {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error":   fmt.Sprintf("生成查询未过只读白名单（%d 轮）：%v——请改用 SPARQL 工作台人工编写", round, verr),
					"history": history,
				})
				return
			}
			prompt = buildNL2SparqlPrompt(id, sp, req.Question, []string{fmt.Sprintf("上一稿未过只读白名单：%v。只允许 SELECT 查询（可带 PREFIX），禁止任何变更操作。", verr)})
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "question": req.Question, "query": query, "rounds": round, "history": history})
		return
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "生成循环异常退出"})
}

// buildNL2SparqlPrompt 生成 prompt（schema 清单+IRI 模板+NL 问题+可选修复回喂）。
func buildNL2SparqlPrompt(id string, sp *pkgspec.Spec, question string, fixErrors []string) string {
	var b strings.Builder
	b.WriteString("你是本体查询专家。把用户的自然语言问题翻译为一条只读 SPARQL 1.1 SELECT 查询。\n")
	fmt.Fprintf(&b, "本体 IRI 模板：概念=<urn:o:%s:concept:名称>，关系=<urn:o:%s:relation:名称>，实例=<urn:o:%s:instance:名称>（名称保持原文，URL 无需转义）。\n", id, id, id)
	b.WriteString("可用概念：" + strings.Join(namesOf(sp.Concepts), "、") + "\n")
	b.WriteString("可用关系：" + strings.Join(relNames(sp.Relations), "、") + "\n")
	if len(sp.DataProperties) > 0 {
		dps := make([]string, 0, len(sp.DataProperties))
		for _, d := range sp.DataProperties {
			dps = append(dps, d.Name)
		}
		b.WriteString("可用数据属性（实例属性断言 IRI=<urn:o:" + id + ":attr:名称>）：" + strings.Join(dps, "、") + "\n")
	}
	b.WriteString("模式：直接用「?x a <概念IRI>」取该概念全部实例（导出 TTL 中个体以 rdf:type 挂概念）；对象属性断言为 <实例IRI> <关系IRI> <实例IRI>；实例属性值为 <实例IRI> <attr:属性IRI> 字面量；层级用 rdfs:subClassOf*。\n")
	b.WriteString("硬性要求：只允许 SELECT（可带 PREFIX 声明）；加 LIMIT（≤200）；只输出查询全文，不要 markdown 代码块、不要解释。\n")
	if len(fixErrors) > 0 {
		b.WriteString("\n上一稿存在以下问题，请修正后重新输出完整查询：\n")
		for _, e := range fixErrors {
			b.WriteString("- " + e + "\n")
		}
	}
	b.WriteString("\n自然语言问题：" + question)
	return b.String()
}

// extractSparqlReply 模型回复取查询全文（容忍 markdown 围栏与前后废话）。
func extractSparqlReply(reply string) string {
	t := strings.TrimSpace(reply)
	if idx := strings.Index(t, "```"); idx >= 0 {
		inner := t[idx:]
		inner = strings.TrimPrefix(inner, "```sparql")
		inner = strings.TrimPrefix(inner, "```")
		if end := strings.Index(inner, "```"); end >= 0 {
			inner = inner[:end]
		}
		t = strings.TrimSpace(inner)
	}
	return t
}
