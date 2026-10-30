package kb

// KB-19/M291：消歧自动合并+可撤销——高置信向量建议（sim ≥ 自动阈值且类型相同）自动执行
// 合并，操作记录（含撤销快照）落 kg_decision 审计链；一键撤销=快照重放（尽力恢复语义）。
// 双保险：自动阈值（默认 0.95）严于建议阈值（0.85），类型不同（同名异义风险）永不自动并。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// autoMergeThreshold 自动合并阈值（env KG_MERGE_AUTO_THRESHOLD，默认 0.95——严于建议 0.85）。
func autoMergeThreshold() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("KG_MERGE_AUTO_THRESHOLD"), 64); err == nil && v > 0 && v <= 1 {
		return v
	}
	return 0.95
}

// mergeSnapshot 撤销快照（合并前置状态——undo 快照重放的依据）。
type mergeSnapshot struct {
	Merge     string                  `json:"merge"`      // 被并实体名
	Entity    *store.KGEntity         `json:"entity"`     // merge 实体原行
	FromAlias string                  `json:"from_alias"` // keep 合并前 alias（undo 摘除比对）
	Relations []*store.KGRelationship `json:"relations"`  // source/target 命中 merge 的关系原值（含自环）
	Claims    []string                `json:"claims"`     // subject 命中 merge 的 claim id 清单（undo 恢复 subject）
}

// AutoMergeHighConfidence 高置信自动合并：向量建议 ≥ 自动阈值且类型相同 → 合并+决策记录
// （快照入 MetaJSON）；返回本轮自动合并记录。无候选时返回空清单（零合并如实）。
func (s *Service) AutoMergeHighConfidence(ctx context.Context, kbID string) ([]store.OntoDecision, error) {
	sugg, err := s.KGVectorMergeSuggestions(ctx, kbID, 50)
	if err != nil {
		return nil, err
	}
	th := autoMergeThreshold()
	out := make([]store.OntoDecision, 0)
	for _, sg := range sugg {
		if sg.Strategy != "vector" || sg.Similarity < th || sg.TypeWarning != "" {
			continue // 双保险：仅 vector 策略+达自动阈值+类型相同
		}
		keep, merge := sg.Keep, sg.Merge
		// 快照（合并前置状态）
		snap, err := s.snapshotForMerge(kbID, merge, keep)
		if err != nil {
			return out, fmt.Errorf("快照失败（merge=%s）: %w", merge, err)
		}
		if _, _, err := s.Store.MergeKGEntities(kbID, keep, []string{merge}); err != nil {
			return out, fmt.Errorf("自动合并失败（keep=%s merge=%s）: %w", keep, merge, err)
		}
		meta, _ := json.Marshal(map[string]any{
			"kind": "kb19_auto_merge", "keep": keep, "merge": merge,
			"similarity": sg.Similarity, "undone": false, "snapshot": snap,
		})
		d, err := s.Store.InsertDecision(&store.OntoDecision{
			SubjectKind: "kg", SubjectID: kbID,
			Title:     fmt.Sprintf("KB-19 高置信自动合并：%s ← %s（相似度 %.3f）", keep, merge, sg.Similarity),
			Rationale: "向量消歧自动合并（KB-19/M291）——sim ≥ " + fmt.Sprintf("%.2f", th) + " 且类型相同；可经 undo 端点快照恢复",
			MetaJSON:  string(meta),
		})
		if err != nil {
			return out, err
		}
		out = append(out, *d)
		log.Printf("[kb] kb=%s 高置信自动合并 %q ← %q（sim=%.3f，决策 %s 可撤销）", kbID, keep, merge, sg.Similarity, d.ID)
	}
	return out, nil
}

// snapshotForMerge 合并前置快照：merge 实体行+命中关系原值+命中 claim id 清单+keep 原 alias。
func (s *Service) snapshotForMerge(kbID, merge, keep string) (*mergeSnapshot, error) {
	ents, _, err := s.Store.KGByKB(kbID)
	if err != nil {
		return nil, err
	}
	snap := &mergeSnapshot{Merge: merge}
	for _, e := range ents {
		if e.Name == merge {
			cp := *e
			snap.Entity = &cp
		}
		if e.Name == keep {
			snap.FromAlias = e.Alias
		}
	}
	if snap.Entity == nil {
		return nil, fmt.Errorf("merge 实体 %q 不存在", merge)
	}
	if snap.Relations, err = s.Store.KGRelationsInvolving(kbID, merge); err != nil {
		return nil, err
	}
	if snap.Claims, err = s.Store.KGClaimIDsBySubject(kbID, merge); err != nil {
		return nil, err
	}
	return snap, nil
}

// UndoAutoMerge 一键撤销：按决策记录快照重放（尽力恢复语义——合并后新写入不回滚）：
// keep alias 摘除 merge 名 → 重插 merge 实体原行 → 关系原值重插（可能补回合并期被自环清理
// 的边——重复关系对检索语义无害）→ claims subject 恢复。已撤销记录幂等拒绝。
func (s *Service) UndoAutoMerge(ctx context.Context, kbID, decisionID string) (*store.OntoDecision, error) {
	d, err := s.Store.GetDecision(decisionID)
	if err != nil {
		return nil, err
	}
	if d.SubjectID != kbID {
		return nil, fmt.Errorf("决策 %s 不属于该知识库", decisionID)
	}
	var meta struct {
		Kind       string         `json:"kind"`
		Keep       string         `json:"keep"`
		Merge      string         `json:"merge"`
		Similarity float64        `json:"similarity"`
		Undone     bool           `json:"undone"`
		Snapshot   *mergeSnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(d.MetaJSON), &meta); err != nil {
		return nil, fmt.Errorf("决策记录解析失败: %w", err)
	}
	if meta.Kind != "kb19_auto_merge" {
		return nil, fmt.Errorf("该决策不是 KB-19 自动合并记录")
	}
	// 幂等拒绝：决策记录只增不改——撤销态经撤销链记录（DerivedFrom=原决策 id）判定
	undoLog, err := s.Store.ListDecisions("kg", kbID, 200)
	if err == nil {
		for _, u := range undoLog {
			if u.DerivedFrom == decisionID && strings.Contains(u.Title, "撤销自动合并") {
				return nil, fmt.Errorf("该合并已撤销（幂等拒绝）")
			}
		}
	}
	if err := s.Store.UndoMergeSnapshot(kbID, meta.Keep, &store.MergeUndoView{
		Merge:     meta.Snapshot.Merge,
		Entity:    meta.Snapshot.Entity,
		FromAlias: meta.Snapshot.FromAlias,
		Relations: meta.Snapshot.Relations,
		ClaimIDs:  meta.Snapshot.Claims,
	}); err != nil {
		return nil, err
	}
	// 记录撤销态
	meta.Undone = true
	b, _ := json.Marshal(map[string]any{
		"kind": meta.Kind, "keep": meta.Keep, "merge": meta.Merge,
		"similarity": meta.Similarity, "undone": true, "snapshot": meta.Snapshot,
	})
	if _, err := s.Store.InsertDecision(&store.OntoDecision{
		SubjectKind: "kg", SubjectID: kbID,
		Title:       fmt.Sprintf("KB-19 撤销自动合并：%s ↩ %s", meta.Keep, meta.Merge),
		Rationale:   "快照重放恢复（尽力语义）",
		DerivedFrom: d.ID,
		MetaJSON:    string(b),
	}); err != nil {
		return nil, err
	}
	log.Printf("[kb] kb=%s 自动合并已撤销 %q ↩ %q（决策 %s）", kbID, meta.Keep, meta.Merge, d.ID)
	return d, nil
}
