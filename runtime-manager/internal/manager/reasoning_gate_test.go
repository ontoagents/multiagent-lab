package manager

// REQ-329 推理形态计数对账单测：fuseki 推理方案（InfModel OWL_FB 规则推理）在引擎侧物化推断
// 三元组，默认图计数恒 ≥ 源计数——等值闸结构性失配（真机实锤 源1342/引擎3595）。本轮起推理
// 形态改下限闸：got < src 仍失配中止（防截断装载），≥ 判 pass 并留痕 engine_triples。
// 沿 graph_gate_test 先例：httptest 假构建平面/假引擎+临时 sqlite，零外部依赖。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

// reasoningFixture 推理方案夹具：引擎对默认图（无 GRAPH）计数查询返回 got。
func reasoningFixture(t *testing.T, source, got int) *Manager {
	t.Helper()
	eng := &giStubEngine{sparqlResp: func(q string) string {
		if strings.Contains(q, "GRAPH <") {
			t.Errorf("推理方案对账应走默认图（legacy 口径），不应使用 GRAPH 查询: %s", q)
		}
		return fmt.Sprintf(`{"results":{"bindings":[{"c":{"value":"%d"}}]}}`, got)
	}}
	engSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(eng.sparqlResp("")))
	}))
	t.Cleanup(engSrv.Close)
	eng.endpoint = engSrv.URL

	build := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/export") {
			w.Header().Set("X-Onto-Triples", fmt.Sprint(source))
			_, _ = w.Write([]byte("<urn:s> <urn:p> <urn:o> ."))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/external-refs") {
			// REQ-480 闭包数据源——推理闸测试本体无引用登记（空清单，闭包零扩展）
			_, _ = w.Write([]byte(`{"refs":[],"dangling":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(build.Close)

	m := New(newStore(t), build.URL, t.TempDir(), t.TempDir())
	_ = m.Store.SetConfig(store.RuntimeConfig{ExecutionMethod: "native"})
	m.RegisterEngine("fuseki", eng)
	return m
}

func newReasoningProfile(id string) *store.Profile {
	return &store.Profile{ID: id, Name: "Fuseki 推理方案", Engine: "fuseki", OntologyIDs: []string{"ont_r"}, Config: `{"reasoning":true}`}
}

// TestStartReasoningGateInferencePasses 推断物化（引擎 > 源）应放行并留痕 engine_triples；
// 对账查询走默认图（无 GRAPH）。
func TestStartReasoningGateInferencePasses(t *testing.T) {
	m := reasoningFixture(t, 1342, 3595)
	p := newReasoningProfile("rt_r1")
	if err := m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background(), p.ID); err != nil {
		t.Fatalf("推理形态引擎计数≥源应放行: %v", err)
	}
	got, _ := m.Store.Get(p.ID)
	if got.Status != "running" {
		t.Fatalf("期望 running，得到 %s（%s）", got.Status, got.LastError)
	}
	var ls map[string]map[string]any
	if err := json.Unmarshal([]byte(got.LoadedStatus), &ls); err != nil {
		t.Fatal(err)
	}
	if ls["ont_r"]["gate"] != "pass" {
		t.Fatalf("期望 gate=pass，得到 %v", ls["ont_r"]["gate"])
	}
	if ls["ont_r"]["engine_triples"] != float64(3595) {
		t.Fatalf("推断物化应留痕 engine_triples=3595，得到 %v", ls["ont_r"]["engine_triples"])
	}
	if ls["ont_r"]["source_triple_count"] != float64(1342) {
		t.Fatalf("源计数快照不应被改写: %v", ls["ont_r"]["source_triple_count"])
	}
}

// TestStartReasoningGateStillAbortsOnShortfall 引擎计数 < 源（截断/部分装载）仍硬闸中止——
// 下限闸不放松防丢失语义。
func TestStartReasoningGateStillAbortsOnShortfall(t *testing.T) {
	m := reasoningFixture(t, 1342, 1000)
	p := newReasoningProfile("rt_r2")
	if err := m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	err := m.Start(context.Background(), p.ID)
	if err == nil || !strings.Contains(err.Error(), "计数硬闸") {
		t.Fatalf("引擎计数<源应仍报计数硬闸，得到 %v", err)
	}
	got, _ := m.Store.Get(p.ID)
	if got.Status != "error" {
		t.Fatalf("失配应落 error 态，得到 %s", got.Status)
	}
}
