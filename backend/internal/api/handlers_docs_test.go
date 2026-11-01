package api

// REQ-184 收尾（2026-10-22）：research/ 白名单分支退役行为测试——旧路径显式 400 诚实指路
// （不静默 404）；REQ-570 四区收敛（2026-11-01）：platform-knowledge/ 分支同规则退役，
// docs/（handbook/research/learning 三子区）照常可读。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDocReadServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	handbook := filepath.Join(docs, "handbook") // REQ-570 四区收敛：现状手册区
	if err := os.MkdirAll(handbook, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "18_REQ编号注册表.md"), []byte("# 18"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(handbook, "导读.md"), []byte("# 导读"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Server{DocsRoot: docs, KnowledgeRoot: handbook}, root
}

func TestDocReadResearchRetired(t *testing.T) {
	s, _ := newDocReadServer(t)
	h := http.HandlerFunc(s.docRead)

	// research/ 旧路径：400 + 诚实指路（REQ-570 后调研档居 docs/research/）
	req := httptest.NewRequest(http.MethodGet, "/api/docs/read?path=research/ontology_industry_best_practices_2026-10.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("research/ 旧路径应 400，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "research/ 目录已退役") || !strings.Contains(rec.Body.String(), "docs/research/") {
		t.Fatalf("应回诚实指路文案: %s", rec.Body.String())
	}

	// platform-knowledge/ 旧路径（REQ-570 退役）：400 + 诚实指路三子区
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=platform-knowledge/导读.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("platform-knowledge/ 旧路径应 400，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "platform-knowledge/ 目录已退役") || !strings.Contains(rec.Body.String(), "docs/handbook") {
		t.Fatalf("应回诚实指路文案: %s", rec.Body.String())
	}

	// docs/ 照常可读（根级编号档）
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=docs/18_REQ编号注册表.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# 18") {
		t.Fatalf("docs/ 应照常可读: %d %s", rec.Code, rec.Body.String())
	}

	// docs/handbook 子区照常可读（原 platform-knowledge 内容新落点）
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=docs/handbook/导读.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# 导读") {
		t.Fatalf("docs/handbook/ 应照常可读: %d %s", rec.Code, rec.Body.String())
	}

	// 未知前缀：新默认文案（仅 docs/，不再含 platform-knowledge/）
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=other/x.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "仅支持 docs/ 目录下的文档") {
		t.Fatalf("默认拒绝文案应更新: %d %s", rec.Code, rec.Body.String())
	}
}
