package rest

// REQ-239/M65 版本发布状态机测试：发布/撤回/回滚链路 + 保存回 draft + 候选采纳自动 Published。
// 临时 SQLite + httptest，零外部依赖（沿 rest_test.go 先例）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

func newPublishTestServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "publish_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("ontest", "测试本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	sp := pkgspec.Spec{Name: "测试本体", Concepts: []pkgspec.Concept{{Name: "概念A"}}}
	bts, _ := json.Marshal(sp)
	if err := st.PutArtifact("ontest", "spec_json", string(bts), true); err != nil {
		t.Fatalf("写 spec 失败: %v", err)
	}
	if _, err := st.BumpVersion("ontest"); err != nil { // create 已是 v1，bump 到 v2 拉开与初始版本差
		t.Fatalf("bump 失败: %v", err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func postJSON(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestPublishRoundTripAndDefaultName(t *testing.T) {
	srv, st := newPublishTestServer(t)
	// 默认命名 v{N}
	resp, out := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	if out["status"] != "published" || out["version_name"] != "v2" {
		t.Fatalf("默认命名期望 published/v2，实际 %v/%v", out["status"], out["version_name"])
	}
	// 显式命名覆盖
	resp, out = postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{"version_name":"k8s-baseline"}`)
	if resp.StatusCode != http.StatusOK || out["version_name"] != "k8s-baseline" {
		t.Fatalf("显式命名失败: %d %v", resp.StatusCode, out["version_name"])
	}
	// 撤回回 draft 清命名（version_name omitempty：空值省略键）
	resp, out = postJSON(t, srv.URL+"/api/ontologies/ontest/unpublish", `{}`)
	if resp.StatusCode != http.StatusOK || out["status"] != "draft" || out["version_name"] != nil {
		t.Fatalf("unpublish 期望 draft/空命名，实际 %v/%v", out["status"], out["version_name"])
	}
	if _, err := st.GetOntology("ontest"); err != nil {
		t.Fatalf("回读失败: %v", err)
	}
}

func TestSaveSpecDemotesPublishedOntology(t *testing.T) {
	srv, st := newPublishTestServer(t)
	if _, err := st.Publish("ontest", ""); err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	// 已发布本体保存 spec → 自动回 draft（发布即快照终态，继续编辑即漂移）
	sp := pkgspec.Spec{Name: "测试本体", Concepts: []pkgspec.Concept{{Name: "概念B"}}}
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ontologies/ontest/spec", strings.NewReader(string(mustJSON(t, sp))))
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("saveSpec 失败: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("saveSpec 期望 200，实际 %d", putResp.StatusCode)
	}
	o, _ := st.GetOntology("ontest")
	if o.Status != "draft" || o.VersionName != "" {
		t.Fatalf("保存后期望 draft/空命名，实际 %s/%s", o.Status, o.VersionName)
	}
}

func TestRestoreVersionCreatesNewDraftVersion(t *testing.T) {
	srv, st := newPublishTestServer(t)
	// 写入历史版本 1（不同内容）
	if err := st.SaveVersion("ontest", 1, `{"name":"历史v1"}`, "", ""); err != nil {
		t.Fatalf("写历史版本失败: %v", err)
	}
	if _, err := st.Publish("ontest", "before-restore"); err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	resp, out := postJSON(t, srv.URL+"/api/ontologies/ontest/versions/1/restore", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	if out["restored_from"] != float64(1) {
		t.Fatalf("restored_from 期望 1，实际 %v", out["restored_from"])
	}
	o, _ := st.GetOntology("ontest")
	if o.Status != "draft" || o.VersionName != "" {
		t.Fatalf("恢复后期望 draft/空命名，实际 %s/%s", o.Status, o.VersionName)
	}
	cur, _ := st.GetVersionSpec("ontest", o.Version)
	if cur != `{"name":"历史v1"}` {
		t.Fatalf("新版本内容应为历史快照，实际 %q", cur)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	return b
}
