package rest

// REQ-542/M349 构建面只读闭包预览 REST 测试：三出料端到端（图+schema+环报告）+
// 预览零写副作用（版本/引用/spec 不动）+404 口径+空闭包形态钉桩。沿 rest_release_test.go 先例。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// newClosureRestServer 闭包预览夹具：A→(import,pin v1)→B、A→(import)→C、B→(import)→C；
// B 演进到 v2（pin v1 解析面验证）。与 newReleaseRestServer 同构（真实保存动线 SaveVersion
// 随 bump 成对）。
func newClosureRestServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "closure_rest.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	seed := func(id, spec string) {
		t.Helper()
		if _, err := st.CreateOntology(id, id, ""); err != nil {
			t.Fatalf("建本体 %s 失败: %v", id, err)
		}
		if err := st.PutArtifact(id, "spec_json", spec, true); err != nil {
			t.Fatalf("写 spec 失败: %v", err)
		}
		if err := st.SaveVersion(id, 1, spec, "", ""); err != nil {
			t.Fatalf("落 v1 快照失败: %v", err)
		}
	}
	seed("A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	seed("B", `{"name":"B","concepts":[{"name":"需求"}]}`)
	seed("C", `{"name":"C","concepts":[{"name":"用例"}]}`)
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "A", FromConcept: repo.OntologyLevelRef, ToOntoID: "B", ToConcept: "需求", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "A", FromConcept: repo.OntologyLevelRef, ToOntoID: "C", ToConcept: "用例", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "B", FromConcept: repo.OntologyLevelRef, ToOntoID: "C", ToConcept: "用例", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("B", "spec_json", `{"name":"B","concepts":[{"name":"需求项"}]}`, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("B"); err != nil || v != 2 {
		t.Fatalf("B 应 bump 到 v2: v=%d err=%v", v, err)
	}
	if err := st.SaveVersion("B", 2, `{"name":"B","concepts":[{"name":"需求项"}]}`, "", ""); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestClosurePreviewEndpointThreeOutputs_Req542(t *testing.T) {
	srv, st := newClosureRestServer(t)
	// 预览前基线（零写副作用对账面）
	versB0, err := st.ListVersions("B")
	if err != nil {
		t.Fatal(err)
	}
	refsA0, err := st.ListExternalRefs("A")
	if err != nil {
		t.Fatal(err)
	}
	ontoA0, err := st.GetOntology("A")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/ontologies/A/closure-preview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("closure-preview 期望 200，实际 %d: %s", resp.StatusCode, body)
	}
	var out struct {
		RootID string           `json:"root_id"`
		Nodes  []map[string]any `json:"nodes"`
		Edges  []map[string]any `json:"edges"`
		Pins   map[string]any   `json:"pins"`
		Schema struct {
			Sections      []map[string]any `json:"sections"`
			ConceptNames  []string         `json:"concept_names"`
			TotalConcepts int              `json:"total_concepts"`
		} `json:"schema"`
		Cycles []any `json:"cycles"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("响应应为合法 JSON: %v\n%s", err, body)
	}
	// ①全量闭包图：root 在首恰 3 节点、3 条装载边、pin 解析
	if out.RootID != "A" || len(out.Nodes) != 3 || len(out.Edges) != 3 {
		t.Fatalf("三出料①闭包图不符: root=%s nodes=%d edges=%d\n%s", out.RootID, len(out.Nodes), len(out.Edges), body)
	}
	if out.Nodes[0]["resolution"] != "root" {
		t.Fatalf("首节点应为 root，实际 %v", out.Nodes[0])
	}
	if out.Pins["B"] == nil || int(out.Pins["B"].(float64)) != 1 {
		t.Fatalf("pins 应 B=1（REQ-540 解析），实际 %v", out.Pins)
	}
	// ②解析后 schema：B 节按 pin v1（含 v2 已删概念「需求」不在 v2 快照的诚实面）
	if len(out.Schema.Sections) != 3 || out.Schema.TotalConcepts != 3 {
		t.Fatalf("三出料②schema 不符: sections=%d total=%d\n%s", len(out.Schema.Sections), out.Schema.TotalConcepts, body)
	}
	if !strings.Contains(string(body), `"concept_names"`) || !strings.Contains(string(body), "缺陷") {
		t.Fatalf("schema 应含合成视图名单与概念内容:\n%s", body)
	}
	// ③环报告：无环=空数组非 null（空集形态钉桩，沿 REQ-541 纪律）
	if out.Cycles == nil || len(out.Cycles) != 0 || !strings.Contains(string(body), `"cycles":[]`) {
		t.Fatalf("三出料③无环应为空数组: cycles=%v body=%s", out.Cycles, body)
	}
	// 预览零写副作用：版本快照行/引用登记行/本体版本号均不动
	versB1, err := st.ListVersions("B")
	if err != nil {
		t.Fatal(err)
	}
	refsA1, err := st.ListExternalRefs("A")
	if err != nil {
		t.Fatal(err)
	}
	ontoA1, err := st.GetOntology("A")
	if err != nil {
		t.Fatal(err)
	}
	if len(versB1) != len(versB0) || len(refsA1) != len(refsA0) || ontoA1.Version != ontoA0.Version {
		t.Fatalf("预览不得产生写副作用: 版本行 %d→%d 引用行 %d→%d 版本 %d→%d",
			len(versB0), len(versB1), len(refsA0), len(refsA1), ontoA0.Version, ontoA1.Version)
	}
}

func TestClosurePreviewEndpointCycleAnd404_Req542(t *testing.T) {
	srv, st := newClosureRestServer(t)
	// 追加互引环：C→A（A 已在闭包）——环专项报告应呈现 A→C→A 或 A→B→C→A 形态
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "C", FromConcept: repo.OntologyLevelRef, ToOntoID: "A", ToConcept: "缺陷", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/ontologies/A/closure-preview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"cycles":[[`) {
		t.Fatalf("互引环应有环路径数组:\n%s", body)
	}
	var out struct {
		Cycles    [][]string `json:"cycles"`
		Dangling  []any      `json:"dangling"`
		RemoteAny []any      `json:"remote_skipped"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Cycles) == 0 || len(out.Cycles[0]) < 3 || out.Cycles[0][0] != "A" || out.Cycles[0][len(out.Cycles[0])-1] != "A" {
		t.Fatalf("环路径应从 root 起回边闭合，实际 %v", out.Cycles)
	}
	if len(out.Dangling) != 0 {
		t.Fatalf("本夹具不应有悬空面，实际 %v", out.Dangling)
	}
	// 404 口径：不存在本体
	r2, err := http.Get(srv.URL + "/api/ontologies/ghost/closure-preview")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(r2.Body)
		t.Fatalf("不存在本体期望 404，实际 %d: %s", r2.StatusCode, b)
	}
}
