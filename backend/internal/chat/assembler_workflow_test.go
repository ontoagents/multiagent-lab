package chat

// REQ-205/M40：workflow_mode 兑现装配单测——sequential/parallel/loop 三模式走 ADK
// 工作流原语（成员即节点）；快照/告警（NOT RECOMMENDED 复核注+loop 迭代上限）/
// WorkflowMeta 节点清单；free 与未知模式回落自由协作。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func workflowFixture(t *testing.T) (*Assembler, *store.Store) {
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
		Name: "wf-conn", ConnType: "chat", Protocol: "openai_compat",
		BaseURL: "http://127.0.0.1:1", ModelName: "stub", Enabled: true, IsDefault: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	asm := &Assembler{Store: st, Box: box}
	return asm, st
}

func workflowProject(t *testing.T, st *store.Store, workflowMode string) (*store.Project, *store.Conversation) {
	return workflowProjectNamed(t, st, workflowMode, "")
}

func workflowProjectNamed(t *testing.T, st *store.Store, workflowMode, tag string) (*store.Project, *store.Conversation) {
	t.Helper()
	names := []string{"wf-alpha" + tag, "wf-beta" + tag}
	var memberIDs []store.ProjectMember
	for i, n := range names {
		ag, err := st.CreateAgent(&store.Agent{Name: n, Description: n + " 节点成员"})
		if err != nil {
			t.Fatal(err)
		}
		role := "member"
		if i == 0 {
			role = "coordinator" // 工作流节点序=协调者首个（确定性拓扑前提）
		}
		memberIDs = append(memberIDs, store.ProjectMember{AgentID: ag.ID, Role: role})
	}
	p, err := st.CreateProject(&store.Project{Name: "wf-project" + tag, CollabMode: "agent_as_tool", WorkflowMode: workflowMode})
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
	return p, conv
}

func TestAssembleWorkflowModes(t *testing.T) {
	for _, mode := range []string{"sequential", "parallel", "loop"} {
		t.Run(mode, func(t *testing.T) {
			asm, st := workflowFixture(t)
			_, conv := workflowProject(t, st, mode)
			rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
			if err != nil {
				t.Fatalf("%s 装配失败: %v", mode, err)
			}
			if rt.Workflow == nil || rt.Workflow.Mode != mode {
				t.Fatalf("Workflow 元数据缺失: %+v", rt.Workflow)
			}
			if len(rt.Workflow.Nodes) != 2 || rt.Workflow.Nodes[0] != "wf-alpha" || rt.Workflow.Nodes[1] != "wf-beta" {
				t.Fatalf("节点清单不符（成员序）: %v", rt.Workflow.Nodes)
			}
			if rt.Runner == nil {
				t.Fatal("Runner 应非 nil")
			}
			warns := strings.Join(rt.Warnings, "\n")
			if !strings.Contains(warns, "NOT RECOMMENDED") {
				t.Fatalf("应含官方口径复核注: %s", warns)
			}
			if mode == "loop" && !strings.Contains(warns, "迭代上限固定 3") {
				t.Fatalf("loop 应注迭代上限: %s", warns)
			}
			if rt.Snapshot == nil {
				t.Fatal("快照应非 nil")
			}
		})
	}
}

func TestAssembleWorkflowFreeFallback(t *testing.T) {
	asm, st := workflowFixture(t)
	// free：按 collab_mode 分发（agent_as_tool），非工作流
	_, conv := workflowProject(t, st, "free")
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Workflow != nil {
		t.Fatalf("free 不应有 Workflow 元数据: %+v", rt.Workflow)
	}
	// 未知模式：回落自由协作 + 告警
	_, conv2 := workflowProjectNamed(t, st, "dag", "-2")
	rt2, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv2)
	if err != nil {
		t.Fatal(err)
	}
	if rt2.Workflow != nil || !strings.Contains(strings.Join(rt2.Warnings, "\n"), "未知工作流模式") {
		t.Fatalf("未知模式应回落+告警: %+v %v", rt2.Workflow, rt2.Warnings)
	}
}
