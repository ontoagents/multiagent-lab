package api

// REQ-389/M185 D2 图状态实验端点（2026-10-29）：项目运行状态 ↔ 伴生图投影与 SPARQL 查询。
// 实验双轨语义（01 §3.10 拍板）：显式 sync 触发、不接主线收尾；Companion 未装配时 501 可懂。

import (
	"encoding/json"
	"errors"
	"net/http"
)

// graphStateSync POST /api/projects/{id}/graph-state/sync {ontology_id}
// 全量投影项目运行状态（会话/todo/委派）到伴生图 urn:state:run（幂等重建）。
func (s *Server) graphStateSync(w http.ResponseWriter, r *http.Request) {
	if s.Companion == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "伴生管线未启用（graph-state 实验件依赖伴生宿主通道）"})
		return
	}
	id := r.PathValue("id")
	var in struct {
		OntologyID string `json:"ontology_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.Store.GetProject(id); err != nil {
		writeErr(w, errors.New("项目不存在"))
		return
	}
	rep, err := s.Companion.SyncRunState(r.Context(), id, in.OntologyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// graphStateQuery POST /api/projects/{id}/graph-state/query {ontology_id, query}
// SPARQL 只读查询运行状态（SELECT/ASK 白名单；空 query 返回示例查询引导）。
func (s *Server) graphStateQuery(w http.ResponseWriter, r *http.Request) {
	if s.Companion == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "伴生管线未启用（graph-state 实验件依赖伴生宿主通道）"})
		return
	}
	id := r.PathValue("id")
	var in struct {
		OntologyID string `json:"ontology_id"`
		Query      string `json:"query"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.Store.GetProject(id); err != nil {
		writeErr(w, errors.New("项目不存在"))
		return
	}
	if in.Query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"sample_query": companionSampleQuery(), "hint": "query 为空时返回示例——状态图 urn:state:run，词表 urn:state:*（实验词表非正式本体资产）"})
		return
	}
	raw, err := s.Companion.QueryRunState(r.Context(), in.OntologyID, in.Query)
	if err != nil {
		writeErr(w, err)
		return
	}
	var out any
	if json.Unmarshal(raw, &out) == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	w.Header().Set("Content-Type", "application/sparql-results+json")
	_, _ = w.Write(raw)
}

func companionSampleQuery() string {
	return "PREFIX state: <urn:state:>\nSELECT ?title ?status WHERE { GRAPH <urn:state:run> { ?t a state:Todo ; state:title ?title ; state:status ?status } }"
}
