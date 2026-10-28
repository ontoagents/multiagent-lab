package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// ---------------------------------------------------------------------------
// REQ-157/M-O15 导入合并底座（docs/23 启示 6「增量消歧而非一次性重建」+ OrionBelt 模式）：
// 外部本体入库前的字段级冲突审查与合并策略——
//   replace          冲突实体以导入版整体替换（导入版优先）
//   merge-overwrite  冲突实体字段级覆盖（导入版非空字段覆盖现行，保留现行独有字段）
//   merge            冲突实体保留现行，导入实体重命名并入（前缀自动协调，增量消歧）
// 复杂语义合并明确不做（26 号方案 §8-3：三选项 + 字段级预览，按误报率迭代）。
// 纯函数便于零依赖单测；预览与应用共用同一构建逻辑（审查向导所见即所得）。
// ---------------------------------------------------------------------------

const (
	StrategyReplace        = "replace"
	StrategyMergeOverwrite = "merge-overwrite"
	StrategyMerge          = "merge"
	DefaultConflictPrefix  = "ext"
)

// MergeConflict 单条实体冲突（字段级）。
type MergeConflict struct {
	Kind       string   `json:"kind"`                  // concept | relation | instance
	Name       string   `json:"name"`                  // 冲突键（实体名；IRI 由名派生，同名即同 IRI）
	Fields     []string `json:"fields"`                // 取值有差异的字段
	Incoming   string   `json:"incoming"`              // 导入版摘要（JSON）
	Current    string   `json:"current"`               // 现行版摘要（JSON）
	Resolution string   `json:"resolution"`            // 本策略下的处置：replaced | field-merged | renamed
	ResolvedAs string   `json:"resolved_as,omitempty"` // renamed 处置后的实体名
}

// MergePreview 合并预览（审查向导数据；Apply 复用同一构建）。
type MergePreview struct {
	Strategy   string          `json:"strategy"`
	Prefix     string          `json:"prefix"`
	Added      []string        `json:"added"` // 将新增的实体（kind:name）
	Conflicts  []MergeConflict `json:"conflicts"`
	// SemanticConflicts 语义级冲突预检（REQ-259/M258）：同名概念跨版本语义分歧（父类不一致/
	// 定义漂移）与 label 同义提示（潜在同实体待对齐）——只报告不做自动合并（详设口径），
	// 审查向导呈现供人工判断。
	SemanticConflicts []SemanticConflict `json:"semantic_conflicts,omitempty"`
	Renamed    []string        `json:"renamed"` // merge 策略重命名清单（old -> new）
	Stats      MergeStats      `json:"stats"`
	TargetName string          `json:"target_name"`
	MergedSpec *pkgspec.Spec   `json:"merged_spec"` // 应用后的完整结果（diff 预览用）
	// BaseVersion 预览时点目标版本（REQ-293② 防陈旧：apply 回传，服务端不符 409 要求重新预览）
	BaseVersion int `json:"base_version,omitempty"`
	// ImportReport 原文件解析报告（REQ-235/H5：lossy 清单+warnings 透出前端，支撑「补录→重跑对账」动线）；
	// spec 直传（无文件解析）时为 nil。
	ImportReport *Report `json:"import_report,omitempty"`
	// Notes 人话 notes（REQ-295②，67 号 A3：merged/unmatched/case/unlinked 四类带具体数字）
	Notes []MergeNote `json:"notes,omitempty"`
}

// MergeNote 构建产出的人话证据条目（每条带具体数字与对象名，人类可核对）。
type MergeNote struct {
	Kind string `json:"kind"` // merged | unmatched | case | unlinked
	Text string `json:"text"`
}

// MergeStats 合并统计。
type MergeStats struct {
	ConceptsAdded    int `json:"concepts_added"`
	ConceptsUpdated  int `json:"concepts_updated"`
	RelationsAdded   int `json:"relations_added"`
	RelationsUpdated int `json:"relations_updated"`
	InstancesAdded   int `json:"instances_added"`
	InstancesUpdated int `json:"instances_updated"`
	InstancesRenamed int `json:"instances_renamed"`
	TotalConflicts   int `json:"total_conflicts"`
}

// BuildMerged 按 strategy 构建合并结果（target=现行，incoming=导入）。纯函数。
func BuildMerged(target, incoming *pkgspec.Spec, strategy, prefix string) (*MergePreview, error) {
	if target == nil || incoming == nil {
		return nil, fmt.Errorf("target 与 incoming spec 必填")
	}
	switch strategy {
	case StrategyReplace, StrategyMergeOverwrite, StrategyMerge:
	default:
		return nil, fmt.Errorf("strategy 须为 replace | merge-overwrite | merge")
	}
	if strings.TrimSpace(prefix) == "" {
		prefix = DefaultConflictPrefix
	}
	pv := &MergePreview{
		Strategy:   strategy,
		Prefix:     prefix,
		Added:      []string{},
		Conflicts:  []MergeConflict{},
		Renamed:    []string{},
		TargetName: target.Name,
		MergedSpec: &pkgspec.Spec{
			ID:          target.ID,
			Name:        target.Name,
			Description: target.Description,
			Concepts:    append([]pkgspec.Concept{}, target.Concepts...),
			Relations:   append([]pkgspec.Relation{}, target.Relations...),
			Instances:   append([]pkgspec.Instance{}, target.Instances...),
		},
	}

	// ---- 概念 ----
	cIdx := map[string]int{}
	for i, c := range pv.MergedSpec.Concepts {
		cIdx[c.Name] = i
	}
	renamedConcepts := map[string]string{} // REQ-268/M77：merge 策略重命名映射（数据属性 domain 改写用）
	for _, in := range incoming.Concepts {
		if i, ok := cIdx[in.Name]; ok {
			cur := pv.MergedSpec.Concepts[i]
			conf := conceptConflict(cur, in)
			pv.Stats.TotalConflicts++
			switch strategy {
			case StrategyReplace:
				pv.MergedSpec.Concepts[i] = in
				conf.Resolution = "replaced"
				pv.Stats.ConceptsUpdated++
			case StrategyMergeOverwrite:
				pv.MergedSpec.Concepts[i] = conceptFieldMerge(cur, in)
				conf.Resolution = "field-merged"
				pv.Stats.ConceptsUpdated++
			default: // merge：保留现行，导入版重命名并入
				newName := uniquify(prefix+"_"+in.Name, cIdx)
				renamed := in
				renamed.Name = newName
				// 指向该概念的父子引用一并改写（保持层级语义）
				rewireConceptRefs(pv.MergedSpec, in.Name, newName)
				renamedConcepts[in.Name] = newName
				pv.MergedSpec.Concepts = append(pv.MergedSpec.Concepts, renamed)
				cIdx[newName] = len(pv.MergedSpec.Concepts) - 1
				conf.Resolution = "renamed"
				conf.ResolvedAs = newName
				pv.Renamed = append(pv.Renamed, fmt.Sprintf("concept %s -> %s", in.Name, newName))
				pv.Stats.ConceptsAdded++
			}
			pv.Conflicts = append(pv.Conflicts, conf)
		} else {
			pv.MergedSpec.Concepts = append(pv.MergedSpec.Concepts, in)
			cIdx[in.Name] = len(pv.MergedSpec.Concepts) - 1
			pv.Added = append(pv.Added, "concept:"+in.Name)
			pv.Stats.ConceptsAdded++
		}
	}

	// ---- 数据属性声明（REQ-268/M77）：merge 原逐字段重组会丢 data_properties（连 cq 亦然，既有缺口顺修）——
	// replace=随 incoming 整体替换；merge-overwrite=同名 incoming 覆盖+并集；merge=同名保留现行+incoming 新名并入
	// （v1 不做声明级冲突检测，审查页可见合并结果）；incoming 声明的 domain 经重命名映射改写防悬空。
	switch strategy {
	case StrategyReplace:
		pv.MergedSpec.DataProperties = incoming.DataProperties
		pv.MergedSpec.CQ = incoming.CQ
	case StrategyMergeOverwrite:
		merged := append([]pkgspec.DataProperty{}, incoming.DataProperties...)
		seenDp := map[string]bool{}
		for _, dp := range incoming.DataProperties {
			seenDp[dp.Name] = true
		}
		for _, dp := range target.DataProperties {
			if !seenDp[dp.Name] {
				merged = append(merged, dp)
			}
		}
		pv.MergedSpec.DataProperties = merged
		pv.MergedSpec.CQ = target.CQ
	default: // merge
		merged := append([]pkgspec.DataProperty{}, target.DataProperties...)
		seenDp := map[string]bool{}
		for _, dp := range target.DataProperties {
			seenDp[dp.Name] = true
		}
		for _, in := range incoming.DataProperties {
			if seenDp[in.Name] {
				continue
			}
			dp := in
			if nn, ok := renamedConcepts[dp.Domain]; ok {
				dp.Domain = nn
			}
			merged = append(merged, dp)
		}
		pv.MergedSpec.DataProperties = merged
		pv.MergedSpec.CQ = target.CQ
	}

	// ---- 关系 ----
	rIdx := map[string]int{}
	for i, r := range pv.MergedSpec.Relations {
		rIdx[r.Name] = i
	}
	for _, in := range incoming.Relations {
		if i, ok := rIdx[in.Name]; ok {
			cur := pv.MergedSpec.Relations[i]
			conf := relationConflict(cur, in)
			pv.Stats.TotalConflicts++
			switch strategy {
			case StrategyReplace:
				pv.MergedSpec.Relations[i] = in
				conf.Resolution = "replaced"
				pv.Stats.RelationsUpdated++
			case StrategyMergeOverwrite:
				pv.MergedSpec.Relations[i] = relationFieldMerge(cur, in)
				conf.Resolution = "field-merged"
				pv.Stats.RelationsUpdated++
			default:
				newName := uniquify(prefix+"_"+in.Name, rIdx)
				renamed := in
				renamed.Name = newName
				// 实例断言引用该关系名的改写
				rewireRelationRefs(pv.MergedSpec, in.Name, newName)
				pv.MergedSpec.Relations = append(pv.MergedSpec.Relations, renamed)
				rIdx[newName] = len(pv.MergedSpec.Relations) - 1
				conf.Resolution = "renamed"
				conf.ResolvedAs = newName
				pv.Renamed = append(pv.Renamed, fmt.Sprintf("relation %s -> %s", in.Name, newName))
				pv.Stats.RelationsAdded++
			}
			pv.Conflicts = append(pv.Conflicts, conf)
		} else {
			pv.MergedSpec.Relations = append(pv.MergedSpec.Relations, in)
			rIdx[in.Name] = len(pv.MergedSpec.Relations) - 1
			pv.Added = append(pv.Added, "relation:"+in.Name)
			pv.Stats.RelationsAdded++
		}
	}

	// ---- 实例 ----
	iIdx := map[string]int{}
	for i, in := range pv.MergedSpec.Instances {
		iIdx[in.Name] = i
	}
	for _, in := range incoming.Instances {
		if i, ok := iIdx[in.Name]; ok {
			cur := pv.MergedSpec.Instances[i]
			conf := instanceConflict(cur, in)
			pv.Stats.TotalConflicts++
			switch strategy {
			case StrategyReplace:
				pv.MergedSpec.Instances[i] = in
				conf.Resolution = "replaced"
				pv.Stats.InstancesUpdated++
			case StrategyMergeOverwrite:
				pv.MergedSpec.Instances[i] = instanceFieldMerge(cur, in)
				conf.Resolution = "field-merged"
				pv.Stats.InstancesUpdated++
			default:
				newName := uniquify(prefix+"_"+in.Name, iIdx)
				renamed := in
				renamed.Name = newName
				// 他者实例断言指向该实例的 target 改写
				rewireInstanceRefs(pv.MergedSpec, in.Name, newName)
				pv.MergedSpec.Instances = append(pv.MergedSpec.Instances, renamed)
				iIdx[newName] = len(pv.MergedSpec.Instances) - 1
				conf.Resolution = "renamed"
				conf.ResolvedAs = newName
				pv.Renamed = append(pv.Renamed, fmt.Sprintf("instance %s -> %s", in.Name, newName))
				pv.Stats.InstancesRenamed++
			}
			pv.Conflicts = append(pv.Conflicts, conf)
		} else {
			pv.MergedSpec.Instances = append(pv.MergedSpec.Instances, in)
			iIdx[in.Name] = len(pv.MergedSpec.Instances) - 1
			pv.Added = append(pv.Added, "instance:"+in.Name)
			pv.Stats.InstancesAdded++
		}
	}
	// REQ-295②：人话 notes（证据故事带数字，非扣分表）
	if pv.Stats.TotalConflicts > 0 {
		pv.Notes = append(pv.Notes, MergeNote{Kind: "merged", Text: fmt.Sprintf("同名冲突 %d 处已按策略 %s 处置", pv.Stats.TotalConflicts, strategy)})
	}
	if len(pv.Renamed) > 0 {
		pv.Notes = append(pv.Notes, MergeNote{Kind: "merged", Text: fmt.Sprintf("导入侧 %d 个对象重命名并入（前缀 %s）", len(pv.Renamed), prefix)})
	}
	conceptSet := map[string]bool{}
	for _, c := range pv.MergedSpec.Concepts {
		conceptSet[c.Name] = true
	}
	dangling := 0
	for _, r := range pv.MergedSpec.Relations {
		if !conceptSet[r.From] || !conceptSet[r.To] {
			dangling++
		}
	}
	if dangling > 0 {
		pv.Notes = append(pv.Notes, MergeNote{Kind: "unmatched", Text: fmt.Sprintf("%d 条关系端点引用未定义概念（结构校验将拦截，请先补概念）", dangling)})
	}
	// REQ-259/M258：语义级冲突预检（同名概念父类分歧/定义漂移+label 同义提示）——
	// 只报告不做自动合并（详设口径）；三选项策略行为零变化。
	pv.SemanticConflicts = semanticConflicts(target, incoming)
	return pv, nil
}

// uniquify 前缀协调：重命名后仍冲突则追加 _2/_3…
func uniquify(base string, taken map[string]int) string {
	name := base
	for n := 2; ; n++ {
		if _, ok := taken[name]; !ok {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
}

// mergeConflictOf 构建冲突条目（导入/现行实体 JSON 摘要，供审查向导字段级对照）。
func mergeConflictOf(kind, name string, fields []string, incoming, current any) MergeConflict {
	inJSON, _ := json.Marshal(incoming)
	curJSON, _ := json.Marshal(current)
	if fields == nil {
		fields = []string{}
	}
	return MergeConflict{
		Kind:     kind,
		Name:     name,
		Fields:   fields,
		Incoming: string(inJSON),
		Current:  string(curJSON),
	}
}

// ---- 冲突字段提取 ----

func conceptConflict(cur, in pkgspec.Concept) MergeConflict {
	fields := []string{}
	if cur.Definition != in.Definition {
		fields = append(fields, "definition")
	}
	if cur.Label != in.Label {
		fields = append(fields, "label")
	}
	if strings.Join(cur.Parents, "|") != strings.Join(in.Parents, "|") {
		fields = append(fields, "parents")
	}
	return mergeConflictOf("concept", in.Name, fields, in, cur)
}

func conceptFieldMerge(cur, in pkgspec.Concept) pkgspec.Concept {
	out := cur
	if strings.TrimSpace(in.Definition) != "" {
		out.Definition = in.Definition
	}
	if strings.TrimSpace(in.Label) != "" {
		out.Label = in.Label
	}
	if len(in.Parents) > 0 {
		out.Parents = in.Parents
	}
	return out
}

func relationConflict(cur, in pkgspec.Relation) MergeConflict {
	fields := []string{}
	if cur.Definition != in.Definition {
		fields = append(fields, "definition")
	}
	if cur.Label != in.Label {
		fields = append(fields, "label")
	}
	if cur.From != in.From {
		fields = append(fields, "from")
	}
	if cur.To != in.To {
		fields = append(fields, "to")
	}
	return mergeConflictOf("relation", in.Name, fields, in, cur)
}

func relationFieldMerge(cur, in pkgspec.Relation) pkgspec.Relation {
	out := cur
	if strings.TrimSpace(in.Definition) != "" {
		out.Definition = in.Definition
	}
	if strings.TrimSpace(in.Label) != "" {
		out.Label = in.Label
	}
	if strings.TrimSpace(in.From) != "" {
		out.From = in.From
	}
	if strings.TrimSpace(in.To) != "" {
		out.To = in.To
	}
	if strings.TrimSpace(in.Evidence) != "" {
		out.Evidence = in.Evidence // REQ-295①：证据分级随字段合并透传
	}
	return out
}

func instanceConflict(cur, in pkgspec.Instance) MergeConflict {
	fields := []string{}
	if cur.Concept != in.Concept {
		fields = append(fields, "concept")
	}
	if len(cur.Attributes) != len(in.Attributes) || !equalAttributes(cur.Attributes, in.Attributes) {
		fields = append(fields, "attributes")
	}
	if len(cur.Relations) != len(in.Relations) || !equalInstanceRels(cur.Relations, in.Relations) {
		fields = append(fields, "relations")
	}
	return mergeConflictOf("instance", in.Name, fields, in, cur)
}

func instanceFieldMerge(cur, in pkgspec.Instance) pkgspec.Instance {
	out := cur
	if strings.TrimSpace(in.Concept) != "" {
		out.Concept = in.Concept
	}
	if len(in.Attributes) > 0 {
		merged := map[string]any{}
		for k, v := range cur.Attributes {
			merged[k] = v
		}
		for k, v := range in.Attributes {
			merged[k] = v
		}
		out.Attributes = merged
	}
	if len(in.Relations) > 0 {
		out.Relations = in.Relations
	}
	return out
}

func equalAttributes(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || fmt.Sprint(bv) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func equalInstanceRels(a, b []pkgspec.InstanceRel) bool {
	if len(a) != len(b) {
		return false
	}
	join := func(rs []pkgspec.InstanceRel) string {
		parts := make([]string, 0, len(rs))
		for _, r := range rs {
			parts = append(parts, r.Rel+"->"+r.Target)
		}
		return strings.Join(parts, "|")
	}
	return join(a) == join(b)
}

// ---- 引用改写（merge 重命名并入时保持语义） ----

func rewireConceptRefs(sp *pkgspec.Spec, oldName, newName string) {
	for i := range sp.Concepts {
		for j, pr := range sp.Concepts[i].Parents {
			if pr == oldName {
				sp.Concepts[i].Parents[j] = newName
			}
		}
	}
	for i := range sp.Relations {
		if sp.Relations[i].From == oldName {
			sp.Relations[i].From = newName
		}
		if sp.Relations[i].To == oldName {
			sp.Relations[i].To = newName
		}
	}
	for i := range sp.Instances {
		if sp.Instances[i].Concept == oldName {
			sp.Instances[i].Concept = newName
		}
	}
}

func rewireRelationRefs(sp *pkgspec.Spec, oldName, newName string) {
	for i := range sp.Instances {
		for j := range sp.Instances[i].Relations {
			if sp.Instances[i].Relations[j].Rel == oldName {
				sp.Instances[i].Relations[j].Rel = newName
			}
		}
	}
}

func rewireInstanceRefs(sp *pkgspec.Spec, oldName, newName string) {
	for i := range sp.Instances {
		for j := range sp.Instances[i].Relations {
			if sp.Instances[i].Relations[j].Target == oldName {
				sp.Instances[i].Relations[j].Target = newName
			}
		}
	}
}


// SemanticConflict 语义级冲突一条（REQ-259/M258 详设口径）。
type SemanticConflict struct {
	Kind     string `json:"kind"` // parent_divergence | definition_drift | label_synonym
	Entity   string `json:"entity"`
	Severity string `json:"severity"` // conflict | hint
	Detail   string `json:"detail"`
}

// semanticConflicts 同名概念语义级预检（REQ-259；三选项策略行为零变化——只加检测）：
// ①parent_divergence（conflict）：同名概念父类集互异（层级语义分歧——合并后无论哪版胜出都改变层级归属）；
// ②definition_drift（conflict）：同名概念双方定义非空且互异超过阈值（同名义不同义风险）；
// ③label_synonym（hint）：导入概念 label/synonym 与现行**不同名**概念的 label 相同（潜在同实体
// 待对齐提示——REQ-262 external_refs alignment 的登记候选线索）。
func semanticConflicts(target, incoming *pkgspec.Spec) []SemanticConflict {
	var out []SemanticConflict
	tIdx := map[string]int{}
	for i, c := range target.Concepts {
		tIdx[c.Name] = i
	}
	labelIdx := map[string]string{} // label → 概念名（现行侧）
	for _, c := range target.Concepts {
		if l := normalizeLabel(c.Label); l != "" {
			labelIdx[l] = c.Name
		}
	}
	parentSet := func(ps []string) map[string]bool {
		m := map[string]bool{}
		for _, p := range ps {
			m[p] = true
		}
		return m
	}
	for _, in := range incoming.Concepts {
		if i, ok := tIdx[in.Name]; ok {
			cur := target.Concepts[i]
			// ① 父类分歧：互有对方没有的父类（真分歧；单方空=增补不算）
			cs, is := parentSet(cur.Parents), parentSet(in.Parents)
			onlyCur, onlyIn := []string{}, []string{}
			for p := range cs {
				if !is[p] {
					onlyCur = append(onlyCur, p)
				}
			}
			for p := range is {
				if !cs[p] {
					onlyIn = append(onlyIn, p)
				}
			}
			if len(onlyCur) > 0 && len(onlyIn) > 0 {
				out = append(out, SemanticConflict{
					Kind: "parent_divergence", Entity: in.Name, Severity: "conflict",
					Detail: fmt.Sprintf("同名概念父类分歧：现行 [%s] vs 导入 [%s]（合并后层级归属随策略变化，请人工裁决）",
						strings.Join(onlyCur, ","), strings.Join(onlyIn, ",")),
				})
			}
			// ② 定义漂移：双方非空且互异（编辑噪声防抖：一方为空=增补不算漂移）
			cd, id := strings.TrimSpace(cur.Definition), strings.TrimSpace(in.Definition)
			if cd != "" && id != "" && cd != id {
				out = append(out, SemanticConflict{
					Kind: "definition_drift", Entity: in.Name, Severity: "conflict",
					Detail: fmt.Sprintf("同名概念定义分歧：现行「%.60s」vs 导入「%.60s」", cd, id),
				})
			}
		}
		// ③ label 同义提示：导入概念 label 命中现行**不同名**概念的 label（潜在对齐候选）
		if l := normalizeLabel(in.Label); l != "" {
			if owner, ok := labelIdx[l]; ok && owner != in.Name {
				out = append(out, SemanticConflict{
					Kind: "label_synonym", Entity: in.Name, Severity: "hint",
					Detail: fmt.Sprintf("导入概念 %q 的显示名「%s」与现行概念 %q 相同（潜在同实体——可登记 external_ref alignment 对齐）", in.Name, l, owner),
				})
			}
		}
	}
	return out
}

func normalizeLabel(l string) string {
	return strings.ToLower(strings.TrimSpace(l))
}
