package tool

// REQ-385/M181 C2 单测补：todo_write patch 增量合并模式——只传变更项、未涉及项保原状、
// 合并后上限校验；full 模式存量语义零变化。

import (
	"context"
	einotool "github.com/cloudwego/eino/components/tool"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestTodoWritePatchMode(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "todoer"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	tl, err := NewTodoWriteTool(TodoDeps{Store: st, ConversationID: conv.ID})
	if err != nil {
		t.Fatal(err)
	}
	// full 建清单三项
	if _, err := tl.(interface {
		InvokableRun(context.Context, string, ...einotool.Option) (string, error)
	}).InvokableRun(context.Background(), `{"todos":[{"title":"设计","status":"done"},{"title":"实现","status":"in_progress"},{"title":"测试","status":"pending"}]}`); err != nil {
		t.Fatal(err)
	}
	// patch 只传一项变更（实现→done）+新项
	out, err := tl.(interface {
		InvokableRun(context.Context, string, ...einotool.Option) (string, error)
	}).InvokableRun(context.Background(), `{"mode":"patch","todos":[{"title":"实现","status":"done"},{"title":"评审","status":"pending"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"count":4`) {
		t.Fatalf("patch 合并应 3+1=4 项: %s", out)
	}
	if !strings.Contains(out, "[x] 实现") || !strings.Contains(out, "[ ] 测试") || !strings.Contains(out, "[x] 设计") || !strings.Contains(out, "[ ] 评审") {
		t.Fatalf("patch 应保持未涉及项原状并应用变更: %s", out)
	}
	// 标题超上限拒绝
	long := strings.Repeat("长", 201)
	out2, _ := tl.(interface {
		InvokableRun(context.Context, string, ...einotool.Option) (string, error)
	}).InvokableRun(context.Background(), `{"mode":"patch","todos":[{"title":"`+long+`","status":"pending"}]}`)
	if !strings.Contains(out2, "超上限") {
		t.Fatalf("超长标题应有界拒绝: %s", out2)
	}
}
