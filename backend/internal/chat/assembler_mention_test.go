package chat

// REQ-623-4371/M433 装配单测：@ 点名定向路由——mention 分散模式（无点名拒绝/多点名
// SequentialAgent 按提及序/工作流编排不消费告警）与既有模式 @ 直投增强（单成员直跑）；
// 无点名时原协作编排零变化（回归确认）。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func mentionFixture(t *testing.T) (*Assembler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	// 默认模型连接（装配期仅构造客户端不发请求——假 base_url 即可）
	if _, err := st.CreateConnection(&store.ModelConnection{
		Name: "mention-conn", ConnType: "chat", Protocol: "openai_compat",
		BaseURL: "http://127.0.0.1:1", ModelName: "stub", Enabled: true, IsDefault: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	asm := &Assembler{Store: st, Box: box}
	return asm, st
}

func mentionProject(t *testing.T, st *store.Store, collabMode string) (*store.Project, *store.Conversation, []*store.Agent) {
	t.Helper()
	names := []string{"m-alpha", "m-beta"}
	var memberIDs []store.ProjectMember
	var agents []*store.Agent
	for _, n := range names {
		ag, err := st.CreateAgent(&store.Agent{Name: n, Description: n + " 点名成员"})
		if err != nil {
			t.Fatal(err)
		}
		agents = append(agents, ag)
		memberIDs = append(memberIDs, store.ProjectMember{AgentID: ag.ID, Role: "member"}) // mention 不设协调者
	}
	p, err := st.CreateProject(&store.Project{Name: "mention-project", CollabMode: collabMode})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectAgents(p.ID, memberIDs); err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "project", ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	return p, conv, agents
}

// ①mention 模式无点名 → 装配拒绝（防御路径——API 层已在 SSE 前拦截 400）
func TestAssembleMentionNoTargetsRejected(t *testing.T) {
	asm, st := mentionFixture(t)
	_, conv, _ := mentionProject(t, st, "mention")
	if _, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv); err == nil {
		t.Fatal("mention 模式无点名应拒绝装配")
	} else if !strings.Contains(err.Error(), "@ 点名") {
		t.Fatalf("错误文案应含 @ 点名指引: %v", err)
	}
}

// ②mention 模式多点名 → SequentialAgent 按提及序（后者可见前者回答的群聊语义）
func TestAssembleMentionMultiSequential(t *testing.T) {
	asm, st := mentionFixture(t)
	_, conv, agents := mentionProject(t, st, "mention")
	ctx := WithMentionTargets(context.Background(), []MentionTarget{
		{AgentID: agents[1].ID}, {AgentID: agents[0].ID}, // 提及序=beta→alpha（非成员表序）
	})
	rt, err := asm.Assemble(ctx, &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("多点名装配失败: %v", err)
	}
	if rt.Workflow == nil || rt.Workflow.Mode != "mention_sequential" {
		t.Fatalf("Workflow 元数据应为 mention_sequential: %+v", rt.Workflow)
	}
	if len(rt.Workflow.Nodes) != 2 || rt.Workflow.Nodes[0] != "m-beta" || rt.Workflow.Nodes[1] != "m-alpha" {
		t.Fatalf("节点应按提及序 beta→alpha: %v", rt.Workflow.Nodes)
	}
	if rt.Runner == nil || rt.Snapshot == nil {
		t.Fatal("Runner/快照应非 nil")
	}
	joined := strings.Join(rt.Warnings, "\n")
	if !strings.Contains(joined, "@ 点名模式") {
		t.Fatalf("应含点名模式语义注: %s", joined)
	}
}

// ③mention 模式点名时工作流编排不消费（诚实告警）
func TestAssembleMentionWorkflowNotConsumed(t *testing.T) {
	asm, st := mentionFixture(t)
	p, conv, agents := mentionProject(t, st, "mention")
	p.WorkflowMode = "sequential"
	if _, err := st.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	ctx := WithMentionTargets(context.Background(), []MentionTarget{{AgentID: agents[0].ID}})
	rt, err := asm.Assemble(ctx, &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	if !strings.Contains(strings.Join(rt.Warnings, "\n"), "不消费工作流编排") {
		t.Fatal("应含工作流编排不消费告警")
	}
}

// ④agent_as_tool 模式 @ 直投增强：单成员按 assembleSingle 形态直跑（无 Workflow 元数据）
func TestAssembleMentionDirectInAgentAsTool(t *testing.T) {
	asm, st := mentionFixture(t)
	_, conv, agents := mentionProject(t, st, "agent_as_tool")
	ctx := WithMentionTargets(context.Background(), []MentionTarget{{AgentID: agents[1].ID}})
	rt, err := asm.Assemble(ctx, &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("直投装配失败: %v", err)
	}
	if rt.Workflow != nil {
		t.Fatalf("单成员直跑不应带 Workflow 元数据: %+v", rt.Workflow)
	}
	if rt.AgentName != "m-beta" {
		t.Fatalf("直投应装配被点名成员（m-beta），got %s", rt.AgentName)
	}
}

// ⑤回归：项目会话无点名 → 原 agent_as_tool 编排零变化（mention 分支不误入）
func TestAssembleProjectNoMentionUnchanged(t *testing.T) {
	asm, st := mentionFixture(t)
	_, conv, _ := mentionProject(t, st, "agent_as_tool")
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("原路径装配失败: %v", err)
	}
	if rt.Workflow != nil {
		t.Fatalf("agent_as_tool 原路径不应带 Workflow 元数据: %+v", rt.Workflow)
	}
	joined := strings.Join(rt.Warnings, "\n")
	if strings.Contains(joined, "@ 点名模式") {
		t.Fatalf("无点名不应出现点名语义注: %s", joined)
	}
}
