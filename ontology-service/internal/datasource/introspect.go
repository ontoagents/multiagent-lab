package datasource

// SQLite 只读内省（REQ-595 管线步①）：sqlite_master 用户表 + PRAGMA table_info/foreign_key_list。
// 只读强制=连接级 query_only(1)（沿 repo.go 同驱动的 _pragma 参数风格，规避 Windows 路径 URI 解析坑）。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 既有依赖（repo 同驱动），零新增
)

// ColDef 列定义（内省结果）。
type ColDef struct {
	Name     string
	DeclType string
	NotNull  bool
	IsPK     bool
}

// FKDef 声明外键（列级；组合外键每列一行）。
type FKDef struct {
	Column    string
	RefTable  string
	RefColumn string
}

// TableDef 表定义。
type TableDef struct {
	Name    string
	Columns []ColDef
	FKs     []FKDef
}

// Schema 内省结果（仅用户表；视图/sqlite_* 内部表排除）。
type Schema struct {
	Tables          []TableDef
	TruncatedTables int // 超 MaxTables 截掉的表数（调用方透出如实注记）
}

// openReadOnly 打开 SQLite（只读）。query_only(1) 连接级禁止写——「采样只读」诚实边界的实现位。
func openReadOnly(path string) (*sql.DB, error) {
	if st, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("文件不存在或不可访问（请确认是服务端本机路径）：%v", err)
	} else if st.IsDir() {
		return nil, fmt.Errorf("路径是目录而非 SQLite 数据库文件")
	}
	// busy_timeout 防「database is locked」瞬时冲突；query_only 只读强制。
	db, err := sql.Open("sqlite", path+"?_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// 探活 5s（坏文件/非 SQLite 文件在此暴露为人话错误）
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接探活失败（文件可能不是 SQLite 格式）：%v", err)
	}
	return db, nil
}

// quoteLit PRAGMA 参数用单引号字面量转义（表名可含引号/特殊字符）。
func quoteLit(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// introspect 内省全部用户表（sqlite_master→table_info→foreign_key_list）。
func introspect(ctx context.Context, db *sql.DB, maxTables int) (*Schema, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("读取表清单失败：%v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("读取表名失败：%v", err)
		}
		names = append(names, n)
	}
	allNames := names
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取表清单失败：%v", err)
	}
	truncated := false
	if maxTables > 0 && len(names) > maxTables {
		names = names[:maxTables]
		truncated = true
	}
	schema := &Schema{Tables: make([]TableDef, 0, len(names))}
	for _, name := range names {
		td := TableDef{Name: name}
		// 列：PRAGMA table_info('t') → cid,name,type,notnull,dflt_value,pk
		crows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteLit(name)+")")
		if err != nil {
			return nil, fmt.Errorf("读取表 %s 列定义失败：%v", name, err)
		}
		for crows.Next() {
			var cid int
			var cname, ctype string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := crows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
				crows.Close()
				return nil, fmt.Errorf("读取表 %s 列失败：%v", name, err)
			}
			td.Columns = append(td.Columns, ColDef{Name: cname, DeclType: strings.TrimSpace(ctype), NotNull: notnull == 1, IsPK: pk > 0})
		}
		cerr := crows.Err()
		crows.Close()
		if cerr != nil {
			return nil, fmt.Errorf("读取表 %s 列失败：%v", name, cerr)
		}
		// 声明外键：PRAGMA foreign_key_list('t') → id,seq,table,from,to（to 可空=引用主键）
		frows, err := db.QueryContext(ctx, "PRAGMA foreign_key_list("+quoteLit(name)+")")
		if err != nil {
			return nil, fmt.Errorf("读取表 %s 外键失败：%v", name, err)
		}
		for frows.Next() {
			var id, seq int
			var refTable, fromCol string
			var toCol sql.NullString
			var onUpdate, onDelete, onMatch sql.NullString
			if err := frows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &onMatch); err != nil {
				frows.Close()
				return nil, fmt.Errorf("读取表 %s 外键失败：%v", name, err)
			}
			td.FKs = append(td.FKs, FKDef{Column: fromCol, RefTable: refTable, RefColumn: toCol.String})
		}
		ferr := frows.Err()
		frows.Close()
		if ferr != nil {
			return nil, fmt.Errorf("读取表 %s 外键失败：%v", name, ferr)
		}
		schema.Tables = append(schema.Tables, td)
	}
	if truncated {
		schema.TruncatedTables = len(allNames) - len(names)
	}
	return schema, nil
}
