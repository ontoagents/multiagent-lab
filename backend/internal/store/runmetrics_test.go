package store

// REQ-557/M364 过程账本派生聚合单测：三出料（参与度/介入/阶段）与事件流逐条对账口径。
import (
	"path/filepath"
	"testing"
)

func newMetricsTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRunMetrics_Req557(t *testing.T) {
	st := newMetricsTestStore(t)
	aid := "builtin-assistant"
	conv, err := st.CreateConversation(&Conversation{AgentID: &aid, Title: "账本", Scope: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-metrics-1"
	evs := []*RunEvent{
		{ConversationID: conv.ID, RunID: runID, Type: "run.started", Data: `{}`},
		{ConversationID: conv.ID, RunID: runID, Type: "tool.call", Data: `{"agent":"甲","name":"read_file"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "tool.call", Data: `{"agent":"甲","name":"grep"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "tool.call", Data: `{"agent":"乙","name":"write_file"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "subagent.enter", Data: `{"agent":"甲"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "subagent.exit", Data: `{"agent":"甲"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "approval.request", Data: `{"tool_name":"kubectl_get"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "approval.granted", Data: `{}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "steer.pushed", Data: `{}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "run.interrupted", Data: `{"kind":"ask_human"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "subagent.delegate", Data: `{"phase":"completed","member":"乙"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "workflow.node", Data: `{"node":"调研","phase":"started"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "workflow.node", Data: `{"node":"调研","phase":"completed"}`, SchemaVersion: 2},
		{ConversationID: conv.ID, RunID: runID, Type: "run.finished", Data: `{"elapsed_ms":5000,"usage_by_agent":[{"agent":"甲","total_tokens":700},{"agent":"乙","total_tokens":300}]}`, SchemaVersion: 2},
	}
	for _, e := range evs {
		if _, err := st.InsertEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	m, err := st.RunMetrics(runID)
	if err != nil || m == nil {
		t.Fatalf("metrics 失败: %v", err)
	}
	if m.EventCount != len(evs) || m.ConversationID != conv.ID {
		t.Fatalf("计数/会话归属失真: %+v", m)
	}
	// ① 参与度：token 与 tool.call 按成员归账（token 降序——甲 700/乙 300）
	if len(m.Participation) != 2 {
		t.Fatalf("参与度成员数失真: %+v", m.Participation)
	}
	if m.Participation[0].Agent != "甲" || m.Participation[0].TotalTokens != 700 || m.Participation[0].ToolCalls != 2 {
		t.Fatalf("甲参与度失真: %+v", m.Participation[0])
	}
	if m.Participation[1].Agent != "乙" || m.Participation[1].TotalTokens != 300 || m.Participation[1].ToolCalls != 1 {
		t.Fatalf("乙参与度失真: %+v", m.Participation[1])
	}
	// token 闭合（107 §8.4 验收①：各成员之和=run 级 usage.total_tokens——此处 700+300=1000）
	sum := 0
	for _, p := range m.Participation {
		sum += p.TotalTokens
	}
	if sum != 1000 {
		t.Fatalf("token 闭合失真: %d", sum)
	}
	// ② 介入：审批 request1/granted1、interrupted kind=ask_human 1、steer 1、委派 completed 1 成功率 1.0
	iv := m.Interventions
	if iv.ApprovalRequests != 1 || iv.ApprovalGranted != 1 || iv.ApprovalDenied != 0 {
		t.Fatalf("审批介入计数失真: %+v", iv)
	}
	if iv.InterruptedBy["ask_human"] != 1 || iv.SteerPushed != 1 {
		t.Fatalf("挂起/插话计数失真: %+v", iv)
	}
	if iv.DelegatePhases["completed"] != 1 || iv.DelegateSuccess != 1.0 {
		t.Fatalf("委派分布失真: %+v", iv)
	}
	// ③ 阶段：workflow.node started/completed 差 + elapsed_ms
	if m.Phases.ElapsedMS != 5000 {
		t.Fatalf("elapsed 失真: %d", m.Phases.ElapsedMS)
	}
	if len(m.Phases.WorkflowNodes) != 1 || m.Phases.WorkflowNodes[0].Name != "调研" || m.Phases.WorkflowNodes[0].Open {
		t.Fatalf("节点时长失真: %+v", m.Phases.WorkflowNodes)
	}
	// 未知 run → (nil, nil)（handler 404 判定）
	if nm, err := st.RunMetrics("nope"); nm != nil || err != nil {
		t.Fatalf("未知 run 应 (nil,nil): %+v %v", nm, err)
	}
}

func TestMemberUsageStats_Req557(t *testing.T) {
	st := newMetricsTestStore(t)
	aid := "builtin-assistant"
	conv, cerr := st.CreateConversation(&Conversation{AgentID: &aid, Title: "成员聚合", Scope: "agent"})
	if cerr != nil || conv == nil {
		t.Fatalf("会话创建失败: %v", cerr)
	}
	for _, fin := range []string{
		`{"usage_by_agent":[{"agent":"甲","total_tokens":500},{"agent":"乙","total_tokens":200}]}`,
		`{"usage_by_agent":[{"agent":"甲","total_tokens":100}]}`,
	} {
		if _, err := st.InsertEvent(&RunEvent{ConversationID: conv.ID, Type: "run.finished", Data: fin, SchemaVersion: 2}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.MemberUsageStats(UsageRange{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("成员数失真: %+v", rows)
	}
	if rows[0].Member != "甲" || rows[0].TotalTokens != 600 || rows[0].Runs != 2 {
		t.Fatalf("甲聚合失真: %+v", rows[0])
	}
	if rows[1].Member != "乙" || rows[1].TotalTokens != 200 || rows[1].Runs != 1 {
		t.Fatalf("乙聚合失真: %+v", rows[1])
	}
}
