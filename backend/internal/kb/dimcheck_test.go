package kb

// KB-20/M292 单测：维度漂移双端校验——①首索引登记 0→dim；②写入端维度不匹配拒绝
//（DimMismatchError 含重建指引）；③检索端不匹配报可懂错误；④匹配路径不受影响。
// 桩 Embedder 定维输出（真实 embedding 链路沿既有测试先例由真机承载）。

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// stubDimEmbedder 定维桩 embedding（dim 可配）。
type stubDimEmbedder struct{ dim int }

func (e stubDimEmbedder) EmbedOne(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, e.dim)
	for i := range v {
		v[i] = 0.1
	}
	return v, nil
}
func (e stubDimEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i], _ = e.EmbedOne(context.Background(), texts[i])
	}
	return out, nil
}

func newDimTestService(t *testing.T, dim int) (*Service, *store.KnowledgeBase) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "dim-kb", Mode: "rag", KBVector: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	embedOverride = stubDimEmbedder{dim: dim}
	t.Cleanup(func() { embedOverride = nil })
	return svc, kb
}

func TestDimRegisterAndWriteReject(t *testing.T) {
	svc, kb := newDimTestService(t, 4)
	// 首索引：dim=4 登记
	if _, err := svc.Import(context.Background(), kb.ID, "d1", "第一段测试内容。"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Store.GetKnowledgeBase(kb.ID)
	if got.EmbedDim != 4 {
		t.Fatalf("首索引应登记维度 4，实得 %d", got.EmbedDim)
	}
	// 同维度再写：不受影响
	if _, err := svc.Import(context.Background(), kb.ID, "d2", "第二段测试内容。"); err != nil {
		t.Fatal(err)
	}
	// 换维度模型写入：拒绝且错误含指引（Embed 字段换桩模拟模型切换）
	embedOverride = stubDimEmbedder{dim: 8}
	_, err := svc.Import(context.Background(), kb.ID, "d3", "第三段测试内容。")
	if err == nil {
		t.Fatal("维度漂移写入应被拒绝")
	}
	var dme DimMismatchError
	if !errors.As(err, &dme) {
		t.Fatalf("应返回 DimMismatchError，实得 %T: %v", err, err)
	}
	if dme.Want != 4 || dme.Have != 8 {
		t.Fatalf("错误维度不符: %+v", dme)
	}
	if !strings.Contains(err.Error(), "整库重建") {
		t.Fatalf("错误应含重建指引: %v", err)
	}
}

func TestDimSearchMismatch(t *testing.T) {
	svc, kb := newDimTestService(t, 4)
	if _, err := svc.Import(context.Background(), kb.ID, "d1", "检索目标内容。"); err != nil {
		t.Fatal(err)
	}
	// 检索端：模型切到 8 维——报可懂错误而非静默空结果
	embedOverride = stubDimEmbedder{dim: 8}
	_, err := svc.Search(context.Background(), kb, "检索目标", 4, 0)
	if err == nil {
		t.Fatal("维度漂移检索应报错而非静默丢召回")
	}
	var dme DimMismatchError
	if !errors.As(err, &dme) || dme.Want != 4 || dme.Have != 8 {
		t.Fatalf("检索端应返回 DimMismatchError(4→8): %v", err)
	}
	// 匹配路径不受影响
	embedOverride = stubDimEmbedder{dim: 4}
	hits, err := svc.Search(context.Background(), kb, "检索目标", 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("同维检索应有命中")
	}
}
