package kb

// KB-21/M293 单测：启动悬挂恢复——①重建中悬挂（有 chunks）自动重入队幂等重索；
// ②导入中悬挂（无 chunks）failed 明示「请重新导入」。wiki 库承载成功路径（chunksOnly
// 零 embedding 依赖）；队列满场景沿既有 enqueue 语义不在此重复。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestRecoverStalledIndexing(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	// wiki 库（chunksOnly 索引零 embedding 依赖——重入队后 worker 可独立完成）
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "rec-kb", Mode: "wiki"})
	if err != nil {
		t.Fatal(err)
	}
	// ①重建中悬挂：indexing + chunks 在库
	d1, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kb.ID, Title: "rebuild-stalled", Status: "indexing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertKnowledgeChunks([]*store.KnowledgeChunk{
		{ID: store.NewID(), KBID: kb.ID, DocID: d1.ID, Seq: 0, Content: "悬挂文档原始内容第一段。", StoreBackend: "none"},
	}); err != nil {
		t.Fatal(err)
	}
	// ②导入中悬挂：indexing 无 chunks
	d2, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kb.ID, Title: "import-stalled", Status: "indexing"})
	if err != nil {
		t.Fatal(err)
	}

	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	svc.startIndexWorker() // 内含 RecoverStalledIndexing + worker 消费

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got1, e1 := st.GetKnowledgeDoc(d1.ID)
		got2, e2 := st.GetKnowledgeDoc(d2.ID)
		if e1 == nil && e2 == nil && got1.Status != "indexing" && got2.Status != "indexing" {
			// 两 doc 均离开 indexing = 恢复完成
			if got1.Status != "success" {
				t.Fatalf("重建中悬挂应恢复 success，实得 %s（err=%s）", got1.Status, got1.Error)
			}
			if got2.Status != "failed" || !strings.Contains(got2.Error, "重新导入") {
				t.Fatalf("导入中悬挂应 failed 明示: %s %q", got2.Status, got2.Error)
			}
			// 成功路径 chunk 重建在位（wiki chunksOnly 落 chunks）
			chunks, _ := st.ListKnowledgeChunksByDoc(d1.ID)
			if len(chunks) == 0 {
				t.Fatal("恢复重索后应有 chunks")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("恢复未在 10s 内完成")
}

// TestRecoverSyncPath 同步路径回归：Recover 不影响既有 Import/ReindexAsync（wiki 库快路径）。
func TestRecoverSyncPath(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "rec-kb2", Mode: "wiki"})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	svc.startIndexWorker()
	doc, err := svc.ImportAsync(context.Background(), kb.ID, "常规导入", "常规导入内容切分后索引。")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := st.GetKnowledgeDoc(doc.ID)
		if got.Status != "indexing" {
			if got.Status != "success" {
				t.Fatalf("常规导入应 success: %s %q", got.Status, got.Error)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("常规导入未完成")
}
