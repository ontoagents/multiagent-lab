package api

// REQ-411/M207：应用内一键备份——SQLite 三库（platform/ontology/runtime）经 VACUUM INTO
// 生成一致性快照（WAL 热备安全——VACUUM INTO 产出为独立紧凑文件，不锁写）+ 数据目录资产
// 打包为 zip。恢复入口=16 号「应用内备份恢复三步」（REQ-517/M324 补节：覆盖 data/ 后重启），
// 应用内不做危险的自动覆盖恢复（单机产品误操作保护——诚实边界随档）。
//
// REQ-517/M324 完整性闭环（评审 P04，双维复核 confirmed）：
//   - 覆盖面扩四项——data/projects（REQ-113 项目文件+REQ-101 save_file 产物+.git，唯一不可
//     从库重建的用户产出）/data/.secret（连接 Key 加密密钥——16 号 §9 自明换机必带）/data/
//     companion-snapshots（REQ-216 对话伴生回灌信源，原备份拷的 companion-graph-snapshots
//     在部分部署形态不存在即静默空过）/agent-files 含子目录（原平铺跳过 agent id 子目录）；
//   - 失败聚合——单源失败不再静默吞掉：skipped 注入 zip 根 manifest.json（区分 not_found 与
//     read/vacuum/zip 错误），响应头 X-Backup-Warnings 携带跳过数（前端备份钮 toast 提示）；
//   - 主库快照失败=HTTP 500（响应头先写的静默 200 缺库 zip 形态根除——磁盘满/临时目录不可写/
//     源库损坏时用户拿到缺库 zip 毫无察觉的缺陷排除）；卫星库（ontology/runtime.db）不在本
//     服务可达范围时如实进 skipped（Docker 三卷形态由 16 号恢复节指引手动补备）；
//   - 实现=先落临时 zip 再回放响应（失败聚合必须在写响应头前完成）。

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// backupCollector zip 收集器：文件/目录递归入包，失败聚合 skipped（REQ-517 失败不静默）。
type backupCollector struct {
	zw      *zip.Writer
	skipped []map[string]string
}

func (bc *backupCollector) skip(rel, reason string) {
	bc.skipped = append(bc.skipped, map[string]string{"path": "data/" + rel, "reason": reason})
}

func (bc *backupCollector) addFile(rel, abs string) {
	f, err := os.Open(abs)
	if err != nil {
		bc.skip(rel, "read_error: "+err.Error())
		return
	}
	defer f.Close()
	zw2, err := bc.zw.Create("data/" + filepath.ToSlash(rel))
	if err != nil {
		bc.skip(rel, "zip_error: "+err.Error())
		return
	}
	if _, err := io.Copy(zw2, f); err != nil {
		bc.skip(rel, "copy_error: "+err.Error())
	}
}

// addDir 递归打包目录（含子目录与隐藏目录——projects/.git、agent-files/agent-id/ 修复原跳过）。
// 源不存在=静默跳过（部署形态差异如实面，不制造告警噪音）；其余读取失败记 skipped。
func (bc *backupCollector) addDir(rel, abs string) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			bc.skip(rel, "read_error: "+err.Error())
		}
		return
	}
	for _, e := range entries {
		erel, eabs := filepath.Join(rel, e.Name()), filepath.Join(abs, e.Name())
		if e.IsDir() {
			bc.addDir(erel, eabs)
		} else {
			bc.addFile(erel, eabs)
		}
	}
}

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	dataDir := filepath.Dir(s.DBPath)
	tmpZip, err := os.CreateTemp("", "backup-*.zip")
	if err != nil {
		http.Error(w, "备份失败：临时文件创建错误 "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpName := tmpZip.Name()
	defer os.Remove(tmpName)

	bc := &backupCollector{zw: zip.NewWriter(tmpZip)}

	// ① 三库 VACUUM INTO 快照（WAL 热备安全）；主库失败=整体失败（500），卫星库缺失/失败如实入 manifest
	platformOK := s.snapDB(bc, s.DBPath, "platform.db")
	if _, err := os.Stat(filepath.Join(dataDir, "ontology.db")); err == nil {
		s.snapDB(bc, filepath.Join(dataDir, "ontology.db"), "ontology.db")
	} else {
		bc.skip("ontology.db", "not_found（卫星库不在本服务数据目录——Docker 三卷形态按 16 号恢复节手动补备）")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "runtime.db")); err == nil {
		s.snapDB(bc, filepath.Join(dataDir, "runtime.db"), "runtime.db")
	} else {
		bc.skip("runtime.db", "not_found（卫星库不在本服务数据目录——Docker 三卷形态按 16 号恢复节手动补备）")
	}

	// ② 数据目录资产（REQ-517 扩四项：companion-snapshots/projects/.secret + agent-files 含子目录）
	bc.addDir("companion-graph-snapshots", filepath.Join(dataDir, "companion-graph-snapshots"))
	bc.addDir("companion-snapshots", filepath.Join(dataDir, "companion-snapshots"))
	bc.addDir("projects", filepath.Join(dataDir, "projects"))
	bc.addDir("agent-files", filepath.Join(dataDir, "agent-files"))
	secretAbs := filepath.Join(dataDir, ".secret")
	if stInfo, serr := os.Stat(secretAbs); serr == nil {
		if stInfo.IsDir() {
			bc.addDir(".secret", secretAbs)
		} else {
			bc.addFile(".secret", secretAbs)
		}
	} else {
		bc.skip(".secret", "not_found（未生成加密密钥——无加密存储的连接时属正常）")
	}

	// ③ manifest.json（zip 根）：跳过清单+生成时间——部分成功对用户可见
	if mf, merr := json.Marshal(map[string]any{
		"generated": time.Now().UTC().Format(time.RFC3339),
		"skipped":   bc.skipped,
	}); merr == nil {
		if zw2, zerr := bc.zw.Create("manifest.json"); zerr == nil {
			_, _ = zw2.Write(mf)
		}
	}
	if err := bc.zw.Close(); err != nil {
		http.Error(w, "备份失败：zip 收尾错误 "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpZip.Close(); err != nil {
		http.Error(w, "备份失败：临时文件收尾错误 "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !platformOK {
		reason := "unknown"
		for _, sk := range bc.skipped {
			if sk["path"] == "data/platform.db" {
				reason = sk["reason"]
				break
			}
		}
		http.Error(w, "备份失败：主库（platform.db）快照错误 "+reason, http.StatusInternalServerError)
		return
	}

	// ④ 回放临时 zip（响应头此时才写——X-Backup-Warnings 可携带真实跳过数）
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=backup-%s.zip", time.Now().UTC().Format("20060102-150405")))
	if len(bc.skipped) > 0 {
		w.Header().Set("X-Backup-Warnings", strconv.Itoa(len(bc.skipped)))
	}
	w.WriteHeader(http.StatusOK)
	if f, ferr := os.Open(tmpName); ferr == nil {
		defer f.Close()
		_, _ = io.Copy(w, f)
	}
}

// snapDB VACUUM INTO 一致性快照入 zip；失败记 skipped 并返回 false（主库失败=整体 500）。
func (s *Server) snapDB(bc *backupCollector, srcDBPath, name string) bool {
	src, err := sql.Open("sqlite", srcDBPath)
	if err != nil {
		bc.skip(name, "open_error: "+err.Error())
		return false
	}
	defer src.Close()
	tmp, err := os.CreateTemp("", "backup-*.db")
	if err != nil {
		bc.skip(name, "temp_error: "+err.Error())
		return false
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err := src.ExecContext(context.Background(), `VACUUM INTO ?`, tmp.Name()); err != nil {
		bc.skip(name, "vacuum_error: "+err.Error())
		return false
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		bc.skip(name, "read_error: "+err.Error())
		return false
	}
	defer f.Close()
	zw2, err := bc.zw.Create("data/" + name)
	if err != nil {
		bc.skip(name, "zip_error: "+err.Error())
		return false
	}
	if _, err := io.Copy(zw2, f); err != nil {
		bc.skip(name, "copy_error: "+err.Error())
		return false
	}
	return true
}
