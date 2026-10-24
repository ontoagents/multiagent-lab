package rest

// REQ-436/M232 元素级操作语义上收（接口统一主线）——facade_ops.go 元素 CRUD 编排（GET spec→改→
// PUT If-Match）下沉为构建平面一等 REST：按名定位语义与工具面同源（本文件 applyElementMutation
// 即原 facade 侧 mutation 闭包的逐字移植——可选字段缺席保持原值、显式传入含空串覆盖、实例整体
// 替换语义），版本门禁/Validate/strict/overlay/入库评分经 persistSpec 单源（saveSpec 同一保存核）。
// 响应包络 {operation, saved, base_version, version, detail} 与工具面 specMutate 输出同形——
// facade 转薄透传后 21 件工具签名/输出/审批档零变化。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// elementKinds 合法元素类型（与工具面四类对齐）。
var elementKinds = map[string]bool{"concept": true, "relation": true, "attribute": true, "instance": true}

// registerElementRoutes 元素级端点注册（Mount 内调用）。
func (s *Server) registerElementRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/ontologies/{id}/elements/{kind}", s.elementUpsert)
	m.HandleFunc("DELETE /api/ontologies/{id}/elements/{kind}/{name}", s.elementDelete)
}

// elementBody 原始字段承载（presence 语义：字段缺席=保持原值，显式 null/空串=覆盖——与工具面
// argString/argUpsertNames 同口径，故按 RawMessage 判存在）。
type elementBody map[string]json.RawMessage

func (b elementBody) str(key string) (string, bool) {
	raw, ok := b[key]
	if !ok {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

func (b elementBody) names(key string) ([]string, bool, error) {
	raw, ok := b[key]
	if !ok {
		return nil, false, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, true, nil
	}
	// 顿号/逗号分隔串（工具面 argNames 同口径）
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == '、' || r == ',' || r == '，' })
		out = out[:0]
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out, true, nil
	}
	return nil, false, fmt.Errorf("%s 须为 JSON 数组或分隔串", key)
}

func (b elementBody) obj(key string) (map[string]any, bool, error) {
	raw, ok := b[key]
	if !ok {
		return nil, false, nil
	}
	out := map[string]any{}
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, true, fmt.Errorf("%s 须为 JSON 对象: %v", key, err)
		}
	}
	return out, true, nil
}

func (b elementBody) rels(key string) ([]pkgspec.InstanceRel, bool, error) {
	raw, ok := b[key]
	if !ok {
		return nil, false, nil
	}
	out := []pkgspec.InstanceRel{}
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, true, fmt.Errorf("%s 须为 JSON 数组: %v", key, err)
		}
	}
	return out, true, nil
}

// elementUpsert POST /api/ontologies/{id}/elements/{kind}——按名定位 upsert（编排服务端单源）。
func (s *Server) elementUpsert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind := r.PathValue("kind")
	if !elementKinds[kind] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kind 须为 concept|relation|attribute|instance"})
		return
	}
	var body elementBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON 解析失败: " + err.Error()})
		return
	}
	name, _ := body.str("name")
	if strings.TrimSpace(name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name 必填"})
		return
	}
	sp, baseVersion, errRes := s.loadSpecForEdit(id)
	if errRes != nil {
		writeErr(w, errRes)
		return
	}
	operation := kind + "_upsert"
	detail, perr := applyElementUpsert(kind, body, name, sp)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": operation + ": " + perr.Error()})
		return
	}
	s.finishElementSave(w, id, sp, baseVersion, operation, detail)
}

// elementDelete DELETE /api/ontologies/{id}/elements/{kind}/{name}。
func (s *Server) elementDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind := r.PathValue("kind")
	name := r.PathValue("name")
	if !elementKinds[kind] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kind 须为 concept|relation|attribute|instance"})
		return
	}
	sp, baseVersion, errRes := s.loadSpecForEdit(id)
	if errRes != nil {
		writeErr(w, errRes)
		return
	}
	operation := kind + "_delete"
	detail, perr := applyElementDelete(kind, name, sp)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": operation + ": " + perr.Error()})
		return
	}
	s.finishElementSave(w, id, sp, baseVersion, operation, detail)
}

// loadSpecForEdit 编辑基载：当前 spec + 版本（元素编排的 GET spec→版本 两步服务端化）。
func (s *Server) loadSpecForEdit(id string) (*pkgspec.Spec, int, error) {
	o, err := s.Store.GetOntology(id)
	if err != nil {
		return nil, 0, err
	}
	sp, err := s.loadSpecOrDefault(id)
	if err != nil {
		return nil, 0, err
	}
	return sp, o.Version, nil
}

// finishElementSave 校验+落盘（persistSpec 单源）+工具面同形包络响应。
func (s *Server) finishElementSave(w http.ResponseWriter, id string, sp *pkgspec.Spec, baseVersion int, operation, detail string) {
	v, _, perr := s.persistSpec(id, sp)
	if perr != nil {
		switch perr.Kind {
		case "validation":
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SPEC_VALIDATION_REJECTED（校验未通过，未保存）: " + perr.Message, "validation_errors": perr.ValidationErrors})
		case "strict":
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SPEC_VALIDATION_REJECTED（校验未通过，未保存）: " + perr.Message, "quality_report": perr.Report})
		default:
			writeErr(w, perr.Err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"operation": operation, "ontology_id": id, "saved": true,
		"base_version": baseVersion, "version": v, "detail": detail,
	})
}

// applyElementUpsert 元素 upsert 语义（facade_ops 闭包逐字移植——语义单源后 facade 侧删除）。
func applyElementUpsert(kind string, body elementBody, name string, sp *pkgspec.Spec) (string, error) {
	switch kind {
	case "concept":
		parents, parentsPresent, err := body.names("parents")
		if err != nil {
			return "", err
		}
		synonyms, synonymsPresent, err := body.names("synonyms")
		if err != nil {
			return "", err
		}
		label, hasLabel := body.str("label")
		definition, hasDefinition := body.str("definition")
		var target *pkgspec.Concept
		for i := range sp.Concepts {
			if sp.Concepts[i].Name == name {
				target = &sp.Concepts[i]
				break
			}
		}
		created := target == nil
		if created {
			sp.Concepts = append(sp.Concepts, pkgspec.Concept{Name: name})
			target = &sp.Concepts[len(sp.Concepts)-1]
		}
		if hasLabel {
			target.Label = label
		}
		if hasDefinition {
			target.Definition = definition
		}
		if parentsPresent {
			target.Parents = parents
		}
		if synonymsPresent {
			target.Synonyms = synonyms // REQ-419：同义词（TBox 一等公民）
		}
		verb := "已更新"
		if created {
			verb = "已新增"
		}
		return fmt.Sprintf("概念「%s」%s（父概念 %d 个，同义词 %d 个）", name, verb, len(target.Parents), len(target.Synonyms)), nil
	case "relation":
		from, hasFrom := body.str("from")
		to, hasTo := body.str("to")
		label, hasLabel := body.str("label")
		definition, hasDefinition := body.str("definition")
		evidence, hasEvidence := body.str("evidence")
		var target *pkgspec.Relation
		for i := range sp.Relations {
			if sp.Relations[i].Name == name {
				target = &sp.Relations[i]
				break
			}
		}
		created := target == nil
		if created {
			if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
				return "", fmt.Errorf("新增关系须提供 from/to（定义域/值域概念名）")
			}
			sp.Relations = append(sp.Relations, pkgspec.Relation{Name: name, From: from, To: to})
			target = &sp.Relations[len(sp.Relations)-1]
		} else {
			if hasFrom {
				target.From = from
			}
			if hasTo {
				target.To = to
			}
		}
		if hasLabel {
			target.Label = label
		}
		if hasDefinition {
			target.Definition = definition
		}
		if hasEvidence {
			target.Evidence = evidence
		}
		verb := "已更新"
		if created {
			verb = "已新增"
		}
		return fmt.Sprintf("关系「%s」%s（%s → %s）", name, verb, target.From, target.To), nil
	case "attribute":
		domain, hasDomain := body.str("domain")
		rng, hasRange := body.str("range")
		label, hasLabel := body.str("label")
		definition, hasDefinition := body.str("definition")
		var target *pkgspec.DataProperty
		for i := range sp.DataProperties {
			if sp.DataProperties[i].Name == name {
				target = &sp.DataProperties[i]
				break
			}
		}
		created := target == nil
		if created {
			sp.DataProperties = append(sp.DataProperties, pkgspec.DataProperty{Name: name})
			target = &sp.DataProperties[len(sp.DataProperties)-1]
		}
		if hasDomain {
			target.Domain = domain
		}
		if hasRange {
			target.Range = rng
		}
		if hasLabel {
			target.Label = label
		}
		if hasDefinition {
			target.Definition = definition
		}
		verb := "已更新"
		if created {
			verb = "已新增"
		}
		dom := target.Domain
		if dom == "" {
			dom = "（不限定义域）"
		}
		return fmt.Sprintf("数据属性「%s」%s（定义域 %s，类型 %s）", name, verb, dom, target.Range), nil
	case "instance":
		concept, hasConcept := body.str("concept")
		attributes, hasAttributes, err := body.obj("attributes")
		if err != nil {
			return "", err
		}
		relations, hasRelations, err := body.rels("relations")
		if err != nil {
			return "", err
		}
		var target *pkgspec.Instance
		for i := range sp.Instances {
			if sp.Instances[i].Name == name {
				target = &sp.Instances[i]
				break
			}
		}
		created := target == nil
		if created {
			if strings.TrimSpace(concept) == "" {
				return "", fmt.Errorf("新增实例须提供 concept（所属概念名）")
			}
			sp.Instances = append(sp.Instances, pkgspec.Instance{Name: name, Concept: concept})
			target = &sp.Instances[len(sp.Instances)-1]
		} else if hasConcept {
			target.Concept = concept
		}
		if hasAttributes {
			target.Attributes = attributes
		}
		if hasRelations {
			target.Relations = relations
		}
		verb := "已更新"
		if created {
			verb = "已新增"
		}
		return fmt.Sprintf("实例「%s」%s（类型 %s，属性 %d 项，关系断言 %d 条）", name, verb, target.Concept, len(target.Attributes), len(target.Relations)), nil
	}
	return "", fmt.Errorf("未知元素类型 %q", kind)
}

// applyElementDelete 元素 delete 语义（按名定位；悬空引用交校验拒绝——同工具面文案）。
func applyElementDelete(kind, name string, sp *pkgspec.Spec) (string, error) {
	switch kind {
	case "concept":
		for i := range sp.Concepts {
			if sp.Concepts[i].Name == name {
				sp.Concepts = append(sp.Concepts[:i], sp.Concepts[i+1:]...)
				return fmt.Sprintf("概念「%s」已删除（若被引用，校验会拒绝并给出悬空定位）", name), nil
			}
		}
		return "", fmt.Errorf("概念「%s」不存在", name)
	case "relation":
		for i := range sp.Relations {
			if sp.Relations[i].Name == name {
				sp.Relations = append(sp.Relations[:i], sp.Relations[i+1:]...)
				return fmt.Sprintf("关系「%s」已删除", name), nil
			}
		}
		return "", fmt.Errorf("关系「%s」不存在", name)
	case "attribute":
		for i := range sp.DataProperties {
			if sp.DataProperties[i].Name == name {
				sp.DataProperties = append(sp.DataProperties[:i], sp.DataProperties[i+1:]...)
				return fmt.Sprintf("数据属性「%s」已删除（实例侧同名属性值不动）", name), nil
			}
		}
		return "", fmt.Errorf("数据属性「%s」不存在", name)
	case "instance":
		for i := range sp.Instances {
			if sp.Instances[i].Name == name {
				sp.Instances = append(sp.Instances[:i], sp.Instances[i+1:]...)
				return fmt.Sprintf("实例「%s」已删除", name), nil
			}
		}
		return "", fmt.Errorf("实例「%s」不存在", name)
	}
	return "", fmt.Errorf("未知元素类型 %q", kind)
}
