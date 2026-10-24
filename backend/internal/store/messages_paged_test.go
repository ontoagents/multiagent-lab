package store

// REQ-413/M209：消息分页单测。
import (
	"path/filepath"
	"testing"
)

func TestListMessagesPaged(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, _ := st.CreateAgent(&Agent{Name: "pg"})
	conv, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		if _, err := st.InsertMessage(&Message{ConversationID: conv.ID, Role: "user", Content: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	// 第 1 页（最近 3 条，正序还原）：e f g
	msgs, total, err := st.ListMessagesPaged(conv.ID, 1, 3)
	if err != nil || total != 7 || len(msgs) != 3 {
		t.Fatalf("p1: %v %d %d", err, total, len(msgs))
	}
	if msgs[0].Content != "e" || msgs[2].Content != "g" {
		t.Fatalf("页内正序不符: %c%c%c", msgs[0].Content[0], msgs[1].Content[0], msgs[2].Content[0])
	}
	// 第 3 页：a + 不足一页
	msgs2, _, _ := st.ListMessagesPaged(conv.ID, 3, 3)
	if len(msgs2) != 1 || msgs2[0].Content != "a" {
		t.Fatalf("p3: %+v", msgs2)
	}
	// 越界页：空
	msgs3, _, _ := st.ListMessagesPaged(conv.ID, 9, 3)
	if len(msgs3) != 0 {
		t.Fatalf("越界页应空: %d", len(msgs3))
	}
}
