package rest

// REQ-262/M256：跨本体引用登记 REST——external_refs 单源三端点（增/查/删+悬空检测）。
// D-O27：external_ref 手动登记 / imports 装载侧解析登记 / 导入外引 IRI 登记三形态统一落
// external_refs 表；本文件承载 API 面（导入侧自动登记在 importer 链，前端区块读 List）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ontoagents/ontoagents/ontology-service/internal/importer"
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

// registerImportRefs REQ-511/M319 登记核（可测）：已登记 IRI（kind=import 同目标）跳过（幂等），
// 新 IRI 落本体级哨兵登记（形态同 mergeApply 装载侧自动登记链）。
func (s *Server) registerImportRefs(ontoID string, iris []string) (registered, skipped []string, err error) {
	existing, err := s.Store.ListExternalRefs(ontoID)
	if err != nil {
		return nil, nil, err
	}
	have := map[string]bool{}
	for _, ref := range existing {
		if ref.RefKind == "import" {
			have[ref.ToConcept] = true
		}
	}
	registered, skipped = []string{}, []string{}
	for _, iri := range iris {
		if have[iri] {
			skipped = append(skipped, iri)
			continue
		}
		if _, er := s.Store.CreateExternalRef(&repo.ExternalRef{
			FromOntoID: ontoID, FromConcept: repo.OntologyLevelRef, ToConcept: iri,
			RefKind: "import", Justification: "import_statement",
			Note: "导入向导引用语义登记（REQ-511；不改原文件，闭包装载侧消费）",
		}); er != nil {
			return nil, nil, fmt.Errorf("登记 %s 失败: %w", iri, er)
		}
		registered = append(registered, iri)
	}
	return registered, skipped, nil
}

// registerExternalRefsFromImport REQ-511/M319：导入向导「引用」复用语义端点——上传文件文本级
// 提取 owl:imports 目标（复用 importer.DetectExternalImports，与 mergeApply 装载侧自动登记同源），
// 登记为 external_refs kind=import 本体级哨兵（from_concept=(ontology)，形态同 mergeApply 链）。
// 幂等：已登记 IRI（kind=import 同目标）跳过不重复登记；**不落版本快照**——引用语义=不改原文件
// （本本体 spec 零变更，吃 REQ-479~482 装载闭包/查询归因下游收益）；复杂语义合并边界不动（26 号 §8-3）。
func (s *Server) registerExternalRefsFromImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, fmt.Errorf("body 解析失败: %w", err))
		return
	}
	iris := importer.DetectExternalImports(body.Content)
	registered, skipped, err := s.registerImportRefs(id, iris)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registered": len(registered), "skipped": len(skipped),
		"iris": registered, "skipped_iris": skipped,
		"note": map[bool]string{true: "文件未探测到 owl:imports 目标——引用语义无登记对象，可改走「并入」路径", false: ""}[len(iris) == 0],
	})
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
