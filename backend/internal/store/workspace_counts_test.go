package store

// REQ-613-b6cc/M421-b6cc 计数字段单测（121 号 §4.5-① 后端必做小件）：workspace 行
// project_count/conv_count、project 行 conv_count——workspace conv_count 口径=直挂会话 ∪
// 归属项目会话（LEFT JOIN 单条出数）；空壳工作区 0/0（前端徽标/总览唯一闭合路径）。
// 零外部依赖。

import (
	"path/filepath"
	"testing"
)

func newWorkspaceCountsTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestWorkspaceAndProjectCounts(t *testing.T) {
	st := newWorkspaceCountsTestStore(t)
	wsA, err := st.CreateWorkspace(&Workspace{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	wsB, err := st.CreateWorkspace(&Workspace{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	// 归属项目 PA→A；未归属 PB（不入 A 计数）
	pa, err := st.CreateProject(&Project{Name: "PA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetProjectWorkspace(pa.ID, wsA.ID); err != nil {
		t.Fatal(err)
	}
	pb, err := st.CreateProject(&Project{Name: "PB"})
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(&Agent{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	// 直挂会话×2（A）+ 归属项目会话×1（PA）+ 未归属项目会话×1（PB）
	for range 2 {
		if _, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &ag.ID, WorkspaceID: &wsA.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &pa.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &pb.ID}); err != nil {
		t.Fatal(err)
	}

	wss, err := st.ListWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	var a, b *Workspace
	for _, w := range wss {
		switch w.ID {
		case wsA.ID:
			a = w
		case wsB.ID:
			b = w
		}
	}
	if a == nil || b == nil {
		t.Fatalf("列表缺行: %+v", wss)
	}
	// A：直挂 2 + 归属项目会话 1 = 3；project_count=1（PB 未归属不入）
	if a.ProjectCount != 1 || a.ConvCount != 3 {
		t.Fatalf("A 计数失真: project_count=%d conv_count=%d（期 1/3）", a.ProjectCount, a.ConvCount)
	}
	// B 空壳：0/0（徽标与 chevron 诚实化数据源）
	if b.ProjectCount != 0 || b.ConvCount != 0 {
		t.Fatalf("B 空壳计数应 0/0: %d/%d", b.ProjectCount, b.ConvCount)
	}

	// 项目行 conv_count：PA=1、PB=1（项目会话按 project_id 直数，与归属无关）
	ps, err := st.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	cm := map[string]int{}
	for _, p := range ps {
		cm[p.ID] = p.ConvCount
	}
	if cm[pa.ID] != 1 || cm[pb.ID] != 1 {
		t.Fatalf("项目 conv_count 失真: PA=%d PB=%d（期 1/1）", cm[pa.ID], cm[pb.ID])
	}
	// 过滤列表与单读同权（GET /api/projects?workspace_id= 与 GetProject 路径）
	byWs, err := st.ListProjectsByWorkspace(wsA.ID)
	if err != nil || len(byWs) != 1 || byWs[0].ConvCount != 1 {
		t.Fatalf("过滤列表 conv_count 失真: %v %v", byWs, err)
	}
	gp, err := st.GetProject(pa.ID)
	if err != nil || gp.ConvCount != 1 {
		t.Fatalf("单读 conv_count 失真: %v %v", gp, err)
	}

	// 删直挂会话 → workspace conv_count 回落（2 直挂剩 1）
	convs, err := st.ListConversations(ConversationFilter{WorkspaceID: wsA.ID})
	if err != nil || len(convs) != 2 {
		t.Fatalf("直挂清单失真: %d %v", len(convs), err)
	}
	if err := st.DeleteConversation(convs[0].ID); err != nil {
		t.Fatal(err)
	}
	wss2, err := st.ListWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wss2 {
		if w.ID == wsA.ID && w.ConvCount != 2 {
			t.Fatalf("删直挂后 A conv_count 应回落 2: %d", w.ConvCount)
		}
	}
}
