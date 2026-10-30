package kb

import (
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// KB-4②③/M287 实体向量读侧纯函数单测（余弦 TopN/BLOB 互转/维度诚实跳过）。
// RefreshEntityVectors 的 embedding 链路需真实连接，随 kbeval 向量臂先例离线不伪造
// （挂点接线由 store/kg_vec_test 与 graphrag 聚焦路径单测承载）。

func TestEntityVecTopN(t *testing.T) {
	mkVec := func(vals ...float32) []byte {
		b, err := encodeFloat32(vals)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	rows := []*store.KGEntityVec{
		{UID: "kb::A", Dim: 3, Vec: mkVec(1, 0, 0)},       // 与查询同向 → 1.0
		{UID: "kb::B", Dim: 3, Vec: mkVec(0, 1, 0)},       // 正交 → 0
		{UID: "kb::C", Dim: 3, Vec: mkVec(1, 1, 0)},       // ~0.707
		{UID: "kb::Bad", Dim: 3, Vec: []byte{1, 2}},       // 损坏行 → 跳过
		{UID: "kb::Dim4", Dim: 4, Vec: mkVec(1, 0, 0, 0)}, // 维度不匹配 → 跳过（KB-20 口径：不猜维度）
	}
	names := map[string]string{"kb::A": "实体A", "kb::B": "实体B", "kb::C": "实体C", "kb::Bad": "坏行", "kb::Dim4": "异维"}
	hits := entityVecTopN([]float32{1, 0, 0}, rows, names, 2)
	if len(hits) != 2 {
		t.Fatalf("topN=2 got %d hits: %+v", len(hits), hits)
	}
	if hits[0].Name != "实体A" || hits[0].Score < 0.999 {
		t.Fatalf("top hit mismatch: %+v", hits[0])
	}
	if hits[1].Name != "实体C" {
		t.Fatalf("second hit mismatch: %+v", hits[1])
	}
	// topN<=0 取全部可比行（损坏/异维行仍被诚实跳过）
	all := entityVecTopN([]float32{1, 0, 0}, rows, names, 0)
	if len(all) != 3 {
		t.Fatalf("all comparable rows = %d, want 3: %+v", len(all), all)
	}
	// 空库 → 空切片（读侧如实降级）
	if got := entityVecTopN([]float32{1, 0, 0}, nil, names, 5); len(got) != 0 {
		t.Fatalf("empty rows should give empty: %+v", got)
	}
}

func TestEntityVecCodec(t *testing.T) {
	orig := []float32{0.25, -1.5, 3.25}
	b, err := encodeFloat32(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decodeFloat32(b, 3)
	if !ok || got[0] != 0.25 || got[1] != -1.5 || got[2] != 3.25 {
		t.Fatalf("roundtrip broken: %+v ok=%v", got, ok)
	}
	// 字节数与 dim 不符 → 拒解（不猜维度）
	if _, ok := decodeFloat32(b, 4); ok {
		t.Fatal("dim mismatch should refuse")
	}
	// 空向量拒绝编码
	if _, err := encodeFloat32(nil); err == nil {
		t.Fatal("empty vector should error")
	}
}

func TestEntityVecText(t *testing.T) {
	if got := entityVecText("实体", ""); got != "实体" {
		t.Fatalf("name only: %q", got)
	}
	if got := entityVecText("实体", "描述"); got != "实体\n描述" {
		t.Fatalf("name+desc: %q", got)
	}
}
