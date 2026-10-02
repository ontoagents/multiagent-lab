package api

// REQ-250/52 号 E6 余项：本体治理审计——五类治理动作（spec 保存/导入/合并/fork/CSV 灌装）
// 经 auditOntologyProxy 转发成功后落 onto_decision（subject_kind=ontology）；
// 失败/非治理动作不记；删除审计在 deleteOntologyGuard（伴生拦截在前，此处验透传成功分支）。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/companion"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/ontology"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newAuditFixture(t *testing.T, upstream http.HandlerFunc) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	return &Server{
		Store:     st,
		Ontology:  &ontology.Service{RuntimeURL: "http://127.0.0.1:1", BuildURL: up.URL},
		Companion: companion.NewService(st, nil, nil),
	}
}

func okUpstream(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}

func countOntologyDecisions(t *testing.T, s *Server) int {
	t.Helper()
	ds, err := s.Store.ListDecisions("ontology", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	return len(ds)
}

func TestOntologyAuditRecordsOnSuccess(t *testing.T) {
	s := newAuditFixture(t, okUpstream(`{"saved":true,"version":2}`))
	req := httptest.NewRequest(http.MethodPut, "/api/ontologies/onto_x/spec", strings.NewReader(`{}`))
	req.SetPathValue("id", "onto_x")
	w := httptest.NewRecorder()
	s.auditOntologyProxy(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("转发应 200，得到 %d", w.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countOntologyDecisions(t, s) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := countOntologyDecisions(t, s); n != 1 {
		t.Fatalf("治理动作成功应落 1 条 onto_decision，得到 %d", n)
	}
	ds, _ := s.Store.ListDecisions("ontology", "", 100)
	if !strings.Contains(ds[0].Title, "spec.save") || ds[0].SubjectKind != "ontology" || ds[0].SubjectID != "onto_x" {
		t.Fatalf("审计字段不符: %+v", ds[0])
	}
}

func TestOntologyAuditSkipsOnFailure(t *testing.T) {
	s := newAuditFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "validation failed", http.StatusBadRequest)
	})
	req := httptest.NewRequest(http.MethodPut, "/api/ontologies/onto_x/spec", strings.NewReader(`{}`))
	req.SetPathValue("id", "onto_x")
	w := httptest.NewRecorder()
	s.auditOntologyProxy(w, req)
	time.Sleep(200 * time.Millisecond)
	if n := countOntologyDecisions(t, s); n != 0 {
		t.Fatalf("失败动作不应落审计，得到 %d", n)
	}
}

func TestOntologyAuditIgnoresNonGovernance(t *testing.T) {
	s := newAuditFixture(t, okUpstream(`{}`))
	req := httptest.NewRequest(http.MethodGet, "/api/ontologies/onto_x", nil)
	req.SetPathValue("id", "onto_x")
	w := httptest.NewRecorder()
	s.auditOntologyProxy(w, req)
	time.Sleep(200 * time.Millisecond)
	if n := countOntologyDecisions(t, s); n != 0 {
		t.Fatalf("非治理动作（GET）不应落审计，得到 %d", n)
	}
}

func TestOntologyAuditAllFiveActions(t *testing.T) {
	s := newAuditFixture(t, okUpstream(`{"ok":true}`))
	actions := []struct{ method, path string }{
		{http.MethodPut, "/api/ontologies/onto_a/spec"},
		{http.MethodPost, "/api/ontologies/import"},
		{http.MethodPost, "/api/ontologies/onto_a/merge/apply"},
		{http.MethodPost, "/api/ontologies/onto_a/fork"},
		{http.MethodPost, "/api/ontologies/onto_a/ingest-csv"},
	}
	for _, a := range actions {
		req := httptest.NewRequest(a.method, a.path, strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		s.auditOntologyProxy(w, req)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countOntologyDecisions(t, s) >= len(actions) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := countOntologyDecisions(t, s); n != len(actions) {
		t.Fatalf("五类治理动作应各落 1 条，得到 %d", n)
	}
}
