package store

// REQ-553/M360 方案分歧裁决审计存储单测：CRUD 往返/清单/resolve 终态守卫/项目规则档列读取。

import (
	"errors"
	"path/filepath"
	"testing"
)

func newConflictTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestConflictReviewCRUD_Req553(t *testing.T) {
	st := newConflictTestStore(t)
	cr := &ConflictReview{
		ConversationID: "conv-1", RunID: "run-1", Kind: "review", RuleID: "default_review_disagree",
		Parties:  []ConflictParty{{AgentID: "agt-a", AgentName: "评审甲", Position: "disagree"}},
		Verdicts: []map[string]any{{"position": "disagree", "plan_summary": "改用方案 B"}},
		State:    "suspended", CheckpointRef: "ckpt_run-1", Summary: "评审成员对当前方案提出分歧——本方方案：改用方案 B；反对要点：X",
	}
	if err := st.CreateConflictReview(cr); err != nil {
		t.Fatal(err)
	}
	if cr.ID == "" {
		t.Fatal("创建后应回填 id")
	}
	got, err := st.GetConflictReview(cr.ID)
	if err != nil || got == nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.State != "suspended" || got.Kind != "review" || len(got.Parties) != 1 || got.Parties[0].AgentName != "评审甲" {
		t.Fatalf("往返失真: %+v", got)
	}
	if len(got.Verdicts) != 1 || got.Verdicts[0]["plan_summary"] != "改用方案 B" {
		t.Fatalf("verdicts 往返失真: %+v", got.Verdicts)
	}
	// 清单（含 event 点行）
	ev := &ConflictReview{ConversationID: "conv-1", Kind: "event", RuleID: "verify_failed_review", State: "pending", Summary: "verify 未通过"}
	if err := st.CreateConflictReview(ev); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListConflictReviews("conv-1")
	if err != nil || len(list) != 2 {
		t.Fatalf("清单失真: %v %v", list, err)
	}
	if none, _ := st.ListConflictReviews("conv-other"); len(none) != 0 {
		t.Fatal("跨会话过滤失真")
	}
	// 裁决：resolution 落库 + 终态守卫
	if _, err := st.ResolveConflictReview(cr.ID, `{"adopt":"b","decision_source":"manual"}`); err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	got2, _ := st.GetConflictReview(cr.ID)
	if got2.State != "resolved" || got2.Resolution == nil || got2.Resolution.Adopt != "b" || got2.DecidedAt == "" {
		t.Fatalf("裁决往返失真: %+v", got2)
	}
	if _, err := st.ResolveConflictReview(cr.ID, `{"adopt":"a","decision_source":"manual"}`); !errors.Is(err, ErrConflictAlreadyResolved) {
		t.Fatalf("终态再裁应 ErrConflictAlreadyResolved, got %v", err)
	}
	// pending 行也可裁决；不存在 id → (nil, nil)
	if _, err := st.ResolveConflictReview(ev.ID, `{"adopt":"a","decision_source":"manual"}`); err != nil {
		t.Fatalf("pending 裁决失败: %v", err)
	}
	if np, err := st.ResolveConflictReview("nope", `{}`); np != nil || err != nil {
		t.Fatalf("不存在 id 应 (nil,nil): %+v %v", np, err)
	}
}

func TestProjectConflictPolicy_Req553(t *testing.T) {
	st := newConflictTestStore(t)
	if v := st.GetProjectConflictPolicy(""); v != "" {
		t.Fatalf("空项目 id 应空串: %q", v)
	}
	// 既有项目行（迁移 085 加列默认 ''）
	p, perr := st.CreateProject(&Project{Name: "p1"})
	if perr != nil || p == nil {
		t.Fatalf("项目创建失败: %v", perr)
	}
	if v := st.GetProjectConflictPolicy(p.ID); v != "" {
		t.Fatalf("默认规则档应为空串（默认档内置）: %q", v)
	}
	if _, err := st.DB.Exec(`UPDATE project SET conflict_policy=? WHERE id=?`, `{"rules":[]}`, p.ID); err != nil {
		t.Fatal(err)
	}
	if v := st.GetProjectConflictPolicy(p.ID); v != `{"rules":[]}` {
		t.Fatalf("规则档读取失真: %q", v)
	}
}
