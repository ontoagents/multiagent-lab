// Package spec 定义本体归一化形态 spec_json 的类型与校验规则。
// 三类要素：concepts / relations / instances（REQ-61）。
// 校验 = JSON 结构校验 + 引用完整性（REQ-63），构建平面服务侧执行；
// 规则与人工编辑、LLM 辅助创建共用（REQ-63/82）。
package spec

import (
	"crypto/sha256"
	"encoding/hex"
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
	// Synonyms 概念同义词（REQ-419/M215：TBox 一等公民——业务口语查询「客户/甲方」在 TBox 层
	// 命中；来源=SKOS altLabel 导入捕获回写/手动编辑/spec_concept_upsert；消费=check_answerable
	// 预检匹配〔沿 altLabel 经 TTL 导出进引擎〕）。dtype 保持轻：纯词表无元属性。
	Synonyms []string `json:"synonyms,omitempty"`
	// Note 建模备注（REQ-510/M318：元素级建模决策记录——随 spec_json 版本留痕；
	// TTL 导出为 skos:note〔sidecar 承载〕；LLM 生成面不扩——人工编辑承载）。
	Note string `json:"note,omitempty"`
	// Restrictions 匿名限制公理承载（REQ-376①/M172 详设拍板子集五型）——自「丢弃入余量账本
	// +warning」升档为建模面保留层。承载路径=人工 spec 编辑+TTL 导入导出 round-trip（Go 读入端
	// REQ-378 对偶）；LLM 生成面不扩（SpecRulesPrompt/SchemaHint 不含 restrictions——77 号
	// 「不做 spec_json 全面 OWL 化」维持，LLM 契约闸门边界随详设拍板留痕）。
	Restrictions []Restriction `json:"restrictions,omitempty"`
}

// Restriction 匿名限制公理（owl:Restriction 承载子集）。
type Restriction struct {
	Type     string `json:"type"`             // some_values_from | all_values_from | cardinality | min_cardinality | max_cardinality
	Property string `json:"property"`         // 对象属性名（引用 relations.name）
	Target   string `json:"target,omitempty"` // some/all_values_from 值域概念名（基数型可空）
	Value    int    `json:"value,omitempty"`  // 基数（基数型必填 ≥0）
}

// RestrictionTypes 限制公理合法类型（详设拍板子集五型）。
var RestrictionTypes = map[string]bool{
	"some_values_from": true, "all_values_from": true,
	"cardinality": true, "min_cardinality": true, "max_cardinality": true,
}

// Relation 对象属性/关系（概念层，From/To 为概念名）。
// Evidence 证据分级（REQ-295①，67 号 A2）：declared（显式声明，如导入 OWL 声明/人工配置列）
// > name（命名推断，如结构化构建列名共现）> values（值/语料证据，如 KG 抽取、LLM 语料抽取）——
// 溯源不是有无而是强度；空=未标注（存量零迁移）。仅 spec 层，不入 TTL 导出（沿 CQ 先例）。
// Characteristics 特性（REQ-473/M273，Protégé 编辑能力对齐批②——D-O22 表达力档「属性特征」兑现）：
// transitive | symmetric | functional | inverse_functional；TTL 导出为附加 rdf:type 行；
// InverseOf 逆属性（引用 relations.name，可空）导出 owl:inverseOf。存量零迁移。
type Relation struct {
	Name           string   `json:"name"`
	Label          string   `json:"label,omitempty"`
	Definition     string   `json:"definition,omitempty"`
	From           string   `json:"from"` // 定义域概念名
	To             string   `json:"to"`   // 值域概念名
	Evidence       string   `json:"evidence,omitempty"`
	Characteristics []string `json:"characteristics,omitempty"`
	// Note 建模备注（REQ-510/M318：同 Concept.Note——skos:note 导出）
	Note string `json:"note,omitempty"`
	InverseOf      string   `json:"inverse_of,omitempty"`
}

// RelationCharacteristics 合法关系特性（Protégé ObjectProperty characteristics 子集；
// 反对称/自反/非自反/属性链不进本子集——spec 轻量口径）。
var RelationCharacteristics = []string{"transitive", "symmetric", "functional", "inverse_functional"}

func validRelationCharacteristic(v string) bool {
	for _, c := range RelationCharacteristics {
		if c == v {
			return true
		}
	}
	return false
}

// 证据分级枚举与排序权重（declared > name > values；权重仅用于构建报告排序，不入存储）。
var EvidenceLevels = []string{"declared", "name", "values"}

func ValidEvidence(v string) bool {
	if v == "" {
		return true
	}
	for _, lv := range EvidenceLevels {
		if v == lv {
			return true
		}
	}
	return false
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
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// CQ 能力问题（REQ-90/REQ-248）：「本体要回答什么问题」的建模锚点——仅 spec 层
	// （不入 TTL 导出/vowljson/不参与结构校验），三 LLM 构建路径回写、手工路径可录。
	CQ             []string       `json:"cq,omitempty"`
	Concepts       []Concept      `json:"concepts"`
	Relations      []Relation     `json:"relations"`
	DataProperties []DataProperty `json:"data_properties,omitempty"` // REQ-268/M77：可选声明层，存量资产零迁移
	Axioms         []Axiom        `json:"axioms,omitempty"`          // REQ-269/M78：可选公理保留层，存量资产零迁移
	Instances      []Instance     `json:"instances"`
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
	// REQ-376①：限制公理校验（Type 枚举/Property 引用关系/基数型 Value≥0/some·all Target 引用概念）
	relNames := map[string]bool{}
	for _, r := range s.Relations {
		relNames[r.Name] = true
	}
	for i, c := range s.Concepts {
		for j, rst := range c.Restrictions {
			p := fmt.Sprintf("concepts[%d].restrictions[%d]", i, j)
			if !RestrictionTypes[rst.Type] {
				add(p+".type", "限制公理类型非法: "+rst.Type+"（合法=some_values_from|all_values_from|cardinality|min_cardinality|max_cardinality）")
			}
			if rst.Property == "" || !relNames[rst.Property] {
				add(p+".property", "限制公理属性必须引用已定义关系: "+rst.Property)
			}
			if (rst.Type == "cardinality" || rst.Type == "min_cardinality" || rst.Type == "max_cardinality") && rst.Value < 0 {
				add(p+".value", "基数型限制 Value 须 ≥0")
			}
			if (rst.Type == "some_values_from" || rst.Type == "all_values_from") && rst.Target != "" {
				if _, ok := cn[rst.Target]; !ok {
					add(p+".target", "限制公理值域引用不存在的概念: "+rst.Target)
				}
			}
		}
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

	// ---- relations：name 必填唯一；from/to 必须是已定义概念；evidence 三档枚举（REQ-295①）----
	rn := map[string]int{}
	for i, r := range s.Relations {
		p := fmt.Sprintf("relations[%d]", i)
		if strings.TrimSpace(r.Name) == "" {
			add(p+".name", "关系名不能为空")
			continue
		}
		if !ValidEvidence(r.Evidence) {
			add(p+".evidence", "证据分级只能是 declared/name/values 或留空: "+r.Evidence)
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
		// REQ-473/M273：特性枚举 + 逆属性引用（自引用即 owl:inverseOf 自身=无意义，拒）
		for _, ch := range r.Characteristics {
			if !validRelationCharacteristic(ch) {
				add(p+".characteristics", "非法特性 "+ch+"（合法：transitive|symmetric|functional|inverse_functional）")
			}
		}
		if r.InverseOf == r.Name {
			add(p+".inverse_of", "逆属性不能是自身")
		}
		// inverse_of 引用校验需全量关系名——两轮遍历（第一轮建名册，第二轮校验引用）
		if r.InverseOf != "" {
			found := false
			for _, r2 := range s.Relations {
				if r2.Name == r.InverseOf {
					found = true
					break
				}
			}
			if !found {
				add(p+".inverse_of", "逆属性 "+r.InverseOf+" 未在 relations 中声明")
			}
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

// TypeID 确定性类型身份（REQ-292②，67 号 B5）：sha256(typeName) 前 20 位 hex。
// 与 62 号 K3 canonicalId 命名脊同族——身份由内容派生、跨重算稳定。
func TypeID(typeName string) string {
	sum := sha256.Sum256([]byte(typeName))
	return hex.EncodeToString(sum[:])[:20]
}

// LogicalID 确定性对象身份（REQ-292②）：sha256(typeID + ":" + key) 前 32 位 hex。
// 对象级 overlay 以此定位（overlay 按 logicalId 存取，身份键不稳则三层无从落地）；
// 实例的 typeName=所属概念名、key=实例名（同名跨概念已被 Validate 禁止，全域唯一）。
// 导出 TTL 实例 IRI（urn:o:{oid}:instance:{name}）以名为键：重算同键→同 IRI 不漂移，
// 与 LogicalID 同一稳定性口径。
func LogicalID(typeName, key string) string {
	sum := sha256.Sum256([]byte(TypeID(typeName) + ":" + key))
	return hex.EncodeToString(sum[:])[:32]
}

// 对象四类（overlay 的 kind 值域）；instance 的 typeName 取所属概念名。
const (
	ObjKindConcept      = "concept"
	ObjKindRelation     = "relation"
	ObjKindDataProperty = "data_property"
	ObjKindInstance     = "instance"
)

// ObjectLogicalID 按对象 kind+name（instance 另取所属概念）计算确定性身份。
func ObjectLogicalID(kind, conceptName, name string) string {
	switch kind {
	case ObjKindInstance:
		return LogicalID(conceptName, name)
	case ObjKindConcept, ObjKindRelation, ObjKindDataProperty:
		return LogicalID(kind, name)
	}
	return ""
}
