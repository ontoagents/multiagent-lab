package rest

// REQ-318/M120 透传公理单测：导入（sidecar 输出 unprojected→artifact axioms-raw 存档）+
// 导出 TTL 拼接还原（X-Onto-Triples 头移除——拼接后计数未知）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/importer"
	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// fakeSidecarScript 生成假 sidecar 桩脚本（python，跨平台——原 /bin/sh 硬编码 Unix-only）：
// parse 输出带 unprojected 的固定 JSON；export 输出固定 TTL。
func fakeSidecarScript(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "fake-sidecar.py")
	script := "# fake sidecar stub（测试基建，非被测行为）\n" +
		"import sys\n" +
		"cmd = sys.argv[1] if len(sys.argv) > 1 else \"\"\n" +
		"if cmd == \"parse\":\n" +
		"    sys.stdout.write('{\"spec\":{\"name\":\"axiom-stub\",\"concepts\":[{\"name\":\"Drug\"}],\"relations\":[],\"instances\":[]},\"warnings\":[],\"lossy\":false,\"unprojected\":[\"<http://ex.org/Drug> rdfs:subClassOf [ ] .\",\"<http://ex.org/hasPart> a <http://www.w3.org/2002/07/owl#TransitiveProperty> .\"]}')\n" +
		"else:\n" +
		"    sys.stdout.write('<http://ex.org/Drug> a <http://www.w3.org/2002/07/owl#Class> .')\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newAxiomTestServer(t *testing.T) *Server {
	t.Helper()
	py := "python"
	if _, err := exec.LookPath(py); err != nil {
		py = "python3"
		if _, err := exec.LookPath(py); err != nil {
			t.Skip("系统无 python（桩脚本运行前置，非被测行为）")
		}
	}
	st, err := repo.Open(filepath.Join(t.TempDir(), "axiom_test.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sc := &importer.Sidecar{Python: py, Script: fakeSidecarScript(t, t.TempDir())}
	// python 桩：argv[1]=parse ✓
	return New(st, sc, llmcreate.New("http://127.0.0.1:1"))
}

func TestAxiomsRawRoundTrip(t *testing.T) {
	s := newAxiomTestServer(t)

	// 导入（sidecar 桩输出 unprojected 2 条）
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/import", strings.NewReader(
		`{"filename":"axiom.ttl","content":"@prefix ex: <http://ex.org/> . ex:Drug a owl:Class ."}`))
	req.Header.Set("Content-Type", "application/json")
	s.importOntology(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("导入期望 201，得 %d body=%s", w.Code, w.Body.String())
	}
	var imp struct {
		Report struct {
			Unprojected []string `json:"unprojected"`
		} `json:"report"`
		Ontology struct {
			ID string `json:"id"`
		} `json:"ontology"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &imp)
	if len(imp.Report.Unprojected) != 2 {
		t.Fatalf("报告应透传 2 条公理片段，得 %v", imp.Report.Unprojected)
	}
	// artifact axioms-raw 在位
	raw, _, err := s.Store.GetArtifact(imp.Ontology.ID, "axioms-raw")
	if err != nil {
		t.Fatalf("axioms-raw 未存档: %v", err)
	}
	if !strings.Contains(raw, "TransitiveProperty") {
		t.Fatalf("axioms-raw 内容不符: %s", raw)
	}

	// 导出 TTL：拼接透传片段 + 无 X-Onto-Triples 头（拼接后计数未知）
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/ontologies/"+imp.Ontology.ID+"/export?format=turtle", nil)
	req2.SetPathValue("id", imp.Ontology.ID)
	s.exportOntology(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("导出期望 200，得 %d body=%s", w2.Code, w2.Body.String())
	}
	body := w2.Body.String()
	if !strings.Contains(body, "未投影公理透传") || !strings.Contains(body, "TransitiveProperty") {
		t.Fatalf("导出应拼接透传片段: %.300s", body)
	}
	if w2.Header().Get("X-Onto-Triples") != "" {
		t.Fatalf("拼接后应移除精确计数头（REQ-293 gate=skipped 诚实口径）")
	}
	_ = fmt.Sprint()
}
