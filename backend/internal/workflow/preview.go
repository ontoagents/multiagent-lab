package workflow

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// 拓扑预览（REQ-562 WF-① 验收③——schema 预览 JSON 与计划确认卡消费形态一致〔WF-⑦ 契约
// 冻结输入〕；回放驱动零新后端可先行的预览面）。
// ---------------------------------------------------------------------------

// PreviewNode 预览节点（确认卡渲染形态——与 Spec.Node 对应+层级/并行标注）。
type PreviewNode struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Name     string   `json:"name,omitempty"`
	Depth    int      `json:"depth"`              // 拓扑深度（0=起；确认卡层级缩进/泳道布局）
	Parallel bool     `json:"parallel,omitempty"` // 与前一同深度节点互不依赖（可并行组）
	Join     string   `json:"join,omitempty"`     // 多入度汇入语义
	Writes   []string `json:"writes,omitempty"`   // 变量写（确认卡「产出」栏）
}

// PreviewEdge 预览边。
type PreviewEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"` // 条件边（确认卡「如果…就…」问句素材——WF-⑦）
}

// Preview 拓扑预览（Validate 通过后调用；Validate 未过返回错误由调用方 400）。
type Preview struct {
	Name  string        `json:"name,omitempty"`
	Nodes []PreviewNode `json:"nodes"`
	Edges []PreviewEdge `json:"edges"`
	Vars  []VarSpec     `json:"vars,omitempty"`
	Depth int           `json:"depth"` // 总深度（确认卡进度条上限）
}

// PreviewOf 校验后的拓扑预览（先 Validate——预览以合法计划为前提）。
func PreviewOf(s *Spec) (*Preview, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	order := s.TopoOrder()
	if order == nil {
		return nil, fmt.Errorf("拓扑序失败（环——Validate 应已拦）")
	}
	depth := map[string]int{}
	nodeByID := map[string]Node{}
	for _, n := range s.Nodes {
		nodeByID[n.ID] = n
	}
	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, e := range s.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	// Kahn 序同时算深度（max(上游 depth)+1）；同深度互不依赖标 parallel
	ready := []string{}
	for _, n := range s.Nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	depthCount := map[int]int{}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		depthCount[depth[id]]++
		for _, to := range adj[id] {
			if d := depth[id] + 1; d > depth[to] {
				depth[to] = d
			}
			indeg[to]--
			if indeg[to] == 0 {
				ready = append(ready, to)
			}
		}
	}
	out := &Preview{Name: s.Name, Vars: s.Vars}
	for _, id := range order {
		n := nodeByID[id]
		pn := PreviewNode{
			ID: n.ID, Type: n.Type, Name: n.Name,
			Depth: depth[id], Join: n.Join, Writes: n.VarWrites,
			Parallel: depthCount[depth[id]] > 1, // 同深度多节点=可并行组
		}
		if pn.Name == "" {
			pn.Name = n.ID
		}
		out.Nodes = append(out.Nodes, pn)
	}
	for _, e := range s.Edges {
		out.Edges = append(out.Edges, PreviewEdge{From: e.From, To: e.To, Condition: e.Condition})
	}
	out.Depth = maxDepth(depth)
	return out, nil
}

func maxDepth(m map[string]int) int {
	max := 0
	for _, d := range m {
		if d > max {
			max = d
		}
	}
	return max
}

// PreviewJSON 预览 JSON（确认卡契约冻结输入——WF-⑦ 消费形态）。
func PreviewJSON(s *Spec) ([]byte, error) {
	p, err := PreviewOf(s)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(p, "", "  ")
}
