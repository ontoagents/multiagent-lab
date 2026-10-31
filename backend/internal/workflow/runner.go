package workflow

// ---------------------------------------------------------------------------
// REQ-563/M370 WF-② DAG Runner 执行引擎（109 号 §WF-② 切粒度——最重件）：
// 自研 Runner 消费 WF-① Spec（不触存量五模式装配链）：顺序层（TopoOrder 深度分层）+
// 层内并行（ParallelLimit 上限）+条件边（v1 语法 var:name==value 对 VarTable 判定）+
// **一等节点事件**（wf.node.started/completed/failed/skipped——node_id/status/phase/attempt，
// event_names.go 单源注册）+失败策略（重试 1 次→skip_downstream 默认——107 号 §三 素材）+
// 变量写入（VarTable 双限——WF-①）。
// LLM/工具执行体经 Executor 函数注入（chat 装配接线在 WF-⑥ 白名单新值后——本件库层完整可测；
// -race 绿为验收②）。
// ---------------------------------------------------------------------------

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// 节点事件（一等节点事件——113 号归一轮口径：node_id/status/phase/attempt 单源注册）。
const (
	EventNodeStarted   = "wf.node.started"
	EventNodeCompleted = "wf.node.completed"
	EventNodeFailed    = "wf.node.failed"
	EventNodeSkipped   = "wf.node.skipped"
)

// DefaultParallelLimit 层内并行默认上限（107 号 §三 并行度上限 4 素材）。
const DefaultParallelLimit = 4

// NodeRetries 节点失败重试次数（107 号 §三「重试 1 次」素材）。
const NodeRetries = 1

// Executor 节点执行体（chat 装配层注入——LLM/工具各一；input=上游产出拼接）。
type Executor func(ctx context.Context, node Node, input string) (string, error)

// RunnerDeps Runner 依赖注入。
type RunnerDeps struct {
	Spec          *Spec
	Vars          *VarTable
	ExecLLM       Executor // llm/delegate/gate 节点执行体（gate 断言语义 WF-③ 前由 LLM 执行体承载）
	ExecTool      Executor // tool 节点执行体
	OnEvent       func(eventType string, data map[string]any)
	ParallelLimit int // 0=DefaultParallelLimit
}

// Runner DAG 执行引擎。
type Runner struct {
	d RunnerDeps
}

// NewRunner 构造（Spec 须已 Validate——调用方先过校验）。
func NewRunner(d RunnerDeps) *Runner {
	if d.ParallelLimit <= 0 {
		d.ParallelLimit = DefaultParallelLimit
	}
	return &Runner{d: d}
}

// RunResult 执行结果（节点产出与终态）。
type RunResult struct {
	NodeOutputs map[string]string `json:"node_outputs"` // 节点 id → 产出
	Skipped     []string          `json:"skipped,omitempty"`
	Failed      []string          `json:"failed,omitempty"`
	FinalOutput string            `json:"final_output,omitempty"` // 汇合末节点产出（拓扑序最后一个成功节点）
}

// emit 节点事件透出（nil 安全）。
func (r *Runner) emit(typ string, nodeID, status, phase string, attempt int, extra map[string]any) {
	if r.d.OnEvent == nil {
		return
	}
	data := map[string]any{
		"node_id": nodeID, "status": status, "phase": phase, "attempt": attempt,
	}
	for k, v := range extra {
		data[k] = v
	}
	r.d.OnEvent(typ, data)
}

// Run 执行 DAG（顺序层+层内并行+条件边+失败 skip_downstream）。
func (r *Runner) Run(ctx context.Context, input string) (*RunResult, error) {
	if err := r.d.Spec.Validate(); err != nil {
		return nil, err
	}
	res := &RunResult{NodeOutputs: map[string]string{}}
	outputs := map[string]string{"__input__": input}
	skipped := map[string]bool{}
	failed := map[string]bool{}
	done := map[string]bool{}
	skipEmitted := map[string]bool{}

	// 深度分层（WF-① PreviewOf 同口径 Kahn 深度）
	layers := r.layers()
	for depth, layer := range layers {
		// 层内就绪节点：过滤条件边不满足的（skip 传播）
		var ready []Node
		for _, n := range layer {
			if skipped[n.ID] || failed[n.ID] {
				// 级联跳过事件（propagateSkips 标记的下游——skip_downstream 传播可观测）
				if !skipEmitted[n.ID] {
					skipEmitted[n.ID] = true
					r.emit(EventNodeSkipped, n.ID, "skipped", "skipped", 0, map[string]any{
						"reason": "上游失败/跳过传播（skip_downstream 默认策略）", "depth": depth,
					})
				}
				continue
			}
			if !r.conditionsMet(n, outputs, skipped, failed) {
				skipped[n.ID] = true
				skipEmitted[n.ID] = true
				r.emit(EventNodeSkipped, n.ID, "skipped", "skipped", 0, map[string]any{
					"reason": "条件边不满足或上游跳过/失败传播", "depth": depth,
				})
				continue
			}
			ready = append(ready, n)
		}
		// 层内并行（ParallelLimit 上限——信号量；join all 语义在层内天然成立：全层完成才进下层；
		// join any=任一上游成功即就绪——按深度分层下 any 提前就绪属 WF-② v1 简化：仍层内等待，
		// 如实标注 JoinAny 提前调度留 Runner 演进）
		var wg sync.WaitGroup
		sem := make(chan struct{}, r.d.ParallelLimit)
		var mu sync.Mutex
		for _, n := range ready {
			wg.Add(1)
			go func(n Node) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				in := r.joinInput(n, outputs, input)
				out, err := r.execNode(ctx, n, in)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed[n.ID] = true
					res.Failed = append(res.Failed, n.ID)
					return
				}
				outputs[n.ID] = out
				done[n.ID] = true
				res.NodeOutputs[n.ID] = out
				r.writeVars(n, out)
			}(n)
		}
		wg.Wait()
		// skip 传播（下游级联——本层失败/跳过节点的下游在下一层 conditionsMet 拦截）
		_ = propagateSkips(r.d.Spec, skipped, failed)
	}
	// 最终产出：拓扑序最后一个成功节点
	if order := r.d.Spec.TopoOrder(); order != nil {
		for i := len(order) - 1; i >= 0; i-- {
			if out, ok := outputs[order[i]]; ok {
				res.FinalOutput = out
				break
			}
		}
	}
	res.Skipped = sortedKeys(skipped)
	res.Failed = sortedKeys(failed)
	return res, nil
}

// execNode 单节点执行（重试 NodeRetries 次——attempt 计数入事件）。
func (r *Runner) execNode(ctx context.Context, n Node, in string) (string, error) {
	var exec Executor
	switch n.Type {
	case NodeTool:
		exec = r.d.ExecTool
	default:
		exec = r.d.ExecLLM
	}
	if exec == nil {
		r.emit(EventNodeFailed, n.ID, "failed", "error", 0, map[string]any{"error": "无执行体（装配未注入）"})
		return "", fmt.Errorf("节点 %s 无执行体", n.ID)
	}
	var lastErr error
	for attempt := 0; attempt <= NodeRetries; attempt++ {
		r.emit(EventNodeStarted, n.ID, "running", "started", attempt, map[string]any{"type": n.Type})
		out, err := exec(ctx, n, in)
		if err == nil {
			r.emit(EventNodeCompleted, n.ID, "completed", "completed", attempt, map[string]any{
				"type": n.Type, "output_bytes": len(out),
			})
			return out, nil
		}
		lastErr = err
		r.emit(EventNodeFailed, n.ID, "failed", "error", attempt, map[string]any{
			"type": n.Type, "error": err.Error(), "will_retry": attempt < NodeRetries,
		})
	}
	return "", lastErr
}

// conditionsMet 条件边判定（v1 语法 var:name==value 对 VarTable；无条件边恒真；
// 上游失败/跳过 → false〔skip_downstream 默认——107 号 §三〕）。
func (r *Runner) conditionsMet(n Node, outputs map[string]string, skipped, failed map[string]bool) bool {
	for _, e := range r.d.Spec.Edges {
		if e.To != n.ID {
			continue
		}
		if failed[e.From] || skipped[e.From] {
			return false // skip_downstream 默认失败策略
		}
		if e.Condition == "" {
			continue
		}
		if !r.evalCondition(e.Condition) {
			return false
		}
	}
	return true
}

// evalCondition 条件求值（v1：var:name==value——VarTable containment；语法不符=不满足如实）。
func (r *Runner) evalCondition(cond string) bool {
	c := strings.TrimSpace(cond)
	if !strings.HasPrefix(c, "var:") {
		return false
	}
	rest := strings.TrimPrefix(c, "var:")
	eq := strings.Index(rest, "==")
	if eq <= 0 {
		return false
	}
	name := strings.TrimSpace(rest[:eq])
	want := strings.TrimSpace(rest[eq+2:])
	got, ok := r.d.Vars.Get(name)
	if !ok {
		return false
	}
	return strings.Contains(string(got), want)
}

// joinInput 上游产出拼接（多入度按声明序 join；单入度=该上游产出；起头=原始输入）。
func (r *Runner) joinInput(n Node, outputs map[string]string, input string) string {
	var ups []string
	for _, e := range r.d.Spec.Edges {
		if e.To == n.ID {
			ups = append(ups, e.From)
		}
	}
	if len(ups) == 0 {
		return input
	}
	sort.Strings(ups) // 声明序确定性
	parts := make([]string, 0, len(ups))
	for _, u := range ups {
		if out, ok := outputs[u]; ok {
			parts = append(parts, out)
		}
	}
	return strings.Join(parts, "\n\n")
}

// writeVars 节点产出写声明变量（VarWrites——截断注记经事件透出）。
func (r *Runner) writeVars(n Node, out string) {
	for _, w := range n.VarWrites {
		if note, err := r.d.Vars.Set(r.d.Spec, w, n.ID, out); err != nil {
			r.emit(EventNodeFailed, n.ID, "warning", "var_write", 0, map[string]any{"error": err.Error()})
		} else if note != "" {
			r.emit(EventNodeFailed, n.ID, "warning", "var_truncated", 0, map[string]any{"note": note})
		}
	}
}

// layers 深度分层（ Kahn 深度——同层可并行）。
func (r *Runner) layers() [][]Node {
	indeg := map[string]int{}
	adj := map[string][]string{}
	byID := map[string]Node{}
	for _, n := range r.d.Spec.Nodes {
		byID[n.ID] = n
		indeg[n.ID] = 0
	}
	for _, e := range r.d.Spec.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	depth := map[string]int{}
	ready := []string{}
	for _, n := range r.d.Spec.Nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	maxDepth := 0
	for len(ready) > 0 {
		var next []string
		for _, id := range ready {
			for _, to := range adj[id] {
				if depth[id]+1 > depth[to] {
					depth[to] = depth[id] + 1
				}
				if depth[to] > maxDepth {
					maxDepth = depth[to]
				}
				indeg[to]--
				if indeg[to] == 0 {
					next = append(next, to)
				}
			}
		}
		ready = next
	}
	out := make([][]Node, maxDepth+1)
	for _, n := range r.d.Spec.Nodes {
		d := depth[n.ID]
		out[d] = append(out[d], n)
	}
	return out
}

// propagateSkips skip 级联传播（失败/跳过节点的下游递归标记——不动 done）。
func propagateSkips(s *Spec, skipped, failed map[string]bool) bool {
	changed := false
	adj := map[string][]string{}
	for _, e := range s.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for {
		iter := false
		for from, tos := range adj {
			if !failed[from] && !skipped[from] {
				continue
			}
			for _, to := range tos {
				if !skipped[to] && !failed[to] {
					skipped[to] = true
					changed = true
					iter = true
				}
			}
		}
		if !iter {
			break
		}
	}
	return changed
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
