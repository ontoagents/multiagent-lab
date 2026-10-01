package manager

// REQ-239/M65 装载发布状态快照测试：FetchMeta 解析（version/status/version_name 一次取回，
// 旧构建平面无 status 字段兜底 draft）+ SetLoadedStatus 存储往返。Start 全链真机验证见冒烟。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

var storeProfileFixture = store.Profile{ID: "rt_s", Name: "状态快照方案", Engine: "oxigraph", OntologyIDs: []string{"onto_a"}, Config: "{}"}

func TestFetchMetaParsesPublishState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ontologies/onto_a" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"id":"onto_a","version":3,"status":"published","version_name":"v3-k8s-baseline"}`))
	}))
	t.Cleanup(srv.Close)
	m := New(newStore(t), srv.URL, t.TempDir(), t.TempDir())
	v, st, vn, err := m.FetchMeta("onto_a")
	if err != nil {
		t.Fatalf("FetchMeta 失败: %v", err)
	}
	if v != 3 || st != "published" || vn != "v3-k8s-baseline" {
		t.Fatalf("解析期望 3/published/v3-k8s-baseline，实际 %d/%s/%s", v, st, vn)
	}
}

func TestFetchMetaDefaultsDraftWithoutStatusField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id":"onto_old","version":1}`)) // 旧构建平面：无发布态字段
	}))
	t.Cleanup(srv.Close)
	m := New(newStore(t), srv.URL, t.TempDir(), t.TempDir())
	_, st, _, err := m.FetchMeta("onto_old")
	if err != nil {
		t.Fatalf("FetchMeta 失败: %v", err)
	}
	if st != "draft" {
		t.Fatalf("无 status 字段应兜底 draft，实际 %q", st)
	}
}

func TestSetLoadedStatusRoundTrip(t *testing.T) {
	st := newStore(t)
	if err := st.Create(&storeProfileFixture); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLoadedStatus("rt_s", `{"onto_a":{"status":"draft","version_name":""}}`); err != nil {
		t.Fatalf("SetLoadedStatus 失败: %v", err)
	}
	p, err := st.Get("rt_s")
	if err != nil {
		t.Fatal(err)
	}
	if p.LoadedStatus == "" {
		t.Fatal("loaded_status 应落库可读")
	}
}
