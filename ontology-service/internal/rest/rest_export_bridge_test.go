package rest

// REQ-479/M279 REST 级导出桥测试：external_refs 登记经 GET /export spec_export 分支出
// 桥三元组（alignment skos 物化+import seeAlso 同源 IRI+owl:imports 头），X-Onto-Triples
// 精确计数含桥三元组（装载对账闸数据源一致性）。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

func TestExportBridgeEndpoint_Req479(t *testing.T) {
	st, err := repo.Open(filepath.Join(t.TempDir(), "bridge_test.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("src", "源", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOntology("dst", "目标", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("src", "spec_json", `{"name":"源","concepts":[{"name":"缺陷"},{"name":"模块"}]}`, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "需求", RefKind: "alignment", Predicate: "exactMatch"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "模块", ToOntoID: "dst", ToConcept: "需求", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/ontologies/src/export?format=turtle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("导出期望 200，实际 %d", resp.StatusCode)
	}
	ttl, _ := io.ReadAll(resp.Body)
	for _, want := range []string{
		"owl:imports <urn:o:dst:>",
		"skos:exactMatch <urn:o:dst:concept:需求>",
		"rdfs:seeAlso <urn:o:dst:concept:需求>",
	} {
		if !strings.Contains(string(ttl), want) {
			t.Fatalf("导出缺少桥片段 %q:\n%s", want, ttl)
		}
	}
	// 精确计数=头 2+imports 1+缺陷(type+label+skos) 3+模块(type+label+seeAlso) 3=9
	if got := resp.Header.Get("X-Onto-Triples"); got != "9" {
		t.Fatalf("X-Onto-Triples 期望 9（含桥三元组），实际 %q ttl=%s", got, ttl)
	}
}
