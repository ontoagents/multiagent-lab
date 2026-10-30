// rest_consumption.go 消费域端点（REQ-340 A1 自 rest.go 拆分，80 号 A1）：CQ→SPARQL 翻译（REQ-255）/LOV 词表搜索（REQ-171）/生命周期 plan/apply（REQ-155）。
package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ontoagents/ontoagents/ontology-service/internal/toolchain"
	"github.com/ontoagents/ontoagents/ontology-service/internal/vocabsearch"
	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func (s *Server) vocabSearch(w http.ResponseWriter, r *http.Request) {
	if s.vocab == nil {
		s.vocab = vocabsearch.New(os.Getenv("LOV_API_BASE"))
	}
	q := r.URL.Query().Get("q")
	if strings.TrimSpace(q) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q 必填"})
		return
	}
	// REQ-261/M259：source 参数路由外部词表通道（缺省 lov 存量零变化；ols=EBI OLS 第二通道）
	var cards []vocabsearch.Card
	var err error
	switch strings.ToLower(r.URL.Query().Get("source")) {
	case "ols":
		if s.vocab.OLSBase == "" {
			s.vocab.OLSBase = vocabsearch.DefaultOLS
		}
		cards, err = s.vocab.SearchOntologies(r.Context(), q)
	default:
		cards, err = s.vocab.Search(r.Context(), q)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"q": q, "count": len(cards), "results": cards})
}

func (s *Server) cqSparql(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	raw, _, err := s.Store.GetArtifact(id, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	sp := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec_json 解析失败: " + err.Error()})
		return
	}
	// REQ-342/OC-2：body.cqs 可选——双轨并轨只翻译存疑项（缺省=全量 spec.CQ 沿既有口径）
	var body struct {
		CQs []string `json:"cqs"`
	}
	_ = decodeJSON(r, &body)
	target := sp.CQ
	if len(body.CQs) > 0 {
		target = body.CQs
	}
	if len(target) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该本体无 CQ（能力问题）——先在 Spec 编辑「CQ」面板录入"})
		return
	}
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "模型通道未配置（LLM Creator 未装配）"})
		return
	}
	var b strings.Builder
	b.WriteString("你是本体查询专家。把每条能力问题（CQ）翻译为一条只读 SPARQL 1.1 SELECT 查询。\n")
	b.WriteString("本体 IRI 模板：概念=<urn:o:" + id + ":concept:名称>，关系=<urn:o:" + id + ":relation:名称>，实例=<urn:o:" + id + ":instance:名称>（名称保持原文，URL 无需转义）。\n")
	b.WriteString("可用概念：" + strings.Join(namesOf(sp.Concepts), "、") + "\n")
	b.WriteString("可用关系：" + strings.Join(relNames(sp.Relations), "、") + "\n")
	b.WriteString("模式：直接用「?x a <概念IRI>」取该概念全部实例（导出 TTL 中个体以 rdf:type 挂概念）；对象属性断言为 <实例IRI> <关系IRI> <实例IRI>。\n")
	b.WriteString("只输出 JSON 数组，每项 {\"cq\":\"原问题\",\"sparql\":\"SELECT ...\"}，不要 markdown 代码块。\n\nCQ 清单：\n")
	for i, cq := range target {
		fmt.Fprintf(&b, "%d. %s\n", i+1, cq)
	}
	reply, _, err := s.LLM.RawChat(r.Context(), b.String())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "CQ 翻译失败: " + err.Error()})
		return
	}
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(strings.TrimPrefix(reply, "```json"), "```")
	reply = strings.TrimSuffix(strings.TrimSpace(reply), "```")
	var items []cqSparqlItem
	if err := json.Unmarshal([]byte(reply), &items); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "翻译结果解析失败（非 JSON 数组）: " + err.Error()})
		return
	}
	if items == nil {
		items = []cqSparqlItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "items": items, "count": len(items)})
}

func namesOf(cs []pkgspec.Concept) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func relNames(rs []pkgspec.Relation) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

// saveCQTestReport PUT /api/ontologies/{id}/cq-test-report（REQ-342/OC-2）：
// 双轨完整报告落 artifact（快筛段=后端 coverage job 已落；SPARQL 精判段由前端并轨编排后
// 整体覆盖保存，含三级覆盖量化）。校验 JSON 合法+必填字段；回归基准=报告带 base_version，
// 版本超前即过期提示重跑。
func (s *Server) saveCQTestReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !json.Valid(body) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "报告须为合法 JSON"})
		return
	}
	var probe struct {
		Summary *json.RawMessage `json:"summary"`
	}
	if json.Unmarshal(body, &probe) != nil || probe.Summary == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "报告缺 summary 段（快筛结果必填——先在覆盖卡完成一次快筛）"})
		return
	}
	if err := s.Store.PutArtifact(id, "cq-test-report", string(body), false); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "size": len(body)})
}

// qualityConfigGet / qualityConfigPut REQ-156：本体级 strict 门禁开关读写。

func (s *Server) lifecyclePlan(w http.ResponseWriter, r *http.Request) {
	versions, err := s.Store.CurrentVersions()
	if err != nil {
		writeErr(w, err)
		return
	}
	plan, err := toolchain.Plan(r.Context(), s.LLM.PlatformURL, versions)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// lifecycleApply REQ-155 阶段二：逐项执行计划动作（start/reload）。
func (s *Server) lifecycleApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actions []toolchain.LifecycleAction `json:"actions"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	results := make([]map[string]any, 0, len(body.Actions))
	okN := 0
	for _, a := range body.Actions {
		aerr := toolchain.ApplyAction(r.Context(), s.LLM.PlatformURL, a)
		if aerr != nil {
			results = append(results, map[string]any{"profile_id": a.ProfileID, "action": a.Action, "ok": false, "error": aerr.Error()})
			continue
		}
		okN++
		results = append(results, map[string]any{"profile_id": a.ProfileID, "action": a.Action, "ok": true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": okN, "total": len(body.Actions), "results": results})
}

// mergeIncoming 解析合并请求体：multipart 文件（filename+content 走 importer.Import）或 JSON {filename,content} / {spec}；
// strategy/prefix 取表单值或 JSON 字段（REQ-157 审查向导走 JSON）。
