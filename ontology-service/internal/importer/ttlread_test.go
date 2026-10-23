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

func TestTTLUnregisteredPrefixFails(t *testing.T) {
	_, err := ReadTurtleSpec("ex:a ex:b ex:c .")
	if err == nil || !strings.Contains(err.Error(), "未声明前缀") {
		t.Fatalf("未声明前缀应报错: %v", err)
	}
}
