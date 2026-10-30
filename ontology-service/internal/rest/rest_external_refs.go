package rest

// REQ-262/M256：跨本体引用登记 REST——external_refs 单源三端点（增/查/删+悬空检测）。
// D-O27：external_ref 手动登记 / imports 装载侧解析登记 / 导入外引 IRI 登记三形态统一落
// external_refs 表；本文件承载 API 面（导入侧自动登记在 importer 链，前端区块读 List）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
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
	s.Store.AnnotateDangling(refs) // REQ-480/M280：逐条结构化悬空标注（运行平面装载门禁消费）
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

// updateExternalRefPin PUT /api/ontologies/{id}/external-refs/{refId}/pin
// REQ-540：显式刷新/清除一条登记的版本 pin（body {pin_version: N}；0=回浮动头版本）。
// lifecycle plan「pin 落后头版本」提示的处置动线——刷新后重装载才取新版本（显式刷新才漂移）。
func (s *Server) updateExternalRefPin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PinVersion int `json:"pin_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, fmt.Errorf("body 解析失败: %w", err))
		return
	}
	ref, err := s.Store.SetExternalRefPin(r.PathValue("id"), r.PathValue("refId"), body.PinVersion)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, err) // 登记不存在 → 404
			return
		}
		w.WriteHeader(http.StatusBadRequest) // pin 校验错误（负数/外部 IRI/快照缺失）→ 400（沿登记端点先例）
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ref)
}
