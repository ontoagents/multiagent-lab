package api

// REQ-400/M196：If-Match 乐观锁单测（三实体）。
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func ifMatchFixture(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Server{Store: st}
}

// TestIfMatchAgentUpdate agent PUT：头缺省放行/一致放行/不一致 409。
func TestIfMatchAgentUpdate(t *testing.T) {
	s := ifMatchFixture(t)
	ag, _ := s.Store.GetAgent("builtin-assistant")
	if ag == nil {
		t.Fatal("内置行应存在")
	}
	mk := func(hdr string) *httptest.ResponseRecorder {
		body := `{"instruction":"更新后的指令"}`
		req := httptest.NewRequest(http.MethodPut, "/api/agents/builtin-assistant", strings.NewReader(body))
		req.SetPathValue("id", "builtin-assistant")
		if hdr != "" {
			req.Header.Set("If-Match", hdr)
		}
		rec := httptest.NewRecorder()
		s.updateAgent(rec, req)
		return rec
	}
	if rec := mk(""); rec.Code == http.StatusConflict {
		t.Fatal("头缺省应放行")
	}
	fresh, _ := s.Store.GetAgent("builtin-assistant") // 头缺省探针已 bump updated_at——重取基准
	if rec := mk(fresh.UpdatedAt); rec.Code == http.StatusConflict {
		t.Fatalf("一致应放行: %s", rec.Body.String())
	}
	rec := mk("2020-01-01T00:00:00Z")
	if rec.Code != http.StatusConflict {
		t.Fatalf("不一致应 409，得 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "If-Match") {
		t.Fatalf("409 应含乐观锁语义: %s", rec.Body.String())
	}
}

// TestIfMatchConversationUpdate conversation PUT 同口径。
func TestIfMatchConversationUpdate(t *testing.T) {
	s := ifMatchFixture(t)
	ag, _ := s.Store.GetAgent("builtin-assistant")
	conv, err := s.Store.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(hdr string) *httptest.ResponseRecorder {
		body := `{"title":"改名"}`
		req := httptest.NewRequest(http.MethodPut, "/api/conversations/"+conv.ID, strings.NewReader(body))
		req.SetPathValue("id", conv.ID)
		if hdr != "" {
			req.Header.Set("If-Match", hdr)
		}
		rec := httptest.NewRecorder()
		s.updateConversation(rec, req)
		return rec
	}
	if rec := mk(""); rec.Code == http.StatusConflict {
		t.Fatal("头缺省应放行")
	}
	fresh, _ := s.Store.GetConversation(conv.ID) // 头缺省探针已 bump updated_at——重取基准
	if rec := mk(fresh.UpdatedAt); rec.Code == http.StatusConflict {
		t.Fatalf("一致应放行: %s", rec.Body.String())
	}
	if rec := mk("2020-01-01T00:00:00Z"); rec.Code != http.StatusConflict {
		t.Fatalf("不一致应 409，得 %d", rec.Code)
	}
}

// TestIfMatchProjectUpdate project PUT 同口径。
func TestIfMatchProjectUpdate(t *testing.T) {
	s := ifMatchFixture(t)
	p, err := s.Store.CreateProject(&store.Project{Name: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(hdr string) *httptest.ResponseRecorder {
		body := `{"name":"p1改名"}`
		req := httptest.NewRequest(http.MethodPut, "/api/projects/"+p.ID, strings.NewReader(body))
		req.SetPathValue("id", p.ID)
		if hdr != "" {
			req.Header.Set("If-Match", hdr)
		}
		rec := httptest.NewRecorder()
		s.updateProject(rec, req)
		return rec
	}
	if rec := mk(""); rec.Code == http.StatusConflict {
		t.Fatal("头缺省应放行")
	}
	if rec := mk(p.UpdatedAt); rec.Code == http.StatusConflict {
		t.Fatalf("一致应放行: %s", rec.Body.String())
	}
	if rec := mk("2020-01-01T00:00:00Z"); rec.Code != http.StatusConflict {
		t.Fatalf("不一致应 409，得 %d", rec.Code)
	}
}
