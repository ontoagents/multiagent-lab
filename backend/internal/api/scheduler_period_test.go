package api

// REQ-412/M208：定时周期化——interval 上限放宽（7 天）校验单测（真实 handler+完整夹具）。
import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func periodFixture(t *testing.T) (*Server, string) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "period-agent"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, sched: newScheduler(&Server{Store: st})}
	return s, conv.ID
}

func TestSchedulePeriodicInterval(t *testing.T) {
	s, convID := periodFixture(t)
	mk := func(interval, maxRuns int) *httptest.ResponseRecorder {
		body := `{"interval_minutes":` + itoa(interval) + `,"max_runs":` + itoa(maxRuns) + `}`
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
		req.SetPathValue("id", convID)
		rec := httptest.NewRecorder()
		s.createSchedule(rec, req)
		return rec
	}
	if rec := mk(10080, 200); strings.Contains(rec.Body.String(), "interval_minutes ∈") {
		t.Fatalf("7 天周期应通过: %s", rec.Body.String())
	}
	if rec := mk(20000, 10); !strings.Contains(rec.Body.String(), "interval_minutes ∈") {
		t.Fatalf("超 7 天应 400: %s", rec.Body.String())
	}
	if rec := mk(60, 200); strings.Contains(rec.Body.String(), "interval_minutes ∈") {
		t.Fatalf("max_runs=200 应通过: %s", rec.Body.String())
	}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
