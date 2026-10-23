package repo

// REQ-381①②/M177：迁移版本表+异步 job 落库单测。
import (
	"path/filepath"
	"testing"
)

func TestSchemaMigrationsVersioned(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	st, err := Open(db, "../../migrations")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// 全部迁移应已记录（新库全应用）
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 13 { // 001~005 + 039~048（实测 15）
		t.Fatalf("应记录全部迁移版本，实得 %d", n)
	}
	// 二次打开：零重放（版本命中跳过——rename/drop 类迁移不再被重放误伤的语义前提）
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(db, "../../migrations")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	var n2 int
	_ = st2.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n2)
	if n2 != n {
		t.Fatalf("重开应零新增记录: %d != %d", n2, n)
	}
}

func TestAsyncJobPersistence(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.UpsertAsyncJob(&AsyncJobRow{ID: "j1", Kind: "draft", Status: "running", Progress: "第 1/3 轮"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 覆盖更新（终态）
	if err := st.UpsertAsyncJob(&AsyncJobRow{ID: "j1", Kind: "draft", Status: "done", ResultJSON: `{"id":"j1","status":"done"}`}); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	row, err := st.GetAsyncJob("j1")
	if err != nil || row == nil || row.Status != "done" {
		t.Fatalf("get: %v %+v", err, row)
	}
	// 无行 nil
	if r2, _ := st.GetAsyncJob("j-none"); r2 != nil {
		t.Fatalf("无行应 nil: %+v", r2)
	}
	// 孤儿对账：running → error
	_ = st.UpsertAsyncJob(&AsyncJobRow{ID: "j2", Status: "running"})
	n, err := st.MarkOrphanAsyncJobs()
	if err != nil || n != 1 {
		t.Fatalf("orphan sweep: %v %d", err, n)
	}
	row2, _ := st.GetAsyncJob("j2")
	if row2.Status != "error" || row2.Error == "" {
		t.Fatalf("孤儿应标 error: %+v", row2)
	}
	// done 行不受对账影响
	row3, _ := st.GetAsyncJob("j1")
	if row3.Status != "done" {
		t.Fatalf("done 行不应被动: %+v", row3)
	}
}
