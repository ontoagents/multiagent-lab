package kg

// KB-18/M290：全局问答社区检索 embedding 臂——摘要向量化缓存 + 词面/余弦双臂融合。
// 设计：
//   - Summarizer.Embed 由装配侧注入（kb.Embedder 适配——kg 不反向依赖 kb）；nil=零 embedding
//     降级纯词面臂（现状语义，词面臂恒保留为回退——KB-18 口径）。
//   - 摘要向量缓存 kg_community_vec（store 层）：检索期惰性补嵌（缓存缺失的社区按摘要 embed
//     后落缓存，下一次生效——不阻塞本次检索）；摘要为空按成员名拼接补嵌。
//   - 融合口径：词面 2-gram 覆盖分（归一 0~1）+ 余弦相似（0~1）各 0.5 权重；Embed 失败/缓存
//     全缺失时词面单臂（score 口径与历史一致——GlobalSearchHit.Score int 语义保持词面分，
//     向量增益经排序体现，不虚构分值）。

import (
	"context"
	"encoding/binary"
	"math"
	"sort"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// EmbedOneFunc embedding 注入形态（kb.Embedder 适配；nil=零 embedding 降级）。
type EmbedOneFunc func(ctx context.Context, text string) ([]float32, error)

// topCommunitiesHybrid 双臂融合 TopN：词面臂（GlobalSearch 评分）+ embedding 余弦臂
//（缓存惰性补嵌）；Embed 为 nil 或失败时纯词面回退（返回值 Score=词面分，历史口径）。
func (x *Summarizer) topCommunitiesHybrid(ctx context.Context, kbID, query string, comms []*store.KGCommunity, n int) []GlobalSearchHit {
	lexHits := GlobalSearch("", query, comms)
	if x.Embed == nil {
		return truncateHits(lexHits, n)
	}
	qvec, err := x.Embed(ctx, query)
	if err != nil || len(qvec) == 0 {
		return truncateHits(lexHits, n)
	}
	// 缓存读取 + 惰性补嵌（缺失社区按摘要或成员名拼接 embed——失败该社区退出向量臂）
	vecs, _, verr := x.Store.ListKGCommunityVecs(kbID)
	if verr != nil {
		vecs = map[string][]byte{}
	}
	byLabel := map[string]*store.KGCommunity{}
	missing := map[string]string{}
	for _, c := range comms {
		byLabel[c.Label] = c
		if _, ok := vecs[c.Label]; !ok {
			text := c.Summary
			if strings.TrimSpace(text) == "" {
				text = "社区成员：" + strings.Join(c.Members, "、")
			}
			missing[c.Label] = text
		}
	}
	if len(missing) > 0 {
		batch := map[string][]byte{}
		dim := len(qvec)
		for label, text := range missing {
			if v, eerr := x.Embed(ctx, text); eerr == nil && len(v) == dim {
				batch[label] = encodeVec(v)
			}
		}
		if len(batch) > 0 {
			if uerr := x.Store.UpsertKGCommunityVecs(kbID, batch, dim); uerr == nil {
				for label, v := range batch {
					vecs[label] = v
				}
			}
		}
	}
	// 融合：词面归一分 + 余弦 0.5/0.5（词面零命中但余弦高的社区仍可入榜——语义臂价值面）
	type fused struct {
		hit  GlobalSearchHit
		sum  float64
		cos  float64
		lex  float64
		rank int
	}
	maxLex := 0
	for _, h := range lexHits {
		if h.Score > maxLex {
			maxLex = h.Score
		}
	}
	lexRank := map[string]int{}
	for i, h := range lexHits {
		lexRank[h.Label] = i + 1
	}
	fusedList := make([]fused, 0, len(comms))
	for _, c := range comms {
		f := fused{hit: GlobalSearchHit{Label: c.Label, Summary: c.Summary, Members: c.Members}}
		if lr, ok := lexRank[c.Label]; ok {
			f.rank = lr
			if maxLex > 0 {
				f.lex = float64(lexRankScore(lexHits, c.Label)) / float64(maxLex)
			}
		}
		if vb, ok := vecs[c.Label]; ok {
			if v := decodeVec(vb); len(v) == len(qvec) {
				f.cos = (cosineVec(qvec, v) + 1) / 2 // [-1,1]→[0,1]
			}
		}
		f.sum = 0.5*f.lex + 0.5*f.cos
		if f.sum > 0 {
			fusedList = append(fusedList, f)
		}
	}
	if len(fusedList) == 0 {
		return truncateHits(lexHits, n)
	}
	sort.Slice(fusedList, func(i, j int) bool {
		if fusedList[i].sum != fusedList[j].sum {
			return fusedList[i].sum > fusedList[j].sum
		}
		return fusedList[i].hit.Label < fusedList[j].hit.Label
	})
	out := make([]GlobalSearchHit, 0, n)
	for _, f := range fusedList {
		if f.rank > 0 {
			f.hit.Score = lexRankScore(lexHits, f.hit.Label) // Score 保持词面分历史口径
		}
		out = append(out, f.hit)
		if len(out) >= n {
			break
		}
	}
	return out
}

// lexRankScore 词面原分（排序融合用原分回填 Score——历史口径不虚构）。
func lexRankScore(hits []GlobalSearchHit, label string) int {
	for _, h := range hits {
		if h.Label == label {
			return h.Score
		}
	}
	return 0
}

func truncateHits(hits []GlobalSearchHit, n int) []GlobalSearchHit {
	if len(hits) > n {
		return hits[:n]
	}
	return hits
}

func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func cosineVec(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
