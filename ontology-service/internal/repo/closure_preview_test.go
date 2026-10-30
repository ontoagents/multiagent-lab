package repo

// REQ-542/M349 构建面只读闭包预览测试：闭包图+解析后 schema（pin/head 解析）+环专项报告 +
// 悬空不阻断 + pin 冲突集 + 预览零写副作用。沿 release_test.go seedOntology 先例。

import (
	"errors"
	"reflect"
	"testing"
)

// mustSeedBump 建本体→写 spec→落 v1→bump→落 v2（pin 多版本场景用）。
func mustSeedBump(t *testing.T, st *Store, id, specV1, specV2 string) {
	t.Helper()
	if _, err := st.CreateOntology(id, id, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(id, "spec_json", specV1, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion(id, 1, specV1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(id, "spec_json", specV2, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion(id); err != nil || v != 2 {
		t.Fatalf("%s 应 bump 到 v2: v=%d err=%v", id, v, err)
	}
	if err := st.SaveVersion(id, 2, specV2, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestClosurePreview_GraphSchemaPin_Req542(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	mustSeedBump(t, st, "B", `{"name":"B","concepts":[{"name":"需求"},{"name":"共享"}]}`, `{"name":"B","concepts":[{"name":"需求项"}]}`)
	seedOntology(t, st, "C", `{"name":"C","concepts":[{"name":"用例"},{"name":"共享"}]}`)
	// A→B（import，pin v1）——B 头版本已演进 v2，预览按 pin 解析 v1 内容
	//（to_concept 指向目标真实概念=免悬空登记惯例，沿 external_refs_pin_test.go 先例）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "需求", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	// A→C（import，浮动）→ head v1
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "C", ToConcept: "用例", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// B→C（import，浮动）→ 闭包传递
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "B", FromConcept: OntologyLevelRef, ToOntoID: "C", ToConcept: "用例", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// A 对齐登记 C（纯登记不产装载边不入闭包）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: "缺陷", ToOntoID: "C", ToConcept: "用例", RefKind: "alignment", Predicate: "exactMatch"}); err != nil {
		t.Fatal(err)
	}
	// A 外部 IRI 哨兵（远端不拉取，D-O27 → remote_skipped）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToConcept: "http://xmlns.com/foaf/0.1/", RefKind: "import", Justification: "import_statement"}); err != nil {
		t.Fatal(err)
	}
	// 预览前基线（零写副作用对账面）
	versB0, err := st.ListVersions("B")
	if err != nil {
		t.Fatal(err)
	}
	refsA0, err := st.ListExternalRefs("A")
	if err != nil {
		t.Fatal(err)
	}
	// B 头版本已 v2：pin 解析后 pin 节点内容应=v1 快照（共享概念仍在 v1）
	prev, err := st.BuildClosurePreview("A")
	if err != nil {
		t.Fatalf("闭包预览不应失败: %v", err)
	}
	// ①全量闭包图：节点 BFS 序 root 在首、恰 A/B/C（alignment 不入闭包）
	if len(prev.Nodes) != 3 {
		t.Fatalf("节点应恰 3（A/B/C，alignment 与远端不入），实际 %+v", prev.Nodes)
	}
	if prev.Nodes[0].OntoID != "A" || prev.Nodes[0].Resolution != "root" || prev.Nodes[0].Version != 1 {
		t.Fatalf("root 节点应为 A/root/v1，实际 %+v", prev.Nodes[0])
	}
	byNode := map[string]ClosureNode{}
	for _, n := range prev.Nodes {
		byNode[n.OntoID] = n
	}
	if b := byNode["B"]; b.Resolution != "pin" || b.Version != 1 || b.Concepts != 2 {
		t.Fatalf("B 应 pin v1 且概念数 2（v1 快照内容），实际 %+v", b)
	}
	if c := byNode["C"]; c.Resolution != "head" || c.Version != 1 || c.Concepts != 2 {
		t.Fatalf("C 应 head v1 概念数 2，实际 %+v", c)
	}
	// 边：恰 3 条装载边（alignment 不产边）；A→B 与 A→C、B→C
	if len(prev.Edges) != 3 {
		t.Fatalf("装载边应恰 3（alignment 不入），实际 %+v", prev.Edges)
	}
	edgeKeys := map[string]bool{}
	for _, e := range prev.Edges {
		edgeKeys[e.FromOntoID+"→"+e.ToOntoID] = true
		if e.RefKind != "import" {
			t.Fatalf("边 ref_kind 应 import，实际 %+v", e)
		}
	}
	if !edgeKeys["A→B"] || !edgeKeys["A→C"] || !edgeKeys["B→C"] {
		t.Fatalf("边集应含 A→B/A→C/B→C: %v", edgeKeys)
	}
	if prev.Pins["B"] != 1 {
		t.Fatalf("Pins 应 B=1，实际 %v", prev.Pins)
	}
	if len(prev.RemoteSkipped) != 1 || prev.RemoteSkipped[0] != "http://xmlns.com/foaf/0.1/" {
		t.Fatalf("外部 IRI 应记 remote_skipped，实际 %v", prev.RemoteSkipped)
	}
	// ②解析后 schema：三节（root/pin/head）；B 节=v1 快照内容（含 v2 已删除的「共享」）
	if len(prev.Schema.Sections) != 3 {
		t.Fatalf("schema 分节应恰 3（missing 才无节），实际 %+v", prev.Schema.Sections)
	}
	var secB SchemaSection
	for _, sec := range prev.Schema.Sections {
		if sec.OntoID == "B" {
			secB = sec
		}
	}
	if secB.OntoID != "B" || secB.Resolution != "pin" || secB.Version != 1 || len(secB.Concepts) != 2 {
		t.Fatalf("B 分节应为 pin/v1/2 概念，实际 %+v", secB)
	}
	names := map[string]bool{}
	for _, n := range prev.Schema.ConceptNames {
		names[n] = true
	}
	for _, want := range []string{"缺陷", "需求", "用例", "共享"} {
		if !names[want] {
			t.Fatalf("合成视图名单应含 %q: %v", want, prev.Schema.ConceptNames)
		}
	}
	if prev.Schema.TotalConcepts != 5 {
		t.Fatalf("TotalConcepts 应 5（A1+B2+C2），实际 %d", prev.Schema.TotalConcepts)
	}
	// 跨本体同名冲突：B/C 都声明「共享」
	foundConflict := false
	for _, cf := range prev.Schema.NameConflicts {
		if cf.Name == "共享" && reflect.DeepEqual(cf.OntoIDs, []string{"B", "C"}) {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("应报告 B/C 同名概念「共享」冲突，实际 %+v", prev.Schema.NameConflicts)
	}
	// ③环报告：无环=空数组非 null
	if prev.Cycles == nil || len(prev.Cycles) != 0 {
		t.Fatalf("无环场景 cycles 应为空数组，实际 %v", prev.Cycles)
	}
	// 预览零写副作用：版本快照行数与引用登记数均不变
	versB1, err := st.ListVersions("B")
	if err != nil {
		t.Fatal(err)
	}
	refsA1, err := st.ListExternalRefs("A")
	if err != nil {
		t.Fatal(err)
	}
	if len(versB1) != len(versB0) || len(refsA1) != len(refsA0) {
		t.Fatalf("预览不得产生写副作用: 版本行 %d→%d 引用行 %d→%d", len(versB0), len(versB1), len(refsA0), len(refsA1))
	}
}

func TestClosurePreview_CycleReport_Req542(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	seedOntology(t, st, "B", `{"name":"B","concepts":[{"name":"需求"}]}`)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "需求", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "B", FromConcept: OntologyLevelRef, ToOntoID: "A", ToConcept: "缺陷", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	prev, err := st.BuildClosurePreview("A")
	if err != nil {
		t.Fatalf("互引环预览不得失败（环=并集语义非阻断）: %v", err)
	}
	if len(prev.Dangling) != 0 {
		t.Fatalf("互引环测试不应混入悬空面（登记形态失真），实际 %v", prev.Dangling)
	}
	if len(prev.Nodes) != 2 {
		t.Fatalf("环闭包节点应 A/B 双入（visited 收敛），实际 %+v", prev.Nodes)
	}
	if len(prev.Cycles) != 1 {
		t.Fatalf("应报告恰 1 条环路径，实际 %v", prev.Cycles)
	}
	cyc := prev.Cycles[0]
	if len(cyc) != 3 || cyc[0] != "A" || cyc[1] != "B" || cyc[2] != "A" {
		t.Fatalf("环路径应 A→B→A（回边闭合），实际 %v", cyc)
	}
}

func TestClosurePreview_DanglingMissingNotBlocking_Req542(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	seedOntology(t, st, "B", `{"name":"B","concepts":[{"name":"需求"}]}`)
	seedOntology(t, st, "C", `{"name":"C","concepts":[{"name":"用例"}]}`)
	// A→B（本体级 import，to_concept 指向真实概念→免悬空；目标本体存在可解析内容）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "需求", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// A→C 概念级引用悬空（概念不存在）——报告不阻断
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: "缺陷", ToOntoID: "C", ToConcept: "NoSuchConcept", RefKind: "reuse"}); err != nil {
		t.Fatal(err)
	}
	// B→D（先建 D 登记、后删 D→missing_ontology；B 不沿孤儿引用再外扩）
	seedOntology(t, st, "D", `{"name":"D","concepts":[{"name":"临时"}]}`)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "B", FromConcept: OntologyLevelRef, ToOntoID: "D", ToConcept: "临时", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOntology("D"); err != nil {
		t.Fatal(err)
	}
	prev, err := st.BuildClosurePreview("A")
	if err != nil {
		t.Fatalf("悬空引用预览不得阻断（运行面门禁才阻断）: %v", err)
	}
	byNode := map[string]ClosureNode{}
	for _, n := range prev.Nodes {
		byNode[n.OntoID] = n
	}
	// D 已删：节点 missing、无内容、B 不沿孤儿引用再外扩（无内容可解析）
	if d := byNode["D"]; d.Resolution != "missing" || d.Version != 0 || d.SpecSHA256 != "" {
		t.Fatalf("已删目标应 missing/v0/无指纹，实际 %+v", d)
	}
	// missing_concept：目标本体存在，节点仍 head 可解析
	if c := byNode["C"]; c.Resolution != "head" || c.Concepts != 1 {
		t.Fatalf("概念悬空但本体存在应 head 可解析，实际 %+v", c)
	}
	// 悬空专项报告：两条（B→D missing_ontology + A→C missing_concept）
	if len(prev.Dangling) != 2 {
		t.Fatalf("悬空报告应 2 条，实际 %+v", prev.Dangling)
	}
	codes := map[string]int{}
	for _, d := range prev.Dangling {
		codes[d.Code]++
		if d.Detail == "" {
			t.Fatalf("悬空条目应带人话明细，实际 %+v", d)
		}
	}
	if codes["missing_ontology"] != 1 || codes["missing_concept"] != 1 {
		t.Fatalf("悬空码应各 1 条，实际 %v", codes)
	}
	// missing 节点无 schema 分节；内容节恰 A/B/C
	if len(prev.Schema.Sections) != 3 {
		t.Fatalf("schema 分节应恰 3（D missing 无节），实际 %+v", prev.Schema.Sections)
	}
}

func TestClosurePreview_PinConflict_Req542(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	mustSeedBump(t, st, "D", `{"name":"D","concepts":[{"name":"概念甲"},{"name":"共通"}]}`, `{"name":"D","concepts":[{"name":"概念乙"},{"name":"共通"}]}`)
	// A→D 两条登记 pin 不一致（v1/v2）：解析取最低 v1（最保守），冲突集如实呈现
	//（to_concept 均指向各自 pin 快照内存在的概念→免悬空噪音，纯测 pin 收敛面）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: "缺陷", ToOntoID: "D", ToConcept: "概念乙", RefKind: "reuse", PinVersion: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "D", ToConcept: "概念甲", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	prev, err := st.BuildClosurePreview("A")
	if err != nil {
		t.Fatal(err)
	}
	if prev.Pins["D"] != 1 {
		t.Fatalf("同目标多 pin 应取最低 1，实际 %v", prev.Pins)
	}
	if prev.PinConflicts["D"] != "v1/v2" {
		t.Fatalf("冲突集应为 v1/v2（版本升序），实际 %v", prev.PinConflicts)
	}
	// pin v1 解析：D 节点内容=概念甲+共通（v1 快照）
	for _, sec := range prev.Schema.Sections {
		if sec.OntoID == "D" {
			if sec.Resolution != "pin" || sec.Version != 1 || len(sec.Concepts) != 2 || sec.Concepts[0].Name != "概念甲" {
				t.Fatalf("D 分节应 pin/v1/概念甲+共通，实际 %+v", sec)
			}
			return
		}
	}
	t.Fatalf("D 应有 schema 分节，实际 %+v", prev.Schema.Sections)
}

func TestClosurePreview_NoRefsMinimalShape_Req542(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "Solo", `{"name":"Solo","concepts":[{"name":"独行"}]}`)
	prev, err := st.BuildClosurePreview("Solo")
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.Nodes) != 1 || prev.Nodes[0].Resolution != "root" {
		t.Fatalf("无引用本体节点应仅 root，实际 %+v", prev.Nodes)
	}
	if len(prev.Edges) != 0 || prev.Edges == nil {
		t.Fatalf("边应为空数组非 null，实际 %v", prev.Edges)
	}
	if len(prev.Schema.Sections) != 1 || len(prev.Schema.ConceptNames) != 1 {
		t.Fatalf("schema 应单节单概念，实际 %+v / %v", prev.Schema.Sections, prev.Schema.ConceptNames)
	}
	if prev.Pins != nil || prev.PinConflicts != nil || prev.Dangling != nil || prev.RemoteSkipped != nil {
		t.Fatalf("无引用场景可选面应为空: %+v", prev)
	}
	if _, err := st.BuildClosurePreview("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在本体应 ErrNotFound，实际 %v", err)
	}
}
