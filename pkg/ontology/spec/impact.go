package spec

// REQ-278/M103 变更影响分析（66 号 §2⑤ 唯一真缺口，oo plan/blast radius 借鉴自研）：
// 两版本 spec 结构 diff + 引用闭包波及面 + 风险分级（65 号 A1 判词纪律）+ B1 去蕴含边检测
//（67 号扩注：与影响分析同属结构图分析，同轮设计免二次返工；判据=概念级关系图上
// A→C 直达边存在 A→B→C 二跳路径即疑似蕴含，measurement 口径只检测不自动删）。
// 纯函数零依赖（与 Validate/Stats 同层），qualitygate 与 rest 影响端点共用。

import "fmt"

// 变更动作与风险档（风险判词沿 65 号 A1：每条带理由，不给无解释的分数）。
const (
	ActionAdded   = "added"
	ActionRemoved = "removed"
	ActionChanged = "changed"

	RiskHigh   = "high"   // 删除仍被引用的对象（悬空必然产生）
	RiskMedium = "medium" // 结构波及（父概念变更波及子树/关系端点改写需人工核对）
	RiskLow    = "low"    // 新增或无引用对象的删除（无波及）
)

// ImpactItem 单条影响记录。
type ImpactItem struct {
	Kind    string   `json:"kind"`              // concept | relation | instance | data_property | axiom
	Name    string   `json:"name"`              // 对象名（身份键）
	Action  string   `json:"action"`            // added | removed | changed
	Risk    string   `json:"risk"`              // high | medium | low
	Reasons []string `json:"reasons,omitempty"` // 判词（引用者清单/波及路径）
}

// ImpactChange 变更集统计。
type ImpactChange struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
	Changed []string `json:"changed,omitempty"`
}

// ImpactReport 影响分析报告。
type ImpactReport struct {
	Changes map[string]ImpactChange `json:"changes"` // kind → 变更集
	Blast   []ImpactItem            `json:"blast"`   // 波及面（变更对象本体 + 闭包受波及对象）
	Implied []string                `json:"implied"` // B1 疑似蕴含边（"关系X（A→C）：已存在 A→B→C 路径"）
	Summary struct {
		High   int `json:"high"`
		Medium int `json:"medium"`
		Low    int `json:"low"`
	} `json:"summary"`
}

// AnalyzeImpact 比较 from→to 两版本 spec，产出变更集、引用闭包波及面与蕴含边清单。
// 波及面口径（诚实边界）：结构级非语义级——①变更对象本体；②被删/被改概念的全部直接
// 引用者（关系端点/实例归属/子概念 parents/数据属性 domain/公理）标记 high；
// ③直接引用者沿概念图（parents 反向=子树）再扩一层标记 medium（v1 不做全量传递闭包）。
func AnalyzeImpact(from, to *Spec) *ImpactReport {
	rep := &ImpactReport{Changes: map[string]ImpactChange{}, Blast: []ImpactItem{}, Implied: []string{}}
	addChange := func(kind, action, name string) {
		c := rep.Changes[kind]
		switch action {
		case ActionAdded:
			c.Added = append(c.Added, name)
		case ActionRemoved:
			c.Removed = append(c.Removed, name)
		case ActionChanged:
			c.Changed = append(c.Changed, name)
		}
		rep.Changes[kind] = c
	}

	// ---- 概念 ----
	fromC, toC := conceptMap(from), conceptMap(to)
	for n, c := range fromC {
		nc, ok := toC[n]
		if !ok {
			addChange("concept", ActionRemoved, n)
			continue
		}
		if c.Definition != nc.Definition || c.Label != nc.Label || !stringSlicesEqual(c.Parents, nc.Parents) {
			addChange("concept", ActionChanged, n)
		}
	}
	for n := range toC {
		if _, ok := fromC[n]; !ok {
			addChange("concept", ActionAdded, n)
		}
	}
	// ---- 关系 ----
	fromR, toR := relationMap(from), relationMap(to)
	for n, r := range fromR {
		nr, ok := toR[n]
		if !ok {
			addChange("relation", ActionRemoved, n)
			continue
		}
		if r != nr {
			addChange("relation", ActionChanged, n)
		}
	}
	for n := range toR {
		if _, ok := fromR[n]; !ok {
			addChange("relation", ActionAdded, n)
		}
	}
	// ---- 实例 / 数据属性 / 公理 ----
	fromI, toI := instanceMap(from), instanceMap(to)
	for n := range fromI {
		if _, ok := toI[n]; !ok {
			addChange("instance", ActionRemoved, n)
		}
	}
	for n := range toI {
		if _, ok := fromI[n]; !ok {
			addChange("instance", ActionAdded, n)
		}
	}
	fromD, toD := dataPropMap(from), dataPropMap(to)
	for n, dp := range fromD {
		nd, ok := toD[n]
		if !ok {
			addChange("data_property", ActionRemoved, n)
			continue
		}
		if dp != nd {
			addChange("data_property", ActionChanged, n)
		}
	}
	for n := range toD {
		if _, ok := fromD[n]; !ok {
			addChange("data_property", ActionAdded, n)
		}
	}
	fromA, toA := axiomKeyMap(from), axiomKeyMap(to)
	for k := range fromA {
		if _, ok := toA[k]; !ok {
			addChange("axiom", ActionRemoved, k)
		}
	}
	for k := range toA {
		if _, ok := fromA[k]; !ok {
			addChange("axiom", ActionAdded, k)
		}
	}

	// ---- 波及面：以「被删/被改概念」为源的引用闭包（结构级）----
	affected := map[string]bool{}
	for _, n := range append(append([]string{}, rep.Changes["concept"].Removed...), rep.Changes["concept"].Changed...) {
		affected[n] = true
	}
	if len(affected) > 0 {
		removedOnly := map[string]bool{}
		for _, n := range rep.Changes["concept"].Removed {
			removedOnly[n] = true
		}
		// 反向引用索引双份：FROM=旧版引用者（波及面基准——合法保存流中引用会同步清理，
		// 但操作者仍需看到本次删除/修改波及过哪些引用）；TO=新版仍引用（悬空告警）。
		refByFrom := map[string][]string{}
		collectRefs := func(sp *Spec, into map[string][]string) {
			for _, r := range sp.Relations {
				into[r.From] = append(into[r.From], fmt.Sprintf("关系「%s」定义域", r.Name))
				into[r.To] = append(into[r.To], fmt.Sprintf("关系「%s」值域", r.Name))
			}
			for _, it := range sp.Instances {
				into[it.Concept] = append(into[it.Concept], fmt.Sprintf("实例「%s」", it.Name))
			}
			for _, c := range sp.Concepts {
				for _, p := range c.Parents {
					into[p] = append(into[p], fmt.Sprintf("子概念「%s」", c.Name))
				}
			}
			for _, dp := range sp.DataProperties {
				if dp.Domain != "" {
					into[dp.Domain] = append(into[dp.Domain], fmt.Sprintf("数据属性「%s」", dp.Name))
				}
			}
			for _, ax := range sp.Axioms {
				into[ax.Subject] = append(into[ax.Subject], fmt.Sprintf("公理（%s）", ax.Type))
			}
		}
		collectRefs(from, refByFrom)
		refByTo := map[string][]string{}
		collectRefs(to, refByTo)
		seen := map[string]bool{}
		for n := range affected {
			dangling := len(refByTo[n])
			for _, desc := range refByFrom[n] {
				risk := RiskMedium
				action := ActionChanged
				reason := desc + "——定义/层级变更，依赖方需人工核对"
				if removedOnly[n] {
					risk = RiskHigh
					action = ActionRemoved
					if dangling > 0 {
						reason = desc + "——已删除且新版仍存在引用（悬空，请立即处理）"
					} else {
						reason = desc + "——本次删除波及的引用（新版已同步处理则为迁移完成）"
					}
				}
				rep.Blast = append(rep.Blast, ImpactItem{Kind: "concept", Name: n, Action: action, Risk: risk, Reasons: []string{reason}})
			}
			if len(refByFrom[n]) == 0 && dangling == 0 {
				if _, wasRemoved := rep.Changes["concept"]; wasRemoved {
					rep.Blast = append(rep.Blast, ImpactItem{Kind: "concept", Name: n, Risk: RiskLow, Reasons: []string{"无其他对象引用（波及面仅自身定义）"}})
				}
			}
			// 子树一层波及（medium）：直接子概念的子概念（v1 诚实边界=一层非全量闭包）
			for _, c := range to.Concepts {
				if containsString(c.Parents, n) {
					for _, gc := range to.Concepts {
						if gc.Name != c.Name && containsString(gc.Parents, c.Name) && !seen[gc.Name] {
							seen[gc.Name] = true
							rep.Blast = append(rep.Blast, ImpactItem{Kind: "concept", Name: gc.Name, Action: ActionChanged, Risk: RiskMedium,
								Reasons: []string{fmt.Sprintf("父概念「%s」变更波及子树（「%s」→「%s」）", n, n, c.Name)}})
						}
					}
				}
			}
		}
	}
	for _, it := range rep.Blast {
		switch it.Risk {
		case RiskHigh:
			rep.Summary.High++
		case RiskMedium:
			rep.Summary.Medium++
		case RiskLow:
			rep.Summary.Low++
		}
	}

	// ---- B1 疑似蕴含边（TO spec 概念级关系图；A→C 存在 A→B→C 二跳路径即标注）----
	adj := map[string]map[string]bool{}
	for _, r := range to.Relations {
		if adj[r.From] == nil {
			adj[r.From] = map[string]bool{}
		}
		adj[r.From][r.To] = true
	}
	for _, r := range to.Relations {
		for mid := range adj[r.From] {
			if mid == r.From || mid == r.To {
				continue
			}
			if adj[mid][r.To] {
				rep.Implied = append(rep.Implied, fmt.Sprintf("关系「%s」（%s→%s）疑似蕴含：已存在 %s→%s→%s 路径（直达边可留给遍历，measurement 口径仅检测不自动删）",
					r.Name, r.From, r.To, r.From, mid, r.To))
				break
			}
		}
	}
	sortImpact(rep)
	return rep
}

func sortImpact(rep *ImpactReport) {
	// 稳定呈现：high 在前，同险按 Kind+Name
	for i := 1; i < len(rep.Blast); i++ {
		for j := i; j > 0 && blastLess(rep.Blast[j], rep.Blast[j-1]); j-- {
			rep.Blast[j], rep.Blast[j-1] = rep.Blast[j-1], rep.Blast[j]
		}
	}
}

func blastLess(a, b ImpactItem) bool {
	rank := map[string]int{RiskHigh: 0, RiskMedium: 1, RiskLow: 2}
	if rank[a.Risk] != rank[b.Risk] {
		return rank[a.Risk] < rank[b.Risk]
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.ReasonsString() < b.ReasonsString()
}

func conceptMap(s *Spec) map[string]Concept {
	m := map[string]Concept{}
	for _, c := range s.Concepts {
		m[c.Name] = c
	}
	return m
}

func relationMap(s *Spec) map[string]Relation {
	m := map[string]Relation{}
	for _, r := range s.Relations {
		m[r.Name] = r
	}
	return m
}

func instanceMap(s *Spec) map[string]bool {
	m := map[string]bool{}
	for _, it := range s.Instances {
		m[it.Name] = true
	}
	return m
}

func dataPropMap(s *Spec) map[string]DataProperty {
	m := map[string]DataProperty{}
	for _, dp := range s.DataProperties {
		m[dp.Name] = dp
	}
	return m
}

func axiomKeyMap(s *Spec) map[string]bool {
	m := map[string]bool{}
	for _, ax := range s.Axioms {
		m[ax.Type+"|"+ax.Subject] = true
	}
	return m
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	bm := map[string]int{}
	for _, x := range b {
		bm[x]++
	}
	for _, x := range a {
		if bm[x] == 0 {
			return false
		}
		bm[x]--
	}
	return true
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ReasonsString 判词拼接（排序比较用）。
func (i ImpactItem) ReasonsString() string {
	out := ""
	for _, r := range i.Reasons {
		out += r + ";"
	}
	return out
}
