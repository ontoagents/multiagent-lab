package store

import (
	"path/filepath"
	"testing"
)

// REQ-311 B2：Harness 审计跨会话聚合——审计族过滤（非审计事件不入列）、跨会话 JOIN 标题、倒序与 limit。
func TestListHarnessAudit(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	a1, err := st.CreateAgent(&Agent{Name: "审计智能体"})
	if err != nil {
		t.Fatalf("create agent1: %v", err)
	}
	a2, err := st.CreateAgent(&Agent{Name: "别的智能体"})
	if err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	c1, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &a1.ID, Title: "会话甲"})
	if err != nil {
		t.Fatalf("create conv1: %v", err)
	}
	c2, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &a1.ID, Title: "会话乙"})
	if err != nil {
		t.Fatalf("create conv2: %v", err)
	}
	c3, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &a2.ID, Title: "别家会话"})
	if err != nil {
		t.Fatalf("create conv3: %v", err)
	}

	mk := func(convID, typ, data string) {
		if _, err := st.InsertEvent(&RunEvent{ConversationID: convID, RunID: "r1", Type: typ, Data: data}); err != nil {
			t.Fatalf("insert %s: %v", typ, err)
		}
	}
	mk(c1.ID, "approval.request", `{"tool_name":"write_file"}`)
	mk(c1.ID, "approval.granted", `{"tool_name":"write_file","decision":"approve"}`)
	mk(c2.ID, "hook.denied", `{"guard":"custom:no-web","tool_name":"http_fetch"}`)
	mk(c2.ID, "verify.failed", `{"command":"go build ./...","error":"exit 1"}`)
	mk(c2.ID, "tool.call", `{"name":"write_file"}`)            // 非审计族，不入列
	mk(c2.ID, "run.finished", `{"reason":"completed"}`)        // 非审计族
	mk(c3.ID, "approval.denied", `{"tool_name":"write_file"}`) // 别家智能体，不入列

	rows, total, err := st.ListHarnessAudit(a1.ID, 50)
	if err != nil {
		t.Fatalf("ListHarnessAudit: %v", err)
	}
	if total != 4 || len(rows) != 4 {
		t.Fatalf("期望 4 条审计事件，得 total=%d len=%d", total, len(rows))
	}
	for _, r := range rows {
		if r.Type != "approval.request" && r.Type != "approval.granted" && r.Type != "hook.denied" && r.Type != "verify.failed" {
			t.Fatalf("混入非审计事件: %s", r.Type)
		}
		if r.ConversationTitle != "会话甲" && r.ConversationTitle != "会话乙" {
			t.Fatalf("会话标题 JOIN 失效: %q", r.ConversationTitle)
		}
	}
	// 倒序：首条为最后插入的审计事件（verify.failed，会话乙）
	if rows[0].Type != "verify.failed" || rows[0].ConversationTitle != "会话乙" {
		t.Fatalf("倒序失效: 首条 %s/%s", rows[0].Type, rows[0].ConversationTitle)
	}

	// limit 生效（total 不变，窗口收窄）
	rows2, total2, err := st.ListHarnessAudit(a1.ID, 2)
	if err != nil {
		t.Fatalf("limit: %v", err)
	}
	if total2 != 4 || len(rows2) != 2 {
		t.Fatalf("limit 分页失效: total=%d len=%d", total2, len(rows2))
	}
}
