// search.go REQ-358/M158 B（EvoOntology 增量借鉴·93 号《伴生本体实现与EvoOntology方案差异差距
// 分析》§五 P1③/§7.1——Tool 层承载是 EvoOntology 消融最大增益件〔Tool +57%〕：伴生图消费从
// 「每轮注入」（retrieval.go RetrievalContext，零改动）升级「按需查询」的语义检索面，browse/resolve
// 式：自然语言→语义匹配→实体+邻域）。检索逻辑独立函数，只读消费面——不触碰抽取-入图-召回主链路
//（D-O26/93 号 §7.4 落码纪律 1/5：主链路 diff 为零）。
package companion

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb"
)

// ErrCompanionNotBound 智能体未绑定伴生本体（按需查询无图可查）。
var ErrCompanionNotBound = errors.New("该智能体未绑定伴生本体（侧板-伴生本体开启后可查询）")

// SemanticSearchHit 语义检索命中实体（browse/resolve 式返回：实体+定义+邻域边）。
type SemanticSearchHit struct {
	Label      string   `json:"label"`
	Kind       string   `json:"kind,omitempty"`
	Definition string   `json:"definition,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Score      float64  `json:"score"`
	Neighbors  []string `json:"neighbors,omitempty"` // 2 跳邻域边文本（subject-predicate-object）
}

// SemanticSearch 伴生图语义检索（REQ-358 B 按需查询面）：图内实体标签（含定义语料）向量
// 匹配查询→余弦 topK（复用 REQ-194② 标签向量缓存，阈值同召回口径 vecMatchThreshold）→
// 命中实体详情+2 跳邻域。embedding 连接不可用/图不可达时诚实报错（不静默降级——按需查询
// 是显式动作，调用方（模型）应得到可理解错误而非空结果编造）。
func (s *Service) SemanticSearch(ctx context.Context, ontologyID, query string, topK int) ([]SemanticSearchHit, error) {
	if s == nil || ontologyID == "" {
		return nil, ErrCompanionNotBound
	}
	if topK <= 0 || topK > 10 {
		topK = 5
	}
	labels, err := s.queryLabels(ctx, ontologyID) // 读侧兜底拉起（宿主方案不可达返回空）
	if err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		return []SemanticSearchHit{}, nil
	}
	emb := &kb.Embedder{Store: s.Store, Box: s.Box}
	corpus := s.entityDefs(ctx, ontologyID) // 概念画像语料（REQ-313，与召回同源）
	labelVecs, err := s.labelVectors(ctx, ontologyID, labels, corpus, emb)
	if err != nil {
		return nil, fmt.Errorf("实体向量失败（embedding 连接可用性请检查设置-模型连接）: %w", err)
	}
	inVec, err := emb.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询向量失败（embedding 连接可用性请检查设置-模型连接）: %w", err)
	}
	type scored struct {
		label string
		sim   float64
	}
	var cands []scored
	for l, v := range labelVecs {
		if sim := cosineSim(inVec, v); sim >= vecMatchThreshold {
			cands = append(cands, scored{l, sim})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
	if len(cands) > topK {
		cands = cands[:topK]
	}
	out := make([]SemanticSearchHit, 0, len(cands))
	for _, c := range cands {
		hit := SemanticSearchHit{Label: c.label, Score: c.sim}
		if def, conf, _, err := s.queryEntityInfo(ctx, ontologyID, c.label); err == nil {
			hit.Definition, hit.Confidence = def, conf
		}
		if edges, err := s.queryEntityNeighborhood(ctx, ontologyID, c.label); err == nil {
			for _, e := range edges {
				if e.Dir == "in" {
					hit.Neighbors = append(hit.Neighbors, e.Other+" —"+e.Rel+"→ "+c.label)
				} else {
					hit.Neighbors = append(hit.Neighbors, c.label+" —"+e.Rel+"→ "+e.Other)
				}
			}
		}
		out = append(out, hit)
	}
	return out, nil
}
