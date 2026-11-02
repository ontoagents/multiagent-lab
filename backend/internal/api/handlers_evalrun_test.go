package api

// REQ-388/M387 C4 handler 单测：LLM 桩注入（agenteval.runJudgeLLM）——终态门禁/双源组装/落档。

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/agenteval"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func evalrunTestServer(t *testing.T) (*Server, *store.Store) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Store: st, Box: box, DBPath: filepath.Join(dir, "platform.db")}, st
}

// fixtureRun 造对话+末轮消息+run 事件（tool.call+run.finished 终态）。
func fixtureRun(t *testing.T, st *store.Store, runID string, withTerminal bool) (convID string) {
	t.Helper()
	agentID := "builtin-assistant"
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &agentID, Title: "evalrun 冒烟"})
	if err != nil {
		t.Fatal(err)
	}
	convID = conv.ID
	if _, err = st.InsertMessage(&store.Message{ConversationID: convID, Role: "user", Content: "查北京天气"}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.InsertMessage(&store.Message{ConversationID: convID, Role: "assistant", Content: "北京今日晴 25 度"}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.InsertEvent(&store.RunEvent{ConversationID: convID, RunID: runID, Type: "tool.call", Data: `{"tool_name":"weather"}`}); err != nil {
		t.Fatal(err)
	}
	if withTerminal {
		if _, err = st.InsertEvent(&store.RunEvent{ConversationID: convID, RunID: runID, Type: "run.finished", Data: `{"reason":"done"}`}); err != nil {
			t.Fatal(err)
		}
	}
	return convID
}

func TestEvaluateRunFullChain(t *testing.T) {
	s, st := evalrunTestServer(t)
	defer agenteval.SetRunJudge(func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		return `{"score":1,"verdict":"partial","reason":"主体达成但缺湿度"}`, nil
	})()
	runID := "run_ev1"
	convID := fixtureRun(t, st, runID, true)

	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{}`))
	req.SetPathValue("id", convID)
	req.SetPathValue("run_id", runID)
	rec := httptest.NewRecorder()
	s.evaluateRun(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	evs, err := st.ListRunEvaluations(runID, 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("评估应落档 1 条: %v %d", err, len(evs))
	}
	e := evs[0]
	if e.Score != 1 || e.Verdict != "partial" || e.Goal != "查北京天气" || e.Answer != "北京今日晴 25 度" {
		t.Fatalf("落档内容不符: %+v", e)
	}
	if len(e.Tools) != 1 || e.Tools[0] != "weather" {
		t.Fatalf("工具轨迹不符: %v", e.Tools)
	}

	// 历史端点
	req = httptest.NewRequest("GET", "/x", nil)
	req.SetPathValue("run_id", runID)
	rec = httptest.NewRecorder()
	s.listRunEvaluations(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "partial") {
		t.Fatalf("评估历史不符: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEvaluateRunGateNonTerminal(t *testing.T) {
	s, st := evalrunTestServer(t)
	runID := "run_ev2"
	convID := fixtureRun(t, st, runID, false) // 无终态事件
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{}`))
	req.SetPathValue("id", convID)
	req.SetPathValue("run_id", runID)
	rec := httptest.NewRecorder()
	s.evaluateRun(rec, req)
	if rec.Code != 409 {
		t.Fatalf("无终态应 409，got %d body=%s", rec.Code, rec.Body.String())
	}
}
