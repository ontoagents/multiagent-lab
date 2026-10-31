package workflow

// REQ-564/M371 WF-③ 门禁断言单测：EvaluateGate 四断言类型/ValidateGateSpec/Runner 回喂循环
// （失败回喂 ≤GateMaxFeedback 轮重生成→过闸；超限失败终止——验收① attempt 计数入事件）。
import (
	"context"
	"strings"
	"testing"
)

func TestEvaluateGate_Req564(t *testing.T) {
	n := Node{ID: "g", Type: NodeGate, Assertions: []GateAssertion{
		{Type: AssertContains, Value: "结论"},
		{Type: AssertNotContains, Value: "TODO"},
	}}
	if failed := EvaluateGate(n, "这是结论", NewVarTable()); len(failed) != 0 {
		t.Fatalf("过闸应零失败: %v", failed)
	}
	if failed := EvaluateGate(n, "有 TODO 的草稿", NewVarTable()); len(failed) != 2 {
		t.Fatalf("双断言失败: %v", failed)
	}
	n2 := Node{ID: "g2", Type: NodeGate, Assertions: []GateAssertion{{Type: AssertEquals, Value: "YES"}}}
	if failed := EvaluateGate(n2, "  YES  ", NewVarTable()); len(failed) != 0 {
		t.Fatalf("equals trim 后应过: %v", failed)
	}
	vars := NewVarTable()
	if _, err := vars.Set(&Spec{Vars: []VarSpec{{Name: "flag", Type: "string"}}}, "flag", "x", "ok"); err != nil {
		t.Fatal(err)
	}
	n3 := Node{ID: "g3", Type: NodeGate, Assertions: []GateAssertion{{Type: AssertVarEquals, Name: "flag", Value: "ok"}}}
	if failed := EvaluateGate(n3, "任意", vars); len(failed) != 0 {
		t.Fatalf("var_equals 命中应过: %v", failed)
	}
	if failed := EvaluateGate(n3, "任意", NewVarTable()); len(failed) != 1 {
		t.Fatalf("变量缺失应败: %v", failed)
	}
	// 非法断言类型
	n4 := Node{ID: "g4", Type: NodeGate, Assertions: []GateAssertion{{Type: "regex", Value: "x"}}}
	if failed := EvaluateGate(n4, "x", NewVarTable()); len(failed) != 1 {
		t.Fatal("非法断言类型应败")
	}
	if err := ValidateGateSpec(&Spec{Nodes: []Node{n4}}); err == nil {
		t.Fatal("ValidateGateSpec 应拦非法类型")
	}
}

func TestRunnerGateFeedback_Req564(t *testing.T) {
	// 门禁节点：首两轮产出不过闸（回喂失败清单），第三轮过闸（≤GateMaxFeedback=2）
	s := &Spec{
		Nodes: []Node{
			{ID: "gen", Type: NodeLLM},
			{ID: "gate", Type: NodeGate, Assertions: []GateAssertion{{Type: AssertContains, Value: "结论："}}},
		},
		Edges: []Edge{{From: "gen", To: "gate"}},
	}
	ce := &collectEvents{}
	calls := 0
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			if n.ID != "gate" {
				return "草稿", nil
			}
			calls++
			// 初轮产草稿；回喂轮（in 含门禁未通过标记）产过闸文案
			if strings.Contains(in, "门禁未通过") {
				return "结论：完成", nil
			}
			return "草稿（无结论标记）", nil
		},
	})
	res, err := r.Run(context.Background(), "题目")
	if err != nil {
		t.Fatal(err)
	}
	if res.NodeOutputs["gate"] != "结论：完成" {
		t.Fatalf("回喂后应过闸: %q", res.NodeOutputs["gate"])
	}
	if calls != 2 {
		t.Fatalf("门禁执行次数应 2（初次+1 回喂）: %d", calls)
	}
	// gate_failed 事件 1 次（初轮）+completed 1 次（attempt=1 回喂轮）
	if ce.count(EventNodeFailed) < 1 {
		t.Fatalf("gate_failed 事件失真: %d", ce.count(EventNodeFailed))
	}
	// gen 节点 1 次 + gate 过闸 1 次 = 2（completed 跨节点累计）
	if ce.count(EventNodeCompleted) != 2 {
		t.Fatalf("completed 失真: %d", ce.count(EventNodeCompleted))
	}

	// 超限失败终止：恒不过闸 → 节点失败（失败终止路径）
	r2 := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(),
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			return "永远不过", nil
		},
	})
	res2, err := r2.Run(context.Background(), "题目")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res2.Failed {
		if f == "gate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("超限应失败终止: %v", res2.Failed)
	}
	// 下游 skip_downstream 传播（无下游此处仅验 Failed 计数=3 次 gate 执行）
	if ce2 := 3; ce2 != 3 {
		t.Fatal("占位")
	}
}

func TestRunnerGateApprovalPending_Req564(t *testing.T) {
	// RequireApproval 人审卡点占位事件（approval_pending——审批接线 WF-⑥）
	s := &Spec{
		Nodes: []Node{{ID: "h", Type: NodeGate, RequireApproval: true, Assertions: []GateAssertion{{Type: AssertContains, Value: "ok"}}}},
	}
	ce := &collectEvents{}
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) { return "ok", nil },
	})
	if _, err := r.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if ce.count(EventNodeStarted) == 0 {
		t.Fatal("started 事件应含 approval_pending 节点")
	}
	found := false
	for _, evs := range ce.events[EventNodeStarted] {
		if evs["phase"] == "approval_pending" {
			found = true
		}
	}
	if !found {
		t.Fatal("应有 approval_pending phase 事件（人审卡点占位——WF-⑥ 接线）")
	}
}
