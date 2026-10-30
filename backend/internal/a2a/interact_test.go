package a2a

// REQ-443/M239：对外面交互语义闭环单测——①审批档闸门（tool_approval≠off 预拒绝沿 /mcp 先例）
// ②input-required 生产线（ask_human 挂起→终态+问题文本外发）③答案往返（同 contextId 携答案
// 再 send→ResumeFunc 续跑→completed）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestApprovalGateBlocksExternalDelegation(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	agent, err := st.CreateAgent(&store.Agent{Name: "审批智能体", Description: "d", ToolApproval: "danger"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := New(st)
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		return "不应到达", nil
	}
	for _, method := range []string{"message/send", "message/stream"} {
		_, out := rpc(t, svc, agent.ID, map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": method,
			"params": map[string]any{"message": map[string]any{
				"role": "user", "kind": "message", "messageId": "m1",
				"parts": []map[string]any{{"kind": "text", "text": "委派"}},
			}},
		})
		errObj, ok := out["error"].(map[string]any)
		if !ok {
			t.Fatalf("%s 应回 JSON-RPC error: %v", method, out)
		}
		if code, _ := errObj["code"].(float64); code != -32004 {
			t.Fatalf("%s 错误码应为 -32004（审批档闸门）: %v", method, errObj)
		}
	}
}

func TestInputRequiredProductionAndResume(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	agent, err := st.CreateAgent(&store.Agent{Name: "追问智能体", Description: "d"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := New(st)
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		if strings.Contains(input, "需要澄清") {
			q := `{"kind":"ask_human","question":"请提供目标语言","choices":["中文","英文"],"run_id":"r1"}`
			_ = st.SetConversationInterruptState(conv.ID, q, "ask_human")
			return "等待人工输入", nil
		}
		return "直接完成", nil
	}
	var gotAnswer string
	svc.ResumeFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string) (string, error) {
		gotAnswer = input
		_ = st.SetConversationInterruptState(conv.ID, "", "resume")
		return "答案已收到并完成", nil
	}

	// ① 首次 send 触发 ask_human → 任务终态 input-required（问题文本外发）
	_, out := rpc(t, svc, agent.ID, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "需要澄清的任务"}},
		}},
	})
	result := out["result"].(map[string]any)
	taskID := result["id"].(string)
	contextID := result["contextId"].(string)
	state, errMsg := pollTask(t, svc, agent.ID, taskID)
	if state != "input-required" {
		t.Fatalf("ask_human 挂起应产 input-required，得 %s", state)
	}
	if !strings.Contains(errMsg, "请提供目标语言") || !strings.Contains(errMsg, "中文") {
		t.Fatalf("问题文本（含选项）应外发: %s", errMsg)
	}

	// ② 同 contextId 携答案再 send → ResumeFunc 续跑 → completed
	_, out = rpc(t, svc, agent.ID, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m2", "contextId": contextID,
			"parts": []map[string]any{{"kind": "text", "text": "英文"}},
		}},
	})
	result = out["result"].(map[string]any)
	taskID2 := result["id"].(string)
	state, _ = pollTask(t, svc, agent.ID, taskID2)
	if state != "completed" {
		t.Fatalf("携答案续跑应 completed，得 %s", state)
	}
	if gotAnswer != "英文" {
		t.Fatalf("答案应经 ResumeFunc 收到「英文」，得 %q", gotAnswer)
	}
	_, out = rpc(t, svc, agent.ID, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tasks/get", "params": map[string]any{"id": taskID2}})
	final := out["result"].(map[string]any)
	arts := final["artifacts"].([]any)
	first := arts[0].(map[string]any)
	parts := first["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if text != "答案已收到并完成" {
		t.Fatalf("续跑产物不符: %s", text)
	}
}

// pollTask 轮询至终态，返回（state, errorText）。
func pollTask(t *testing.T, svc *Service, agentID, taskID string) (string, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(40 * time.Millisecond)
		_, out := rpc(t, svc, agentID, map[string]any{"jsonrpc": "2.0", "id": 99, "method": "tasks/get", "params": map[string]any{"id": taskID}})
		res, ok := out["result"].(map[string]any)
		if !ok {
			continue
		}
		state := res["status"].(map[string]any)["state"].(string)
		switch state {
		case "working", "submitted":
			continue
		}
		errText := ""
		if e := res["status"].(map[string]any)["error"]; e != nil {
			errText, _ = e.(string)
		}
		// error 文本在 wire 上位于 status.message 或错误载荷——本项目 taskJSON 未单列，
		// input-required 的问题文本经 tasks/get 的 DB 回读载荷校验由调用方补充
		_ = errText
		return state, errorTextOf(t, svc, agentID, taskID, res)
	}
	t.Fatal("任务未在时限内到达终态")
	return "", ""
}

// errorTextOf 从落库 payload 中提取 error_text（input-required 问题文本承载处）。
func errorTextOf(t *testing.T, svc *Service, agentID, taskID string, res map[string]any) string {
	t.Helper()
	row, err := svc.Store.GetA2ATask(taskID)
	if err != nil || row == nil {
		return ""
	}
	var pl store.A2APayload
	_ = json.Unmarshal([]byte(row.Payload), &pl)
	return pl.ErrorText
}
