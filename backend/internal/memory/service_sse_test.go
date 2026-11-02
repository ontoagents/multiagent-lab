package memory

// REQ-585/M400 单测：emit 双写通道——ExtractNew 事件落库+live SSE 回调双达；nil emit 仅入库。

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func sseTestService(t *testing.T) (*Service, *store.Store, *store.Conversation, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := st.CreateAgent(&store.Agent{Name: "sse585", Instruction: "i"})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := st.CreateWorkspace(&store.Workspace{Name: "sse585-ws"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &agent.ID, Title: "sse585", WorkspaceID: &ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "请记住：项目代号启明"}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, box)
	return svc, st, conv, agent.ID
}

func TestExtractNewDualWrite(t *testing.T) {
	svc, st, conv, agentID := sseTestService(t)
	orig := generateStructured
	t.Cleanup(func() { generateStructured = orig })
	generateStructured = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, prompt, schema string, temperature *float64) (*chat.GenerateStructuredResult, error) {
		return &chat.GenerateStructuredResult{DraftJSON: []byte(`{"memories":[{"content":"用户项目代号启明"}]}`)}, nil
	}
	var mu sync.Mutex
	var sseEvents []string
	emit := func(eventType string, data map[string]any) {
		mu.Lock()
		sseEvents = append(sseEvents, eventType)
		mu.Unlock()
	}
	n, err := svc.ExtractNew(context.Background(), conv.ID, agentID, "run_sse", emit)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("候选应新增 1: %d", n)
	}
	mu.Lock()
	hasExtract, hasCand := false, false
	for _, e := range sseEvents {
		if strings.HasSuffix(e, "memory.extract") || e == "memory.extract" {
			hasExtract = true
		}
		if e == "memory.candidates" {
			hasCand = true
		}
	}
	mu.Unlock()
	if !hasExtract || !hasCand {
		t.Fatalf("live SSE 回调应双达 extract+candidates: %v", sseEvents)
	}
	evs, err := st.ListEvents(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	nMem := 0
	for _, e := range evs {
		if strings.Contains(e.Type, "memory.extract") || e.Type == "memory.candidates" {
			nMem++
		}
	}
	if nMem < 2 {
		t.Fatalf("入库应含 extract+candidates: %d", nMem)
	}
	_ = chat.EventMemoryPrefix
}

func TestExtractNewNilEmitStoreOnly(t *testing.T) {
	svc, st, conv, agentID := sseTestService(t)
	orig := generateStructured
	t.Cleanup(func() { generateStructured = orig })
	generateStructured = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, prompt, schema string, temperature *float64) (*chat.GenerateStructuredResult, error) {
		return &chat.GenerateStructuredResult{DraftJSON: []byte(`{"memories":[{"content":"用户偏好周五提交"}]}`)}, nil
	}
	if _, err := svc.ExtractNew(context.Background(), conv.ID, agentID, "run_nil", nil); err != nil {
		t.Fatal(err)
	}
	evs, err := st.ListEvents(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	nMem := 0
	for _, e := range evs {
		if strings.Contains(e.Type, "memory") {
			nMem++
		}
	}
	if nMem < 2 {
		t.Fatalf("nil emit 应仅入库仍可见: %d", nMem)
	}
}
