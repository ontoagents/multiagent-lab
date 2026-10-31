package rest

// REQ-545/M352 check_answerable 程序化调用口（POST /api/check-answerable）——
// 出题预检等后端服务经 REST 消费（与 MCP 工具同源 CheckAnswerableCore 零漂移）。

import (
	"encoding/json"
	"net/http"
)

func (s *Server) checkAnswerable(w http.ResponseWriter, r *http.Request) {
	if s.Facade == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facade 未装配"})
		return
	}
	var req struct {
		OntologyID string `json:"ontology_id"`
		Question   string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OntologyID == "" || req.Question == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 与 question 必填"})
		return
	}
	out, err := s.Facade.CheckAnswerableCore(r.Context(), req.OntologyID, req.Question)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}
