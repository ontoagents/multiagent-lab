package rest

// REQ-480/M280：POST /api/ontologies/reason-check 端点测试——运行平面组装载前并置
// 一致性检查的承载（sidecar reason 管线透传）+sidecar 未配置错误路径。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/importer"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

func reasonCheckStubServer(t *testing.T) *Server {
	t.Helper()
	py := "python"
	if _, err := exec.LookPath(py); err != nil {
		py = "python3"
		if _, err := exec.LookPath(py); err != nil {
			t.Skip("系统无 python（桩脚本运行前置，非被测行为）")
		}
	}
	st, err := repo.Open(filepath.Join(t.TempDir(), "reason_check_test.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p := filepath.Join(t.TempDir(), "fake-reason-sidecar.py")
	script := "# fake sidecar stub（测试基建）\n" +
		"import sys\n" +
		"if sys.argv[1] == \"reason\":\n" +
		"    sys.stdout.write('{\"consistent\":true,\"violations\":[],\"note\":\"stub\"}')\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return New(st, &importer.Sidecar{Python: py, Script: p}, llmcreate.New("http://127.0.0.1:1"))
}

func TestReasonCheckEndpoint_Req480(t *testing.T) {
	s := reasonCheckStubServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/reason-check",
		strings.NewReader(`{"content":"<urn:a> <urn:b> <urn:c> .","format":"turtle"}`))
	s.reasonCheckEndpoint(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"consistent":true`) {
		t.Fatalf("期望 consistent=true 透传: %s", w.Body.String())
	}

	// 空 content → 400
	w2 := httptest.NewRecorder()
	s.reasonCheckEndpoint(w2, httptest.NewRequest("POST", "/api/ontologies/reason-check", strings.NewReader(`{}`)))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("空 content 期望 400，实际 %d", w2.Code)
	}

	// sidecar 未配置 → 500 错误透出（运行平面降级跳过的数据源）
	st, err := repo.Open(filepath.Join(t.TempDir(), "nosidecar.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s2 := New(st, nil, nil)
	w3 := httptest.NewRecorder()
	s2.reasonCheckEndpoint(w3, httptest.NewRequest("POST", "/api/ontologies/reason-check",
		strings.NewReader(`{"content":"x"}`)))
	if w3.Code == http.StatusOK {
		t.Fatalf("sidecar 未配置期望错误，实际 200")
	}
}
