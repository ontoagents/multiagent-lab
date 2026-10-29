package store

import (
	"path/filepath"
	"testing"
)

// KB-4②③/M287 实体向量缓存单测：upsert/读回、指纹稳定性、孤儿清理三挂点
// （ReplaceKGForDoc 陈旧回收 / MergeKGEntities 合并回收 / deleteKG 级联回收）。

func TestKGEntityVecUpsertRead(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb1"
	if _, err := st.CreateKnowledgeBase(&KnowledgeBase{ID: kb, Name: "库", Mode: "graphrag"}); err != nil {
		t.Fatal(err)
	}
	ents := []*KGEntity{{ID: "e1", KBID: kb, Name: "实体A", Description: "描述A"}}
	rels := []*KGRelationship{}
	if err := st.ReplaceKGForDoc(kb, "d1", ents, rels, nil); err != nil {
		t.Fatal(err)
	}
	uid := KGEntityUID(kb, "实体A")
	if err := st.UpsertKGEntityVecs([]*KGEntityVec{{
		KBID: kb, UID: uid, Dim: 4, Vec: []byte{1, 2, 3, 4}, Fingerprint: "aa", UpdatedAt: "now",
	}}); err != nil {
		t.Fatal(err)
	}
	vecs, err := st.KGEntityVecs(kb)
	if err != nil || len(vecs) != 1 {
		t.Fatalf("vecs=%+v err=%v", vecs, err)
	}
	if vecs[0].UID != uid || vecs[0].Dim != 4 || string(vecs[0].Vec) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("vec row mismatch: %+v", vecs[0])
	}
	// 冲突覆盖（重算刷新）
	if err := st.UpsertKGEntityVecs([]*KGEntityVec{{
		KBID: kb, UID: uid, Dim: 4, Vec: []byte{9, 9, 9, 9}, Fingerprint: "bb", UpdatedAt: "now2",
	}}); err != nil {
		t.Fatal(err)
	}
	vecs, _ = st.KGEntityVecs(kb)
	if len(vecs) != 1 || vecs[0].Fingerprint != "bb" || vecs[0].Vec[0] != 9 {
		t.Fatalf("upsert overwrite failed: %+v", vecs[0])
	}
}

func TestKGEntityVecOrphanCleanupOnReplace(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb1"
	if _, err := st.CreateKnowledgeBase(&KnowledgeBase{ID: kb, Name: "库", Mode: "graphrag"}); err != nil {
		t.Fatal(err)
	}
	ents := []*KGEntity{{ID: "e1", KBID: kb, DocID: "d1", Name: "实体A"}, {ID: "e2", KBID: kb, DocID: "d1", Name: "实体B"}}
	if err := st.ReplaceKGForDoc(kb, "d1", ents, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"实体A", "实体B"} {
		if err := st.UpsertKGEntityVecs([]*KGEntityVec{{
			KBID: kb, UID: KGEntityUID(kb, n), Dim: 2, Vec: []byte{1, 1}, Fingerprint: "f", UpdatedAt: "now",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	// 重索引：实体B 不再产出 → 实体行与向量缓存一并回收；实体A 保留
	if err := st.ReplaceKGForDoc(kb, "d1", []*KGEntity{{ID: "e1", KBID: kb, DocID: "d1", Name: "实体A"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	vecs, err := st.KGEntityVecs(kb)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || vecs[0].UID != KGEntityUID(kb, "实体A") {
		t.Fatalf("orphan vec not recycled on replace: %+v", vecs)
	}
}

func TestKGEntityVecOrphanCleanupOnMergeAndDelete(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const kb = "kb1"
	if _, err := st.CreateKnowledgeBase(&KnowledgeBase{ID: kb, Name: "库", Mode: "graphrag"}); err != nil {
		t.Fatal(err)
	}
	ents := []*KGEntity{{ID: "e1", KBID: kb, Name: "keep"}, {ID: "e2", KBID: kb, Name: "gone"}}
	if err := st.ReplaceKGForDoc(kb, "", ents, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"keep", "gone"} {
		if err := st.UpsertKGEntityVecs([]*KGEntityVec{{
			KBID: kb, UID: KGEntityUID(kb, n), Dim: 2, Vec: []byte{1, 1}, Fingerprint: "f", UpdatedAt: "now",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	// 合并：gone 并入 keep → gone 实体行+向量缓存回收；keep 缓存保留（指纹未变无需重算）
	if _, _, err := st.MergeKGEntities(kb, "keep", []string{"gone"}); err != nil {
		t.Fatal(err)
	}
	vecs, _ := st.KGEntityVecs(kb)
	if len(vecs) != 1 || vecs[0].UID != KGEntityUID(kb, "keep") {
		t.Fatalf("merge orphan vec cleanup failed: %+v", vecs)
	}
	// KB 删除级联
	st.deleteKG(kb, "")
	if vecs, _ = st.KGEntityVecs(kb); len(vecs) != 0 {
		t.Fatalf("kb-level delete should recycle all vecs: %+v", vecs)
	}
}

func TestKGEntityVecFingerprint(t *testing.T) {
	a := KGEntityVecFingerprint("实体A", "描述A")
	b := KGEntityVecFingerprint("实体A", "描述A")
	c := KGEntityVecFingerprint("实体A", "描述B")
	d := KGEntityVecFingerprint("实体B", "描述A")
	if a != b || a == c || a == d || len(a) != 16 {
		t.Fatalf("fingerprint broken: a=%s b=%s c=%s d=%s", a, b, c, d)
	}
}
