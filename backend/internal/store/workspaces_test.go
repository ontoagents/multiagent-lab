package store

// REQ-548/M355 工作区领域容器存储层单测：CRUD/创建时继承（写时复制）/互斥校验/失效回落/
// 删工作区 SET NULL/列表过滤/显式关闭不被复活。零外部依赖。

import (
	"errors"
	"path/filepath"
	"testing"
)

func newWorkspaceTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestWorkspaceCRUD(t *testing.T) {
	st := newWorkspaceTestStore(t)
	w, err := st.CreateWorkspace(&Workspace{Name: "学习工作区", Description: "日常学习"})
	if err != nil {
		t.Fatal(err)
	}
	if w.ID == "" || w.Name != "学习工作区" || w.DefaultKBID != "" {
		t.Fatalf("创建往返失真: %+v", w)
	}
	w2, err := st.UpdateWorkspace(&Workspace{ID: w.ID, Name: "改名", DefaultProfileID: "prof_x"})
	if err != nil {
		t.Fatal(err)
	}
	if w2.Name != "改名" || w2.DefaultProfileID != "prof_x" {
		t.Fatalf("更新失真: %+v", w2)
	}
	list, err := st.ListWorkspaces()
	if err != nil || len(list) != 1 || list[0].ID != w.ID {
		t.Fatalf("列表失真: %v %v", list, err)
	}
	if err := st.DeleteWorkspace(w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetWorkspace(w.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删后应 404: %v", err)
	}
	if err := st.DeleteWorkspace("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删不存在应 404: %v", err)
	}
	if _, err := st.CreateWorkspace(&Workspace{}); err == nil {
		t.Fatal("空名应 400")
	}
}

func TestCreateConversationWorkspaceInheritance(t *testing.T) {
	st := newWorkspaceTestStore(t)
	kb, err := st.CreateKnowledgeBase(&KnowledgeBase{Name: "默认库", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := st.CreateAgent(&Agent{Name: "助手"})
	if err != nil {
		t.Fatal(err)
	}
	// 工作区绑默认库+默认方案（方案 ID 任意字符串——store 层不校验存在性，失效走降级路径）
	ws, err := st.CreateWorkspace(&Workspace{Name: "w", DefaultKBID: kb.ID, DefaultProfileID: "prof-1", DefaultAgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	agentID := agent.ID
	c, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	// 继承断言：免选库与方案即获召回与本体指引
	if c.KBID == nil || *c.KBID != kb.ID {
		t.Fatalf("默认库未继承: %+v", c.KBID)
	}
	if !c.EnableKB {
		t.Fatal("继承默认库应启用召回")
	}
	if c.RuntimeProfileID == nil || *c.RuntimeProfileID != "prof-1" {
		t.Fatalf("默认方案未继承: %+v", c.RuntimeProfileID)
	}
	if !c.OntologyEnabled {
		t.Fatal("继承默认方案应启用本体指引")
	}
	if c.WorkspaceID == nil || *c.WorkspaceID != ws.ID {
		t.Fatalf("归属缺失: %+v", c.WorkspaceID)
	}

	// 显式参数优先（前向兼容位——请求显式带 KBID/方案时不继承）
	kb2, _ := st.CreateKnowledgeBase(&KnowledgeBase{Name: "显式库", Mode: "rag"})
	explicitKB := kb2.ID
	explicitProfile := "prof-explicit"
	c2, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws.ID, KBID: &explicitKB, RuntimeProfileID: &explicitProfile})
	if err != nil {
		t.Fatal(err)
	}
	if *c2.KBID != kb2.ID || *c2.RuntimeProfileID != "prof-explicit" {
		t.Fatalf("显式参数应优先: %+v %+v", c2.KBID, c2.RuntimeProfileID)
	}
}

func TestCreateConversationWorkspaceMissingAndMutex(t *testing.T) {
	st := newWorkspaceTestStore(t)
	agent, _ := st.CreateAgent(&Agent{Name: "助手"})
	agentID := agent.ID
	// 工作区不存在→400 人话（FK 亦拒，显式前置）
	noWS := "ws_missing"
	if _, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &noWS}); err == nil {
		t.Fatal("不存在工作区应报错")
	}
	// 互斥：project_id + workspace_id 同带→400
	proj, perr := st.CreateProject(&Project{Name: "互斥夹具项目"})
	if perr != nil {
		t.Fatal(perr)
	}
	projID := proj.ID
	wsID := "ws_x"
	_, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &projID, WorkspaceID: &wsID})
	if err == nil {
		t.Fatal("互斥应 400")
	}
	if he, ok := err.(*HTTPError); !ok || he.Status != 400 {
		t.Fatalf("互斥应 400: %v", err)
	}
	// project 会话不带 workspace_id 不受影响（存量路径零变化）
	pc, err := st.CreateConversation(&Conversation{Scope: "project", ProjectID: &projID})
	if err != nil {
		t.Fatalf("存量项目路径应不变: %v", err)
	}
	if pc.WorkspaceID != nil {
		t.Fatalf("存量项目会话不应有工作区: %+v", pc.WorkspaceID)
	}
}

func TestDeleteWorkspaceSetsConversationNull(t *testing.T) {
	st := newWorkspaceTestStore(t)
	agent, _ := st.CreateAgent(&Agent{Name: "助手"})
	agentID := agent.ID
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w"})
	c, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetConversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != nil {
		t.Fatalf("删工作区应 SET NULL（消息资产保留）: %+v", got.WorkspaceID)
	}
}

func TestListConversationsWorkspaceFilter(t *testing.T) {
	st := newWorkspaceTestStore(t)
	agent, _ := st.CreateAgent(&Agent{Name: "助手"})
	agentID := agent.ID
	ws1, _ := st.CreateWorkspace(&Workspace{Name: "w1"})
	ws2, _ := st.CreateWorkspace(&Workspace{Name: "w2"})
	st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws1.ID})
	st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws2.ID})
	st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID}) // 无归属
	got, err := st.ListConversations(ConversationFilter{WorkspaceID: ws1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].WorkspaceID == nil || *got[0].WorkspaceID != ws1.ID {
		t.Fatalf("工作区过滤失真: %v", got)
	}
}

func TestExplicitCloseNotRevived(t *testing.T) {
	st := newWorkspaceTestStore(t)
	kb, _ := st.CreateKnowledgeBase(&KnowledgeBase{Name: "默认库", Mode: "rag"})
	agent, _ := st.CreateAgent(&Agent{Name: "助手"})
	agentID := agent.ID
	ws, _ := st.CreateWorkspace(&Workspace{Name: "w", DefaultKBID: kb.ID})
	c, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &agentID, WorkspaceID: &ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.KBID == nil {
		t.Fatal("前置：应继承默认库")
	}
	// 用户在会话内显式关闭 KB chip（patchConv 现有通路：kb_id='' + enable_kb=false）
	empty := ""
	no := false
	c2, err := st.UpdateConversation(&Conversation{ID: c.ID, KBID: &empty, EnableKB: no})
	if err != nil {
		t.Fatal(err)
	}
	// 写时复制核心收益：关闭后行内即普通会话级配置，不被工作区默认复活
	if c2.KBID != nil && *c2.KBID != "" {
		t.Fatalf("显式关闭后被复活: %+v", c2.KBID)
	}
	if c2.EnableKB {
		t.Fatal("显式关闭后 enable_kb 应为 false")
	}
}
