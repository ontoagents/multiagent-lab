package a2a

// REQ-444/M240 + REQ-445/M241：card 鉴权声明与错误脱敏 / 任务 map 淘汰与保留窗口单测。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestCardSecurityDeclaration(t *testing.T) {
	svc, st, _ := newA2AFixture(t)
	agents, _ := st.ListAgents()
	card := svc.AgentCard(agents[0])
	if card["provider"] == nil {
		t.Fatal("card 应含 provider 元数据（REQ-444）")
	}
	if _, has := card["securitySchemes"]; has {
		t.Fatal("未设 PLATFORM_TOKEN 时不应带 securitySchemes")
	}
	t.Setenv("PLATFORM_TOKEN", "secret")
	card = svc.AgentCard(agents[0])
	schemes, ok := card["securitySchemes"].(map[string]any)
	if !ok || schemes["bearer"] == nil {
		t.Fatalf("TOKEN 启用时应有 bearer securityScheme: %v", card["securitySchemes"])
	}
	if card["security"] == nil {
		t.Fatal("TOKEN 启用时应有 security 引用")
	}
}

func TestFailedErrorSanitized(t *testing.T) {
	svc, st, _ := newA2AFixture(t)
	agents, _ := st.ListAgents()
	aid := agents[0].ID
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		return "", errors.New("dial tcp /home/dongkui/secret.go:42: connection refused（sql: no rows）")
	}
	_, out := rpc(t, svc, aid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "会失败"}},
		}},
	})
	taskID := out["result"].(map[string]any)["id"].(string)
	state, msg := pollTask(t, svc, aid, taskID)
	if state != "failed" {
		t.Fatalf("应 failed: %s", state)
	}
	if msg == "" {
		t.Fatal("失败原因应经 status.message 外发（wire 完整性）")
	}
	if strings.Contains(msg, "/home/dongkui") || strings.Contains(msg, ".go:42") {
		t.Fatalf("内部路径/行号应脱敏: %s", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Fatalf("概要应保留: %s", msg)
	}
}

func TestTaskMapEvictionAndRetention(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	agent, _ := st.CreateAgent(&store.Agent{Name: "淘汰测试", Description: "d"})
	svc := New(st)
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		return "ok", nil
	}
	// 预置 245 条终态任务（模拟长期运行驻留）
	svc.mu.Lock()
	for i := 0; i < 245; i++ {
		svc.tasks["old-"+string(rune('a'+i%26))+"-"+time.Now().Format("150405.000000000")+string(rune(i%26+'a'))] = &Task{ID: "x", State: "completed"}
	}
	nBefore := len(svc.tasks)
	svc.mu.Unlock()

	_, out := rpc(t, svc, agent.ID, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "触发淘汰"}},
		}},
	})
	taskID := out["result"].(map[string]any)["id"].(string)
	svc.mu.Lock()
	nAfter := len(svc.tasks)
	svc.mu.Unlock()
	if nBefore <= 240 || nAfter > 201 {
		t.Fatalf("淘汰未生效：before=%d after=%d（应 ≤201）", nBefore, nAfter)
	}
	// 被淘汰任务仍可经 DB 回读查询（tasks/get 不回插终态）
	if state, _ := pollTask(t, svc, agent.ID, taskID); state != "completed" {
		t.Fatalf("新任务应 completed: %s", state)
	}
	svc.mu.Lock()
	nFinal := len(svc.tasks)
	svc.mu.Unlock()
	if nFinal != nAfter {
		t.Fatalf("终态 DB 回读不应回插 map：after=%d final=%d", nAfter, nFinal)
	}
	// 保留窗口：40 天前行清理
	old := time.Now().AddDate(0, 0, -40)
	if err := st.UpsertA2ATask(&store.A2ATaskRow{ID: "ancient", ContextID: "c", AgentID: agent.ID, State: "completed", UpdatedAt: old.UTC().Format(time.RFC3339Nano), CreatedAt: old.UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	n, err := st.CleanupOldA2ATasks(time.Now().AddDate(0, 0, -30))
	if err != nil || n < 1 {
		t.Fatalf("保留窗口清理应删 40 天前行：n=%d err=%v", n, err)
	}
	if row, _ := st.GetA2ATask("ancient"); row != nil {
		t.Fatal("ancient 行应被清理")
	}
}
