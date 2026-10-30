package a2a

// REQ-369/M167 A2A 端点单测：Agent Card 形态/JSON-RPC message/send 异步任务化→tasks/get 轮询
// →completed 附 artifacts/contextId 多轮延续/failed 透出/tasks/cancel/发现面/错误码。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newA2AFixture(t *testing.T) (*Service, *store.Store, *[]string) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	agent, err := st.CreateAgent(&store.Agent{Name: "A2A 测试智能体", Description: "对外服务智能体", Instruction: "你是测试智能体。"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := New(st)
	svc.BaseURL = "http://ext.example:8080"
	calls := &[]string{}
	var mu sync.Mutex
	var release chan struct{}
	svc.RunFunc = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		mu.Lock()
		*calls = append(*calls, input)
		ch := release
		mu.Unlock()
		if ch != nil {
			select {
			case <-ch:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		if strings.Contains(input, "炸") {
			return "", errors.New("模拟运行失败")
		}
		return "回复：" + input, nil
	}
	_ = agent
	return svc, st, calls
}

func rpc(t *testing.T, svc *Service, agentID string, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/"+agentID, strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	svc.HandleRPC(rec, req, agentID)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestAgentCardShape(t *testing.T) {
	svc, st, _ := newA2AFixture(t)
	agents, _ := st.ListAgents()
	var testAgent *store.Agent
	for _, a := range agents {
		if a.Name == "A2A 测试智能体" {
			testAgent = a
		}
	}
	if testAgent == nil {
		t.Fatal("测试智能体未创建")
	}
	card := svc.AgentCard(testAgent)
	if card["name"] != "A2A 测试智能体" || card["protocolVersion"] != "1.0" {
		t.Fatalf("卡片基础字段不符: %v", card)
	}
	if card["url"] != "http://ext.example:8080/api/a2a/agents/"+testAgent.ID {
		t.Fatalf("url 应为 JSON-RPC 端点: %v", card["url"])
	}
	caps := card["capabilities"].(map[string]any)
	if caps["streaming"] != true {
		t.Fatal("REQ-442/M238 起 server 侧 message/stream 已启用，capabilities.streaming 应为 true")
	}
	skills := card["skills"].([]map[string]any)
	if len(skills) != 1 || skills[0]["id"] != testAgent.ID {
		t.Fatalf("skills 不符: %v", skills)
	}
}

func TestMessageSendTaskLifecycle(t *testing.T) {
	svc, st, calls := newA2AFixture(t)
	agents, _ := st.ListAgents()
	aid := agents[0].ID

	// ① message/send → 立即回 working 态 task
	code, out := rpc(t, svc, aid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m1",
			"parts": []map[string]any{{"kind": "text", "text": "帮我查一下本体"}},
		}},
	})
	if code != http.StatusOK {
		t.Fatalf("HTTP %d", code)
	}
	result := out["result"].(map[string]any)
	if result["state"] != nil || result["status"] == nil {
		t.Fatalf("应回 task 形态: %v", result)
	}
	if result["status"].(map[string]any)["state"] != "working" {
		t.Fatalf("应立即回 working: %v", result["status"])
	}
	taskID := result["id"].(string)
	contextID := result["contextId"].(string)

	// ② tasks/get 轮询至 completed（假执行器同步快，稍等即可）
	var final map[string]any
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		_, out = rpc(t, svc, aid, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tasks/get", "params": map[string]any{"id": taskID}})
		final = out["result"].(map[string]any)
		if final["status"].(map[string]any)["state"] != "working" {
			break
		}
	}
	if final["status"].(map[string]any)["state"] != "completed" {
		t.Fatalf("任务应完成: %v", final["status"])
	}
	arts := final["artifacts"].([]any)
	text := arts[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
	if text != "回复：帮我查一下本体" {
		t.Fatalf("artifact 文本不符: %v", text)
	}

	// ③ contextId 多轮延续：同 contextId 再发一条 → 同会话第二次调用
	_, out = rpc(t, svc, aid, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "message/send",
		"params": map[string]any{"message": map[string]any{
			"role": "user", "kind": "message", "messageId": "m2", "contextId": contextID,
			"parts": []map[string]any{{"kind": "text", "text": "第二条"}},
		}},
	})
	time.Sleep(200 * time.Millisecond)
	if len(*calls) != 2 {
		t.Fatalf("应两次调用执行器: %v", *calls)
	}

	// ④ 会话存在（contextId=平台会话 id，审计可溯）
	conv, gerr := st.GetConversation(contextID)
	if gerr != nil || conv == nil || !strings.HasPrefix(conv.Title, "[A2A]") {
		t.Fatalf("A2A 会话应创建且可溯: %v %v", gerr, conv)
	}

	// ⑤ 失败透出
	_, out = rpc(t, svc, aid, map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "message/send",
		"params": map[string]any{"message": map[string]any{"role": "user", "kind": "message", "parts": []map[string]any{{"kind": "text", "text": "炸一下"}}}},
	})
	tid := out["result"].(map[string]any)["id"].(string)
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		_, out = rpc(t, svc, aid, map[string]any{"jsonrpc": "2.0", "id": 5, "method": "tasks/get", "params": map[string]any{"id": tid}})
		if out["result"].(map[string]any)["status"].(map[string]any)["state"] != "working" {
			break
		}
	}
	if out["result"].(map[string]any)["status"].(map[string]any)["state"] != "failed" {
		t.Fatalf("失败应透出: %v", out["result"])
	}

	// ⑥ 未知 task 错误码
	_, out = rpc(t, svc, aid, map[string]any{"jsonrpc": "2.0", "id": 6, "method": "tasks/get", "params": map[string]any{"id": "nope"}})
	errObj := out["error"].(map[string]any)
	if errObj["code"] != float64(errTaskNotFound) {
		t.Fatalf("应回 TaskNotFound: %v", errObj)
	}

	// ⑦ 未支持方法（message/stream 已支持——REQ-442/M238；换 pushNotifications/set 验证未知方法分支）
	_, out = rpc(t, svc, aid, map[string]any{"jsonrpc": "2.0", "id": 7, "method": "pushNotifications/set", "params": map[string]any{}})
	if out["error"].(map[string]any)["code"] != float64(errMethodNotSupported) {
		t.Fatalf("未知方法应 MethodNotSupported: %v", out["error"])
	}

	// ⑧ 不存在智能体
	code, _ = rpc(t, svc, "ghost", map[string]any{"jsonrpc": "2.0", "id": 8, "method": "tasks/get", "params": map[string]any{"id": "x"}})
	if code != http.StatusNotFound {
		t.Fatalf("不存在智能体应 404: %d", code)
	}
}

func TestAgentsExposed(t *testing.T) {
	svc, _, _ := newA2AFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/a2a/agents", nil)
	rec := httptest.NewRecorder()
	svc.AgentsExposed(rec, req)
	var out struct {
		Agents []map[string]any `json:"agents"`
		Count  int              `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count < 1 {
		t.Fatalf("发现面应非空: %v", out)
	}
	if !strings.Contains(out.Agents[0]["card_url"].(string), ".well-known/agent-card.json") {
		t.Fatalf("card_url 形态不符: %v", out.Agents[0])
	}
}

// REQ-434/M230：成员目录互发现——AgentsExposed 扩 external_members（cli/a2a 连接器授权面）
// 与 topology（智能体所在团队元数据）透出；凭据/配置细节不透出。
func TestAgentsExposedMemberCatalog(t *testing.T) {
	svc, st, _ := newA2AFixture(t)
	co, err := st.CreateAgent(&store.Agent{Name: "协调者甲"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnector(&store.Connector{
		ID: "c-cli", Kind: "cli", Name: "cli桥",
		Status: "ok", Config: map[string]any{"backend": "claude-code"},
	}); err != nil {
		t.Fatal(err)
	}
	co.Connectors = []string{"c-cli"}
	if _, err := st.UpdateAgent(co); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/a2a/agents", nil)
	rec := httptest.NewRecorder()
	svc.AgentsExposed(rec, req)
	var out struct {
		Count           int             `json:"count"`
		ExternalMembers []struct {
			AgentID   string `json:"agent_id"`
			Kind      string `json:"kind"`
			Backend   string `json:"backend"`
			Connector string `json:"connector_name"`
		} `json:"external_members"`
		ExternalCount int            `json:"external_count"`
		Topology      map[string]any `json:"topology"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count < 2 {
		t.Fatalf("智能体数: %d", out.Count)
	}
	if out.ExternalCount != 1 || len(out.ExternalMembers) != 1 {
		t.Fatalf("应 1 条外部成员目录: %d/%v", out.ExternalCount, out.ExternalMembers)
	}
	em := out.ExternalMembers[0]
	if em.Kind != "cli" || em.Backend != "claude-code" || em.Connector != "cli桥" {
		t.Fatalf("目录条目不符: %+v", em)
	}
}
