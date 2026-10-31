package api

// REQ-548/M355 工作区领域容器 REST（Q-24 分层批① M1a；106 号 §五）。
// 五端点：GET/POST /api/workspaces、GET/PUT/DELETE /api/workspaces/{id}。
// 工作区仅承载创建时默认值（KB 库/本体方案/默认智能体）与对话归属，M1 无运行期语义。

import (
	"net/http"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := s.Store.ListWorkspaces()
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws == nil {
		ws = []*store.Workspace{}
	}
	writeJSON(w, http.StatusOK, ws)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var ws store.Workspace
	if err := decodeJSON(r, &ws); err != nil {
		writeErr(w, err)
		return
	}
	created, err := s.Store.CreateWorkspace(&ws)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := s.Store.GetWorkspace(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var ws store.Workspace
	if err := decodeJSON(r, &ws); err != nil {
		writeErr(w, err)
		return
	}
	ws.ID = r.PathValue("id")
	updated, err := s.Store.UpdateWorkspace(&ws)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteWorkspace(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}
