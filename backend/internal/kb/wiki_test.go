// REQ-241（M67）wiki 摄取管线与检索臂单测：mock LLM 注入（零网络）——
// 五类页面生成 / 未变更 hash 跳过 / 内容变更重生成 / 2-gram 评分命中 / store CRUD 往返。
package kb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newWikiTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "wiki-test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := NewService(st, nil, "sqlite", "")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_ = svc
	return svc, st
}

// seedWikiDoc 直灌文档+chunks（绕开 embedder——wiki 库 indexDoc 路径无 embedding，这里用 store 直写等价形态）。
func seedWikiDoc(t *testing.T, st *store.Store, kbID, title, content string) *store.KnowledgeDoc {
	t.Helper()
	doc, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbID, Title: title, Status: "indexing"})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	pieces := SplitPieces(content)
	chunks := make([]*store.KnowledgeChunk, 0, len(pieces))
	for i, p := range pieces {
		chunks = append(chunks, &store.KnowledgeChunk{
			ID: store.NewID(), KBID: kbID, DocID: doc.ID, Seq: i,
			Content: p.Content, StoreBackend: "none",
		})
	}
	if err := st.InsertKnowledgeChunks(chunks); err != nil {
		t.Fatalf("insert chunks: %v", err)
	}
	if err := st.UpdateKnowledgeDocStatus(doc.ID, "success", len(chunks), ""); err != nil {
		t.Fatalf("status: %v", err)
	}
	out, _ := st.GetKnowledgeDoc(doc.ID)
	return out
}

func TestWikiBuildAndSkip(t *testing.T) {
	svc, st := newWikiTestService(t)
	// 建 wiki 库
	row, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "wiki 单测库", Mode: "wiki"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	d1 := seedWikiDoc(t, st, row.ID, "文档一", strings.Repeat("Kubernetes 编排引擎负责调度智能体工作负载。", 20))
	d2 := seedWikiDoc(t, st, row.ID, "文档二", strings.Repeat("大模型推理会消耗 Token，成本与上下文长度相关。", 20))

	calls := 0
	svc.SetWikiLLM(func(ctx context.Context, connID, prompt, schemaJSON string) (string, error) {
		calls++
		if strings.Contains(prompt, "wiki 编纂器。阅读下面这份文档") {
			return `{"summary":"文档摘要内容","entities":["编排引擎"],"concepts":["调度"]}`, nil
		}
		return `{"pages":[{"type":"entity","title":"编排引擎","content":"跨文档实体页 [[文档一]]","docs":["文档一","文档二"]},{"type":"synthesis","title":"综合","content":"全库综合页","docs":[]},{"type":"bogus","title":"脏页","content":"x","docs":[]}]}`, nil
	})

	info, err := svc.WikiBuild(context.Background(), row.ID, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !info.OK || info.Degraded {
		t.Fatalf("期望成功非降级: %+v", info)
	}
	// 2 文档 summary + 1 聚合 = 3 次；页面 = 2 summary + 2 有效聚合页 + 1 index = 5（脏页被丢弃）
	if info.LLMCalls != 3 {
		t.Fatalf("期望 3 次 LLM 调用，得 %d", info.LLMCalls)
	}
	if info.Pages != 5 {
		t.Fatalf("期望 5 页（含 index，脏类型丢弃），得 %d", info.Pages)
	}
	pages, _ := st.ListWikiPages(row.ID)
	if len(pages) != 5 {
		t.Fatalf("落库页面数 %d", len(pages))
	}
	if pages[0].PageType != "index" {
		t.Fatalf("首页应为 index，得 %s", pages[0].PageType)
	}
	// 聚合页 sources = docs 引用文档的 chunk 并集（两文档各 2 chunk → 4 个）
	var entPage *store.WikiPage
	for _, p := range pages {
		if p.Title == "编排引擎" {
			entPage = p
		}
	}
	if entPage == nil || len(entPage.Sources) != 4 {
		t.Fatalf("聚合页应落库且 sources 为涉及文档 chunk 并集(4): %+v", entPage)
	}

	// 未变更重建：全部跳过，LLM 0 次，页面不替换
	info2, err := svc.WikiBuild(context.Background(), row.ID, "")
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if info2.LLMCalls != 0 || info2.SkippedDocs != 2 {
		t.Fatalf("期望全跳过 0 调用，得 calls=%d skipped=%d", info2.LLMCalls, info2.SkippedDocs)
	}

	// 变更一个文档：只重生成它的 summary + 聚合页 = 2 次调用
	if err := st.DeleteKnowledgeChunksByDoc(d1.ID); err != nil {
		t.Fatalf("del chunks: %v", err)
	}
	chunks := []*store.KnowledgeChunk{{ID: store.NewID(), KBID: row.ID, DocID: d1.ID, Seq: 0, Content: "全新内容：向量数据库 Qdrant 支撑语义检索。", StoreBackend: "none"}}
	if err := st.InsertKnowledgeChunks(chunks); err != nil {
		t.Fatalf("insert: %v", err)
	}
	info3, err := svc.WikiBuild(context.Background(), row.ID, "")
	if err != nil {
		t.Fatalf("rebuild3: %v", err)
	}
	if info3.LLMCalls != 2 || info3.SkippedDocs != 1 {
		t.Fatalf("期望 2 次（1 summary+1 聚合）跳过 1，得 calls=%d skipped=%d", info3.LLMCalls, info3.SkippedDocs)
	}
	_ = d2
}

func TestWikiArmScoring(t *testing.T) {
	svc, _ := newWikiTestService(t)
	st := svc.Store
	row, _ := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "评分库", Mode: "wiki"})
	d := seedWikiDoc(t, st, row.ID, "源文档", "Qdrant 是向量数据库。")
	chunks, _ := st.ListKnowledgeChunksByDoc(d.ID)
	pages := []*store.WikiPage{
		{PageType: "index", Title: "目录", ContentMD: "# 目录"},
		{PageType: "summary", Title: "源文档", ContentMD: "Qdrant 是向量数据库的摘要。", Sources: []string{chunks[0].ID}},
		{PageType: "entity", Title: "Qdrant", ContentMD: "Qdrant：开源向量数据库，支撑语义检索。", Sources: []string{chunks[0].ID}},
		{PageType: "concept", Title: "检索", ContentMD: "语义检索相关概念。"},
	}
	if err := st.ReplaceWikiPages(row.ID, pages); err != nil {
		t.Fatalf("replace: %v", err)
	}
	k, _ := st.GetKnowledgeBase(row.ID)
	hits, err := svc.wikiArm(k, "Qdrant 向量", 4)
	if err != nil {
		t.Fatalf("wikiArm: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("期望命中 Qdrant 相关页")
	}
	if hits[0].Strategy != "wiki" {
		t.Fatalf("strategy 应为 wiki，得 %s", hits[0].Strategy)
	}
	// 命中排序：title 含 Qdrant 的实体页/摘要页应排在概念页前
	if !strings.Contains(hits[0].Doc, "Qdrant") {
		t.Fatalf("首命中应为 Qdrant 页，得 %s", hits[0].Doc)
	}
	// 溯源：实体页 sources 解析为 doc:seq
	foundSrc := false
	for _, h := range hits {
		for _, s := range h.Sources {
			if s.Doc == "源文档" && s.Seq == 0 {
				foundSrc = true
			}
		}
	}
	if !foundSrc {
		t.Fatalf("期望 sources 解析出 源文档#0")
	}
	// index 页不参与命中
	for _, h := range hits {
		if strings.HasSuffix(h.Doc, "目录") {
			t.Fatalf("index 页不应命中")
		}
	}
	// 无关查询零命中 → route 层 degraded 语义（此处验证臂返回空；2-gram 粗匹配下
	// 查询词的 2-gram 须与页面内容零重合）
	empty, err := svc.wikiArm(k, "行星轨道卫星", 4)
	if err != nil || len(empty) != 0 {
		t.Fatalf("无关查询期望空命中: %v %v", empty, err)
	}
}

func TestWikiModeNormalization(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "mode.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	// wiki 库能力开关保持 false（零 embedding 依赖）
	k1, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "w1", Mode: "wiki"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if k1.Mode != "wiki" || k1.KBVector || k1.KBGraph {
		t.Fatalf("wiki 库归一错误: %+v", k1)
	}
	// 未知 mode 回落 rag；rag 全关派生 vector
	k2, _ := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "w2", Mode: "bogus"})
	if k2.Mode != "rag" || !k2.KBVector {
		t.Fatalf("bogus 库归一错误: %+v", k2)
	}
	// 更新为 wiki 亦归一
	k2.Mode = "wiki"
	k2.KBVector = true
	up, err := st.UpdateKnowledgeBase(k2)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if up.Mode != "wiki" || up.KBVector {
		t.Fatalf("update 归一错误: %+v", up)
	}
}

func TestWikiDocHashRoundTrip(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "hash.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	k, _ := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "h", Mode: "wiki"})
	d := seedWikiDoc(t, st, k.ID, "哈希文档", "内容")
	if d.WikiHash != "" {
		t.Fatalf("初始 hash 应为空")
	}
	if err := st.SetDocWikiHash(k.ID, d.ID, "abc123"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := st.GetKnowledgeDoc(d.ID)
	if got.WikiHash != "abc123" {
		t.Fatalf("hash 往返失败: %q", got.WikiHash)
	}
	// summary 页删除（文档删除联动）
	st.ReplaceWikiPages(k.ID, []*store.WikiPage{
		{PageType: "summary", Title: "哈希文档", ContentMD: "x"},
		{PageType: "index", Title: "目录", ContentMD: "y"},
	})
	if err := st.DeleteWikiSummaryPage(k.ID, "哈希文档"); err != nil {
		t.Fatalf("del summary: %v", err)
	}
	pages, _ := st.ListWikiPages(k.ID)
	if len(pages) != 1 || pages[0].PageType != "index" {
		t.Fatalf("期望仅剩 index 页: %+v", pages)
	}
}
