package store

// REQ-550/M357 项目归属与继承 M2 存储层单测（Q-24 分层批③；详设 106 号 §五 M2）：
// 归属/解除/不存在 400/未归属存量不变/项目层继承转发/显式参数优先/批量应用幂等与
// 显式偏差保留/删工作区 SET NULL/按工作区过滤列表。零外部依赖。

import (
	"path/filepath"
	"testing"
)

func newProjectWorkspaceTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestProjectWorkspaceAssign 归属/解除往返 + 不存在工作区 400 + 解除置 NULL（FK 兼容）。
func TestProjectWorkspaceAssign(t *testing.T) {
	st := newProjectWorkspaceTestStore(t)
	p, err := st.CreateProject(&Project{Name: "研发项目"})
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkspaceID != "" {
		t.Fatalf("新建项目应未归属: %q", p.WorkspaceID)
	}
	ws, err := st.CreateWorkspace(&Workspace{Name: "研发工作区"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.SetProjectWorkspace(p.ID, ws.ID)
	if err != nil || got.WorkspaceID != ws.ID {
		t.Fatalf("归属失真: %+v %v", got, err)
	}
	// 不存在工作区 → 400
	if _, err := st.SetProjectWorkspace(p.ID, "nope"); err == nil {
		t.Fatal("归属不存在工作区应 400")
	} else if he, ok := err.(*HTTPError); !ok || he.Status != 400 {
		t.Fatalf("应为人话 400: %v", err)
	}
	// 解除 → NULL（存回读出为空串；'' 会违反 FK，必须走 NULL）
	got2, err := st.SetProjectWorkspace(p.ID, "")
	if err != nil || got2.WorkspaceID != "" {
		t.Fatalf("解除失真: %+v %v", got2, err)
	}
	// 解除后再归属（循环验证 UPDATE NULL 列无残留）
	if _, err := st.SetProjectWorkspace(p.ID, ws.ID); err != nil {
		t.Fatal(err)
	}
	// 删工作区 → project.workspace_id SET NULL
	if err := st.DeleteWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	p2, err := st.GetProject(p.ID)
	if err != nil || p2.WorkspaceID != "" {
		t.Fatalf("删工作区项目应脱钩: %+v %v", p2, err)
	}
}

// TestProjectInheritanceViaProject 归属项目建会话继承工作区默认（写时复制）；未归属项目
// 建会话零值（存量逐字节不变）；显式参数优先；agent 直挂路径不受项目层影响。
func TestProjectInheritanceViaProject(t *testing.T) {
	st := newProjectWorkspaceTestStore(t)
	kb, err := st.CreateKnowledgeBase(&KnowledgeBase{Name: "研发库"})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := st.CreateWorkspace(&Workspace{Name: "w", DefaultKBID: kb.ID, DefaultProfileID: "prof-1"})
	if err != nil {
		t.Fatal(err)
	}
	assigned, _ := st.CreateProject(&Project{Name: "归属项目"})
	_, _ = st.SetProjectWorkspace(assigned.ID, ws.ID)
	unassigned, _ := st.CreateProject(&Project{Name: "未归属项目"})

	// 归属项目建会话 → 继承
	c1, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &assigned.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c1.KBID == nil || *c1.KBID != kb.ID || !c1.EnableKB {
		t.Fatalf("归属项目会话应继承默认库: %+v", c1.KBID)
	}
	if c1.RuntimeProfileID == nil || *c1.RuntimeProfileID != "prof-1" || !c1.OntologyEnabled {
		t.Fatalf("归属项目会话应继承默认方案: %+v", c1.RuntimeProfileID)
	}
	// 未归属项目 → 零值（存量不变）
	c2, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &unassigned.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c2.KBID != nil || c2.EnableKB || c2.RuntimeProfileID != nil || c2.OntologyEnabled {
		t.Fatalf("未归属项目会话应零值: kb=%v prof=%v", c2.KBID, c2.RuntimeProfileID)
	}
	// 显式参数优先（前端兼容位——显式传参压过工作区默认）
	other := "other-kb"
	c3, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &assigned.ID, KBID: &other})
	if err != nil {
		t.Fatal(err)
	}
	if c3.KBID == nil || *c3.KBID != other {
		t.Fatalf("显式参数应优先: %v", c3.KBID)
	}
	// 工作区直挂路径不受影响（回归 M1a）
	ag, err := st.CreateAgent(&Agent{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	c4, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &ag.ID, WorkspaceID: &ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c4.KBID == nil || *c4.KBID != kb.ID || c4.WorkspaceID == nil || *c4.WorkspaceID != ws.ID {
		t.Fatalf("直挂路径回归失败: %+v", c4.KBID)
	}
}

// TestApplyWorkspaceDefaults 批量应用：补齐空行/显式偏差不覆写/重复执行幂等/未归属 400。
func TestApplyWorkspaceDefaults(t *testing.T) {
	st := newProjectWorkspaceTestStore(t)
	kb1, _ := st.CreateKnowledgeBase(&KnowledgeBase{Name: "库一"})
	kb2, _ := st.CreateKnowledgeBase(&KnowledgeBase{Name: "库二"})
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w", DefaultKBID: kb1.ID, DefaultProfileID: "prof-1"})
	p, _ := st.CreateProject(&Project{Name: "p"})
	// 会话先建于归属前（存量态：空行/显式绑定当前值/显式偏差）——归属后再应用
	empty, _ := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &p.ID})
	onDefault, _ := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &p.ID, KBID: &kb1.ID, RuntimeProfileID: &[]string{"prof-1"}[0]})
	deviated, _ := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &p.ID, KBID: &kb2.ID, RuntimeProfileID: &[]string{"prof-x"}[0]})
	_ = onDefault
	_ = deviated
	if _, err := st.SetProjectWorkspace(p.ID, ws.ID); err != nil {
		t.Fatal(err)
	}
	n, err := st.ApplyWorkspaceDefaultsToProject(p.ID)
	if err != nil || n != 1 {
		t.Fatalf("应恰好补齐 1 行: n=%d err=%v", n, err)
	}
	got, _ := st.GetConversation(empty.ID)
	if got.KBID == nil || *got.KBID != kb1.ID || !got.EnableKB || got.RuntimeProfileID == nil || *got.RuntimeProfileID != "prof-1" {
		t.Fatalf("空行应补齐库+方案: %+v %+v", got.KBID, got.RuntimeProfileID)
	}
	dev, _ := st.GetConversation(deviated.ID)
	if dev.KBID == nil || *dev.KBID != kb2.ID {
		t.Fatalf("显式偏差不应覆写: %v", dev.KBID)
	}
	// 幂等：重复执行零变化
	n2, err := st.ApplyWorkspaceDefaultsToProject(p.ID)
	if err != nil || n2 != 0 {
		t.Fatalf("重复执行应零变化: n=%d err=%v", n2, err)
	}
	// 工作区默认换库后再应用：仍处旧默认绑定的行不可判定（无继承溯源列——106 号「仍处继承值」
	// 可判定集=空行∪等于当前默认；旧默认行与显式换绑同形）→ 不覆写，诚实边界落断言
	if _, err := st.UpdateWorkspace(&Workspace{ID: ws.ID, Name: "w", DefaultKBID: kb2.ID}); err != nil {
		t.Fatal(err)
	}
	n3, err := st.ApplyWorkspaceDefaultsToProject(p.ID)
	if err != nil || n3 != 0 {
		t.Fatalf("旧默认绑定行不可判定应不动: n=%d err=%v", n3, err)
	}
	ref, _ := st.GetConversation(empty.ID)
	if ref.KBID == nil || *ref.KBID != kb1.ID {
		t.Fatalf("旧行应保持原绑定: %v", ref.KBID)
	}
	// 未归属项目 → 400
	free, _ := st.CreateProject(&Project{Name: "free"})
	if _, err := st.ApplyWorkspaceDefaultsToProject(free.ID); err == nil {
		t.Fatal("未归属项目应用应 400")
	}
}

// TestListProjectsByWorkspace 按归属过滤列表（未归属不入结果）。
func TestListProjectsByWorkspace(t *testing.T) {
	st := newProjectWorkspaceTestStore(t)
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w"})
	pa, _ := st.CreateProject(&Project{Name: "A"})
	pb, _ := st.CreateProject(&Project{Name: "B"})
	_, _ = st.SetProjectWorkspace(pa.ID, ws.ID)
	got, err := st.ListProjectsByWorkspace(ws.ID)
	if err != nil || len(got) != 1 || got[0].ID != pa.ID {
		t.Fatalf("过滤失真: %v %v", got, err)
	}
	_ = pb
	// 全量列表含 workspace_id 字段往返
	all, err := st.ListProjects()
	if err != nil || len(all) != 2 {
		t.Fatalf("全量失真: %v", err)
	}
}
