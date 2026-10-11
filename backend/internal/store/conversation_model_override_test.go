package store

import (
	"path/filepath"
	"testing"
)

// REQ-291：会话级模型覆盖（conversation.model_conn_id，''=跟随）CRUD 语义——
// 创建默认 ''/显式设置/nil 更新保留（COALESCE）/'' 更新清除/整行替换路径不丢其他字段。
func TestConversationModelOverride(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ag := &Agent{Name: "t-conv-model"}
	created, err := st.CreateAgent(ag)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// 1) 创建不传 → 默认 ''（跟随智能体默认）
	conv := &Conversation{ID: "c1", Scope: "agent", AgentID: &created.ID}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatalf("create conv: %v", err)
	}
	got, err := st.GetConversation("c1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ModelConnID == nil || *got.ModelConnID != "" {
		t.Fatalf("新建会话应默认 ''（跟随），得到 %#v", got.ModelConnID)
	}

	// 2) 设置覆盖 → 落库可读
	if _, err := st.UpdateConversation(&Conversation{ID: "c1", ModelConnID: strPtr("conn-b")}); err != nil {
		t.Fatalf("update override: %v", err)
	}
	if got, _ = st.GetConversation("c1"); got.ModelConnID == nil || *got.ModelConnID != "conn-b" {
		t.Fatalf("覆盖未落库: %#v", got.ModelConnID)
	}

	// 3) nil 更新（部分更新方如标题改名）→ 保留原值不丢
	if _, err := st.UpdateConversation(&Conversation{ID: "c1", Title: "改名"}); err != nil {
		t.Fatalf("update title: %v", err)
	}
	if got, _ = st.GetConversation("c1"); got.ModelConnID == nil || *got.ModelConnID != "conn-b" {
		t.Fatalf("nil 更新应保留覆盖（COALESCE）: %#v", got.ModelConnID)
	}
	if got.Title != "改名" {
		t.Fatalf("标题应已更新: %q", got.Title)
	}

	// 4) '' 更新 → 清除覆盖（恢复跟随）
	if _, err := st.UpdateConversation(&Conversation{ID: "c1", ModelConnID: strPtr("")}); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if got, _ = st.GetConversation("c1"); got.ModelConnID == nil || *got.ModelConnID != "" {
		t.Fatalf("'' 更新应清除覆盖恢复跟随: %#v", got.ModelConnID)
	}
}

// REQ-291：项目会话同样承载列（chip 对项目会话可见，作用于运行目标）。
func TestConversationModelOverrideProjectScope(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	p := &Project{Name: "t-proj-conv-model"}
	created, err := st.CreateProject(p)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	conv := &Conversation{ID: "cp1", Scope: "project", ProjectID: &created.ID}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatalf("create conv: %v", err)
	}
	if _, err := st.UpdateConversation(&Conversation{ID: "cp1", ModelConnID: strPtr("conn-x")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetConversation("cp1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ModelConnID == nil || *got.ModelConnID != "conn-x" {
		t.Fatalf("项目会话覆盖未落库: %#v", got.ModelConnID)
	}
}

func strPtr(s string) *string { return &s }
