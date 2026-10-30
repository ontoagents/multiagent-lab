// REQ-478③ 派生会话单测：POST /api/conversations/{id}/fork——
// 配置拷贝（agent/KB/方案/开关族）+ 全量消息拷贝 + 新会话隔离（挂起/todo/事件不随行）+ 标题「（派生）」后缀。
package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func strP(s string) *string { return &s }

func TestForkConversationCopiesConfigAndMessages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Box: box}

	agentID := "ag-src"
	if _, err := st.CreateAgent(&store.Agent{ID: agentID, Name: "源智能体"}); err != nil {
		t.Fatal(err)
	}
	src, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &agentID, Title: "源会话", KBID: strP("kb-1"), EnableKB: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct{ role, content string }{
		{"user", "第一问"},
		{"assistant", "第一答"},
		{"user", "第二问"},
		{"assistant", "第二答"},
	} {
		if _, err := st.InsertMessage(&store.Message{ConversationID: src.ID, Role: m.role, Content: m.content}); err != nil {
			t.Fatal(err)
		}
	}
	// 源会话挂起状态/todo 置位——派生会话不应继承
	if _, err := st.InsertEvent(&store.RunEvent{ConversationID: src.ID, RunID: "r1", Type: "run.started", Data: "{}"}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/conversations/"+src.ID+"/fork", strings.NewReader(`{}`))
	req.SetPathValue("id", src.ID)
	s.forkConversation(w, req)
	if w.Code != 201 {
		t.Fatalf("fork status = %d, body = %s", w.Code, w.Body.String())
	}
	var forked store.Conversation
	if err := json.Unmarshal(w.Body.Bytes(), &forked); err != nil {
		t.Fatal(err)
	}
	if forked.ID == src.ID {
		t.Fatal("fork must create a new conversation id")
	}
	if forked.Title != "源会话（派生）" {
		t.Fatalf("fork title = %q, want 源会话（派生）", forked.Title)
	}
	if forked.AgentID == nil || *forked.AgentID != agentID {
		t.Fatalf("fork agent_id = %v, want %s", forked.AgentID, agentID)
	}
	if forked.KBID == nil || *forked.KBID != "kb-1" || !forked.EnableKB {
		t.Fatalf("fork kb config not copied: kb=%v enable=%v", forked.KBID, forked.EnableKB)
	}
	if forked.InterruptState != "" || forked.TodoJSON != "" {
		t.Fatalf("fork must not inherit run state: interrupt=%q todo=%q", forked.InterruptState, forked.TodoJSON)
	}
	msgs, err := st.ListMessages(forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("fork messages = %d, want 4", len(msgs))
	}
	if msgs[0].Content != "第一问" || msgs[3].Content != "第二答" {
		t.Fatalf("fork message order/content drifted: %q..%q", msgs[0].Content, msgs[3].Content)
	}
	evs, err := st.ListEvents(forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("fork must not copy run events, got %d", len(evs))
	}
	// 显式标题覆盖
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/conversations/"+src.ID+"/fork", strings.NewReader(`{"title":"自定义分支"}`))
	req2.SetPathValue("id", src.ID)
	s.forkConversation(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("fork(2) status = %d", w2.Code)
	}
	var forked2 store.Conversation
	_ = json.Unmarshal(w2.Body.Bytes(), &forked2)
	if forked2.Title != "自定义分支" {
		t.Fatalf("explicit title = %q, want 自定义分支", forked2.Title)
	}
}
