package store

// REQ-551/M358 工作区记忆存储层单测（Q-24 分层批④）：CRUD/同工作区内容去重/三桶状态流转/
// 终态不可再裁/行级溯源/accepted 读侧排序/删工作区 CASCADE/游标 upsert。零外部依赖。

import (
	"errors"
	"path/filepath"
	"testing"
)

func newMemoryTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMemoryCRUDAndDedup(t *testing.T) {
	st := newMemoryTestStore(t)
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w"})
	// 空 content / 空 workspace → 400
	if _, _, err := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "  "}); err == nil {
		t.Fatal("空内容应 400")
	}
	if _, _, err := st.CreateMemory(&Memory{Content: "x"}); err == nil {
		t.Fatal("空工作区应 400")
	}
	m1, created, err := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "用户在做学位论文", Status: "candidate"})
	if err != nil || !created || m1.Status != "candidate" {
		t.Fatalf("候选落库失真: %+v %v %v", m1, created, err)
	}
	if m1.SourceConversationID != "" || m1.SourceMessageID != "" {
		t.Fatalf("手工录入溯源应为空: %+v", m1)
	}
	// 同内容去重（候选∪已入跨状态）
	m2, created2, err := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "用户在做学位论文", Status: "accepted"})
	if err != nil || created2 || m2 != nil {
		t.Fatalf("同内容应去重: %+v %v %v", m2, created2, err)
	}
	list, _ := st.ListMemories(ws.ID, "")
	if len(list) != 1 {
		t.Fatalf("去重后应 1 行: %d", len(list))
	}
	// 三桶过滤
	if _, err := st.ListMemories(ws.ID, "accepted"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMemory(m1.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMemory("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删不存在应 404: %v", err)
	}
}

func TestMemoryDecideFlow(t *testing.T) {
	st := newMemoryTestStore(t)
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w"})
	m, _, _ := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "偏好简明回答", SourceConversationID: "c1", SourceMessageID: "m9"})
	// 采纳 → accepted（行级溯源保留）
	changed, err := st.DecideMemory(m.ID, true)
	if err != nil || !changed {
		t.Fatalf("采纳失真: %v %v", changed, err)
	}
	got, _ := st.GetMemory(m.ID)
	if got.Status != "accepted" || got.SourceMessageID != "m9" {
		t.Fatalf("采纳后失真: %+v", got)
	}
	// 终态不可再裁（审计链单向）
	changed2, err := st.DecideMemory(m.ID, false)
	if err != nil || changed2 {
		t.Fatalf("终态应不可再裁: %v %v", changed2, err)
	}
	// accepted 读侧（召回源）
	m2, _, _ := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "第二条"})
	_, _ = st.DecideMemory(m2.ID, true)
	acc, err := st.AcceptedMemories(ws.ID)
	if err != nil || len(acc) != 2 || acc[0].CreatedAt > acc[1].CreatedAt {
		t.Fatalf("accepted 读侧失真: %v %v", acc, err)
	}
	// 拒绝桶
	m3, _, _ := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "噪声"})
	if _, err := st.DecideMemory(m3.ID, false); err != nil {
		t.Fatal(err)
	}
	rej, _ := st.ListMemories(ws.ID, "rejected")
	if len(rej) != 1 {
		t.Fatalf("拒绝桶失真: %d", len(rej))
	}
}

// TestMemoryWorkspaceCascade 删工作区记忆随删（CASCADE——记忆无回落宿主，106 号终值口径）。
func TestMemoryWorkspaceCascade(t *testing.T) {
	st := newMemoryTestStore(t)
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w"})
	m, _, _ := st.CreateMemory(&Memory{WorkspaceID: ws.ID, Content: "x", Status: "accepted"})
	if err := st.DeleteWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetMemory(m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删工作区记忆应级联: %v", err)
	}
}

// TestMemoryCursor 游标 upsert 幂等。
func TestMemoryCursor(t *testing.T) {
	st := newMemoryTestStore(t)
	if last, err := st.GetMemoryCursor("c1"); err != nil || last != "" {
		t.Fatalf("空游标应空串: %q %v", last, err)
	}
	if err := st.AdvanceMemoryCursor("c1", "m1"); err != nil {
		t.Fatal(err)
	}
	if err := st.AdvanceMemoryCursor("c1", "m2"); err != nil {
		t.Fatal(err)
	}
	if last, err := st.GetMemoryCursor("c1"); err != nil || last != "m2" {
		t.Fatalf("游标应推进到 m2: %q %v", last, err)
	}
}
