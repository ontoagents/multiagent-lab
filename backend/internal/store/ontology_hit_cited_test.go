package store

// REQ-530/M337：cited 归因单测——containment 命中 cited=1/未命中 cited=0/空白答案不归因。

import (
	"path/filepath"
	"testing"
)

func TestUpdateOntologyHitsCited(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, target := range []string{"HNSW 索引", "未引用概念"} {
		if err := st.InsertOntologyHit(&OntologyHit{
			ConversationID: "c1", RunID: "r1", OntologyID: "o1", Tool: "get_concept", Target: target, OK: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 最终回答含「HNSW索引」（空白差异归一化）不含「未引用概念」
	hits, cited := st.UpdateOntologyHitsCited("c1", "r1", "结论：Qdrant 使用 HNSW索引 加速检索。")
	if hits != 2 || cited != 1 {
		t.Fatalf("归因计数不符: hits=%d cited=%d", hits, cited)
	}
	// 空白答案：不归因（保持 0）
	hits, cited = st.UpdateOntologyHitsCited("c1", "r1", "  ")
	if hits != 2 || cited != 0 {
		t.Fatalf("空白答案不应归因: hits=%d cited=%d", hits, cited)
	}
}
