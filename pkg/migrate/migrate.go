// Package migrate REQ-535/M342（P14②后半）：三服务迁移机制统一——单实现单点行为
// （事务包裹/legacy 容错/版本记录），三服务仅数据源差异（backend=go:embed FS，
// ontology-service/runtime-manager=os.ReadDir 目录）。
//
// 口径与 REQ-519/M326 各服务实现一致：
//   - 单文件迁移事务包裹（BEGIN…COMMIT）——中途失败全回滚、版本不记录、重启重放；
//   - legacy 容错=duplicate column name + already exists（存量库收敛：REQ-391/REQ-156/REQ-519）；
//   - 已记录版本跳过（applied map）；
//   - 迁移文件按文件名排序（数字前缀时序）。
//
// 消费方改造：backend runMigrations 换调本包（embed.FS 直传）；两卫星服务换调本包
// （os.DirFS(migrationsDir)）；行为以各服务既有迁移测试锁定（REQ-519 fstest 两段式）。
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// Options 迁移执行选项。
type Options struct {
	// Dir FS 内迁移目录（默认 "migrations"）。
	Dir string
	// Table 版本表名（默认 schema_migrations——三服务既有表名同源）。
	Table string
	// Now 版本记录时间戳函数（nil=time.Now UTC——三服务 now() 语义）。
	Now func() interface{}
}

func (o *Options) dir() string {
	if o.Dir == "" {
		return "migrations"
	}
	return o.Dir
}

func (o *Options) table() string {
	if o.Table == "" {
		return "schema_migrations"
	}
	return o.Table
}

func (o *Options) now() interface{} {
	if o.Now == nil {
		return timeNowUTC()
	}
	return o.Now()
}

// IsLegacyTolerable 存量库 legacy 已应用容忍（REQ-391/REQ-156 duplicate column 维持+
// REQ-519 already exists 扩展——存量半套 schema 重放收敛）。
func IsLegacyTolerable(err error) bool {
	e := err.Error()
	return strings.Contains(e, "duplicate column name") || strings.Contains(e, "already exists")
}

// EnsureTable 版本表就绪（IF NOT EXISTS——三服务既有建表语句同源）。
func EnsureTable(ctx context.Context, db *sql.DB, table string) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create %s: %w", table, err)
	}
	return nil
}

// Applied 已记录版本集合。
func Applied(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM `+table)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		applied[n] = true
	}
	return applied, rows.Err()
}

// Run 执行迁移：fsys 注入（embed.FS/os.DirFS/fstest.MapFS 三形态通吃）。
// 单文件事务包裹；失败全回滚返回错误（版本不记录，重启重放）。
func Run(ctx context.Context, db *sql.DB, fsys fs.FS, opts Options) error {
	table := opts.table()
	if err := EnsureTable(ctx, db, table); err != nil {
		return err
	}
	applied, err := Applied(ctx, db, table)
	if err != nil {
		return err
	}
	entries, err := fs.ReadDir(fsys, opts.dir())
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if applied[name] {
			continue
		}
		b, err := fs.ReadFile(fsys, opts.dir()+"/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(b)); err != nil {
			tx.Rollback()
			if IsLegacyTolerable(err) {
				if _, err := db.ExecContext(ctx,
					`INSERT OR IGNORE INTO `+table+` (version, applied_at) VALUES (?, ?)`, name, opts.now()); err != nil {
					return fmt.Errorf("record legacy migration %s: %w", name, err)
				}
				continue
			}
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO `+table+` (version, applied_at) VALUES (?, ?)`, name, opts.now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func timeNowUTC() interface{} {
	return time.Now().UTC()
}
