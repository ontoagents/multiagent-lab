package companion

// REQ-309/M113 伴生宿主引擎池化单测：桩运行平面+构建平面，验证——
// ①两空容器 Ensure 进同一共享池（池单例+成员 append）
// ②有 spec 本体保持独立宿主（不进池）
// ③池 stopped 时 Ensure 拉起+append
// ④RemoveFromPools 删除摘除。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakePlane struct {
	mu        sync.Mutex
	profiles  map[string]*map[string]any
	seq       int
	specCount map[string]int // ontologyID → 五层元素总数
}

func (f *fakePlane) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/runtime-profiles":
		out := []map[string]any{}
		for _, p := range f.profiles {
			out = append(out, *p)
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == "POST" && r.URL.Path == "/api/runtime-profiles":
		f.seq++
		id := "rt_pool_test_" + string(rune('0'+f.seq))
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		p := map[string]any{"id": id, "name": body["name"], "engine": "oxigraph", "status": "stopped",
			"ontology_ids": body["ontology_ids"], "port": float64(9300 + f.seq)}
		f.profiles[id] = &p
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == "PUT" && len(r.URL.Path) > len("/api/runtime-profiles/"):
		id := r.URL.Path[len("/api/runtime-profiles/"):]
		p, ok := f.profiles[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if oids, ok := body["ontology_ids"]; ok {
			(*p)["ontology_ids"] = oids
		}
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == "POST" && len(r.URL.Path) > len("/api/runtime-profiles/") && r.URL.Path[len(r.URL.Path)-6:] == "/start":
		id := r.URL.Path[len("/api/runtime-profiles/") : len(r.URL.Path)-len("/start")]
		p, ok := f.profiles[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		(*p)["status"] = "running"
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == "GET" && len(r.URL.Path) > len("/api/ontologies/") && r.URL.Path[len(r.URL.Path)-5:] == "/spec":
		oid := r.URL.Path[len("/api/ontologies/") : len(r.URL.Path)-len("/spec")]
		n := f.specCount[oid]
		empty := func(m string) json.RawMessage { return json.RawMessage("[]") }
		out := map[string]any{}
		for _, k := range []string{"concepts", "relations", "instances", "data_properties", "axioms"} {
			arr := []any{}
			for i := 0; i < n; i++ {
				arr = append(arr, empty(k))
			}
			out[k] = arr
		}
		_ = json.NewEncoder(w).Encode(out)
	default:
		w.WriteHeader(404)
	}
}

func newPoolTestEngine(t *testing.T, specCount map[string]int) *PlanEngines {
	t.Helper()
	f := &fakePlane{profiles: map[string]*map[string]any{}, specCount: specCount}
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(srv.Close)
	return &PlanEngines{RuntimeURL: srv.URL, BuildURL: srv.URL, HTTP: &http.Client{Timeout: 5 * time.Second},
		eps: map[string]string{}, hostIDs: map[string]string{}, names: map[string]string{},
		locks: map[string]*sync.Mutex{}, startFailAt: map[string]time.Time{}, startFailErr: map[string]string{}}
}

// 两空容器先后 Ensure → 同一共享池承载（池单例+两成员）。
func TestEnsureHostPlanPoolSharing(t *testing.T) {
	pe := newPoolTestEngine(t, map[string]int{"ont_a": 0, "ont_b": 0})
	baseA, err := pe.EnsureHostPlan(context.Background(), "ont_a")
	if err != nil {
		t.Fatal(err)
	}
	baseB, err := pe.EnsureHostPlan(context.Background(), "ont_b")
	if err != nil {
		t.Fatal(err)
	}
	if baseA != baseB {
		t.Fatalf("两空容器应共享同一池基址: %s vs %s", baseA, baseB)
	}
	profiles, err := pe.listProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pools := 0
	members := 0
	for _, p := range profiles {
		if p.Name == CompanionPoolName {
			pools++
			members = len(p.OntologyIDs)
		}
	}
	if pools != 1 {
		t.Fatalf("池应只建一个，得 %d", pools)
	}
	if members != 2 {
		t.Fatalf("池成员应为 2，得 %d", members)
	}
}

// 有 spec 的 regular 本体 → 独立宿主（不进池）。
func TestEnsureHostPlanSpecOntologyDedicated(t *testing.T) {
	pe := newPoolTestEngine(t, map[string]int{"ont_a": 0, "ont_regular": 12})
	if _, err := pe.EnsureHostPlan(context.Background(), "ont_a"); err != nil {
		t.Fatal(err)
	}
	base, err := pe.EnsureHostPlan(context.Background(), "ont_regular")
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := pe.listProfiles(context.Background())
	for _, p := range profiles {
		if p.Name == CompanionPoolName && containsOID(p.OntologyIDs, "ont_regular") {
			t.Fatalf("有 spec 本体不应进池")
		}
	}
	if base == "" {
		t.Fatal("独立宿主基址为空")
	}
}

// RemoveFromPools：删除本体从池成员摘除。
func TestRemoveFromPools(t *testing.T) {
	pe := newPoolTestEngine(t, map[string]int{"ont_a": 0, "ont_b": 0})
	_, _ = pe.EnsureHostPlan(context.Background(), "ont_a")
	_, _ = pe.EnsureHostPlan(context.Background(), "ont_b")
	pe.RemoveFromPools(context.Background(), "ont_a")
	profiles, _ := pe.listProfiles(context.Background())
	for _, p := range profiles {
		if p.Name == CompanionPoolName {
			if containsOID(p.OntologyIDs, "ont_a") {
				t.Fatal("ont_a 应已从池摘除")
			}
			if len(p.OntologyIDs) != 1 {
				t.Fatalf("池成员应剩 1，得 %v", p.OntologyIDs)
			}
		}
	}
}
