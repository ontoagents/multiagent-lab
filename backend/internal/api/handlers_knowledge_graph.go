package api

// REQ-372 P2：知识体系 L2 图谱层 REST 面——图谱查询/学习路径（prereq 闭包拓扑序）/摄取刷新。
// 卡为事实源单源、KG 为投影（每次 refresh 全量重建）；断链检测随摄取透出。

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/knowledge"
)

// knowledgeGraph GET /api/knowledge/graph：全量投影（节点+边；空库自动摄取一次）。
func (s *Server) knowledgeGraph(w http.ResponseWriter, r *http.Request) {
	nodes, edges, err := s.Store.ListKnowledgeGraph()
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(nodes) == 0 && s.KnowledgeRoot != "" {
		if _, rerr := knowledge.IngestKnowledgeGraph(knowledgeRootOf(s), "platform-knowledge", s.Store); rerr == nil {
			nodes, edges, err = s.Store.ListKnowledgeGraph()
			if err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
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
func (s *Server) knowledgeGraphRefresh(w http.ResponseWriter, r *http.Request) {
	if s.KnowledgeRoot == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "知识目录未配置（KNOWLEDGE_ROOT）"})
		return
	}
	res, err := knowledge.IngestKnowledgeGraph(knowledgeRootOf(s), "platform-knowledge", s.Store)
	if err != nil {
		writeErr(w, err)
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
