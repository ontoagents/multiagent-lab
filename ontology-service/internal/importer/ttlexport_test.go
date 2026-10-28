package importer

// TTLGoExport 测试面：
//   - TestTTLGoExportShapeAndCount：无 python 单测——精确输出串 + 算术计数（导出路径脱离 python 的验收面）；
//   - TestTTLGoExportParityWithSidecar：python+rdflib 在场时与 sidecar.py export 逐字节比对
//     （TTL 正文 + TRIPLES 计数双断言；rdflib 缺失自动 skip——对拍是迁移安全网，不是运行依赖）。

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func paritySpec() *pkgspec.Spec {
	return &pkgspec.Spec{
		Name:        "parity",
		Description: "对拍夹具",
		Concepts: []pkgspec.Concept{
			{Name: "Person", Definition: "人类", Parents: []string{"Agent"}},
			{Name: "带 空格/概念.1", Parents: []string{"Person"}},
		},
		Relations: []pkgspec.Relation{
			{Name: "knows", From: "Person", To: "Person"},
			{Name: "orphan_rel"},
		},
		DataProperties: []pkgspec.DataProperty{
			{Name: "age", Domain: "Person", Range: "integer"},
			{Name: "homepage", Domain: "Person", Range: "https://example.org/ns#hp"},
			{Name: "remark", Range: ""},
		},
		Axioms: []pkgspec.Axiom{
			{Type: "disjoint_with", Subject: "Person", Targets: []string{"Agent", "Robot"}},
			{Type: "equivalent_class", Subject: "Person", Targets: []string{"Human"}},
		},
		Instances: []pkgspec.Instance{
			{Name: "alice", Concept: "Person",
				Attributes: map[string]any{"name": "Alice", "age": "30"},
				Relations:  []pkgspec.InstanceRel{{Rel: "knows", Target: "bob"}}},
			{Name: "bob", Attributes: map[string]any{"score": 4.5, "active": true, "note": nil}},
		},
	}
}

func TestTTLGoExportShapeAndCount(t *testing.T) {
	ttl, n, err := TTLGoExport("rt", paritySpec())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	wantCount := 0
	// Person: type+label+comment+subClassOf = 4；带 空格/概念.1: type+label+subClassOf = 3
	wantCount += 4 + 3
	// knows: type+label+domain+range = 4；orphan_rel: type+label = 2
	wantCount += 4 + 2
	// age: type+label+domain+range = 4；homepage: 4；remark: type+label+range = 3
	wantCount += 4 + 4 + 3
	// 公理：2 + 1
	wantCount += 2 + 1
	// alice: type+class+label+2attr+1rel = 6；bob: type+label+3attr = 5
	wantCount += 6 + 5
	// REQ-474：owl:Ontology 头 = type+label+comment = 3
	wantCount += 3
	if n != wantCount {
		t.Fatalf("计数不符: got %d want %d", n, wantCount)
	}
	for _, want := range []string{
		"@prefix o: <urn:o:rt:> .",
		"<urn:o:rt:concept:Person> rdf:type owl:Class ; rdfs:label \"Person\" ;\n    rdfs:comment \"人类\" ;\n    rdfs:subClassOf <urn:o:rt:concept:Agent> .",
		"<urn:o:rt:relation:orphan_rel> rdf:type owl:ObjectProperty ; rdfs:label \"orphan_rel\" .",
		"<urn:o:rt:attr:homepage> rdf:type owl:DatatypeProperty ; rdfs:label \"homepage\" ;\n    rdfs:domain <urn:o:rt:concept:Person> ;\n    rdfs:range <https://example.org/ns#hp> .",
		"<urn:o:rt:attr:remark> rdf:type owl:DatatypeProperty ; rdfs:label \"remark\" ;\n    rdfs:range xsd:string .",
		"<urn:o:rt:concept:Person> owl:disjointWith <urn:o:rt:concept:Agent>, <urn:o:rt:concept:Robot> .",
		"<urn:o:rt:concept:Person> owl:equivalentClass <urn:o:rt:concept:Human> .",
		"<urn:o:rt:instance:alice> rdf:type owl:NamedIndividual ;\n    rdf:type <urn:o:rt:concept:Person> ;\n    rdfs:label \"alice\" ;\n    <urn:o:rt:attr:age> 30 ;\n    <urn:o:rt:attr:name> \"Alice\" ;\n    <urn:o:rt:relation:knows> <urn:o:rt:instance:bob> .",
		"<urn:o:rt:instance:bob> rdf:type owl:NamedIndividual ;\n    rdfs:label \"bob\" ;\n    <urn:o:rt:attr:active> \"True\" ;\n    <urn:o:rt:attr:note> \"None\" ;\n    <urn:o:rt:attr:score> \"4.5\" .",
		"<urn:o:rt:concept:带_空格_概念.1> rdf:type owl:Class ; rdfs:label \"带 空格/概念.1\" ;\n    rdfs:subClassOf <urn:o:rt:concept:Person> .",
	} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("输出缺少期望片段:\n%s\n--- 实际 ---\n%s", want, ttl)
		}
	}
}

func TestTTLGoExportEmptySpec(t *testing.T) {
	ttl, n, err := TTLGoExport("rt", &pkgspec.Spec{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if n != 0 {
		t.Fatalf("空 spec 计数应为 0, got %d", n)
	}
	if !strings.HasPrefix(ttl, "@prefix o: <urn:o:rt:> .") || strings.Contains(ttl, "owl:Class") {
		t.Fatalf("空 spec 应仅含前缀块: %q", ttl)
	}
}

// sidecarRun 调 sidecar.py export；返回 stdout TTL、stderr TRIPLES 计数。rdflib 缺失时 skip=true。
// REQ-479：refs 非空时注入 stdin JSON 的 external_refs 键（旁挂表投影，spec 模型不动）。
func sidecarRun(t *testing.T, ontologyID string, sp *pkgspec.Spec, refs []ExtRefBridge) (string, int, bool) {
	t.Helper()
	script := filepath.Join("..", "..", "..", "tools", "rdf-sidecar", "sidecar.py")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("sidecar.py 不可达: %v", err)
	}
	py := "python"
	if _, err := exec.LookPath(py); err != nil {
		py = "python3"
		if _, err := exec.LookPath(py); err != nil {
			t.Skip("系统无 python（对拍是迁移安全网，非运行依赖）")
		}
	}
	var payload map[string]any
	if b, err := json.Marshal(sp); err != nil {
		t.Fatalf("marshal: %v", err)
	} else if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(refs) > 0 {
		payload["external_refs"] = refs
	}
	bts, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	cmd := exec.Command(py, script, "export", "--ontology-id", ontologyID)
	cmd.Stdin = bytes.NewReader(bts)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if strings.Contains(errb.String(), "rdflib") {
			t.Skipf("rdflib 未安装（对拍 skip）: %s", strings.TrimSpace(errb.String()))
		}
		t.Fatalf("sidecar export: %v; stderr: %s", err, errb.String())
	}
	n := -1
	if m := regexp.MustCompile(`TRIPLES (\d+)`).FindStringSubmatch(errb.String()); m != nil {
		n, _ = strconv.Atoi(m[1])
	}
	return out.String(), n, false
}

// bridgeRefs REQ-479 对拍与单测共用的桥登记夹具：仓内 alignment/import/reuse+外部 IRI
// 对齐+本体级哨兵 imports+去重目标+脏外部 IRI（防御性跳过）。
func bridgeRefs() []ExtRefBridge {
	return []ExtRefBridge{
		{FromConcept: "Person", ToOntoID: "other", ToConcept: "Human", RefKind: "alignment", Predicate: "exactMatch"},
		{FromConcept: "Person", ToConcept: "http://example.org/vocab#Thing", RefKind: "alignment", Predicate: "closeMatch"},
		{FromConcept: "Person", ToOntoID: "dep", ToConcept: "Widget", RefKind: "import"},
		{FromConcept: "带 空格/概念.1", ToOntoID: "dep", ToConcept: "Sensor", RefKind: "reuse"},
		{FromConcept: ontologyLevelRef, ToOntoID: "dep", ToConcept: "dep", RefKind: "import"},
		{FromConcept: ontologyLevelRef, ToOntoID: "other", ToConcept: "other", RefKind: "import"},
		{FromConcept: "Person", ToOntoID: "other", ToConcept: "Human", RefKind: "import"}, // imports 目标去重
		{FromConcept: "Person", ToConcept: "not an iri", RefKind: "alignment"},            // 脏外部 IRI 跳过
	}
}

func TestTTLGoExportParityWithSidecar(t *testing.T) {
	cases := []struct {
		oid  string
		sp   *pkgspec.Spec
		refs []ExtRefBridge
	}{
		{"rt", paritySpec(), nil},
		{"empty", &pkgspec.Spec{}, nil},
		{"onto_261015135935253672200", &pkgspec.Spec{Name: "legacy-id", Concepts: []pkgspec.Concept{{Name: "A"}}}, nil},
		{"bridge", paritySpec(), bridgeRefs()}, // REQ-479：桥三元组+owl:imports 双侧字节级对拍
	}
	for _, tc := range cases {
		goTTL, goN, err := TTLGoExport(tc.oid, tc.sp, tc.refs...)
		if err != nil {
			t.Fatalf("[%s] go export: %v", tc.oid, err)
		}
		pyTTL, pyN, skip := sidecarRun(t, tc.oid, tc.sp, tc.refs)
		if skip {
			return // 整组对拍 skip（rdflib 缺失）
		}
		// Windows python stdout 文本态会把 \n 译成 \r\n（triplesRe 的 \r? 即此由来）——对拍按 LF 归一
		pyTTL = strings.ReplaceAll(pyTTL, "\r\n", "\n")
		if goTTL != pyTTL {
			t.Fatalf("[%s] TTL 与 sidecar 输出不一致:\n--- go ---\n%s\n--- sidecar ---\n%s", tc.oid, goTTL, pyTTL)
		}
		if goN != pyN {
			t.Fatalf("[%s] 计数不一致: go=%d sidecar=%d", tc.oid, goN, pyN)
		}
	}
}

// REQ-474/M274：TTL 导出保真三修——①rdfs:label 导出 Label 字段 ②实例断言按 range 类型化
//（boolean 真小写/integer 裸数字/date ^^xsd:date——修 True/False/None 非法字面量）③owl:Ontology 头。
func TestTTLExportFidelity_Req474(t *testing.T) {
	sp := &pkgspec.Spec{
		Name: "保真", Description: "描述",
		Concepts: []pkgspec.Concept{{Name: "Device", Label: "设备"}},
		Relations: []pkgspec.Relation{{Name: "connects", From: "Device", To: "Device", Label: "连接"}},
		DataProperties: []pkgspec.DataProperty{
			{Name: "online", Domain: "Device", Range: "boolean"},
			{Name: "port", Domain: "Device", Range: "integer"},
			{Name: "score", Domain: "Device", Range: "number"},
			{Name: "since", Domain: "Device", Range: "date"},
		},
		Instances: []pkgspec.Instance{{Name: "dev1", Concept: "Device",
			Attributes: map[string]any{"online": true, "port": "8080", "score": 4.5, "since": "2026-01-01"}}},
	}
	ttl, _, err := TTLGoExport("fid", sp)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"rdf:type owl:Ontology",                    // ①本体头
		"rdfs:comment \"描述\"",                     // 头部描述
		"rdfs:label \"设备\"",                       // ②概念展示名
		"rdfs:label \"连接\"",                       // ②关系展示名
		"<urn:o:fid:attr:online> true",             // ③boolean 真小写（原 "True" 非法字面量）
		"<urn:o:fid:attr:port> 8080",               // ③integer 裸数字（字符串 "8080" 也类型化）
		"<urn:o:fid:attr:score> 4.5",               // ③number
		"<urn:o:fid:attr:since> \"2026-01-01\"^^xsd:date", // ③date
	} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("导出缺少 %q:\n%s", want, ttl)
		}
	}
	if strings.Contains(ttl, "True") || strings.Contains(ttl, "None") {
		t.Fatalf("不应再出现 python str() 非法字面量形态:\n%s", ttl)
	}
}

// REQ-479/M279：external_refs 导出桥——alignment skos 物化（异源轨）/import·reuse seeAlso 被引方
// IRI 原样出现（同源轨 join 键）/owl:imports 头语句去重升序/脏外部 IRI 防御跳过。
func TestTTLExportBridge_Req479(t *testing.T) {
	ttl, n, err := TTLGoExport("rt", paritySpec(), bridgeRefs()...)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, want := range []string{
		"@prefix skos: <http://www.w3.org/2004/02/skos/core#> .",
		"<urn:o:rt:__ontology> rdf:type owl:Ontology ; rdfs:label \"parity\" ;\n    rdfs:comment \"对拍夹具\" ;\n    owl:imports <urn:o:dep:> ;\n    owl:imports <urn:o:other:> .",
		"<urn:o:rt:concept:Person> rdf:type owl:Class ; rdfs:label \"Person\" ;\n    rdfs:comment \"人类\" ;\n    rdfs:subClassOf <urn:o:rt:concept:Agent> ;\n    skos:exactMatch <urn:o:other:concept:Human> ;\n    skos:closeMatch <http://example.org/vocab#Thing> ;\n    rdfs:seeAlso <urn:o:dep:concept:Widget> ;\n    rdfs:seeAlso <urn:o:other:concept:Human> .",
		"<urn:o:rt:concept:带_空格_概念.1> rdf:type owl:Class ; rdfs:label \"带 空格/概念.1\" ;\n    rdfs:subClassOf <urn:o:rt:concept:Person> ;\n    rdfs:seeAlso <urn:o:dep:concept:Sensor> .",
	} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("输出缺少期望片段:\n%s\n--- 实际 ---\n%s", want, ttl)
		}
	}
	if strings.Contains(ttl, "not an iri") {
		t.Fatalf("脏外部 IRI 应防御性跳过:\n%s", ttl)
	}
	// 基线 41（TestTTLGoExportShapeAndCount 同夹具 wantCount）+ imports 2 + Person 桥段 4 + 空格概念桥段 1
	if n != 48 {
		t.Fatalf("计数不符: got %d want %d", n, 48)
	}
	// 对偶断言：导出的 owl:imports 可被装载侧探测回收（REQ-262 detectExternalImports 同源）
	got := detectExternalImports(ttl)
	if len(got) != 2 || got[0] != "urn:o:dep:" || got[1] != "urn:o:other:" {
		t.Fatalf("owl:imports 装载侧探测对偶失败: %v", got)
	}
}

// REQ-479/M279 边缘：空 predicate 回落 exactMatch；空 spec（空头）哨兵 import 不出语句、计数 0
//（空 spec 仅前缀计数 0 的装载对账不变量保持）。
func TestTTLExportBridgeEdge_Req479(t *testing.T) {
	sp := &pkgspec.Spec{Name: "边", Concepts: []pkgspec.Concept{{Name: "A"}}}
	ttl, n, err := TTLGoExport("rt", sp, ExtRefBridge{FromConcept: "A", ToOntoID: "b", ToConcept: "B", RefKind: "alignment"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ttl, "skos:exactMatch <urn:o:b:concept:B>") {
		t.Fatalf("空 predicate 应回落 exactMatch:\n%s", ttl)
	}
	if n != 5 { // 头 2 + A 块 type+label 2 + 桥段 1
		t.Fatalf("计数不符: got %d want 5", n)
	}
	ttl2, n2, err := TTLGoExport("rt", &pkgspec.Spec{},
		ExtRefBridge{FromConcept: ontologyLevelRef, ToOntoID: "b", ToConcept: "b", RefKind: "import"})
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 || strings.Contains(ttl2, "owl:imports") {
		t.Fatalf("空 spec 空头应跳过 imports 且计数 0: n=%d ttl=%q", n2, ttl2)
	}
}
