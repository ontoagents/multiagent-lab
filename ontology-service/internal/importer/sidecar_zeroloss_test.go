package importer

// REQ-328/M130 零丢失对账改造：用法推断补声明 + 全量余量账本 + 对账恒等式 + canon 直挂。
// sidecar 真机驱动（无 python3/rdflib 环境 skip，沿 sidecar_skos_test.go 口径）。

import (
	"strings"
	"testing"
)

// 纯 ABox 形态（77 号触发案例缩影）：类型只被使用未声明、个体无 owl:NamedIndividual 标记、
// 含 xsd:decimal 字面量（REQ-328/E 崩溃顺修点）。
const aboxFixture = `@prefix ex: <http://fastq.example/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
ex:ReadType a ex:SeqType .
ex:r1 a ex:ReadType ; ex:length 150 ; ex:gc "0.53"^^xsd:decimal ; ex:sample ex:s1 .
ex:r2 a ex:ReadType ; ex:length 89 .
ex:s1 a ex:Sample ; ex:name "SRR000001" .
`

// 标准声明形态回归（A 推断不得干扰既有投影）。
const richFixture = `@prefix ex: <http://rich.example/> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
ex:Disease a owl:Class ; rdfs:label "疾病" .
ex:Drug a owl:Class ; rdfs:label "药物" .
ex:treats a owl:ObjectProperty ; rdfs:domain ex:Drug ; rdfs:range ex:Disease .
ex:penicillin a owl:NamedIndividual ; a ex:Drug .
`

// A：纯 ABox 用法推断——概念/实例/数据属性三推断补齐（投影从 0/0/0 到全捕获）。
func TestUsageInferenceABox(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := Import(sc, "abox.ttl", aboxFixture)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(sp.Concepts) != 3 { // ReadType/SeqType/Sample 均被用作类型 → 全部推断为概念（ReadType 兼为 r1 的类型）
		t.Fatalf("推断概念数期望 3 得到 %d: %+v", len(sp.Concepts), sp.Concepts)
	}
	if len(sp.Instances) != 3 {
		t.Fatalf("推断实例数期望 3 得到 %d", len(sp.Instances))
	}
	// 实例属性（length/gc/name/sample 谓词）→ 数据属性声明推断
	if len(sp.DataProperties) < 3 {
		t.Fatalf("推断数据属性期望 ≥3 得到 %d", len(sp.DataProperties))
	}
	if rep.Reconciliation == nil {
		t.Fatal("对账块缺失")
	}
	if rep.Reconciliation.Unaccounted != 0 {
		t.Fatalf("对账恒等式破坏: %+v", rep.Reconciliation)
	}
	// 余量恰 1 条=ReadType a SeqType（类作为实例的元建模断言——spec 形态表达不了，
	// 余量账本保真+导出还原即 REQ-328 的正确行为，非投影缺陷）
	if rep.Reconciliation.Remainder != 1 {
		t.Fatalf("元建模断言应入余量（恰 1 条）: %d", rep.Reconciliation.Remainder)
	}
	if len(rep.Reconciliation.Inferred) == 0 {
		t.Fatal("推断清单应非空（明示非文件原生）")
	}
}

// E：xsd:decimal 不再崩溃（Decimal → float），值保真。
func TestDecimalLiteralNoCrash(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, _, err := Import(sc, "dec.ttl", aboxFixture)
	if err != nil {
		t.Fatalf("decimal 字面量导入崩溃（REQ-328/E 顺修失效）: %v", err)
	}
	for _, it := range sp.Instances {
		if v, ok := it.Attributes["gc"]; ok {
			if f, ok := v.(float64); !ok || f < 0.52 || f > 0.54 {
				t.Fatalf("decimal 值异常: %v (%T)", v, v)
			}
		}
	}
}

// 标准声明形态回归：显式投影不受推断干扰（概念不重复、关系/实例/对账全保真）。
func TestRichFormNoRegression(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := Import(sc, "rich.ttl", richFixture)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(sp.Concepts) != 2 || len(sp.Relations) != 1 || len(sp.Instances) != 1 {
		t.Fatalf("标准形态投影回归: %d/%d/%d", len(sp.Concepts), len(sp.Relations), len(sp.Instances))
	}
	if sp.Relations[0].From != "药物" || sp.Relations[0].To != "疾病" {
		t.Fatalf("关系投影回归: %+v", sp.Relations[0])
	}
	if rep.Reconciliation == nil || rep.Reconciliation.Unaccounted != 0 {
		t.Fatalf("对账块缺失或恒等式破坏: %+v", rep.Reconciliation)
	}
}

// B：余量账本——不可投影语义（BNode 限制结构）全量入账无上限。
func TestRemainderLedgerCapturesBlankRestrictions(t *testing.T) {
	sc := sidecarAvailable(t)
	fixture := `@prefix ex: <http://bnode.example/> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
ex:Thing a owl:Class .
ex:Thing rdfs:subClassOf [ a owl:Restriction ; owl:onProperty ex:hasPart ; owl:someValuesFrom ex:Part ] .
`
	sp, rep, err := Import(sc, "bnode.ttl", fixture)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(sp.Concepts) != 1 {
		t.Fatalf("概念数期望 1: %d", len(sp.Concepts))
	}
	if rep.Reconciliation == nil || rep.Reconciliation.Remainder == 0 {
		t.Fatalf("BNode 限制结构应入余量账本: %+v", rep.Reconciliation)
	}
	if len(rep.Unprojected) < 3 {
		t.Fatalf("余量应含限制结构链（restriction/onProperty/someValuesFrom ≥3 行）: %d", len(rep.Unprojected))
	}
}

// canon：original 全图 canonical 直出 + 精确计数（C 直挂数据源）。
func TestCanonicalTTL(t *testing.T) {
	sc := sidecarAvailable(t)
	ttl, n, err := CanonicalTTL(sc, "turtle", richFixture)
	if err != nil {
		t.Fatalf("canon 失败: %v", err)
	}
	if n != 9 { // rich fixture 9 三元组（2 概念×2 + treats×3 + penicillin 类型×2）
		t.Fatalf("canon 计数期望 9 得到 %d", n)
	}
	if !strings.Contains(ttl, "treats") {
		t.Fatal("canon 输出应保留原语义内容")
	}
}
