package chat

// REQ-446/M242 单测：外部成员（CLI/A2A）进 workflow 节点与 transfer 轨——适配器执行级
// （Run 输出 artifacts 文本/REQ-319 降级载荷流转/任务取末条用户消息）+ 装配级（workflow
// 节点清单与快照/transfer 转移面告警/连接器序确定性/CLI 沙箱闸在 workflow 同样生效）。

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/inference"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// collectAgentEvents 收敛 adk.Agent 迭代器取末事件（适配器为单事件形态）。
func collectAgentEvents(t *testing.T, ag adk.Agent, msgs []*schema.Message) *adk.AgentEvent {
	t.Helper()
	iter := ag.Run(context.Background(), &adk.AgentInput{Messages: msgs})
	var last *adk.AgentEvent
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		last = ev
	}
	if last == nil {
		t.Fatal("适配器应至少产出一个事件")
	}
	return last
}

func TestExternalMemberAgentRun(t *testing.T) {
	ag := newA2AMemberAgent(a2aMemberDeps{
		Name:     "外部审查员",
		Endpoint: stubA2AEndpoint(t, ""),
		CardJSON: `{"name":"外部审查员","skills":[{"id":"s","name":"代码审查"}]}`,
		PollSec:  0.002,
	})
	if ag.Name(context.Background()) != "外部审查员" {
		t.Fatalf("Name 应为成员名: %s", ag.Name(context.Background()))
	}
	if desc := ag.Description(context.Background()); !strings.Contains(desc, "代码审查") {
		t.Fatalf("描述应复用卡片合成（transfer 转移决策路由信号）: %s", desc)
	}
	ev := collectAgentEvents(t, ag, []*schema.Message{{Role: schema.User, Content: "审查这段代码"}})
	if ev.Err != nil {
		t.Fatalf("成功路径不应有错: %v", ev.Err)
	}
	if ev.AgentName != "外部审查员" {
		t.Fatalf("事件应带成员 AgentName（runner 节点事件基准）: %q", ev.AgentName)
	}
	msg, err := ev.Output.MessageOutput.GetMessage()
	if err != nil {
		t.Fatalf("取输出消息失败: %v", err)
	}
	if !strings.Contains(msg.Content, "外部审查结论") {
		t.Fatalf("输出应含远端 artifacts 文本: %s", msg.Content)
	}

	// REQ-319 同语义降级：远端 failed → member_failed 载荷作节点输出（不炸 run，下游可见）
	bad := newA2AMemberAgent(a2aMemberDeps{Name: "外部审查员", Endpoint: stubA2AEndpoint(t, "fail"), PollSec: 0.002})
	ev = collectAgentEvents(t, bad, []*schema.Message{{Role: schema.User, Content: "任务"}})
	if ev.Err != nil {
		t.Fatalf("降级路径不返回 error（载荷文本流转）: %v", ev.Err)
	}
	msg, _ = ev.Output.MessageOutput.GetMessage()
	if !strings.Contains(msg.Content, "member_failed") {
		t.Fatalf("降级载荷应作节点输出: %s", msg.Content)
	}
}

func TestExternalTaskFromMessages(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.User, Content: "第一轮"},
		{Role: schema.Assistant, Content: "中间产物"},
		{Role: schema.User, Content: "最新任务"},
	}
	if got := externalTaskFromMessages(msgs); got != "最新任务" {
		t.Fatalf("应取末条用户消息: %q", got)
	}
	// 无用户消息才退化取末条非空（有用户消息时优先级更高，即便更早）
	if got := externalTaskFromMessages([]*schema.Message{
		{Role: schema.User, Content: "第一轮"},
		{Role: schema.Assistant, Content: "中间产物"},
	}); got != "第一轮" {
		t.Fatalf("有用户消息时应优先取用户消息: %q", got)
	}
	if got := externalTaskFromMessages([]*schema.Message{
		{Role: schema.Assistant, Content: "甲"},
		{Role: schema.Assistant, Content: "乙"},
	}); got != "乙" {
		t.Fatalf("无用户消息退化取末条非空: %q", got)
	}
	if got := externalTaskFromMessages(nil); got != "" {
		t.Fatalf("空输入应得空串: %q", got)
	}
}

// attachConnector 建外部连接器并挂到指定智能体（工作流/transfer 夹具用）。
func attachConnector(t *testing.T, st *store.Store, agentID, kind, name string, cfg map[string]any, card string) {
	t.Helper()
	c, err := st.CreateConnector(&store.Connector{Kind: kind, Name: name, Config: cfg, CardJSON: card})
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.GetAgent(agentID)
	if err != nil {
		t.Fatal(err)
	}
	ag.Connectors = append(ag.Connectors, c.ID)
	if _, err := st.UpdateAgent(ag); err != nil {
		t.Fatal(err)
	}
}

func findAgentByName(t *testing.T, st *store.Store, name string) *store.Agent {
	t.Helper()
	ags, err := st.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, ag := range ags {
		if ag.Name == name {
			return ag
		}
	}
	t.Fatalf("智能体 %q 不存在", name)
	return nil
}

func TestAssembleWorkflowExternalMembers(t *testing.T) {
	asm, st := workflowFixture(t)
	_, conv := workflowProjectNamed(t, st, "sequential", "")
	attachConnector(t, st, findAgentByName(t, st, "wf-alpha").ID, store.ConnectorKindA2A, "外部审查员",
		map[string]any{"url": stubA2AEndpoint(t, ""), "poll_interval_seconds": 0.002},
		`{"name":"外部审查员","skills":[{"id":"s","name":"代码审查"}]}`)
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	if rt.Workflow == nil {
		t.Fatal("Workflow 元数据缺失")
	}
	// 节点序=本地成员（协调者首个+名字序）后按连接器序追加外部成员
	if len(rt.Workflow.Nodes) != 3 || rt.Workflow.Nodes[2] != "外部审查员" {
		t.Fatalf("外部成员应追加为第三节点: %v", rt.Workflow.Nodes)
	}
	warns := strings.Join(rt.Warnings, "\n")
	if !strings.Contains(warns, "工作流接入外部成员节点 1 个") || !strings.Contains(warns, "外部节点无审批面") {
		t.Fatalf("应含外部节点接入注记与诚实边界: %s", warns)
	}
	if !strings.Contains(rt.SourceOf["外部审查员"], "a2a:") {
		t.Fatalf("来源表应标注 a2a 连接器: %v", rt.SourceOf["外部审查员"])
	}
	snap := jsonMarshalSnapshot(t, rt.Snapshot)
	if !strings.Contains(snap, `"role":"node"`) || !strings.Contains(snap, "A2A 外部智能体") {
		t.Fatalf("快照应含外部节点条目: %s", snap)
	}
}

func TestAssembleTransferExternalMembers(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnection(&store.ModelConnection{
		Name: "tr-conn", ConnType: "chat", Protocol: "openai_compat",
		BaseURL: "http://127.0.0.1:1", ModelName: "stub", Enabled: true, IsDefault: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	asm := &Assembler{Store: st, Box: box}
	coord, err := st.CreateAgent(&store.Agent{Name: "协调者", Description: "转移对照"})
	if err != nil {
		t.Fatal(err)
	}
	member, err := st.CreateAgent(&store.Agent{Name: "内部成员", Description: "本地节点"})
	if err != nil {
		t.Fatal(err)
	}
	attachConnector(t, st, coord.ID, store.ConnectorKindA2A, "外部审查员",
		map[string]any{"url": stubA2AEndpoint(t, ""), "poll_interval_seconds": 0.002},
		`{"name":"外部审查员"}`)
	p, err := st.CreateProject(&store.Project{Name: "tr-project", CollabMode: "transfer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectAgents(p.ID, []store.ProjectMember{
		{AgentID: coord.ID, Role: "coordinator"},
		{AgentID: member.ID, Role: "member"},
	}); err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "project", ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	warns := strings.Join(rt.Warnings, "\n")
	if !strings.Contains(warns, "transfer 轨接入外部成员 1 个") {
		t.Fatalf("应含 transfer 外部成员注记: %s", warns)
	}
	if !strings.Contains(rt.SourceOf["外部审查员"], "a2a:") {
		t.Fatalf("来源表应标注 a2a 连接器: %v", rt.SourceOf["外部审查员"])
	}
}

func TestAssembleWorkflowCLIMemberNode(t *testing.T) {
	stubClaudeBin(t)
	asm, st := workflowFixture(t)
	asm.Inference = inference.NewRegistry() // 预置内置适配器（claude-code 在册）
	_, conv := workflowProjectNamed(t, st, "sequential", "")
	attachConnector(t, st, findAgentByName(t, st, "wf-alpha").ID, store.ConnectorKindCLI, "cli编码者",
		map[string]any{"backend": "claude-code", "task_prefix": "只写代码"},
		`{"type":"cli-probe","backend":"claude-code","version":"9.9.9","path":"/tmp/stub/claude"}`)
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	if len(rt.Workflow.Nodes) != 3 || rt.Workflow.Nodes[2] != "cli编码者" {
		t.Fatalf("CLI 外部成员应追加为第三节点: %v", rt.Workflow.Nodes)
	}
	snap := jsonMarshalSnapshot(t, rt.Snapshot)
	if !strings.Contains(snap, "CLI 外部智能体（claude-code）") {
		t.Fatalf("快照应标注 CLI 外部节点: %s", snap)
	}
}

func TestAssembleWorkflowCLISandboxGateStillApplies(t *testing.T) {
	asm, st := workflowFixture(t)
	asm.Inference = inference.NewRegistry()
	_, conv := workflowProjectNamed(t, st, "sequential", "")
	attachConnector(t, st, findAgentByName(t, st, "wf-alpha").ID, store.ConnectorKindCLI, "沙箱成员",
		map[string]any{"backend": "claude-code", "sandbox_mode": "docker"}, "")
	rt, err := asm.Assemble(context.Background(), &store.Agent{Name: "entry"}, conv)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	// REQ-427② 三前置闸在 workflow 轨同样生效：docker 不可达 → 告警跳过（不装配节点）
	if len(rt.Workflow.Nodes) != 2 {
		t.Fatalf("沙箱闸不满足时外部节点不应装配: %v", rt.Workflow.Nodes)
	}
	if !strings.Contains(strings.Join(rt.Warnings, "\n"), "docker 不可达") {
		t.Fatal("应如实告警 docker 不可达")
	}
}

// jsonMarshalSnapshot 快照序列化（断言辅助）。
func jsonMarshalSnapshot(t *testing.T, snap map[string]any) string {
	t.Helper()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestExternalMemberAgentInADKWorkflow 适配器进 ADK 工作流原语集成：sequential 两外部节点
// 顺序执行——事件 AgentName 沿流转（runner trackAgent 据此发 subagent.enter/exit 与
// workflow.node started/completed）、节点输出文本符合远端 stub。
func TestExternalMemberAgentInADKWorkflow(t *testing.T) {
	first := newA2AMemberAgent(a2aMemberDeps{
		Name: "外部规划者", Endpoint: stubA2AEndpoint(t, ""), PollSec: 0.002,
		CardJSON: `{"name":"外部规划者"}`,
	})
	second := newA2AMemberAgent(a2aMemberDeps{
		Name: "外部审查员", Endpoint: stubA2AEndpoint(t, ""), PollSec: 0.002,
		CardJSON: `{"name":"外部审查员"}`,
	})
	wf, err := adk.NewSequentialAgent(context.Background(), &adk.SequentialAgentConfig{
		Name: "workflow_sequential", Description: "全外部节点工作流",
		SubAgents: []adk.Agent{first, second},
	})
	if err != nil {
		t.Fatalf("构建工作流失败: %v", err)
	}
	iter := wf.Run(context.Background(), &adk.AgentInput{
		Messages: []*schema.Message{{Role: schema.User, Content: "规划并审查"}},
	})
	seen := map[string]string{} // AgentName → 输出文本
	var order []string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("工作流执行不应出错: %v", ev.Err)
		}
		if ev.AgentName == "" || ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		if _, dup := seen[ev.AgentName]; !dup {
			order = append(order, ev.AgentName)
			if msg, gerr := ev.Output.MessageOutput.GetMessage(); gerr == nil {
				seen[ev.AgentName] = msg.Content
			}
		}
	}
	if len(order) != 2 || order[0] != "外部规划者" || order[1] != "外部审查员" {
		t.Fatalf("两外部节点应按序产出事件: %v", order)
	}
	if !strings.Contains(seen["外部规划者"], "外部审查结论") {
		t.Fatalf("节点输出应为远端 artifacts 文本: %q", seen["外部规划者"])
	}
}
