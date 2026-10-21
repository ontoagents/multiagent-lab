// REQ-358/M158 随批小件（93 号 §7.2 Reject 归因留档）：reject_reason 落库与回读。
package store

import (
	"path/filepath"
	"testing"
)

func TestCompanionCandidateRejectReason(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 最小夹具：直插候选行（SaveCompanionCandidates 需消息存在，这里走 SQL 最短路径）
	if _, err := st.DB.Exec(`INSERT INTO companion_candidate (id,conversation_id,agent_id,kind,name,confidence,status,created_at) VALUES ('c1','cv','ag','concept','测试实体',0.9,'pending',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	got, err := st.RejectCompanionCandidateWithReason("c1", "gate:benchmark_recall@5=0.42 < τ=0.60")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "rejected" || got.RejectReason == "" {
		t.Fatalf("应 rejected 且带归因，got %s/%q", got.Status, got.RejectReason)
	}
	// 幂等边界：已裁决不可再动
	if _, err := st.RejectCompanionCandidateWithReason("c1", "again"); err == nil {
		t.Fatal("非 pending 应 ErrNotFound")
	}
}
