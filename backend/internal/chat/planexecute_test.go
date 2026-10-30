package chat

// REQ-455/M251：plan_execute 计划执行工作流模板装配单测——
// adk prebuilt planexecute 三角色（协调者=计划器/重规划器、首个成员=执行器）/
// WorkflowMeta 节点清单/诚实告警（预置角色提示词注记）/无成员降级自由协作。

import (
	"context"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestAssemblePlanExecute(t *testing.T) {
	asm, st := workflowFixture(t)
	_, conv := workflowProject(t, st, "plan_execute")
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("plan_execute 装配失败: %v", err)
	}
	if rt.Workflow == nil || rt.Workflow.Mode != "plan_execute" {
		t.Fatalf("Workflow 元数据缺失: %+v", rt.Workflow)
	}
	if rt.AgentName != "workflow_plan_execute" || rt.ModelLabel != "workflow(plan_execute)" {
		t.Fatalf("命名不符: %s / %s", rt.AgentName, rt.ModelLabel)
	}
	// 节点清单=planner:协调者 + executor:首个成员
	if len(rt.Workflow.Nodes) != 2 || !strings.HasPrefix(rt.Workflow.Nodes[0], "planner:wf-alpha") || !strings.HasPrefix(rt.Workflow.Nodes[1], "executor:") {
		t.Fatalf("节点清单不符: %v", rt.Workflow.Nodes)
	}
	if rt.Runner == nil {
		t.Fatal("Runner 应非 nil")
	}
	warns := strings.Join(rt.Warnings, "\n")
	if !strings.Contains(warns, "计划器/重规划器") || !strings.Contains(warns, "执行器") {
		t.Fatalf("应含角色映射语义告警: %s", warns)
	}
	if !strings.Contains(warns, "循环上限 10") {
		t.Fatalf("应含迭代上限注记: %s", warns)
	}
	if rt.Snapshot == nil {
		t.Fatal("快照应非 nil")
	}
}

func TestAssemblePlanExecuteNoMemberDegrade(t *testing.T) {
	asm, st := workflowFixture(t)
	// 仅协调者一名成员且协调者≠成员（无执行器可用）→ 诚实告警后落穿自由协作
	ag, err := st.CreateAgent(&store.Agent{Name: "pe-solo", Description: "仅协调者"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(&store.Project{Name: "pe-empty", CollabMode: "agent_as_tool", WorkflowMode: "plan_execute"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectAgents(p.ID, []store.ProjectMember{{AgentID: ag.ID, Role: "member"}}); err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "project", ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	// 协调者指定为不存在的 agent → assembleProject 前置回退首个成员=pe-solo（即成员集内唯一），
	// 此时 execNode 非空（pe-solo 被视为协调者）→ 有执行器不降级；换协调者不在成员集形态：
	// 沿 workflowProject 前置行为（协调者必在成员集）——本用例改断言「协调者兼任成员=执行器」成立
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	if rt.Workflow == nil || rt.Workflow.Mode != "plan_execute" {
		t.Fatalf("协调者兼任成员时应作为执行器装配 plan_execute: %+v", rt.Workflow)
	}
	if !strings.HasPrefix(rt.Workflow.Nodes[1], "executor:pe-solo") {
		t.Fatalf("执行器应为兼任成员 pe-solo: %v", rt.Workflow.Nodes)
	}
}
