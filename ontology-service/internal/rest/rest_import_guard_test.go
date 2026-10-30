package rest

// REQ-305 导入防线单测：体积拒绝（413）与 spec 规模拒绝（400）——拒绝语义取代历史 64MB 静默截断；
// env 阈值可配（<=0 关闭防线）。spec_json 路径零 sidecar 依赖（本地解析）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func newImportGuardServer(t *testing.T) *Server {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "import_guard_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st, nil, nil)
}

func importJSONReq(specContent string) *http.Request {
	// 真实请求形态：{filename, content, name}——spec 内容是 content 字段的字符串值（非裸 spec 当请求体）
	body, _ := json.Marshal(map[string]string{"filename": "spec.json", "content": specContent})
	req := httptest.NewRequest("POST", "/api/ontologies/import", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func minSpecJSON(n int) string {
	var b strings.Builder
	b.WriteString(`{"name":"压测导入","concepts":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf(`{"name":"概念%d"}`, i))
	}
	b.WriteString(`],"relations":[],"instances":[]}`)
	return b.String()
}

// 体积上限：JSON 粘贴路径超限 → 413 友好报错（含「上限」与 MB 数），而非截断后的 JSON parse 错。
func TestImportRejectsOversizedBody(t *testing.T) {
	t.Setenv("IMPORT_MAX_BODY_MB", "1")
	s := newImportGuardServer(t)
	// ~1.2MB 合法 JSON（4 万概念 < 默认 5 万规模线）：体积线独立命中
	big := minSpecJSON(40000)
	w := httptest.NewRecorder()
	s.importOntology(w, importJSONReq(big))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("期望 413，得 %d body=%s", w.Code, w.Body.String())
	}
	msg := w.Body.String()
	if !strings.Contains(msg, "上限") || !strings.Contains(msg, "1 MB") {
		t.Fatalf("报错应含上限与体积： %s", msg)
	}
	// Content-Length 预检分支：请求头带超大长度直接拒绝
	req2 := importJSONReq(`{"name":"x","concepts":[],"relations":[],"instances":[]}`)
	req2.ContentLength = 100 << 20
	w2 := httptest.NewRecorder()
	s.importOntology(w2, req2)
	if w2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("Content-Length 预检期望 413，得 %d", w2.Code)
	}
}

// 规模上限：对象总数超限 → 400 规模拒绝（体积远小于体积线，规模线独立生效）。
func TestImportRejectsOversizedSpecObjects(t *testing.T) {
	t.Setenv("IMPORT_MAX_SPEC_OBJECTS", "100")
	s := newImportGuardServer(t)
	w := httptest.NewRecorder()
	s.importOntology(w, importJSONReq(minSpecJSON(150)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得 %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "规模超过上限") || !strings.Contains(w.Body.String(), "150") {
		t.Fatalf("报错应含规模与计数： %s", w.Body.String())
	}
}

// env 关闭（<=0）：防线停用，行为同历史开放口径。
func TestImportLimitDisabled(t *testing.T) {
	t.Setenv("IMPORT_MAX_SPEC_OBJECTS", "0")
	s := newImportGuardServer(t)
	w := httptest.NewRecorder()
	s.importOntology(w, importJSONReq(minSpecJSON(150)))
	if w.Code != http.StatusCreated {
		t.Fatalf("防线关闭时期望 201，得 %d body=%s", w.Code, w.Body.String())
	}
}

// 正常回归：小 spec_json 默认阈值下导入 201，响应含 ontology 与 report。
func TestImportNormalSmallRegression(t *testing.T) {
	s := newImportGuardServer(t)
	w := httptest.NewRecorder()
	s.importOntology(w, importJSONReq(`{"name":"回归小本体","description":"d","concepts":[{"name":"A"},{"name":"B"}],"relations":[],"instances":[]}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("期望 201，得 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Report struct {
			Format string `json:"format"`
		} `json:"report"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if resp.Report.Format != "spec_json" {
		t.Fatalf("期望 spec_json 格式，得 %q", resp.Report.Format)
	}
}
