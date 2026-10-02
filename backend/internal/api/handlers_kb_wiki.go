// REQ-241（M67）知识库 LLM Wiki 类型：页面列表 / 全量重建端点。
// 检索试运行复用 previewKBSearch（wiki 库经 SearchUnified 路由进 wiki 臂，mode=wiki）。
package api

import (
	"net/http"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// wikiPages GET /api/kb/{id}/wiki/pages：页面清单（含 content_md 全文，学习尺度直接列表回显）。
func (s *Server) wikiPages(w http.ResponseWriter, r *http.Request) {
	k, err := s.Store.GetKnowledgeBase(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if k.Mode != "wiki" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅 wiki 模式知识库提供页面视图"})
		return
	}
	pages, err := s.Store.ListWikiPages(k.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kb_id": k.ID, "pages": pages})
}

// wikiRebuild POST /api/kb/{id}/wiki/rebuild：手动全量重建（沿 REQ-130 重建先例）。
// body {conn_id?}（空 = 默认 chat 连接）；未变更文档（内容 hash）自动跳过生成。
func (s *Server) wikiRebuild(w http.ResponseWriter, r *http.Request) {
	k, err := s.Store.GetKnowledgeBase(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		ConnID string `json:"conn_id"`
	}
	_ = decodeJSON(r, &in) // 空体合法（沿 publish 空体先例）
	info, err := s.KB.WikiBuild(r.Context(), k.ID, in.ConnID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kb_id": k.ID, "result": info})
}

var _ = store.ErrNotFound
