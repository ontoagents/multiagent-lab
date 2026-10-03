package rest

// REQ-255/M62 批次测试（60 号 H2+H3）：
// ①推理级检查档——original 形态含 disjoint 冲突时 quality/check(reasoning=true) 追加
//   reasoning_owlrl 错误级检查项（依赖 python3+rdflib+owlrl，缺失自动 skip）；
// ②reasoning_check 配置往返；③cq-sparql 守卫（无 CQ 400 / 无 LLM 503）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/importer"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func importerSidecarFor(py, script string) *importer.Sidecar {
	return &importer.Sidecar{Python: py, Script: script}
}

func newReasonTestServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "reason_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("onreason", "推理检查测试本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func postJSONRaw(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func putJSONRaw(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func sidecarReasonReady(t *testing.T) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 不可用")
	}
	for _, dep := range []string{"rdflib", "owlrl"} {
		if out, err := exec.Command(py, "-c", "import "+dep).CombinedOutput(); err != nil {
			t.Skipf("%s 未安装: %s", dep, string(out[:min(60, len(out))]))
		}
	}
	if _, err := os.Stat("../../tools/rdf-sidecar/sidecar.py"); err != nil {
		t.Skip("sidecar.py 不可达")
	}
}

// 注：Server 需要 Sidecar 才能跑推理档——本测试自建带 Sidecar 的服务器。
func newReasonServerWithSidecar(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "reason2.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("onreason2", "矛盾本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	// original 形态：disjoint 冲突（推理档应检出）
	conflict := `@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix ex: <http://ex.org/> .
ex:A a owl:Class ; rdfs:label "A" .
ex:B a owl:Class ; rdfs:label "B" .
ex:A owl:disjointWith ex:B .
ex:x a ex:A, ex:B .`
	if err := st.PutArtifact("onreason2", "turtle", conflict, false); err != nil {
		t.Fatalf("写 original 失败: %v", err)
	}
	// spec_json 合法空壳（结构校验必须过）
	sp := pkgspec.Spec{Name: "矛盾本体", Concepts: []pkgspec.Concept{{Name: "A"}, {Name: "B"}}}
	bts, _ := json.Marshal(sp)
	if err := st.PutArtifact("onreason2", "spec_json", string(bts), true); err != nil {
		t.Fatalf("写 spec 失败: %v", err)
	}
	py, _ := exec.LookPath("python3")
	script, _ := filepath.Abs("../../tools/rdf-sidecar/sidecar.py")
	mux := http.NewServeMux()
	New(st, importerSidecarFor(py, script), nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestReasoningCheckDetectsDisjointConflict(t *testing.T) {
	sidecarReasonReady(t)
	srv := newReasonServerWithSidecar(t)
	code, out := postJSONRaw(t, srv.URL+"/api/ontology/quality/check", `{"ontology_id":"onreason2","save":false,"reasoning":true}`)
	if code != http.StatusOK {
		t.Fatalf("quality/check 期望 200，实际 %d: %v", code, out)
	}
	if out["reasoning_error"] != nil {
		t.Fatalf("推理档执行出错: %v", out["reasoning_error"])
	}
	rn, _ := out["reasoning"].(map[string]any)
	if rn == nil || rn["consistent"] != false {
		t.Fatalf("disjoint 冲突应 consistent=false，实际 %v", out["reasoning"])
	}
	if rn["source"] != "original_turtle" {
		t.Fatalf("检查对象应取 original 形态，实际 %v", rn["source"])
	}
	rep, _ := out["report"].(map[string]any)
	hit := false
	for _, f := range rep["findings"].([]any) {
		if fm := f.(map[string]any); fm["check_id"] == "reasoning_owlrl" && fm["severity"] == "error" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("应追加 reasoning_owlrl 错误级检查项: %v", rep["findings"])
	}
}

func TestQualityConfigReasoningRoundTrip(t *testing.T) {
	srv, _ := newReasonTestServer(t)
	code, out := putJSONRaw(t, srv.URL+"/api/ontologies/onreason/quality-config", `{"reasoning_check":true}`)
	if code != http.StatusOK || out["reasoning_check"] != true {
		t.Fatalf("reasoning_check 开启失败: %d %v", code, out)
	}
	// strict 只改 strict 不清 reasoning
	code, out = putJSONRaw(t, srv.URL+"/api/ontologies/onreason/quality-config", `{"strict":true}`)
	if code != http.StatusOK || out["reasoning_check"] != true || out["strict"] != true {
		t.Fatalf("独立开关互不影响失败: %d %v", code, out)
	}
}

func TestCQSparqlGuards(t *testing.T) {
	srv, st := newReasonTestServer(t)
	// 写入无 CQ 的空 spec（cq-sparql 先读 spec artifact）
	sp := pkgspec.Spec{Name: "推理检查测试本体", Concepts: []pkgspec.Concept{{Name: "A"}}}
	bts, _ := json.Marshal(sp)
	if err := st.PutArtifact("onreason", "spec_json", string(bts), true); err != nil {
		t.Fatalf("写 spec 失败: %v", err)
	}
	// 无 CQ → 400 引导录入
	code, out := postJSONRaw(t, srv.URL+"/api/ontologies/onreason/cq-sparql", `{}`)
	if code != http.StatusBadRequest {
		t.Fatalf("无 CQ 期望 400，实际 %d: %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "CQ") {
		t.Fatalf("错误信息应引导录入 CQ: %v", out)
	}
}
