package rest

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

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
