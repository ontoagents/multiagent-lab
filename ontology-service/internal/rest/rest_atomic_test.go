package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/importer"
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

func TestAtomicMergePreviewMatchesContentVersion(t *testing.T) {
	srv, st := newPublishTestServer(t)
	content := func(v int) string {
		return fmt.Sprintf(`{"name":"test","concepts":[{"name":"概念A","label":"%d"}]}`, v)
	}
	if _, err := st.SaveSpecSnapshot("ontest", repo.SpecWrite{ExpectedVersion: 2, SpecJSON: content(3)}); err != nil {
		t.Fatal(err)
	}
	incoming := pkgspec.Spec{Name: "incoming"}
	for n := 0; n < 128; n++ {
		incoming.Concepts = append(incoming.Concepts, pkgspec.Concept{Name: fmt.Sprintf("incoming%d", n)})
	}
	body, err := json.Marshal(map[string]any{"spec": incoming, "strategy": "merge"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	writer := make(chan error, 1)
	go func() {
		<-start
		for v := 3; v < 131; v++ {
			if _, err := st.SaveSpecSnapshot("ontest", repo.SpecWrite{ExpectedVersion: v, SpecJSON: content(v + 1)}); err != nil {
				writer <- err
				return
			}
		}
		writer <- nil
	}()
	results := make(chan error, 4)
	var readers sync.WaitGroup
	for n := 0; n < 4; n++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for n := 0; n < 20; n++ {
				resp, err := http.Post(srv.URL+"/api/ontologies/ontest/merge/preview", "application/json", strings.NewReader(string(body)))
				if err != nil {
					results <- err
					return
				}
				var pv importer.MergePreview
				err = json.NewDecoder(resp.Body).Decode(&pv)
				resp.Body.Close()
				if err != nil || resp.StatusCode != http.StatusOK {
					results <- fmt.Errorf("preview status=%d decode=%v", resp.StatusCode, err)
					return
				}
				if pv.MergedSpec == nil || len(pv.MergedSpec.Concepts) == 0 || pv.MergedSpec.Concepts[0].Label != strconv.Itoa(pv.BaseVersion) {
					label := "missing"
					if pv.MergedSpec != nil && len(pv.MergedSpec.Concepts) > 0 {
						label = pv.MergedSpec.Concepts[0].Label
					}
					results <- fmt.Errorf("preview mixes content version %s and base_version %d", label, pv.BaseVersion)
					return
				}
			}
			results <- nil
		}()
	}
	close(start)
	readers.Wait()
	if err := <-writer; err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 4; n++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	// A preview of an older head must be refused after another save.
	resp, out := doJSON(t, "POST", srv.URL+"/api/ontologies/ontest/merge/apply", `{"spec":{"name":"incoming","concepts":[{"name":"B"}]},"strategy":"merge","base_version":130}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale preview accepted: status=%d body=%v", resp.StatusCode, out)
	}
}

func TestMergePreviewMissingArtifactRemainsNotFound(t *testing.T) {
	srv, st := newPublishTestServer(t)
	if _, err := st.CreateOntology("empty-preview", "empty", ""); err != nil {
		t.Fatal(err)
	}
	resp, out := doJSON(t, "POST", srv.URL+"/api/ontologies/empty-preview/merge/preview", `{"spec":{"name":"incoming","concepts":[{"name":"B"}]},"strategy":"merge"}`, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing artifact status=%d want=404: %v", resp.StatusCode, out)
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
