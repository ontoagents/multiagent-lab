package kb

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// Service 知识库编排：导入切分 → Embedding → VectorStore → 状态回写；检索；删除级联（§6.9）。
type Service struct {
	Store  *store.Store
	Box    *secrets.Box
	Vector VectorStore

	// kgExtract KG 抽取注入点（D-O15：internal/kg 实现，api 层装配；nil = graphrag 抽取 degraded）
	kgExtract KGExtractFunc

	// wikiLLM wiki 页面生成注入点（REQ-241：chat.GenerateStructured 的转接闭包，api 层装配；
	// nil = wiki 重建引导态「生成器未装配」）
	wikiLLM WikiLLMFunc

	// jobs 后台索引队列（REQ-440/KB-8③：NewService 起单 worker；容量见 async.go）
	jobs chan indexJob
}

// NewService 构造（backend=qdrant|sqlite，qdrantURL 见 §469 QDRANT_URL）。
func NewService(st *store.Store, box *secrets.Box, backend, qdrantURL string) (*Service, error) {
	vs, err := NewVectorStore(backend, qdrantURL, st)
	if err != nil {
		return nil, err
	}
	s := &Service{Store: st, Box: box, Vector: vs}
	s.startIndexWorker() // REQ-440：后台串行索引队列（ImportAsync/ReindexAsync 消费）
	if n, err := st.MarkStaleRebuildJobs(); err == nil && n > 0 {
		log.Printf("[kb] 启动对账：%d 个库级重建任务标记 failed（进程重启中断，KB-8②）", n)
	}
	return s, nil
}

// embedder 惰性构造（依赖 Store/Box）。
// embedClient 向量化最小接口（KB-20 测试注入点：embedOverride 非 nil 时绕过库装配）。
type embedClient interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
	EmbedOne(ctx context.Context, text string) ([]float32, error)
}

// embedOverride 测试注入（生产恒 nil——embedder() 走库装配）。
var embedOverride embedClient

func embedClientOf(s *Service) embedClient {
	if embedOverride != nil {
		return embedOverride
	}
	return s.embedder()
}

func (s *Service) embedder() *Embedder { return &Embedder{Store: s.Store, Box: s.Box} }

// Import 粘贴文本导入并同步索引（内部路径：chat.KBAdminSource/平台助手等单条小文本场景；
// HTTP 端点走 ImportAsync——REQ-440 落库即返回+后台队列）。
func (s *Service) Import(ctx context.Context, kbID, title, content string) (*store.KnowledgeDoc, error) {
	kbcfg, err := s.Store.GetKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	pieces := SplitPieces(content)
	if len(pieces) == 0 {
		return nil, fmt.Errorf("内容为空，无法索引")
	}
	doc, err := s.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbID, Title: title, Status: "indexing"})
	if err != nil {
		return nil, err
	}
	chunks, err := s.indexDoc(ctx, kbcfg, kbID, doc, pieces)
	if err != nil {
		s.Store.UpdateKnowledgeDocStatus(doc.ID, "failed", 0, 0, err.Error())
		return nil, fmt.Errorf("索引失败: %w", err)
	}
	out, err := s.Store.GetKnowledgeDoc(doc.ID)
	if err != nil {
		return nil, err
	}
	if kbcfg.Mode == "graphrag" {
		out.Graphrag = s.GraphragIngest(ctx, kbID, chunks) // M14 ②：chunks → KG 抽取（降级不阻断）
		s.persistGraphrag(kbID, doc.ID, out.Graphrag)      // REQ-440：结果落库（与异步路径一致）
	}
	return out, nil
}

// Reindex 重建文档索引（同步内部路径；HTTP 端点走 ReindexAsync——REQ-440）。
func (s *Service) Reindex(ctx context.Context, kbID, docID string) (*store.KnowledgeDoc, error) {
	kbcfg, err := s.Store.GetKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	doc, err := s.Store.GetKnowledgeDoc(docID)
	if err != nil {
		return nil, err
	}
	if doc.KBID != kbID {
		return nil, store.ErrNotFound
	}
	pieces, err := s.clearForReindex(ctx, kbID, docID, kbcfg) // 旧正文拼接+清理（async.go 共用）
	if err != nil {
		return nil, err
	}
	s.Store.UpdateKnowledgeDocStatus(docID, "indexing", 0, 0, "")
	chunks, err := s.indexDoc(ctx, kbcfg, kbID, doc, pieces)
	if err != nil {
		s.Store.UpdateKnowledgeDocStatus(docID, "failed", 0, 0, err.Error())
		return nil, fmt.Errorf("索引失败: %w", err)
	}
	out, err := s.Store.GetKnowledgeDoc(doc.ID)
	if err != nil {
		return nil, err
	}
	if kbcfg.Mode == "graphrag" {
		out.Graphrag = s.GraphragIngest(ctx, kbID, chunks) // 同步重抽 KG（降级不阻断）
		s.persistGraphrag(kbID, doc.ID, out.Graphrag)      // REQ-440：结果落库（与异步路径一致）
	}
	return out, nil
}

// indexDoc 切分（KB-10② 父子块）→embed→写入向量库与 chunks→状态回写（返回落库 chunks 供 graphrag 联动）。
// REQ-241：wiki 模式走 chunksOnly 路径（chunk 池供 wiki 溯源/重建，不 embed 不写向量——检索读页非片段，
// 兑现「零 embedding 依赖」：wiki 库无需向量连接即可用）。
func (s *Service) indexDoc(ctx context.Context, kbcfg *store.KnowledgeBase, kbID string, doc *store.KnowledgeDoc, pieces []Piece) ([]*store.KnowledgeChunk, error) {
	start := time.Now()
	if kbcfg.Mode == "wiki" {
		chunks := make([]*store.KnowledgeChunk, 0, len(pieces))
		for i, piece := range pieces {
			chunks = append(chunks, &store.KnowledgeChunk{
				ID: store.NewID(), KBID: kbID, DocID: doc.ID, Seq: i,
				Content: piece.Content, ParentContent: piece.Parent, StoreBackend: "none",
			})
		}
		if err := s.Store.InsertKnowledgeChunks(chunks); err != nil {
			return nil, fmt.Errorf("save chunks: %w", err)
		}
		if err := s.Store.UpdateKnowledgeDocStatus(doc.ID, "success", len(pieces), 0, ""); err != nil { // wiki 零 embedding：dim=0 如实
			return nil, err
		}
		log.Printf("[kb] doc %q indexed (wiki, no embedding): %d chunks, %s", doc.Title, len(pieces), time.Since(start).Round(time.Millisecond))
		return chunks, nil
	}
	contents := make([]string, len(pieces))
	for i, p := range pieces {
		contents[i] = p.Content
	}
	vecs, err := embedClientOf(s).EmbedTexts(ctx, contents)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(pieces) {
		return nil, fmt.Errorf("embedding 数量不匹配: %d/%d", len(vecs), len(pieces))
	}
	dim := len(vecs[0])
	if dim == 0 {
		return nil, fmt.Errorf("embedding 维度为 0")
	}
	// KB-20/M292：库级维度登记与写入校验（维度漂移根治——换 embedding 模型拒绝写而非混存）
	if kbcfg.EmbedDim > 0 && kbcfg.EmbedDim != dim {
		return nil, DimMismatchError{KB: kbcfg.Name, Want: kbcfg.EmbedDim, Have: dim}
	}
	if kbcfg.EmbedDim == 0 {
		registered, err := s.Store.SetKnowledgeBaseEmbedDim(kbID, dim)
		if err != nil {
			return nil, fmt.Errorf("登记库维度失败: %w", err)
		}
		if registered {
			log.Printf("[kb] 库 %q 登记 embedding 维度 %d（首次索引）", kbcfg.Name, dim)
		}
	}
	backend := s.backendName()
	if backend == "qdrant" {
		if err := s.Vector.EnsureCollection(ctx, kbID, dim); err != nil {
			return nil, fmt.Errorf("qdrant collection: %w", err)
		}
	}
	chunks := make([]*store.KnowledgeChunk, 0, len(pieces))
	pts := make([]Chunk, 0, len(pieces))
	for i, piece := range pieces {
		c := &store.KnowledgeChunk{
			ID:            store.NewID(), // 先生成：Qdrant point id 与 chunk id 一一对应
			KBID:          kbID,
			DocID:         doc.ID,
			Seq:           i,
			Content:       piece.Content,
			ParentContent: piece.Parent, // KB-10②：子块检索、父块召回上下文（冗余存，检索/索引只见子块）
			StoreBackend:  backend,
		}
		if backend == "qdrant" {
			c.VectorRef = pointUUIDOf(c.ID)
		} else {
			c.Vector = EncodeVector(vecs[i])
		}
		chunks = append(chunks, c)
		pts = append(pts, Chunk{ID: c.ID, DocID: doc.ID, Seq: i, Content: piece.Content, Vector: vecs[i]})
	}
	if backend == "qdrant" {
		if err := s.Vector.Upsert(ctx, kbID, pts); err != nil {
			return nil, fmt.Errorf("qdrant upsert: %w", err)
		}
	}
	if err := s.Store.InsertKnowledgeChunks(chunks); err != nil {
		return nil, fmt.Errorf("save chunks: %w", err)
	}
	if err := s.Store.UpdateKnowledgeDocStatus(doc.ID, "success", len(pieces), dim, ""); err != nil {
		return nil, err
	}
	withParent := 0
	for _, p := range pieces {
		if p.Parent != "" {
			withParent++
		}
	}
	log.Printf("[kb] doc %q indexed: %d chunks（父子块 %d）, dim=%d, %s", doc.Title, len(pieces), withParent, dim, time.Since(start).Round(time.Millisecond))
	return chunks, nil
}

// DeleteDoc 删除文档：向量库级联清理 + chunks + doc（wiki 模式移除其 summary 页，聚合页留待重建收敛）。
func (s *Service) DeleteDoc(ctx context.Context, kbID, docID string) error {
	doc, err := s.Store.GetKnowledgeDoc(docID)
	if err != nil {
		return err
	}
	if err := s.Vector.DeleteByDoc(ctx, kbID, docID); err != nil {
		// 向量库清理失败不阻塞删除（chunks 已清，残留 point 无引用；SQLite 路径无操作）
		log.Printf("[kb] warn: delete vectors for doc %s: %v", docID, err)
	}
	if err := s.Store.DeleteKnowledgeChunksByDoc(docID); err != nil {
		return err
	}
	if kbcfg, kerr := s.Store.GetKnowledgeBase(kbID); kerr == nil && kbcfg.Mode == "wiki" {
		_ = s.Store.DeleteWikiSummaryPage(kbID, doc.Title)
	}
	// KB-8①/M288：受影响子图增量清理——空抽取替换语义（ReplaceKGForDoc doc 级删除分支：
	// 清该 doc 的关系/claims/chunk_entity 倒排与实体行；跨 doc 共享实体〔upsert 单行
	// doc_id=最后写入者〕被误删时重索引留存 doc 即复原——KB-16 语义既有边界如实）。
	// graphrag 检索面不再命中已删文档的实体/claims（原缺陷：删除后 KG 行残留）。
	if kbcfg, kerr := s.Store.GetKnowledgeBase(kbID); kerr == nil && kbcfg.Mode == "graphrag" {
		if err := s.Store.ReplaceKGForDoc(kbID, docID, nil, nil, nil); err != nil {
			// 清理失败不阻塞删除（残留行无 chunk 引用；重建入口可再清）——log 留排障
			log.Printf("[kb] warn: delete KG subgraph for doc %s: %v", docID, err)
		}
	}
	return s.Store.DeleteKnowledgeDoc(kbID, docID)
}

// DeleteKB 删除整个知识库（逐 doc 清向量；wiki 页面级联清理 REQ-241）。
func (s *Service) DeleteKB(ctx context.Context, kbID string) error {
	docs, err := s.Store.ListKnowledgeDocs(kbID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		s.Vector.DeleteByDoc(ctx, kbID, d.ID)
	}
	if err := s.Store.DeleteWikiPagesByKB(kbID); err != nil {
		log.Printf("[kb] warn: delete wiki pages for kb %s: %v", kbID, err)
	}
	return s.Store.DeleteKnowledgeBase(kbID)
}

// Search 向量臂检索（KB-11 起为统一出口的向量臂实现，亦可独立调用）：混合检索 + 父块上下文回溯。
func (s *Service) Search(ctx context.Context, kb *store.KnowledgeBase, query string, topK int, minScore float64) ([]RetrievalHit, error) {
	return s.vectorArm(ctx, kb, query, topK, minScore)
}

// vectorArm 向量臂：问题 Embedding → 混合检索（KB-10① 词法+向量 RRF）→ 父块上下文（KB-10②）→ hits。
func (s *Service) vectorArm(ctx context.Context, kb *store.KnowledgeBase, query string, topK int, minScore float64) ([]RetrievalHit, error) {
	if query == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = kb.TopK
	}
	if topK <= 0 {
		topK = 4
	}
	if minScore <= 0 {
		minScore = kb.MinScore
	}
	vec, err := embedClientOf(s).EmbedOne(ctx, query)
	if err != nil {
		return nil, err
	}
	// KB-20/M292：检索端维度校验——库登记维度与当前模型输出不匹配时报可懂错误（不再静默丢召回）。
	// kb 参数为调用方快照，重取一次拿最新 EmbedDim（改连 embedding 模型后登记即时生效）。
	if kbcfg, gerr := s.Store.GetKnowledgeBase(kb.ID); gerr == nil && kbcfg.EmbedDim > 0 && kbcfg.EmbedDim != len(vec) {
		return nil, DimMismatchError{KB: kbcfg.Name, Want: kbcfg.EmbedDim, Have: len(vec)}
	}
	hits, err := s.hybridSearch(ctx, kb.ID, query, vec, topK, minScore)
	if err != nil {
		return nil, err
	}
	// 解析文档标题
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.DocID)
	}
	titles, err := s.Store.DocTitles(ids)
	if err != nil {
		return nil, err
	}
	out := make([]RetrievalHit, 0, len(hits))
	for _, h := range hits {
		name := titles[h.DocID]
		if name == "" {
			name = h.DocID
		}
		content := h.Content
		if h.ParentContent != "" { // KB-10②：命中子块，召回父块上下文
			content = h.ParentContent
		}
		excerpt := truncateRunes(content, 200)
		// B1 引用溯源：命中区间（相对 excerpt 的 rune 偏移；父块命中时区间在父块全文上计算后裁剪）
		spans := MatchSpans(content, query)
		if len([]rune(excerpt)) < len([]rune(content)) {
			spans = ClipSpans(spans, len([]rune(excerpt)))
		}
		out = append(out, RetrievalHit{Doc: name, DocID: h.DocID, Seq: h.Seq, Score: h.Score, Excerpt: excerpt, Strategy: h.Strategy, Spans: spans})
	}
	return out, nil
}

// RetrievalHit retrieval 事件 / search-preview 响应条目（§306：hits[{doc,seq,score,excerpt}]）。
type RetrievalHit struct {
	Doc      string  `json:"doc"`
	DocID    string  `json:"doc_id,omitempty"` // REQ-523/M330：溯源最后一跳——命中可下钻原文（graphrag 实体/claims 无文档为空）
	Seq      int     `json:"seq"`
	Score    float64 `json:"score"`
	Excerpt  string  `json:"excerpt"`
	Strategy string  `json:"strategy,omitempty"` // KB-10①：vector|lexical|hybrid|graph|wiki（只增枚举，空 = 历史口径）
	// Spans 命中区间（B1 引用溯源：Excerpt 内 rune 偏移；空 = 无词项命中，如实不标）
	Spans []Span `json:"spans,omitempty"`
	// Sources 命中页溯源定位（REQ-241 wiki 臂：chunk_id 解析为 doc 标题+序号，回答可下钻原文；
	// 非wiki 臂为空）
	Sources []HitSource `json:"sources,omitempty"`
}

// HitSource wiki 页 sources 的可读定位（doc 标题 + chunk 序号）。
type HitSource struct {
	Doc   string `json:"doc,omitempty"`
	DocID string `json:"doc_id,omitempty"` // REQ-523/M330：来源页可下钻原文
	Seq   int    `json:"seq"`
}

// RenderContext 检索结果注入文本（§341：[片段 doc:seq score] 格式）。
func RenderContext(kbName string, hits []RetrievalHit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# 知识库参考（来自知识库「" + kbName + "」，回答时优先依据以下片段并标注来源）\n")
	for _, h := range hits {
		fmt.Fprintf(&b, "[片段 %s:%d 得分%.3f]\n%s\n\n", h.Doc, h.Seq, h.Score, h.Excerpt)
	}
	return strings.TrimRight(b.String(), "\n")
}

// backendName 当前向量后端（sqlite|qdrant）。
func (s *Service) backendName() string {
	switch s.Vector.(type) {
	case *QdrantStore:
		return "qdrant"
	default:
		return "sqlite"
	}
}

// Healthz 向量后端可达性（healthz 汇总用，§8）：sqlite 恒 ok；qdrant 探测 /collections。
func (s *Service) Healthz(ctx context.Context) string {
	switch v := s.Vector.(type) {
	case *QdrantStore:
		if v.ping(ctx) {
			return "qdrant-ok"
		}
		return "qdrant-unreachable"
	default:
		return "sqlite-ok"
	}
}

// pointUUIDOf 与 qdrant.go 的 pointUUID 一致（避免循环依赖，此包内直接复用）。
func pointUUIDOf(chunkID string) string { return pointUUID(chunkID) }

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
