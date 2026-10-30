package repo

// REQ-540/M347：external_refs 版本 pin 单测——pin 校验/刷新（显式刷新才漂移）/pin 感知悬空
// 检测（引用解析点按 pin 解析：被引方头版本演进不再触发悬空误报）/pin 边投影。

import (
	"path/filepath"
	"strings"
	"testing"
)

// pinFixture dst 两版本历史：v1 spec 含「需求」，v2（头）改名为「需求项」——
// 语义漂移场景（被引方头版本演进，pin v1 的引用方不应感知为悬空）。
func pinFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "pin.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("src", "源本体", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOntology("src2", "源本体二", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOntology("dst", "目标本体", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("src", "spec_json", `{"concepts":[{"name":"缺陷"}]}`, true); err != nil {
		t.Fatal(err)
	}
	specV1 := `{"concepts":[{"name":"需求"}]}`
	specV2 := `{"concepts":[{"name":"需求项"}]}`
	if err := st.PutArtifact("dst", "spec_json", specV1, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion("dst", 1, specV1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("dst", "spec_json", specV2, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("dst"); err != nil || v != 2 {
		t.Fatalf("dst 应 bump 到 v2: v=%d err=%v", v, err)
	}
	if err := st.SaveVersion("dst", 2, specV2, "", ""); err != nil {
		t.Fatal(err)
	}
	return st, "src", "dst"
}

func TestExternalRefPinValidation_Req540(t *testing.T) {
	st, src, dst := pinFixture(t)
	// 负数拒绝
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: -1}); err == nil || !contains(err.Error(), "pin_version") {
		t.Fatalf("负 pin 应拒绝: %v", err)
	}
	// 外部 IRI 登记态无版本可钉
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToConcept: "http://ex.org/v", RefKind: "import", PinVersion: 1}); err == nil || !contains(err.Error(), "无版本可钉") {
		t.Fatalf("外部 IRI pin 应拒绝: %v", err)
	}
	// pin 版本快照不存在（超前）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: 9}); err == nil || !contains(err.Error(), "无版本快照") {
		t.Fatalf("超前 pin 应拒绝: %v", err)
	}
	// 合法 pin 落库回读
	r, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: 1})
	if err != nil {
		t.Fatalf("合法 pin 登记失败: %v", err)
	}
	refs, err := st.ListExternalRefs(src)
	if err != nil || len(refs) != 1 || refs[0].PinVersion != 1 {
		t.Fatalf("pin 应回读: %v %+v", err, refs)
	}
	// 无 pin 默认 0（浮动头版本，零迁移向后兼容）
	r2, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "reuse"})
	if err != nil {
		t.Fatal(err)
	}
	refs, _ = st.ListExternalRefs(src)
	for _, ref := range refs {
		if ref.RefID == r2.RefID && ref.PinVersion != 0 {
			t.Fatalf("缺省 pin 应为 0，实际 %d", ref.PinVersion)
		}
	}
	_ = r
}

func TestSetExternalRefPin_Req540(t *testing.T) {
	st, src, dst := pinFixture(t)
	r, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import"})
	if err != nil {
		t.Fatal(err)
	}
	// 显式刷新 pin 1→2
	got, err := st.SetExternalRefPin(src, r.RefID, 2)
	if err != nil || got.PinVersion != 2 {
		t.Fatalf("刷新 pin 应成功: %v %+v", err, got)
	}
	refs, _ := st.ListExternalRefs(src)
	if refs[0].PinVersion != 2 {
		t.Fatalf("刷新应落库: %d", refs[0].PinVersion)
	}
	// 清除（0=回浮动头版本）
	if got, err = st.SetExternalRefPin(src, r.RefID, 0); err != nil || got.PinVersion != 0 {
		t.Fatalf("清除 pin 应成功: %v %+v", err, got)
	}
	// pin 超前拒绝（刷新口径与登记一致）
	if _, err := st.SetExternalRefPin(src, r.RefID, 7); err == nil || !contains(err.Error(), "无版本快照") {
		t.Fatalf("刷新到不存在版本应拒绝: %v", err)
	}
	// 不存在登记
	if _, err := st.SetExternalRefPin(src, "extref-ghost", 1); err != ErrNotFound {
		t.Fatalf("幽灵 ref 应 ErrNotFound: %v", err)
	}
}

// 引用解析点按 pin 解析的核心语义：被引方头版本改名后，无 pin 引用报悬空、
// pin v1 引用不报（钉住的快照里概念存在）、pin v2 引用报（钉住的快照里概念本就不存在）。
func TestAnnotateDanglingPinAware_Req540(t *testing.T) {
	st, src, dst := pinFixture(t)
	// 无 pin：头版本 v2 已改名 → missing_concept（现状语义不变）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// pin v1：快照里有「需求」→ 不悬空（pin 防护）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	// pin v2：快照里本就没有「需求」→ missing_concept（pin 快照内如实报告）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "reuse", PinVersion: 2}); err != nil {
		t.Fatal(err)
	}
	refs, err := st.ListExternalRefs(src)
	if err != nil || len(refs) != 3 {
		t.Fatalf("应有 3 条登记: %v %d", err, len(refs))
	}
	st.AnnotateDangling(refs)
	codes := map[int]string{}
	for _, r := range refs {
		codes[r.PinVersion] = r.Dangling
	}
	if codes[0] != "missing_concept" {
		t.Fatalf("无 pin 期望 missing_concept，实际 %q", codes[0])
	}
	if codes[1] != "" {
		t.Fatalf("pin v1 期望不悬空（快照含概念），实际 %q", codes[1])
	}
	if codes[2] != "missing_concept" {
		t.Fatalf("pin v2 期望 missing_concept，实际 %q", codes[2])
	}
	// DetectDanglingRefs 消息带 pin 上下文
	msgs, err := st.DetectDanglingRefs(src)
	if err != nil {
		t.Fatal(err)
	}
	pinned := false
	for _, m := range msgs {
		if strings.Contains(m, "pin v2") {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("悬空消息应带 pin 上下文: %v", msgs)
	}
}

func TestListPinEdges_Req540(t *testing.T) {
	st, src, dst := pinFixture(t)
	// 同 (src,dst) 两条 pin=2/1 → 收敛最低 1
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "reuse", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	// 外部 IRI 登记态与无 pin 登记不入边
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToConcept: "http://ex.org/v", RefKind: "import", Justification: "import_statement"}); err != nil {
		t.Fatal(err)
	}
	// src2→dst pin=2（快照存在）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "src2", FromConcept: OntologyLevelRef, ToOntoID: dst, ToConcept: "需求", RefKind: "import", PinVersion: 2}); err != nil {
		t.Fatal(err)
	}
	edges, err := st.ListPinEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("应 2 条 pin 边（同对收敛+外部 IRI 排除）: %+v", edges)
	}
	byPair := map[string]int{}
	for _, e := range edges {
		byPair[e.FromOntoID+"→"+e.ToOntoID] = e.PinVersion
	}
	if byPair["src→"+dst] != 1 {
		t.Fatalf("src→dst 应收敛最低 pin 1: %+v", byPair)
	}
	if byPair["src2→"+dst] != 2 {
		t.Fatalf("src2→dst 应为 pin 2: %+v", byPair)
	}
}
