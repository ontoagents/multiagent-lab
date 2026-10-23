package manager

// REQ-377/M173：伴生图持久化单测——快照 SELECT 落盘 / 回放 INSERT DATA / 空图清快照。
// httptest 桩 SPARQL 端点（SELECT 返回 bindings；UPDATE 捕获注入语句）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompanionGraphSnapshotAndRestore(t *testing.T) {
	var lastUpdate string
	selCount := 0
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("Content-Type"), "sparql-update"):
			bufr, _ := io.ReadAll(r.Body)
			lastUpdate = string(bufr)
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.Header.Get("Accept"), "sparql-results+json"):
			selCount++
			// 首次 SELECT：伴生图 1 三元组（URI/字面量/语言标签三形态）；第二次（restore 后无查询）不达
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": map[string]any{"bindings": []map[string]any{
					{
						"s": map[string]any{"type": "uri", "value": "urn:o:o1:instance:n1"},
						"p": map[string]any{"type": "uri", "value": "urn:o:o1:relation:部署"},
						"o": map[string]any{"type": "literal", "value": "节点\"甲\"\n二行", "xml:lang": "zh"},
					},
				}},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer svc.Close()

	dir := t.TempDir()
	m := &Manager{SnapshotDir: dir, HTTP: svc.Client()}
	_ = os.MkdirAll(dir, 0o755)

	m.snapshotCompanionGraphs("p1", []string{"o1"}, svc.URL)
	b, err := os.ReadFile(filepath.Join(dir, "o1.json"))
	if err != nil {
		t.Fatalf("快照文件应落盘: %v", err)
	}
	if !strings.Contains(string(b), "urn:o:o1:instance:n1") {
		t.Fatalf("快照应含三元组: %s", b)
	}

	// 回放：INSERT DATA 带 GRAPH + 转义
	m.restoreCompanionGraphs("p1", []string{"o1"}, svc.URL)
	if !strings.Contains(lastUpdate, "INSERT DATA") || !strings.Contains(lastUpdate, "http://eino-lab/graph/ont-o1") {
		t.Fatalf("回放应 INSERT DATA 到伴生图: %s", lastUpdate)
	}
	if !strings.Contains(lastUpdate, "@zh") {
		t.Fatalf("语言标签应保留: %s", lastUpdate)
	}
	if !strings.Contains(lastUpdate, `\n`) {
		t.Fatalf("换行应转义: %s", lastUpdate)
	}
	}

func TestCompanionGraphEmptyClears(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": []map[string]any{}}})
	}))
	defer svc.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "o1.json"), []byte(`{"triples":[[1,2,3]]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{SnapshotDir: dir, HTTP: svc.Client()}
	m.snapshotCompanionGraphs("p1", []string{"o1"}, svc.URL)
	if _, err := os.Stat(filepath.Join(dir, "o1.json")); !os.IsNotExist(err) {
		t.Fatal("空图应清除旧快照（重载后不回放已消失的伴生数据）")
	}
}
