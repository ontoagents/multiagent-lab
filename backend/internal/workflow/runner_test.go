package workflow

// REQ-563/M370 WF-② Runner 单测（-race 绿为验收②）：顺序/并行/条件边三类拓扑端到端+
// 一等节点事件四族+失败重试与 skip_downstream 传播+变量写入。
import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// collectEvents 事件收集器（并发安全）。
type collectEvents struct {
	mu     sync.Mutex
	events map[string][]map[string]any
}

func (c *collectEvents) on(t string, d map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.events == nil {
		c.events = map[string][]map[string]any{}
	}
	c.events[t] = append(c.events[t], d)
}

func (c *collectEvents) count(t string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events[t])
}

func TestRunnerSequential_Req563(t *testing.T) {
	s := &Spec{
		Nodes: []Node{
			{ID: "n1", Type: NodeLLM, Name: "首"},
			{ID: "n2", Type: NodeLLM, Name: "次"},
		},
		Edges:   []Edge{{From: "n1", To: "n2"}},
	}
	ce := &collectEvents{}
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			if n.ID == "n2" && !strings.Contains(in, "n1产出") {
				t.Errorf("n2 输入应含 n1 产出，实得 %q", in)
			}
			return n.ID + "产出", nil
		},
	})
	res, err := r.Run(context.Background(), "原始输入")
	if err != nil {
		t.Fatal(err)
	}
	if res.NodeOutputs["n1"] != "n1产出" || res.NodeOutputs["n2"] != "n2产出" || res.FinalOutput != "n2产出" {
		t.Fatalf("顺序执行产出失真: %+v", res)
	}
	// 一等节点事件：started/completed 各 2（attempt=0）
	if ce.count(EventNodeStarted) != 2 || ce.count(EventNodeCompleted) != 2 {
		t.Fatalf("节点事件计数失真: started=%d completed=%d", ce.count(EventNodeStarted), ce.count(EventNodeCompleted))
	}
}

func TestRunnerParallelAndCondition_Req563(t *testing.T) {
	// start → p1/p2 并行 → join（join all）；p1 写变量 flag=ok，条件边 var:flag==ok 控制 j2
	s := &Spec{
		Nodes: []Node{
			{ID: "start", Type: NodeLLM},
			{ID: "p1", Type: NodeLLM, VarWrites: []string{"flag"}},
			{ID: "p2", Type: NodeTool, Tool: "http_fetch"},
			{ID: "join", Type: NodeLLM, Join: JoinAll},
		},
		Edges: []Edge{
			{From: "start", To: "p1"},
			{From: "start", To: "p2"},
			{From: "p1", To: "join"},
			{From: "p2", To: "join"},
		},
		Vars: []VarSpec{{Name: "flag", Type: "string"}},
	}
	ce := &collectEvents{}
	var concurrent int32
	var mu sync.Mutex
	var maxConc int32
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on, ParallelLimit: 2,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			mu.Lock()
			concurrent++
			if concurrent > maxConc {
				maxConc = concurrent
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond) // 制造并行窗口
			if n.ID == "p1" {
				return "flag=ok", nil
			}
			return n.ID + "产出", nil
		},
		ExecTool: func(ctx context.Context, n Node, in string) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "tool产出", nil
		},
	})
	res, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if maxConc < 2 {
		t.Fatalf("同层应并行（max 并发 %d）", maxConc)
	}
	if _, ok := res.NodeOutputs["join"]; !ok {
		t.Fatalf("join 应完成: %+v", res)
	}
	// 变量写入（p1 VarWrites）
	if v, ok := res.NodeOutputs["p1"]; !ok || !strings.Contains(v, "flag=ok") {
		t.Fatalf("p1 产出失真: %v", v)
	}
	// 事件：4 节点 started/completed
	if ce.count(EventNodeStarted) != 4 || ce.count(EventNodeCompleted) != 4 {
		t.Fatalf("事件计数失真: %d/%d", ce.count(EventNodeStarted), ce.count(EventNodeCompleted))
	}
}

func TestRunnerConditionSkip_Req563(t *testing.T) {
	// 条件边不满足 → 下游 skipped（事件 wf.node.skipped）+FinalOutput 落成功末节点
	s := &Spec{
		Nodes: []Node{
			{ID: "a", Type: NodeLLM, VarWrites: []string{"flag"}},
			{ID: "b", Type: NodeLLM},
			{ID: "c", Type: NodeLLM},
		},
		Edges: []Edge{
			{From: "a", To: "b", Condition: "var:flag==yes"},
			{From: "a", To: "c"},
		},
		Vars: []VarSpec{{Name: "flag", Type: "string"}},
	}
	ce := &collectEvents{}
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			if n.ID == "a" {
				return "no", nil
			}
			return n.ID + "产出", nil
		},
	})
	// a 写 flag="no" —— 条件 var:flag==yes 不满足
	// VarWrites 把整产出写入（含 "no"）→ 条件 contains "yes" 为 false
	res, err := r.Run(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if ce.count(EventNodeSkipped) == 0 {
		t.Fatalf("条件不满足应有 skipped 事件: %+v", ce.events)
	}
	found := false
	for _, sc := range res.Skipped {
		if sc == "b" {
			found = true
		}
	}
	if !found {
		t.Fatalf("b 应被跳过: %v", res.Skipped)
	}
	if _, ok := res.NodeOutputs["c"]; !ok {
		t.Fatal("c 应正常执行")
	}
}

func TestRunnerRetryAndSkipDownstream_Req563(t *testing.T) {
	s := &Spec{
		Nodes: []Node{
			{ID: "a", Type: NodeLLM},
			{ID: "b", Type: NodeLLM},
		},
		Edges: []Edge{{From: "a", To: "b"}},
	}
	ce := &collectEvents{}
	attempts := 0
	r := NewRunner(RunnerDeps{
		Spec: s, Vars: NewVarTable(), OnEvent: ce.on,
		ExecLLM: func(ctx context.Context, n Node, in string) (string, error) {
			if n.ID == "a" {
				attempts++
				return "", context.DeadlineExceeded // 恒失败（重试 1 次后仍败）
			}
			return "ok", nil
		},
	})
	res, err := r.Run(context.Background(), "in")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != NodeRetries+1 {
		t.Fatalf("重试次数失真: %d", attempts)
	}
	// a 失败 → b skip_downstream
	found := false
	for _, f := range res.Failed {
		if f == "a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a 应失败: %v", res.Failed)
	}
	bSkipped := false
	for _, sc := range res.Skipped {
		if sc == "b" {
			bSkipped = true
		}
	}
	if !bSkipped {
		t.Fatalf("b 应 skip_downstream: %v", res.Skipped)
	}
	if ce.count(EventNodeFailed) < 2 {
		t.Fatalf("失败事件（含重试）应 ≥2: %d", ce.count(EventNodeFailed))
	}
	if ce.count(EventNodeSkipped) == 0 {
		t.Fatal("skip 事件应透出")
	}
}
