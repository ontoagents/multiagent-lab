package chat

// REQ-385/M181 单测补：异步委派工具（asyncDelegateTool）——启动即回 delegate_id 不阻塞、
// 任务空回执降级、subagent.delegate 事件带 via=agent_as_tool_async 标注。

import (
	"context"
	"encoding/json"
	einotool "github.com/cloudwego/eino/components/tool"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func TestAsyncDelegateToolStartsBackground(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dm := NewDelegateManager(st)
	dm.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		return "子任务完成：" + input, nil // 桩：真实 StartRun 为 chat 全链，此处验证编排面
	}
	member, err := st.CreateAgent(&store.Agent{Name: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &member.ID, Title: "p"})
	if err != nil {
		t.Fatal(err)
	}
	tl := newAsyncDelegateTool(dm, member, parent)
	// 事件面：sink 捕获（生产由 runOnce 的 WithEventSink 承载——tool.EmitEvent 审计半边）
	var sinkEvents []string
	sinkCtx := tool.WithEventSink(context.Background(), func(typ string, data map[string]any) {
		b, _ := json.Marshal(data)
		sinkEvents = append(sinkEvents, typ+" "+string(b))
	})
	out, err := tl.(interface {
		InvokableRun(context.Context, string, ...einotool.Option) (string, error)
	}).InvokableRun(sinkCtx, `{"request":"做一件长事"}`)
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("回执应为 JSON: %v: %s", err, out)
	}
	if resp["delegate_id"] == "" || resp["status"] != "running" {
		t.Fatalf("回执应含 delegate_id+running: %s", out)
	}
	found := false
	for _, ev := range sinkEvents {
		if strings.Contains(ev, "subagent.delegate") && strings.Contains(ev, "agent_as_tool_async") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应发 subagent.delegate 事件（via=agent_as_tool_async）: %v", sinkEvents)
	}
	// 空任务降级回执
	out2, err := tl.(interface {
		InvokableRun(context.Context, string, ...einotool.Option) (string, error)
	}).InvokableRun(context.Background(), `{"request":"  "}`)
	if err != nil || !strings.Contains(out2, "bad_request") {
		t.Fatalf("空任务应降级回执: %v %s", err, out2)
	}
}
