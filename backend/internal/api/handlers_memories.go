package api

// REQ-551/M358 工作区记忆 REST（Q-24 分层批④收官）：候选确认 UI 数据面。
// 四端点：GET /api/workspaces/{id}/memories（?status= 三桶过滤）/POST 同路径（手工录入——
// 显式确认文本线，落 accepted）/POST /api/memories/{id}/decide（candidate→accepted|rejected）/
// DELETE /api/memories/{id}。

import (
	"net/http"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	ms, err := s.Store.ListMemories(r.PathValue("id"), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if ms == nil {
		ms = []*store.Memory{}
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *Server) createMemory(w http.ResponseWriter, r *http.Request) {
	var m store.Memory
	if err := decodeJSON(r, &m); err != nil {
		writeErr(w, err)
		return
	}
	m.WorkspaceID = r.PathValue("id")
	// 手工录入=用户显式确认（REQ-495 拍板「记忆=显式确认文本」语义），直接落 accepted；
	// 抽取链产生的候选恒走 status=candidate（memory service 落库路径），本端点不承载。
	if m.Status != "accepted" && m.Status != "candidate" {
		m.Status = "accepted"
	}
	created, _, err := s.Store.CreateMemory(&m)
	if err != nil {
		writeErr(w, err)
		return
	}
	if created == nil {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "duplicate"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) decideMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accept bool `json:"accept"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	changed, err := s.Store.DecideMemory(r.PathValue("id"), body.Accept)
	if err != nil {
		writeErr(w, err)
		return
	}
	m, err := s.Store.GetMemory(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "memory": m})
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteMemory(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}
