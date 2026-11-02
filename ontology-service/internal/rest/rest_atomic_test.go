package rest

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestAtomicHTTPConcurrentSave(t *testing.T) {
	srv, st := newPublishTestServer(t)
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, label := range []string{"one", "two"} {
		go func(label string) {
			<-start
			req, _ := http.NewRequest("PUT", srv.URL+"/api/ontologies/ontest/spec", strings.NewReader(`{"name":"test","concepts":[{"name":"概念A","label":"`+label+`"}]}`))
			req.Header.Set("If-Match", "v2")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- 0
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			results <- resp.StatusCode
		}(label)
	}
	close(start)
	a, b := <-results, <-results
	if !((a == 200 && b == 409) || (a == 409 && b == 200)) {
		t.Fatalf("statuses %d/%d", a, b)
	}
	head, err := st.ReadSpecSnapshot("ontest")
	if err != nil {
		t.Fatal(err)
	}
	version, err := st.GetVersionSpec("ontest", 3)
	if err != nil {
		t.Fatal(err)
	}
	if head.Version != 3 || head.SpecJSON != version {
		t.Fatalf("head/history mismatch: %+v %s", head, version)
	}
}

func TestAtomicHTTPOverlayFailurePreservesHead(t *testing.T) {
	srv, st := newPublishTestServer(t)
	lid := pkgspec.ObjectLogicalID("concept", "", "概念A")
	_, err := st.UpsertOverlay(&repo.ObjectOverlay{OntologyID: "ontest", LogicalID: lid, Kind: "concept", Name: "概念A", Action: "edit", BaseJSON: `{"name":"概念A"}`, OverlayJSON: `{"name":"概念A","definition":"old"}`}, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := st.ReadSpecSnapshot("ontest")
	resp, _ := doJSON(t, "PUT", srv.URL+"/api/ontologies/ontest/objects/concept/概念A/overlay", `{"values":{"name":"概念A","parents":["missing"]}}`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("invalid edit status %d", resp.StatusCode)
	}
	row, err := st.GetOverlay("ontest", lid)
	if err != nil || row.Revision != 1 || !strings.Contains(row.OverlayJSON, "old") {
		t.Fatalf("invalid edit damaged existing overlay: %+v %v", row, err)
	}
	if _, err := st.DB().Exec(`CREATE TRIGGER fail_overlay BEFORE UPDATE ON ontology_object_overlay BEGIN SELECT RAISE(ABORT,'overlay fault'); END`); err != nil {
		t.Fatal(err)
	}
	resp, _ = doJSON(t, "PUT", srv.URL+"/api/ontologies/ontest/spec", `{"name":"test","concepts":[{"name":"概念A","definition":"new"}]}`, nil)
	if resp.StatusCode != 500 {
		t.Fatalf("fault status %d", resp.StatusCode)
	}
	after, _ := st.ReadSpecSnapshot("ontest")
	if after.Version != before.Version || after.SpecJSON != before.SpecJSON {
		t.Fatalf("partial save: before=%+v after=%+v", before, after)
	}
	if _, err := st.GetVersionSpec("ontest", 3); err != repo.ErrNotFound {
		t.Fatalf("failed save left snapshot: %v", err)
	}
}
