package api

// REQ-184 收尾（2026-10-22）：research/ 白名单分支退役行为测试——
// 旧路径显式 400 诚实指路（不静默 404）；docs/ 与 platform-knowledge/ 照常可读。

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
	knowledge := filepath.Join(root, "platform-knowledge")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(knowledge, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "18_REQ编号注册表.md"), []byte("# 18"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowledge, "导读.md"), []byte("# 导读"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Server{DocsRoot: docs, KnowledgeRoot: knowledge}, root
}

func TestDocReadResearchRetired(t *testing.T) {
	s, _ := newDocReadServer(t)
	h := http.HandlerFunc(s.docRead)

	// research/ 旧路径：400 + 诚实指路（G-5 并入 platform-knowledge）
	req := httptest.NewRequest(http.MethodGet, "/api/docs/read?path=research/ontology_industry_best_practices_2026-10.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("research/ 旧路径应 400，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "research/ 目录已退役") || !strings.Contains(rec.Body.String(), "platform-knowledge/") {
		t.Fatalf("应回诚实指路文案: %s", rec.Body.String())
	}

	// docs/ 照常可读
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=docs/18_REQ编号注册表.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# 18") {
		t.Fatalf("docs/ 应照常可读: %d %s", rec.Code, rec.Body.String())
	}

	// platform-knowledge/ 照常可读
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=platform-knowledge/导读.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# 导读") {
		t.Fatalf("platform-knowledge/ 应照常可读: %d %s", rec.Code, rec.Body.String())
	}

	// 未知前缀：新默认文案（不再含 research/）
	req = httptest.NewRequest(http.MethodGet, "/api/docs/read?path=other/x.md", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "仅支持 docs/ 或 platform-knowledge/") {
		t.Fatalf("默认拒绝文案应更新: %d %s", rec.Code, rec.Body.String())
	}
}
