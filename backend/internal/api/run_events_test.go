package api

// REQ-527/M334（P16 后半）：事件回放端点单测——run_id 过滤+offset 增量+排序升序。

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestRunEventsReplay(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "replay-agent"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "replay"})
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"run.started", "run.warning", "run.finished"} {
		if _, err := st.InsertEvent(&store.RunEvent{ConversationID: conv.ID, RunID: "run-replay", Type: typ, Data: "{}", SchemaVersion: store.EventSchemaVersion}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Millisecond) // created_at 单调（同毫秒插入排序按随机 id 不稳定）
	}
	// 干扰 run 的事件（run_id 过滤应排除）
	if _, err := st.InsertEvent(&store.RunEvent{ConversationID: conv.ID, RunID: "run-other", Type: "run.started", Data: "{}", SchemaVersion: store.EventSchemaVersion}); err != nil {
		t.Fatal(err)
	}

	s := &Server{Store: st, DBPath: filepath.Join(dir, "platform.db")}
	req := httptest.NewRequest("GET", "/x?run_id=run-replay", nil)
	req.SetPathValue("id", conv.ID)
	rec := httptest.NewRecorder()
	s.runEvents(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"total":3`) || !strings.Contains(body, "run.finished") {
		t.Fatalf("应回放 3 条含终态: %.240s", body)
	}
	// offset 增量：offset=2 只剩终态
	req2 := httptest.NewRequest("GET", "/x?run_id=run-replay&offset=2", nil)
	req2.SetPathValue("id", conv.ID)
	rec2 := httptest.NewRecorder()
	s.runEvents(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "run.finished") || strings.Contains(rec2.Body.String(), "run.started") {
		t.Fatalf("offset=2 应只含终态: %.200s", rec2.Body.String())
	}
}
