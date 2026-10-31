package api

// REQ-558/M365 运行生命周期 pause/redo 与产物预览（结构化协作批 P1；107 号 §九）——
// pause=非决策点安全点挂起（复用抢占原语+TurnLoop cleanup checkpoint；与 ask_human/approval
// 决策点挂起区别=用户主动任意时点）；unpause=无答复恢复（ResumePaused 薄变体；instruction
// 非空=checkpoint 锚点 redo 纠偏指令）；redo=消息边界锚点截断重发（run_event 追加式不删——
// 原过程可回看；新 runID+redo_of 溯源键拒绝复活旧 run）；preview=HTML 产物内联沙箱预览。

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/api/sse"
	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// pauseConversation POST /api/conversations/{id}/pause——运行中安全点挂起。
func (s *Server) pauseConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil || conv == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	ok, err := s.Chat.Pause(conv.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		// 无进行中 TurnLoop（或 pause 到达过晚）——409 沿 steer 形态；工作流运行不支持如实
		writeJSON(w, http.StatusConflict, map[string]string{"error": "没有可暂停的运行（无进行中的智能体轮；工作流运行暂不支持暂停）"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"paused": true, "note": "安全点挂起已受理——运行将在下一安全点收束并落 checkpoint"})
}

// unpauseConversation POST /api/conversations/{id}/unpause {instruction?}——暂停恢复
// （SSE 流与 resume 同构；instruction=checkpoint 锚点 redo 纠偏指令，落消息流随全量历史重建）。
func (s *Server) unpauseConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Instruction string `json:"instruction"`
	}
	if r.Body != nil {
		_ = decodeJSON(r, &in) // body 可选
	}
	conv, agent, projectName, err := s.resolveRunTarget(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	agent = chat.ApplyConversationModelOverride(conv, agent)
	sw, err := sse.NewWriter(w)
	if err != nil {
		writeErr(w, err)
		return
	}
	newRunID := store.NewID()
	meta := map[string]any{"run_id": newRunID, "conversation_id": conv.ID, "agent_name": agent.Name, "resumed": "paused"}
	if projectName != "" {
		meta["project_name"] = projectName
	}
	_ = sw.Event("meta", meta)
	emit := func(ev *chat.Event) {
		if ev == nil {
			return
		}
		_ = sw.Event(ev.Type, ev)
	}
	if _, _, err := s.Chat.ResumePaused(r.Context(), conv, agent, in.Instruction, 0, false, emit); err != nil {
		ev := chat.NewErrorEvent(newRunID, "unpause_failed", err.Error())
		_ = sw.Event(ev.Type, ev)
		return
	}
	s.Companion.OnRunComplete(conv, agent, newRunID)
	s.Memory.OnRunComplete(conv, agent, newRunID)
}

// redoConversation POST /api/conversations/{id}/redo {anchor_message_id}——消息边界锚点重跑：
// 删除锚点之后的消息（run_event 追加式不删，原过程可回看）→ redo.anchored 事件（含 redo_of
// 与截断计数）→ 以锚点用户消息重发新 run（新 runID+run.started 携 redo_of——拒绝复活旧 runID）。
func (s *Server) redoConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil || conv == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	var in struct {
		AnchorMessageID string `json:"anchor_message_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(in.AnchorMessageID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "anchor_message_id 必填——选择重跑锚点（锚点本身保留，其后消息截断）"})
		return
	}
	// 锚点从消息列表定位（store 无单条 Get——ListMessages 过滤；消息量级单机可控）
	var anchor *store.Message
	if ms, lerr := s.Store.ListMessages(conv.ID); lerr == nil {
		for _, m := range ms {
			if m.ID == in.AnchorMessageID {
				anchor = m
				break
			}
		}
	}
	if anchor == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "锚点消息不存在或不属于该会话"})
		return
	}
	if anchor.Role != "user" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "锚点须为用户消息（消息边界锚点——checkpoint 锚点走 /unpause instruction）"})
		return
	}
	deleted, err := s.Store.DeleteMessagesAfter(conv.ID, anchor.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	// redo.anchored 事件（结构化审计 schema_version=2；原 run 事件不受影响可完整回放）
	data, _ := json.Marshal(map[string]any{
		"anchor_message_id": anchor.ID, "deleted_messages": deleted,
		"note": "消息边界锚点重跑——锚点后消息已截断；原运行事件保留可回放",
	})
	if ev, ierr := s.Store.InsertEvent(&store.RunEvent{
		ConversationID: conv.ID, Type: chat.EventRedoAnchored, Data: string(data), SchemaVersion: 2,
	}); ierr == nil && ev != nil {
		_ = ev
	}
	// 以锚点用户消息重发新 run（复用 runConversation 全链——SSE/历史重建/指标同源）：
	// 截断保留锚点行（锚点即历史边界），本次输入=锚点内容——runConversation 落新用户消息
	// 与锚点文本一致（redo 语义=同题重做，历史可见两次同文以 redo.anchored 事件解释）
	body, _ := json.Marshal(chat.RunInput{Input: anchor.Content})
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	s.runConversation(w, r)
}

// previewProjectFile GET /api/projects/{id}/files/{fid}/preview——HTML 产物内联沙箱预览：
// 仅白名单文本产物（text/html）内联返回；响应强制 CSP sandbox（无 allow-same-origin——LLM 生成
// HTML 触不到 localStorage/cookie，XSS 面被隔离）+nosniff；非白名单类型 415（107 §9.4 验收⑤）。
func (s *Server) previewProjectFile(w http.ResponseWriter, r *http.Request) {
	pf, err := s.Store.GetProjectFile(r.PathValue("fid"))
	if err != nil || pf == nil || pf.ProjectID != r.PathValue("id") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	proj, err := s.Store.GetProject(pf.ProjectID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	ext := strings.ToLower(filepath.Ext(pf.Name))
	if ext != ".html" && ext != ".htm" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "仅 HTML 产物支持内联预览（白名单单文件静态形态；动态服务类 demo 不做——107 §9.5 边界）"})
		return
	}
	full, jerr := safeJoinPath(s.projectRoot(proj), pf.Path)
	if jerr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	b, err := os.ReadFile(full)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "文件读取失败（绑定目录文件可能已移除）"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 沙箱隔离：无 allow-same-origin（同源能力全禁）；allow-scripts 允许内联脚本演示交互
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}
