package kb

// KB-19/M291 单测：高置信自动合并+撤销往返——①sim 达自动阈值且类型相同→自动合并+决策记录
//（快照含实体/关系/claim 清单）；②undo 后实体回归/关系恢复/别名摘除/claim subject 回迁；
// ③类型不同永不自动并（双保险）；④已撤销幂等拒绝。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// amEmbed 桩：「阿司匹林」与「Aspirin」语义近（余弦≈0.9985），其余正交（实现 embedClient 接口）。
type amEmbed struct{}

func (amEmbed) EmbedOne(_ context.Context, text string) ([]float32, error) {
	if strings.Contains(text, "阿司匹林") || strings.Contains(text, "Aspirin") {
		return []float32{1, 0.05}, nil
	}
	return []float32{0, 1}, nil
}

func (e amEmbed) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i], _ = e.EmbedOne(ctx, t)
	}
	return out, nil
}

func newAMService(t *testing.T) (*Service, *store.Store, string) {
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
	embedOverride = amEmbed{}
	t.Cleanup(func() { embedOverride = nil })
	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "am-kb", Mode: "graphrag", KBGraph: true})
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, kb.ID
}

func TestAutoMergeAndUndoRoundtrip(t *testing.T) {
	svc, st, kbID := newAMService(t)
	// 种子：近义对（阿司匹林/Aspirin，同类型 drug）+各带一条专属关系+一条 claim
	ents := []*store.KGEntity{
		{ID: store.NewID(), KBID: kbID, DocID: "d1", Name: "阿司匹林", Type: "drug", Description: "解热镇痛抗血小板"},
		{ID: store.NewID(), KBID: kbID, DocID: "d1", Name: "Aspirin", Type: "drug", Description: "乙酰水杨酸西文名"},
		{ID: store.NewID(), KBID: kbID, DocID: "d1", Name: "头痛", Type: "symptom"},
	}
	rels := []*store.KGRelationship{
		{ID: "r1", KBID: kbID, DocID: "d1", Source: "阿司匹林", Target: "头痛", Type: "缓解"},
		{ID: "r2", KBID: kbID, DocID: "d1", Source: "Aspirin", Target: "头痛", Type: "适应"},
	}
	claims := []*store.KGClaim{
		{ID: "cl1", KBID: kbID, DocID: "d1", ChunkID: "c1", Subject: "Aspirin", Text: "Aspirin 可缓解头痛"},
	}
	if err := st.ReplaceKGForDoc(kbID, "d1", ents, rels, claims); err != nil {
		t.Fatal(err)
	}
	// 自动合并（阈值默认 0.95——桩 0.9985 余弦达标）
	records, err := svc.AutoMergeHighConfidence(context.Background(), kbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("应自动合并一对，实得 %d", len(records))
	}
	// 合并后：Aspirin 实体消失（别名并入）、关系迁移
	after, _, _ := st.KGByKB(kbID)
	foundAspirin := false
	for _, e := range after {
		if e.Name == "Aspirin" {
			foundAspirin = true
		}
		if e.Name == "阿司匹林" && !strings.Contains(e.Alias, "Aspirin") {
			t.Fatalf("keep 别名应并入 merge 名: alias=%q", e.Alias)
		}
	}
	if foundAspirin {
		t.Fatal("merge 实体应已删除")
	}
	// 撤销
	if _, err := svc.UndoAutoMerge(context.Background(), kbID, records[0].ID); err != nil {
		t.Fatal(err)
	}
	restored, _, _ := st.KGByKB(kbID)
	back := false
	for _, e := range restored {
		if e.Name == "Aspirin" {
			back = true
			if e.Type != "drug" {
				t.Fatal("恢复实体类型应原样")
			}
		}
		if e.Name == "阿司匹林" && strings.Contains(e.Alias, "Aspirin") {
			t.Fatal("undo 后 keep 别名应摘除 merge 名")
		}
	}
	if !back {
		t.Fatal("undo 后 merge 实体应恢复")
	}
	// 关系 r2 恢复（source 回指 Aspirin）+ claim subject 恢复（KGByKB 列表扫）
	_, gotRels, _ := st.KGByKB(kbID)
	r2Source := ""
	for _, r := range gotRels {
		if r.ID == "r2" {
			r2Source = r.Source
		}
	}
	if r2Source != "Aspirin" {
		t.Fatalf("r2 source 应恢复 Aspirin，实得 %q", r2Source)
	}
	gotClaimList, err := st.KGClaimsForSubjects(kbID, []string{"Aspirin", "阿司匹林"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	sawCl1 := false
	for _, c := range gotClaimList {
		if c.ID == "cl1" {
			sawCl1 = true
			if c.Subject != "Aspirin" {
				t.Fatalf("claim subject 应恢复 Aspirin，实得 %s", c.Subject)
			}
		}
	}
	if !sawCl1 {
		t.Fatal("undo 后 cl1 应可经 Aspirin subject 检索到")
	}
	// 幂等拒绝
	if _, err := svc.UndoAutoMerge(context.Background(), kbID, records[0].ID); err == nil || !strings.Contains(err.Error(), "已撤销") {
		t.Fatalf("重复撤销应幂等拒绝: %v", err)
	}
}

func TestAutoMergeTypeMismatchNever(t *testing.T) {
	svc, _, kbID := newAMService(t)
	ents := []*store.KGEntity{
		{ID: store.NewID(), KBID: kbID, DocID: "d1", Name: "Python", Type: "language", Description: "编程语言"},
		{ID: store.NewID(), KBID: kbID, DocID: "d1", Name: "Python 蛇", Type: "animal", Description: "蟒蛇属"},
	}
	if err := svc.Store.ReplaceKGForDoc(kbID, "d1", ents, nil, nil); err != nil {
		t.Fatal(err)
	}
	records, err := svc.AutoMergeHighConfidence(context.Background(), kbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("类型不同（同名异义风险）永不自动并: %d", len(records))
	}
}
