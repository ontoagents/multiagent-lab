package store

// REQ-519/M326（P14①）：单文件迁移事务包裹——中途失败全回滚、版本不记录、重启重放；
// already exists 存量收敛。fstest.MapFS 两段式注入（runMigrations fsys 参数化承载）。

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/ontoagents/ontoagents/pkg/migrate"
)

// 两段式：m002 中途失败（第二语句 INSERT 不存在的表）→ 断言 001 已提交、002 未记录且 m002
// 第一语句的建表已回滚 → 换修复版 m002 重放成功。
func TestMigrateTxRollbackAndReplay(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	st.Close() // 真库先建好（真实迁移全部应用）

	failing := fstest.MapFS{
		"migrations/001_ok.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE tx_probe_a (id TEXT PRIMARY KEY);\nINSERT INTO tx_probe_a VALUES ('a1');\n")},
		"migrations/002_broken.sql": &fstest.MapFile{Data: []byte("CREATE TABLE tx_probe_b (id TEXT PRIMARY KEY);\nINSERT INTO no_such_table_tx VALUES ('x');\n")},
	}
	st2, err := Open(dir + "/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if err := migrate.Run(context.Background(), st2.DB, failing, migrate.Options{}); err == nil {
		t.Fatal("m002 中途失败应返回错误")
	}

	// 001 已提交（逐文件事务边界）
	var n int
	if err := st2.DB.QueryRow(`SELECT COUNT(*) FROM tx_probe_a`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("001 应已提交（tx_probe_a 1 行）: n=%d err=%v", n, err)
	}
	// 002 第一语句已回滚（表不存在）
	if err := st2.DB.QueryRow(`SELECT COUNT(*) FROM tx_probe_b`).Scan(&n); err == nil {
		t.Fatal("002 中途失败应回滚（tx_probe_b 不应存在）")
	}
	// 002 版本未记录（重启可重放）
	var applied int
	if err := st2.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version='002_broken.sql'`).Scan(&applied); err != nil || applied != 0 {
		t.Fatalf("002 版本不应记录: applied=%d err=%v", applied, err)
	}

	// 修复版 m002 重放成功
	fixed := fstest.MapFS{
		"migrations/001_ok.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS tx_probe_a (id TEXT PRIMARY KEY);\n")},
		"migrations/002_broken.sql": &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS tx_probe_b (id TEXT PRIMARY KEY);\nINSERT INTO tx_probe_b VALUES ('b1');\n")},
	}
	if err := migrate.Run(context.Background(), st2.DB, fixed, migrate.Options{}); err != nil {
		t.Fatalf("修复版重放应成功: %v", err)
	}
	if err := st2.DB.QueryRow(`SELECT COUNT(*) FROM tx_probe_b`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("修复版 m002 应生效: n=%d err=%v", n, err)
	}
	if err := st2.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version='002_broken.sql'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("重放成功后 002 应记录: applied=%d", applied)
	}
}

// TestMigrateAlreadyExistsTolerance REQ-519：存量半套 schema（表已存在版本未记录）重放收敛。
func TestMigrateAlreadyExistsTolerance(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// 模拟存量库：表已在（历史半套应用）但版本未记录
	if _, err := st.DB.Exec(`CREATE TABLE tx_probe_legacy (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	legacy := fstest.MapFS{
		"migrations/900_legacy.sql": &fstest.MapFile{Data: []byte("CREATE TABLE tx_probe_legacy (id TEXT PRIMARY KEY);\n")},
	}
	if err := migrate.Run(context.Background(), st.DB, legacy, migrate.Options{}); err != nil {
		t.Fatalf("already exists 应收敛为 legacy 已应用: %v", err)
	}
	var applied int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version='900_legacy.sql'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("legacy 收敛后应补记版本: applied=%d err=%v", applied, err)
	}
}
