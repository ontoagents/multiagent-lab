// Package workflow REQ-562/M369 WF-① DAG schema+校验闭环+变量表地基（动态工作流编排批链首；
// 109 号 §WF-① 切粒度）——schema（节点类型化/条件边/join 语义）+Validate 校验闭环四项（环/
// 悬空边/join 缺条件/变量类型不匹配，逐类人话报错）+变量表 v1（checkpoint 载荷承载——106 号
// :31 口径：无新表，VarTable JSON 可序列化随 checkpoint gob 走，WF-② Runner 消费）+拓扑预览
// （WF-⑦ 计划确认卡契约冻结输入）。本包不触存量五模式装配链（109 号改动面约束）。
package workflow

import (
	"fmt"
	"strings"
)

// 节点类型（109 号 WF-②：LLM/工具节点+委派型节点〔REQ-552 改道件 M377+ 扩展位〕+门禁 WF-③）。
const (
	NodeLLM      = "llm"      // LLM 节点（instruction 驱动）
	NodeTool     = "tool"     // 工具节点（直接调已装配工具）
	NodeGate     = "gate"     // 门禁断言节点（WF-③；v1 schema 占位校验）
	NodeDelegate = "delegate" // 委派型节点（子会话后端形态——REQ-552 改道件挂 M377+；v1 校验占位）
)

// 合法节点类型白名单。
var ValidNodeTypes = []string{NodeLLM, NodeTool, NodeGate, NodeDelegate}

// 合法变量类型白名单（变量类型不匹配校验的值域）。
var ValidVarTypes = []string{"string", "number", "bool", "json"}

// Join 语义（入度 ≥2 的节点必须显式声明——「join 缺条件」校验对象）。
const (
	JoinAll = "all" // 全上游完成才就绪（默认语义；多入度须显式写）
	JoinAny = "any" // 任一上游完成即就绪
)

// Node DAG 节点（类型化）。
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	// LLM 节点指令（NodeLLM 时建议非空——校验按 warning 不阻断）
	Instruction string `json:"instruction,omitempty"`
	// ToolNode 工具名（NodeTool 时必填）
	Tool string `json:"tool,omitempty"`
	// Join join 语义（入度≥2 必填 all|any；单入度可空=默认 all）
	Join string `json:"join,omitempty"`
	// VarWrites 本节点写入的变量名清单（须在 Spec.Vars 声明——悬空写校验）
	VarWrites []string `json:"var_writes,omitempty"`
	// Condition 条件边判定用节点输出标记（门禁/条件边 v1 占位；WF-③ 消费）
	Condition string `json:"condition,omitempty"`
}

// Edge 有向边（条件边=Condition 非空）。
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Condition 条件表达式（v1=节点输出标记等值形态 "var:name==value" 语法占位——Runner 侧消费；
	// 空=无条件边恒走）
	Condition string `json:"condition,omitempty"`
}

// VarSpec 变量声明（类型+上限）。
type VarSpec struct {
	Name string `json:"name"`
	Type string `json:"type"` // string|number|bool|json（ValidVarTypes）
	// MaxSize 单变量字节上限覆写（0=默认 VarMaxSize；超限写入截断并出注记——验收②）
	MaxSize int `json:"max_size,omitempty"`
	Desc    string `json:"desc,omitempty"`
}

// Spec DAG 计划 schema（WF-① 交付面；Runner=WF-② 消费本结构执行）。
type Spec struct {
	Name  string    `json:"name,omitempty"`
	Nodes []Node    `json:"nodes"`
	Edges []Edge    `json:"edges,omitempty"`
	Vars  []VarSpec `json:"vars,omitempty"`
}

// 默认上限（锚点量级对照 REQ-500 候选上限口径——109 号护栏「单变量 size cap/总量 cap」）。
const (
	VarMaxSize      = 8 * 1024        // 单变量默认 8KB
	VarTotalMaxSize = 64 * 1024       // 变量表总量 64KB
	MaxNodes        = 64              // 节点数上限（防计划爆炸）
	MaxEdges        = 256             // 边数上限
)

// Validate 校验闭环四项（109 号验收句①——逐类人话报错；返回首个错误即 400 透传形态）。
func (s *Spec) Validate() error {
	if s == nil {
		return fmt.Errorf("DAG 计划为空")
	}
	if len(s.Nodes) == 0 {
		return fmt.Errorf("DAG 计划无节点（nodes 为空）——至少需要一个节点")
	}
	if len(s.Nodes) > MaxNodes {
		return fmt.Errorf("节点数 %d 超上限 %d（防计划爆炸）", len(s.Nodes), MaxNodes)
	}
	if len(s.Edges) > MaxEdges {
		return fmt.Errorf("边数 %d 超上限 %d", len(s.Edges), MaxEdges)
	}
	// ① 节点：id 唯一/类型合法
	ids := map[string]bool{}
	for _, n := range s.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return fmt.Errorf("存在无 id 节点——每个节点须有唯一 id")
		}
		if ids[n.ID] {
			return fmt.Errorf("节点 id 重复：%s", n.ID)
		}
		ids[n.ID] = true
		if !validNodeType(n.Type) {
			return fmt.Errorf("节点 %s 类型 %q 非法（须 %s）", n.ID, n.Type, strings.Join(ValidNodeTypes, "|"))
		}
		if n.Type == NodeTool && strings.TrimSpace(n.Tool) == "" {
			return fmt.Errorf("工具节点 %s 缺 tool 字段（须指定已装配工具名）", n.ID)
		}
		if n.Join != "" && n.Join != JoinAll && n.Join != JoinAny {
			return fmt.Errorf("节点 %s join 语义 %q 非法（须 all|any）", n.ID, n.Join)
		}
	}
	// ② 边：端点存在（悬空边）
	indeg := map[string]int{}
	for _, e := range s.Edges {
		if !ids[e.From] {
			return fmt.Errorf("悬空边：from %q 不在节点清单（边 %s→%s）", e.From, e.From, e.To)
		}
		if !ids[e.To] {
			return fmt.Errorf("悬空边：to %q 不在节点清单（边 %s→%s）", e.To, e.From, e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("自环边 %s→%s（环校验外显式拒绝）", e.From, e.To)
		}
		indeg[e.To]++
	}
	// ③ join 缺条件：入度≥2 的节点须显式 join 语义
	for _, n := range s.Nodes {
		if indeg[n.ID] >= 2 && n.Join == "" {
			return fmt.Errorf("节点 %s 有 %d 条入边但未声明 join 语义（须 all|any）——多上游汇入须显式声明完成判定", n.ID, indeg[n.ID])
		}
	}
	// ④ 环（DFS 三色着色——107 号 §三 无环校验素材）
	if cyc := findCycle(s.Nodes, s.Edges); cyc != "" {
		return fmt.Errorf("计划含环：%s——DAG 不允许循环依赖", cyc)
	}
	// ⑤ 变量声明类型合法 + 节点写悬空变量（变量类型不匹配类）
	vars := map[string]string{}
	for _, v := range s.Vars {
		if strings.TrimSpace(v.Name) == "" {
			return fmt.Errorf("存在无名变量声明")
		}
		if !validVarType(v.Type) {
			return fmt.Errorf("变量 %s 类型 %q 非法（须 %s）", v.Name, v.Type, strings.Join(ValidVarTypes, "|"))
		}
		vars[v.Name] = v.Type
	}
	for _, n := range s.Nodes {
		for _, w := range n.VarWrites {
			if _, ok := vars[w]; !ok {
				return fmt.Errorf("节点 %s 写未声明变量 %q——先在 vars 中声明（变量类型不匹配防护）", n.ID, w)
			}
		}
	}
	return nil
}

func validNodeType(t string) bool {
	for _, v := range ValidNodeTypes {
		if v == t {
			return true
		}
	}
	return false
}

func validVarType(t string) bool {
	for _, v := range ValidVarTypes {
		if v == t {
			return true
		}
	}
	return false
}

// findCycle DFS 三色着色找环（返回环路径人话形态；无环空串）。107 号 §三 无环校验素材的
// 自研实现（backend 零现成——109 号亲证）。
func findCycle(nodes []Node, edges []Edge) string {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	const (
		white = 0 // 未访
		gray  = 1 // 在栈（当前 DFS 路径）
		black = 2 // 完成
	)
	color := map[string]int{}
	var path []string
	var cyc string
	var dfs func(id string) bool // true=发现环
	dfs = func(id string) bool {
		color[id] = gray
		path = append(path, id)
		for _, to := range adj[id] {
			switch color[to] {
			case gray:
				// 环：从栈中 to 的位置截到当前
				start := 0
				for i, p := range path {
					if p == to {
						start = i
						break
					}
				}
				cyc = strings.Join(append(append([]string{}, path[start:]...), to), " → ")
				return true
			case white:
				if dfs(to) {
					return true
				}
			}
		}
		path = path[:len(path)-1]
		color[id] = black
		return false
	}
	for _, n := range nodes {
		if color[n.ID] == white {
			if dfs(n.ID) {
				return cyc
			}
		}
	}
	return ""
}

// TopoOrder 拓扑序（Kahn——WF-② Runner 顺序层执行序；环返回 nil——调用方先 Validate）。
func (s *Spec) TopoOrder() []string {
	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, n := range s.Nodes {
		indeg[n.ID] += 0
	}
	for _, e := range s.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	var order []string
	// 就绪队列按声明序（确定性）
	ready := []string{}
	for _, n := range s.Nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, to := range adj[id] {
			indeg[to]--
			if indeg[to] == 0 {
				ready = append(ready, to)
			}
		}
	}
	if len(order) != len(s.Nodes) {
		return nil // 环（Validate 已拦——防御性）
	}
	return order
}
