// rest_cqrag.go CQ 驱动增量扩展端点（REQ-343/OE-1+OE-2，ontorag 三段式 RAG 的 REST 面）。
//
// POST /api/ontology/cqrag/draft {ontology_id, cqs?, profile?, top_k?} → 202 {job_id, total}
// 轮询复用 GET /api/ai-draft-jobs/{id}（job.rag 字段透出）。
//
// 新能力承载，不复用现有 ontoextend 路径 API 名（64 号 §3.1「同名不同物」既定决策）：
// ODP/LOV 现路径=「模式与词表驱动」，本路径=「CQ+元素级检索+LLM 生成的论文三段式」。
// 审查/入库不在本端点：片段 spec 经通用 merge/preview|apply（REQ-157 底座，strict 门禁+版本快照）。
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/buildflow"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontorag"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// cqragDraftResult job 终局载荷。
type cqragDraftResult struct {
	Fragments []*ontorag.GenerateResult `json:"fragments"`
	DraftSpec map[string]any            `json:"draft_spec,omitempty"` // 合并去重后的片段（map 形态与 draftResult.Spec 同构）
	Removed   []string                  `json:"removed,omitempty"`    // 去重剔除清单（Integrator）
	Retrieval cqragRetrievalInfo        `json:"retrieval"`
	Quality   *qualitygate.Report       `json:"quality,omitempty"`  // 合并片段质量报告（预览；strict 强制在 merge/apply）
	Precheck  *ontorag.PrecheckReport   `json:"precheck,omitempty"` // REQ-344/OE-5：CQ 质量预检信号（非闸门）
}

// cqragRetrievalInfo 检索元数据（对标论文 cq_processing_log/generation_runs 口径的 v1 最小集；
// 全量落 artifact cqrag-retrieval-log 供审计）。
type cqragRetrievalInfo struct {
	ElementsIndexed int    `json:"elements_indexed"`
	TopK            int    `json:"top_k"`
	Profile         string `json:"profile"`
	CQTotal         int    `json:"cq_total"`
	CQOK            int    `json:"cq_ok"`
	CQFailed        int    `json:"cq_failed"`
	ElementsUsed    int    `json:"elements_used"` // 全部片段实际注入 prompt 的命中条数（top-k 截断前）
	DurationMS      int64  `json:"duration_ms"`
}

// cqragDraft POST /api/ontology/cqrag/draft → 202 {job_id, total}。
func (s *Server) cqragDraft(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	var req struct {
		OntologyID   string   `json:"ontology_id"`
		CQs          []string `json:"cqs"`           // 缺省用 spec.CQ
		Profile      string   `json:"profile"`       // structure | restriction；缺省 structure
		TopK         int      `json:"top_k"`         // 缺省 20（论文默认）
		SkipPrecheck bool     `json:"skip_precheck"` // REQ-344/OE-5：跳过 CQ 质量预检（默认执行；信号非闸门）
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.OntologyID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 必填"})
		return
	}
	profile := req.Profile
	if profile == "" {
		profile = ontorag.ProfileStructure
	}
	if !ontorag.ValidProfile(profile) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "profile 只支持 structure | restriction"})
		return
	}
	oid := req.OntologyID
	raw, _, err := s.Store.GetArtifact(oid, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	target := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec_json 解析失败: " + err.Error()})
		return
	}
	cqs := req.CQs
	if len(cqs) == 0 {
		cqs = target.CQ
	}
	if len(cqs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "本体的 spec.CQ 为空且未显式传入 cqs——CQ 驱动扩展以能力问题为需求输入，请先在构建会话抽取确认 CQ"})
		return
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 20
	}
	job := &draftJob{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	s.persistDraftJob(job)
	go s.runCQRAGDraft(job, oid, target, ontorag.SpecFingerprint(raw), cqs, profile, topK, req.SkipPrecheck)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "total": len(cqs)})
}

// runCQRAGDraft 后台执行三段式：索引检索 → 逐 CQ 生成（两阶段校验）→ 去重合并 → 检索日志落 artifact。
// 段留痕经 buildflow（cq-rag-draft 实例）；审查/入库由前端走通用 merge 端点（REQ-157，版本快照）。
// specFP=目标 spec 内容指纹（向量化结果按其缓存，84 号 A2/86 号 N3——同 spec 重跑不再重调 embedding）。
func (s *Server) runCQRAGDraft(job *draftJob, oid string, target *pkgspec.Spec, specFP string, cqs []string, profile string, topK int, skipPrecheck bool) {
	started := time.Now()
	setProgress := func(msg string) {
		draftJobsMu.Lock()
		job.Progress = msg
		draftJobsMu.Unlock()
	}
	var (
		elements  []ontorag.Element
		vectors   [][]float64
		fragments []*ontorag.GenerateResult
		merged    *ontorag.FragmentSpec
		removed   []string
		precheck  *ontorag.PrecheckReport
	)
	perr := buildflow.Run(context.Background(), "cq-rag-draft",
		buildflow.Segment{Stage: buildflow.StageSource, Run: func(context.Context) (string, error) {
			return fmt.Sprintf("目标本体 %s · CQ×%d · profile=%s · top_k=%d", oid, len(cqs), profile, topK), nil
		}},
		// Retriever：元素级抽取 + 管道符序列化 + 向量索引（论文第一段；单机暴力内积等价 FAISS IP）
		buildflow.Segment{Stage: buildflow.StageExtract, Run: func(ctx context.Context) (string, error) {
			if els, vecs, ok := s.vecCache.Get(oid, specFP); ok {
				elements, vectors = els, vecs
				return fmt.Sprintf("元素 %d 个命中向量缓存（spec 未变更，跳过 embedding）", len(els)), nil
			}
			setProgress("目标本体元素向量化中")
			client := ontorag.NewClient(s.LLM.PlatformURL)
			els, vecs, err := client.Index(ctx, oid, target)
			if err != nil {
				return "", err
			}
			elements, vectors = els, vecs
			s.vecCache.Put(oid, specFP, els, vecs)
			return fmt.Sprintf("元素 %d 个向量化（概念/关系/数据属性）", len(els)), nil
		}},
		// REQ-344/OE-5：CQ 质量预检（可答性/原子性/具体性——N9 信号「框架不能补救坏需求」；
		// 信号非闸门：低分 CQ 警告透出仍继续，工程师决策）。拍 draft 段（Retriever 之后、生成之前）。
		buildflow.Segment{Stage: buildflow.StageDraft, Run: func(ctx context.Context) (string, error) {
			if skipPrecheck {
				return "预检已跳过（skip_precheck）", nil
			}
			setProgress("CQ 质量预检中（可答性/原子性/具体性）")
			if rep := ontorag.Precheck(ctx, s.LLM, target, cqs, setProgress); rep != nil {
				precheck = rep
				return fmt.Sprintf("CQ 预检：不可答 %d / 非原子 %d / 不具体 %d（共 %d 条；信号非闸门）",
					rep.NotAnswerable, rep.NotAtomic, rep.NotSpecific, len(rep.Items)), nil
			}
			return "CQ 预检降级（输出不合法）——无信号继续", nil
		}},
		// Extender：逐 CQ 检索 top-k + 生成片段 + 两阶段校验（失败标记不静默入库）
		buildflow.Segment{Stage: buildflow.StageDraft, Run: func(ctx context.Context) (string, error) {
			client := ontorag.NewClient(s.LLM.PlatformURL)
			fragments = make([]*ontorag.GenerateResult, 0, len(cqs))
			used := 0
			for i, cq := range cqs {
				setProgress(fmt.Sprintf("CQ %d/%d：检索相关元素", i+1, len(cqs)))
				hits, err := client.Retrieve(ctx, cq, elements, vectors, topK)
				if err != nil {
					fragments = append(fragments, &ontorag.GenerateResult{CQ: cq, Status: "failed", Error: "检索失败: " + err.Error()})
					continue
				}
				used += len(hits)
				setProgress(fmt.Sprintf("CQ %d/%d：生成片段（命中 %d 元素）", i+1, len(cqs), len(hits)))
				res := ontorag.Generate(ctx, s.LLM, target, cq, profile, hits,
					func(msg string) { setProgress(fmt.Sprintf("CQ %d/%d：%s", i+1, len(cqs), msg)) })
				fragments = append(fragments, res)
			}
			okN, failN := countFragments(fragments)
			if okN == 0 {
				return fmt.Sprintf("全部 %d 条 CQ 片段生成失败", failN), fmt.Errorf("全部 CQ 片段生成失败（两阶段校验未过或模型调用失败）——已如实标记，不静默入库")
			}
			return fmt.Sprintf("CQ %d 条：成功 %d / 失败 %d（命中元素 %d 条次）", len(cqs), okN, failN, used), nil
		}},
		// Integrator：去重合并 + 质量报告预检（strict 强制在 merge/apply；此处仅预览报告）
		buildflow.Segment{Stage: buildflow.StageGate, Run: func(_ context.Context) (string, error) {
			setProgress("去重合并片段")
			merged = &ontorag.FragmentSpec{Name: "CQ 驱动扩展片段"}
			for _, f := range fragments {
				if f.Status != "ok" || f.Fragment == nil {
					continue
				}
				merged.Concepts = append(merged.Concepts, f.Fragment.Concepts...)
				merged.Relations = append(merged.Relations, f.Fragment.Relations...)
				merged.DataProperties = append(merged.DataProperties, f.Fragment.DataProperties...)
				merged.Axioms = append(merged.Axioms, f.Fragment.Axioms...)
			}
			cleaned, rm := ontorag.Dedupe(target, merged)
			merged, removed = cleaned, rm
			rep := qualitygate.Check(cleaned.ToSpec(), nil)
			okN, failN := countFragments(fragments)
			note := fmt.Sprintf("合并片段：概念 %d · 关系 %d · 数据属性 %d · 公理 %d（去重剔除 %d）；质量 err=%d warn=%d",
				len(cleaned.Concepts), len(cleaned.Relations), len(cleaned.DataProperties), len(cleaned.Axioms), len(rm),
				rep.ErrorCount, rep.WarningCount)
			draftJobsMu.Lock()
			job.RAG = &cqragDraftResult{Fragments: fragments, Removed: removed, Precheck: precheck, Retrieval: cqragRetrievalInfo{
				ElementsIndexed: len(elements), TopK: topK, Profile: profile,
				CQTotal: len(cqs), CQOK: okN, CQFailed: failN,
				ElementsUsed: countUsed(fragments), DurationMS: time.Since(started).Milliseconds(),
			}, Quality: rep}
			draftJobsMu.Unlock()
			return note, nil
		}},
	)
	draftJobsMu.Lock()
	defer func() {
		draftJobsMu.Unlock()
		s.persistDraftJob(job) // REQ-381②：终态写穿
	}()
	if perr != nil {
		job.Status = "failed"
		job.Error = perr.Error()
		return
	}
	job.Status = "done"
	if b, merr := json.Marshal(merged.ToSpec()); merr == nil {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			job.RAG.DraftSpec = m
		}
	}
	job.Progress = ""
	// 检索日志落 artifact（最近一次覆盖式；对标论文四 CSV 的审计口径 v1 最小集；
	// 落库失败不阻断主链路——审计增强非依赖）
	if b, merr := json.Marshal(job.RAG); merr == nil {
		_ = s.Store.PutArtifact(oid, "cqrag-retrieval-log", string(b), false)
	}
}

// countFragments 统计片段成功/失败数。
func countFragments(frags []*ontorag.GenerateResult) (ok, failed int) {
	for _, f := range frags {
		if f.Status == "ok" {
			ok++
		} else {
			failed++
		}
	}
	return
}

// countUsed 全部片段注入 prompt 的命中条数合计（检索日志 elements_used）。
func countUsed(frags []*ontorag.GenerateResult) int {
	n := 0
	for _, f := range frags {
		n += len(f.Used)
	}
	return n
}
