package api

// C4 Evaluator（REQ-388/M387——运行质量评估器）：对终态 run 做质量归因。
// 复用 REQ-223 agenteval Judge 契约（agenteval.EvaluateRun）不开新面——55 号既定口径；
// 评估输入双源组装=对话末轮 goal/answer 快照（message 表）+ run 工具轨迹/警告（run_event 表）；
// 结果落 run_evaluation（迁移 089）可回溯。诚实口径：message.delta 不落库，answer 取 message
// 表最终 assistant 文本（与回放/导出的保真边界一致——非 live 流全量）。

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/agenteval"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// evaluateRun POST /api/conversations/{id}/runs/{run_id}/evaluate {conn_id?}
// → {id,run_id,score,verdict,reason,tools,created_at}（终态门禁沿 REQ-555 导出口径）。
func (s *Server) evaluateRun(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	runID := r.PathValue("run_id")
	var in struct {
		ConnID string `json:"conn_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.Store.GetConversation(convID); err != nil {
		writeErr(w, err)
		return
	}
	evs, _, err := s.Store.ListEventsQ(convID, store.EventQuery{RunID: runID, Order: "asc"})
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(evs) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "该 run 无落库事件（run_id 有误或事件未落库）"})
		return
	}
	// 终态门禁（沿 exportRunEvents REQ-555 口径——进行中评估会得到不完整归因）
	terminal := ""
	var tools []string
	var warnings []string
	errMsg := ""
	for _, e := range evs {
		switch e.Type {
		case "run.finished", "run.interrupted":
			terminal = e.Type
		case "run.error":
			terminal = e.Type
			var d map[string]any
			if json.Unmarshal([]byte(e.Data), &d) == nil {
				errMsg, _ = d["message"].(string)
			}
		case "tool.call", "run.warning":
			var d map[string]any
			if json.Unmarshal([]byte(e.Data), &d) != nil {
				continue
			}
			if e.Type == "tool.call" {
				if name, _ := d["tool_name"].(string); name != "" {
					tools = append(tools, name)
				}
			} else if m, _ := d["message"].(string); m != "" {
				warnings = append(warnings, m)
			}
		}
	}
	if terminal == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "运行尚未结束（无终态事件）——仅终态 run 可评估，可待运行完成后重试"})
		return
	}
	goal, answer := s.Store.LastUserAndAssistantMessages(convID)
	if goal == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该会话无用户输入可作评估目标"})
		return
	}
	res, err := agenteval.EvaluateRun(r.Context(), s.Store, s.Box, in.ConnID, agenteval.RunEvalInput{
		Goal: goal, Answer: answer, Tools: tools, Warnings: warnings, ErrMsg: errMsg,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	rec, err := s.Store.InsertRunEvaluation(&store.RunEvaluation{
		RunID: runID, ConversationID: convID,
		Goal: goal, Answer: answer,
		Score: res.Score, Verdict: res.Verdict, Reason: res.Reason,
		Tools: tools,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// listRunEvaluations GET /api/conversations/{id}/runs/{run_id}/evaluations：评估历史（倒序）。
func (s *Server) listRunEvaluations(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id 必填"})
		return
	}
	evs, err := s.Store.ListRunEvaluations(runID, 20)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": evs})
}
