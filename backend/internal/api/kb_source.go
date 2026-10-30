// REQ-355 R1/M155 装配层适配：kb.Service → chat.KBSource / chat.KBAdminSource。
// kb 包保持零 chat 依赖（产品面独立包）；chat 零 kb 依赖（对话主链路接口反转）——
// 双向解耦在本文件汇合（可插拔判据 2：装配点集中一处）。类型转换保持事件载荷逐字节不变
// （Spans/Sources/Entities/Relationships/Claims 原样透传，空切片归 nil 与旧 len()>0 判定等价）。
package api

import (
	"context"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// KBChatSource kb.Service 的 chat 接口适配器（cmd/backend/main.go 装配注入）。
type KBChatSource struct {
	Svc *kb.Service
}

// 编译期接口契约断言（REQ-355 R1：适配器实现双接口）。
var (
	_ chat.KBSource      = KBChatSource{}
	_ chat.KBAdminSource = KBChatSource{}
)

// NewKBChatSource 构造适配器（kbSvc 为 nil 时返回双双 nil 接口——对话无 KB 注入，沿未接入降级语义）。
func NewKBChatSource(kbSvc *kb.Service) (chat.KBSource, chat.KBAdminSource) {
	if kbSvc == nil {
		return nil, nil
	}
	a := KBChatSource{Svc: kbSvc}
	return a, a
}

// RetrieveContext 实现 chat.KBSource：graphrag 扩展检索（KG 无命中降级向量）+ 注入文本渲染（单源 kb.RenderContext）。
func (a KBChatSource) RetrieveContext(ctx context.Context, cfg *store.KnowledgeBase, query string, topK int, minScore float64) (string, *chat.KBGraphragDetail, string, bool, error) {
	detail, mode, degraded, err := a.Svc.GraphragQueryWithFallbackDetail(ctx, cfg, query, topK, minScore, kb.GraphragOpts{})
	if err != nil || detail == nil {
		return "", nil, mode, degraded, err
	}
	hits := make([]chat.KBHit, 0, len(detail.Hits))
	for _, h := range detail.Hits {
		ch := chat.KBHit{Doc: h.Doc, DocID: h.DocID, Seq: h.Seq, Score: h.Score, Excerpt: h.Excerpt}
		if len(h.Spans) > 0 {
			ch.Spans = h.Spans
		}
		if len(h.Sources) > 0 {
			ch.Sources = h.Sources
		}
		hits = append(hits, ch)
	}
	out := &chat.KBGraphragDetail{Hits: hits, Entities: detail.Entities, Relationships: detail.Relationships, Claims: detail.Claims}
	return kb.RenderContext(cfg.Name, detail.Hits), out, mode, degraded, nil
}

// Import 实现 chat.KBAdminSource（签名与 kb.Service.Import 一致，直通）。
func (a KBChatSource) Import(ctx context.Context, kbID, title, content string) (*store.KnowledgeDoc, error) {
	return a.Svc.Import(ctx, kbID, title, content)
}

// DeleteDoc 实现 chat.KBAdminSource（直通）。
func (a KBChatSource) DeleteDoc(ctx context.Context, kbID, docID string) error {
	return a.Svc.DeleteDoc(ctx, kbID, docID)
}

// Search 实现 chat.KBAdminSource：向量检索命中转 chat 侧投影。
func (a KBChatSource) Search(ctx context.Context, cfg *store.KnowledgeBase, query string, topK int, minScore float64) ([]chat.KBHit, error) {
	hits, err := a.Svc.Search(ctx, cfg, query, topK, minScore)
	if err != nil {
		return nil, err
	}
	out := make([]chat.KBHit, 0, len(hits))
	for _, h := range hits {
		ch := chat.KBHit{Doc: h.Doc, DocID: h.DocID, Seq: h.Seq, Score: h.Score, Excerpt: h.Excerpt}
		if len(h.Spans) > 0 {
			ch.Spans = h.Spans
		}
		if len(h.Sources) > 0 {
			ch.Sources = h.Sources
		}
		out = append(out, ch)
	}
	return out, nil
}
