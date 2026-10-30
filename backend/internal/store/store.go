// Package store 提供 SQLite 存储层：打开连接、执行迁移与各实体 CRUD。
package store

import (
	"crypto/rand"
	"context"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ontoagents/ontoagents/pkg/migrate"
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

	// REQ-535/M342：迁移执行统一切 pkg/migrate 单实现（事务包裹/legacy 容错/版本记录单点
	// ——三服务仅数据源差异；行为由 REQ-519 fstest 两段式测试锁定）。
	return migrate.Run(context.Background(), s.DB, migrationsFS, migrate.Options{Dir: "migrations"})
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
