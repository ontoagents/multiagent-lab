package manager

// REQ-481/M281 测试面：
//   - TestMaterializeInferredManager：假构建平面端点（计数透传/图命名/失败降级三态）；
//   - TestEndToEndMaterializedInference：真机 oxigraph（二进制可达时）——raw 双图+推断图
//     装载后 union 查询见跨本体分类推断，GRAPH 域查询证 raw/inferred 双图隔离
//     （验收：跨本体 subClassOf 闭包对实例分类的物化可见）。二进制缺失自动 skip。

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine/oxigraph"
)

func TestMaterializeInferredManager(t *testing.T) {
	mk := func(body string, hdr string, code int) *Manager {
		svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/ontologies/materialize" {
				http.NotFound(w, r)
				return
			}
			var in struct {
				Content string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			if !strings.Contains(in.Content, "ttlA") {
				t.Errorf("并置内容应含本体 TTL，实际 %.60s", in.Content)
			}
			if hdr != "" {
				w.Header().Set("X-Onto-Inferred-Triples", hdr)
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(svc.Close)
		return &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	}
	m := mk("<urn:i> <urn:t> <urn:c> .", "7", 200)
	gr, ttl, cnt, line := m.materializeInferred(map[string]string{"a": "ttlA", "b": "ttlB"})
	if gr != "urn:inferred:assembly" || cnt != 7 || !strings.Contains(ttl, "urn:i") || !strings.Contains(line, "推断 7") {
		t.Fatalf("多本体物化不符: gr=%s cnt=%d line=%s", gr, cnt, line)
	}
	if gr2, _, _, _ := m.materializeInferred(map[string]string{"o1": "ttlA"}); gr2 != "urn:inferred:o1" {
		t.Fatalf("单本体图名期望 urn:inferred:o1，实际 %s", gr2)
	}
	m2 := mk(`{"error":"owlrl 未安装"}`, "", 500)
	if gr3, _, _, line3 := m2.materializeInferred(map[string]string{"a": "ttlA"}); gr3 != "" || !strings.Contains(line3, "跳过（HTTP 500") {
		t.Fatalf("失败期望降级跳过，实际 gr=%s line=%s", gr3, line3)
	}
}

func TestEndToEndMaterializedInference(t *testing.T) {
	bin, err := exec.LookPath("oxigraph")
	if err != nil {
		t.Skip("oxigraph 二进制不可达（真机 E2E 为环境增强项，装载语义由桩测试与既有 gate 测试覆盖）")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	// raw 图：A 的 X subClassOf B 的 Y（跨本体公理——被引方 IRI 原样出现即同源轨语义）
	ttlA := "<urn:o:a:concept:X> <http://www.w3.org/2000/01/rdf-schema#subClassOf> <urn:o:b:concept:Y> .\n" +
		"<urn:o:a:instance:i> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:o:a:concept:X> .\n"
	ttlB := "<urn:o:b:concept:Z> <http://www.w3.org/2000/01/rdf-schema#subClassOf> <urn:o:b:concept:Y> .\n"
	// 推断图：并置全集经 sidecar materialize（owlrl）的差集——sidecar 链路由 importer/端点
	// 单测承载，本测试验证装载与查询面。期望推断含实例 i 的跨本体分类 i type B.Y。
	inferred := "<urn:o:a:instance:i> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:o:b:concept:Y> .\n" +
		"<urn:o:a:concept:X> <http://www.w3.org/2000/01/rdf-schema#subClassOf> <urn:o:b:concept:Y> .\n" +
		"<urn:o:b:concept:Z> <http://www.w3.org/2000/01/rdf-schema#subClassOf> <urn:o:b:concept:Y> .\n"
	r := oxigraph.New("", t.TempDir(), t.TempDir())
	proc, err := r.StartWithGraphIsolation(context.Background(), "e2e_mat", port,
		map[string]string{"a": ttlA, "b": ttlB},
		engine.GraphIsolationOpts{ExtraGraphs: map[string]string{"urn:inferred:assembly": inferred}})
	if err != nil {
		t.Skipf("真机 oxigraph 启动失败（环境项）: %v", err)
	}
	defer proc.Stop()
	_ = bin
	endpoint := proc.Endpoint
	deadline := time.Now().Add(20 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("健康检查超时")
		}
		resp, err := http.Post(endpoint, "application/sparql-query", strings.NewReader("SELECT * WHERE { ?s ?p ?o } LIMIT 1"))
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	// 验收查询：union 图中实例 i 的类型含跨本体 B.Y（物化推断可见）
	body := sparqlJSON(t, endpoint, "SELECT ?s WHERE { ?s <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:o:b:concept:Y> }")
	if !strings.Contains(body, "urn:o:a:instance:i") {
		t.Fatalf("union 图应见跨本体推断分类（i type B.Y）:\n%s", body)
	}
	// 双图隔离：推断三元组在 inferred 图、raw 图不含
	q2 := "SELECT ?s WHERE { GRAPH <urn:inferred:assembly> { ?s <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:o:b:concept:Y> } }"
	if !strings.Contains(sparqlJSON(t, endpoint, q2), "urn:o:a:instance:i") {
		t.Fatalf("inferred 图应含跨本体分类三元组")
	}
	q3 := "SELECT ?s WHERE { GRAPH <urn:onto:a> { ?s <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:o:b:concept:Y> } }"
	if strings.Contains(sparqlJSON(t, endpoint, q3), "urn:o:a:instance:i") {
		t.Fatalf("raw 图不应含推断三元组（双图隔离被破坏）")
	}
}

func sparqlJSON(t *testing.T, endpoint, query string) string {
	t.Helper()
	tr := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(query))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("SPARQL %d: %s", resp.StatusCode, string(b))
	}
	return string(b)
}
