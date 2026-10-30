package rest

// REQ-540/M347 REST 级测试：①export ?version=N 版本化导出（运行面 FetchTTL pin 解析的
// 数据源——快照缺失 404 显式可见=pin 钉死语义，不静默回落头版本）；②PUT pin 端点
// （显式刷新/清除 pin 的处置动线）。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func pinRestFixture(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "pinrest.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("src", "源", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOntology("dst", "目标", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("src", "spec_json", `{"concepts":[{"name":"缺陷"}]}`, true); err != nil {
		t.Fatal(err)
	}
	specV1 := `{"name":"目标","concepts":[{"name":"需求"}]}`
	specV2 := `{"name":"目标","concepts":[{"name":"需求项"}]}`
	if err := st.PutArtifact("dst", "spec_json", specV1, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion("dst", 1, specV1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("dst", "spec_json", specV2, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("dst"); err != nil || v != 2 {
		t.Fatalf("dst 应 bump 到 v2: v=%d err=%v", v, err)
	}
	if err := st.SaveVersion("dst", 2, specV2, "", ""); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestExportVersioned_Req540(t *testing.T) {
	srv, _ := pinRestFixture(t)
	// 版本化导出：?version=1 取 v1 快照（概念=需求）
	resp, err := http.Get(srv.URL + "/api/ontologies/dst/export?format=turtle&version=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("v1 导出期望 200，实际 %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	ttl := string(b)
	if !strings.Contains(ttl, "urn:o:dst:concept:需求>;\n") && !strings.Contains(ttl, "urn:o:dst:concept:需求") {
		t.Fatalf("v1 导出应含 v1 概念「需求」:\n%s", ttl)
	}
	if strings.Contains(ttl, "concept:需求项") {
		t.Fatalf("v1 导出不应含头版本概念「需求项」:\n%s", ttl)
	}
	if got := resp.Header.Get("X-Onto-Version"); got != "1" {
		t.Fatalf("X-Onto-Version 期望 1，实际 %q", got)
	}
	// 头版本导出（无 version 参数）行为不变：v2 概念
	resp2, err := http.Get(srv.URL + "/api/ontologies/dst/export?format=turtle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(b2), "concept:需求项") {
		t.Fatalf("头版本导出应含「需求项」:\n%s", b2)
	}
	// 快照不存在 → 404（pin 钉死语义：显式失败不回落）
	resp3, err := http.Get(srv.URL + "/api/ontologies/dst/export?format=turtle&version=9")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("超前版本期望 404，实际 %d", resp3.StatusCode)
	}
	// 非法 version 参数 → 400
	resp4, err := http.Get(srv.URL + "/api/ontologies/dst/export?format=turtle&version=abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 version 期望 400，实际 %d", resp4.StatusCode)
	}
}

func TestUpdateExternalRefPinEndpoint_Req540(t *testing.T) {
	srv, st := pinRestFixture(t)
	ref, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "需求", RefKind: "import"})
	if err != nil {
		t.Fatal(err)
	}
	// 刷新 pin → v2
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ontologies/src/external-refs/"+ref.RefID+"/pin", strings.NewReader(`{"pin_version":2}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("刷新 pin 期望 200，实际 %d body=%s", resp.StatusCode, b)
	}
	refs, _ := st.ListExternalRefs("src")
	if len(refs) != 1 || refs[0].PinVersion != 2 {
		t.Fatalf("pin 刷新应落库: %+v", refs)
	}
	// 清除 pin（0=浮动头版本）
	req2, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ontologies/src/external-refs/"+ref.RefID+"/pin", strings.NewReader(`{"pin_version":0}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("清除 pin 期望 200，实际 %d", resp2.StatusCode)
	}
	refs, _ = st.ListExternalRefs("src")
	if refs[0].PinVersion != 0 {
		t.Fatalf("pin 应已清除: %d", refs[0].PinVersion)
	}
	// 超前版本 → 400（人话错误）
	req3, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ontologies/src/external-refs/"+ref.RefID+"/pin", strings.NewReader(`{"pin_version":9}`))
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound && resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("超前 pin 期望 4xx，实际 %d", resp3.StatusCode)
	}
}
