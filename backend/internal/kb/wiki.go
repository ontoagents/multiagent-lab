// REQ-241（M67）知识库 LLM Wiki 类型：写时合成互链 Markdown 页面层 + index/页面评分检索臂。
// 模式沿 Karpathy LLM Wiki（51 号调研）：文档入库在 chunk 池之外由 LLM 摄取生成五类页面
// （summary 每文档一页 / entity·concept 实体概念累计页 / topic·synthesis 主题综合页 / index 全库目录），
// 页带 sources（chunk_id）溯源；查询读页非检索片段（确定性 2-gram 评分，零 embedding 依赖）。
// 定位=写时合成层，不替代 RAG/GraphRAG（三类型并存可对照，SC-K9）。
// LLM 生成经 WikiLLMFunc 注入（chat → kb 已有依赖，反向 import 成环；api 层装配转 chat.GenerateStructured）。
// 诚实边界：v1 手动全量重建 + 未变更文档 hash 跳过；聚合页在任一文档变更时全量重算；
// 学习尺度 ~百文档内（模式自述 index+评分检索边界）。
package kb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// WikiLLMFunc 结构化生成注入签名（返回 JSON 字符串；schemaJSON 为输出契约）。
type WikiLLMFunc func(ctx context.Context, connID, prompt, schemaJSON string) (string, error)

// wikiSummarySchema / wikiPagesSchema 输出契约（api 层转交 GenerateStructured）。
const (
	wikiSummarySchema = `{"type":"object","properties":{"summary":{"type":"string"},"entities":{"type":"array","items":{"type":"string"}},"concepts":{"type":"array","items":{"type":"string"}}},"required":["summary","entities","concepts"]}`
	wikiPagesSchema   = `{"type":"object","properties":{"pages":{"type":"array","items":{"type":"object","properties":{"type":{"type":"string"},"title":{"type":"string"},"content":{"type":"string"},"docs":{"type":"array","items":{"type":"string"}}},"required":["type","title","content","docs"]}}},"required":["pages"]}`
)

// SetWikiLLM api 层装配（Service 构造后调用；不装配 = wiki 重建 degraded 引导）。
func (s *Service) SetWikiLLM(fn WikiLLMFunc) { s.wikiLLM = fn }

// WikiBuildInfo 重建结果（REST 回显 + 前端成本可见）。
type WikiBuildInfo struct {
	OK          bool     `json:"ok"`
	Pages       int      `json:"pages"`        // 全量替换后页面总数
	LLMCalls    int      `json:"llm_calls"`    // 本次实际 LLM 调用次数（成本可见）
	SkippedDocs int      `json:"skipped_docs"` // 未变更跳过的文档数
	Degraded    bool     `json:"degraded,omitempty"`
	Error       string   `json:"error,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	DurationMS  int64    `json:"duration_ms"`
}

// docCorpus 单文档语料（chunks 拼接 + 指纹）。
type docCorpus struct {
	Doc      *store.KnowledgeDoc
	Content  string
	Hash     string
	ChunkIDs []string
}

// WikiBuild 全量重建（手动触发，沿 REQ-130 重建先例）：
// summary（每文档 1 次 LLM，未变更 hash 跳过）→ 聚合页（entity/concept/topic/synthesis 批量 1 次 LLM，
// 任一文档变更才调）→ index（确定性零 LLM）→ 全量替换入库。
func (s *Service) WikiBuild(ctx context.Context, kbID, connID string) (*WikiBuildInfo, error) {
	start := time.Now()
	k, err := s.Store.GetKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	if k.Mode != "wiki" {
		return nil, fmt.Errorf("仅 wiki 模式知识库支持重建 wiki 页面")
	}
	info := &WikiBuildInfo{Warnings: []string{}}
	docs, err := s.Store.ListKnowledgeDocs(kbID)
	if err != nil {
		return nil, err
	}
	chunks, err := s.Store.ListKnowledgeChunksByKB(kbID)
	if err != nil {
		return nil, err
	}
	byDoc := map[string][]*store.KnowledgeChunk{}
	for _, c := range chunks {
		byDoc[c.DocID] = append(byDoc[c.DocID], c)
	}
	corpora := make([]docCorpus, 0, len(docs))
	for _, d := range docs {
		cs := byDoc[d.ID]
		sort.Slice(cs, func(i, j int) bool { return cs[i].Seq < cs[j].Seq })
		c := docCorpus{Doc: d, ChunkIDs: make([]string, 0, len(cs))}
		var sb strings.Builder
		for _, ch := range cs {
			sb.WriteString(ch.Content)
			sb.WriteString("\n")
			c.ChunkIDs = append(c.ChunkIDs, ch.ID)
		}
		c.Content = strings.TrimSpace(sb.String())
		sum := sha256.Sum256([]byte(c.Content))
		c.Hash = fmt.Sprintf("%x", sum)
		corpora = append(corpora, c)
	}
	if len(corpora) == 0 {
		return nil, fmt.Errorf("知识库为空：请先导入文档再重建 wiki 页面")
	}

	pages := []*store.WikiPage{}
	summaries := make([]string, 0, len(corpora)) // 聚合页输入（title+summary+清单）
	chunksByTitle := map[string][]string{}       // 文档标题 → chunk_id 集合（聚合页 docs 溯源用）
	changed := 0
	oldPages, _ := s.Store.ListWikiPages(kbID) // 旧页面（未变更文档的 summary 沿用）
	oldSummary := map[string]*store.WikiPage{}
	for _, p := range oldPages {
		if p.PageType == "summary" {
			oldSummary[p.Title] = p
		}
	}
	for _, cp := range corpora {
		chunksByTitle[cp.Doc.Title] = cp.ChunkIDs
		if cp.Doc.WikiHash == cp.Hash && cp.Doc.ChunkCount > 0 {
			// 未变更：旧 summary 页原样并入本次全量替换（零 LLM 调用）
			info.SkippedDocs++
			if old := oldSummary[cp.Doc.Title]; old != nil {
				pages = append(pages, old)
			} else {
				// 无旧页（首次重建前导入即重索引等）：按变更处理补生成
				changed++
				sum, entities, concepts, callErr := s.wikiSummarize(ctx, connID, cp)
				if callErr != nil {
					info.Warnings = append(info.Warnings, fmt.Sprintf("文档「%s」摘要生成失败：%v（本次不含其 summary 页）", cp.Doc.Title, callErr))
					continue
				}
				info.LLMCalls++
				pages = append(pages, &store.WikiPage{KBID: kbID, PageType: "summary", Title: cp.Doc.Title, ContentMD: sum, Sources: cp.ChunkIDs})
				summaries = append(summaries, fmt.Sprintf("## %s\n%s\n实体：%s\n概念：%s", cp.Doc.Title, sum, strings.Join(entities, "、"), strings.Join(concepts, "、")))
				_ = s.Store.SetDocWikiHash(kbID, cp.Doc.ID, cp.Hash)
			}
			continue
		}
		changed++
		sum, entities, concepts, callErr := s.wikiSummarize(ctx, connID, cp)
		if callErr != nil {
			info.Warnings = append(info.Warnings, fmt.Sprintf("文档「%s」摘要生成失败：%v（本次不含其 summary 页）", cp.Doc.Title, callErr))
			continue
		}
		info.LLMCalls++
		pages = append(pages, &store.WikiPage{
			KBID: kbID, PageType: "summary", Title: cp.Doc.Title,
			ContentMD: sum, Sources: cp.ChunkIDs,
		})
		summaries = append(summaries, fmt.Sprintf("## %s\n%s\n实体：%s\n概念：%s",
			cp.Doc.Title, sum, strings.Join(entities, "、"), strings.Join(concepts, "、")))
		if err := s.Store.SetDocWikiHash(kbID, cp.Doc.ID, cp.Hash); err != nil {
			log.Printf("[kb] warn: set wiki hash (doc=%s): %v", cp.Doc.ID, err)
		}
	}

	// 聚合页：任一文档变更（或首次）才调 LLM；零变更时旧页面由本次全量替换外的路径保留——
	// v1 简化：零变更直接返回当前页面清单（不替换，连 index 都不变），调用数=0。
	if changed == 0 {
		existing, lerr := s.Store.ListWikiPages(kbID)
		if lerr != nil {
			return nil, lerr
		}
		info.OK, info.Pages, info.DurationMS = true, len(existing), time.Since(start).Milliseconds()
		return info, nil
	}

	aggPages, callErr := s.wikiAggregatePages(ctx, connID, k.Name, summaries, chunksByTitle)
	if callErr != nil {
		info.Degraded = true
		info.Warnings = append(info.Warnings, "聚合页（实体/概念/主题综合）生成失败："+callErr.Error()+"（本次仅 summary+index）")
	} else {
		info.LLMCalls++
		pages = append(pages, aggPages...)
	}
	pages = append(pages, wikiIndexPage(kbID, pages))

	if err := s.Store.ReplaceWikiPages(kbID, pages); err != nil {
		return nil, err
	}
	info.OK, info.Pages, info.DurationMS = true, len(pages), time.Since(start).Milliseconds()
	log.Printf("[kb] wiki rebuilt (kb=%s): pages=%d llm_calls=%d skipped=%d %dms", kbID, info.Pages, info.LLMCalls, info.SkippedDocs, info.DurationMS)
	return info, nil
}

// wikiSummarize 单文档摘要 + 实体/概念清单（1 次 LLM）。
func (s *Service) wikiSummarize(ctx context.Context, connID string, cp docCorpus) (string, []string, []string, error) {
	if s.wikiLLM == nil {
		return "", nil, nil, fmt.Errorf("wiki 生成器未装配")
	}
	body := cp.Content
	if rs := []rune(body); len(rs) > 6000 {
		body = string(rs[:6000]) + "…（截断）"
	}
	prompt := fmt.Sprintf(`你是知识库 wiki 编纂器。阅读下面这份文档（标题：%s），输出 JSON：
1. summary：该文档的 Markdown 摘要（300 字内，覆盖核心事实与结论，不要复述目录）；
2. entities：文中出现的重要实体名（人物/组织/系统/产品等，≤15 个）；
3. concepts：文中涉及的重要概念/术语（≤15 个）。
只输出 JSON。`, cp.Doc.Title) + "\n---\n文档内容：\n" + body
	raw, err := s.wikiLLM(ctx, connID, prompt, wikiSummarySchema)
	if err != nil {
		return "", nil, nil, err
	}
	var out struct {
		Summary  string   `json:"summary"`
		Entities []string `json:"entities"`
		Concepts []string `json:"concepts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return "", nil, nil, fmt.Errorf("解析摘要输出: %w", err)
	}
	return strings.TrimSpace(out.Summary), out.Entities, out.Concepts, nil
}

// wikiAggregatePages 跨文档聚合页批量生成（1 次 LLM：entity/concept/topic/synthesis 四类）。
// chunksByTitle：文档标题 → chunk_id 集合（LLM 返回的 docs 引用经确定性后处理转为 chunk 溯源）。
func (s *Service) wikiAggregatePages(ctx context.Context, connID, kbName string, summaries []string, chunksByTitle map[string][]string) ([]*store.WikiPage, error) {
	if s.wikiLLM == nil {
		return nil, fmt.Errorf("wiki 生成器未装配")
	}
	corpus := strings.Join(summaries, "\n\n")
	if rs := []rune(corpus); len(rs) > 12000 {
		corpus = string(rs[:12000]) + "…（截断）"
	}
	prompt := fmt.Sprintf(`你是知识库「%s」的 wiki 编纂器。以下是各文档的摘要与实体/概念清单。请跨文档综合，输出 JSON pages 数组（4~10 页）：
- type="entity"：重要实体累计档案（跨文档出现、信息量足够的实体；汇总各方说法）；
- type="concept"：重要概念解释页；
- type="topic"：跨文档主题页（按主题组织事实）；
- type="synthesis"：全库主题综合（整体讲了什么、观点之间关系）。
每页：title（简洁名词短语）、content（Markdown，300 字内，可写 [[其他页面标题]] 互链）、docs（涉及文档标题数组，与摘要标题一致）。
信息不足的实体/概念不要建页；不要虚构摘要之外的内容。只输出 JSON。`, kbName) + "\n---\n各文档摘要：\n" + corpus
	raw, err := s.wikiLLM(ctx, connID, prompt, wikiPagesSchema)
	if err != nil {
		return nil, err
	}
	var out struct {
		Pages []struct {
			Type    string   `json:"type"`
			Title   string   `json:"title"`
			Content string   `json:"content"`
			Docs    []string `json:"docs"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil, fmt.Errorf("解析聚合页输出: %w", err)
	}
	docTitles := map[string]bool{}
	for _, s := range summaries {
		if t, ok := strings.CutPrefix(s, "## "); ok {
			if idx := strings.Index(t, "\n"); idx > 0 {
				t = t[:idx]
			}
			docTitles[strings.TrimSpace(t)] = true
		}
	}
	pages := make([]*store.WikiPage, 0, len(out.Pages))
	seenPage := map[string]bool{}
	for _, p := range out.Pages {
		t := strings.TrimSpace(p.Type)
		if t != "entity" && t != "concept" && t != "topic" && t != "synthesis" {
			continue // 未知类型丢弃（契约外的输出不落库）
		}
		title := strings.TrimSpace(p.Title)
		if title == "" || strings.TrimSpace(p.Content) == "" || seenPage[title] {
			continue // 空页与重名页丢弃
		}
		seenPage[title] = true
		sources := []string{}
		seenChunk := map[string]bool{}
		for _, d := range p.Docs { // docs 标题 → chunk_id 并集（库外标题忽略 = 诚实缩源）
			for _, id := range chunksByTitle[strings.TrimSpace(d)] {
				if !seenChunk[id] {
					seenChunk[id] = true
					sources = append(sources, id)
				}
			}
		}
		pages = append(pages, &store.WikiPage{
			PageType: t, Title: title, ContentMD: strings.TrimSpace(p.Content), Sources: sources,
		})
	}
	return pages, nil
}

// wikiIndexPage 全库目录（确定性生成，零 LLM）。
func wikiIndexPage(kbID string, pages []*store.WikiPage) *store.WikiPage {
	groups := map[string][]string{}
	for _, p := range pages {
		groups[p.PageType] = append(groups[p.PageType], p.Title)
	}
	var b strings.Builder
	b.WriteString("# 目录\n\n本 wiki 当前共 " + fmt.Sprint(len(pages)) + " 页（含本页）。\n")
	for _, g := range []struct{ key, label string }{
		{"summary", "文档摘要"}, {"entity", "实体"}, {"concept", "概念"}, {"topic", "主题"}, {"synthesis", "综合"},
	} {
		if titles := groups[g.key]; len(titles) > 0 {
			b.WriteString("\n## " + g.label + "\n")
			for _, t := range titles {
				b.WriteString("- [[" + t + "]]\n")
			}
		}
	}
	return &store.WikiPage{KBID: kbID, PageType: "index", Title: "目录", ContentMD: strings.TrimRight(b.String(), "\n")}
}

// ---- wiki 检索臂（index + 页面评分；确定性 2-gram，沿 REQ-130 global-search 同款思路）----

// wikiArm 查询词对页面 title（权重 ×3）+content 覆盖评分，TopK 页面作为命中。
func (s *Service) wikiArm(k *store.KnowledgeBase, query string, topK int) ([]RetrievalHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	pages, err := s.Store.ListWikiPages(k.ID)
	if err != nil {
		return nil, err
	}
	if topK <= 0 {
		topK = k.TopK
	}
	if topK <= 0 {
		topK = 4
	}
	// chunk_id → (docID, seq)（sources 下钻呈现用；学习尺度全量拉取无压力）
	chunks, err := s.Store.ListKnowledgeChunksByKB(k.ID)
	if err != nil {
		return nil, err
	}
	docIDs := make([]string, 0, len(chunks))
	type chunkPosEntry struct {
		docID string
		seq   int
	}
	chunkPos := map[string]chunkPosEntry{}
	for _, c := range chunks {
		docIDs = append(docIDs, c.DocID)
		chunkPos[c.ID] = chunkPosEntry{docID: c.DocID, seq: c.Seq}
	}
	titles, _ := s.Store.DocTitles(uniqueStrings(docIDs))

	terms := wikiGramTerms(query)
	type scored struct {
		page  *store.WikiPage
		score float64
	}
	hits := []scored{}
	for _, p := range pages {
		if p.PageType == "index" {
			continue // 目录页不参与命中（导航专用）
		}
		titleHits, bodyHits := 0, 0
		for _, t := range terms {
			if strings.Contains(p.Title, t) {
				titleHits++
			}
			if strings.Contains(p.ContentMD, t) {
				bodyHits++
			}
		}
		if titleHits == 0 && bodyHits == 0 {
			continue
		}
		score := (float64(titleHits)*3 + float64(bodyHits)) / (float64(len(terms)) * 3)
		if strings.Contains(p.Title, strings.TrimSpace(query)) { // 整题命中加成
			score = 1.0
		}
		hits = append(hits, scored{page: p, score: score})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].page.Title < hits[j].page.Title // 并列按标题稳定
	})
	out := make([]RetrievalHit, 0, topK)
	for _, h := range hits {
		if len(out) >= topK {
			break
		}
		src := make([]HitSource, 0, len(h.page.Sources))
		for _, id := range h.page.Sources {
			if p, ok := chunkPos[id]; ok {
				src = append(src, HitSource{Doc: titles[p.docID], Seq: p.seq})
			}
		}
		out = append(out, RetrievalHit{
			Doc: k.Name + " · " + h.page.Title, Score: h.score,
			Excerpt: truncateRunes(h.page.ContentMD, 200), Strategy: "wiki", Sources: src,
		})
	}
	return out, nil
}

// wikiGramTerms 中文 2-gram + 空格分词（零分词依赖的粗粒度匹配面；kg.gramTerms 同思路的 kb 包副本——
// 跨包复用需引 kg（其 import chat 会与 kb 成环），学习尺度下 30 行副本成本更低）。
func wikiGramTerms(q string) []string {
	set := map[string]bool{}
	for _, field := range strings.Fields(q) {
		rs := []rune(field)
		if len(rs) <= 2 {
			set[field] = true
			continue
		}
		for i := 0; i+2 <= len(rs); i++ {
			set[string(rs[i:i+2])] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
