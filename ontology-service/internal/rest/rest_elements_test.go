package rest

// REQ-436/M232 元素级端点单测：upsert 按名定位（新增/更新 presence 语义）/delete 悬空引用校验拒绝/
// 版本推进与包络同形——persistSpec 单源（saveSpec 同一保存核）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func TestElementUpsertAndDelete(t *testing.T) {
	srv, st := newPublishTestServer(t)

	// 1) 新增概念（parents 顿号串形态）——包络与工具面同形
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"概念B","label":"概念B","parents":"概念A"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert 期望 200，实际 %d", resp.StatusCode)
	}
	var out struct {
		Operation   string `json:"operation"`
		Saved       bool   `json:"saved"`
		BaseVersion int    `json:"base_version"`
		Version     int    `json:"version"`
		Detail      string `json:"detail"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out.Saved || out.Operation != "concept_upsert" || out.Detail == "" || !strings.Contains(out.Detail, "已新增") {
		t.Fatalf("包络不符: %+v", out)
	}
	if out.BaseVersion != 2 || out.Version != 3 {
		t.Fatalf("版本推进不符（v2→v3）: %+v", out)
	}

	// 2) 更新已存在：只覆盖传入字段（presence 语义——概念A definition 未传保持原值）
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"概念A","label":"概念A改"}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("更新期望 200，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()
	raw, _, _ := st.GetArtifact("ontest", "spec_json")
	var sp pkgspec.Spec
	_ = json.Unmarshal([]byte(raw), &sp)
	if len(sp.Concepts) != 2 {
		t.Fatalf("应 2 个概念: %v", sp.Concepts)
	}
	a := sp.Concepts[0]
	if a.Name != "概念A" || a.Label != "概念A改" {
		t.Fatalf("更新语义不符: %+v", a)
	}

	// 3) 悬空引用 delete → 校验拒绝（概念A 被引用场景由 Validate 兜底；此处删不存在报错）
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/ontologies/ontest/elements/concept/不存在", nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("删不存在期望 400，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4) 删除成功 + 落盘
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/ontologies/ontest/elements/concept/概念B", nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("删除期望 200，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()
	raw, _, _ = st.GetArtifact("ontest", "spec_json")
	_ = json.Unmarshal([]byte(raw), &sp)
	if len(sp.Concepts) != 1 {
		t.Fatalf("删除后应剩 1 个概念: %v", sp.Concepts)
	}
}

func TestElementValidationRejects(t *testing.T) {
	srv, _ := newPublishTestServer(t)
	// 悬空引用 upsert（parents 指向不存在概念）→ Validate 拒绝 400+明细
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"孤儿","parents":["幽灵"]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("悬空引用期望 400，实际 %d", resp.StatusCode)
	}
	var e struct {
		Error   string `json:"error"`
		Details []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"validation_errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if !strings.Contains(e.Error, "SPEC_VALIDATION_REJECTED") || len(e.Details) == 0 {
		t.Fatalf("校验拒绝包络不符: %+v", e)
	}
	// 非法 kind
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/nope", strings.NewReader(`{}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 kind 期望 400，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()
	// 实例新增缺 concept
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/instance", strings.NewReader(`{"name":"I1"}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("实例缺 concept 期望 400: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// REQ-419/M215：元素端点同义词——concept upsert 传 synonyms（JSON 数组/分隔串两形态）落 spec。
func TestElementConceptSynonyms(t *testing.T) {
	srv, st := newPublishTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"客户","synonyms":["甲方","委托方"]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert 期望 200，实际 %d", resp.StatusCode)
	}
	raw, _, _ := st.GetArtifact("ontest", "spec_json")
	var sp pkgspec.Spec
	_ = json.Unmarshal([]byte(raw), &sp)
	for _, c := range sp.Concepts {
		if c.Name == "客户" {
			if len(c.Synonyms) != 2 || c.Synonyms[0] != "甲方" {
				t.Fatalf("同义词应落 spec: %+v", c)
			}
			return
		}
	}
	t.Fatal("概念应存在")
	// 分隔串形态更新（整体覆盖）
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"客户","synonyms":"甲方"}`))
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("分隔串更新期望 200，实际 %d", resp2.StatusCode)
	}
	resp2.Body.Close()
	raw, _, _ = st.GetArtifact("ontest", "spec_json")
	_ = json.Unmarshal([]byte(raw), &sp)
	for _, c := range sp.Concepts {
		if c.Name == "客户" {
			if len(c.Synonyms) != 1 || c.Synonyms[0] != "甲方" {
				t.Fatalf("整体覆盖语义不符: %+v", c)
			}
			return
		}
	}
}

// REQ-472/M272：axiom kind（公理组 upsert/删除）+ concept restrictions 整组接收。
func TestElementAxiomAndRestrictions_Req472(t *testing.T) {
	srv, _ := newPublishTestServer(t)
	_ = struct{}{}

	// 0) 备料：概念B/概念C 与关系 relOf（夹具仅含概念A）
	for _, body := range []string{
		`{"name":"概念B"}`, `{"name":"概念C"}`,
		`{"name":"relOf","from":"概念A","to":"概念B"}`,
	} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept", strings.NewReader(body))
		if strings.Contains(body, "relOf") {
			req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/relation", strings.NewReader(body))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("备料失败 %s: %d", body, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// 1) 公理新增（组键 type+subject）
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/axiom",
		strings.NewReader(`{"type":"disjoint_with","subject":"概念A","targets":["概念B"]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("axiom upsert 期望 200，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2) 非法类型拒绝
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/axiom",
		strings.NewReader(`{"type":"gci","subject":"概念A","targets":["概念B"]}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法公理类型期望 400，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3) 公理组更新（targets 整组替换）
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/axiom",
		strings.NewReader(`{"type":"disjoint_with","subject":"概念A","targets":["概念B","概念C"]}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("axiom 更新期望 200，实际 %d", resp.StatusCode)
	}
	var out struct {
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if !strings.Contains(out.Detail, "已更新") || !strings.Contains(out.Detail, "2 个") {
		t.Fatalf("更新 detail 不符: %+v", out)
	}

	// 4) 悬空目标被 Validate 拒
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/axiom",
		strings.NewReader(`{"type":"disjoint_with","subject":"概念A","targets":["不存在"]}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("悬空公理目标期望 400，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5) concept restrictions 整组接收
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/ontologies/ontest/elements/concept",
		strings.NewReader(`{"name":"概念B","restrictions":[{"type":"some_values_from","property":"relOf","target":"概念A"}]}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restrictions upsert 期望 200，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6) 公理删除（type:subject 组键）
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/ontologies/ontest/elements/axiom/disjoint_with:概念A", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("axiom 删除期望 200，实际 %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 7) spec 落库核验：restrictions 在、公理已删
	greq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/ontologies/ontest/spec", nil)
	gresp, err := http.DefaultClient.Do(greq)
	if err != nil {
		t.Fatal(err)
	}
	defer gresp.Body.Close()
	var sp pkgspec.Spec
	_ = json.NewDecoder(gresp.Body).Decode(&sp)
	var b *pkgspec.Concept
	for i := range sp.Concepts {
		if sp.Concepts[i].Name == "概念B" {
			b = &sp.Concepts[i]
		}
	}
	if b == nil || len(b.Restrictions) != 1 || b.Restrictions[0].Type != "some_values_from" {
		t.Fatalf("restrictions 未落库: %+v", b)
	}
	if len(sp.Axioms) != 0 {
		t.Fatalf("公理应已删除，实际 %v", sp.Axioms)
	}
}
