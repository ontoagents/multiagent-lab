package kb

// KB-8/M288 单测：①doc 删除受影响子图增量清理（graphrag 删除后实体/关系/claims 不残留）；
// ②EnqueueRebuild 异步任务全链（running→success 终态+结果 JSON+列表回读）；
// ③重启悬挂对账（running→failed 明示）。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestDeleteDocCleansKGSubgraph(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "del-kb", Mode: "graphrag", KBGraph: true})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kb.ID, Title: "待删文档", Status: "success"})
	if err != nil {
		t.Fatal(err)
	}
	ents := []*store.KGEntity{{ID: store.NewID(), KBID: kb.ID, DocID: doc.ID, Name: "残留实体", Type: "concept"}}
	rels := []*store.KGRelationship{{ID: "rd1", KBID: kb.ID, DocID: doc.ID, Source: "残留实体", Target: "另一实体", Type: "涉及"}}
	claims := []*store.KGClaim{{ID: "cd1", KBID: kb.ID, DocID: doc.ID, ChunkID: "c1", Subject: "残留实体", Text: "残留声明"}}
	if err := st.ReplaceKGForDoc(kb.ID, doc.ID, ents, rels, claims); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	if err := svc.DeleteDoc(context.Background(), kb.ID, doc.ID); err != nil {
		t.Fatal(err)
	}
	// 子图清理验证：实体/关系/claims 均不残留（graphrag 检索面不再命中已删文档）
	restEnts, restRels, _ := st.KGByKB(kb.ID)
	for _, e := range restEnts {
		if e.Name == "残留实体" {
			t.Fatal("已删文档实体不应残留")
		}
	}
	for _, r := range restRels {
		if r.ID == "rd1" {
			t.Fatal("已删文档关系不应残留")
		}
	}
	gotClaims, _ := st.KGClaimsForSubjects(kb.ID, []string{"残留实体"}, 10)
	if len(gotClaims) > 0 {
		t.Fatalf("已删文档 claims 不应残留: %d", len(gotClaims))
	}
}

func TestEnqueueRebuildLifecycle(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "rj-kb", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Box: box, Vector: SQLiteStore{ST: st}}
	job, err := svc.EnqueueRebuild(kb.ID, "kg", func(ctx context.Context) (map[string]any, error) {
		return map[string]any{"chunks": 7}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetKBRebuildJob(job.ID)
		if err == nil && got.Status != "running" {
			if got.Status != "success" {
				t.Fatalf("任务应 success: %s %q", got.Status, got.Error)
			}
			if !strings.Contains(got.ResultJSON, `"chunks":7`) {
				t.Fatalf("结果 JSON 应落库: %s", got.ResultJSON)
			}
			list, _ := st.ListKBRebuildJobs(kb.ID, 20)
			if len(list) != 1 {
				t.Fatalf("列表应含 1 条: %d", len(list))
			}
			// 失败路径
			job2, err := svc.EnqueueRebuild(kb.ID, "kg", func(ctx context.Context) (map[string]any, error) {
				return nil, context.DeadlineExceeded
			})
			if err != nil {
				t.Fatal(err)
			}
			for {
				got2, e := st.GetKBRebuildJob(job2.ID)
				if e == nil && got2.Status != "running" {
					if got2.Status != "failed" {
						t.Fatalf("失败任务应 failed: %s", got2.Status)
					}
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("任务未在 10s 内收束")
}

func TestMarkStaleRebuildJobs(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	job, err := st.CreateKBRebuildJob("kb_x", "wiki")
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.MarkStaleRebuildJobs()
	if err != nil || n != 1 {
		t.Fatalf("应悬挂对账 1 条: n=%d err=%v", n, err)
	}
	got, err := st.GetKBRebuildJob(job.ID)
	if err != nil || got.Status != "failed" || !strings.Contains(got.Error, "重新触发") {
		t.Fatalf("悬挂任务应 failed 明示: %+v %v", got, err)
	}
}
