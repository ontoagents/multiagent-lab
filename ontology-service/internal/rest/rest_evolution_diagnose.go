package rest

// REQ-207 余项/M43 深化（「领取开发所有剩余需求」批第二轮）：进化闭环「诊断→归因→补丁」
// LLM 自动产出端点——一期（M43）候选仅能经 REST 人工提交（04 §6.3 诚实遗留项），本件把
// 三步产出一键化：诊断步直接消费 cqbench 基准（E5 注兑现），归因步 LLM 三层定位（每次只动
// 一层），补丁步产出类型化编辑并**机械校验 Evidence 锚定**（防本体幻觉——evidence 必须引用
// 失评测信号，否则拒绝产出候选）。
//
// POST /api/ontology/evolution/diagnose {ontology_id, dataset?} → 202 {job_id}
// GET  /api/ai-draft-jobs/{id}                          → 轮询（job.diagnose 透出）
//
// 一期口径不变：本端点仍是**人工触发**（无人值守自动循环不做）；产出候选后门控（gate）/
// 采纳（accept）/拒绝（reject）沿既有 REST 人工链——LLM 产物不自动过门控不自动转正。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/cqbench"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/evolution"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// specExternalRatio 「spec 外比例」一等指标（REQ-376③）。
type specExternalRatio struct {
	TriplesIn int     `json:"triples_in"`
	Remainder int     `json:"remainder"`
	Ratio     float64 `json:"ratio"`
	Note      string  `json:"note,omitempty"`
}

// diagnoseResult 进化诊断 job 结果。
type diagnoseResult struct {
	SpecExternalRatio *specExternalRatio   `json:"spec_external_ratio,omitempty"` // REQ-376③：spec 外比例一等指标
	Candidate         *evolution.Candidate `json:"candidate,omitempty"`           // 产出的候选补丁（proposed，待人门控）
	Report      *cqbench.Report      `json:"report,omitempty"`     // 诊断评测报告（三率+分层；不落测我历史）
	Attribution map[string]any       `json:"attribution,omitempty"` // 归因产出（layer/summary/evidence）
	Note        string               `json:"note,omitempty"`
}

// evolutionDiagnose POST /api/ontology/evolution/diagnose → 202 {job_id}。
// 诊断（cqbench 基准评测）→ 归因+补丁（LLM 单轮结构化产出）→ 候选落库（轮次预算内）。
func (s *Server) evolutionDiagnose(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "模型通道未配置（LLM Creator 未装配）"})
		return
	}
	var req struct {
		OntologyID string `json:"ontology_id"`
		Dataset    string `json:"dataset"` // 可选：缺省取清单第一份
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.OntologyID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 必填"})
		return
	}
	specMap, version, err := s.getSpecMap(req.OntologyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	// 轮次预算前置校验（LLM 花费前 409——REQ-207② 防无限迭代）
	rounds, _ := s.evoStore().CountRounds(req.OntologyID)
	if rounds >= evolution.MaxRoundsPerPass {
		writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("轮次预算已用尽（%d/%d）——留档后重开进化 pass", rounds, evolution.MaxRoundsPerPass)})
		return
	}
	// 数据集：显式指定或清单第一份
	var ds *cqbench.Dataset
	if strings.TrimSpace(req.Dataset) != "" {
		if ds, err = cqbench.LoadDataset(cqBenchDir() + "/" + req.Dataset + ".json"); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "数据集不存在: " + req.Dataset})
			return
		}
	} else {
		metas, lerr := cqbench.ListDatasets(cqBenchDir())
		if lerr != nil || len(metas) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "无基准数据集（seeds/evaldata/ontology-cq 为空）——诊断步无法开跑"})
			return
		}
		if ds, err = cqbench.LoadDataset(cqBenchDir() + "/" + metas[0].Name + ".json"); err != nil {
			writeErr(w, err)
			return
		}
	}
	job := &draftJob{ID: fmt.Sprintf("evo_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	s.persistDraftJob(job)
	go s.runEvolutionDiagnose(job, req.OntologyID, version, specMap, ds, rounds+1)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID, "dataset": ds.Name, "total": fmt.Sprint(len(ds.CQs))})
}

// runEvolutionDiagnose 诊断→归因→补丁→候选三步（job goroutine 内聚；失败原因透出 job.error）。
func (s *Server) runEvolutionDiagnose(job *draftJob, ontologyID string, version int, specMap map[string]any, ds *cqbench.Dataset, round int) {
	ctx := context.Background()
	fail := func(format string, args ...any) {
		draftJobsMu.Lock()
		job.Status, job.Error = "failed", fmt.Sprintf(format, args...)
		draftJobsMu.Unlock()
		s.persistDraftJob(job) // REQ-381②：失败写穿
	}
	// REQ-376③：「spec 外比例」透出（诊断报告注记——余量占比越高=门禁/进化/编辑作用面越窄）
	if b, _, gerr := s.Store.GetArtifact(ontologyID, "import-reconciliation"); gerr == nil {
		var rc struct {
			TriplesIn int `json:"triples_in"`
			Remainder int `json:"remainder"`
		}
		if json.Unmarshal([]byte(b), &rc) == nil && rc.TriplesIn > 0 {
			ratio := float64(rc.Remainder) / float64(rc.TriplesIn)
			draftJobsMu.Lock()
			if job.Diagnose == nil {
				job.Diagnose = &diagnoseResult{}
			}
			job.Diagnose.SpecExternalRatio = &specExternalRatio{
				TriplesIn: rc.TriplesIn, Remainder: rc.Remainder,
				Ratio:    ratio,
				Note:     fmt.Sprintf("spec 外比例 %.1f%%——余量越高，门禁/进化补丁/overlay 编辑的作用面越窄（REQ-376③ 诚实信号）", ratio*100),
			}
			draftJobsMu.Unlock()
		}
	}
	setProgress := func(msg string) {
		draftJobsMu.Lock()
		job.Progress = msg
		draftJobsMu.Unlock()
	}

	// ---- 第①步 诊断：基准评测当前本体（进化诊断步直接消费基准，E5 注兑现）----
	setProgress(fmt.Sprintf("诊断：基准集 %s 评测中（%d 条 CQ）", ds.Name, len(ds.CQs)))
	target := &pkgspec.Spec{}
	if b, err := json.Marshal(specMap); err != nil || json.Unmarshal(b, target) != nil {
		fail("spec 解析失败: %v", err)
		return
	}
	rep := cqbench.Run(ctx, s.LLM, target, ontologyID, version, ds, setProgress, cqbench.WithExecutor(s.benchExecutor()))
	failed := failingSignals(rep)
	if len(failed) == 0 {
		// 全绿：如实产出「无进化建议」结论，不出候选（门控最承重——无问题不硬造补丁）
		draftJobsMu.Lock()
		job.Status, job.Diagnose = "done", &diagnoseResult{Report: rep, Note: "基准全绿（三率满格）——本次诊断无进化建议，未产出候选（不为改而改）"}
		draftJobsMu.Unlock()
		s.persistDraftJob(job)
		return
	}

	// ---- 第②③步 归因+补丁：LLM 单轮结构化产出（三层定位每次只动一层+类型化编辑）----
	setProgress(fmt.Sprintf("归因：依据 %d 条失评测信号产出补丁（三层定位）", len(failed)))
	prompt := buildDiagnosePrompt(ontologyID, target, rep, failed)
	attribJSON, _, err := s.LLM.Chat(ctx, prompt, diagnoseSchemaHint)
	if err != nil {
		fail("归因产出失败: %v", err)
		return
	}
	var attrib struct {
		Layer    string          `json:"layer"`
		Summary  string          `json:"summary"`
		Evidence string          `json:"evidence"`
		Patch    json.RawMessage `json:"patch"`
	}
	if err := decodeLooseJSON(attribJSON, &attrib); err != nil {
		fail("归因输出解析失败（模型未按 JSON 形态返回）: %v", err)
		return
	}
	switch attrib.Layer {
	case "content", "tool", "schema":
	default:
		fail("归因 layer 非法 %q（须为 content|tool|schema）", attrib.Layer)
		return
	}
	// Evidence 锚定机械校验（防本体幻觉）：必须引用 ≥1 条失评测信号的 ID 或 CQ 原文子串
	if !evidenceAnchored(attrib.Evidence, failed) {
		fail("Evidence 未锚定到任何失评测信号（须引用基准 CQ id 或题面原文）——候选拒绝产出（REQ-207③ 防本体幻觉）")
		return
	}
	// 补丁可用性机械校验：类型化编辑至少落一处且应用不破坏结构
	applied, err := evolution.ApplyPatch(deepCopyMap(specMap), string(attrib.Patch))
	if err != nil || applied == 0 {
		fail("补丁应用失败（applied=%d err=%v）——候选拒绝产出", applied, err)
		return
	}
	c, err := s.evoStore().Create(&evolution.Candidate{
		OntologyID:  ontologyID,
		BaseVersion: version,
		Layer:       attrib.Layer,
		Summary:     attrib.Summary,
		Evidence:    attrib.Evidence,
		PatchJSON:   compactJSON(attrib.Patch),
		Round:       round,
	})
	if err != nil {
		fail("候选落库失败: %v", err)
		return
	}
	draftJobsMu.Lock()
	job.Status = "done"
	job.Diagnose = &diagnoseResult{
		Candidate: c, Report: rep,
		Attribution: map[string]any{"layer": attrib.Layer, "summary": attrib.Summary, "evidence": attrib.Evidence},
		Note:        fmt.Sprintf("候选 %s 已产出（proposed）——门控/采纳走既有 REST 人工链（一期口径：LLM 产物不自动转正）", c.Label),
	}
	draftJobsMu.Unlock()
	s.persistDraftJob(job) // REQ-381②：完成写穿
}

// failingSignals 提取失评测信号（翻译/白名单/gold 任一不过；上限 12 条防 prompt 膨胀）。
func failingSignals(rep *cqbench.Report) []cqbench.Signal {
	var out []cqbench.Signal
	for _, sig := range rep.Signals {
		if !sig.Translated || !sig.WhitelistOK || !sig.GoldAligned {
			out = append(out, sig)
		}
		if len(out) >= 12 {
			break
		}
	}
	return out
}

// evidenceAnchored Evidence 锚定机械校验：引用了 ≥1 条失评测信号的 ID，或含某条 CQ 题面的
// ≥10 rune 连续子串（LLM 换述容忍；纯空想 evidence 两者皆不命中即拒绝）。
func evidenceAnchored(evidence string, failed []cqbench.Signal) bool {
	ev := strings.TrimSpace(evidence)
	if ev == "" {
		return false
	}
	for _, sig := range failed {
		if sig.ID != "" && strings.Contains(ev, sig.ID) {
			return true
		}
		runes := []rune(sig.CQ)
		if len(runes) >= 10 {
			if strings.Contains(ev, string(runes[:10])) {
				return true
			}
		}
	}
	return false
}

// buildDiagnosePrompt 归因+补丁提示词（当前本体概览+失评测信号+补丁 Schema+三层纪律）。
func buildDiagnosePrompt(ontologyID string, target *pkgspec.Spec, rep *cqbench.Report, failed []cqbench.Signal) string {
	var b strings.Builder
	b.WriteString("你是本体进化专家。给定本体现状与基准评测的失评测信号，做一次「归因→补丁提议」。\n\n")
	b.WriteString("## 本体现状（" + ontologyID + " v" + fmt.Sprint(rep.Version) + "）\n")
	fmt.Fprintf(&b, "概念 %d 个（最多列 40）：%s\n", len(target.Concepts), joinNames(conceptNames(target), 40))
	fmt.Fprintf(&b, "关系 %d 条（最多列 40）：%s\n", len(target.Relations), joinNames(relationNames(target), 40))
	fmt.Fprintf(&b, "实例 %d 个。\n\n", len(target.Instances))
	fmt.Fprintf(&b, "## 诊断结果（基准集 %s：翻译 %d/%d · 白名单 %d/%d · gold 对齐 %d/%d）\n", rep.Dataset, rep.Translated, rep.Total, rep.Whitelist, rep.Total, rep.GoldAlign, rep.Total)
	b.WriteString("失评测信号（逐条：id | 题型/难度 | 能力问题 | 失败环节与原因）：\n")
	for _, sig := range failed {
		stage := "gold 对齐失败（查询未引用答案依赖的概念/关系）"
		if !sig.Translated {
			stage = "翻译失败：" + sig.Error
		} else if !sig.WhitelistOK {
			stage = "白名单拦截：" + sig.Error
		}
		fmt.Fprintf(&b, "- %s | %s/%s | %s | %s\n", sig.ID, sig.Kind, sig.Difficulty, sig.CQ, stage)
	}
	b.WriteString(`
## 归因三层纪律（每次只动一层）
- content：本体内容缺漏/错误（概念缺失、关系缺失、定义偏差）——评测信号指向「本体覆盖不住问题」时选此层；
- tool：查询/工具使用问题（SPARQL 模板不匹配、IRI 拼法）——信号指向「翻译得出但查询写错」时选此层；
- schema：结构问题（层级错置、关系端点错误）——信号指向「结构本身矛盾」时选此层。
注意：tool 层问题**不要**用 content 补丁硬修（加概念救不了查询模板错）。

## 补丁 Schema（类型化编辑；只允许以下键，应用顺序 remove→add/update）
{"add_concepts":[{"name":"...","label":"...","definition":"...","parents":["..."]}],"update_concepts":[{"name":"现有名","definition":"新定义"}],"remove_concepts":["现有名"],"add_relations":[{"name":"...","from":"概念名","to":"概念名"}],"remove_relations":[{"name":"...","from":"...","to":"..."}]}

## 输出要求（只输出一个 JSON 对象，不要 markdown 代码块）
{"layer":"content|tool|schema","summary":"一句话归因摘要","evidence":"Evidence 锚定——必须引用上表至少一条失评测信号的 id（如 Q03）或题面原文片段，说明该信号为何指向本补丁","patch":{...上列 Schema 的补丁对象...}}
约束：补丁必须最小（≤4 处编辑）；新概念的 parents/from/to 只能引用本体现有概念名；不得虚构未出现的评测信号。`)
	return b.String()
}

func conceptNames(sp *pkgspec.Spec) []string {
	out := make([]string, 0, len(sp.Concepts))
	for _, c := range sp.Concepts {
		out = append(out, c.Name)
	}
	return out
}

func relationNames(sp *pkgspec.Spec) []string {
	out := make([]string, 0, len(sp.Relations))
	for _, r := range sp.Relations {
		out = append(out, r.Name)
	}
	return out
}

func joinNames(names []string, cap int) string {
	if len(names) > cap {
		names = names[:cap]
	}
	return strings.Join(names, "、")
}

// diagnoseSchemaHint 归因产出 JSON Schema（llmcreate.Chat schema 参数）。
const diagnoseSchemaHint = `{"type":"object","required":["layer","summary","evidence","patch"],"properties":{
 "layer":{"type":"string","enum":["content","tool","schema"]},
 "summary":{"type":"string"},
 "evidence":{"type":"string"},
 "patch":{"type":"object","properties":{
   "add_concepts":{"type":"array"},"update_concepts":{"type":"array"},
   "remove_concepts":{"type":"array"},"add_relations":{"type":"array"},"remove_relations":{"type":"array"}}}}}`

// decodeLooseJSON 容忍模型输出裹 code fence/前后杂文的 JSON 提取解析。
func decodeLooseJSON(raw string, v any) error {
	t := strings.TrimSpace(raw)
	if i := strings.Index(t, "{"); i >= 0 {
		if j := strings.LastIndex(t, "}"); j > i {
			t = t[i : j+1]
		}
	}
	return json.Unmarshal([]byte(t), v)
}

// compactJSON 紧凑化 JSON（补丁落库形态）。
func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}
