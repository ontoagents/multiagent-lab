package rest

// REQ-292/M96 本体编辑三层（base + overlay + audit）与确定性身份（67 号 B4/B5，D-O24）。
// 机器产出（merge apply / CSV 灌装 / 进化采纳）写 spec（base 语义），人工编辑记 overlay 行；
// 机器写入后按确定性身份 logical_id 重放 overlay——根治「重算覆盖人工编辑」。
// spec_json 恒为合并后的 effective 文档（所有读路径零改动）；overlay 行另存 base 快照供 revert。
// 审计：backend auditOntologyProxy 对 overlay 端点落 onto_decision（overlay.edit / overlay.revert）。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

// ---- 对象存取（按 kind 的统一四大件）----

func validOverlayKind(kind string) bool {
	switch kind {
	case pkgspec.ObjKindConcept, pkgspec.ObjKindRelation, pkgspec.ObjKindDataProperty, pkgspec.ObjKindInstance:
		return true
	}
	return false
}

// specGetObject 取对象（返回副本指针与 JSON）；instance 需 conceptName 定位逻辑身份。
func specGetObject(sp *pkgspec.Spec, kind, name string) (obj any, raw string, ok bool) {
	switch kind {
	case pkgspec.ObjKindConcept:
		for i := range sp.Concepts {
			if sp.Concepts[i].Name == name {
				return &sp.Concepts[i], jsonOf(sp.Concepts[i]), true
			}
		}
	case pkgspec.ObjKindRelation:
		for i := range sp.Relations {
			if sp.Relations[i].Name == name {
				return &sp.Relations[i], jsonOf(sp.Relations[i]), true
			}
		}
	case pkgspec.ObjKindDataProperty:
		for i := range sp.DataProperties {
			if sp.DataProperties[i].Name == name {
				return &sp.DataProperties[i], jsonOf(sp.DataProperties[i]), true
			}
		}
	case pkgspec.ObjKindInstance:
		for i := range sp.Instances {
			if sp.Instances[i].Name == name {
				return &sp.Instances[i], jsonOf(sp.Instances[i]), true
			}
		}
	}
	return nil, "", false
}

// specSetObject 按名替换或追加（由 obj 的 name/concept 定位）。
func specSetObject(sp *pkgspec.Spec, kind string, obj any) {
	switch kind {
	case pkgspec.ObjKindConcept:
		c := obj.(pkgspec.Concept)
		for i := range sp.Concepts {
			if sp.Concepts[i].Name == c.Name {
				sp.Concepts[i] = c
				return
			}
		}
		sp.Concepts = append(sp.Concepts, c)
	case pkgspec.ObjKindRelation:
		rl := obj.(pkgspec.Relation)
		for i := range sp.Relations {
			if sp.Relations[i].Name == rl.Name {
				sp.Relations[i] = rl
				return
			}
		}
		sp.Relations = append(sp.Relations, rl)
	case pkgspec.ObjKindDataProperty:
		dp := obj.(pkgspec.DataProperty)
		for i := range sp.DataProperties {
			if sp.DataProperties[i].Name == dp.Name {
				sp.DataProperties[i] = dp
				return
			}
		}
		sp.DataProperties = append(sp.DataProperties, dp)
	case pkgspec.ObjKindInstance:
		it := obj.(pkgspec.Instance)
		for i := range sp.Instances {
			if sp.Instances[i].Name == it.Name {
				sp.Instances[i] = it
				return
			}
		}
		sp.Instances = append(sp.Instances, it)
	}
}

func specRemoveObject(sp *pkgspec.Spec, kind, name string) bool {
	switch kind {
	case pkgspec.ObjKindConcept:
		for i := range sp.Concepts {
			if sp.Concepts[i].Name == name {
				sp.Concepts = append(sp.Concepts[:i], sp.Concepts[i+1:]...)
				return true
			}
		}
	case pkgspec.ObjKindRelation:
		for i := range sp.Relations {
			if sp.Relations[i].Name == name {
				sp.Relations = append(sp.Relations[:i], sp.Relations[i+1:]...)
				return true
			}
		}
	case pkgspec.ObjKindDataProperty:
		for i := range sp.DataProperties {
			if sp.DataProperties[i].Name == name {
				sp.DataProperties = append(sp.DataProperties[:i], sp.DataProperties[i+1:]...)
				return true
			}
		}
	case pkgspec.ObjKindInstance:
		for i := range sp.Instances {
			if sp.Instances[i].Name == name {
				sp.Instances = append(sp.Instances[:i], sp.Instances[i+1:]...)
				return true
			}
		}
	}
	return false
}

// specMachineJSON 全对象机器值快照（kind:name → JSON）——overlay base 刷新数据源。
func specMachineJSON(sp *pkgspec.Spec) map[string]string {
	m := map[string]string{}
	for _, c := range sp.Concepts {
		m[pkgspec.ObjKindConcept+":"+c.Name] = jsonOf(c)
	}
	for _, r := range sp.Relations {
		m[pkgspec.ObjKindRelation+":"+r.Name] = jsonOf(r)
	}
	for _, dp := range sp.DataProperties {
		m[pkgspec.ObjKindDataProperty+":"+dp.Name] = jsonOf(dp)
	}
	for _, it := range sp.Instances {
		m[pkgspec.ObjKindInstance+":"+it.Name] = jsonOf(it)
	}
	return m
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// jsonEqual 结构相等比较（map 键序无关）。
func jsonEqual(a, b string) bool {
	var av, bv any
	if json.Unmarshal([]byte(a), &av) != nil || json.Unmarshal([]byte(b), &bv) != nil {
		return a == b
	}
	aj, _ := json.Marshal(av)
	bj, _ := json.Marshal(bv)
	return string(aj) == string(bj)
}

// ---- saveSpec 人工整份 PUT 的 overlay 提取 ----

// extractOverlaysFromSave 比较人工新 spec 与现行 spec，产出 overlay 变更集。
// base 取既有行的 base（机器值穿透多轮人工编辑不丢失），无既有行取旧对象值。
func (s *Server) extractOverlaysFromSave(id string, oldSp, newSp *pkgspec.Spec) (apply []*repo.ObjectOverlay, removeKeys []string) {
	type entry struct {
		kind, concept, name, raw string
	}
	collect := func(sp *pkgspec.Spec) map[string]entry {
		m := map[string]entry{}
		for _, c := range sp.Concepts {
			m["concept:"+c.Name] = entry{pkgspec.ObjKindConcept, "", c.Name, jsonOf(c)}
		}
		for _, r := range sp.Relations {
			m["relation:"+r.Name] = entry{pkgspec.ObjKindRelation, "", r.Name, jsonOf(r)}
		}
		for _, dp := range sp.DataProperties {
			m["data_property:"+dp.Name] = entry{pkgspec.ObjKindDataProperty, "", dp.Name, jsonOf(dp)}
		}
		for _, it := range sp.Instances {
			m["instance:"+it.Name] = entry{pkgspec.ObjKindInstance, it.Concept, it.Name, jsonOf(it)}
		}
		return m
	}
	oldM, newM := collect(oldSp), collect(newSp)
	for key, ne := range newM {
		oe, inOld := oldM[key]
		if inOld && oe.raw == ne.raw {
			continue
		}
		lid := pkgspec.ObjectLogicalID(ne.kind, ne.concept, ne.name)
		existing, gerr := s.Store.GetOverlay(id, lid)
		base := ""
		action := "edit"
		switch {
		case !inOld:
			action = "add"
			if gerr == nil && existing.Action == "delete" {
				// 人工删除后又加回 → 转为 edit（base 保留机器值）
				action = "edit"
				base = existing.BaseJSON
			}
		case gerr == nil && existing.Action != "add":
			base = existing.BaseJSON // 机器值穿透：沿用最初 base
		case gerr == nil && existing.Action == "add":
			base = "{}"
		default:
			base = oe.raw
		}
		row := &repo.ObjectOverlay{
			OntologyID: id, LogicalID: lid, Kind: ne.kind, Name: ne.name,
			Action: action, BaseJSON: base, OverlayJSON: ne.raw,
		}
		if action == "edit" && base != "" && jsonEqual(base, ne.raw) {
			// 人工改回了机器值 → 撤销 overlay
			removeKeys = append(removeKeys, lid)
			continue
		}
		apply = append(apply, row)
	}
	for key, oe := range oldM {
		if _, inNew := newM[key]; inNew {
			continue
		}
		lid := pkgspec.ObjectLogicalID(oe.kind, oe.concept, oe.name)
		apply = append(apply, &repo.ObjectOverlay{
			OntologyID: id, LogicalID: lid, Kind: oe.kind, Name: oe.name,
			Action: "delete", BaseJSON: oe.raw, OverlayJSON: "{}",
		})
	}
	return apply, removeKeys
}

// persistOverlayDeltas 落 overlay 变更集（保留既有 revision 递增语义：expectedRevision=0 表示跟随现状）。
func (s *Server) persistOverlayDeltas(id string, apply []*repo.ObjectOverlay, removeKeys []string) {
	for _, row := range apply {
		if cur, gerr := s.Store.GetOverlay(id, row.LogicalID); gerr == nil {
			row.Revision = cur.Revision // Upsert 内部 +1
		}
		_, _ = s.Store.UpsertOverlay(row, 0)
	}
	for _, lid := range removeKeys {
		_ = s.Store.DeleteOverlay(id, lid)
	}
}

// ---- 机器写入后的 overlay 重放（merge apply / ingest apply）----

// reapplyOverlays 把 overlay 行重放到机器产出 spec 上（先 delete 后 edit 后 add；
// 逐行校验，引用缺失的行跳过不阻断——退化但不停工，REQ-295④ 口径）。
func reapplyOverlays(sp *pkgspec.Spec, rows []*repo.ObjectOverlay) (applied, skipped int, notes []string) {
	order := map[string]int{"delete": 0, "edit": 1, "add": 2}
	sorted := append([]*repo.ObjectOverlay{}, rows...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && order[sorted[j].Action] < order[sorted[j-1].Action]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	for _, row := range sorted {
		before := jsonOf(sp)
		switch row.Action {
		case "delete":
			if specRemoveObject(sp, row.Kind, row.Name) {
				notes = append(notes, fmt.Sprintf("「%s」为人工删除（overlay），重算产物已同步移除", row.Name))
			} else {
				continue
			}
		case "edit", "add":
			obj, ok := decodeOverlayObject(row.Kind, row.OverlayJSON)
			if !ok {
				skipped++
				notes = append(notes, fmt.Sprintf("overlay 行 %s/%s 解析失败，已跳过", row.Kind, row.Name))
				continue
			}
			specSetObject(sp, row.Kind, obj)
			if row.Action == "edit" {
				notes = append(notes, fmt.Sprintf("「%s」的人工编辑已保留（重算不覆盖，REQ-292）", row.Name))
			} else {
				notes = append(notes, fmt.Sprintf("「%s」为人工新增（overlay），重算后已恢复", row.Name))
			}
		default:
			continue
		}
		if errs := sp.Validate(); len(errs) > 0 {
			// 单行应用破坏引用完整性 → 回滚该行，继续（诚实注记）
			restored := &pkgspec.Spec{}
			if json.Unmarshal([]byte(before), restored) == nil {
				*sp = *restored
			}
			skipped++
			notes = append(notes, fmt.Sprintf("overlay 行 %s/%s 应用后引用缺失，已跳过（可人工核对）", row.Kind, row.Name))
			continue
		}
		applied++
	}
	return applied, skipped, notes
}

func decodeOverlayObject(kind, raw string) (any, bool) {
	switch kind {
	case pkgspec.ObjKindConcept:
		var v pkgspec.Concept
		return v, json.Unmarshal([]byte(raw), &v) == nil
	case pkgspec.ObjKindRelation:
		var v pkgspec.Relation
		return v, json.Unmarshal([]byte(raw), &v) == nil
	case pkgspec.ObjKindDataProperty:
		var v pkgspec.DataProperty
		return v, json.Unmarshal([]byte(raw), &v) == nil
	case pkgspec.ObjKindInstance:
		var v pkgspec.Instance
		return v, json.Unmarshal([]byte(raw), &v) == nil
	}
	return nil, false
}

// ---- 防陈旧守卫（REQ-293②，67 号 B7 三重防陈旧的平台映射）----

// parseIfMatch 解析 If-Match 头（v3 / "v3" / 3）。
func parseIfMatch(v string) (int, bool) {
	v = strings.Trim(strings.TrimSpace(v), `"`)
	v = strings.TrimPrefix(v, "v")
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// guardIfMatch saveSpec 文档级乐观锁：不符 409（缺省头=旧客户端兼容，跳过）。
func (s *Server) guardIfMatch(w http.ResponseWriter, r *http.Request, id string) bool {
	em := r.Header.Get("If-Match")
	if em == "" {
		return true
	}
	want, ok := parseIfMatch(em)
	if !ok {
		return true
	}
	o, err := s.Store.GetOntology(id)
	if err != nil {
		writeErr(w, err)
		return false
	}
	if o.Version != want {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":           fmt.Sprintf("保存冲突：本体已更新至 v%d（本次编辑基于 v%d）——请刷新后基于最新内容重试，勿静默覆盖", o.Version, want),
			"current_version": o.Version,
			"base_version":    want,
		})
		return false
	}
	return true
}

// guardBaseVersion 机器写入前置校验（merge apply / ingest apply 的 base_version 表单/JSON 字段；
// 0=缺省跳过）：预览时点后目标被并发修改 → 409 要求重新预览（Foundry「Proposal changed; review its current version」口径）。
func (s *Server) guardBaseVersion(w http.ResponseWriter, id string, baseVersion int) bool {
	if baseVersion <= 0 {
		return true
	}
	o, err := s.Store.GetOntology(id)
	if err != nil {
		writeErr(w, err)
		return false
	}
	if o.Version != baseVersion {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":           fmt.Sprintf("目标本体已变更（预览基于 v%d，当前 v%d）——请基于最新内容重新预览", baseVersion, o.Version),
			"current_version": o.Version,
			"base_version":    baseVersion,
		})
		return false
	}
	return true
}

// loadSpecOrDefault 读现行 spec（无 artifact 视为空骨架——新建流兼容）。
func (s *Server) loadSpecOrDefault(id string) (*pkgspec.Spec, error) {
	raw, _, err := s.Store.GetArtifact(id, "spec_json")
	if errors.Is(err, repo.ErrNotFound) {
		o, gerr := s.Store.GetOntology(id)
		if gerr != nil {
			return nil, gerr
		}
		return &pkgspec.Spec{Name: o.Name, Description: o.Description}, nil
	}
	if err != nil {
		return nil, err
	}
	sp := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), sp); err != nil {
		return nil, fmt.Errorf("spec_json 解析失败: %w", err)
	}
	return sp, nil
}

// ---- overlay 端点 ----

// listOverlays GET /api/ontologies/{id}/overlays：overlay 清单 + 实时生效态。
func (s *Server) listOverlays(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.Store.ListOverlays(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	sp, _ := s.loadSpecOrDefault(id)
	type overlayView struct {
		*repo.ObjectOverlay
		Effective bool   `json:"effective"` // 人工值当前已在 spec 生效
		Missing   bool   `json:"missing"`   // edit 行：对象在现行 spec 不存在（机器已删，重算时将恢复）
		KindLabel string `json:"kind_label"`
	}
	views := make([]overlayView, 0, len(rows))
	labels := map[string]string{pkgspec.ObjKindConcept: "概念", pkgspec.ObjKindRelation: "关系", pkgspec.ObjKindDataProperty: "数据属性", pkgspec.ObjKindInstance: "实例"}
	for _, row := range rows {
		v := overlayView{ObjectOverlay: row, KindLabel: labels[row.Kind]}
		_, raw, ok := specGetObject(sp, row.Kind, row.Name)
		v.Effective = ok && jsonEqual(raw, row.OverlayJSON)
		v.Missing = !ok && row.Action != "add"
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "count": len(views), "overlays": views})
}

// putObjectOverlay PUT /api/ontologies/{id}/objects/{kind}/{name}/overlay：
// 对象级人工编辑（revision 乐观锁 409 + idempotency_key 幂等 + 身份键不可变）。
// body: {values: <对象 JSON>, expected_revision?: int, idempotency_key?: string}
func (s *Server) putObjectOverlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind, name := r.PathValue("kind"), r.PathValue("name")
	if !validOverlayKind(kind) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind 只能是 concept | relation | data_property | instance"})
		return
	}
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Values           json.RawMessage `json:"values"`
		ExpectedRevision int             `json:"expected_revision"`
		IdempotencyKey   string          `json:"idempotency_key"`
		Actor            string          `json:"actor"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Values) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "values（对象 JSON）必填"})
		return
	}
	obj, ok := decodeOverlayObject(kind, string(body.Values))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "values 解析失败（形态须与 kind 匹配）"})
		return
	}
	// 身份键不可变（Foundry：immutable primaryKey；provenance 锁定沿 REQ-286 C1 先例）
	concept := ""
	switch o := obj.(type) {
	case pkgspec.Concept:
		if o.Name != name {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "身份键不可变：values.name 与路径名不一致"})
			return
		}
	case pkgspec.Relation:
		if o.Name != name {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "身份键不可变：values.name 与路径名不一致"})
			return
		}
	case pkgspec.DataProperty:
		if o.Name != name {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "身份键不可变：values.name 与路径名不一致"})
			return
		}
	case pkgspec.Instance:
		if o.Name != name {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "身份键不可变：values.name 与路径名不一致"})
			return
		}
		concept = o.Concept
	}
	lid := pkgspec.ObjectLogicalID(kind, concept, name)

	sp, err := s.loadSpecOrDefault(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, curRaw, exists := specGetObject(sp, kind, name)
	action := "edit"
	base := ""
	if exists {
		base = curRaw
	} else {
		action = "add"
		base = "{}"
	}
	// 幂等：同 key 同 hash 直返（不重复写版本）；同 key 异 hash 409
	hash := sha256Hex(string(body.Values))
	if body.IdempotencyKey != "" {
		if cur, gerr := s.Store.GetOverlay(id, lid); gerr == nil && cur.IdemKey == body.IdempotencyKey {
			if cur.IdemHash == hash {
				writeJSON(w, http.StatusOK, map[string]any{"saved": false, "idempotent": true, "logical_id": lid, "revision": cur.Revision})
				return
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": "幂等键已复用于不同内容（idempotency key reused）"})
			return
		}
	}
	row := &repo.ObjectOverlay{
		OntologyID: id, LogicalID: lid, Kind: kind, Name: name,
		Action: action, BaseJSON: base, OverlayJSON: jsonOf(obj),
		IdemKey: body.IdempotencyKey, IdemHash: hash, Actor: body.Actor,
	}
	saved, err := s.Store.UpsertOverlay(row, body.ExpectedRevision)
	if errors.Is(err, repo.ErrConflict) {
		msg := "编辑冲突：该对象已被其他编辑更新（revision 不符）——请刷新后重试"
		if body.IdempotencyKey != "" {
			msg = "幂等键已复用于不同内容（idempotency key reused）"
		}
		curRev := 0
		if cur, gerr := s.Store.GetOverlay(id, lid); gerr == nil {
			curRev = cur.Revision
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": msg, "current_revision": curRev})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	// 应用进现行 spec → 校验 → 新版本
	specSetObject(sp, kind, obj)
	if errs := sp.Validate(); len(errs) > 0 {
		_ = s.Store.DeleteOverlay(id, lid) // 不留半截 overlay
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "应用后校验未通过（引用缺失？）", "validation_errors": errs})
		return
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(id, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	v, _ := s.Store.BumpVersion(id)
	_ = s.Store.SaveVersion(id, v, string(bts), "", "")
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "version": v, "logical_id": lid, "revision": saved.Revision, "action": action})
}

// revertOverlay DELETE /api/ontologies/{id}/objects/{kind}/{name}/overlay：撤回人工编辑回到机器 base。
func (s *Server) revertOverlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind, name := r.PathValue("kind"), r.PathValue("name")
	row, err := s.Store.GetOverlay(id, pkgspec.ObjectLogicalID(kind, "", name))
	if err != nil && kind == pkgspec.ObjKindInstance {
		// instance 的 logical_id 含概念名——按名兜底扫描
		row, err = s.findInstanceOverlay(id, name)
	}
	if err != nil {
		writeErr(w, repo.ErrNotFound)
		return
	}
	sp, err := s.loadSpecOrDefault(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	note := ""
	switch row.Action {
	case "edit":
		if base, ok := decodeOverlayObject(row.Kind, row.BaseJSON); ok {
			specSetObject(sp, row.Kind, base)
			note = "已恢复为机器产出值"
		} else {
			specRemoveObject(sp, row.Kind, row.Name)
			note = "机器值缺失（对象已被重算删除），人工版本已移除"
		}
	case "add":
		specRemoveObject(sp, row.Kind, row.Name)
		note = "人工新增对象已移除（回到机器产出态）"
	case "delete":
		if base, ok := decodeOverlayObject(row.Kind, row.BaseJSON); ok {
			specSetObject(sp, row.Kind, base)
			note = "人工删除已撤销，对象恢复"
		} else {
			note = "base 缺失，仅清除 overlay 记录"
		}
	}
	if errs := sp.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "恢复后校验未通过", "validation_errors": errs})
		return
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(id, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	v, _ := s.Store.BumpVersion(id)
	_ = s.Store.SaveVersion(id, v, string(bts), "", "")
	_ = s.Store.DeleteOverlay(id, row.LogicalID)
	writeJSON(w, http.StatusOK, map[string]any{"reverted": true, "version": v, "note": note, "action": row.Action})
}

// findInstanceOverlay 按名扫描实例 overlay 行（instance logical_id 含概念名，路径只有 name）。
func (s *Server) findInstanceOverlay(id, name string) (*repo.ObjectOverlay, error) {
	rows, err := s.Store.ListOverlays(id)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Kind == pkgspec.ObjKindInstance && row.Name == name {
			return row, nil
		}
	}
	return nil, repo.ErrNotFound
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
