package store

// REQ-287/M92 伴生 FTS 投影单测：回填→BM25 命中（含「定义含查询词而标签不含」的隐式召回
// 用例——32 号报告口径的单元级承载）+ 失效重建（Replace 幂等）。

import (
	"path/filepath"
	"testing"
)

func newFTSTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "fts_test.db"))
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCompanionFTSRoundtrip(t *testing.T) {
	st := newFTSTestStore(t)
	docs := []CompanionFTSDoc{
		{Label: "水平伸缩", Content: "水平伸缩\n根据负载自动调整副本数量的机制"},
		{Label: "HPA", Content: "HPA\nHorizontal Pod Autoscaler，容器编排平台的自动扩缩容组件"},
	}
	if err := st.ReplaceCompanionFTS("ont_x", docs); err != nil {
		t.Fatalf("回填失败: %v", err)
	}
	if n, _ := st.CompanionFTSCount("ont_x"); n != 2 {
		t.Fatalf("投影行数应=2，got %d", n)
	}
	// 隐式召回用例：查询词只出现在定义（容器编排），标签均不含
	hits, err := st.SearchCompanionFTS("ont_x", FTSMatchQuery("容器编排平台"), 5)
	if err != nil {
		t.Fatalf("FTS 检索失败: %v", err)
	}
	found := false
	for _, h := range hits {
		if h.Label == "HPA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("定义含查询词的实体应被 BM25 命中: %v", hits)
	}
	// 标签精确命中
	hits2, _ := st.SearchCompanionFTS("ont_x", FTSMatchQuery("水平伸缩"), 5)
	if len(hits2) == 0 || hits2[0].Label != "水平伸缩" {
		t.Fatalf("标签应首命中: %v", hits2)
	}
	// 本体隔离
	hits3, _ := st.SearchCompanionFTS("ont_other", FTSMatchQuery("水平伸缩"), 5)
	if len(hits3) != 0 {
		t.Fatalf("跨本体不应命中: %v", hits3)
	}
	// 失效重建（Replace 幂等覆盖）
	if err := st.ReplaceCompanionFTS("ont_x", docs[:1]); err != nil {
		t.Fatalf("重建失败: %v", err)
	}
	if n, _ := st.CompanionFTSCount("ont_x"); n != 1 {
		t.Fatalf("重建后行数应=1，got %d", n)
	}
}
