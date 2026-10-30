package migrate

// REQ-535/M342：pkg/migrate 单实现单测——事务回滚重放/legacy 收敛/版本记录
// （fstest.MapFS 注入——与 backend REQ-519 同构语义锁定）。

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunTxRollbackAndReplay(t *testing.T) {
	db := openTestDB(t)
	failing := fstest.MapFS{
		"migrations/001_ok.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE probe_a (id TEXT PRIMARY KEY);\nINSERT INTO probe_a VALUES ('a1');\n")},
		"migrations/002_bad.sql":    &fstest.MapFile{Data: []byte("CREATE TABLE probe_b (id TEXT PRIMARY KEY);\nINSERT INTO no_such_table VALUES ('x');\n")},
	}
	if err := Run(context.Background(), db, failing, Options{}); err == nil {
		t.Fatal("002 中途失败应返回错误")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM probe_a`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("001 应已提交: n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM probe_b`).Scan(&n); err == nil {
		t.Fatal("002 应已回滚（probe_b 不存在）")
	}
	fixed := fstest.MapFS{
		"migrations/001_ok.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS probe_a (id TEXT PRIMARY KEY);\n")},
		"migrations/002_bad.sql": &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS probe_b (id TEXT PRIMARY KEY);\nINSERT INTO probe_b VALUES ('b1');\n")},
	}
	if err := Run(context.Background(), db, fixed, Options{}); err != nil {
		t.Fatalf("修复版重放: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM probe_b`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("002 重放应生效: n=%d", n)
	}
}

func TestRunLegacyTolerance(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE probe_legacy (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	legacy := fstest.MapFS{
		"migrations/900_legacy.sql": &fstest.MapFile{Data: []byte("CREATE TABLE probe_legacy (id TEXT PRIMARY KEY);\n")},
	}
	if err := Run(context.Background(), db, legacy, Options{}); err != nil {
		t.Fatalf("already exists 应收敛: %v", err)
	}
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version='900_legacy.sql'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("legacy 应补记版本: %d %v", applied, err)
	}
}
