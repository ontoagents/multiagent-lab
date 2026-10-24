package api

// REQ-411/M207：应用内一键备份——SQLite 三库（platform/ontology/runtime）经 VACUUM INTO
// 生成一致性快照（WAL 热备安全——VACUUM INTO 产出为独立紧凑文件，不锁写）+ 伴生图快照目录
// 拷贝，打包为 zip。恢复入口=16 号文档口径（覆盖 data/ 三库文件后重启），应用内不做危险
// 的自动覆盖恢复（单机产品误操作保护——诚实边界随档）。

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// backupSources 备份源清单（相对 DBPath 父目录=data/）。
type backupSource struct {
	rel    string // zip 内路径（data/ 前缀）
	absOrV bool   // true=VACUUM INTO 虚拟源（后缀 .db）；false=目录递归
}

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	dataDir := filepath.Dir(s.DBPath)
	ontologyDB := filepath.Join(dataDir, "ontology.db")
	runtimeDB := filepath.Join(dataDir, "runtime.db")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=backup-%s.zip", time.Now().UTC().Format("20060102-150405")))

	zw := zip.NewWriter(w)
	defer zw.Close()

	// ① 三库 VACUUM INTO 快照（WAL 热备安全）
	snap := func(srcDBPath, name string) {
		src, err := sql.Open("sqlite", srcDBPath)
		if err != nil {
			return
		}
		defer src.Close()
		tmp, err := os.CreateTemp("", "backup-*.db")
		if err != nil {
			return
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		if _, err := src.ExecContext(context.Background(), `VACUUM INTO ?`, tmp.Name()); err != nil {
			return
		}
		if f, err := os.Open(tmp.Name()); err == nil {
			if w, err := zw.Create("data/" + name); err == nil {
				_, _ = io.Copy(w, f)
			}
			f.Close()
		}
	}
	snap(s.DBPath, "platform.db")
	if _, err := os.Stat(ontologyDB); err == nil {
		snap(ontologyDB, "ontology.db")
	}
	if _, err := os.Stat(runtimeDB); err == nil {
		snap(runtimeDB, "runtime.db")
	}
	// 伴生图快照目录（REQ-377 产物——对话沉淀不可重建数据）
	compDir := filepath.Join(dataDir, "companion-graph-snapshots")
	if entries, err := os.ReadDir(compDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if f, err := os.Open(filepath.Join(compDir, e.Name())); err == nil {
				if zw2, err := zw.Create("data/companion-graph-snapshots/" + e.Name()); err == nil {
					_, _ = io.Copy(zw2, f)
				}
				f.Close()
			}
		}
	}
	// 智能体工作目录（agent-files——REQ-336 文件原语产物）
	agentFiles := filepath.Join(dataDir, "agent-files")
	if entries, err := os.ReadDir(agentFiles); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue // 子目录粒度=agent id 目录；文件平铺备份（大文件场景如实边界）
			}
			f, oerr := os.Open(filepath.Join(agentFiles, e.Name()))
			if oerr != nil {
				continue
			}
			if zw2, err := zw.Create("data/agent-files/" + e.Name()); err == nil {
				_, _ = io.Copy(zw2, f)
			}
			f.Close()
		}
	}
}
