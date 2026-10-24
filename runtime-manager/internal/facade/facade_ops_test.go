package facade

// REQ-366/M164 facade 工具面扩展单测：13 件扩展工具注册可枚举（stub MCP tools/list）、
// spec CRUD 全链（If-Match 版本门禁/校验明细回喂/409 冲突/名称列表参解析）、
// spec_get 超限回落摘要、T3 透传、T5 job 轮询三态。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

// reqOf 构造 tools/call 请求（各 handler 测试共用）。
func reqOf(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// resultJSON 解析 TextContent 结果。
func resultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("空结果: %v", res)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("非文本结果: %T", res.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("结果非 JSON: %v\n%s", err, tc.Text)
	}
	return out
}

// TestOpsToolsRegistered stub MCP 冒烟：Mount 后经真实 MCP 客户端 tools/list 枚举——
// 验收草图「工具清单 15+ 全注册可枚举」；13 件扩展工具名与 backend 门控清单同源对照。
func TestOpsToolsRegistered(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	f := New(st, func(string) (string, error) { return "http://127.0.0.1:1", nil })
	ts := httptest.NewServer(f.Mount())
	defer ts.Close()

	cli, err := mcpclient.NewStreamableHttpClient(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("client create: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("client start: %v", err)
	}
	if _, err := cli.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{ProtocolVersion: "2025-03-26",
			ClientInfo: mcp.Implementation{Name: "smoke", Version: "0"}}}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	res, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	if len(res.Tools) < 15 {
		t.Fatalf("工具清单应 ≥15（REQ-366 6→15+），实际 %d: %v", len(res.Tools), names)
	}
	for _, want := range []string{"get_concept", "get_instance", "list_instances", "neighbors", "list_concepts", "sparql_query",
		"spec_get", "spec_concept_upsert", "spec_concept_delete", "spec_relation_upsert", "spec_relation_delete",
		"spec_attribute_upsert", "spec_attribute_delete", "spec_instance_upsert", "spec_instance_delete",
		"spec_validate", "spec_quality_check", "cq_benchmark_datasets", "cq_benchmark_run"} {
		if !names[want] {
			t.Fatalf("工具 %s 未注册", want)
		}
	}
}

// stubBuildPlane 构建平面桩：内存 spec + 版本，实现 GET 详情/spec、PUT If-Match+校验门禁、
// validate、quality/check、cq-benchmark 三端点与 ai-draft-jobs 轮询。
type stubBuildPlane struct {
	mu      sync.Mutex
	spec    map[string]any
	version int
	// putBehavior 可注入 409/400 行为（空=正常保存）——REQ-436 后元素端点用 elementStatus/elementBody
	putStatus     int
	putBody       string
	elementStatus int
	elementBody   string
	jobStatus   string
	jobProgress string
	jobError    string
	reportRuns  int
	gotIfMatch  string
	gotPutSpec  map[string]any
}

func newStubBuildPlane(specJSON string) *stubBuildPlane {
	sp := map[string]any{}
	_ = json.Unmarshal([]byte(specJSON), &sp)
	return &stubBuildPlane{spec: sp, version: 3, jobStatus: "done",
		jobProgress: "翻译 2/2 · 白名单 2/2 · gold 对齐 1/2", reportRuns: 1}
}

func (s *stubBuildPlane) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ontologies/{id}", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		fmt.Fprintf(w, `{"id":"t","version":%d}`, s.version)
	})
	mux.HandleFunc("GET /api/ontologies/{id}/spec", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		b, _ := json.Marshal(s.spec)
		w.Write(b)
	})
	mux.HandleFunc("PUT /api/ontologies/{id}/spec", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.gotIfMatch = r.Header.Get("If-Match")
		if s.putStatus != 0 {
			w.WriteHeader(s.putStatus)
			io.WriteString(w, s.putBody)
			return
		}
		b, _ := io.ReadAll(r.Body)
		sp := map[string]any{}
		if err := json.Unmarshal(b, &sp); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"JSON 解析失败: %v"}`, err)
			return
		}
		s.gotPutSpec = sp
		s.spec = sp // 内存回写：后续 GET spec 读到最新态（与真构建平面一致）
		s.version++
		fmt.Fprintf(w, `{"saved":true,"version":%d,"overlays_recorded":0}`, s.version)
	})
	// REQ-436：元素级端点桩（编排下沉后的 facade 透传目标——内存语义与构建面一致）
	mux.HandleFunc("POST /api/ontologies/{id}/elements/{kind}", func(w http.ResponseWriter, r *http.Request) {
		kind := r.PathValue("kind")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.elementStatus != 0 {
			w.WriteHeader(s.elementStatus)
			io.WriteString(w, s.elementBody)
			return
		}
		name, _ := body["name"].(string)
		verb := "已更新"
		switch kind {
		case "concept":
			concepts, _ := s.spec["concepts"].([]any)
			var target map[string]any
			for _, c := range concepts {
				if cm, ok := c.(map[string]any); ok && cm["name"] == name {
					target = cm
				}
			}
			if target == nil {
				verb = "已新增"
				target = map[string]any{"name": name}
				s.spec["concepts"] = append(concepts, target)
			}
			for _, k := range []string{"label", "definition", "parents"} {
				if v, ok := body[k]; ok {
					target[k] = v
				}
			}
			parentsN := 0
			if ps, ok := target["parents"].([]any); ok {
				parentsN = len(ps)
			}
			s.version++
			fmt.Fprintf(w, `{"operation":"concept_upsert","ontology_id":"t","saved":true,"base_version":%d,"version":%d,"detail":"概念「%s」%s（父概念 %d 个）"}`, s.version-1, s.version, name, verb, parentsN)
		case "instance":
			instances, _ := s.spec["instances"].([]any)
			var target map[string]any
			for _, c := range instances {
				if cm, ok := c.(map[string]any); ok && cm["name"] == name {
					target = cm
				}
			}
			if target == nil {
				concept, _ := body["concept"].(string)
				if concept == "" {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, `{"error":"instance_upsert: 新增实例须提供 concept（所属概念名）"}`)
					return
				}
				verb = "已新增"
				target = map[string]any{"name": name, "concept": concept}
				s.spec["instances"] = append(instances, target)
			} else if v, ok := body["concept"]; ok {
				target["concept"] = v
			}
			if v, ok := body["attributes"]; ok {
				target["attributes"] = v
			}
			if v, ok := body["relations"]; ok {
				target["relations"] = v
			}
			s.version++
			fmt.Fprintf(w, `{"operation":"instance_upsert","ontology_id":"t","saved":true,"base_version":%d,"version":%d,"detail":"实例「%s」%s（类型 %s，属性 2 项，关系断言 1 条）"}`, s.version-1, s.version, name, verb, target["concept"])
		case "relation":
			relations, _ := s.spec["relations"].([]any)
			var target map[string]any
			for _, c := range relations {
				if cm, ok := c.(map[string]any); ok && cm["name"] == name {
					target = cm
				}
			}
			if target == nil {
				frm, _ := body["from"].(string)
				to, _ := body["to"].(string)
				if frm == "" || to == "" {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, `{"error":"relation_upsert: 新增关系须提供 from/to（定义域/值域概念名）"}`)
					return
				}
				verb = "已新增"
				target = map[string]any{"name": name, "from": frm, "to": to}
				s.spec["relations"] = append(relations, target)
			}
			for _, k := range []string{"from", "to", "label", "definition", "evidence"} {
				if v, ok := body[k]; ok {
					target[k] = v
				}
			}
			s.version++
			fmt.Fprintf(w, `{"operation":"relation_upsert","ontology_id":"t","saved":true,"base_version":%d,"version":%d,"detail":"关系「%s」%s"}`, s.version-1, s.version, name, verb)
		case "attribute":
			dps, _ := s.spec["data_properties"].([]any)
			var target map[string]any
			for _, c := range dps {
				if cm, ok := c.(map[string]any); ok && cm["name"] == name {
					target = cm
				}
			}
			if target == nil {
				verb = "已新增"
				target = map[string]any{"name": name}
				s.spec["data_properties"] = append(dps, target)
			}
			for _, k := range []string{"domain", "range", "label", "definition"} {
				if v, ok := body[k]; ok {
					target[k] = v
				}
			}
			s.version++
			fmt.Fprintf(w, `{"operation":"attribute_upsert","ontology_id":"t","saved":true,"base_version":%d,"version":%d,"detail":"数据属性「%s」%s"}`, s.version-1, s.version, name, verb)
		default:
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"kind 不支持"}`)
		}
	})
	mux.HandleFunc("DELETE /api/ontologies/{id}/elements/{kind}/{name}", func(w http.ResponseWriter, r *http.Request) {
		kind, name := r.PathValue("kind"), r.PathValue("name")
		s.mu.Lock()
		defer s.mu.Unlock()
		key := map[string]string{"concept": "concepts", "relation": "relations", "attribute": "data_properties", "instance": "instances"}[kind]
		items, _ := s.spec[key].([]any)
		out := items[:0]
		found := false
		for _, it := range items {
			if im, ok := it.(map[string]any); ok && im["name"] == name {
				found = true
				continue
			}
			out = append(out, it)
		}
		if !found {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"%s_delete: 「%s」不存在"}`, kind, name)
			return
		}
		s.spec[key] = out
		s.version++
		fmt.Fprintf(w, `{"operation":"%s_delete","ontology_id":"t","saved":true,"base_version":%d,"version":%d,"detail":"已删除"}`, kind, s.version-1, s.version)
	})
	mux.HandleFunc("POST /api/ontologies/{id}/validate", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"ok":false,"validation_errors":[{"path":"concepts[0].name","message":"概念名不能为空"}]}`)
	})
	mux.HandleFunc("POST /api/ontology/quality/check", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"score":{"overall":88.5,"completeness":90,"consistency":87,"maintainability":88},"error_count":0,"warning_count":2}`)
	})
	mux.HandleFunc("GET /api/ontology/cq-benchmark/datasets", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"datasets":[{"name":"defects","count":10},{"name":"gene_core","count":10}],"count":2}`)
	})
	mux.HandleFunc("POST /api/ontology/cq-benchmark/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"job_test_1","total":2}`)
	})
	mux.HandleFunc("GET /api/ai-draft-jobs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		st, prog, rerr := s.jobStatus, s.jobProgress, s.jobError
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"job_test_1","status":%q,"progress":%q,"error":%q}`, st, prog, rerr)
	})
	mux.HandleFunc("GET /api/ontologies/{id}/cq-benchmark-report", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		fmt.Fprintf(w, `{"runs":[{"translated":2,"total":2,"whitelist":2,"gold_align":1}],"count":%d}`, s.reportRuns)
	})
	return mux
}

const stubSpecJSON = `{"name":"测试本体","concepts":[
  {"name":"网络设备","label":"网络设备","definition":"网络相关设备"},
  {"name":"路由器","label":"路由器","parents":["网络设备"]}],
 "relations":[{"name":"连接到","from":"路由器","to":"网络设备"}],
 "data_properties":[{"name":"重量","domain":"网络设备","range":"string"}],
 "instances":[{"name":"R1","concept":"路由器","attributes":{"重量":"2kg"}}]}`

func newOpsFacade(t *testing.T) (*Facade, *stubBuildPlane) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	bp := newStubBuildPlane(stubSpecJSON)
	bts := httptest.NewServer(bp.handler())
	t.Cleanup(bts.Close)
	t.Setenv("BUILD_SVC_URL", bts.URL)
	f := New(st, func(string) (string, error) { return "http://127.0.0.1:1", nil })
	return f, bp
}

func TestSpecConceptUpsertAndDelete(t *testing.T) {
	f, bp := newOpsFacade(t)
	h := f.handleConceptUpsert()

	// 1) 新增概念（parents 顿号分隔形态）——REQ-436 后编排服务端单源（无客户端 If-Match）
	res, err := h(context.Background(), reqOf("spec_concept_upsert", map[string]any{
		"ontology_id": "t", "name": "交换机", "label": "交换机", "definition": "数据链路层设备", "parents": "网络设备、硬件",
	}))
	if err != nil || res.IsError {
		t.Fatalf("upsert 失败: %v / %v", err, res)
	}
	out := resultJSON(t, res)
	if out["saved"] != true || out["version"] != float64(4) {
		t.Fatalf("保存结果不符: %v", out)
	}
	if !strings.Contains(out["detail"].(string), "已新增") {
		t.Fatalf("detail 应标注新增: %v", out["detail"])
	}

	// 2) 更新已存在概念：只覆盖传入字段（definition/parents 保留原值——服务端 presence 语义）
	res, err = h(context.Background(), reqOf("spec_concept_upsert", map[string]any{
		"ontology_id": "t", "name": "路由器", "label": "路由设备",
	}))
	if err != nil || res.IsError {
		t.Fatalf("update 失败: %v / %v", err, res)
	}
	b, _ := json.Marshal(bp.spec)
	var sp struct {
		Concepts []struct {
			Name       string   `json:"name"`
			Label      string   `json:"label"`
			Definition string   `json:"definition"`
			Parents    []string `json:"parents"`
		} `json:"concepts"`
	}
	if err := json.Unmarshal(b, &sp); err != nil {
		t.Fatalf("spec 解析: %v", err)
	}
	var router *struct {
		Name       string   `json:"name"`
		Label      string   `json:"label"`
		Definition string   `json:"definition"`
		Parents    []string `json:"parents"`
	}
	for i := range sp.Concepts {
		if sp.Concepts[i].Name == "路由器" {
			router = &sp.Concepts[i]
		}
	}
	if router == nil || router.Label != "路由设备" || router.Definition != "" || len(router.Parents) != 1 {
		t.Fatalf("更新语义不符（应只覆盖 label，未传字段保持原值）: %+v", router)
	}

	// 3) 删除概念
	dh := f.handleConceptDelete()
	res, err = dh(context.Background(), reqOf("spec_concept_delete", map[string]any{"ontology_id": "t", "name": "交换机"}))
	if err != nil || res.IsError {
		t.Fatalf("delete 失败: %v / %v", err, res)
	}
	var sp2 struct {
		Concepts []map[string]any `json:"concepts"`
	}
	b2, _ := json.Marshal(bp.spec)
	_ = json.Unmarshal(b2, &sp2)
	if len(sp2.Concepts) != 2 {
		t.Fatalf("删除后应剩 2 个概念: %v", sp2.Concepts)
	}
	// 4) 删除不存在概念诚实报错
	res, err = dh(context.Background(), reqOf("spec_concept_delete", map[string]any{"ontology_id": "t", "name": "不存在"}))
	if err != nil || !res.IsError {
		t.Fatalf("删除不存在概念应报错: %v / %v", err, res)
	}
}

func TestSpecCRUDErrorPaths(t *testing.T) {
	f, bp := newOpsFacade(t)
	h := f.handleConceptUpsert()

	// 400 校验未过：validation_errors 明细逐条回喂（元素端点注入）
	bp.mu.Lock()
	bp.elementStatus = http.StatusBadRequest
	bp.elementBody = `{"error":"SPEC_VALIDATION_REJECTED（校验未通过，未保存）: 校验未通过，不允许保存坏本体","validation_errors":[{"path":"concepts[2].parents[0]","message":"引用了未定义概念: 幽灵"}]}`
	bp.mu.Unlock()
	res, err := h(context.Background(), reqOf("spec_concept_upsert", map[string]any{"ontology_id": "t", "name": "X"}))
	if err != nil || !res.IsError {
		t.Fatalf("校验拒绝应报错: %v / %v", err, res)
	}
	tc := res.Content[0].(mcp.TextContent)
	if !strings.Contains(tc.Text, "concepts[2].parents[0]") || !strings.Contains(tc.Text, "幽灵") {
		t.Fatalf("应回喂校验定位明细: %s", tc.Text)
	}

	// 409 版本冲突：回喂 current_version 与重试指引
	bp.mu.Lock()
	bp.elementStatus = http.StatusConflict
	bp.elementBody = `{"error":"保存冲突：本体已更新至 v9（本次编辑基于 v3）","current_version":9,"base_version":3}`
	bp.mu.Unlock()
	res, err = h(context.Background(), reqOf("spec_concept_upsert", map[string]any{"ontology_id": "t", "name": "Y"}))
	if err != nil || !res.IsError {
		t.Fatalf("冲突应报错: %v / %v", err, res)
	}
	tc = res.Content[0].(mcp.TextContent)
	if !strings.Contains(tc.Text, "SPEC_VERSION_CONFLICT") || !strings.Contains(tc.Text, "v9") {
		t.Fatalf("冲突回喂不符: %s", tc.Text)
	}

	// parents JSON 数组形态 + 非法 JSON 报错
	bp.mu.Lock()
	bp.elementStatus = 0
	bp.mu.Unlock()
	res, err = h(context.Background(), reqOf("spec_concept_upsert", map[string]any{
		"ontology_id": "t", "name": "Z", "parents": `["网络设备", "路由器"]`}))
	if err != nil || res.IsError {
		t.Fatalf("JSON 数组 parents 应合法: %v / %v", err, res)
	}
	res, err = h(context.Background(), reqOf("spec_concept_upsert", map[string]any{
		"ontology_id": "t", "name": "Z", "parents": `[网络设备]`}))
	if err != nil || !res.IsError {
		t.Fatalf("非法 parents JSON 应报错: %v / %v", err, res)
	}
}

func TestSpecInstanceUpsert(t *testing.T) {
	f, bp := newOpsFacade(t)
	h := f.handleInstanceUpsert()
	res, err := h(context.Background(), reqOf("spec_instance_upsert", map[string]any{
		"ontology_id": "t", "name": "R2", "concept": "路由器",
		"attributes_json": `{"重量":"3kg","厂商":"ACME"}`,
		"relations_json":  `[{"rel":"连接到","target":"R1"}]`,
	}))
	if err != nil || res.IsError {
		t.Fatalf("instance upsert 失败: %v / %v", err, res)
	}
	b, _ := json.Marshal(bp.spec["instances"])
	var instances []map[string]any
	_ = json.Unmarshal(b, &instances)
	if len(instances) != 2 {
		t.Fatalf("应 2 个实例: %s", b)
	}
	r2 := instances[1]
	if r2["name"] != "R2" {
		t.Fatalf("实例不符: %v", r2)
	}
	// 非法 attributes_json 报错
	res, err = h(context.Background(), reqOf("spec_instance_upsert", map[string]any{
		"ontology_id": "t", "name": "R3", "concept": "路由器", "attributes_json": `{重量:3kg}`}))
	if err != nil || !res.IsError {
		t.Fatalf("非法 attributes_json 应报错: %v / %v", err, res)
	}
	// 新增缺 concept 报错
	res, err = h(context.Background(), reqOf("spec_instance_upsert", map[string]any{"ontology_id": "t", "name": "R4"}))
	if err != nil || !res.IsError {
		t.Fatalf("新增实例缺 concept 应报错: %v / %v", err, res)
	}
}

func TestSpecGetSummaryFallback(t *testing.T) {
	f, _ := newOpsFacade(t)
	// 构造超 48KB 的 spec：3000 概念
	big := map[string]any{"name": "大本体"}
	concepts := make([]map[string]any, 0, 3000)
	for i := 0; i < 3000; i++ {
		concepts = append(concepts, map[string]any{"name": fmt.Sprintf("概念%d号", i), "definition": "这是一个用于撑大体积的定义文本，包含足够的字节数。"})
	}
	big["concepts"] = concepts
	bts, _ := json.Marshal(big)
	bp := newStubBuildPlane(string(bts))
	srv := httptest.NewServer(bp.handler())
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	h := f.handleSpecGet()
	res, err := h(context.Background(), reqOf("spec_get", map[string]any{"ontology_id": "t"}))
	if err != nil || res.IsError {
		t.Fatalf("spec_get 失败: %v / %v", err, res)
	}
	tc := res.Content[0].(mcp.TextContent)
	if !strings.Contains(tc.Text, `"truncated":true`) {
		t.Fatalf("大 spec 应回落摘要并标注 truncated: %.200s", tc.Text)
	}
	if !strings.Contains(tc.Text, "概念0号") || !strings.Contains(tc.Text, "concept_names") {
		t.Fatal("摘要应含概念名清单")
	}
	if strings.Contains(tc.Text, "这是一个用于撑大体积") {
		t.Fatal("摘要不应含定义全文")
	}
}

func TestSpecValidateAndQualityPassthrough(t *testing.T) {
	f, _ := newOpsFacade(t)
	vh := f.handleSpecValidate()
	res, err := vh(context.Background(), reqOf("spec_validate", map[string]any{"ontology_id": "t"}))
	if err != nil || res.IsError {
		t.Fatalf("validate 失败: %v / %v", err, res)
	}
	out := resultJSON(t, res)
	if out["ok"] != false {
		t.Fatalf("桩返回 ok=false 应透传: %v", out)
	}
	qh := f.handleSpecQualityCheck()
	res, err = qh(context.Background(), reqOf("spec_quality_check", map[string]any{"ontology_id": "t", "strict": true}))
	if err != nil || res.IsError {
		t.Fatalf("quality check 失败: %v / %v", err, res)
	}
	out = resultJSON(t, res)
	if out["error_count"] != float64(0) {
		t.Fatalf("质量报告应透传: %v", out)
	}
}

func TestCQBenchRunJobPolling(t *testing.T) {
	oldStep := cqRunPollStep
	cqRunPollStep = 10 * time.Millisecond
	defer func() { cqRunPollStep = oldStep }()

	f, bp := newOpsFacade(t)
	dh := f.handleCQBenchDatasets()
	res, err := dh(context.Background(), reqOf("cq_benchmark_datasets", nil))
	if err != nil || res.IsError {
		t.Fatalf("datasets 失败: %v / %v", err, res)
	}
	out := resultJSON(t, res)
	if out["count"] != float64(2) {
		t.Fatalf("数据集清单应透传: %v", out)
	}

	// done 路径：轮询至完成并附最新报告
	rh := f.handleCQBenchRun()
	res, err = rh(context.Background(), reqOf("cq_benchmark_run", map[string]any{
		"ontology_id": "t", "dataset": "defects", "wait_seconds": float64(5)}))
	if err != nil || res.IsError {
		t.Fatalf("run 失败: %v / %v", err, res)
	}
	out = resultJSON(t, res)
	if out["status"] != "done" || out["job_id"] != "job_test_1" {
		t.Fatalf("done 结果不符: %v", out)
	}
	if out["latest_report"] == nil {
		t.Fatal("done 应附最新报告")
	}

	// running 超时路径：回 job_id 续查指引
	bp.mu.Lock()
	bp.jobStatus = "running"
	bp.jobProgress = "翻译 1/2"
	bp.mu.Unlock()
	res, err = rh(context.Background(), reqOf("cq_benchmark_run", map[string]any{
		"ontology_id": "t", "dataset": "defects", "wait_seconds": float64(0.05)}))
	if err != nil || res.IsError {
		t.Fatalf("running 路径失败: %v / %v", err, res)
	}
	out = resultJSON(t, res)
	if out["status"] != "running" || out["job_id"] != "job_test_1" {
		t.Fatalf("running 结果不符: %v", out)
	}

	// failed 路径
	bp.mu.Lock()
	bp.jobStatus = "failed"
	bp.jobError = "模型通道未配置"
	bp.mu.Unlock()
	res, err = rh(context.Background(), reqOf("cq_benchmark_run", map[string]any{
		"ontology_id": "t", "dataset": "defects"}))
	if err != nil || !res.IsError {
		t.Fatalf("failed 路径应报错: %v / %v", err, res)
	}

	// job_id 续查路径：状态回到 done 后凭 job_id 续查（不重新发起 run）
	bp.mu.Lock()
	bp.jobStatus = "done"
	bp.jobProgress = "翻译 2/2 · 白名单 2/2 · gold 对齐 1/2"
	bp.mu.Unlock()
	res, err = rh(context.Background(), reqOf("cq_benchmark_run", map[string]any{
		"ontology_id": "t", "job_id": "job_test_1", "wait_seconds": float64(1)}))
	if err != nil || res.IsError {
		t.Fatalf("续查失败: %v / %v", err, res)
	}
	out = resultJSON(t, res)
	if out["status"] != "done" {
		t.Fatalf("续查应回 done: %v", out)
	}
}

// TestSpecMutateGuards 元素不存在/必填缺失的诚实报错（不发 PUT）。
func TestSpecMutateGuards(t *testing.T) {
	f, _ := newOpsFacade(t)
	rh := f.handleRelationUpsert()
	// 更新已存在关系：只传 name+label 不需要 from/to
	res, err := rh(context.Background(), reqOf("spec_relation_upsert", map[string]any{
		"ontology_id": "t", "name": "连接到", "label": "连接"}))
	if err != nil || res.IsError {
		t.Fatalf("relation update 失败: %v / %v", err, res)
	}
	// 新增缺 from/to 报错
	res, err = rh(context.Background(), reqOf("spec_relation_upsert", map[string]any{
		"ontology_id": "t", "name": "新关系"}))
	if err != nil || !res.IsError {
		t.Fatalf("新增关系缺 from/to 应报错: %v / %v", err, res)
	}
	// evidence 枚举校验在工具层前置
	res, err = rh(context.Background(), reqOf("spec_relation_upsert", map[string]any{
		"ontology_id": "t", "name": "连接到", "evidence": "猜的"}))
	if err != nil || !res.IsError {
		t.Fatalf("非法 evidence 应报错: %v / %v", err, res)
	}
	// 构建平面不可达诚实报错
	t.Setenv("BUILD_SVC_URL", "http://127.0.0.1:1")
	f3 := New(f.Store, f.Endpoint)
	res, err = f3.handleSpecGet()(context.Background(), reqOf("spec_get", map[string]any{"ontology_id": "t"}))
	if err != nil || !res.IsError {
		t.Fatalf("构建平面不可达应诚实报错: %v / %v", err, res)
	}
}
