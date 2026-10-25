package kb

// REQ-440/KB-8③（M236）索引异步化单测：落库即返回（indexing）→后台串行队列→
// 状态机收敛 success|failed；graphrag 抽取结果落库回读（KG 徽标持久可见顺修）；
// ReindexAsync 清旧重索。均用 wiki 库（零 embedding）或 graphrag degraded 路径（kgExtract
// 未装配）——零网络依赖。
import (
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// waitDocStable 轮询至文档脱离 indexing/pending（后台 worker 完成或失败）。
func waitDocStable(t *testing.T, st *store.Store, docID string) *store.KnowledgeDoc {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d, err := st.GetKnowledgeDoc(docID)
		if err != nil {
			t.Fatalf("get doc: %v", err)
		}
		if d.Status != "indexing" && d.Status != "pending" {
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("后台索引 5s 未收敛")
	return nil
}

func TestImportAsyncWiki(t *testing.T) {
	svc, st := newWikiTestService(t)
	kbw, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "async-wiki", Mode: "wiki"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.ImportAsync(nil, kbw.ID, "异步导入", strings.Repeat("REQ-440 异步索引验证。", 120))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != "indexing" {
		t.Fatalf("ImportAsync 应立即返回 indexing 态，得 %s", doc.Status)
	}
	out := waitDocStable(t, st, doc.ID)
	if out.Status != "success" || out.ChunkCount == 0 {
		t.Fatalf("后台应收敛 success+chunks>0，得 status=%s chunks=%d", out.Status, out.ChunkCount)
	}
	if out.Dim != 0 {
		t.Fatalf("wiki 零 embedding dim 应为 0，得 %d", out.Dim)
	}
	if out.Graphrag != nil {
		t.Fatal("wiki 库不应有 graphrag 结果")
	}
}

// TestGraphragPersistRoundtrip KG 抽取结果落库→列表回读（REQ-440 顺修「刷新即丢」的核心链；
// graphrag 全链〔索引成功→抽取→落库〕依赖 embedding 连接，headless 冒烟沿 20 号 5.9 真机验证）。
func TestGraphragPersistRoundtrip(t *testing.T) {
	svc, st := newWikiTestService(t)
	kbg, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "async-graphrag", Mode: "graphrag"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbg.ID, Title: "KG 落库回读", Status: "indexing"})
	if err != nil {
		t.Fatal(err)
	}
	svc.persistGraphrag(kbg.ID, doc.ID, &store.GraphragInfo{
		OK: true, Method: "lightweight", Chunks: 3, Entities: 7, Relationships: 4,
	})
	docs, err := st.ListKnowledgeDocs(kbg.ID)
	if err != nil || len(docs) != 1 {
		t.Fatalf("列表回读: %v %d", err, len(docs))
	}
	g := docs[0].Graphrag
	if g == nil || !g.OK || g.Method != "lightweight" || g.Entities != 7 {
		t.Fatalf("graphrag 落库回读不符: %+v", g)
	}
	// rag 模式/无结果文档不带 graphrag（nil 语义保持）
	kbr, _ := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "async-rag", Mode: "rag"})
	d2, _ := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbr.ID, Title: "rag-doc"})
	docs2, _ := st.ListKnowledgeDocs(kbr.ID)
	if docs2[0].Graphrag != nil || docs2[0].ID != d2.ID {
		t.Fatalf("rag 库不应回读出 graphrag: %+v", docs2[0].Graphrag)
	}
}

func TestReindexAsync(t *testing.T) {
	svc, st := newWikiTestService(t)
	kbw, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "async-reindex", Mode: "wiki"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.ImportAsync(nil, kbw.ID, "重索源", strings.Repeat("重建索引异步验证正文。", 150))
	if err != nil {
		t.Fatal(err)
	}
	first := waitDocStable(t, st, doc.ID)
	if first.Status != "success" {
		t.Fatalf("首轮应 success，得 %s", first.Status)
	}
	got, err := svc.ReindexAsync(nil, kbw.ID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "indexing" {
		t.Fatalf("ReindexAsync 应立即返回 indexing，得 %s", got.Status)
	}
	second := waitDocStable(t, st, doc.ID)
	if second.Status != "success" || second.ChunkCount == 0 {
		t.Fatalf("重索应收敛 success+chunks>0，得 %s %d", second.Status, second.ChunkCount)
	}
	chunks, err := st.ListKnowledgeChunksByDoc(doc.ID)
	if err != nil || len(chunks) != second.ChunkCount {
		t.Fatalf("重索后 chunks 应与计数一致: %v %d/%d", err, len(chunks), second.ChunkCount)
	}
}
