package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// A restarted worker cannot finish its old job. Polling must use the reconciled
// database state while retaining any completed result, not replay a stale status
// embedded in result_json.
func TestDraftJobPollingAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	st, err := repo.Open(path, "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id, status, payload, wantStatus string
		wantResult                      bool
	}{
		{"restart-running", "running", `{"id":"restart-running","status":"running","progress":"old progress"}`, "failed", false},
		{"restart-queued", "queued", `{"id":"restart-queued","status":"queued"}`, "failed", false},
		{"restart-done", "done", `{"id":"restart-done","status":"done","result":{"spec":{"name":"retained"},"rounds":2}}`, "done", true},
		{"restart-failed", "failed", `{"id":"restart-failed","status":"failed","error":"stale error"}`, "failed", false},
		{"restart-metadata-only", "running", "", "failed", false},
	}
	for _, tc := range cases {
		if err := st.UpsertAsyncJob(&repo.AsyncJobRow{ID: tc.id, Kind: "draft", Status: tc.status,
			ResultJSON: tc.payload, Progress: "persisted progress", Error: "persisted error", CreatedAt: "2026-11-03T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = repo.Open(path, "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if n, err := st.MarkOrphanAsyncJobs(); err != nil || n != 3 {
		t.Fatalf("restart reconciliation: changed=%d err=%v", n, err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ai-draft-jobs/"+tc.id, nil))
			var got draftJob
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
				t.Fatalf("poll: status=%d body=%s", w.Code, w.Body.String())
			}
			if got.ID != tc.id || got.Status != tc.wantStatus || got.Progress != "persisted progress" {
				t.Errorf("poll did not use durable metadata: %+v", got)
			}
			if got.CreatedAt.Format(time.RFC3339) != "2026-11-03T00:00:00Z" {
				t.Errorf("created_at lost: %v", got.CreatedAt)
			}
			if tc.status == "running" || tc.status == "queued" {
				if !strings.Contains(got.Error, "重启中断") {
					t.Errorf("missing restart error: %q", got.Error)
				}
			} else if got.Error != "persisted error" {
				t.Errorf("error metadata lost: %q", got.Error)
			}
			if tc.wantResult && (got.Result == nil || got.Result.Spec["name"] != "retained" || got.Result.Rounds != 2) {
				t.Errorf("completed result lost: %+v", got.Result)
			}
		})
	}
}

func TestDraftJobPollingConcurrentProgress(t *testing.T) {
	job := &draftJob{ID: "poll-concurrent-progress", Status: "running"}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	t.Cleanup(func() {
		draftJobsMu.Lock()
		delete(draftJobs, job.ID)
		draftJobsMu.Unlock()
	})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := 0; ; n++ {
			select {
			case <-stop:
				return
			default:
			}
			draftJobsMu.Lock()
			job.Progress = fmt.Sprintf("question %d", n)
			draftJobsMu.Unlock()
		}
	}()
	defer func() { close(stop); <-done }()
	s := &Server{}
	for n := 0; n < 100; n++ {
		r := httptest.NewRequest("GET", "/api/ai-draft-jobs/"+job.ID, nil)
		r.SetPathValue("id", job.ID)
		w := httptest.NewRecorder()
		s.aiDraftJob(w, r)
		var out draftJob
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.ID != job.ID || out.Status != "running" || (out.Progress != "" && !strings.HasPrefix(out.Progress, "question ")) {
			t.Fatalf("invalid polling snapshot: %s, %v", w.Body.String(), err)
		}
	}
}
