package api

// REQ-555/M362 运行回放文件导出单测：envelope 结构（format/fidelity 自声明/schema_versions）/
// 事件与 ListEventsQ 升序逐条相等/终态门禁（进行中 409）/负路径（缺 run_id 400/未知 run 404）。

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func strPtr(v string) *string { return &v }

func newExportFixture(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Server{Store: st}
}

func TestExportRunEvents_Req555(t *testing.T) {
	s := newExportFixture(t)
	conv, err := s.Store.CreateConversation(&store.Conversation{AgentID: strPtr("builtin-assistant"), Title: "导出测试", Scope: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-exp-1"
	evs := []*store.RunEvent{
		{ConversationID: conv.ID, RunID: runID, Type: "run.started", Data: `{"agent_name":"a"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "tool.result", Data: `{"name":"read_file","truncated":true}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "message.delta", Data: `{"text":"你好"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "run.finished", Data: `{"reason":"completed"}`, SchemaVersion: 2},
	}
	for _, e := range evs {
		if _, err := s.Store.InsertEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	// ① 正常导出：200 + envelope 断言
	w := doReq216(s, s.exportRunEvents, "GET", "/api/conversations/"+conv.ID+"/events/export?run_id="+runID, nil, map[string]string{"id": conv.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("导出应 200，got %d: %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "replay-") {
		t.Fatalf("应附件下载: %q", cd)
	}
	var env struct {
		Format        string `json:"format"`
		FormatVersion int    `json:"format_version"`
		Source        struct {
			ConversationID string `json:"conversation_id"`
			RunID          string `json:"run_id"`
			Title          string `json:"title"`
		} `json:"source"`
		Fidelity map[string]any   `json:"fidelity"`
		Events   []store.RunEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("envelope 解析失败: %v", err)
	}
	if env.Format != "ontoagents-replay" || env.FormatVersion != 1 {
		t.Fatalf("format 失真: %s v%d", env.Format, env.FormatVersion)
	}
	if env.Source.RunID != runID || env.Source.ConversationID != conv.ID || env.Source.Title != "导出测试" {
		t.Fatalf("source 失真: %+v", env.Source)
	}
	if env.Fidelity["message_delta"] != true || env.Fidelity["model_step"] != false || env.Fidelity["tool_result_truncated"] != true {
		t.Fatalf("fidelity 自声明失真: %v", env.Fidelity)
	}
	if int(env.Fidelity["event_count"].(float64)) != 4 || env.Fidelity["terminal_type"] != "run.finished" {
		t.Fatalf("fidelity 终态/计数失真: %v", env.Fidelity)
	}
	svs := env.Fidelity["schema_versions"].([]any)
	if len(svs) != 1 || svs[0] != float64(2) {
		t.Fatalf("schema_versions 失真: %v", svs)
	}
	// ② 事件逐条相等（type/data/schema_version 升序——107 验收①）
	stored, _, err := s.Store.ListEventsQ(conv.ID, store.EventQuery{RunID: runID, Order: "asc"})
	if err != nil || len(stored) != 4 {
		t.Fatalf("取数失真: %v %d", err, len(stored))
	}
	for i, e := range env.Events {
		if e.Type != stored[i].Type || e.Data != stored[i].Data || e.SchemaVersion != stored[i].SchemaVersion {
			t.Fatalf("事件 %d 与取数不一致: %+v vs %+v", i, e, stored[i])
		}
	}
	// ③ 终态门禁：进行中 run（无终态事件）→ 409 人话
	if _, err := s.Store.InsertEvent(&store.RunEvent{ConversationID: conv.ID, RunID: "run-live", Type: "run.started", Data: `{}`, SchemaVersion: 2}); err != nil {
		t.Fatal(err)
	}
	w = doReq216(s, s.exportRunEvents, "GET", "/api/conversations/"+conv.ID+"/events/export?run_id=run-live", nil, map[string]string{"id": conv.ID})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "尚未结束") {
		t.Fatalf("进行中导出应 409 人话: %d %s", w.Code, w.Body.String())
	}
	// ④ 负路径：缺 run_id 400 / 未知 run 404
	w = doReq216(s, s.exportRunEvents, "GET", "/api/conversations/"+conv.ID+"/events/export", nil, map[string]string{"id": conv.ID})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺 run_id 应 400: %d", w.Code)
	}
	w = doReq216(s, s.exportRunEvents, "GET", "/api/conversations/"+conv.ID+"/events/export?run_id=nope", nil, map[string]string{"id": conv.ID})
	if w.Code != http.StatusNotFound {
		t.Fatalf("未知 run 应 404: %d", w.Code)
	}
}
