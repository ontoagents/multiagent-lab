package store

// KB-16/M286 实体稳定 id 与 chunk-entity 倒排单测：
//   - 重建/重索引前后实体 uid 与行 id 稳定（upsert 不换 id）；
//   - 倒排表与 claim 反查两路结果一致；
//   - KB 级重建陈旧实体清理（uid 不在新集）与别名保留并存；
//   - 跨 doc 同名实体 uid 归一（同 uid 单行）。

import (
	"path/filepath"
	"testing"
)

func uidFixture() ([]*KGEntity, []*KGRelationship, []*KGClaim) {
	return []*KGEntity{
			{ID: NewID(), KBID: "kb-uid", DocID: "d1", Name: "工作流引擎", Type: "concept", Description: "调度执行"},
			{ID: NewID(), KBID: "kb-uid", DocID: "d1", Name: "大模型", Type: "concept"},
		},
		[]*KGRelationship{
			{ID: NewID(), KBID: "kb-uid", DocID: "d1", Source: "工作流引擎", Target: "大模型", Type: "依赖"},
		},
		[]*KGClaim{
			{ID: NewID(), KBID: "kb-uid", DocID: "d1", ChunkID: "c1", Subject: "工作流引擎", Text: "工作流引擎负责调度。"},
			{ID: NewID(), KBID: "kb-uid", DocID: "d1", ChunkID: "c2", Subject: "大模型", Text: "大模型被引擎调用。"},
		}
}

func TestReplaceKGForDoc_UIDStableAcrossRebuild(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb-uid"
	ents, rels, claims := uidFixture()
	if err := st.ReplaceKGForDoc(kb, "", ents, rels, claims); err != nil {
		t.Fatal(err)
	}
	first, _, err := st.KGByKB(kb)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("期望 2 实体，实际 %d", len(first))
	}
	uidFirst := map[string]string{}
	idFirst := map[string]string{}
	for _, e := range first {
		want := KGEntityUID(kb, e.Name)
		if e.UID != want {
			t.Fatalf("uid 应为 %q，实际 %q", want, e.UID)
		}
		uidFirst[e.Name] = e.UID
		idFirst[e.Name] = e.ID
	}
	// KB 级重建：新随机行 id 输入 → uid/行 id 均不变（upsert 不换 id）
	ents2, rels2, claims2 := uidFixture()
	if err := st.ReplaceKGForDoc(kb, "", ents2, rels2, claims2); err != nil {
		t.Fatal(err)
	}
	second, _, err := st.KGByKB(kb)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 {
		t.Fatalf("重建后应仍 2 实体，实际 %d", len(second))
	}
	for _, e := range second {
		if e.UID != uidFirst[e.Name] || e.ID != idFirst[e.Name] {
			t.Fatalf("重建后身份应稳定: %s uid %s→%s id %s→%s", e.Name, uidFirst[e.Name], e.UID, idFirst[e.Name], e.ID)
		}
	}
}

func TestKGChunkEntityParityWithClaims(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb-uid"
	ents, rels, claims := uidFixture()
	if err := st.ReplaceKGForDoc(kb, "", ents, rels, claims); err != nil {
		t.Fatal(err)
	}
	// 两路一致：倒排表名称集 == claim 反查名称集（逐 chunk 对照）
	for _, chunk := range []string{"c1", "c2"} {
		viaInverted, err := st.KGChunkSubjectsByChunks(kb, []string{chunk})
		if err != nil {
			t.Fatal(err)
		}
		viaClaims, err := st.KGSubjectsByChunks(kb, []string{chunk})
		if err != nil {
			t.Fatal(err)
		}
		if len(viaInverted) != 1 || len(viaClaims) != 1 || viaInverted[0] != viaClaims[0] {
			t.Fatalf("chunk %s 两路不一致: inverted=%v claims=%v", chunk, viaInverted, viaClaims)
		}
	}
	refs, err := st.KGEntitiesByChunks(kb, []string{"c1"})
	if err != nil || len(refs) != 1 {
		t.Fatalf("倒行投影: %v %d", err, len(refs))
	}
	if refs[0].UID != KGEntityUID(kb, "工作流引擎") || refs[0].Name != "工作流引擎" {
		t.Fatalf("倒行应携 uid+name: %+v", refs[0])
	}
}

func TestReplaceKGForDoc_StaleEntityCleanup(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb-uid"
	ents, rels, claims := uidFixture()
	if err := st.ReplaceKGForDoc(kb, "", ents, rels, claims); err != nil {
		t.Fatal(err)
	}
	// KB 级重建只产出其中一个实体：陈旧实体（大模型）被清理，存留者 uid 稳定
	keep := []*KGEntity{{ID: NewID(), KBID: kb, DocID: "d1", Name: "工作流引擎", Type: "concept", Description: "v2"}}
	if err := st.ReplaceKGForDoc(kb, "", keep, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _, err := st.KGByKB(kb)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "工作流引擎" {
		t.Fatalf("陈旧实体应清理: %d 实体", len(got))
	}
}

func TestReplaceKGForDoc_CrossDocSameNameUnified(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb-uid"
	ents, rels, _ := uidFixture()
	if err := st.ReplaceKGForDoc(kb, "d1", ents, rels, nil); err != nil {
		t.Fatal(err)
	}
	// doc2 重索引同名实体：同 uid 单行（doc_id 随最新抽取），不再两行
	ents2 := []*KGEntity{{ID: NewID(), KBID: kb, DocID: "d2", Name: "工作流引擎", Type: "concept", Description: "doc2 版"}}
	if err := st.ReplaceKGForDoc(kb, "d2", ents2, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _, err := st.KGByKB(kb)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range got {
		if e.Name == "工作流引擎" {
			count++
			if e.DocID != "d2" {
				t.Fatalf("同名实体 doc_id 应随最新抽取: %s", e.DocID)
			}
		}
	}
	if count != 1 {
		t.Fatalf("跨 doc 同名应归一单行，实际 %d 行", count)
	}
}
