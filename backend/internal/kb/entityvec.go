package kb

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ---- KB-4②③/M287：实体「名称+描述」向量召回双入口（写侧刷新 + 读侧融合）----
//
// 35 号 KB-4 草案口径：实体向量存 SQLite BLOB 余弦（kg_entity_vec，迁移 071——沿 vector.go
// 现成模式，实体数百~千级够用），graphrag-search 扩为「chunk 向量 + 实体向量」双入口融合打分。
// 写侧：GraphragIngest 成功后刷新（fingerprint 未变跳过——embedding 调用只花在新/变实体）；
// 读侧：query 向量 vs 实体向量余弦 TopN 并入种子集，种子权重=max(chunk 路径分, 实体向量分)。
// embedding 不可用/失败：写侧降级不阻断抽取链路，读侧如实跳过实体臂（沿 KB-7 vector_degraded 先例）。

// entityVecText 实体向量化文本：名称必带，描述换行拼接（有则拼）。
func entityVecText(name, description string) string {
	if description == "" {
		return name
	}
	return name + "\n" + description
}

// RefreshEntityVectors 全库实体向量刷新（抽取后挂点）：
// 读全库实体 → fingerprint 未变的跳过 → 缺失/变化的批量 embedding → upsert 落 kg_entity_vec。
// 返回 (embedded, skipped, error)；embedding 链路失败返回 error 由调用方降级（不阻断导入）。
func (s *Service) RefreshEntityVectors(ctx context.Context, kbID string) (embedded, skipped int, err error) {
	ents, _, err := s.Store.KGByKB(kbID)
	if err != nil {
		return 0, 0, err
	}
	if len(ents) == 0 {
		return 0, 0, nil
	}
	// 指纹缓存对账：uid → fingerprint
	existing := map[string]string{}
	if rows, verr := s.Store.KGEntityVecs(kbID); verr == nil {
		for _, r := range rows {
			existing[r.UID] = r.Fingerprint
		}
	}
	todo := []*store.KGEntity{}
	for _, e := range ents {
		if existing[e.UID] == store.KGEntityVecFingerprint(e.Name, e.Description) {
			skipped++
			continue
		}
		todo = append(todo, e)
	}
	if len(todo) == 0 {
		return 0, skipped, nil
	}
	texts := make([]string, 0, len(todo))
	for _, e := range todo {
		texts = append(texts, entityVecText(e.Name, e.Description))
	}
	vecs, verr := s.embedder().EmbedTexts(ctx, texts)
	if verr != nil {
		return 0, skipped, verr
	}
	rows := make([]*store.KGEntityVec, 0, len(todo))
	for i, e := range todo {
		b, berr := encodeFloat32(vecs[i])
		if berr != nil {
			continue // 单条编码失败跳过（维度异常防御），不阻断整批
		}
		rows = append(rows, &store.KGEntityVec{
			KBID: kbID, UID: e.UID, Dim: len(vecs[i]), Vec: b,
			Fingerprint: store.KGEntityVecFingerprint(e.Name, e.Description), UpdatedAt: nowRFC3339(),
		})
	}
	if uerr := s.Store.UpsertKGEntityVecs(rows); uerr != nil {
		return 0, skipped, uerr
	}
	log.Printf("[kb] 实体向量刷新 (kb=%s): embedded=%d skipped=%d", kbID, len(rows), skipped)
	return len(rows), skipped, nil
}

// entityVecHit 实体向量臂命中（名称投影——种子集以实体名为键，与 chunk 路径对齐）。
type entityVecHit struct {
	Name  string
	Score float64
}

// entityVecTopN 纯函数：query 向量 vs 实体向量行余弦 TopN（降序）。
// dim 与查询向量不等长或向量异常的行跳过（诚实丢弃，不猜维度——维度混存根治挂 KB-20）。
// topN<=0 取全部。空向量库返回空切片（读侧如实降级纯 chunk 路径）。
func entityVecTopN(query []float32, rows []*store.KGEntityVec, namesByUID map[string]string, topN int) []entityVecHit {
	type scored struct {
		name  string
		score float64
	}
	out := []scored{}
	for _, r := range rows {
		name := r.UID
		if n, ok := namesByUID[r.UID]; ok && n != "" {
			name = n
		}
		vec, ok := decodeFloat32(r.Vec, r.Dim)
		if !ok || len(vec) != len(query) {
			continue // 维度不匹配=该实体向量不可比（如实跳过；KB-20 承接告警面）
		}
		out = append(out, scored{name: name, score: cosine(query, vec)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	hits := make([]entityVecHit, 0, len(out))
	for _, s := range out {
		hits = append(hits, entityVecHit{Name: s.name, Score: s.score})
	}
	return hits
}

// encodeFloat32 / decodeFloat32 向量与 BLOB 的互转（kg_entity_vec 存储形态；
// dim 与字节数不符视为损坏行——decode 返回 false）。
func encodeFloat32(vec []float32) ([]byte, error) {
	if len(vec) == 0 {
		return nil, fmt.Errorf("空向量")
	}
	b := make([]byte, 4*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(v))
	}
	return b, nil
}

func decodeFloat32(b []byte, dim int) ([]float32, bool) {
	if dim <= 0 || len(b) != 4*dim {
		return nil, false
	}
	out := make([]float32, dim)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out, true
}

// nowRFC3339 时间戳（本包独立小函数——store 内部 now() 不外露）。
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
