package chat

import (
	"sync"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeInner struct{ calls int }

func (f *fakeInner) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	f.calls++
	return schema.AssistantMessage("ok", nil), nil
}
func (f *fakeInner) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	f.calls++
	msg := schema.AssistantMessage("stream-ok", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 210, CompletionTokens: 12, TotalTokens: 222}, FinishReason: "stop"}
	sr, sw := schema.Pipe[*schema.Message](2)
	_ = sw.Send(msg, nil)
	sw.Close()
	return sr, nil
}

func TestDebugModelEmitsStep(t *testing.T) {
	// REQ-522/M328（P28）：旁路 goroutine 上报与测试读取此前仅靠 sleep 时序——DATA RACE。
	// mutex 收集+deadline 轮询替代固定 sleep（并发安全且不再赌 200ms）。
	var mu sync.Mutex
	var got []map[string]any
	rec := &debugRecorder{level: DebugFull, runID: "r1", emit: func(ev *Event) {
		var m map[string]any
		_ = json.Unmarshal(ev.Data, &m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}}
	inner := &fakeInner{}
	m := wrapDebug(inner, "测试Agent", rec)
	if m == inner {
		t.Fatal("expected wrapped model")
	}
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("你好")}, model.WithTools([]*schema.ToolInfo{{Name: "current_time", Desc: "查时间"}}))
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, e := sr.Recv(); e != nil {
			break
		}
	}
	// REQ-522/M328（P28）：deadline 轮询替代固定 200ms sleep（旁路 goroutine 上报时序不赌）
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("model.step events = 0, want 1（2s 轮询超时）")
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("model.step events = %d, want 1", len(got))
	}
	d := got[0]
	if d["agent"] != "测试Agent" || d["finish_reason"] != "stop" {
		t.Fatalf("unexpected step data: %v", d)
	}
	if u, ok := d["usage"].(map[string]any); !ok || u["total_tokens"] != float64(222) {
		t.Fatalf("usage missing: %v", d["usage"])
	}
	raw, _ := json.Marshal(d)
	joined := string(raw)
	for _, want := range []string{`"current_time"`, `"查时间"`, `"finish_reason":"stop"`, `"total_tokens":222`, `"role":"user"`, `"content":"你好"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("step data missing %s in: %s", want, joined)
		}
	}
}
