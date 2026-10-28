package manager

// REQ-320/M122 per-graph 装载与逐图对账单测：
//   ①GraphIRI 派生稳定与非法字符消毒；
//   ②countGraphTriples 查询形状（GRAPH <urn:onto:{id}> 包裹）；
//   ③companionProfileName 判定（REQ-309 命名契约 → legacy 口径）；
//   ④Start 路由：stub GraphIsolationRuntime 引擎捕获 opts——regular 方案逐图 gate pass/失配中止、
//     伴生命名方案保持默认图口径（对账查询不含 GRAPH）、无计数头 gate=skipped 保持（REQ-318）。
// 沿 gate_test/reconcile_test 先例：httptest 假构建平面/假引擎+临时 sqlite，零外部依赖。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine/oxigraph"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

func TestGraphIRI(t *testing.T) {
	if got := engine.GraphIRI("ont_a-1.2"); got != "urn:onto:ont_a-1.2" {
		t.Fatalf("合法字符应原样保留，得到 %q", got)
	}
	// 非法 IRI 字符消毒（中文/空格/冒号），且派生确定性（装载与对账两侧共用）
	a := engine.GraphIRI("本体 A:B")
	b := engine.GraphIRI("本体 A:B")
	idPart := strings.TrimPrefix(a, "urn:onto:")
	if a != b || !strings.HasPrefix(a, "urn:onto:") || idPart == "" || strings.ContainsAny(idPart, " :/") {
		t.Fatalf("派生应确定且 ID 段为合法 IRI 片段，得到 %q", a)
	}
}

func TestCountGraphTriples(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		q := string(body[:n])
		if !strings.Contains(q, "GRAPH <urn:onto:o1>") {
			t.Errorf("计数查询应按命名图包裹，实际: %s", q)
		}
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(`{"results":{"bindings":[{"c":{"value":"7"}}]}}`))
	}))
	defer svc.Close()
	n, err := countGraphTriples(context.Background(), svc.URL, "urn:onto:o1")
	if err != nil || n != 7 {
		t.Fatalf("期望逐图计数 7，实际 n=%d err=%v", n, err)
	}
}

func TestCompanionProfileName(t *testing.T) {
	for name, want := range map[string]bool{
		"伴生·共享引擎": true, "伴生·K8s 运维": true, "生产方案": false, "伴生": false, "": false,
	} {
		if got := companionProfileName(name); got != want {
			t.Fatalf("companionProfileName(%q)=%v 期望 %v", name, got, want)
		}
	}
}

// giStubEngine 记录最近一次 StartWithGraphIsolation 的 opts（路由断言载体）。
type giStubEngine struct {
	endpoint   string
	gotOpts    engine.GraphIsolationOpts
	sparqlResp func(query string) string // 按 SPARQL 文本回计数 JSON
}

func (e *giStubEngine) Start(_ context.Context, _ string, _ int, _ map[string]string) (*engine.Process, error) {
	return e.start(nil)
}

func (e *giStubEngine) StartWithGraphIsolation(_ context.Context, _ string, _ int, _ map[string]string, opts engine.GraphIsolationOpts) (*engine.Process, error) {
	return e.start(&opts)
}

func (e *giStubEngine) start(opts *engine.GraphIsolationOpts) (*engine.Process, error) {
	if opts != nil {
		e.gotOpts = *opts
	}
	return engine.NewProcess(e.endpoint, 0, func() error { return nil }), nil
}

func (e *giStubEngine) HealthCheck(_ context.Context, _ string) error { return nil }

// startFixture 组装 Manager 全链夹具：构建平面（export 计数头/meta）+ 引擎 SPARQL 桩。
type startFixture struct {
	m         *Manager
	sparqlReq *[]string // 引擎收到的查询文本（对账形状断言）
}

func startFixtureSetup(t *testing.T, exports map[string]int) *startFixture {
	t.Helper()
	// 引擎桩：POST SPARQL → 按查询文本返回对应命名图计数（无 GRAPH 查询回默认图计数 0）
	var queries []string
	eng := &giStubEngine{sparqlResp: func(q string) string {
		for oid, n := range exports {
			if strings.Contains(q, engine.GraphIRI(oid)+"> {") {
				return fmt.Sprintf(`{"results":{"bindings":[{"c":{"value":"%d"}}]}}`, n)
			}
		}
		return `{"results":{"bindings":[{"c":{"value":"0"}}]}}`
	}}
	engSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		q := string(buf)
		queries = append(queries, q)
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(eng.sparqlResp(q)))
	}))
	t.Cleanup(engSrv.Close)
	eng.endpoint = engSrv.URL

	build := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET /api/ontologies/{id}/export?format=turtle（计数头）与 /meta（尽力而为，404 容忍）
		if strings.Contains(r.URL.Path, "/export") {
			parts := strings.Split(r.URL.Path, "/")
			oid := parts[len(parts)-2] // .../ontologies/{id}/export 倒数第二段为本体 ID
			if n, ok := exports[oid]; ok {
				w.Header().Set("X-Onto-Triples", fmt.Sprint(n))
			}
			_, _ = w.Write([]byte("<urn:s> <urn:p> <urn:o> ."))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/external-refs") {
			// REQ-480 闭包数据源——门禁测试本体无引用登记（空清单，闭包零扩展）
			_, _ = w.Write([]byte(`{"refs":[],"dangling":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(build.Close)

	st := newStore(t)
	f := &startFixture{m: New(st, build.URL, t.TempDir(), t.TempDir()), sparqlReq: &queries}
	// 迁移默认 docker；stub 引擎非 oxigraph.Runtime，切 native 走 GraphIsolation 路由
	_ = f.m.Store.SetConfig(store.RuntimeConfig{ExecutionMethod: "native"})
	f.m.RegisterEngine("oxigraph", eng)
	return f
}

func TestStartPerGraphGatePass(t *testing.T) {
	f := startFixtureSetup(t, map[string]int{"ont_a": 3, "ont_b": 5})
	p := &store.Profile{ID: "rt_multi", Name: "多本体方案", Engine: "oxigraph", OntologyIDs: []string{"ont_a", "ont_b"}, Config: "{}"}
	if err := f.m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Start(context.Background(), p.ID); err != nil {
		t.Fatalf("逐图对账应通过: %v", err)
	}
	got, _ := f.m.Store.Get(p.ID)
	if got.Status != "running" {
		t.Fatalf("期望 running，得到 %s（%s）", got.Status, got.LastError)
	}
	var ls map[string]map[string]any
	if err := json.Unmarshal([]byte(got.LoadedStatus), &ls); err != nil {
		t.Fatal(err)
	}
	for _, oid := range []string{"ont_a", "ont_b"} {
		if ls[oid]["gate"] != "pass" {
			t.Fatalf("%s 期望 gate=pass，得到 %v", oid, ls[oid]["gate"])
		}
		if ls[oid]["triple_count"] != float64(map[string]int{"ont_a": 3, "ont_b": 5}[oid]) {
			t.Fatalf("%s 三元组计数快照不符: %v", oid, ls[oid]["triple_count"])
		}
	}
	if f.m.Engines["oxigraph"].(*giStubEngine).gotOpts.CompanionMode {
		t.Fatal("regular 方案不应进入伴生模式")
	}
}

func TestStartPerGraphGateMismatchAborts(t *testing.T) {
	f := startFixtureSetup(t, map[string]int{"ont_a": 3, "ont_b": 5})
	// ont_b 引擎侧返回失配计数（原地改原引擎的应答闭包——handler 闭包持有同一指针）
	eng := f.m.Engines["oxigraph"].(*giStubEngine)
	eng.sparqlResp = func(q string) string {
		n := 3
		if strings.Contains(q, engine.GraphIRI("ont_b")+"> {") {
			n = 4 // ≠ 源 5
		}
		return fmt.Sprintf(`{"results":{"bindings":[{"c":{"value":"%d"}}]}}`, n)
	}
	p := &store.Profile{ID: "rt_bad", Name: "失配方案", Engine: "oxigraph", OntologyIDs: []string{"ont_a", "ont_b"}, Config: "{}"}
	if err := f.m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	err := f.m.Start(context.Background(), p.ID)
	if err == nil || !strings.Contains(err.Error(), "计数硬闸") {
		t.Fatalf("期望失配中止报计数硬闸，得到 %v", err)
	}
	got, _ := f.m.Store.Get(p.ID)
	if got.Status != "error" {
		t.Fatalf("失配应落 error 态，得到 %s", got.Status)
	}
}

func TestStartCompanionKeepsDefaultGraphGate(t *testing.T) {
	f := startFixtureSetup(t, map[string]int{"ont_c": 7})
	// 引擎侧仅应收到默认图计数查询（伴生模式不开逐图对账）
	eng := f.m.Engines["oxigraph"].(*giStubEngine)
	eng.sparqlResp = func(q string) string {
		if strings.Contains(q, "GRAPH <") {
			t.Errorf("伴生方案对账不应使用 GRAPH 查询: %s", q)
		}
		return `{"results":{"bindings":[{"c":{"value":"7"}}]}}`
	}
	p := &store.Profile{ID: "rt_pool", Name: "伴生·共享引擎", Engine: "oxigraph", OntologyIDs: []string{"ont_c"}, Config: "{}"}
	if err := f.m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Start(context.Background(), p.ID); err != nil {
		t.Fatalf("伴生方案默认图口径应通过: %v", err)
	}
	if !eng.gotOpts.CompanionMode {
		t.Fatal("伴生命名方案应进入伴生模式（legacy 装载路径不动）")
	}
}

func TestStartUnknownCountGateSkipped(t *testing.T) {
	f := startFixtureSetup(t, map[string]int{}) // export 无计数头（original turtle/REQ-318 拼接还原）
	p := &store.Profile{ID: "rt_raw", Name: "原始导入方案", Engine: "oxigraph", OntologyIDs: []string{"ont_raw"}, Config: "{}"}
	if err := f.m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Start(context.Background(), p.ID); err != nil {
		t.Fatalf("无计数头应跳过硬闸正常启动: %v", err)
	}
	got, _ := f.m.Store.Get(p.ID)
	var ls map[string]map[string]any
	if err := json.Unmarshal([]byte(got.LoadedStatus), &ls); err != nil {
		t.Fatal(err)
	}
	if ls["ont_raw"]["gate"] != "skipped" {
		t.Fatalf("期望 gate=skipped，得到 %v", ls["ont_raw"]["gate"])
	}
}

// TestStartDockerUnavailableDegradesNative REQ-325 顺修：默认执行方式 docker + docker 不可用
// 时，非 oxigraph 引擎（fuseki）也应降级进程内 native——原实现降级 break 仅包住 oxigraph 分支，
// fresh 环境（迁移默认 docker、无 docker）fuseki 方案恒报「docker 仅支持 oxigraph」无法首启。
func TestStartDockerUnavailableDegradesNative(t *testing.T) {
	if oxigraph.DockerAvailable() {
		t.Skip("本机 docker 可用，降级路径不可达（降级语义由无 docker 环境承载）")
	}
	f := startFixtureSetup(t, map[string]int{"ont_a": 4})
	// 恢复迁移默认 docker（startFixtureSetup 为走 GraphIsolation 路由显式切了 native）
	if err := f.m.Store.SetConfig(store.RuntimeConfig{ExecutionMethod: "docker"}); err != nil {
		t.Fatal(err)
	}
	p := &store.Profile{ID: "rt_dlg", Name: "fuseki 降级方案", Engine: "fuseki", OntologyIDs: []string{"ont_a"}, Config: "{}"}
	if err := f.m.Store.Create(p); err != nil {
		t.Fatal(err)
	}
	// 同一 stub 桩以 fuseki 名补注册（startFixtureSetup 只注册了 oxigraph 名）
	f.m.RegisterEngine("fuseki", f.m.Engines["oxigraph"])
	if err := f.m.Start(context.Background(), p.ID); err != nil {
		t.Fatalf("docker 不可用时 fuseki 应降级 native 启动: %v", err)
	}
	got, _ := f.m.Store.Get(p.ID)
	if got.Status != "running" {
		t.Fatalf("期望 running，得到 %s（%s）", got.Status, got.LastError)
	}
}
