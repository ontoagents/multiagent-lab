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

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontochat"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
)

type draftJob struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`             // running | done | failed
	Progress  string                 `json:"progress,omitempty"` // REQ-295③（A4）：服务端实测计数进度（轮次/问题数，永不虚构百分比与 ETA）
	Result    *draftResult           `json:"result,omitempty"`
	CQs       []ontochat.ExtractedCQ `json:"cqs,omitempty"`   // REQ-308：CQ 抽取 job 结果（ai-cq-extract）
	Dedup     int                    `json:"dedup,omitempty"` // REQ-308：与既有 CQ 重复被滤除条数
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
	go func() {
		res, err := s.LLM.DraftWithCQ(context.Background(), req.Description, req.ExtraHint, req.CapabilityQuestions,
			func(round int, msg string) { // REQ-295③：实测轮次进度透出（「第 N/M 轮…」，永不虚构百分比）
				draftJobsMu.Lock()
				job.Progress = msg
				draftJobsMu.Unlock()
			}) // REQ-248/242：CQ 回写+质量透出
		draftJobsMu.Lock()
		defer draftJobsMu.Unlock()
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			return
		}
		job.Status = "done"
		// spec 结构体 → map（与同步端点 JSON 形态一致）
		b, _ := json.Marshal(res.Spec)
		var specMap map[string]any
		_ = json.Unmarshal(b, &specMap)
		job.Result = &draftResult{Spec: specMap, Rounds: res.Rounds, Quality: res.Quality}
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
		cqs, dedup, err := s.OntoChat.ExtractCQsFromMaterial(context.Background(), req.Description, req.Hints, nil,
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
		job.CQs = cqs
		job.Dedup = dedup
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// aiDraftJob GET /api/ai-draft-jobs/{id} 轮询。
func (s *Server) aiDraftJob(w http.ResponseWriter, r *http.Request) {
	draftJobsMu.Lock()
	job, ok := draftJobs[r.PathValue("id")]
	draftJobsMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job 不存在（或服务重启丢失——请重新发起）"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}
