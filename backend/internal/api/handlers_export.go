// REQ-113 数据导出与生命周期管理：①对话导出 Markdown（消息 + 可选过程事件附录）；
// ②设置「数据与安全」数据量概览（各表行数 + DB 文件大小 + 对话/项目级联规模）。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// exportConversation GET /api/conversations/{id}/export?events=1：
// 对话导出为 Markdown——头部元信息 + 消息正文（按角色分节）；events=1 时追加过程事件附录（≤500 条）。
func (s *Server) exportConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	msgs, err := s.Store.ListMessages(conv.ID)
	if err != nil {
		writeErr(w, err)
		return
	}

	owner := conv.Scope
	if conv.Scope == "agent" && conv.AgentID != nil {
		if a, err := s.Store.GetAgent(*conv.AgentID); err == nil {
			owner = "智能体 · " + a.Name
		}
	} else if conv.Scope == "project" && conv.ProjectID != nil {
		if p, err := s.Store.GetProject(*conv.ProjectID); err == nil {
			owner = "项目 · " + p.Name
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", conv.Title)
	fmt.Fprintf(&b, "- 会话 ID：%s\n", conv.ID)
	fmt.Fprintf(&b, "- 归属：%s\n", owner)
	fmt.Fprintf(&b, "- 创建时间：%s\n", conv.CreatedAt)
	fmt.Fprintf(&b, "- 导出时间：%s\n\n---\n\n", time.Now().Format("2006-01-02 15:04:05"))
	for _, m := range msgs {
		label := map[string]string{"user": "用户", "assistant": "助手", "system": "系统", "tool": "工具"}[m.Role]
		if label == "" {
			label = m.Role
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", label, strings.TrimRight(m.Content, "\n"))
	}
	if r.URL.Query().Get("events") == "1" {
		if evs, err := s.Store.ListEvents(conv.ID); err == nil && len(evs) > 0 {
			const maxEvents = 500
			if len(evs) > maxEvents {
				evs = evs[:maxEvents]
			}
			b.WriteString("---\n\n## 附录：过程事件\n\n")
			for _, ev := range evs {
				data := strings.ReplaceAll(strings.ReplaceAll(ev.Data, "\n", " "), "\r", " ")
				fmt.Fprintf(&b, "- `%s` **%s** %s\n", ev.CreatedAt, ev.Type, data)
			}
			b.WriteString("\n")
		}
	}

	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(conv.Title), "/", "_"))
	if name == "" || name == "." {
		name = "conversation"
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-%s.md\"", name, conv.ID[:8]))
	_, _ = w.Write([]byte(b.String()))
}

// storageOverview GET /api/stats/storage：数据量概览（各表行数 + DB 文件大小 + 对话/项目级联规模）。
func (s *Server) storageOverview(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.StorageStats()
	if err != nil {
		writeErr(w, err)
		return
	}
	msgCounts, err := s.Store.ConversationMessageCounts()
	if err != nil {
		writeErr(w, err)
		return
	}
	convCounts, err := s.Store.ProjectConversationCounts()
	if err != nil {
		writeErr(w, err)
		return
	}
	type convRow struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Scope     string `json:"scope"`
		Messages  int    `json:"messages"`
		UpdatedAt string `json:"updated_at"`
	}
	convs, err := s.Store.ListConversations(store.ConversationFilter{})
	if err != nil {
		writeErr(w, err)
		return
	}
	convRows := make([]convRow, 0, len(convs))
	for _, c := range convs {
		convRows = append(convRows, convRow{ID: c.ID, Title: c.Title, Scope: c.Scope, Messages: msgCounts[c.ID], UpdatedAt: c.UpdatedAt})
	}
	type projRow struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Conversations int    `json:"conversations"`
	}
	projects, err := s.Store.ListProjects()
	if err != nil {
		writeErr(w, err)
		return
	}
	projRows := make([]projRow, 0, len(projects))
	for _, p := range projects {
		projRows = append(projRows, projRow{ID: p.ID, Name: p.Name, Conversations: convCounts[p.ID]})
	}

	var dbBytes int64
	if s.DBPath != "" {
		if fi, err := os.Stat(s.DBPath); err == nil {
			dbBytes = fi.Size()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"db_bytes":      dbBytes,
		"stats":         st,
		"conversations": convRows,
		"projects":      projRows,
	})
}

// exportTodo REQ-204/M39 C2：任务清单 Markdown 导出（进度产物人类侧；机器侧=conversation.todo_json/todo_write）。
// GET /api/conversations/{id}/todo.md
func (s *Server) exportTodo(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	conv, err := s.Store.GetConversation(convID)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=todo-%s.md", convID[:8]))
	fmt.Fprintf(w, "# 任务清单 · %s\n\n", conv.Title)
	if conv.TodoJSON == "" {
		fmt.Fprintf(w, "> 该会话尚未建立任务清单（模型经 todo_write 工具写入后可导出）。\n")
		return
	}
	var items []struct {
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(conv.TodoJSON), &items); err != nil {
		fmt.Fprintf(w, "> 清单解析失败：%v\n", err)
		return
	}
	done, prog := 0, 0
	fmt.Fprintf(w, "| # | 任务 | 状态 |\n| --- | --- | --- |\n")
	for i, it := range items {
		mark := map[string]string{"done": "✅ 已完成", "in_progress": "🔄 进行中", "pending": "⬜ 待办"}[it.Status]
		if it.Status == "done" {
			done++
		}
		if it.Status == "in_progress" {
			prog++
		}
		fmt.Fprintf(w, "| %d | %s | %s |\n", i+1, it.Title, mark)
	}
	fmt.Fprintf(w, "\n> 进度：%d/%d 完成，%d 进行中（导出于 %s；失败尝试与已知局限见会话过程事件）\n", done, len(items), prog, time.Now().UTC().Format("2006-01-02 15:04"))
}


// ---------------------------------------------------------------------------
// REQ-555/M362 运行回放文件导出（结构化协作批 P0；107 号 §六）——终态 run 落库事件流导出为
// 自包含、格式版本化 JSON 回放文件（envelope：format_version 与 events[].schema_version 两维
// 正交；fidelity 保真度自声明如实统计不虚构缺失）。导出=只读投影（ListEventsQ 单一取数面，
// 与 REQ-527 续看/REQ-217 过滤共享查询语义）；导入回放=纯前端离线行为（不写本地库——不制造
// 伪 conversation 污染统计/检索/伴生）。零迁移；附件下载沿本档 handlers_diag 自助包先例。
// ---------------------------------------------------------------------------

// exportRunEvents GET /api/conversations/{id}/events/export?run_id=
// 仅允许导出已达终态的 run（run.finished/run.error/run.interrupted 之一落库）；
// 进行中运行 409 人话。文件名 replay-{run_id}-{日期}.json（浏览器附件下载）。
func (s *Server) exportRunEvents(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id 必填——选择要导出的运行（轨迹面板「重放此运行」选中态下导出）"})
		return
	}
	conv, err := s.Store.GetConversation(convID)
	if err != nil || conv == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
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
	// 终态门禁（107 §6.3：仅终态 run 可导出——进行中导出会得到永远不完整的文件）
	terminal := ""
	for _, e := range evs {
		switch e.Type {
		case "run.finished", "run.error", "run.interrupted":
			terminal = e.Type
		}
		if terminal != "" {
			break
		}
	}
	if terminal == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "运行尚未结束（无终态事件）——仅终态 run 可导出，可待运行完成后重试"})
		return
	}
	// fidelity 保真度自声明（如实统计该 run 实际落库内容——EventReplayDrawer hasDebugRecorded
	// 启发式的导出侧显式化；未开 debug_persist 的历史缺口如实标注而非静默留白）
	fidelity := map[string]any{
		"message_delta": false, "model_step": false,
		"tool_result_truncated": false,
		"schema_versions":       []int{},
		"event_count":           len(evs),
		"terminal_type":         terminal,
	}
	svSeen := map[int]bool{}
	svs := []int{}
	toolTruncated := false
	for _, e := range evs {
		switch e.Type {
		case "message.delta":
			fidelity["message_delta"] = true
		case "model.step":
			fidelity["model_step"] = true
		case "tool.result":
			var d map[string]any
			if json.Unmarshal([]byte(e.Data), &d) == nil {
				if t, ok := d["truncated"].(bool); ok && t {
					toolTruncated = true
				}
			}
		}
		if e.SchemaVersion > 0 && !svSeen[e.SchemaVersion] {
			svSeen[e.SchemaVersion] = true
			svs = append(svs, e.SchemaVersion)
		}
	}
	fidelity["tool_result_truncated"] = toolTruncated
	fidelity["schema_versions"] = svs
	envelope := map[string]any{
		"format":         "ontoagents-replay",
		"format_version": 1,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"generator":      "ontoagents",
		"source": map[string]any{
			"conversation_id": convID, "run_id": runID,
			"agent_name": conv.AgentID, // 会话归属智能体 id（名称快照不回溯——诚实口径）
			"title":      conv.Title, "scope": conv.Scope,
		},
		"fidelity": fidelity,
		"events":   evs,
	}
	b, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=replay-%s-%s.json", sanitizeFilePart(runID), time.Now().UTC().Format("20060102")))
	_, _ = w.Write(b)
}

// sanitizeFilePart 文件名安全段（run id 为平台生成 id——防御性只留安全字符）。
func sanitizeFilePart(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
