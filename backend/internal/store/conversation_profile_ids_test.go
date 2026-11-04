package store

// REQ-619/M427 会话本体运行方案多选（runtime_profile_ids JSON 数组列）存储层单测：
// 创建往返/更新覆盖/nil 不覆盖（COALESCE）/空数组显式清空/单值回落字段保留。零外部依赖。

import (
	"path/filepath"
	"reflect"
	"testing"
)

func newConvTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestConversationProfileIDsRoundtrip(t *testing.T) {
	st := newConvTestStore(t)
	ag, err := st.CreateAgent(&Agent{Name: "t-conv-profile-ids"})
	if err != nil {
		t.Fatal(err)
	}
	// ① 创建带多选集合 → 往返回读一致
	c, err := st.CreateConversation(&Conversation{Scope: "agent", AgentID: &ag.ID, Title: "多选会话", RuntimeProfileIDs: []string{"p1", "p2"}, OntologyEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetConversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.RuntimeProfileIDs, []string{"p1", "p2"}) {
		t.Fatalf("创建多选往返失真: %v", got.RuntimeProfileIDs)
	}
	// ② 更新覆盖为新集合
	up := *got
	up.RuntimeProfileIDs = []string{"p3"}
	if _, err := st.UpdateConversation(&up); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.GetConversation(c.ID)
	if !reflect.DeepEqual(got2.RuntimeProfileIDs, []string{"p3"}) {
		t.Fatalf("更新多选未覆盖: %v", got2.RuntimeProfileIDs)
	}
	// ③ 更新不带 ids（nil）→ 保留原值（COALESCE 语义）
	up2 := *got2
	up2.RuntimeProfileIDs = nil
	up2.Title = "只改标题"
	if _, err := st.UpdateConversation(&up2); err != nil {
		t.Fatal(err)
	}
	got3, _ := st.GetConversation(c.ID)
	if !reflect.DeepEqual(got3.RuntimeProfileIDs, []string{"p3"}) {
		t.Fatalf("nil 应保留原值: %v", got3.RuntimeProfileIDs)
	}
	if got3.Title != "只改标题" {
		t.Fatalf("标题更新失真: %s", got3.Title)
	}
	// ④ 空数组=显式清空（关闭本体语义，落 '[]' 而非 NULL）
	up3 := *got3
	up3.RuntimeProfileIDs = []string{}
	if _, err := st.UpdateConversation(&up3); err != nil {
		t.Fatal(err)
	}
	got4, _ := st.GetConversation(c.ID)
	if got4.RuntimeProfileIDs == nil || len(got4.RuntimeProfileIDs) != 0 {
		t.Fatalf("空数组应显式清空: %v", got4.RuntimeProfileIDs)
	}
	// ⑤ 单值 legacy 列保留（继承链/对比窗格兼容面不被多选破坏）
	if got4.RuntimeProfileID != nil && *got4.RuntimeProfileID != "" {
		t.Fatalf("未写过单值列应为空: %v", *got4.RuntimeProfileID)
	}
}
