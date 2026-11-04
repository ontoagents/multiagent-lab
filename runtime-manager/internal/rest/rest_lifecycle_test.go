package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/manager"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

type failingStopEngine struct{ fail bool }

func (e *failingStopEngine) Start(context.Context, string, int, map[string]string) (*engine.Process, error) {
	return engine.NewProcess("http://127.0.0.1:1/sparql", 42, func() error {
		if e.fail {
			return errors.New("engine stop refused")
		}
		return nil
	}), nil
}

func (*failingStopEngine) HealthCheck(context.Context, string) error { return nil }

func TestRESTMutationPreservesFailedStop(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			dir := t.TempDir()
			st, err := store.Open(filepath.Join(dir, "rt.db"), "../../migrations")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			build := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/external-refs") {
					_, _ = w.Write([]byte(`{"refs":[],"dangling":[]}`))
				} else if strings.HasSuffix(r.URL.Path, "/export") {
					_, _ = w.Write([]byte("<urn:s> <urn:p> <urn:o> ."))
				} else {
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(build.Close)
			mg := manager.New(st, build.URL, dir, dir)
			e := &failingStopEngine{fail: true}
			mg.RegisterEngine("fuseki", e)
			mg.HealthTries = 1
			if err := st.SetConfig(store.RuntimeConfig{ExecutionMethod: "native"}); err != nil {
				t.Fatal(err)
			}
			if err := st.Create(&store.Profile{ID: "p1", Name: "original", Engine: "fuseki", OntologyIDs: []string{"o1"}, Config: "{}"}); err != nil {
				t.Fatal(err)
			}
			if err := mg.Start(context.Background(), "p1"); err != nil {
				t.Fatal(err)
			}
			if err := mg.Stop("p1"); err == nil {
				t.Fatal("fixture stop must fail")
			}
			mux := http.NewServeMux()
			New(st, mg, nil).Mount(mux)
			invoke := func(method, path string) *httptest.ResponseRecorder {
				rr := httptest.NewRecorder()
				mux.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(`{"name":"changed","ontology_ids":["o2"],"config":"{\"reasoning\":true}","port":9300}`)))
				return rr
			}
			res := invoke(method, "/api/runtime-profiles/p1")
			if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "engine stop refused") {
				t.Fatalf("failed stop must reach HTTP caller: %d %s", res.Code, res.Body.String())
			}
			p, err := st.Get("p1")
			if err != nil || p.Name != "original" || p.Status != "error" || len(p.OntologyIDs) != 1 || p.OntologyIDs[0] != "o1" {
				t.Fatalf("HTTP mutation lost/changed live profile: %+v %v", p, err)
			}
			if _, err := mg.ProcEndpoint("p1"); err != nil {
				t.Fatalf("lost retry handle: %v", err)
			}
			e.fail = false
			res = invoke(method, "/api/runtime-profiles/p1")
			if res.Code != http.StatusOK {
				t.Fatalf("retry mutation: %d %s", res.Code, res.Body.String())
			}
			if method == http.MethodPut {
				p, err = st.Get("p1")
				if err != nil || p.Name != "changed" || p.Status != "stopped" || p.Port != 9300 || p.Config != `{"reasoning":true}` {
					t.Fatalf("successful update: %+v %v", p, err)
				}
			} else if _, err := st.Get("p1"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("successful delete retained profile: %v", err)
			}
			if _, err := mg.ProcEndpoint("p1"); err == nil {
				t.Fatal("successful stop retained handle")
			}
		})
	}
}
