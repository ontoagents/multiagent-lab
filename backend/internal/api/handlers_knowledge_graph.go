package api

// REQ-372 P2：知识体系 L2 图谱层 REST 面——图谱查询/学习路径（prereq 闭包拓扑序）/摄取刷新。
// 卡为事实源单源、KG 为投影（每次 refresh 全量重建）；断链检测随摄取透出。

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/knowledge"
)

// knowledgeGraph GET /api/knowledge/graph：全量投影（节点+边；空库自动摄取一次）。
// 修复轮（2026-10-27）：①断链随读随检（ScanBroken dry-run，不再依赖先 refresh）
// ②自动摄取失败不再静默——ingestError 字段透出（含解析后绝对路径，REQ-115 三要素文案），
// 前端据此渲染错误态（原形态下用户只见「节点 0」无解释）。
func (s *Server) knowledgeGraph(w http.ResponseWriter, r *http.Request) {
	root := knowledgeRootOf(s)
	nodes, edges, err := s.Store.ListKnowledgeGraph()
	if err != nil {
		writeErr(w, err)
		return
	}
	resp := map[string]any{"nodes": nodes, "edges": edges}
	if len(nodes) == 0 && s.KnowledgeRoot != "" {
		if _, rerr := knowledge.IngestKnowledgeGraph(root, "platform-knowledge", s.Store); rerr != nil {
			abs, _ := filepath.Abs(root)
			resp["ingestError"] = fmt.Sprintf(
				"图谱为空且自动摄取失败：知识目录 %s 不可达（可能原因：启动工作目录不含该相对路径，或 KNOWLEDGE_ROOT 配置错误）。请修正后点击「重新摄取」", abs)
		} else if nodes, edges, err = s.Store.ListKnowledgeGraph(); err != nil {
			writeErr(w, err)
			return
		} else {
			resp["nodes"], resp["edges"] = nodes, edges
		}
	}
	if s.KnowledgeRoot != "" {
		if broken, berr := knowledge.ScanBroken(root, "platform-knowledge"); berr == nil {
			resp["broken"] = broken
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// knowledgeGraphPath GET /api/knowledge/graph/path?target=<卡 id>：前置闭包拓扑序
// （学习路径自动编排——目标卡全部 prereq 传递闭包，依赖先行）。
func (s *Server) knowledgeGraphPath(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target 必填（卡 id=仓库相对路径）"})
		return
	}
	nodes, edges, err := s.Store.ListKnowledgeGraph()
	if err != nil {
		writeErr(w, err)
		return
	}
	path, err := knowledge.LearningPath(nodes, edges, target)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"target": target, "path": path, "count": len(path)})
}

// knowledgeGraphRefresh POST /api/knowledge/graph/refresh：重新摄取（全量重建投影）。
// 修复轮（2026-10-27）：错误文案三要素化（REQ-115）——原样透出 Go lstat 错误无下一步动作。
func (s *Server) knowledgeGraphRefresh(w http.ResponseWriter, r *http.Request) {
	root := knowledgeRootOf(s)
	if s.KnowledgeRoot == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "知识目录未配置（KNOWLEDGE_ROOT）。请设置环境变量后重启后端再试"})
		return
	}
	res, err := knowledge.IngestKnowledgeGraph(root, "platform-knowledge", s.Store)
	if err != nil {
		abs, _ := filepath.Abs(root)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("重新摄取失败：知识目录 %s 不可达（可能原因：启动工作目录不含该相对路径，或 KNOWLEDGE_ROOT 配置错误）。请修正启动目录/环境变量后重试", abs)})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// knowledgeRootOf 知识体系根目录（KnowledgeRoot 指向 platform-knowledge/；子目录 09_知识体系）。
func knowledgeRootOf(s *Server) string {
	base := strings.TrimRight(s.KnowledgeRoot, "/")
	if strings.HasSuffix(base, "09_知识体系") {
		return base
	}
	return filepath.ToSlash(filepath.Join(base, "09_知识体系"))
}
