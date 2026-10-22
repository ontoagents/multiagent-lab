//go:build eval

package facade

// REQ-368/M166 companion_history 真机验证（build tag eval，依赖 data/bin/oxigraph 二进制在位）：
// 真实 oxigraph 引擎承载伴生 named graph——入图三节点+两边（其一失效）+vt 起点 →
// 三模式查询断言（snapshot 当日末状态/evolution 新增+失效/evidence 证据链含 vt）。
// SPARQL INSERT 用与 backend/internal/companion 同谓词的夹具（跨 module 不互引，词表同值）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

var histHTTP = &http.Client{Timeout: 15 * time.Second}

func httpNewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, url, body)
}

func histSparqlUpdate(t *testing.T, ctx context.Context, endpoint, q string) {
	t.Helper()
	req, err := httpNewRequestWithContext(ctx, "POST", strings.TrimSuffix(endpoint, "/query")+"/update", strings.NewReader(q))
	if err != nil {
		t.Fatalf("请求构造失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/sparql-update")
	resp, err := histHTTP.Do(req)
	if err != nil {
		t.Fatalf("update 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("update 返回 %d", resp.StatusCode)
	}
}

func histSparqlQuery(t *testing.T, ctx context.Context, endpoint, q string) []map[string]any {
	t.Helper()
	req, err := httpNewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(q))
	if err != nil {
		t.Fatalf("请求构造失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := histHTTP.Do(req)
	if err != nil {
		t.Fatalf("query 失败: %v", err)
	}
	defer resp.Body.Close()
	var res struct {
		Results struct {
			Bindings []map[string]any `json:"bindings"`
		} `json:"results"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res.Results.Bindings
}

// TestCompanionHistoryRealEngine 真机 oxigraph 三模式查询正确性。
func TestCompanionHistoryRealEngine(t *testing.T) {
	bin := "../../../data/bin/oxigraph"
	if _, err := os.Stat(bin); err != nil {
		bin = "../../../../data/bin/oxigraph"
		if _, err2 := os.Stat(bin); err2 != nil {
			t.Skipf("oxigraph 二进制不在位（data/bin/oxigraph），跳过真机验证")
		}
	}
	dir := filepath.Join("..", "data", "facade-hist-eval")
	_ = exec.Command("pkill", "-f", "facade-hist-eval").Run()
	time.Sleep(300 * time.Millisecond)
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o755)
	defer func() {
		_ = exec.Command("pkill", "-f", "facade-hist-eval").Run()
	}()
	cmd := exec.Command(bin, "serve", "--location", dir, "--bind", "127.0.0.1:9197")
	if err := cmd.Start(); err != nil {
		t.Fatalf("oxigraph 启动失败: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	endpoint := "http://127.0.0.1:9197/query"
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// 健康等待
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := histHTTP.Get(endpoint + "?query=SELECT%20*%20WHERE%20%7B%7D%20LIMIT%201"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("oxigraph 健康等待超时")
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 入图夹具（与 companion 词表同值）：2 实体（其一带 vt）+2 边（其一失效）
	const bot = "http://eino-lab/ontology/thin/"
	const prov = "http://www.w3.org/ns/prov#"
	const ent = "http://eino-lab/e/"
	g := companionGraphURI("ontx")
	insert := func(q string) { histSparqlUpdate(t, ctx, endpoint, q) }
	insert(fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <http://www.w3.org/2001/XMLSchema#>
INSERT DATA { GRAPH <%s> {
  <%sA1> a bot:Concept ; rdfs:label "散热不良" ; bot:confidence 0.92 ;
    bot:extractedFrom <http://eino-lab/msg/msg-abc> ; prov:generatedAtTime "2026-10-20T08:00:00Z"^^xsd:dateTime ;
    bot:validFrom "2024-01-01"^^xsd:date .
  <%sA2> a bot:Event ; rdfs:label "过热停机" ; bot:confidence 0.88 ;
    bot:extractedFrom <http://eino-lab/msg/msg-def> ; prov:generatedAtTime "2026-10-21T09:00:00Z"^^xsd:dateTime .
} }`, bot, prov, g, ent, ent))
	insert(fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <http://www.w3.org/2001/XMLSchema#>
INSERT DATA { GRAPH <%s> {
  <%sE1> a bot:Relation ; rdfs:label "引起" ; bot:subject <%sA1> ; bot:object <%sA2> ; bot:relName "引起" ;
    bot:confidence 0.9 ; bot:extractedFrom <http://eino-lab/msg/msg-abc> ; prov:generatedAtTime "2026-10-20T08:30:00Z"^^xsd:dateTime .
} }`, bot, prov, g, ent, ent, ent))
	insert(fmt.Sprintf(`PREFIX bot: <%s>
PREFIX prov: <%s>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX xsd: <http://www.w3.org/2001/XMLSchema#>
INSERT DATA { GRAPH <%s> {
  <%sE2> a bot:Relation ; rdfs:label "引起" ; bot:subject <%sA2> ; bot:object <%sA1> ; bot:relName "引起" ;
    bot:confidence 0.7 ; bot:extractedFrom <http://eino-lab/msg/msg-def> ; prov:generatedAtTime "2026-10-21T09:30:00Z"^^xsd:dateTime ;
    bot:invalidAt "2026-10-22T10:00:00Z"^^xsd:dateTime .
} }`, bot, prov, g, ent, ent, ent))

	// facade 挂真引擎端点
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Now().Format(time.RFC3339)
	p := &store.Profile{ID: "p1", Name: "n", Engine: "oxigraph", OntologyIDs: []string{"ontx"},
		Config: "{}", Status: "created", CreatedAt: now, UpdatedAt: now}
	if err := st.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := st.SetStatus("p1", "running", "", ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	f := New(st, func(string) (string, error) { return endpoint, nil })
	h := f.handleCompanionHistory()

	// ① snapshot(2026-10-21)：两实体+两边（E2 当日未失效）
	res, err := h(ctx, reqOf("companion_history", map[string]any{"ontology_id": "ontx", "mode": "snapshot", "day": "2026-10-21"}))
	if err != nil || res.IsError {
		t.Fatalf("snapshot 失败: %v / %v", err, res)
	}
	var snap struct {
		Entities []map[string]any `json:"entities"`
		Edges    []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &snap); err != nil {
		t.Fatalf("snapshot 解析: %v", err)
	}
	if len(snap.Entities) != 2 || len(snap.Edges) != 2 {
		t.Fatalf("快照应为 2 实体+2 边: %d/%d", len(snap.Entities), len(snap.Edges))
	}

	// ② snapshot(2026-10-22)：E2 已失效 → 只剩 1 边（失效语义重放正确性）
	res, _ = h(ctx, reqOf("companion_history", map[string]any{"ontology_id": "ontx", "mode": "snapshot", "day": "2026-10-23"}))
	json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &snap)
	if len(snap.Edges) != 1 {
		t.Fatalf("失效后快照应剩 1 边: %d", len(snap.Edges))
	}

	// ③ evolution(10-21~10-22)：新增 A2+E2，失效 1
	res, _ = h(ctx, reqOf("companion_history", map[string]any{"ontology_id": "ontx", "mode": "evolution", "from": "2026-10-21", "to": "2026-10-22"}))
	var evo struct {
		Added       map[string][]map[string]any `json:"added"`
		Invalidated []map[string]any            `json:"invalidated"`
	}
	json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &evo)
	if len(evo.Added["entities"]) != 1 || len(evo.Added["edges"]) != 1 || len(evo.Invalidated) != 1 {
		t.Fatalf("演化区间不符: %+v", evo)
	}

	// ④ evidence(散热不良)：证据链含源消息 msg-abc 与 vt=2024-01-01
	res, _ = h(ctx, reqOf("companion_history", map[string]any{"ontology_id": "ontx", "mode": "evidence", "entity": "散热不良"}))
	var ev struct {
		Assertions []map[string]any `json:"assertions"`
	}
	json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &ev)
	found := false
	for _, a := range ev.Assertions {
		if a["name"] == "散热不良" {
			found = true
			if a["source_msg"] != "msg-abc" || a["valid_from"] != "2024-01-01" {
				t.Fatalf("证据链不符: %v", a)
			}
		}
	}
	if !found {
		t.Fatalf("未找到实体证据: %v", ev.Assertions)
	}
}
