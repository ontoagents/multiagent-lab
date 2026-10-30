package a2a

// REQ-441/M237：任务状态机与取消可靠性收口单测——①cancel 后台完成不覆写 canceled
// ②cancel 写穿落库（重启对账不再 working→failed 漂移）③Busy 会话级单运行闸。

import (
	"context"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestCancelReliability(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	agent, err := st.CreateAgent(&store.Agent{Name: "取消测试", Description: "d"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := New(st)
	block := make(chan struct{})
	started := make(chan struct{})
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		close(started)
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return "后台完成结果", nil
	}
	svc.Stop = func(string) bool { return true }

	_, out := rpc(t, svc, agent.ID, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "长任务"}},
		}},
	})
	taskID := out["result"].(map[string]any)["id"].(string)
	<-started

	_, out = rpc(t, svc, agent.ID, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tasks/cancel", "params": map[string]any{"id": taskID}})
	if s := out["result"].(map[string]any)["status"].(map[string]any)["state"]; s != "canceled" {
		t.Fatalf("cancel 后应 canceled: %v", s)
	}

	// ① cancel 写穿：DB 行即 canceled
	row, gerr := st.GetA2ATask(taskID)
	if gerr != nil || row == nil {
		t.Fatalf("任务未落库: %v", gerr)
	}
	if row.State != "canceled" {
		t.Fatalf("DB 应 canceled（写穿）: %s", row.State)
	}

	// ② 后台完成不得覆写 canceled（goroutine guard）
	close(block)
	deadline := time.Now().Add(2 * time.Second)
	state := "working"
	for time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
		_, out = rpc(t, svc, agent.ID, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tasks/get", "params": map[string]any{"id": taskID}})
		state = out["result"].(map[string]any)["status"].(map[string]any)["state"].(string)
		if state != "working" {
			break
		}
	}
	if state != "canceled" {
		t.Fatalf("后台完成后状态应保持 canceled，得 %s", state)
	}
	row, _ = st.GetA2ATask(taskID)
	if row != nil && row.State != "canceled" {
		t.Fatalf("DB 终态应保持 canceled: %s", row.State)
	}
}

func TestMessageSendBusyGate(t *testing.T) {
	svc, st, _ := newA2AFixture(t)
	agents, _ := st.ListAgents()
	aid := agents[0].ID
	svc.Busy = func(conversationID string) bool { return true }
	_, out := rpc(t, svc, aid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "并发"}},
		}},
	})
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("应回 JSON-RPC error: %v", out)
	}
	if code, _ := errObj["code"].(float64); code != -32003 {
		t.Fatalf("错误码应为 -32003（会话忙）: %v", errObj)
	}
}
