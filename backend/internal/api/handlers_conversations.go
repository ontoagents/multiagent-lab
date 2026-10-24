package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ---- Conversations ----

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	convs, err := s.Store.ListConversations(store.ConversationFilter{
		Scope:     q.Get("scope"),
		AgentID:   q.Get("agent_id"),
		ProjectID: q.Get("project_id"),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if convs == nil {
		convs = []*store.Conversation{}
	}
	writeJSON(w, http.StatusOK, convs)
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var c store.Conversation
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, err)
		return
	}
	if c.Scope != "agent" && c.Scope != "project" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be agent|project"})
		return
	}
	created, err := s.Store.CreateConversation(&c)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) updateConversation(w http.ResponseWriter, r *http.Request) {
	var c store.Conversation
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, err)
		return
	}
	c.ID = r.PathValue("id")
	// REQ-400/M196：If-Match 乐观锁（同 updateAgent 口径）
	if prev, perr := s.Store.GetConversation(c.ID); perr == nil && prev != nil {
		if !checkIfMatch(w, r, prev.UpdatedAt) {
			return
		}
	}
	updated, err := s.Store.UpdateConversation(&c)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteConversation(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.Store.ListMessages(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

// listEvents GET /api/conversations/{id}/events?run_id=&type=&type_prefix=&limit=&offset=&order=
// REQ-217③：query 过滤扩展（不带参数=全量裸数组，向后兼容）；X-Total-Count 头恒回命中总数供分页。
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := store.EventQuery{
		RunID:      strings.TrimSpace(r.URL.Query().Get("run_id")),
		Type:       strings.TrimSpace(r.URL.Query().Get("type")),
		TypePrefix: strings.TrimSpace(r.URL.Query().Get("type_prefix")), // REQ-281：伴生事件族前缀过滤
		Limit:      atoiDefault(r.URL.Query().Get("limit"), 0),
		Offset:     atoiDefault(r.URL.Query().Get("offset"), 0),
		Order:      strings.TrimSpace(r.URL.Query().Get("order")), // REQ-330①：desc=最新优先（收尾轮询增量取数）
	}
	events, total, err := s.Store.ListEventsQ(r.PathValue("id"), q)
	if err != nil {
		writeErr(w, err)
		return
	}
	if events == nil {
		events = []*store.RunEvent{}
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, events)
}

// listMessagesPaged GET /api/conversations/{id}/messages/paged?page=&page_size=：
// REQ-413/M209 长会话护栏——分页读取（倒序定位正序还原；X-Total-Count 回总数供前端页脚）。
// 不带参数（page/page_size 缺省）返回最近一页；全量语义沿 GET /messages 不动。
func (s *Server) listMessagesPaged(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetConversation(id); err != nil {
		writeErr(w, err)
		return
	}
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	size := atoiDefault(r.URL.Query().Get("page_size"), 200)
	msgs, total, err := s.Store.ListMessagesPaged(id, page, size)
	if err != nil {
		writeErr(w, err)
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, msgs)
}

// conversationState GET /api/conversations/{id}/state：REQ-224① 状态机抽象读模型——
// 状态族（interrupt_state/context_state/todo_json/schedule）收敛为单一快照+派生 phase。
// phase 优先级=waiting_approval / waiting_human（挂起中断，最需人注意）> running >
// scheduled > idle；家族各成员全量透出不隐藏（治理面可核对派生依据）。
func (s *Server) conversationState(w http.ResponseWriter, r *http.Request) {
	conv, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	running := s.Chat != nil && s.Chat.IsRunning(conv.ID)
	var interrupt json.RawMessage
	if conv.InterruptState != "" {
		interrupt = json.RawMessage(conv.InterruptState)
	}
	var schedule *store.ConversationSchedule
	if sc, err := s.Store.GetSchedule(conv.ID); err == nil {
		schedule = sc
	}
	phase := "idle"
	if running {
		phase = "running"
	}
	if schedule != nil && phase == "idle" {
		phase = "scheduled"
	}
	if interrupt != nil {
		var st struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal([]byte(conv.InterruptState), &st) == nil && st.Kind == "approval" {
			phase = "waiting_approval"
		} else {
			phase = "waiting_human"
		}
	}
	todoHead := conv.TodoJSON
	if n := len(todoHead); n > 300 {
		todoHead = todoHead[:300] + "…"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": conv.ID,
		"phase":           phase,
		"running":         running,
		"interrupt_state": interrupt,
		"scheduled":       schedule != nil,
		"schedule":        schedule,
		"todo_present":    conv.TodoJSON != "",
		"todo_head":       todoHead,
		"context_state":   conv.ContextState != "",
		"updated_at":      conv.UpdatedAt,
	})
}

// conversationStateLog GET /api/conversations/{id}/state-log?limit=：REQ-224① 状态变更留痕
// （interrupt_state/context_state/todo_json/schedule 四族 prev→next 快照，倒序）。
func (s *Server) conversationStateLog(w http.ResponseWriter, r *http.Request) {
	logs, err := s.Store.ListConversationStateLog(r.PathValue("id"), atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, err)
		return
	}
	if logs == nil {
		logs = []*store.ConversationStateLog{}
	}
	writeJSON(w, http.StatusOK, logs)
}
