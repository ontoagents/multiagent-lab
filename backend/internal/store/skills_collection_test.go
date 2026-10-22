package store

// REQ-373/M171：技能集合 store 层单测——children 校验（存在性/非嵌套/去重）、
// 删除成员同步摘除集合引用、集合免 instruction 必填。

import (
	"errors"
	"testing"
)

func TestValidateCollectionChildren(t *testing.T) {
	st, err := Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m, err := st.CreateSkill(&Skill{Name: "m", Instruction: "x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	coll, err := st.CreateSkill(&Skill{Name: "coll", Kind: "collection", Children: []string{}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// 去重保序
	got, err := st.ValidateCollectionChildren(coll.ID, []string{m.ID, m.ID})
	if err != nil || len(got) != 1 || got[0] != m.ID {
		t.Fatalf("应去重保序: %v %v", got, err)
	}
	// 未知子件 400
	if _, err := st.ValidateCollectionChildren(coll.ID, []string{"ghost"}); err == nil {
		t.Fatal("未知子件应 400")
	} else if he, ok := err.(*HTTPError); !ok || he.Status != 400 {
		t.Fatalf("应为人话 400: %v", err)
	}
	// 嵌套集合 400（自身引用被静默去重；他集合入成员才是嵌套路径）
	coll2, err := st.CreateSkill(&Skill{Name: "coll2", Kind: "collection", Children: []string{}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ValidateCollectionChildren(coll2.ID, []string{coll.ID}); err == nil {
		t.Fatal("嵌套集合应 400")
	}
}

func TestCollectionCRUDAndDeleteCleanup(t *testing.T) {
	st, err := Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	// 集合免 instruction 必填；普通技能保留必填
	if _, err := st.CreateSkill(&Skill{Name: "plain", Enabled: true}); err == nil {
		t.Fatal("普通技能空 instruction 应 400")
	}
	m1, err := st.CreateSkill(&Skill{Name: "m1", Instruction: "x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := st.CreateSkill(&Skill{Name: "m2", Instruction: "y", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	coll, err := st.CreateSkill(&Skill{Name: "pack", Kind: "collection", Children: []string{m1.ID, m2.ID}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if coll.Instruction != "" {
		t.Fatal("集合 instruction 应归空")
	}
	got, err := st.GetSkill(coll.ID)
	if err != nil || got.Kind != "collection" || len(got.Children) != 2 {
		t.Fatalf("集合往返应保 kind/children: %+v %v", got, err)
	}

	// kind 创建后不可变（UpdateSkill 不改 kind 列）
	got.Kind = ""
	if _, err := st.UpdateSkill(got); err != nil {
		t.Fatal(err)
	}
	reread, _ := st.GetSkill(coll.ID)
	if reread.Kind != "collection" {
		t.Fatal("kind 应创建后不可变")
	}

	// 删除成员 → 集合 children 同步摘除
	if err := st.DeleteSkill(m1.ID); err != nil {
		t.Fatal(err)
	}
	reread, _ = st.GetSkill(coll.ID)
	if len(reread.Children) != 1 || reread.Children[0] != m2.ID {
		t.Fatalf("删除成员应摘除集合引用: %v", reread.Children)
	}

	// 删除集合 → 成员不受影响
	if err := st.DeleteSkill(coll.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSkill(m2.ID); err != nil {
		t.Fatalf("删除集合不应动成员: %v", err)
	}
	if _, err := st.GetSkill(coll.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("集合应已删除: %v", err)
	}
}
