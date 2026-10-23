package rest

// REQ-382/M178：按本体路由执行只读 SPARQL——cqbench 执行级对齐与进化门控第三信号的
// 执行通道（跨服务：ontology-service 经 RUNTIME_MGR_URL 调用本端点）。白名单校验
// pkg/sparqlguard（REQ-380 单源）+Facade.QueryOntology 路由执行（RunningByOntology 确定序）。

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ontologySparql(w http.ResponseWriter, r *http.Request) {
	if s.Facade == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facade 未装配"})
		return
	}
	var req struct {
		OntologyID string `json:"ontology_id"`
		Query      string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OntologyID == "" || req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 与 query 必填"})
		return
	}
	bindings, err := s.Facade.QueryOntology(r.Context(), req.OntologyID, req.Query)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": bindings, "count": len(bindings)})
}
