package a2a

// REQ-442/M238：server 侧 message/stream SSE 流式应答单测——快照形态（初始 working/过程增量
// 〔节流〕/终态 completed）+ 平台自吃回环（真 server × 真 client SendStream）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newStreamFixture(t *testing.T) (*Service, *store.Agent) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	agent, err := st.CreateAgent(&store.Agent{Name: "流式测试", Description: "d"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := New(st)
	svc.RunStreamFunc = func(ctx context.Context, conv *store.Conversation, a *store.Agent, runID, input string, onProgress func(string)) (string, error) {
		onProgress("部分答")
		time.Sleep(600 * time.Millisecond) // 跨过 500ms 节流窗
		onProgress("部分答案已完成")              // 累积部分文本（装配层语义）
		return "最终答复", nil
	}
	return svc, agent
}

func TestMessageStreamServerShape(t *testing.T) {
	svc, agent := newStreamFixture(t)
	body := `{"jsonrpc":"2.0","id":9,"method":"message/stream","params":{"message":{"role":"user","kind":"message","messageId":"m1","parts":[{"kind":"text","text":"你好"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/"+agent.ID, strings.NewReader(body))
	rec := httptest.NewRecorder()
	svc.HandleRPC(rec, req, agent.ID)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("应回 SSE Content-Type: %s", ct)
	}
	events := parseSSEEvents(t, rec.Body.String())
	if len(events) < 3 {
		t.Fatalf("应含初始/过程/终态 ≥3 条快照，得 %d: %v", len(events), events)
	}
	first := events[0]
	if first["status"].(map[string]any)["state"] != "working" {
		t.Fatalf("首条应为 working: %v", first)
	}
	if _, has := first["artifacts"]; has {
		t.Fatal("首条 working 快照不应带 artifacts")
	}
	var sawProgress bool
	for _, ev := range events[1 : len(events)-1] {
		if ev["status"].(map[string]any)["state"] == "working" {
			if art := artifactText(ev); art != "" {
				sawProgress = true
			}
		}
	}
	if !sawProgress {
		t.Fatal("过程快照应含累积部分文本（onProgress 透出）")
	}
	last := events[len(events)-1]
	if last["status"].(map[string]any)["state"] != "completed" {
		t.Fatalf("末条应为 completed: %v", last)
	}
	if artifactText(last) != "最终答复" {
		t.Fatalf("终态 artifacts 应为最终答复: %v", last)
	}
	// 落库一致
	taskID := last["id"].(string)
	if row, _ := svc.Store.GetA2ATask(taskID); row == nil || row.State != "completed" {
		t.Fatalf("终态应写穿落库: %+v", row)
	}
}

// TestSendStreamAgainstRealService 平台自吃回环：真 server（message/stream SSE）× 真 client
// （SendStream 订阅）——REQ-442 与 REQ-432 两半边的 wire 对偶验证。
func TestSendStreamAgainstRealService(t *testing.T) {
	svc, agent := newStreamFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc.HandleRPC(w, r, agent.ID)
	}))
	defer srv.Close()

	snapshots := 0
	rt, err := SendStream(context.Background(), srv.URL, nil, "你好", func(u *RemoteTask) {
		snapshots++
	})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if rt.State != "completed" || rt.ArtifactText != "最终答复" {
		t.Fatalf("终态不符: %+v", rt)
	}
	if snapshots < 2 {
		t.Fatalf("过程快照应 ≥2（初始+过程/终态），得 %d", snapshots)
	}
}

func parseSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var env struct {
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal([]byte(payload), &env); err != nil || env.Result == nil {
			continue
		}
		events = append(events, env.Result)
	}
	return events
}

func artifactText(task map[string]any) string {
	arts, ok := task["artifacts"].([]any)
	if !ok || len(arts) == 0 {
		return ""
	}
	first, _ := arts[0].(map[string]any)
	parts, _ := first["parts"].([]any)
	for _, p := range parts {
		pm, _ := p.(map[string]any)
		if pm["kind"] == "text" {
			s, _ := pm["text"].(string)
			return s
		}
	}
	return ""
}
