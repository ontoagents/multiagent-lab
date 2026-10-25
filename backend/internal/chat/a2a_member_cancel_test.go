package chat

// REQ-441/M237：委派超时/打断弃单时 best-effort 取消远端任务（client 面此前从不调
// tasks/cancel——远端白跑至自身超时；协议方法双方均已实现，本测锁定接线路径）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestA2ADelegateTimeoutCancelsRemote(t *testing.T) {
	var mu sync.Mutex
	methods := map[string]int{}
	taskSnap := map[string]any{"id": "t-timeout", "contextId": "c1", "kind": "task",
		"status": map[string]any{"state": "working"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		methods[req.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		snap := taskSnap
		if req.Method == "tasks/cancel" {
			cp := map[string]any{"id": "t-timeout", "contextId": "c1", "kind": "task",
				"status": map[string]any{"state": "canceled"}}
			snap = cp
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": snap})
	}))
	defer srv.Close()

	deps := a2aMemberDeps{Name: "远端成员", Endpoint: srv.URL, PollSec: 0.05}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if _, err := a2aDelegateRun(ctx, deps, "任务", nil); err == nil {
		t.Fatal("超时应回错误")
	}
	// defer 的 CancelTask 异步发出——轮询断言
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := methods["tasks/cancel"]
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("ctx 超时后未对远端发出 tasks/cancel（弃单白跑未治）")
}
