package a2a

// REQ-416/M212 单测：A2A 委派客户端四行为——卡片真握手（成功/不可达/非卡片结构）、
// message/send+tasks/get 轮询至终态（artifacts 拼接）、JSON-RPC 错误转译、失败终态报错。
// 桩服务器与 server 端 wire 形态同构（a2a.go taskJSON）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// stubA2A 端到端桩：卡片 + message/send（立即 working）+ tasks/get（第 N 次转 completed）。
type stubA2A struct {
	srv        *httptest.Server
	polls      atomic.Int32 // tasks/get 次数
	completeAt int32        // 第几次轮询回终态
	failMode   string       // "" | "rpc-error" | "failed-state" | "http500"
	sendErr    string
}

func newStubA2A(t *testing.T, failMode string) *stubA2A {
	s := &stubA2A{completeAt: 2, failMode: failMode}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent-card.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "外部翻译官", "description": "中英互译", "url": "TBD",
			"preferredTransport": "JSONRPC",
			"skills":             []map[string]any{{"id": "s1", "name": "翻译", "description": "中英互译"}},
		})
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if s.failMode == "http500" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		if s.failMode == "rpc-error" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32001, "message": "task 不存在"}})
			return
		}
		switch req.Method {
		case "message/send":
			if s.sendErr != "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -32602, "message": s.sendErr}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"id": "task-1", "contextId": "conv-9", "kind": "task",
				"status": map[string]any{"state": "working", "timestamp": "now"},
			}})
		case "tasks/get":
			n := s.polls.Add(1)
			state := "working"
			artifact := map[string]any{}
			if n >= s.completeAt || s.failMode == "failed-state" {
				state = "completed"
				if s.failMode == "failed-state" {
					state = "failed"
				} else {
					artifact = map[string]any{"taskId": "task-1", "parts": []map[string]any{{"kind": "text", "text": "外部成员产出 A"}}}
				}
			}
			result := map[string]any{
				"id": "task-1", "contextId": "conv-9", "kind": "task",
				"status": map[string]any{"state": state, "timestamp": "now"},
			}
			if len(artifact) > 0 {
				result["artifacts"] = []map[string]any{artifact}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "方法未支持"}})
		}
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func TestFetchAgentCardHandshake(t *testing.T) {
	s := newStubA2A(t, "")
	card, raw, err := FetchAgentCard(context.Background(), s.srv.URL, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "外部翻译官" || len(card.Skills) != 1 || card.Skills[0].Name != "翻译" {
		t.Fatalf("卡片解析不符: %+v", card)
	}
	if len(raw) == 0 || !strings.Contains(string(raw), "preferredTransport") {
		t.Fatal("卡片原文应保留")
	}
	// 不可达 → 报错
	if _, _, err := FetchAgentCard(context.Background(), "http://127.0.0.1:1", nil, time.Second); err == nil {
		t.Fatal("不可达应报错")
	}
	// 非卡片结构（缺 name）→ 报错
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"foo":"bar"}`))
	}))
	defer bad.Close()
	if _, _, err := FetchAgentCard(context.Background(), bad.URL, nil, time.Second); err == nil {
		t.Fatal("缺 name 应判非卡片")
	}
}

func TestSendAndAwaitTerminal(t *testing.T) {
	s := newStubA2A(t, "")
	ctx := context.Background()
	rt, err := Send(ctx, s.srv.URL, nil, "翻译：hello")
	if err != nil {
		t.Fatal(err)
	}
	if rt.ID != "task-1" || rt.State != "working" {
		t.Fatalf("send 应回 working 任务: %+v", rt)
	}
	final, err := AwaitTerminal(ctx, s.srv.URL, nil, rt.ID, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "completed" || !strings.Contains(final.ArtifactText, "外部成员产出 A") {
		t.Fatalf("终态/artifacts 不符: %+v", final)
	}
	if s.polls.Load() < 2 {
		t.Fatal("应至少轮询两次至终态")
	}
}

func TestSendRPCErrorAndFailedState(t *testing.T) {
	// JSON-RPC 错误转译
	s := newStubA2A(t, "rpc-error")
	if _, err := Send(context.Background(), s.srv.URL, nil, "x"); err == nil || !strings.Contains(err.Error(), "-32001") {
		t.Fatalf("rpc 错误应转译: %v", err)
	}
	// 终态 failed → error 带 errorText
	s2 := newStubA2A(t, "failed-state")
	rt, err := Send(context.Background(), s2.srv.URL, nil, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AwaitTerminal(context.Background(), s2.srv.URL, nil, rt.ID, 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("failed 终态应报错: %v", err)
	}
}

// 协议回环：真 a2a.Service（stub RunFunc）×真 client——双实例狗粮的协议级验证
//（wire 形态双方零假设；message/send 异步任务化 + tasks/get 轮询 + artifacts 回传全链）。
func TestClientAgainstRealService(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ag, err := st.CreateAgent(&store.Agent{Name: "远端智能体", Description: "协议回环桩", MaxIteration: 3})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st)
	svc.BaseURL = "http://stub-base"
	svc.RunFunc = func(_ context.Context, _ *store.Conversation, _ *store.Agent, _ string, input string) (string, error) {
		return "回环结果：" + input, nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/a2a/agents/{id}/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
		svc.HandleCard(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/a2a/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		svc.HandleRPC(w, r, r.PathValue("id"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	endpoint := srv.URL + "/api/a2a/agents/" + ag.ID
	ctx := context.Background()
	card, _, err := FetchAgentCard(ctx, endpoint, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "远端智能体" {
		t.Fatalf("卡片名不符: %+v", card)
	}
	rt, err := Send(ctx, endpoint, nil, "你好")
	if err != nil {
		t.Fatal(err)
	}
	if rt.State == "completed" {
		if !strings.Contains(rt.ArtifactText, "回环结果") {
			t.Fatalf("同步终态应带 artifacts: %+v", rt)
		}
		return
	}
	final, err := AwaitTerminal(ctx, endpoint, nil, rt.ID, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "completed" || !strings.Contains(final.ArtifactText, "回环结果：你好") {
		t.Fatalf("回环终态不符: %+v err=%v", final, err)
	}
}
