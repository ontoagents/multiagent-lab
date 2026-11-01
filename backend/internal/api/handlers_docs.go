// REQ-140 内部方案文档点击查看：只读服务白名单目录下的 Markdown
// （白名单目录 + .md 后缀，fsutil 防目录穿越）。前端弹层渲染（不要求在线编辑）。
// REQ-150 扩展（2026-09-25）：白名单由 docs/ 扩至 docs/ + research/。
// REQ-161 扩展（2026-09-25）：白名单扩至 docs/ + research/ + platform-knowledge/——
// 平台知识页（原参考资料中心）目录化后，文档互引相对路径点击查看覆盖平台知识目录。
// REQ-184 收尾（2026-10-22）：research/ 白名单分支退役（G-5 已全量并入 platform-knowledge，
// research/ 目录删除）——旧路径显式 400 诚实指路，不静默 404。
// REQ-570 四区收敛（2026-11-01）：platform-knowledge/ 分支退役（24 档迁 docs/handbook/、
// 调研档迁 docs/research/、09_知识体系 迁 docs/learning/，platform-knowledge/ 目录删除）——
// docs/ 前缀天然覆盖三子区零改动；旧路径显式 400 诚实指路，不静默 404。
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/fsutil"
)

// docRead GET /api/docs/read?path=docs/01_智能体_需求文档_PRD.md
// → {title, content}（纯文本 Markdown，前端渲染）。越界/非 md → 400。
func (s *Server) docRead(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path 必填（如 docs/01_xxx.md）"})
		return
	}
	if !strings.HasSuffix(rel, ".md") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅支持 .md 文档"})
		return
	}
	// 白名单目录：docs/（方案/需求事实源 + handbook/research/learning 三子区，前缀天然覆盖）；
	// fsutil 双重防越界。
	// REQ-184 收尾（2026-10-22）：research/ 分支退役——G-5（2026-09-27）曾并入 platform-knowledge。
	// REQ-570 四区收敛（2026-11-01）：platform-knowledge/ 分支退役——原树拆解至 docs/handbook|
	// research|learning；旧路径不静默 404，显式 400 指路新位置。
	clean := filepath.ToSlash(filepath.Clean(rel))
	const docsPrefix = "docs/"
	const researchPrefix = "research/"
	const knowledgePrefix = "platform-knowledge/"
	var root, sub string
	switch {
	case strings.HasPrefix(clean, researchPrefix):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "research/ 目录已退役（G-5 2026-09-27 曾并入 platform-knowledge/；REQ-570 2026-11-01 四区收敛后调研档居 docs/research/）——请改用 docs/research/ 下的对应文档"})
		return
	case strings.HasPrefix(clean, docsPrefix):
		root, sub = s.DocsRoot, strings.TrimPrefix(clean, docsPrefix)
	case strings.HasPrefix(clean, knowledgePrefix):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform-knowledge/ 目录已退役（REQ-570 2026-11-01 四区收敛：现状手册迁 docs/handbook/、调研档迁 docs/research/、知识体系迁 docs/learning/）——请改用 docs/ 下的对应文档"})
		return
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅支持 docs/ 目录下的文档（含 handbook/research/learning 三子区）"})
		return
	}
	if strings.Contains(clean, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	abs, err := fsutil.SafeJoin(root, sub)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	b, rerr := os.ReadFile(abs)
	if rerr != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "文档不存在或不可读: " + rerr.Error()})
		return
	}
	title := strings.TrimPrefix(filepath.Base(clean), "docs/")
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    clean,
		"title":   strings.TrimSuffix(title, ".md"),
		"content": string(b),
	})
}
