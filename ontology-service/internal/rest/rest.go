// Package rest 构建平面 REST（方案 04 §3.6）。
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/evolution"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/importer"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontochat"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontorag"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/pipeline"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/vocabsearch"
)

type Server struct {
	Store    *repo.Store
	Sidecar  *importer.Sidecar
	LLM      *llmcreate.Creator
	OntoChat *ontochat.Engine
	// RuntimeURL REQ-382/M178：运行平面基址（RUNTIME_MGR_URL；空=cqbench 执行级对齐回落文本子串口径）
	RuntimeURL string

	ontoChatDB *ontochat.Store     // 惰性初始化（rest_ontochat.go）
	pipelineDB *pipeline.Store     // 惰性初始化（rest_pipeline.go）
	evoDB      *evolution.Store    // REQ-207/M43：进化候选（惰性初始化）
	vocab      *vocabsearch.Client // REQ-171 P1：LOV 词表搜索（惰性初始化；LOV_API_BASE 可注入测试桩）

	vecCache *ontorag.VectorCache // CQ 扩展元素向量缓存（84 号 A2/86 号 N3：按 spec 指纹命中，进程内）

	jobCancelsMu sync.Mutex
	// jobCancels OntoChat 生成 job 的运行期取消句柄（jobID→cancel；终态后摘除）。
	// job 状态本体在 SQLite（ontochat_job，重启可恢复）；此 map 仅运行期句柄，缺失时取消走收敛兜底。
	jobCancels map[string]context.CancelFunc
}

func New(st *repo.Store, sc *importer.Sidecar, llm *llmcreate.Creator, runtimeURL ...string) *Server {
	s := &Server{Store: st, Sidecar: sc, LLM: llm, OntoChat: &ontochat.Engine{LLM: llm},
		vecCache: ontorag.NewVectorCache(0), jobCancels: map[string]context.CancelFunc{}}
	// REQ-382/M178：运行平面基址（cqbench 执行级对齐通道；variadic 保持既有调用零破坏）
	if len(runtimeURL) > 0 {
		s.RuntimeURL = runtimeURL[0]
	}
	return s
}

// Mount 注册到主平台兼容的 1.22 pattern mux。
func (s *Server) Mount(m *http.ServeMux) {
	m.HandleFunc("GET /api/ontologies", s.list)
	m.HandleFunc("POST /api/ontologies", s.create)
	m.HandleFunc("GET /api/ontologies/{id}", s.get)
	m.HandleFunc("PUT /api/ontologies/{id}", s.update)
	m.HandleFunc("DELETE /api/ontologies/{id}", s.remove)
	m.HandleFunc("GET /api/ontologies/{id}/spec", s.getSpec)
	m.HandleFunc("PUT /api/ontologies/{id}/spec", s.saveSpec)
	s.registerElementRoutes(m) // REQ-436/M232：元素级端点（编排服务端单源）
	m.HandleFunc("POST /api/ontologies/{id}/validate", s.validate)
	m.HandleFunc("GET /api/ontologies/{id}/artifacts", s.artifacts)
	// REQ-240⑥/M66：通用形态读写（图布局持久化 layout_json 等；不写回 spec——O-3 口径修正，存 artifact 跨端一致）
	m.HandleFunc("PUT /api/ontologies/{id}/artifacts/{format}", s.putArtifactContent)
	m.HandleFunc("GET /api/ontologies/{id}/artifacts/{format}/content", s.getArtifactContent)
	m.HandleFunc("POST /api/ontologies/import", s.importOntology)
	m.HandleFunc("POST /api/ontologies/{id}/merge/preview", s.mergePreview)
	m.HandleFunc("POST /api/ontologies/{id}/merge/apply", s.mergeApply)
	m.HandleFunc("GET /api/ontologies/{id}/quality-config", s.qualityConfigGet)
	m.HandleFunc("PUT /api/ontologies/{id}/quality-config", s.qualityConfigPut)
	m.HandleFunc("GET /api/ontologies/{id}/export", s.exportOntology)
	m.HandleFunc("GET /api/ontologies/{id}/guide", s.guide)
	m.HandleFunc("POST /api/ontologies/seed-sample", s.seedSample)
	m.HandleFunc("POST /api/ontologies/ai-draft", s.aiDraft)

	// 质量门禁与词表搜索（REQ-171 P1，26 号方案 §9 P1 底座 A + LOV 薄层；路径沿用方案原文 /api/ontology/ 前缀）
	m.HandleFunc("POST /api/ontology/quality/check", s.qualityCheck)
	// REQ-207/M43（52 号 E5④）：生成异步化（ai-draft 202+轮询）
	m.HandleFunc("POST /api/ontologies/ai-draft-async", s.aiDraftAsync)
	m.HandleFunc("GET /api/ai-draft-jobs/{id}", s.aiDraftJob)
	// REQ-306 A1/M110：AI 创建澄清轮（生成前缺口清单+逐轮补充，可跳过直接生成）
	m.HandleFunc("POST /api/ontologies/ai-clarify", s.aiClarify)
	// REQ-308/M112：AI 创建 CQ 候选生成（描述+澄清轮材料→ExtractCQs→前端确认卡人工勾选）
	m.HandleFunc("POST /api/ontologies/ai-cq-extract", s.aiCQExtract)
	// REQ-340 A2：构建管线段留痕聚合口（kb-extract 等远端路径回传；REQ-328 对账单点出口）
	m.HandleFunc("POST /api/buildflow/traces", s.buildflowTraces)
	// REQ-207/M43 本体自进化（候选 vN-cK 状态机：诊断→补丁→配对门控→人工采纳）
	m.HandleFunc("GET /api/ontologies/{id}/evolution/candidates", s.listEvolutionCandidates)
	m.HandleFunc("POST /api/ontologies/{id}/evolution/candidates", s.proposeEvolutionCandidate)
	m.HandleFunc("POST /api/ontologies/{id}/evolution/candidates/{cid}/gate", s.gateEvolutionCandidate)
	m.HandleFunc("POST /api/ontologies/{id}/evolution/candidates/{cid}/accept", s.acceptEvolutionCandidate)
	m.HandleFunc("POST /api/ontologies/{id}/evolution/candidates/{cid}/reject", s.rejectEvolutionCandidate)
	// REQ-207 余项：进化诊断→归因→补丁 LLM 自动产出（人工触发一键化；候选仍走 gate/accept 人工链）
	m.HandleFunc("POST /api/ontology/evolution/diagnose", s.evolutionDiagnose)
	m.HandleFunc("GET /api/ontology/quality/report", s.qualityReport)
	m.HandleFunc("GET /api/ontology/quality/remediations", s.qualityRemediations) // REQ-331②：检查项修复指引
	m.HandleFunc("GET /api/ontologies/{id}/quality-history", s.qualityHistory)    // REQ-331①：评分历史（趋势线+三点对照）
	// REQ-255/M62 批次（60 号 H2）：CQ→SPARQL 翻译（LLM 辅助+人工确认模板；执行走运行方案 SPARQL 端点）
	m.HandleFunc("POST /api/ontologies/{id}/cq-sparql", s.cqSparql)
	m.HandleFunc("PUT /api/ontologies/{id}/cq-test-report", s.saveCQTestReport) // REQ-342/OC-2：双轨报告持久化
	m.HandleFunc("POST /api/ontology/toolchain/{tool}", s.toolchain)
	m.HandleFunc("GET /api/ontology/vocabularies/search", s.vocabSearch)
	m.HandleFunc("GET /api/ontology/ontoextend/odps", s.ontoextendListODPs)
	m.HandleFunc("POST /api/ontology/ontoextend/draft", s.ontoextendDraft)
	m.HandleFunc("POST /api/ontology/cqrag/draft", s.cqragDraft)
	m.HandleFunc("POST /api/ontology/nl2sparql", s.nl2sparql)                         // REQ-333/M133：NL2SPARQL 受控生成
	m.HandleFunc("GET /api/ontology/cq-benchmark/datasets", s.cqBenchmarkDatasets)    // REQ-346/M146：基准集管理面
	m.HandleFunc("POST /api/ontology/cq-benchmark/run", s.cqBenchmarkRun)             // REQ-346/M146：用基准集测我（202 job）
	m.HandleFunc("GET /api/ontologies/{id}/cq-benchmark-report", s.cqBenchmarkReport) // REQ-346/M146：报告历史 // REQ-343：CQ 驱动增量扩展（论文三段式 RAG，新能力承载不复用 ontoextend API 名）

	// 方案生命周期（REQ-155 阶段二/M-O15：Terraform 式 plan/apply，monitor=plan 只读形态）
	m.HandleFunc("GET /api/ontology/lifecycle/plan", s.lifecyclePlan)
	m.HandleFunc("POST /api/ontology/lifecycle/apply", s.lifecycleApply)
	m.HandleFunc("GET /api/ontologies/seed-learning", s.listLearning)
	m.HandleFunc("POST /api/ontologies/seed-learning", s.seedLearning)
	m.HandleFunc("GET /api/ontologies/{id}/versions", s.listVersions)
	m.HandleFunc("GET /api/ontologies/{id}/versions/{version}/original", s.versionOriginal)
	m.HandleFunc("GET /api/ontologies/{id}/versions/{version}/spec", s.versionSpec)
	m.HandleFunc("POST /api/ontologies/{id}/versions/{version}/restore", s.restoreVersion)
	// REQ-239/M65 版本发布状态机（发布/撤回；回滚经 restore——重发布动作走历史快照恢复）
	m.HandleFunc("POST /api/ontologies/{id}/publish", s.publishOntology)
	m.HandleFunc("POST /api/ontologies/{id}/unpublish", s.unpublishOntology)
	m.HandleFunc("GET /api/ontologies/{id}/diff", s.diffVersions)
	m.HandleFunc("GET /api/ontologies/{id}/impact", s.impactVersions) // REQ-278/M104：变更影响分析（闭包波及面+风险分级+B1 蕴含边）
	m.HandleFunc("POST /api/ontologies/{id}/ingest-csv", s.ingestCSV)
	m.HandleFunc("GET /api/ontologies/{id}/ingest-mapping", s.getIngestMapping)
	m.HandleFunc("PUT /api/ontologies/{id}/ingest-mapping", s.putIngestMapping)
	// REQ-292/M96 对象级 overlay（编辑三层 base+overlay+audit）：清单 / 对象编辑（revision 乐观锁+幂等）/ 撤回到 base
	m.HandleFunc("GET /api/ontologies/{id}/overlays", s.listOverlays)
	m.HandleFunc("PUT /api/ontologies/{id}/objects/{kind}/{name}/overlay", s.putObjectOverlay)
	m.HandleFunc("DELETE /api/ontologies/{id}/objects/{kind}/{name}/overlay", s.revertOverlay)

	// 工具链配置（REQ-75/76，04 §4.6）
	m.HandleFunc("GET /api/pipelines/catalog", s.listToolCatalog)
	m.HandleFunc("GET /api/pipelines", s.listPipelines)
	m.HandleFunc("POST /api/pipelines", s.createPipeline)
	m.HandleFunc("GET /api/pipelines/{id}", s.getPipeline)
	m.HandleFunc("PUT /api/pipelines/{id}", s.updatePipeline)
	m.HandleFunc("POST /api/pipelines/{id}/clone", s.clonePipeline)
	m.HandleFunc("DELETE /api/pipelines/{id}", s.deletePipeline)
	m.HandleFunc("POST /api/pipelines/{id}/check", s.checkPipeline)
	m.HandleFunc("POST /api/ontologies/{id}/fork", s.fork)
	// OntoChat 多轮引导（REQ-103 模式 A；REQ-271/M80 生成轮异步化+提示词只读透出）
	m.HandleFunc("GET /api/ontochat/sessions", s.listOntoChatSessions)
	m.HandleFunc("POST /api/ontochat/sessions", s.createOntoChatSession)
	m.HandleFunc("GET /api/ontochat/sessions/{id}", s.getOntoChatSession)
	m.HandleFunc("DELETE /api/ontochat/sessions/{id}", s.deleteOntoChatSession)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/turn", s.ontoChatTurn)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/save", s.ontoChatSave)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/cq-extract", s.ontoChatCQExtract)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/cqs", s.ontoChatSetCQs)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/cq-analyze", s.ontoChatCQAnalyze)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/story-back", s.ontoChatStoryBack)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/story-finish", s.ontoChatStoryFinish)
	m.HandleFunc("GET /api/ontochat/story-templates", s.ontoChatStoryTemplates)
	m.HandleFunc("POST /api/ontologies/{id}/cq-coverage", s.ontoCoverageTest)
	m.HandleFunc("POST /api/ontochat/sessions/{id}/test", s.ontoChatSessionTest) // REQ-342/OC-2：会话草稿覆盖测试（test 站）
	m.HandleFunc("GET /api/ontochat/jobs/{id}", s.getOntoChatJob)
	m.HandleFunc("GET /api/ontochat/sessions/{id}/job", s.getOntoChatSessionJob)
	m.HandleFunc("POST /api/ontochat/jobs/{id}/cancel", s.cancelOntoChatJob)
	m.HandleFunc("GET /api/ontochat/prompts", s.listOntoChatPrompts)
	// REQ-338③：草稿版本链（中断续作/跨设备/审计回放取数口）
	m.HandleFunc("GET /api/ontochat/sessions/{id}/drafts", s.ontoChatDrafts)
	// REQ-354/P2：会话草稿 Turtle 直出（ontochat_export_ttl 工具数据源，X-Onto-Source: ontochat_draft）
	m.HandleFunc("GET /api/ontochat/sessions/{id}/export", s.ontoChatSessionExport)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, repo.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(v)
}

// ---- 本体元数据 ----

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListOntologies()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Description string }
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name 必填"})
		return
	}
	id := "onto_" + newID()
	o, err := s.Store.CreateOntology(id, req.Name, req.Description)
	if err != nil {
		writeErr(w, err)
		return
	}
	// 新建即给一个空 spec 形态，编辑器可直接编辑；REQ-246/G1：三数组骨架显式落盘
	// （此前 nil 切片不序列化，首编者面对无键 JSON 与前端示意形态不符）
	empty := pkgspec.Spec{Name: req.Name, Description: req.Description, Concepts: []pkgspec.Concept{}, Relations: []pkgspec.Relation{}, Instances: []pkgspec.Instance{}}
	bts, _ := json.Marshal(empty)
	_ = s.Store.PutArtifact(id, "spec_json", string(bts), true)
	_ = s.Store.SaveVersion(id, 1, string(bts), "", "")
	writeJSON(w, http.StatusCreated, o)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	o, err := s.Store.GetOntology(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct{ Name, Description string }
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.UpdateOntology(id, req.Name, req.Description); err != nil {
		writeErr(w, err)
		return
	}
	o, _ := s.Store.GetOntology(id)
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteOntology(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("id")})
}

// ---- spec_json 编辑与校验 ----

func (s *Server) getSpec(w http.ResponseWriter, r *http.Request) {
	raw, _, err := s.Store.GetArtifact(r.PathValue("id"), "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(raw))
}

func (s *Server) saveSpec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// REQ-293②（67 号 B7）：If-Match 文档级防陈旧——不符 409 不静默覆盖（缺省头兼容旧客户端）
	if !s.guardIfMatch(w, r, id) {
		return
	}
	var sp pkgspec.Spec
	if err := decodeJSON(r, &sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON 解析失败: " + err.Error()})
		return
	}
	v, overlaysRecorded, perr := s.persistSpec(id, &sp)
	if perr != nil {
		switch perr.Kind {
		case "validation":
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": perr.Message, "validation_errors": perr.ValidationErrors})
		case "strict":
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": perr.Message, "quality_report": perr.Report})
		default:
			writeErr(w, perr.Err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "version": v, "overlays_recorded": overlaysRecorded})
}

// persistError 保存核结构化错误（kind=validation|strict|internal）。
type persistError struct {
	Kind             string
	Message          string
	ValidationErrors []pkgspec.ValidationError
	Report           any
	Err              error
}

// persistSpec REQ-436①：Validate+strict 门禁+落盘+overlay 提取+版本推进+入库评分的**单一保存核**
//（saveSpec 整包保存与元素级端点共用——版本门禁/Validate/伴生审计服务端单源，语义不再两处维护）。
// 返回（新版本号, overlay 记录数, 结构化错误）。
func (s *Server) persistSpec(id string, sp *pkgspec.Spec) (int, int, *persistError) {
	if errs := sp.Validate(); len(errs) > 0 {
		return 0, 0, &persistError{Kind: "validation", Message: "校验未通过，不允许保存坏本体", ValidationErrors: errs}
	}
	// REQ-156/M-O15：strict 门禁（本体级开关，默认宽松）——结构合法后过质量门禁，错误级命中阻断保存
	if o, gerr := s.Store.GetOntology(id); gerr == nil && o.QualityStrict {
		if rep := qualitygate.Check(sp, nil); rep.ErrorCount > 0 {
			return 0, 0, &persistError{Kind: "strict", Message: fmt.Sprintf("strict 门禁拦截：%d 处错误级质量命中（可在资产详情质量卡查看明细，或关闭 strict 开关）", rep.ErrorCount), Report: rep}
		}
	}
	// REQ-292① 编辑三层：按对象 diff 提取 overlay（机器重算后重放，根治覆盖人工编辑）
	var oldSp *pkgspec.Spec
	if prev, gerr := s.loadSpecOrDefault(id); gerr == nil {
		oldSp = prev
	}
	bts, err := json.Marshal(sp)
	if err != nil {
		return 0, 0, &persistError{Kind: "internal", Err: err}
	}
	if err := s.Store.PutArtifact(id, "spec_json", string(bts), true); err != nil {
		return 0, 0, &persistError{Kind: "internal", Err: err}
	}
	overlaysRecorded := 0
	if oldSp != nil {
		apply, removeKeys := s.extractOverlaysFromSave(id, oldSp, sp)
		s.persistOverlayDeltas(id, apply, removeKeys)
		overlaysRecorded = len(apply)
	}
	v, _ := s.Store.BumpVersion(id)
	_ = s.Store.SaveVersion(id, v, string(bts), "", "")
	// REQ-331/M131：入库评分随版本追加一行（source=ingest；本地计算零成本，失败不阻断保存）
	s.recordIngestScore(id, v, sp)
	return v, overlaysRecorded, nil
}

// recordIngestScore 入库评分落历史（REQ-331：saveSpec/mergeApply/ontoChatSave 三动线共用；
// 评分历史是增强不是依赖——错误静默不阻断入库）。
func (s *Server) recordIngestScore(id string, version int, sp *pkgspec.Spec) {
	rep := qualitygate.Check(sp, nil)
	_ = s.Store.SaveQualityScore(repo.QualityScoreRow{
		OntologyID: id, Version: version, Source: "ingest",
		Overall: rep.Score.Overall, Completeness: rep.Score.Completeness,
		Consistency: rep.Score.Consistency, Maintainability: rep.Score.Maintainability,
		ErrorCount: rep.ErrorCount, WarningCount: rep.WarningCount,
	})
}

func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	raw, _, err := s.Store.GetArtifact(id, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	var sp pkgspec.Spec
	if err := json.Unmarshal([]byte(raw), &sp); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "validation_errors": []map[string]string{{"path": "$", "message": err.Error()}}})
		return
	}
	errs := sp.Validate()
	if errs == nil {
		errs = []pkgspec.ValidationError{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": len(errs) == 0, "validation_errors": errs})
}

// newID 生成实体 id（时间戳形态，本体/job/方案共用）。
func newID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("060102150405.000000000"), ".", "")
}
