// Package spec 定义本体归一化形态 spec_json 的类型与校验规则。
// 三类要素：concepts / relations / instances（REQ-61）。
// 校验 = JSON 结构校验 + 引用完整性（REQ-63），构建平面服务侧执行；
// 规则与人工编辑、LLM 辅助创建共用（REQ-63/82）。
package spec

import (
	"fmt"
	"strings"
	"unicode"
)

// Concept 概念（类）。
type Concept struct {
	Name       string   `json:"name"`
	Label      string   `json:"label,omitempty"`
	Definition string   `json:"definition,omitempty"`
	Parents    []string `json:"parents,omitempty"` // 父概念名（多继承）
}

// Relation 对象属性/关系（概念层，From/To 为概念名）。
type Relation struct {
	Name       string `json:"name"`
	Label      string `json:"label,omitempty"`
	Definition string `json:"definition,omitempty"`
	From       string `json:"from"` // 定义域概念名
	To         string `json:"to"`   // 值域概念名
}

// InstanceRel 实例关系断言。
type InstanceRel struct {
	Rel    string `json:"rel"`    // 关系名
	Target string `json:"target"` // 目标实例名
}

// Instance 实例。
type Instance struct {
	Name       string         `json:"name"`
	Concept    string         `json:"concept"` // 所属概念名
	Attributes map[string]any `json:"attributes,omitempty"`
	Relations  []InstanceRel  `json:"relations,omitempty"`
}

// DataProperty 数据属性（字面量属性的概念层声明，REQ-268/M77 表达力升级一期）。
// 声明与实例 attributes 键同名关联；TTL 导出使用 attr: 命名空间（与实例属性断言同 IRI，
// 自 REQ-235⑥「声明丢弃+warning」升档为声明捕获）。Domain 可空=不限定义域；
// Range 用短名（string/number/integer/boolean/date），未知形态保留原始 IRI，空=string。
type DataProperty struct {
	Name       string `json:"name"`
	Label      string `json:"label,omitempty"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"` // 定义域概念名（引用 concepts.name）
	Range      string `json:"range,omitempty"`  // 数据类型（短名或 xsd:/完整 IRI）
}

// Axiom 公理/约束保留层（REQ-269/M78，D-O22 表达力升级二期）。
// v1 最小集：类级 disjoint_with / equivalent_class——由真实资产导入保真承载
// （严格语义不由 LLM 生成，诚实边界随行）；TTL 导出以 owl:disjointWith/owl:equivalentClass 回写。
type Axiom struct {
	Type    string   `json:"type"`    // disjoint_with | equivalent_class
	Subject string   `json:"subject"` // 主体概念名（引用 concepts.name）
	Targets []string `json:"targets"` // 目标概念名（≥1，全部引用已定义概念）
}

// Spec 本体归一化形态（spec_json）。
type Spec struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	// CQ 能力问题（REQ-90/REQ-248）：「本体要回答什么问题」的建模锚点——仅 spec 层
	// （不入 TTL 导出/vowljson/不参与结构校验），三 LLM 构建路径回写、手工路径可录。
	CQ             []string        `json:"cq,omitempty"`
	Concepts       []Concept       `json:"concepts"`
	Relations      []Relation      `json:"relations"`
	DataProperties []DataProperty  `json:"data_properties,omitempty"` // REQ-268/M77：可选声明层，存量资产零迁移
	Axioms         []Axiom         `json:"axioms,omitempty"`          // REQ-269/M78：可选公理保留层，存量资产零迁移
	Instances      []Instance      `json:"instances"`
}

// ValidationError 单条校验错误（结构化，供 LLM 修正循环回喂）。
type ValidationError struct {
	Path    string `json:"path"` // 如 concepts[2].parents[0]
	Message string `json:"message"`
}

func (e ValidationError) Error() string { return e.Path + ": " + e.Message }

// Validate 校验结构完整性与引用完整性，返回全部错误（不短 路）。
func (s *Spec) Validate() []ValidationError {
	var errs []ValidationError
	add := func(path, msg string) { errs = append(errs, ValidationError{Path: path, Message: msg}) }

	// ---- concepts：name 必填唯一；parents 引用已定义概念 ----
	cn := map[string]int{}
	for i, c := range s.Concepts {
		p := fmt.Sprintf("concepts[%d]", i)
		if strings.TrimSpace(c.Name) == "" {
			add(p+".name", "概念名不能为空")
			continue
		}
		if _, dup := cn[c.Name]; dup {
			add(p+".name", "概念名重复: "+c.Name)
			continue
		}
		cn[c.Name] = i
	}
	for i, c := range s.Concepts {
		if _, ok := cn[c.Name]; !ok {
			continue // 名字本身有问题，跳过引用检查
		}
		for j, p := range c.Parents {
			if _, ok := cn[p]; !ok {
				add(fmt.Sprintf("concepts[%d].parents[%d]", i, j), "引用了未定义概念: "+p)
			}
		}
	}

	// ---- relations：name 必填唯一；from/to 必须是已定义概念 ----
	rn := map[string]int{}
	for i, r := range s.Relations {
		p := fmt.Sprintf("relations[%d]", i)
		if strings.TrimSpace(r.Name) == "" {
			add(p+".name", "关系名不能为空")
			continue
		}
		if _, dup := rn[r.Name]; dup {
			add(p+".name", "关系名重复: "+r.Name)
			continue
		}
		rn[r.Name] = i
		if _, ok := cn[r.From]; !ok {
			add(p+".from", "定义域引用了未定义概念: "+r.From)
		}
		if _, ok := cn[r.To]; !ok {
			add(p+".to", "值域引用了未定义概念: "+r.To)
		}
	}

	// ---- data_properties（REQ-268/M77）：name 必填唯一；domain 引用已定义概念 ----
	// （range 宽松不校验——未知类型诚实保留；实例 attributes 键未声明属完备性提示，
	//  qualitygate undeclared_attribute_key info 级承载，不在此硬阻断以兼容存量自由属性资产）
	dn := map[string]int{}
	for i, dp := range s.DataProperties {
		p := fmt.Sprintf("data_properties[%d]", i)
		if strings.TrimSpace(dp.Name) == "" {
			add(p+".name", "数据属性名不能为空")
			continue
		}
		if _, dup := dn[dp.Name]; dup {
			add(p+".name", "数据属性名重复: "+dp.Name)
			continue
		}
		dn[dp.Name] = i
		if dp.Domain != "" {
			if _, ok := cn[dp.Domain]; !ok {
				add(p+".domain", "定义域引用了未定义概念: "+dp.Domain)
			}
		}
	}

	// ---- axioms（REQ-269/M78）：type 枚举；subject/targets 必填且引用已定义概念——
	// 公理是严格语义，悬空引用直接错误级（导入侧已防：未注册目标 warning 丢弃不落库）----
	an := map[string]int{}
	for i, ax := range s.Axioms {
		p := fmt.Sprintf("axioms[%d]", i)
		if ax.Type != "disjoint_with" && ax.Type != "equivalent_class" {
			add(p+".type", "不支持的公理类型: "+ax.Type+"（仅 disjoint_with/equivalent_class）")
			continue
		}
		if strings.TrimSpace(ax.Subject) == "" {
			add(p+".subject", "公理主体不能为空")
			continue
		}
		if _, ok := cn[ax.Subject]; !ok {
			add(p+".subject", "主体引用了未定义概念: "+ax.Subject)
		}
		if len(ax.Targets) == 0 {
			add(p+".targets", "公理目标不能为空")
			continue
		}
		for j, tgt := range ax.Targets {
			if strings.TrimSpace(tgt) == "" {
				add(fmt.Sprintf("%s.targets[%d]", p, j), "公理目标不能为空")
				continue
			}
			if _, ok := cn[tgt]; !ok {
				add(fmt.Sprintf("%s.targets[%d]", p, j), "目标引用了未定义概念: "+tgt)
			}
		}
		key := ax.Type + "|" + ax.Subject
		if _, dup := an[key]; dup {
			add(p, "重复公理（同类型同主体）: "+ax.Subject)
		}
		an[key] = i
	}

	// ---- instances：name 必填唯一；concept/relations 引用完整性 ----
	in := map[string]int{}
	for i, it := range s.Instances {
		p := fmt.Sprintf("instances[%d]", i)
		if strings.TrimSpace(it.Name) == "" {
			add(p+".name", "实例名不能为空")
			continue
		}
		if _, dup := in[it.Name]; dup {
			add(p+".name", "实例名重复: "+it.Name)
			continue
		}
		in[it.Name] = i
		if _, ok := cn[it.Concept]; !ok {
			add(p+".concept", "引用了未定义概念: "+it.Concept)
		}
	}
	for i, it := range s.Instances {
		if _, ok := in[it.Name]; !ok {
			continue
		}
		for j, ir := range it.Relations {
			if _, ok := rn[ir.Rel]; !ok {
				add(fmt.Sprintf("instances[%d].relations[%d].rel", i, j), "引用了未定义关系: "+ir.Rel)
			}
			if _, ok := in[ir.Target]; !ok {
				add(fmt.Sprintf("instances[%d].relations[%d].target", i, j), "引用了未定义实例: "+ir.Target)
			}
		}
	}

	// ---- 跨域查重（REQ-249/G3，唯一性规则统一）：概念与实例不可同名——
	// 此前仅 GraphEditor 提交时检查（useEditorActions），JSON/LLM 路径可保存出
	// 图形编辑器判定非法的本体；TTL 导出下 concept:/instance: URI 前缀不同虽不冲突，
	// 但 onto_* 精确匹配按名称命中会歧义（get_concept/get_instance 同名双命中）。
	for name, ci := range cn {
		if _, ok := in[name]; ok {
			add(fmt.Sprintf("concepts[%d]", ci), "概念与实例同名: "+name+"（请改名其一——名称跨概念/实例全域唯一）")
		}
	}
	return errs
}

// Stats 返回概念/关系/实例数量（列表页统计，REQ-60）。
func (s *Spec) Stats() (concepts, relations, instances int) {
	return len(s.Concepts), len(s.Relations), len(s.Instances)
}

// ConceptURI / RelationURI / InstanceURI / AttrURI 生成确定性 URN。
// 导出（spec_json→TTL）与查询翻译（onto_*→SPARQL）共用同一规则，
// 保证自建本体在 RDF 形态下可双向定位。
func URIPrefix(ontologyID string) string {
	return "urn:o:" + Sanitize(ontologyID) + ":"
}

func ConceptURI(ontologyID, name string) string {
	return URIPrefix(ontologyID) + "concept:" + Sanitize(name)
}

func RelationURI(ontologyID, name string) string {
	return URIPrefix(ontologyID) + "relation:" + Sanitize(name)
}

func InstanceURI(ontologyID, name string) string {
	return URIPrefix(ontologyID) + "instance:" + Sanitize(name)
}

func AttrURI(ontologyID, key string) string {
	return URIPrefix(ontologyID) + "attr:" + Sanitize(key)
}

func Sanitize(s string) string {
	// 确定性规则：保留 Unicode 字母/数字与 _-. ，其余每字符替换为 '_'。
	// 与 tools/rdf-sidecar/sidecar.py 的 sanitize 保持逐字符一致（导出与查询翻译共用）。
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
