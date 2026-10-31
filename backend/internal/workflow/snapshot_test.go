package workflow

// REQ-565/M372 WF-④ 快照/rerun/冷恢复单测（109 号验收①②③钉桩）：
// rerun 节点+下游重置 pending 上游保留/ResumeInputs 冷恢复重建/快照序列化往返。
import (
	"strings"
	"testing"
)

func snapSpec() *Spec {
	return &Spec{
		Nodes: []Node{
			{ID: "a", Type: NodeLLM},
			{ID: "b", Type: NodeLLM},
			{ID: "c", Type: NodeLLM},
			{ID: "d", Type: NodeLLM},
		},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}},
	}
}

func TestSnapshotRerun_Req565(t *testing.T) {
	snap, err := NewSnapshot("run-1", snapSpec())
	if err != nil {
		t.Fatal(err)
	}
	// 模拟执行完成：a/b/c/d 全 completed 带产出
	for id, out := range map[string]string{"a": "A产", "b": "B产", "c": "C产", "d": "D产"} {
		ns := snap.NodeStates[id]
		ns.Status = StateCompleted
		ns.Output = out
	}
	// 验收②：rerun b → b/c/d 重置 pending（下游作废）、a 保留 completed
	reset, err := snap.RerunFrom("b")
	if err != nil {
		t.Fatal(err)
	}
	if len(reset) != 3 {
		t.Fatalf("重置清单应 b/c/d 三节点: %v", reset)
	}
	if snap.NodeStates["a"].Status != StateCompleted || snap.NodeStates["a"].Output != "A产" {
		t.Fatalf("上游 a 应保留: %+v", snap.NodeStates["a"])
	}
	for _, id := range []string{"b", "c", "d"} {
		ns := snap.NodeStates[id]
		if ns.Status != StatePending || ns.Output != "" {
			t.Fatalf("%s 应重置 pending 清产出: %+v", id, ns)
		}
	}
	// 未知节点
	if _, err := snap.RerunFrom("ghost"); err == nil {
		t.Fatal("未知节点应报错")
	}
}

func TestSnapshotResumeInputs_Req565(t *testing.T) {
	snap, _ := NewSnapshot("run-2", snapSpec())
	snap.NodeStates["a"].Status = StateCompleted
	snap.NodeStates["a"].Output = "A产"
	snap.NodeStates["b"].Status = StateFailed
	snap.NodeStates["b"].Error = "模型超时"
	// c/d 保持 pending
	outputs, pending := snap.ResumeInputs()
	if outputs["a"] != "A产" {
		t.Fatalf("已完成产出应保留: %v", outputs)
	}
	if len(pending) != 3 {
		t.Fatalf("pending+failed 应入待执行清单: %v", pending)
	}
	if pending[0] != "b" {
		t.Fatalf("待执行清单应含 b: %v", pending)
	}
}

func TestSnapshotMarshalRoundTrip_Req565(t *testing.T) {
	snap, _ := NewSnapshot("run-3", snapSpec())
	snap.ConversationID = "conv-9"
	snap.NodeStates["a"].Status = StateCompleted
	snap.NodeStates["a"].Output = "A产"
	snap.Status = StatePaused
	b, err := snap.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.RunID != "run-3" || back.ConversationID != "conv-9" || back.Status != StatePaused {
		t.Fatalf("往返失真: %+v", back)
	}
	if back.NodeStates["a"].Output != "A产" {
		t.Fatalf("节点状态往返失真: %+v", back.NodeStates["a"])
	}
	// 残缺快照拒绝（兼容边界——109 验收③ run.warning 形态的库层等价）
	if _, err := UnmarshalSnapshot([]byte(`{"run_id":"x"}`)); err == nil {
		t.Fatal("残缺快照应报错（调用方 run.warning 留痕）")
	}
	if _, err := UnmarshalSnapshot([]byte(`not json`)); err == nil || !strings.Contains(err.Error(), "反序列化") {
		t.Fatalf("坏 JSON 人话: %v", err)
	}
}
