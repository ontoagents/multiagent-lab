package facade

// REQ-368/M166 companion_history 三模式单测：桩 SPARQL 端点承载 bindings，
// 断言查询请求形态（FILTER/NOT EXISTS/区间）与结果投影/计数/诚实标注。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

func newHistFacade(t *testing.T, sparqlHandler http.HandlerFunc) *Facade {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		// 路径容错（Windows 分隔符差异场景）——正常仓库形态走上一行
		st, err = store.Open(filepath.Join(t.TempDir(), "t.db"), filepath.Join("..", "..", "migrations"))
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
	}
	t.Cleanup(func() { st.Close() })
	now := time.Now().Format(time.RFC3339)
	p := &store.Profile{ID: "p1", Name: "n", Engine: "oxigraph", OntologyIDs: []string{"ont"},
		Config: "{}", Status: "created", CreatedAt: now, UpdatedAt: now}
	if err := st.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := st.SetStatus("p1", "running", "", ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	srv := httptest.NewServer(sparqlHandler)
	t.Cleanup(srv.Close)
	return New(st, func(string) (string, error) { return srv.URL, nil })
}

const histEntityBindings = `{"results":{"bindings":[
 {"kind":{"type":"uri","value":"http://eino-lab/ontology/thin/Concept"},"label":{"type":"literal","value":"散热不良"},"tt":{"type":"literal","value":"2026-10-20T08:00:00Z"}},
 {"kind":{"type":"uri","value":"http://eino-lab/ontology/thin/Event"},"label":{"type":"literal","value":"过热停机"},"tt":{"type":"literal","value":"2026-10-21T09:00:00Z"}}]}}`

const histEdgeBindings = `{"results":{"bindings":[
 {"src":{"type":"literal","value":"散热不良"},"rel":{"type":"literal","value":"引起"},"dst":{"type":"literal","value":"过热停机"}}]}}`

const histEvolutionInvBindings = `{"results":{"bindings":[
 {"src":{"type":"literal","value":"皮带松动"},"rel":{"type":"literal","value":"引起"},"dst":{"type":"literal","value":"过热停机"},"inv":{"type":"literal","value":"2026-10-22T10:00:00Z"}}]}}`

const histEvidenceNodeBindings = `{"results":{"bindings":[
 {"kind":{"type":"uri","value":"http://eino-lab/ontology/thin/Concept"},"label":{"type":"literal","value":"散热不良"},
  "conf":{"type":"literal","value":"0.92"},"msg":{"type":"uri","value":"http://eino-lab/msg/msg-abc"},
  "tt":{"type":"literal","value":"2026-10-20T08:00:00Z"},"vf":{"type":"literal","value":"2024-01-01"}}]}}`

func histHandler(t *testing.T, gotQueries *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*gotQueries = append(*gotQueries, string(b))
		w.Header().Set("Content-Type", "application/sparql-results+json")
		switch {
		case strings.Contains(string(b), "?inv >="):
			io.WriteString(w, histEvolutionInvBindings)
		case strings.Contains(string(b), "bot:confidence"):
			if strings.Contains(string(b), "bot:subject") {
				io.WriteString(w, `{"results":{"bindings":[]}}`)
			} else {
				io.WriteString(w, histEvidenceNodeBindings)
			}
		case strings.Contains(string(b), "bot:relName"):
			io.WriteString(w, histEdgeBindings)
		default:
			io.WriteString(w, histEntityBindings)
		}
	}
}

func TestCompanionHistorySnapshot(t *testing.T) {
	var got []string
	f := newHistFacade(t, histHandler(t, &got))
	h := f.handleCompanionHistory()
	res, err := h(context.Background(), reqOf("companion_history", map[string]any{
		"ontology_id": "ont", "mode": "snapshot", "day": "2026-10-21",
	}))
	if err != nil || res.IsError {
		t.Fatalf("snapshot 失败: %v / %v", err, res)
	}
	out := resultJSON(t, res)
	if out["day"] != "2026-10-21" || out["mode"] != "snapshot" {
		t.Fatalf("快照参数回显不符: %v", out)
	}
	if out["counts"].(map[string]any)["entities"] != float64(2) || out["counts"].(map[string]any)["edges"] != float64(1) {
		t.Fatalf("计数不符: %v", out["counts"])
	}
	// 查询形态：快照截止点 + 失效排除
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `<= "2026-10-21T23:59:59Z"^^xsd:dateTime`) {
		t.Fatalf("快照截止点缺失: %s", joined)
	}
	if !strings.Contains(joined, "FILTER NOT EXISTS { ?edge bot:invalidAt ?inv") {
		t.Fatalf("失效排除缺失: %s", joined)
	}
}

func TestCompanionHistoryEvolutionAndEvidence(t *testing.T) {
	var got []string
	f := newHistFacade(t, histHandler(t, &got))
	h := f.handleCompanionHistory()

	res, err := h(context.Background(), reqOf("companion_history", map[string]any{
		"ontology_id": "ont", "mode": "evolution", "from": "2026-10-20", "to": "2026-10-22",
	}))
	if err != nil || res.IsError {
		t.Fatalf("evolution 失败: %v / %v", err, res)
	}
	out := resultJSON(t, res)
	counts := out["counts"].(map[string]any)
	if counts["added_entities"] != float64(2) || counts["added_edges"] != float64(1) || counts["invalidated"] != float64(1) {
		t.Fatalf("演化计数不符: %v", counts)
	}
	inv := out["invalidated"].([]any)[0].(map[string]any)
	if inv["source"] != "皮带松动" || inv["invalidated_at"] != "2026-10-22T10:00:00Z" {
		t.Fatalf("失效边投影不符: %v", inv)
	}

	// evidence：实体过滤 + 溯源字段（vt 起点随行）
	res, err = h(context.Background(), reqOf("companion_history", map[string]any{
		"ontology_id": "ont", "mode": "evidence", "entity": "散热不良",
	}))
	if err != nil || res.IsError {
		t.Fatalf("evidence 失败: %v / %v", err, res)
	}
	out = resultJSON(t, res)
	assertions := out["assertions"].([]any)
	if len(assertions) != 1 {
		t.Fatalf("断言数不符: %v", assertions)
	}
	a := assertions[0].(map[string]any)
	if a["source_msg"] != "msg-abc" || a["valid_from"] != "2024-01-01" || a["confidence"] != "0.92" {
		t.Fatalf("证据链投影不符: %v", a)
	}
	if out["note"] == nil || !strings.Contains(out["note"].(string), "源对话消息") {
		t.Fatalf("证据粒度诚实标注缺失: %v", out["note"])
	}

	// 非法 mode 诚实报错
	res, err = h(context.Background(), reqOf("companion_history", map[string]any{"ontology_id": "ont", "mode": "rewind"}))
	if err != nil || !res.IsError {
		t.Fatalf("非法 mode 应报错: %v / %v", err, res)
	}
	// 非法 day 诚实报错
	res, err = h(context.Background(), reqOf("companion_history", map[string]any{"ontology_id": "ont", "mode": "snapshot", "day": "下周三"}))
	if err != nil || !res.IsError {
		t.Fatalf("非法 day 应报错: %v / %v", err, res)
	}
}
