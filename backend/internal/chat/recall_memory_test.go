package chat

// REQ-551/M358 工作区记忆召回源单测（沿 recallKB 召回夹具口径）：
// 有归属+已入记忆→注入+retrieval 事件；无归属→短路零注入；项目归属转发路径；空记忆零注入。

import (
	"context"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestRecallMemoryInjects(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/mem.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ws, _ := st.CreateWorkspace(&store.Workspace{Name: "w"})
	_, _, _ = st.CreateMemory(&store.Memory{WorkspaceID: ws.ID, Content: "用户在做学位论文", Status: "accepted"})
	_, _, _ = st.CreateMemory(&store.Memory{WorkspaceID: ws.ID, Content: "未确认候选不应注入", Status: "candidate"})
	svc := NewService(st, nil, nil)
	wsid := ws.ID
	conv := &store.Conversation{ID: "c1", Scope: "agent", WorkspaceID: &wsid}
	var events []*Event
	msgs := svc.recallMemory(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) { events = append(events, ev) })
	if len(msgs) != 1 {
		t.Fatalf("应注入 1 条 system，got %d", len(msgs))
	}
	content := msgs[0].Content
	if !contains(content, "用户在做学位论文") || contains(content, "未确认候选不应注入") {
		t.Fatalf("注入内容失真: %q", content)
	}
	if len(events) != 1 || events[0].Type != "retrieval" {
		t.Fatalf("应发 retrieval 事件: %+v", events)
	}
}

func TestRecallMemoryNoWorkspaceShortCircuit(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/mem.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := NewService(st, nil, nil)
	conv := &store.Conversation{ID: "c1", Scope: "agent"}
	var events []*Event
	msgs := svc.recallMemory(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) { events = append(events, ev) })
	if len(msgs) != 0 || len(events) != 0 {
		t.Fatalf("无工作区应短路: msgs=%d events=%d", len(msgs), len(events))
	}
}

func TestRecallMemoryViaProjectInheritance(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/mem.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ws, _ := st.CreateWorkspace(&store.Workspace{Name: "w"})
	_, _, _ = st.CreateMemory(&store.Memory{WorkspaceID: ws.ID, Content: "项目级记忆", Status: "accepted"})
	p, _ := st.CreateProject(&store.Project{Name: "p"})
	_, _ = st.SetProjectWorkspace(p.ID, ws.ID)
	svc := NewService(st, nil, nil)
	pid := p.ID
	conv := &store.Conversation{ID: "c2", Scope: "project", ProjectID: &pid}
	msgs := svc.recallMemory(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) {})
	if len(msgs) != 1 || !contains(msgs[0].Content, "项目级记忆") {
		t.Fatalf("项目归属转发应命中: %v", msgs)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
