package importer

// REQ-378①/M174：Turtle Go 读入端单测——①ttlexport round-trip（导出→读入还原）②教学主路径
// （owl:Class/subClassOf/label/ObjectProperty domain·range/DatatypeProperty/个体断言）。

import (
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func TestTTLRoundTrip(t *testing.T) {
	sp := &pkgspec.Spec{
		Name: "roundtrip", Description: "对拍",
		Concepts: []pkgspec.Concept{
			{Name: "计算节点", Definition: "执行计算的节点"},
			{Name: "存储节点", Definition: "提供存储", Parents: []string{"计算节点"}},
		},
		Relations: []pkgspec.Relation{{Name: "挂载", From: "计算节点", To: "存储节点", Definition: "挂载关系"}},
		Instances: []pkgspec.Instance{
			{Name: "node-1", Concept: "计算节点", Attributes: map[string]any{"ip": "10.0.0.1"},
				Relations: []pkgspec.InstanceRel{{Rel: "挂载", Target: "stor-1"}}},
			{Name: "stor-1", Concept: "存储节点"},
		},
	}
	ttl, _, err := TTLGoExport("o1", sp)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatalf("Go 读入失败: %v", err)
	}
	if len(res.Spec.Concepts) != 2 {
		t.Fatalf("概念数不符: %d", len(res.Spec.Concepts))
	}
	byName := map[string]pkgspec.Concept{}
	for _, c := range res.Spec.Concepts {
		byName[c.Name] = c
	}
	cc := byName["计算节点"]
	if cc.Definition != "执行计算的节点" {
		t.Fatalf("定义还原不符: %+v", cc)
	}
	sn := byName["存储节点"]
	if len(sn.Parents) != 1 || sn.Parents[0] != "计算节点" {
		t.Fatalf("层级还原不符: %+v", sn)
	}
	if len(res.Spec.Relations) != 1 || res.Spec.Relations[0].From != "计算节点" || res.Spec.Relations[0].To != "存储节点" {
		t.Fatalf("关系还原不符: %+v", res.Spec.Relations)
	}
	if len(res.Spec.Instances) != 2 {
		t.Fatalf("实例数不符: %d", len(res.Spec.Instances))
	}
	for _, it := range res.Spec.Instances {
		if it.Name == "node-1" {
			if it.Attributes["ip"] != "10.0.0.1" {
				t.Fatalf("属性还原不符: %+v", it)
			}
			if len(it.Relations) != 1 || it.Relations[0].Target != "stor-1" {
				t.Fatalf("实例关系还原不符: %+v", it)
			}
		}
	}
}

func TestTTLTeachingMainPath(t *testing.T) {
	ttl := `
@prefix ex: <http://example.org/> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:Device a owl:Class ; rdfs:label "设备" ; rdfs:comment "物理设备" .
ex:Server a owl:Class ; rdfs:label "服务器" ; rdfs:subClassOf ex:Device .
ex:runs a owl:ObjectProperty ; rdfs:label "运行" ; rdfs:domain ex:Server ; rdfs:range ex:Service .
ex:coreCount a owl:DatatypeProperty ; rdfs:label "核数" ; rdfs:domain ex:Server .
ex:svc-a a ex:Service ; rdfs:label "服务A" .
ex:host-1 a ex:Server ; rdfs:label "主机一" ; ex:coreCount "16" ; ex:runs ex:svc-a .
`
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(res.Spec.Concepts) != 3 {
		t.Fatalf("概念应 3（含 Service 用法推断）: %+v", res.Spec.Concepts)
	}
	byName := map[string]pkgspec.Concept{}
	for _, c := range res.Spec.Concepts {
		byName[c.Name] = c
	}
	if len(byName["服务器"].Parents) != 1 || byName["服务器"].Parents[0] != "设备" {
		t.Fatalf("层级不符: %+v", byName["服务器"])
	}
	if byName["设备"].Definition != "物理设备" {
		t.Fatalf("定义不符: %+v", byName["设备"])
	}
	var runs *pkgspec.Relation
	for i := range res.Spec.Relations {
		if res.Spec.Relations[i].Name == "运行" {
			runs = &res.Spec.Relations[i]
		}
	}
	if runs == nil || runs.From != "服务器" || runs.To != "Service" { // Service 类无 label→local 名（实例 svc-a 才叫服务A）
		t.Fatalf("关系不符: %+v", res.Spec.Relations)
	}
	var inst *pkgspec.Instance
	for i := range res.Spec.Instances {
		if res.Spec.Instances[i].Name == "主机一" {
			inst = &res.Spec.Instances[i]
		}
	}
	if inst == nil || inst.Concept != "服务器" || inst.Attributes["核数"] != "16" {
		t.Fatalf("实例属性不符: %+v", inst)
	}
	if len(inst.Relations) != 1 || inst.Relations[0].Target != "服务A" {
		t.Fatalf("实例关系不符: %+v", inst)
	}
	if res.Recon.TriplesIn == 0 || res.Recon.TriplesIn != res.Recon.Vocabulary+res.Recon.Projected+res.Recon.Remainder {
		t.Fatalf("对账恒等式破: %+v", res.Recon)
	}
}

func TestTTLRestrictionRoundTrip(t *testing.T) {
	sp := &pkgspec.Spec{
		Name: "restr", Concepts: []pkgspec.Concept{
			{Name: "服务器", Restrictions: []pkgspec.Restriction{
				{Type: "min_cardinality", Property: "挂载", Value: 1},
				{Type: "some_values_from", Property: "挂载", Target: "存储节点"},
			}},
			{Name: "存储节点"},
		},
		Relations: []pkgspec.Relation{{Name: "挂载", From: "服务器", To: "存储节点"}},
	}
	ttl, _, err := TTLGoExport("o1", sp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ttl, "owl:Restriction") || !strings.Contains(ttl, "owl:minCardinality") {
		t.Fatalf("导出应含限制公理: %.400s", ttl)
	}
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatal(err)
	}
	var got *pkgspec.Concept
	for i := range res.Spec.Concepts {
		if res.Spec.Concepts[i].Name == "服务器" {
			got = &res.Spec.Concepts[i]
		}
	}
	if got == nil || len(got.Restrictions) != 2 {
		t.Fatalf("限制公理应还原 2 条: %+v", got)
	}
	if got.Restrictions[0].Type != "min_cardinality" || got.Restrictions[0].Value != 1 || got.Restrictions[0].Property != "挂载" {
		t.Fatalf("基数限制还原不符: %+v", got.Restrictions[0])
	}
	if got.Restrictions[1].Type != "some_values_from" || got.Restrictions[1].Target != "存储节点" {
		t.Fatalf("someValuesFrom 还原不符: %+v", got.Restrictions[1])
	}
}

func TestTTLUnregisteredPrefixFails(t *testing.T) {
	_, err := ReadTurtleSpec("ex:a ex:b ex:c .")
	if err == nil || !strings.Contains(err.Error(), "未声明前缀") {
		t.Fatalf("未声明前缀应报错: %v", err)
	}
}

// REQ-419/M215：同义词 round-trip——spec.Synonyms 导出 skos:altLabel、读入 altLabel 捕获回写
//（首个别名不再丢弃）；纯 altLabel 概念（无 prefLabel）名取 altLabel 首值、其余入 Synonyms。
func TestTTLSynonymRoundTrip(t *testing.T) {
	sp := &pkgspec.Spec{
		Name:     "syn",
		Concepts: []pkgspec.Concept{{Name: "客户", Synonyms: []string{"甲方", "委托方"}}},
	}
	ttl, _, err := TTLGoExport("o1", sp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ttl, `skos:altLabel "甲方"`) || !strings.Contains(ttl, `skos:altLabel "委托方"`) {
		t.Fatalf("导出应含 skos:altLabel: %s", ttl)
	}
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Spec.Concepts) != 1 {
		t.Fatalf("概念数: %d", len(res.Spec.Concepts))
	}
	c := res.Spec.Concepts[0]
	if c.Name != "客户" {
		t.Fatalf("名不符: %+v", c)
	}
	if len(c.Synonyms) != 2 {
		t.Fatalf("同义词应捕获 2 个: %+v", c.Synonyms)
	}
	got := map[string]bool{}
	for _, s := range c.Synonyms {
		got[s] = true
	}
	if !got["甲方"] || !got["委托方"] {
		t.Fatalf("同义词值不符: %+v", c.Synonyms)
	}
}

// REQ-472/M272：类级公理 round-trip（Go 读入端捕获 disjointWith/equivalentClass——原断裂点）。
func TestTTLAxiomRoundTrip_Req472(t *testing.T) {
	sp := &pkgspec.Spec{
		Name: "axm", Concepts: []pkgspec.Concept{
			{Name: "服务器"}, {Name: "存储节点"}, {Name: "网络设备"},
		},
		Relations: []pkgspec.Relation{{Name: "挂载", From: "服务器", To: "存储节点"}},
		Axioms: []pkgspec.Axiom{
			{Type: "disjoint_with", Subject: "服务器", Targets: []string{"存储节点", "网络设备"}},
			{Type: "equivalent_class", Subject: "网络设备", Targets: []string{"服务器"}},
		},
	}
	ttl, _, err := TTLGoExport("o1", sp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ttl, "owl:disjointWith") || !strings.Contains(ttl, "owl:equivalentClass") {
		t.Fatalf("导出应含类级公理: %.400s", ttl)
	}
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Spec.Axioms) != 2 {
		t.Fatalf("公理应还原 2 组，实际 %d 组 %v", len(res.Spec.Axioms), res.Spec.Axioms)
	}
	var dw, eq *pkgspec.Axiom
	for i := range res.Spec.Axioms {
		switch res.Spec.Axioms[i].Type {
		case "disjoint_with":
			dw = &res.Spec.Axioms[i]
		case "equivalent_class":
			eq = &res.Spec.Axioms[i]
		}
	}
	if dw == nil || dw.Subject != "服务器" || len(dw.Targets) != 2 {
		t.Fatalf("disjoint_with 还原不符: %+v", dw)
	}
	if eq == nil || eq.Subject != "网络设备" || len(eq.Targets) != 1 || eq.Targets[0] != "服务器" {
		t.Fatalf("equivalent_class 还原不符: %+v", eq)
	}
}

// REQ-473/M273：关系特性 round-trip（导出附加 rdf:type 行+owl:inverseOf，读入端捕获还原）。
func TestTTLRelationCharacteristicsRoundTrip_Req473(t *testing.T) {
	sp := &pkgspec.Spec{
		Name: "chrt", Concepts: []pkgspec.Concept{
			{Name: "服务器"}, {Name: "存储节点"},
		},
		Relations: []pkgspec.Relation{
			{Name: "挂载", From: "服务器", To: "存储节点", Characteristics: []string{"functional"}},
			{Name: "同驻", From: "服务器", To: "服务器", Characteristics: []string{"transitive", "symmetric"}, InverseOf: "挂载"},
		},
	}
	ttl, _, err := TTLGoExport("o1", sp)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"owl:FunctionalProperty", "owl:TransitiveProperty", "owl:SymmetricProperty", "owl:inverseOf"} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("导出应含 %s: %.500s", want, ttl)
		}
	}
	res, err := ReadTurtleSpec(ttl)
	if err != nil {
		t.Fatal(err)
	}
	var mount, cohost *pkgspec.Relation
	for i := range res.Spec.Relations {
		switch res.Spec.Relations[i].Name {
		case "挂载":
			mount = &res.Spec.Relations[i]
		case "同驻":
			cohost = &res.Spec.Relations[i]
		}
	}
	if mount == nil || len(mount.Characteristics) != 1 || mount.Characteristics[0] != "functional" {
		t.Fatalf("挂载特性还原不符: %+v", mount)
	}
	if cohost == nil || len(cohost.Characteristics) != 2 || cohost.InverseOf != "挂载" {
		t.Fatalf("同驻特性/逆属性还原不符: %+v", cohost)
	}
}
