package rest

// REQ-331/M131 资产级质量评估 P1：评分历史（写入点+趋势线取数端点）与修复指引端点。
// 临时 SQLite + httptest，零外部依赖（沿 rest_test.go 夹具口径）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func newQualityHistoryServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "qhist_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("qhist", "评分历史测试本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

// 评分历史追加与读取：draft（v0）→ ingest（v1）→ manual（v1）三行，端点时间正序返回
// （三点对照的取数语义=草稿/入库/当前按 source 与时序派生）。
func TestQualityScoreHistoryRoundTrip(t *testing.T) {
	srv, st := newQualityHistoryServer(t)
	for _, r := range []repo.QualityScoreRow{
		{OntologyID: "qhist", Version: 0, Source: "draft", Overall: 88, Completeness: 90, Consistency: 85, Maintainability: 89, ErrorCount: 1, WarningCount: 2, CreatedAt: "2026-10-16T10:00:00Z"},
		{OntologyID: "qhist", Version: 1, Source: "ingest", Overall: 92, Completeness: 93, Consistency: 90, Maintainability: 92, ErrorCount: 0, WarningCount: 1, CreatedAt: "2026-10-16T11:00:00Z"},
		{OntologyID: "qhist", Version: 1, Source: "manual", Overall: 95, Completeness: 96, Consistency: 94, Maintainability: 95, ErrorCount: 0, WarningCount: 0, CreatedAt: "2026-10-16T12:00:00Z"},
	} {
		if err := st.SaveQualityScore(r); err != nil {
			t.Fatalf("写评分行失败: %v", err)
		}
	}
	resp, err := http.Get(srv.URL + "/api/ontologies/qhist/quality-history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200 得到 %d", resp.StatusCode)
	}
	var out struct {
		History []repo.QualityScoreRow `json:"history"`
		Count   int                    `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 3 || len(out.History) != 3 {
		t.Fatalf("期望 3 行得到 %d", out.Count)
	}
	if out.History[0].Source != "draft" || out.History[1].Source != "ingest" || out.History[2].Source != "manual" {
		t.Fatalf("时间正序不符: %s/%s/%s", out.History[0].Source, out.History[1].Source, out.History[2].Source)
	}
	if out.History[0].Version != 0 || out.History[1].Version != 1 {
		t.Fatalf("版本不符: %d/%d", out.History[0].Version, out.History[1].Version)
	}
}

// saveSpec 入库动线自动写 ingest 行（评分随版本追加的写入点实锤）。
func TestSaveSpecWritesIngestScore(t *testing.T) {
	srv, _ := newQualityHistoryServer(t)
	spec := `{"name":"入库评分","concepts":[{"name":"疾病","label":"Disease","definition":"异常生命活动"}],"relations":[],"instances":[]}`
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/ontologies/qhist/spec", strings.NewReader(spec))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("saveSpec 期望 200 得到 %d", resp.StatusCode)
	}
	hresp, err := http.Get(srv.URL + "/api/ontologies/qhist/quality-history")
	if err != nil {
		t.Fatal(err)
	}
	defer hresp.Body.Close()
	var out struct {
		History []repo.QualityScoreRow `json:"history"`
	}
	if err := json.NewDecoder(hresp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.History) != 1 || out.History[0].Source != "ingest" {
		t.Fatalf("saveSpec 应写 1 行 ingest: %+v", out.History)
	}
	if out.History[0].Version != 2 {
		t.Fatalf("版本应为 2（建库 v1+保存 v2）: %d", out.History[0].Version)
	}
}

// 修复指引端点：全量 map 非空且含已知检查项。
func TestQualityRemediationsEndpoint(t *testing.T) {
	srv, _ := newQualityHistoryServer(t)
	resp, err := http.Get(srv.URL + "/api/ontology/quality/remediations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200 得到 %d", resp.StatusCode)
	}
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if len(m) == 0 || m["dangling_relation_endpoint"] == "" || m["hierarchy_cycle"] == "" {
		t.Fatalf("指引缺失: %d 项", len(m))
	}
}
