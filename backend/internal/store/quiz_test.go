package store

// REQ-524/M331：掌握度落表单测——首评初始化/加权聚合/correct 计数/列表薄弱优先。

import "testing"

func TestUpsertMastery(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	if err := st.UpsertMastery("o1", "概念A", true, 0.9); err != nil {
		t.Fatal(err)
	}
	ms, err := st.ListMastery("o1")
	if err != nil || len(ms) != 1 {
		t.Fatalf("首评应落一行: %v %d", err, len(ms))
	}
	if ms[0].AskedCount != 1 || ms[0].CorrectCount != 1 || ms[0].Score != 0.9 {
		t.Fatalf("首评值不符: %+v", ms[0])
	}
	// 二次 0.5：score=0.7*0.9+0.3*0.5=0.78；correct 不增
	if err := st.UpsertMastery("o1", "概念A", false, 0.5); err != nil {
		t.Fatal(err)
	}
	ms, _ = st.ListMastery("o1")
	if ms[0].AskedCount != 2 || ms[0].CorrectCount != 1 {
		t.Fatalf("累计不符: %+v", ms[0])
	}
	if ms[0].Score < 0.77 || ms[0].Score > 0.79 {
		t.Fatalf("加权分应 0.78: %v", ms[0].Score)
	}
	// 薄弱优先排序（score 升序）
	_ = st.UpsertMastery("o1", "概念B", false, 0.2)
	ms, _ = st.ListMastery("o1")
	if len(ms) != 2 || ms[0].ConceptName != "概念B" {
		t.Fatalf("列表应 score 升序薄弱优先: %+v", ms)
	}
}

func TestQuizItemRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	q, err := st.InsertQuizItem(&QuizItem{OntologyID: "o1", ConceptNames: []string{"概念A", "概念B"}, Question: "什么是A？", Answer: "依据…", Difficulty: "basic"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetQuizItem(q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ConceptNames) != 2 || got.ConceptNames[0] != "概念A" || got.Question != "什么是A？" {
		t.Fatalf("往返不符: %+v", got)
	}
	items, err := st.ListQuizItems("o1", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("历史列表: %v %d", err, len(items))
	}
	if _, err := st.GetQuizItem("nope"); err == nil {
		t.Fatal("不存在应 404")
	}
}
