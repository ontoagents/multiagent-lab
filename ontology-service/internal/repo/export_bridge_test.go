package repo

// REQ-479/M279：external_refs 登记 → 导出桥端到端（store 登记→bridge 投影→TTL 桥三元组）。
// 验证 REST 导出端点同款映射（repo.ExternalRef → importer.ExtRefBridge）字段不漂移。

import (
	"strings"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
	"github.com/ontoagents/ontoagents/ontology-service/internal/importer"
)

func TestExportBridgeFromRefs_Req479(t *testing.T) {
	st, src, dst := extRefFixture(t)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "缺陷", ToOntoID: dst, ToConcept: "需求", RefKind: "alignment", Predicate: "exactMatch"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: src, FromConcept: "模块", ToOntoID: dst, ToConcept: "需求", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	refs, err := st.ListExternalRefs(src)
	if err != nil || len(refs) != 2 {
		t.Fatalf("登记读取: %v %d", err, len(refs))
	}
	// REST 导出端点同款投影映射
	var bridges []importer.ExtRefBridge
	for _, ref := range refs {
		bridges = append(bridges, importer.ExtRefBridge{
			FromConcept: ref.FromConcept, ToOntoID: ref.ToOntoID, ToConcept: ref.ToConcept,
			RefKind: ref.RefKind, Predicate: ref.Predicate,
		})
	}
	sp := &pkgspec.Spec{Name: "源", Concepts: []pkgspec.Concept{{Name: "缺陷"}, {Name: "模块"}}}
	ttl, n, err := importer.TTLGoExport(src, sp, bridges...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<urn:o:src:__ontology> rdf:type owl:Ontology ; rdfs:label \"源\" ;\n    owl:imports <urn:o:dst:> .",
		"<urn:o:src:concept:缺陷> rdf:type owl:Class ; rdfs:label \"缺陷\" ;\n    skos:exactMatch <urn:o:dst:concept:需求> .",
		"<urn:o:src:concept:模块> rdf:type owl:Class ; rdfs:label \"模块\" ;\n    rdfs:seeAlso <urn:o:dst:concept:需求> .",
	} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("输出缺少期望片段:\n%s\n--- 实际 ---\n%s", want, ttl)
		}
	}
	if n != 9 { // 头 2 + imports 1 + 缺陷 3 + 模块 3
		t.Fatalf("计数不符: got %d want 9", n)
	}
}
