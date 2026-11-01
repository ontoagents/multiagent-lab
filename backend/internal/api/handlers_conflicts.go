package api

// REQ-553/M360 方案分歧人工裁决 API（107 号 §4.5）：
//   GET  /api/conversations/{id}/conflicts                      分歧清单（审计/回放/裁决卡）
//   POST /api/conversations/{id}/conflicts/{cid}/resolve        人工裁决（SSE——suspended 态
//        复用 Service.Resume 定向恢复续跑流；pending 态回注消息发起新运行流）
// 两态统一落 resolution（decision_source=manual，审计对齐 approval.granted）+conflict.resolved 事件。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/api/sse"
	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// listConflicts GET 分歧清单。
func (s *Server) listConflicts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListConflictReviews(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// resolveConflict POST 人工裁决（SSE 流：meta → conflict.resolved → suspended:run.*/pending:run.*）。
func (s *Server) resolveConflict(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	cid := r.PathValue("cid")
	var in struct {
		Adopt     string `json:"adopt"`      // a | b | third
		ThirdText string `json:"third_text"` // adopt=third 必填
		Rationale string `json:"rationale"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.Adopt = strings.ToLower(strings.TrimSpace(in.Adopt))
	switch in.Adopt {
	case "a", "b":
	case "third":
		if strings.TrimSpace(in.ThirdText) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "adopt=third 时 third_text 必填（第三案全文）"})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "adopt 须为 a|b|third（A 案/B 案/第三案）"})
		return
	}
	cr, err := s.Store.GetConflictReview(cid)
	if err != nil {
		writeErr(w, err)
		return
	}
	if cr == nil || cr.ConversationID != convID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "分歧裁决单不存在或不属于该会话"})
		return
	}
	if cr.State != "suspended" && cr.State != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "分歧已裁决（" + cr.State + "）——终态不可再裁"})
		return
	}
	res := store.ConflictResolution{
		Adopt: in.Adopt, ThirdText: strings.TrimSpace(in.ThirdText),
		Rationale: truncNote300(in.Rationale), DecisionSource: "manual",
		DecidedAt: time.Now().UTC().Format(time.RFC3339),
	}
	resJSON, _ := json.Marshal(&res)
	// 先落裁决（审计链单向——恢复失败不回滚裁决，人工动作已发生；重试仅续跑）
	if _, rerr := s.Store.ResolveConflictReview(cid, string(resJSON)); rerr != nil {
		writeErr(w, rerr)
		return
	}

	sw, err := sse.NewWriter(w)
	if err != nil {
		writeErr(w, err)
		return
	}
	emit := func(ev *chat.Event) {
		if ev == nil {
			return
		}
		_ = sw.Event(ev.Type, ev)
	}
	// conflict.resolved 事件（事件流+run_event 落库同源——store.InsertEvent 直录；
	// schema_version=2 结构化审计沿 REQ-224 契约）
	data, _ := json.Marshal(map[string]any{
		"conflict_id": cid, "adopt": res.Adopt, "decision_source": res.DecisionSource,
		"has_third": res.ThirdText != "", "from_state": cr.State,
	})
	if ev, ierr := s.Store.InsertEvent(&store.RunEvent{
		ConversationID: convID, Type: chat.EventConflictReso, Data: string(data), SchemaVersion: 2,
	}); ierr == nil && ev != nil {
		var parsed chat.Event
		if json.Unmarshal([]byte(ev.Data), &parsed) == nil {
			parsed.Type = ev.Type
			_ = sw.Event(ev.Type, &parsed)
		} else {
			_ = sw.Event(ev.Type, map[string]any{"conflict_id": cid, "adopt": res.Adopt})
		}
	}

	conv, agent, projectName, rerr := s.resolveRunTarget(r)
	if rerr != nil {
		ev := chat.NewErrorEvent("", "resolve_failed", rerr.Error())
		_ = sw.Event(ev.Type, ev)
		return
	}
	agent = chat.ApplyConversationModelOverride(conv, agent)

	if cr.State == "suspended" {
		// 挂起态：定向恢复续跑（复用 Service.Resume 全链——并发闸/checkpoint/事件管线同源）。
		// 挂起校验：interrupt_state 须仍指向本裁决单（被新消息放弃/已被答复则 409）。
		var st struct {
			Kind       string `json:"kind"`
			ConflictID string `json:"conflict_id"`
			RunID      string `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(conv.InterruptState), &st)
		if st.Kind != "conflict" || st.ConflictID != cid || st.RunID == "" {
			_ = sw.Event("run.error", chat.NewErrorEvent("", "resolve_failed", "该分歧的挂起状态已失效（可能已被放弃或答复）——裁决已留痕，续跑需重新发起"))
			return
		}
		meta := map[string]any{"run_id": st.RunID, "conversation_id": conv.ID, "agent_name": agent.Name, "resumed": true, "resolved_conflict": cid}
		if projectName != "" {
			meta["project_name"] = projectName
		}
		_ = sw.Event("meta", meta)
		answer := composeResolutionText(cr, &res)
		if _, rerr := s.Chat.Resume(r.Context(), conv, agent, st.RunID, answer, 0, false, emit); rerr != nil {
			_ = sw.Event("run.error", chat.NewErrorEvent(st.RunID, "resume_failed", rerr.Error()))
			return
		}
		s.Companion.OnRunComplete(conv, agent, st.RunID)
		s.Memory.OnRunComplete(conv, agent, st.RunID)
		return
	}
	// pending 态：裁决文本+分歧要点摘要作为回注消息发起新运行（显式落消息流，用户可见）
	meta := map[string]any{"conversation_id": conv.ID, "agent_name": agent.Name, "resolved_conflict": cid, "reinject": true}
	if projectName != "" {
		meta["project_name"] = projectName
	}
	_ = sw.Event("meta", meta)
	runID := store.NewID()
	if _, rerr := s.Chat.Run(r.Context(), conv, agent, runID, composeResolutionText(cr, &res), 0, false, emit); rerr != nil {
		_ = sw.Event("run.error", chat.NewErrorEvent(runID, "run_failed", rerr.Error()))
		return
	}
	s.Companion.OnRunComplete(conv, agent, runID)
	s.Memory.OnRunComplete(conv, agent, runID)
}

// composeResolutionText 裁决回注文本（结构化摘要——分歧要点+裁决结论，不注入双方全量轨迹，
// 107 号 §4.8 预算沿 REQ-531）。
func composeResolutionText(cr *store.ConflictReview, res *store.ConflictResolution) string {
	var b strings.Builder
	b.WriteString("【方案分歧裁决】")
	switch res.Adopt {
	case "a":
		b.WriteString("采纳 A 案（评审提交方的替代方案）")
	case "b":
		b.WriteString("采纳 B 案（原方案维持）")
	default:
		b.WriteString("采用第三案：" + res.ThirdText)
	}
	if res.Rationale != "" {
		b.WriteString("。理由：" + res.Rationale)
	}
	if cr.Summary != "" {
		b.WriteString("。分歧要点回顾：" + cr.Summary)
	}
	b.WriteString("。请按本裁决继续执行；如裁决涉及你此前立场，请以裁决为准并说明后续安排。")
	return b.String()
}
