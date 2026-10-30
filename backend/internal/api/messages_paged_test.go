package api

// REQ-413/M209：分页端点单测。
import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestListMessagesPagedEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	ag, _ := st.CreateAgent(&store.Agent{Name: "pg"})
	conv, _ := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	for i := 0; i < 5; i++ {
		_, _ = st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: string(rune('a' + i))})
	}
	req := httptest.NewRequest("GET", "/x?page=1&page_size=2", nil)
	req.SetPathValue("id", conv.ID)
	rec := httptest.NewRecorder()
	s.listMessagesPaged(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("X-Total-Count") != "5" {
		t.Fatalf("total header: %s", rec.Header().Get("X-Total-Count"))
	}
	if !strings.Contains(rec.Body.String(), `"d"`) {
		t.Fatalf("首页应含最近消息 d: %s", rec.Body.String())
	}
}
