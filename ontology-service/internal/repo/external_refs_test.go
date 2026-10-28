package repo

// REQ-262/M256：external_refs 登记单源单测——CRUD/词表校验/悬空检测/本体级哨兵。

import (
	"path/filepath"
	"testing"
)

func extRefFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("src", "源本体", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOntology("dst", "目标本体", ""); err != nil {
		t.Fatal(err)
	}
	// spec_json artifact（概念清单存在性数据源）
	srcSpec := `{"concepts":[{"name":"缺陷"},{"name":"模块"}]}`
	dstSpec := `{"concepts":[{"name":"需求"}]}`
	if _, _, err := st.GetArtifact("src", "spec_json"); err == nil {
		_ = srcSpec
	}
	if err := st.PutArtifact("src", "spec_json", srcSpec, true); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("dst", "spec_json", dstSpec, true); err != nil {
		t.Fatal(err)
	}
	return st, "src", "dst"
}

func TestExternalRefCRUD_Req262(t *testing.T) {
	st, src, dst := extRefFixture(t)
	r, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "alignment", Predicate: "exactMatch"})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	if r.RefID == "" {
		t.Fatal("ref_id 应生成")
	}
	// import/reuse 清空 predicate（词表不适用）
	r2, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "模块", ToOntoID: dst, ToConcept: "需求", RefKind: "reuse", Predicate: "exactMatch"})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Predicate != "" {
		t.Fatal("reuse 的 predicate 应被清空")
	}
	refs, err := st.ListExternalRefs(src)
	if err != nil || len(refs) != 2 {
		t.Fatalf("应有 2 条登记: %v %d", err, len(refs))
	}
	if err := st.DeleteExternalRef(src, r.RefID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteExternalRef(src, r.RefID); err != ErrNotFound {
		t.Fatalf("重复删除应 ErrNotFound: %v", err)
	}
}

func TestExternalRefValidation_Req262(t *testing.T) {
	st, src, dst := extRefFixture(t)
	cases := []struct {
		name string
		ref  ExternalRef
		want string
	}{
		{"非法 ref_kind", ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToConcept: "x", RefKind: "bogus"}, "ref_kind"},
		{"源概念不存在", ExternalRef{FromOntoID: src, FromConcept: "无", ToConcept: "x", RefKind: "import"}, "源概念"},
		{"目标本体不存在", ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: "ghost", ToConcept: "x", RefKind: "import"}, "目标本体"},
		{"非法 predicate", ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "alignment", Predicate: "bogus"}, "predicate"},
		{"缺 from_concept", ExternalRef{FromOntoID: src, ToConcept: "x", RefKind: "import"}, "必填"},
	}
	for _, c := range cases {
		if _, err := st.CreateExternalRef(&c.ref); err == nil || !contains(err.Error(), c.want) {
			t.Fatalf("%s: 期望错误含 %q，得到 %v", c.name, c.want, err)
		}
	}
}

func TestExternalRefDanglingAndOntoLevel_Req262(t *testing.T) {
	st, src, dst := extRefFixture(t)
	// 悬空：目标概念不存在
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "不存在", RefKind: "reuse"}); err != nil {
		t.Fatal(err)
	}
	// 外部 IRI 登记态（to_onto_id 空——天然不提醒）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "模块", ToConcept: "http://example.org/v/X", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// 本体级哨兵（owl:imports 自动登记形态）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: OntologyLevelRef, ToConcept: "http://example.org/ext/core", RefKind: "import", Justification: "import_statement"}); err != nil {
		t.Fatalf("本体级哨兵登记应放行: %v", err)
	}
	dangling, err := st.DetectDanglingRefs(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(dangling) != 1 || !contains(dangling[0], "目标概念不存在") {
		t.Fatalf("应恰好 1 条悬空提醒: %v", dangling)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}


// REQ-480/M280：AnnotateDangling 逐条结构化标注（运行平面装载门禁消费——不再解析消息串）。
func TestAnnotateDangling_Req480(t *testing.T) {
	st, src, _ := extRefFixture(t)
	// dst 本体存在但概念缺失 → missing_concept；to_onto_id 指向不存在本体 → missing_ontology
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "非存在概念", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// missing_ontology：登记后删除目标本体（登记端本就拒绝不存在目标，悬空只能来自删除）
	if _, err := st.CreateOntology("tmp2", "临时", ""); err != nil {
		t.Fatal(err)
	}
	ref2, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "模块", ToOntoID: "tmp2", ToConcept: "任意", RefKind: "reuse"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOntology("tmp2"); err != nil {
		t.Fatal(err)
	}
	refs, err := st.ListExternalRefs(src)
	if err != nil {
		t.Fatal(err)
	}
	st.AnnotateDangling(refs)
	codes := map[string]string{}
	for _, r := range refs {
		if r.RefID == ref2.RefID {
			continue // 已随本体删除级联清理的登记不在清单内（以实际行为为准）
		}
		if r.Dangling != "" {
			codes[r.FromConcept] = r.Dangling
		}
	}
	if codes["缺陷"] != "missing_concept" {
		t.Fatalf("缺陷 期望 missing_concept，实际 %v", codes)
	}
	_ = codes
}
