package rest

// REQ-262/M256：跨本体引用登记 REST——external_refs 单源三端点（增/查/删+悬空检测）。
// D-O27：external_ref 手动登记 / imports 装载侧解析登记 / 导入外引 IRI 登记三形态统一落
// external_refs 表；本文件承载 API 面（导入侧自动登记在 importer 链，前端区块读 List）。

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

// registerExternalRef POST /api/ontologies/{id}/external-refs
func (s *Server) registerExternalRef(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body repo.ExternalRef
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, fmt.Errorf("body 解析失败: %w", err))
		return
	}
	body.FromOntoID = id
	ref, err := s.Store.CreateExternalRef(&body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ref)
}

// listExternalRefs GET /api/ontologies/{id}/external-refs（附悬空检测结果）
func (s *Server) listExternalRefs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	refs, err := s.Store.ListExternalRefs(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if refs == nil {
		refs = []*repo.ExternalRef{}
	}
	dangling, _ := s.Store.DetectDanglingRefs(id)
	if dangling == nil {
		dangling = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"refs": refs, "dangling": dangling})
}

// deleteExternalRef DELETE /api/ontologies/{id}/external-refs/{refId}
func (s *Server) deleteExternalRef(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteExternalRef(r.PathValue("id"), r.PathValue("refId")); err != nil {
		writeErr(w, fmt.Errorf("引用登记不存在或已删除: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("refId")})
}
