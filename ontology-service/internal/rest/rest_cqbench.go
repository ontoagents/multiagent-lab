// rest_cqbench.go CQ 基准集管理面与「测我」动线（REQ-346/M146 v1）。
//
// GET  /api/ontology/cq-benchmark/datasets               → 数据集清单
// POST /api/ontology/cq-benchmark/run {ontology_id, dataset} → 202 job（LLM 逐条翻译，复用 draftJob 轮询）
// GET  /api/ontologies/{id}/cq-benchmark-report          → 报告历史（artifact runs 追加式）
//
// 报告落 artifact cq-benchmark-report：{runs:[...]} 追加式历史（版本可比化的 v1 形态——
// 每条 run 带 ontology version/ts/三率，前端表格呈现即历史对比；独立对比视图留后续）。
// Judge 复用预留：rubric 化答案内容评分（非 gold IRI 口径）待与 agenteval judge.go 共契（见 cqbench 包头注）。
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/buildflow"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/cqbench"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// cqBenchDir 数据集目录（CQ_BENCH_DIR 可覆写；默认仓库形态相对路径）。
func cqBenchDir() string {
	if v := os.Getenv("CQ_BENCH_DIR"); v != "" {
		return v
	}
	return "seeds/evaldata/ontology-cq"
}

// cqBenchmarkDatasets GET /api/ontology/cq-benchmark/datasets → 数据集清单。
func (s *Server) cqBenchmarkDatasets(w http.ResponseWriter, _ *http.Request) {
	list, err := cqbench.ListDatasets(cqBenchDir())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"datasets": list, "count": len(list)})
}

// cqBenchmarkRun POST /api/ontology/cq-benchmark/run → 202 {job_id, total}。
func (s *Server) cqBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "模型通道未配置（LLM Creator 未装配）"})
		return
	}
	var req struct {
		OntologyID string `json:"ontology_id"`
		Dataset    string `json:"dataset"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.OntologyID) == "" || strings.TrimSpace(req.Dataset) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 与 dataset 必填"})
		return
	}
	// 数据集名防目录穿越（文件名白名单：仅字母数字-_）
	for _, ch := range req.Dataset {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dataset 名仅允许字母数字-_"})
			return
		}
	}
	ds, err := cqbench.LoadDataset(cqBenchDir() + "/" + req.Dataset + ".json")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "数据集不存在: " + req.Dataset})
		return
	}
	raw, _, gerr := s.Store.GetArtifact(req.OntologyID, "spec_json")
	if gerr != nil {
		writeErr(w, gerr)
		return
	}
	target := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec_json 解析失败: " + err.Error()})
		return
	}
	o, _ := s.Store.GetOntology(req.OntologyID)
	version := 0
	if o != nil {
		version = o.Version
	}
	job := &draftJob{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	s.persistDraftJob(job)
	go s.runCQBenchmark(job, req.OntologyID, version, target, ds)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "total": len(ds.CQs)})
}

// runCQBenchmark 后台执行「测我」：段留痕（cq-benchmark 实例）→ cqbench.Run → 报告追加 artifact。
func (s *Server) runCQBenchmark(job *draftJob, oid string, version int, target *pkgspec.Spec, ds *cqbench.Dataset) {
	started := time.Now()
	setProgress := func(msg string) {
		draftJobsMu.Lock()
		job.Progress = msg
		draftJobsMu.Unlock()
	}
	var rep *cqbench.Report
	perr := buildflow.Run(context.Background(), "cq-benchmark",
		buildflow.Segment{Stage: buildflow.StageSource, Run: func(context.Context) (string, error) {
			return fmt.Sprintf("基准集 %s · CQ×%d · 目标本体 %s v%d", ds.Name, len(ds.CQs), oid, version), nil
		}},
		buildflow.Segment{Stage: buildflow.StageExtract, Skip: true}, // 无抽取段（CQ 来自数据集非材料抽取）
		buildflow.Segment{Stage: buildflow.StageDraft, Run: func(ctx context.Context) (string, error) {
			rep = cqbench.Run(ctx, s.LLM, target, oid, version, ds, setProgress, cqbench.WithExecutor(s.benchExecutor()))
			return fmt.Sprintf("翻译 %d/%d · 白名单 %d/%d · gold 对齐 %d/%d",
				rep.Translated, rep.Total, rep.Whitelist, rep.Total, rep.GoldAlign, rep.Total), nil
		}},
	)
	// buildflow.Run 会因 registry 未声明「cq-benchmark」跳过未登记段——登记见 buildflow.go；
	// 若未登记则上面段全 skip，rep 为 nil——兜底直接跑（注册与执行解耦的保守路径）
	if rep == nil {
		rep = cqbench.Run(context.Background(), s.LLM, target, oid, version, ds, setProgress, cqbench.WithExecutor(s.benchExecutor()))
	}
	draftJobsMu.Lock()
	job.Result = &draftResult{}
	draftJobsMu.Unlock()
	// 报告追加 artifact {runs:[...]}（历史=多次运行列表，版本可比化 v1 形态）
	var envelope struct {
		Runs []json.RawMessage `json:"runs"`
	}
	if oldRaw, _, aerr := s.Store.GetArtifact(oid, "cq-benchmark-report"); aerr == nil && oldRaw != "" {
		_ = json.Unmarshal([]byte(oldRaw), &envelope)
	}
	repBytes, _ := json.Marshal(rep)
	envelope.Runs = append(envelope.Runs, repBytes)
	if b, merr := json.Marshal(envelope); merr == nil {
		_ = s.Store.PutArtifact(oid, "cq-benchmark-report", string(b), false)
	}
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
	job.Progress = fmt.Sprintf("翻译 %d/%d · 白名单 %d/%d · gold 对齐 %d/%d（耗时 %dms）",
		rep.Translated, rep.Total, rep.Whitelist, rep.Total, rep.GoldAlign, rep.Total, time.Since(started).Milliseconds())
}

// cqBenchmarkReport GET /api/ontologies/{id}/cq-benchmark-report → 报告历史。
func (s *Server) cqBenchmarkReport(w http.ResponseWriter, r *http.Request) {
	raw, ts, err := s.Store.GetArtifact(r.PathValue("id"), "cq-benchmark-report")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": r.PathValue("id"), "imported_at": ts, "report": json.RawMessage(raw)})
}
