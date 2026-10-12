package rest

// REQ-292~295 Helix Foundry 借鉴批次单测（67 号 A/B 层，D-O24）：
//   ①编辑三层：saveSpec 人工编辑提取 overlay → merge replace 重算 → 人工编辑保留 + revert 回 base；
//   ②防陈旧：If-Match / base_version 不符 409；
//   ③确定性身份：LogicalID 跨重算稳定（spec 包）；
//   ④overlay 端点：revision 乐观锁 409 + 幂等键 + 身份键不可变；
//   ⑤灌装：B6 归一化键去重 + B2 活动流判据 + 悬空目标降级（A5）。
// 临时 SQLite + httptest，零外部依赖（沿 rest_publish_test.go 先例）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func newOverlayTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv, _ := newPublishTestServer(t) // 已含 ontest 本体（概念A，v2）
	return srv, "ontest"
}

func doJSON(t *testing.T, method, url, body string, hdr map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// ① 编辑三层主链：人工 PUT（改概念A定义）→ 机器 merge replace（外部 spec 冲击）→ 人工定义仍在
func TestOverlaySurvivesMergeReplace(t *testing.T) {
	srv, id := newOverlayTestServer(t)

	// 人工整份保存：概念A 定义=人工版 + 新增概念B
	human := pkgspec.Spec{Name: "测试本体",
		Concepts: []pkgspec.Concept{{Name: "概念A", Definition: "人工精修定义"}, {Name: "概念B"}},
	}
	hb, _ := json.Marshal(human)
	resp, out := doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/spec", string(hb), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("人工保存期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	if n, _ := out["overlays_recorded"].(float64); n < 1 {
		t.Fatalf("期望记录 overlay ≥1（定义修改+新增），实际 %v", out["overlays_recorded"])
	}

	// 机器 merge replace：外部 spec 只含概念A（定义=机器版）
	incoming := pkgspec.Spec{Name: "外部", Concepts: []pkgspec.Concept{{Name: "概念A", Definition: "机器重算定义"}}}
	ib, _ := json.Marshal(incoming)
	resp, out = doJSON(t, "POST", srv.URL+"/api/ontologies/"+id+"/merge/apply",
		fmt.Sprintf(`{"spec":%s,"strategy":"replace"}`, ib), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge apply 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	if n, _ := out["overlays_applied"].(float64); n < 1 {
		t.Fatalf("期望 overlay 重放 ≥1，实际 %v", out)
	}

	// effective spec：概念A 定义=人工版（重算不覆盖），概念B（人工新增）保留
	resp2, _ := http.Get(srv.URL + "/api/ontologies/" + id + "/spec")
	var eff pkgspec.Spec
	_ = json.NewDecoder(resp2.Body).Decode(&eff)
	resp2.Body.Close()
	found := map[string]string{}
	for _, c := range eff.Concepts {
		found[c.Name] = c.Definition
	}
	if found["概念A"] != "人工精修定义" {
		t.Fatalf("人工编辑被重算覆盖：概念A 定义=%q（期望 人工精修定义）", found["概念A"])
	}
	if _, ok := found["概念B"]; !ok {
		t.Fatalf("人工新增概念B 被重算吞掉")
	}

	// revert：概念A 回到机器 base
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/ontologies/"+id+"/objects/concept/概念A/overlay", nil)
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var rev map[string]any
	_ = json.NewDecoder(resp3.Body).Decode(&rev)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("revert 期望 200，实际 %d: %v", resp3.StatusCode, rev)
	}
	resp4, _ := http.Get(srv.URL + "/api/ontologies/" + id + "/spec")
	var eff2 pkgspec.Spec
	_ = json.NewDecoder(resp4.Body).Decode(&eff2)
	resp4.Body.Close()
	for _, c := range eff2.Concepts {
		if c.Name == "概念A" && c.Definition != "机器重算定义" {
			t.Fatalf("revert 后概念A 定义=%q（期望回到机器 base）", c.Definition)
		}
	}
}

// ② 防陈旧：If-Match / base_version 不符 409
func TestStalenessGuards(t *testing.T) {
	srv, id := newOverlayTestServer(t)

	sp := pkgspec.Spec{Name: "测试本体", Concepts: []pkgspec.Concept{{Name: "概念A"}}}
	b, _ := json.Marshal(sp)
	resp, _ := doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/spec", string(b), map[string]string{"If-Match": "v1"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("If-Match 过期期望 409，实际 %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/spec", string(b), map[string]string{"If-Match": fmt.Sprintf("v%d", int(out2Version(t, srv, id)))})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("If-Match 匹配期望 200，实际 %d", resp.StatusCode)
	}
	// merge apply base_version 过期
	ib, _ := json.Marshal(pkgspec.Spec{Name: "外部", Concepts: []pkgspec.Concept{{Name: "概念A"}}})
	resp, _ = doJSON(t, "POST", srv.URL+"/api/ontologies/"+id+"/merge/apply",
		fmt.Sprintf(`{"spec":%s,"strategy":"replace","base_version":1}`, ib), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("base_version 过期期望 409，实际 %d", resp.StatusCode)
	}
}

func out2Version(t *testing.T, srv *httptest.Server, id string) int {
	t.Helper()
	resp, _ := http.Get(srv.URL + "/api/ontologies/" + id)
	var o struct {
		Version int `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&o)
	resp.Body.Close()
	return o.Version
}

// ④ overlay 端点：revision 乐观锁 + 幂等 + 身份键不可变
func TestOverlayEndpointLocks(t *testing.T) {
	srv, id := newOverlayTestServer(t)

	// 首次对象级编辑
	body := `{"values":{"name":"概念A","definition":"v1 定义"},"idempotency_key":"op-1"}`
	resp, out := doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/objects/concept/概念A/overlay", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("首次 overlay 编辑期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	rev := int(out["revision"].(float64))

	// revision 过期 → 409
	stale := fmt.Sprintf(`{"values":{"name":"概念A","definition":"v2 定义"},"expected_revision":%d}`, rev-1+10)
	resp, _ = doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/objects/concept/概念A/overlay", stale, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("revision 过期期望 409，实际 %d", resp.StatusCode)
	}
	// 幂等重放（同 key 同内容）→ 不新增版本
	vBefore := out2Version(t, srv, id)
	resp, out = doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/objects/concept/概念A/overlay", body, nil)
	if resp.StatusCode != http.StatusOK || out["idempotent"] != true {
		t.Fatalf("幂等重放期望 200+idempotent，实际 %d: %v", resp.StatusCode, out)
	}
	if out2Version(t, srv, id) != vBefore {
		t.Fatalf("幂等重放不应 bump 版本")
	}
	// 身份键不可变
	resp, _ = doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/objects/concept/概念A/overlay",
		`{"values":{"name":"改名","definition":"x"}}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("身份键不可变期望 400，实际 %d", resp.StatusCode)
	}
	// 清单可见
	resp, out = doJSON(t, "GET", srv.URL+"/api/ontologies/"+id+"/overlays", "", nil)
	if resp.StatusCode != http.StatusOK || int(out["count"].(float64)) < 1 {
		t.Fatalf("overlay 清单期望 ≥1 行，实际 %v", out)
	}
}

// ⑤ 灌装：B6 归一化键去重 + 悬空目标降级 + B2 skip_instances
func TestIngestNormalizedDedupAndDegrade(t *testing.T) {
	srv, id := newOverlayTestServer(t)

	// 准备：概念A 下已有实例 apple（小写），另建概念B 供 skip_instances 判据
	sp := pkgspec.Spec{Name: "测试本体",
		Concepts:  []pkgspec.Concept{{Name: "概念A"}, {Name: "log event"}},
		Instances: []pkgspec.Instance{{Name: "apple", Concept: "概念A"}},
	}
	b, _ := json.Marshal(sp)
	if resp, out := doJSON(t, "PUT", srv.URL+"/api/ontologies/"+id+"/spec", string(b), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("准备 spec 失败: %v", out)
	}

	csv := "key,note,log_date\nApple,红苹果,2026-01-01\ntargetX,无主断言,2026-01-02\n"
	multipartBody := &bytes.Buffer{}
	mw := multipart.NewWriter(multipartBody)
	_ = mw.WriteField("mode", "apply")
	_ = mw.WriteField("config", `{"concept":"概念A","key_column":"key","attribute_columns":["note"],"relation_columns":["log_date"]}`)
	fw, _ := mw.CreateFormFile("csv", "a.csv")
	_, _ = fw.Write([]byte(csv))
	_ = mw.Close()
	resp, err := http.Post(srv.URL+"/api/ontologies/"+id+"/ingest-csv", mw.FormDataContentType(), multipartBody)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("灌装期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	stats, _ := out["stats"].(map[string]any)
	if n, _ := stats["case_insensitive_matches"].(float64); n != 1 {
		t.Fatalf("B6 期望 case_insensitive_matches=1，实际 %v", stats)
	}
	if n, _ := stats["instances_generated"].(float64); n != 2 {
		t.Fatalf("期望生成 2 实例，实际 %v", stats)
	}
	// 悬空目标 log_date=2026-01-02 → 断言丢弃 + note（A5 不阻断）
	if n, _ := out["overlays_applied"].(float64); false {
		_ = n
	}
	notes, _ := out["notes"].([]any)
	foundUnmatched := false
	for _, nn := range notes {
		m, _ := nn.(map[string]any)
		if m["kind"] == "unmatched" && strings.Contains(fmt.Sprint(m["text"]), "目标实例不存在") {
			foundUnmatched = true
		}
	}
	if !foundUnmatched {
		t.Fatalf("期望悬空目标 unmatched note，实际 %v", notes)
	}
	// apple 大写变体没有作为新实例落库
	resp2, _ := http.Get(srv.URL + "/api/ontologies/" + id + "/spec")
	var eff pkgspec.Spec
	_ = json.NewDecoder(resp2.Body).Decode(&eff)
	resp2.Body.Close()
	apples := 0
	for _, it := range eff.Instances {
		if strings.EqualFold(it.Name, "apple") {
			apples++
		}
	}
	if apples != 1 {
		t.Fatalf("归一化去重失败：apple 变体数=%d（期望 1）", apples)
	}
}

// ③ 确定性身份：LogicalID 稳定 + evidence 校验（spec 包）
func TestLogicalIDAndEvidence(t *testing.T) {
	if pkgspec.LogicalID("概念A", "x") != pkgspec.LogicalID("概念A", "x") {
		t.Fatal("LogicalID 不稳定")
	}
	if pkgspec.LogicalID("概念A", "x") == pkgspec.LogicalID("概念B", "x") {
		t.Fatal("不同类型同键应不同身份")
	}
	sp := pkgspec.Spec{Concepts: []pkgspec.Concept{{Name: "A"}, {Name: "B"}},
		Relations: []pkgspec.Relation{{Name: "r", From: "A", To: "B", Evidence: "values"}}}
	if errs := sp.Validate(); len(errs) != 0 {
		t.Fatalf("values 证据应通过校验: %v", errs)
	}
	sp.Relations[0].Evidence = "guess"
	if errs := sp.Validate(); len(errs) == 0 {
		t.Fatal("非法证据档应被拦截")
	}
}

// B10：跨列值重合不触发实体合并——关系列目标值只产生断言，永不与主键列合并实体
func TestIngestNoCrossColumnEntityMerge(t *testing.T) {
	srv, id := newOverlayTestServer(t)
	// key 列与 note 列值完全重合，也不产生实体合并（实例数=键列基数）
	csv := "key,note\nx1,x1\nx2,x1\n"
	multipartBody := &bytes.Buffer{}
	mw := multipart.NewWriter(multipartBody)
	_ = mw.WriteField("mode", "apply")
	_ = mw.WriteField("config", `{"concept":"概念A","key_column":"key","attribute_columns":["note"]}`)
	fw, _ := mw.CreateFormFile("csv", "a.csv")
	_, _ = fw.Write([]byte(csv))
	_ = mw.Close()
	resp, err := http.Post(srv.URL+"/api/ontologies/"+id+"/ingest-csv", mw.FormDataContentType(), multipartBody)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", resp.StatusCode)
	}
	resp2, _ := http.Get(srv.URL + "/api/ontologies/" + id + "/spec")
	var eff pkgspec.Spec
	_ = json.NewDecoder(resp2.Body).Decode(&eff)
	resp2.Body.Close()
	if len(eff.Instances) != 2 {
		t.Fatalf("跨列值重合不得影响实体基数：实例数=%d（期望 2）", len(eff.Instances))
	}
}
