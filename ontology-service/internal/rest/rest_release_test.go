package rest

// REQ-541/M348 发布快照 REST 测试：发布冻结→按 tag 取工件→?version= 命中冻结快照（REQ-540 pin
// 联动）→重发布同 digest（验收锚点）→内容漂移 409。沿 rest_publish_test.go 先例。

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// newReleaseRestServer 与 newPublishTestServer 同构，另落版本快照行（版本列表合并发布 tag 需要：
// 真实保存动线 SaveVersion 随 bump 成对，此处对齐该不变量）。
func newReleaseRestServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "release_rest.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("ontest", "测试本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	spec := `{"name":"测试本体","concepts":[{"name":"概念A"}]}`
	if err := st.PutArtifact("ontest", "spec_json", spec, true); err != nil {
		t.Fatalf("写 spec 失败: %v", err)
	}
	if err := st.SaveVersion("ontest", 1, spec, "", ""); err != nil {
		t.Fatalf("落 v1 快照失败: %v", err)
	}
	if _, err := st.BumpVersion("ontest"); err != nil { // → v2 拉开与初始版本差
		t.Fatalf("bump 失败: %v", err)
	}
	if err := st.SaveVersion("ontest", 2, spec, "", ""); err != nil {
		t.Fatalf("落 v2 快照失败: %v", err)
	}
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestPublishEndpointFreezesRelease_Req541(t *testing.T) {
	srv, _ := newReleaseRestServer(t)
	resp, out := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{"version_name":"baseline"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	rel, ok := out["release"].(map[string]any)
	if !ok {
		t.Fatalf("发布响应应携带 release 元信息: %v", out)
	}
	specSum, _ := rel["spec_sha256"].(string)
	ttlSum, _ := rel["ttl_sha256"].(string)
	if len(specSum) != 64 || len(ttlSum) != 64 {
		t.Fatalf("digest 应为 sha256 hex（64 位），实际 %q / %q", specSum, ttlSum)
	}
	if rel["version_name"] != "baseline" || rel["version"].(float64) != 2 {
		t.Fatalf("release 元信息期望 v2/baseline，实际 %v", rel)
	}
	// 清单端点：依赖账本 manifest 随行
	resp2, err := http.Get(srv.URL + "/api/ontologies/ontest/releases")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body), `"manifest_json"`) || !strings.Contains(string(body), specSum) {
		t.Fatalf("releases 清单应含 manifest 与 digest:\n%s", body)
	}
	// 单取端点（元信息+账本，无内容 blob）
	resp3, err := http.Get(srv.URL + "/api/ontologies/ontest/releases/2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	b3, _ := io.ReadAll(resp3.Body)
	if strings.Contains(string(b3), `"spec_json":"`) || strings.Contains(string(b3), `"ttl":"`) {
		t.Fatalf("单取端点不应吐内容 blob:\n%s", b3)
	}
	if !strings.Contains(string(b3), `"ontology_id":"ontest"`) || !strings.Contains(string(b3), `\"dependencies\":[]`) {
		t.Fatalf("单取端点应含账本（本体标识+manifest 依赖账本）:\n%s", b3)
	}
	// 工件端点：TTL 冻结内容 + digest 头（内容寻址可校验）
	resp4, err := http.Get(srv.URL + "/api/ontologies/ontest/releases/2/artifact/ttl")
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	b4, _ := io.ReadAll(resp4.Body)
	if got := resp4.Header.Get("X-Release-Digest"); got != ttlSum {
		t.Fatalf("TTL 工件 X-Release-Digest 期望冻结 digest，实际 %q", got)
	}
	if sum256InTest(string(b4)) != ttlSum {
		t.Fatal("TTL 工件内容 sha256 应与冻结 digest 一致")
	}
	// 工件端点：spec_json + manifest
	resp5, err5 := http.Get(srv.URL + "/api/ontologies/ontest/releases/2/artifact/spec_json")
	if err5 != nil {
		t.Fatal(err5)
	}
	defer resp5.Body.Close()
	b5, _ := io.ReadAll(resp5.Body)
	if resp5.Header.Get("X-Release-Digest") != specSum || sum256InTest(string(b5)) != specSum {
		t.Fatal("spec_json 工件应携带且内容匹配冻结 digest")
	}
	resp6, err6 := http.Get(srv.URL + "/api/ontologies/ontest/releases/2/artifact/manifest")
	if err6 != nil {
		t.Fatal(err6)
	}
	defer resp6.Body.Close()
	b6, _ := io.ReadAll(resp6.Body)
	if !strings.Contains(string(b6), `"ontology_id":"ontest"`) || !strings.Contains(string(b6), `"dependencies":[]`) {
		t.Fatalf("manifest 工件应为依赖账本 JSON:\n%s", b6)
	}
	// 404/400 口径
	if r, _ := http.Get(srv.URL + "/api/ontologies/ontest/releases/9"); r.StatusCode != http.StatusNotFound {
		t.Fatalf("未发布版本取快照期望 404，实际 %d", r.StatusCode)
	}
	if r, _ := http.Get(srv.URL + "/api/ontologies/ontest/releases/2/artifact/bogus"); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知工件名期望 400，实际 %d", r.StatusCode)
	}
}

func TestExportVersionHitsFrozenRelease_Req541(t *testing.T) {
	srv, _ := newReleaseRestServer(t)
	resp, out := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	rel := out["release"].(map[string]any)
	ttlSum, _ := rel["ttl_sha256"].(string)
	// REQ-540 pin 联动验收：?version=2（pin 解析点）命中冻结发布快照直出
	resp2, err := http.Get(srv.URL + "/api/ontologies/ontest/export?format=turtle&version=2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	if resp2.Header.Get("X-Onto-Source") != "release_snapshot" {
		t.Fatalf("已发布版本导出应直出冻结快照，X-Onto-Source=%q", resp2.Header.Get("X-Onto-Source"))
	}
	if resp2.Header.Get("X-Release-Digest") != ttlSum {
		t.Fatalf("导出应携带冻结 digest，实际 %q", resp2.Header.Get("X-Release-Digest"))
	}
	if sum256InTest(string(b2)) != ttlSum {
		t.Fatal("版本化导出内容应与冻结快照逐字节一致")
	}
	// 版本列表并入发布 tag
	resp3, err := http.Get(srv.URL + "/api/ontologies/ontest/versions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	b3, _ := io.ReadAll(resp3.Body)
	if !strings.Contains(string(b3), `"release"`) || !strings.Contains(string(b3), ttlSum) {
		t.Fatalf("版本列表应并入发布快照 tag:\n%s", b3)
	}
	// 未发布版本导出维持既有重导出路径（v1 无发布快照 → spec_export）
	resp4, err := http.Get(srv.URL + "/api/ontologies/ontest/export?format=turtle&version=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.Header.Get("X-Onto-Source") != "spec_export" {
		t.Fatalf("未发布版本导出应维持重导出路径，X-Onto-Source=%q", resp4.Header.Get("X-Onto-Source"))
	}
}

func TestRepublishDigestStableAndDriftRejected_Req541(t *testing.T) {
	srv, _ := newReleaseRestServer(t)
	resp, out := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish 期望 200，实际 %d: %v", resp.StatusCode, out)
	}
	first := out["release"].(map[string]any)
	// 同版本重发布同内容：digest 一致（验收锚点）
	resp2, out2 := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{}`)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("重发布期望 200（幂等），实际 %d: %v", resp2.StatusCode, out2)
	}
	second := out2["release"].(map[string]any)
	if first["ttl_sha256"] != second["ttl_sha256"] || first["spec_sha256"] != second["spec_sha256"] {
		t.Fatalf("同内容重发布 digest 必须一致: %v vs %v", first, second)
	}
	if first["published_at"] != second["published_at"] {
		t.Fatal("重发布不应刷新 published_at（不可变）")
	}
	// 内容漂移（引用登记改变 TTL 物化面、版本号未动）：重发布 409 拒绝
	resp3, out3 := postJSON(t, srv.URL+"/api/ontologies/ontest/external-refs",
		`{"from_concept":"(ontology)","to_concept":"http://example.org/ext","ref_kind":"import","justification":"import_statement"}`)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("登记外引应成功，实际 %d: %v", resp3.StatusCode, out3)
	}
	resp4, out4 := postJSON(t, srv.URL+"/api/ontologies/ontest/publish", `{}`)
	if resp4.StatusCode != http.StatusConflict {
		t.Fatalf("内容漂移重发布期望 409，实际 %d: %v", resp4.StatusCode, out4)
	}
	if !strings.Contains(out4["error"].(string), "FP-004") {
		t.Fatalf("409 消息应指明发布冻结纪律: %v", out4)
	}
	// 拒绝后冻结工件原样可取（digest 不变）
	r5, err := http.Get(srv.URL + "/api/ontologies/ontest/releases/2/artifact/ttl")
	if err != nil {
		t.Fatal(err)
	}
	defer r5.Body.Close()
	b5, _ := io.ReadAll(r5.Body)
	if r5.Header.Get("X-Release-Digest") != first["ttl_sha256"].(string) || sum256InTest(string(b5)) != first["ttl_sha256"].(string) {
		t.Fatal("漂移拒绝后冻结工件必须原样保留")
	}
}

// sum256InTest 内容寻址校验用 sha256 hex（与 repo.sha256Hex 同算法）。
func sum256InTest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
