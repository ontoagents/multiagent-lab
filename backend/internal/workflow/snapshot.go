package workflow

import (
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// REQ-565/M372 WF-④ run 级 DAG 快照+节点状态+rerun 语义（109 号 §WF-④）——
// RunSnapshot（spec+vars+节点状态）JSON 序列化（checkpoint/快照表载荷形态——gob 兼容走
// JSON 字节）；RerunFrom 节点重跑语义=该节点及其下游重置 pending、上游输出保留（验收②）；
// 冷恢复=快照重建 Runner 输入（验收①——进程重启后按快照拓扑重建续跑）。
// ---------------------------------------------------------------------------

// 节点状态枚举（节点状态表——快照行 JSON 列承载）。
const (
	StatePending   = "pending"
	StateRunning   = "running"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateSkipped   = "skipped"
	StatePaused    = "paused" // 快照级状态（run 挂起——WF-④ 冷恢复载体；非节点态）
)

// NodeState 单节点状态。
type NodeState struct {
	Status    string `json:"status"` // pending|running|completed|failed|skipped
	Attempt   int    `json:"attempt,omitempty"`
	Output    string `json:"output,omitempty"`    // completed 时产出（rerun 上游保留语义）
	Error     string `json:"error,omitempty"`     // failed 时原因
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RunSnapshot run 级快照（持久化载荷——store.workflow_run_snapshot 行 JSON 列组）。
type RunSnapshot struct {
	RunID          string                `json:"run_id"`
	ConversationID string                `json:"conversation_id,omitempty"`
	Spec           *Spec                 `json:"spec"`
	Vars           *VarTable             `json:"vars"`
	NodeStates     map[string]*NodeState `json:"node_states"`
	Status         string                `json:"status"` // running|paused|completed|failed
	CreatedAt      string                `json:"created_at"`
	UpdatedAt      string                `json:"updated_at"`
}

// NewSnapshot 由 Spec 建 initial 快照（全节点 pending）。
func NewSnapshot(runID string, s *Spec) (*RunSnapshot, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ns := map[string]*NodeState{}
	for _, n := range s.Nodes {
		ns[n.ID] = &NodeState{Status: StatePending, UpdatedAt: now}
	}
	return &RunSnapshot{RunID: runID, Spec: s, Vars: NewVarTable(), NodeStates: ns, Status: StateRunning, CreatedAt: now, UpdatedAt: now}, nil
}

// SnapshotFromRunner 执行后快照（由 NodeOutputs/Skipped/Failed 投影节点状态——Runner 结果回填）。
func SnapshotFromRunner(snap *RunSnapshot, res *RunResult) {
	now := time.Now().UTC().Format(time.RFC3339)
	for id, out := range res.NodeOutputs {
		if ns, ok := snap.NodeStates[id]; ok {
			ns.Status = StateCompleted
			ns.Output = out
			ns.UpdatedAt = now
		}
	}
	for _, id := range res.Skipped {
		if ns, ok := snap.NodeStates[id]; ok {
			ns.Status = StateSkipped
			ns.UpdatedAt = now
		}
	}
	for _, id := range res.Failed {
		if ns, ok := snap.NodeStates[id]; ok {
			ns.Status = StateFailed
			ns.UpdatedAt = now
		}
	}
	snap.UpdatedAt = now
}

// RerunFrom 节点重跑语义（验收②）：目标节点及其**全部下游**重置 pending（产出作废）、
// 上游输出保留；目标节点状态置 pending 由 Runner 重执行。返回重置节点清单（人话透出）。
// 目标节点上游未完成的先例（failed/skipped 上游）不拦截——重跑语义=以当前快照状态为准重执行。
func (snap *RunSnapshot) RerunFrom(nodeID string) ([]string, error) {
	if _, ok := snap.NodeStates[nodeID]; !ok {
		return nil, fmt.Errorf("重跑节点 %q 不在快照节点清单", nodeID)
	}
	// 下游闭包（BFS 沿边）
	adj := map[string][]string{}
	for _, e := range snap.Spec.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	reset := map[string]bool{nodeID: true}
	queue := []string{nodeID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, to := range adj[id] {
			if !reset[to] {
				reset[to] = true
				queue = append(queue, to)
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	resetList := make([]string, 0, len(reset))
	for id := range reset {
		if ns, ok := snap.NodeStates[id]; ok {
			ns.Status = StatePending
			ns.Output = ""
			ns.Error = ""
			ns.Attempt = 0
			ns.UpdatedAt = now
		}
		delete(snap.resumedOutputs(), id) // 产出作废
		resetList = append(resetList, id)
	}
	sortStrings(resetList)
	snap.UpdatedAt = now
	snap.Status = StateRunning
	return resetList, nil
}

// resumedOutputs 快照内已产出的节点集（重跑作废的对象域）。
func (snap *RunSnapshot) resumedOutputs() map[string]string {
	// Runner 重建时的输出表视图：completed 节点的 Output 即上游保留语义的载体。
	// 本方法返回可变视图的轻量代理——直接以 NodeStates.Output 为准（delete 语义由
	// NodeState.Output="" 承担，此 map 仅为接口兼容占位）。
	out := map[string]string{}
	for id, ns := range snap.NodeStates {
		if ns.Status == StateCompleted && ns.Output != "" {
			out[id] = ns.Output
		}
	}
	return out
}

// ResumeInputs 冷恢复/续跑输入（验收①）：从快照重建 Runner 输入——已完成节点的产出表
// （上游保留）、pending/failed 节点清单（待重执行）。Runner 由调用方以同 Spec 重建。
func (snap *RunSnapshot) ResumeInputs() (outputs map[string]string, pending []string) {
	outputs = map[string]string{}
	for id, ns := range snap.NodeStates {
		switch ns.Status {
		case StateCompleted:
			outputs[id] = ns.Output
		case StatePending, StateFailed:
			pending = append(pending, id)
		}
	}
	sortStrings(pending)
	return outputs, pending
}

// MarshalJSON 字节形态（快照表列/gob 载荷——JSON 兼容形态跨版本可读）。
func (snap *RunSnapshot) Marshal() ([]byte, error) {
	return json.Marshal(snap)
}

// UnmarshalSnapshot 快照反序列化（冷恢复装载）。
func UnmarshalSnapshot(b []byte) (*RunSnapshot, error) {
	var snap RunSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("快照反序列化失败: %w", err)
	}
	if snap.Spec == nil || snap.NodeStates == nil {
		return nil, fmt.Errorf("快照不完整（缺 spec 或节点状态）")
	}
	return &snap, nil
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
