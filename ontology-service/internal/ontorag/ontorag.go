// Package ontorag CQ 驱动增量扩展——OntoExtend 论文三段式 RAG（REQ-343/OE-1，64 号 §4 蓝图 S1~S3）。
//
// 论文：arXiv 2607.17963（OntoExtend，需求驱动的本体扩展）。三段式：
//
//	Retriever  元素级抽取（OntologyElement）→ 管道符序列化（含 comments，N2 口径）→ 向量检索 top-k
//	Extender   CQ + 检索片段 → LLM 生成 spec 片段（双 profile）→ 两阶段校验（失败重试/标记，不静默入库）
//	Integrator 去重 → 既有 merge/preview|apply 审查底座（REQ-157，版本快照）
//
// 以新能力承载，不复用现有 ontoextend 路径 API 名（64 号 §3.1「同名不同物」既定决策）。
// 诚实边界：OntoExtend 仓库无 LICENSE——零代码引入，只借鉴算法/prompt/口径（64 号 §2.7）；
// 论文用 FAISS IndexFlatIP，本平台单机本体元素量级为百~千，纯 Go 暴力内积等价且免重依赖（D-O15）。
package ontorag

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// Element 本体元素（论文 OntologyElement 适配 spec_json 形态）。
// 仅 TBox 进索引（类/对象属性/数据属性）——实例是 A-Box，论文管线明确禁入（Extender 指令③）。
type Element struct {
	URI       string   `json:"uri"`               // urn:o:{oid}:concept:{name} 等（spec 同一 URI 口径，可回指定位）
	Kind      string   `json:"kind"`              // class | object_property | data_property
	LocalName string   `json:"local_name"`        // spec 内名称（中文/标识符）
	Label     string   `json:"label,omitempty"`   // 人类可读标签
	Comment   string   `json:"comment,omitempty"` // 定义（N2 口径：序列化必须含 comments）
	Domain    []string `json:"domain,omitempty"`  // 关系 from / 数据属性 domain
	Range     []string `json:"range,omitempty"`   // 关系 to
	Super     []string `json:"super,omitempty"`   // 父概念（parents）
	Snippet   string   `json:"snippet"`           // verbatim spec 对象片段（JSON；prompt 只读上下文与前端展示）
}

// ElementsFromSpec 元素级抽取（Retriever 前置，64 号 §4.3 模块 A）：
// 遍历 concepts / relations / data_properties 构建 Element；snippet 为该对象
// 的 verbatim JSON 片段（深度 1——对象自身声明，层级关系由 Super/Domain 字段承载）。
func ElementsFromSpec(ontologyID string, sp *pkgspec.Spec) []Element {
	out := make([]Element, 0, len(sp.Concepts)+len(sp.Relations)+len(sp.DataProperties))
	add := func(e Element, obj any) {
		if b, err := json.Marshal(obj); err == nil {
			e.Snippet = string(b)
		}
		out = append(out, e)
	}
	for _, c := range sp.Concepts {
		add(Element{
			URI:       pkgspec.ConceptURI(ontologyID, c.Name),
			Kind:      "class",
			LocalName: c.Name,
			Label:     c.Label,
			Comment:   c.Definition,
			Super:     append([]string(nil), c.Parents...),
		}, c)
	}
	for _, r := range sp.Relations {
		add(Element{
			URI:       pkgspec.RelationURI(ontologyID, r.Name),
			Kind:      "object_property",
			LocalName: r.Name,
			Label:     r.Label,
			Comment:   r.Definition,
			Domain:    []string{r.From},
			Range:     []string{r.To},
		}, r)
	}
	for _, dp := range sp.DataProperties {
		dom := []string(nil)
		if dp.Domain != "" {
			dom = []string{dp.Domain}
		}
		add(Element{
			URI:       pkgspec.URIPrefix(ontologyID) + "attr:" + pkgspec.Sanitize(dp.Name),
			Kind:      "data_property",
			LocalName: dp.Name,
			Label:     dp.Label,
			Comment:   dp.Definition,
			Domain:    dom,
		}, dp)
	}
	return out
}

// kindText 元素类型中文名（论文 "Type: object property" 的序列化位）。
func kindText(kind string) string {
	switch kind {
	case "class":
		return "概念"
	case "object_property":
		return "对象关系"
	case "data_property":
		return "数据属性"
	}
	return kind
}

// SearchText 管道符序列化（论文 get_searchable_text 口径，字段顺序一致）：
// localName | label | comment | Type: {kind} | Domain: {names} | Range: {names} | 上位: {names}
// ——N2 最优配置=管道符分隔+含 comments（论文 Table 3：ada-002/pipe/with comments M_w 0.63）。
func (e Element) SearchText() string {
	parts := []string{e.LocalName}
	if e.Label != "" && e.Label != e.LocalName {
		parts = append(parts, e.Label)
	}
	if e.Comment != "" {
		parts = append(parts, e.Comment)
	}
	parts = append(parts, "Type: "+kindText(e.Kind))
	if len(e.Domain) > 0 {
		parts = append(parts, "Domain: "+strings.Join(e.Domain, "、"))
	}
	if len(e.Range) > 0 {
		parts = append(parts, "Range: "+strings.Join(e.Range, "、"))
	}
	if len(e.Super) > 0 {
		parts = append(parts, "上位: "+strings.Join(e.Super, "、"))
	}
	return strings.Join(parts, " | ")
}

// Hit 检索命中（元素+相似度得分）。
type Hit struct {
	Element Element `json:"element"`
	Score   float64 `json:"score"`
}

// l2normalize 向量 L2 归一（归一后内积=余弦，论文 IndexFlatIP 口径）。
func l2normalize(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

// TopK 查询向量与元素向量集的内积 top-k（暴力；单机百~千元素尺度等价 FAISS IndexFlatIP）。
// vectors 与 elements 同序一一对应；k<=0 取默认 20。
func TopK(queryVec []float64, elements []Element, vectors [][]float64, k int) []Hit {
	if k <= 0 {
		k = 20
	}
	q := l2normalize(queryVec)
	idx := make([]int, 0, len(elements))
	for i, v := range vectors {
		if i >= len(elements) || len(v) != len(q) {
			continue // 维度不齐的向量（embedding 服务异常）如实跳过
		}
		idx = append(idx, i)
	}
	sort.Slice(idx, func(a, b int) bool {
		return dot(q, vectors[idx[a]]) > dot(q, vectors[idx[b]])
	})
	if len(idx) > k {
		idx = idx[:k]
	}
	hits := make([]Hit, 0, len(idx))
	for _, i := range idx {
		hits = append(hits, Hit{Element: elements[i], Score: dot(q, vectors[i])})
	}
	return hits
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// FragmentSpec 生成片段的 spec 形态（与 pkgspec.Spec 同构的子集——直接进 merge 审查底座）。
type FragmentSpec struct {
	Name           string                 `json:"name,omitempty"`
	Concepts       []pkgspec.Concept      `json:"concepts,omitempty"`
	Relations      []pkgspec.Relation     `json:"relations,omitempty"`
	DataProperties []pkgspec.DataProperty `json:"data_properties,omitempty"`
	Axioms         []pkgspec.Axiom        `json:"axioms,omitempty"`
}

// ToSpec 转 pkgspec.Spec（补空 Instances 以过 Validate）。
func (f *FragmentSpec) ToSpec() *pkgspec.Spec {
	return &pkgspec.Spec{
		Name:           f.Name,
		Concepts:       f.Concepts,
		Relations:      f.Relations,
		DataProperties: f.DataProperties,
		Axioms:         f.Axioms,
		Instances:      []pkgspec.Instance{},
	}
}

// ValidateAgainstTarget 两阶段校验①：对「目标本体∪片段」合并视图做结构+引用完整性校验。
// 片段复用已有元素只引用名称（不重复声明）——对片段单独校验会误报悬空引用；
// LLM 若重复声明已有元素，合并视图撞名报「名称重复」，回喂修正即强复用约束。
// 目标本体已通过入库侧结构校验，合并视图报错均可归因于片段。
func ValidateAgainstTarget(target *pkgspec.Spec, frag *FragmentSpec) []pkgspec.ValidationError {
	union := &pkgspec.Spec{
		Name:           frag.Name,
		Concepts:       make([]pkgspec.Concept, 0, len(target.Concepts)+len(frag.Concepts)),
		Relations:      make([]pkgspec.Relation, 0, len(target.Relations)+len(frag.Relations)),
		DataProperties: make([]pkgspec.DataProperty, 0, len(target.DataProperties)+len(frag.DataProperties)),
		Axioms:         make([]pkgspec.Axiom, 0, len(target.Axioms)+len(frag.Axioms)),
		Instances:      []pkgspec.Instance{},
	}
	union.Concepts = append(union.Concepts, target.Concepts...)
	union.Concepts = append(union.Concepts, frag.Concepts...)
	union.Relations = append(union.Relations, target.Relations...)
	union.Relations = append(union.Relations, frag.Relations...)
	union.DataProperties = append(union.DataProperties, target.DataProperties...)
	union.DataProperties = append(union.DataProperties, frag.DataProperties...)
	union.Axioms = append(union.Axioms, target.Axioms...)
	union.Axioms = append(union.Axioms, frag.Axioms...)
	return union.Validate()
}

// Dedupe Integrator 去重（论文 Integrator 第一步：移除与目标本体重复的具名类/属性/公理；
// 前缀块在 spec_json 形态下无对应物——平台统一 URN 免命名空间冲突，去重项清单透出审查）。
// 返回（去重后片段, 被剔除项描述清单）。
func Dedupe(target *pkgspec.Spec, frag *FragmentSpec) (*FragmentSpec, []string) {
	cn := map[string]bool{}
	for _, c := range target.Concepts {
		cn[c.Name] = true
	}
	rn := map[string]bool{}
	for _, r := range target.Relations {
		rn[r.Name] = true
	}
	dn := map[string]bool{}
	for _, d := range target.DataProperties {
		dn[d.Name] = true
	}
	an := map[string]bool{}
	for _, a := range target.Axioms {
		an[a.Type+"|"+a.Subject] = true
	}
	out := &FragmentSpec{Name: frag.Name}
	var removed []string
	for _, c := range frag.Concepts {
		if cn[c.Name] {
			removed = append(removed, "概念（已存在）: "+c.Name)
			continue
		}
		cn[c.Name] = true // 片段内批内去重同口径
		out.Concepts = append(out.Concepts, c)
	}
	for _, r := range frag.Relations {
		if rn[r.Name] {
			removed = append(removed, "关系（已存在）: "+r.Name)
			continue
		}
		rn[r.Name] = true
		out.Relations = append(out.Relations, r)
	}
	for _, d := range frag.DataProperties {
		if dn[d.Name] {
			removed = append(removed, "数据属性（已存在）: "+d.Name)
			continue
		}
		dn[d.Name] = true
		out.DataProperties = append(out.DataProperties, d)
	}
	for _, a := range frag.Axioms {
		if an[a.Type+"|"+a.Subject] {
			removed = append(removed, fmt.Sprintf("公理（已存在）: %s/%s", a.Type, a.Subject))
			continue
		}
		an[a.Type+"|"+a.Subject] = true
		out.Axioms = append(out.Axioms, a)
	}
	return out, removed
}
