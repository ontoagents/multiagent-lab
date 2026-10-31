package rest

// REQ-511/M319 导入向导「引用」复用语义登记核单测——幂等跳过/本体级哨兵形态/空目标。
// 登记逻辑抽为 registerImportRefs（handler 薄壳），repo 真库夹具沿 external_refs_test 先例。

import (
	"path/filepath"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func req511Fixture(t *testing.T) *Server {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "t.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("src", "源本体", ""); err != nil {
		t.Fatal(err)
	}
	return &Server{Store: st}
}

func TestRegisterImportRefs_Req511(t *testing.T) {
	s := req511Fixture(t)
	iris := []string{"http://example.org/onto/core", "http://example.org/onto/ext"}
	reg, skip, err := s.registerImportRefs("src", iris)
	if err != nil || len(reg) != 2 || len(skip) != 0 {
		t.Fatalf("首次登记应 2 条: reg=%v skip=%v err=%v", reg, skip, err)
	}
	refs, _ := s.Store.ListExternalRefs("src")
	if len(refs) != 2 {
		t.Fatalf("应落 2 行: %d", len(refs))
	}
	for _, r := range refs {
		if r.RefKind != "import" || r.FromConcept != repo.OntologyLevelRef || r.ToOntoID != "" {
			t.Fatalf("登记形态失真（本体级哨兵+外部 IRI 态）: %+v", r)
		}
	}
	// 幂等：重复登记同 IRI → 全部跳过
	reg2, skip2, err := s.registerImportRefs("src", iris)
	if err != nil || len(reg2) != 0 || len(skip2) != 2 {
		t.Fatalf("重复登记应全跳过: reg=%v skip=%v err=%v", reg2, skip2, err)
	}
	refs2, _ := s.Store.ListExternalRefs("src")
	if len(refs2) != 2 {
		t.Fatalf("幂等后仍应 2 行: %d", len(refs2))
	}
	// 混合：1 旧 1 新
	reg3, skip3, err := s.registerImportRefs("src", []string{iris[0], "http://example.org/onto/new"})
	if err != nil || len(reg3) != 1 || len(skip3) != 1 {
		t.Fatalf("混合登记失真: reg=%v skip=%v err=%v", reg3, skip3, err)
	}
}

func TestRegisterImportRefs_Empty(t *testing.T) {
	s := req511Fixture(t)
	reg, skip, err := s.registerImportRefs("src", nil)
	if err != nil || len(reg) != 0 || len(skip) != 0 {
		t.Fatalf("空目标应零登记: %v %v %v", reg, skip, err)
	}
}
