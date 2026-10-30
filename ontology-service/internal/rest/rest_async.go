package rest

// REQ-207/M43（52 号 E5④）：生成异步化——ai-draft 202+轮询（沿 REQ-146 先例）。
// 治 A-6「生成链路同步阻塞 120s」：POST ai-draft-async 立即返回 job_id（202），
// 后台 goroutine 执行 Draft；GET ai-draft-jobs/{id} 轮询结果。旧同步端点保留兼容。
// 进程内 map（单机教学尺度；重启丢任务=前端重新发起，可接受诚实边界）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/buildflow"

	"github.com/ontoagents/ontoagents/ontology-service/internal/ontochat"
	"github.com/ontoagents/ontoagents/ontology-service/internal/qualitygate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

type draftJob struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`             // running | done | failed
	Progress  string                 `json:"progress,omitempty"` // REQ-295③（A4）：服务端实测计数进度（轮次/问题数，永不虚构百分比与 ETA）
	Result    *draftResult           `json:"result,omitempty"`
	CQs       []ontochat.ExtractedCQ `json:"cqs,omitempty"`    // REQ-308：CQ 抽取 job 结果（ai-cq-extract）
	Dedup     int                    `json:"dedup,omitempty"`  // REQ-308：与既有 CQ 重复被滤除条数
	Purify    *ontochat.PurifyStats  `json:"purify,omitempty"` // REQ-342/OC-1：净化统计（拆分/抽象率）
	RAG       *cqragDraftResult      `json:"rag,omitempty"`    // REQ-343：CQ 驱动扩展 job 结果（cqrag/draft）
	Diagnose  *diagnoseResult        `json:"diagnose,omitempty"` // REQ-207 余项：进化诊断→归因→补丁候选 job 结果
	Error     string                 `json:"error,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type draftResult struct {
	Spec    map[string]any      `json:"spec"`
	Rounds  int                 `json:"rounds"`
	Quality *qualitygate.Report `json:"quality,omitempty"` // REQ-247/G4：草案质量报告透出
}

var (
	draftJobsMu sync.Mutex
	draftJobs   = map[string]*draftJob{}
)

// persistDraftJob REQ-381②/M177：job 写穿 async_job 表（进程内 map 仍为热路径缓存；
// 重启后轮询回读 DB——done/failed 结果不丢）。落库失败仅日志（旁路不阻断任务）。
func (s *Server) persistDraftJob(j *draftJob) {
	if s.Store == nil || j == nil {
		return
	}
	b, err := json.Marshal(j)
	if err != nil {
		return
	}
	_ = s.Store.UpsertAsyncJob(&repo.AsyncJobRow{ID: j.ID, Kind: "draft", Status: j.Status, Progress: j.Progress, ResultJSON: string(b), Error: j.Error, CreatedAt: j.CreatedAt.Format(time.RFC3339Nano)})
}

// aiDraftAsync POST /api/ontologies/ai-draft-async → 202 {job_id}。
func (s *Server) aiDraftAsync(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	var req struct {
		Description         string   `json:"description"`
		ExtraHint           string   `json:"extra_hint"`
		CapabilityQuestions []string `json:"capability_questions"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Description) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description 必填"})
		return
	}
	job := &draftJob{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	s.persistDraftJob(job)
	go func() {
		// REQ-340 A2：管线段留痕（source→extract/draft→gate；审查=前端确认卡、入库=通用 create+saveSpec，不在本端点）
		desc := req.Description
		perr := buildflow.Run(context.Background(), "ai-draft",
			buildflow.Segment{Stage: buildflow.StageSource, Run: func(context.Context) (string, error) {
				note := desc
				if len(note) > 60 {
					note = note[:60] + "…"
				}
				if len(req.CapabilityQuestions) > 0 {
					note += fmt.Sprintf(" · CQ×%d", len(req.CapabilityQuestions))
				}
				return note, nil
			}},
			buildflow.Segment{Stage: buildflow.StageExtract, Skip: true}, // 抽取与草稿在 llmcreate 生成-校验循环内聚
			buildflow.Segment{Stage: buildflow.StageDraft, Run: func(ctx context.Context) (string, error) {
				res, err := s.LLM.DraftWithCQ(ctx, desc, req.ExtraHint, req.CapabilityQuestions,
					func(round int, msg string) { // REQ-295③：实测轮次进度透出（「第 N/M 轮…」，永不虚构百分比）
						draftJobsMu.Lock()
						job.Progress = msg
						draftJobsMu.Unlock()
						s.persistDraftJob(job)
					}) // REQ-248/242：CQ 回写+质量透出
				if err != nil {
					return "", err
				}
				// spec 结构体 → map（与同步端点 JSON 形态一致）
				b, _ := json.Marshal(res.Spec)
				var specMap map[string]any
				_ = json.Unmarshal(b, &specMap)
				draftJobsMu.Lock()
				job.Result = &draftResult{Spec: specMap, Rounds: res.Rounds, Quality: res.Quality}
				draftJobsMu.Unlock()
				return fmt.Sprintf("rounds=%d", res.Rounds), nil
			}},
			buildflow.Segment{Stage: buildflow.StageGate, Run: func(ctx context.Context) (string, error) {
				draftJobsMu.Lock()
				q := job.Result
				draftJobsMu.Unlock()
				if q == nil || q.Quality == nil {
					return "质量报告不可用（跳过）", nil
				}
				return fmt.Sprintf("overall=%.1f err=%d warn=%d", q.Quality.Score.Overall, q.Quality.ErrorCount, q.Quality.WarningCount), nil
			}},
		)
		draftJobsMu.Lock()
		if job.Result == nil {
			job.Status = "failed"
			if perr != nil {
				job.Error = perr.Error() // 既有语义保留：失败原因透出到 job（前端 toast）
			} else {
				job.Error = "生成失败"
			}
		} else {
			job.Status = "done"
		}
		draftJobsMu.Unlock()
		s.persistDraftJob(job)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// aiCQExtract POST /api/ontologies/ai-cq-extract → 202 {job_id}（REQ-308）：
// AI 创建路径 CQ 候选生成——描述+澄清轮累积 hints 为原料，复用 ExtractCQs（P5 两净化+回喂重试）；
// 轮询复用 GET /api/ai-draft-jobs/{id}（job.cqs 字段透出）。候选经前端确认卡人工勾选后才并入生成。
func (s *Server) aiCQExtract(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string   `json:"description"`
		Hints       []string `json:"hints"` // 澄清轮累积的补充材料（可选）
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Description) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description 必填"})
		return
	}
	if s.LLM == nil || s.OntoChat == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	job := &draftJob{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	go func() {
		res, err := s.OntoChat.ExtractCQsFromMaterial(context.Background(), req.Description, req.Hints, nil,
			func(_ int, msg string) {
				draftJobsMu.Lock()
				job.Progress = msg
				draftJobsMu.Unlock()
			})
		draftJobsMu.Lock()
		defer draftJobsMu.Unlock()
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			return
		}
		job.Status = "done"
		job.CQs = res.CQs
		job.Dedup = res.Dedup
		job.Purify = res.Purification
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// aiDraftJob GET /api/ai-draft-jobs/{id} 轮询。
// REQ-381②/M177：进程内 map 未命中回读 async_job 表（重启后 done/failed 结果仍可查；
// running 行已被启动对账标 error）。
func (s *Server) aiDraftJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	draftJobsMu.Lock()
	job, ok := draftJobs[id]
	draftJobsMu.Unlock()
	if ok {
		writeJSON(w, http.StatusOK, job)
		return
	}
	if s.Store != nil {
		if row, gerr := s.Store.GetAsyncJob(id); gerr == nil && row != nil {
			var j draftJob
			if json.Unmarshal([]byte(row.ResultJSON), &j) == nil && j.ID != "" {
				writeJSON(w, http.StatusOK, &j)
				return
			}
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "job 不存在（或服务重启丢失——请重新发起）"})
}
