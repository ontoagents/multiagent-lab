// Package store 提供 SQLite 存储层：打开连接、执行迁移与各实体 CRUD。
package store

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store 封装主平台 SQLite。
type Store struct {
	DB *sql.DB
	// KB-10①：FTS5 词法索引可用性（惰性探测一次；false = 环境不支持，词法臂退役）
	ftsOnce sync.Once
	ftsOK   bool
	// REQ-528/M335：会话内容检索 FTS5（msg_fts.go——惰性建表+回填+写穿，chunk_fts 同族）
	msgFtsOnce sync.Once
	msgFtsOK   bool
}

// Open 打开（必要时创建）SQLite 数据库并执行迁移。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// 学习尺度：单写连接即可，避免 SQLITE_BUSY
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// REQ-148：老连接按 (protocol, base_url) 幂等回填供应商分组（存量行为不变，新建连接可拆多实例）
	if err := s.BackfillProviderGroups(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("backfill provider groups: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	// REQ-391/M187：迁移版本表统一（复用 REQ-381 ontology-service/runtime-manager 已交付模式）
	// ——schema_migrations(version 主键) 已记录版本跳过；重复执行遇 duplicate column 视为
	// legacy 已应用并补记（058 迁移已把旧 schema_migration 单数表记录全量转入+编号重复治理映射，
	// 存量库零重放；此后 rename/drop 类迁移不再被重放误伤）。
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied := map[string]bool{}
	rows, err := s.DB.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		applied[n] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	return s.runMigrations(migrationsFS, "migrations", applied)
}

// runMigrations REQ-519/M326：单文件迁移事务包裹（sql.Tx BEGIN…COMMIT）——SQLite 支持事务 DDL，
// 中途失败全回滚、版本不记录，重启自动重放；防「部分 DDL 已生效但版本未记录→重放撞 already
// exists→store.Open 失败服务卡死」（单用户库即全部资产且无 DBA 自修手段）。fsys 注入=中途
// 失败回滚+修复重放可测（fstest.MapFS 两段式）。tolerance=存量库 legacy 已应用收敛（REQ-391
// duplicate column 语义维持+REQ-519 扩 already exists——存量半套 schema 重放收敛）。
func (s *Store) runMigrations(fsys fs.FS, dir string, applied map[string]bool) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
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
		b, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(b)); err != nil {
			tx.Rollback()
			if isLegacyTolerable(err) {
				// legacy 已应用收敛：补记版本（本文件后续语句未执行——存量收敛语义与迁移前一致）
				if _, err := s.DB.Exec(`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)`, name, now()); err != nil {
					return fmt.Errorf("record legacy migration %s: %w", name, err)
				}
				continue
			}
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		// REQ-519/M326 顺修存量缺陷：原 version-only INSERT 因 applied_at NOT NULL 无默认
		// 被 OR IGNORE 静默吞掉——049+ 迁移从未被记录、每次启动全量重放（dev 库实证：52 条
		// 记录止于 048，058/069/070/071/072 均未记录，容错掩盖）。补 applied_at 后「已记录
		// 跳过」机制首次真正生效（存量库下次启动一次性收敛补记，此后零重放）。
		if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)`, name, now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// isLegacyTolerable 存量库 legacy 已应用容忍（REQ-391 duplicate column 维持；REQ-519 扩
// already exists——CREATE TABLE 无 IF NOT EXISTS 的存量库重放收敛）。
func isLegacyTolerable(err error) bool {
	e := err.Error()
	return strings.Contains(e, "duplicate column name") || strings.Contains(e, "already exists")
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.DB.Close() }

// NewID 生成 32 位十六进制随机 ID（本地学习尺度，无需外键可读性）。
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// HTTPError 带 HTTP 状态码的错误，用于 handler 返回。
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string { return e.Msg }

// ErrNotFound 通用 404。
var ErrNotFound = &HTTPError{Status: http.StatusNotFound, Msg: "not found"}

// ErrConflict 通用 409（如唯一名冲突）。
var ErrConflict = &HTTPError{Status: http.StatusConflict, Msg: "conflict"}

func now() time.Time { return time.Now().UTC() }
