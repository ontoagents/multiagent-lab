package kg

// KB-18/M290 单测：社区检索双臂融合——①向量命中词面未命中的社区可入榜（embedding 臂价值面）
// ②Embed nil 纯词面回退（历史口径）③惰性补嵌落缓存（缺失社区 embed 后写 kg_community_vec）。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// stubKGEmbed 桩 embedding：文本含「量子」→ 向量 A，否则 B（正交判定）。
func stubKGEmbed(_ context.Context, text string) ([]float32, error) {
	if containsAll(text, "量子", "纠缠") {
		return []float32{1, 0}, nil
	}
	return []float32{0, 1}, nil
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func newCommTestService(t *testing.T) (*Summarizer, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Summarizer{Store: st, Embed: stubKGEmbed}, st
}

func TestTopCommunitiesHybridVecArm(t *testing.T) {
	x, st := newCommTestService(t)
	comms := []*store.KGCommunity{
		{KBID: "kb1", Label: "经典力学", Summary: "牛顿力学与运动定律讨论", Members: []string{"牛顿"}},
		{KBID: "kb1", Label: "量子信息", Summary: "量子纠缠与量子计算基础", Members: []string{"贝尔"}},
	}
	// 查询与「量子信息」语义同域但词面零重叠（词面臂不可见、向量臂应命中）
	hits := x.topCommunitiesHybrid(context.Background(), "kb1", "quantum entanglement 量子纠缠现象", comms, 2)
	if len(hits) == 0 || hits[0].Label != "量子信息" {
		t.Fatalf("向量臂应把语义同域社区排首位: %+v", hits)
	}
	// 惰性补嵌落缓存
	vecs, dim, err := st.ListKGCommunityVecs("kb1")
	if err != nil || dim != 2 || len(vecs) != 2 {
		t.Fatalf("缓存应补嵌两社区: n=%d dim=%d err=%v", len(vecs), dim, err)
	}
}

func TestTopCommunitiesLexicalFallback(t *testing.T) {
	x, _ := newCommTestService(t)
	x.Embed = nil // 零 embedding 降级
	comms := []*store.KGCommunity{
		{KBID: "kb1", Label: "经典力学", Summary: "牛顿力学与运动定律", Members: []string{"牛顿"}},
	}
	hits := x.topCommunities(context.Background(), "kb1", "牛顿力学", comms, 2)
	if len(hits) != 1 || hits[0].Label != "经典力学" || hits[0].Score <= 0 {
		t.Fatalf("词面回退应命中且 Score=词面分: %+v", hits)
	}
}
