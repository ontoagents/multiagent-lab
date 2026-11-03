package facade

// REQ-435/M231 单测：运行面工具组——方案 CRUD/观测面（进程内 store+manager 桩）/caller 归因落痕/
// engine_install 未知引擎如实报。生命周期 start/stop/reload 需真实引擎二进制，集成面由冒烟承载。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/manager"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

func newRuntimeFixture(t *testing.T) *Facade {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "rt.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mg := manager.New(st, "http://127.0.0.1:8091", filepath.Join(dir, "logs"), filepath.Join(dir, "bin"))
	f := New(st, func(string) (string, error) { return "http://127.0.0.1:9999/sparql", nil })
	f.Manager = mg
	return f
}

type profileStopEngine struct {
	stopErr error
	entered chan struct{}
	release chan struct{}
}

func (e *profileStopEngine) Start(ctx context.Context, _ string, _ int, _ map[string]string) (*engine.Process, error) {
	if e.entered != nil {
		e.entered <- struct{}{}
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return engine.NewProcess("http://127.0.0.1:1/sparql", 42, func() error { return e.stopErr }), nil
}

func (*profileStopEngine) HealthCheck(context.Context, string) error { return nil }

func TestRuntimeMutationPreservesLifecycle(t *testing.T) {
	for _, mode := range []string{"starting", "stop-failed"} {
		for _, op := range []string{"delete", "update"} {
			t.Run(mode+"/"+op, func(t *testing.T) {
				f := newRuntimeFixture(t)
				build := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/external-refs") {
						_, _ = w.Write([]byte(`{"refs":[],"dangling":[]}`))
					} else if strings.HasSuffix(r.URL.Path, "/export") {
						_, _ = w.Write([]byte("<urn:s> <urn:p> <urn:o> ."))
					} else {
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(build.Close)
				f.Manager.BuildURL = build.URL
				f.Manager.HealthTries = 1
				if err := f.Store.SetConfig(store.RuntimeConfig{ExecutionMethod: "native"}); err != nil {
					t.Fatal(err)
				}
				p := &store.Profile{ID: "p1", Name: "original", Engine: "fuseki", OntologyIDs: []string{"o1"}, Config: "{}"}
				if err := f.Store.Create(p); err != nil {
					t.Fatal(err)
				}
				e := &profileStopEngine{stopErr: errors.New("engine stop refused")}
				f.Manager.RegisterEngine("fuseki", e)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.Cleanup(cancel)
				if mode == "starting" {
					e.entered, e.release = make(chan struct{}, 1), make(chan struct{})
					var workers sync.WaitGroup
					workers.Add(1)
					t.Cleanup(func() { close(e.release); workers.Wait() })
					go func() { defer workers.Done(); _ = f.Manager.Start(ctx, p.ID) }()
					select {
					case <-e.entered:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				} else {
					if err := f.Manager.Start(ctx, p.ID); err != nil {
						t.Fatal(err)
					}
					if err := f.Manager.Stop(p.ID); err == nil {
						t.Fatal("fixture stop must fail")
					}
				}
				var result string
				if op == "delete" {
					result = call(t, f.handleProfileDelete(), ctx, "runtime_profile_delete", map[string]any{"profile_id": p.ID})
				} else {
					result = call(t, f.handleProfileUpdate(), ctx, "runtime_profile_update", map[string]any{"profile_id": p.ID, "name": "changed", "ontologies_json": `["o2"]`})
				}
				if !strings.Contains(result, map[string]string{"delete": "删除失败", "update": "更新失败"}[op]) {
					t.Fatalf("mutation must fail: %s", result)
				}
				got, err := f.Store.Get(p.ID)
				if err != nil || got.Name != p.Name || len(got.OntologyIDs) != 1 || got.OntologyIDs[0] != "o1" {
					t.Fatalf("unsafe mutation changed or removed profile: %+v %v", got, err)
				}
			})
		}
	}
}

// call 断言 handler 无 Go 层错误并回文本结果（工具级错误在文本里）。
func call(t *testing.T, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), ctx context.Context, name string, args map[string]any) string {
	t.Helper()
	res, err := h(ctx, reqOf(name, args))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("空结果")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("非文本结果: %T", res.Content[0])
	}
	return tc.Text
}

func TestRuntimeProfileCreateListAudit(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	out := call(t, f.handleProfileCreate(), ctx, "runtime_profile_create", map[string]any{
		"name": "测试方案", "engine": "oxigraph", "ontologies_json": `["ins-1"]`, "caller": "测试协调者",
	})
	var created struct {
		Created string `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil || created.Created == "" {
		t.Fatalf("创建应成功: %s", out)
	}
	// caller 归因落 trace_log（runtime_audit 行，profile_trace 读回）
	trRaw := call(t, f.handleProfileTrace(), ctx, "profile_trace", map[string]any{"profile_id": created.Created})
	if !strings.Contains(trRaw, "caller=测试协调者") || !strings.Contains(trRaw, "action=runtime_profile_create") {
		t.Fatalf("runtime_audit 归因行应落 trace: %s", trRaw)
	}
	// list 含该方案
	if !strings.Contains(call(t, f.handleProfileList(), ctx, "runtime_profile_list", nil), "测试方案") {
		t.Fatal("list 应含新建方案")
	}
	// 非法引擎拒绝
	if !strings.Contains(call(t, f.handleProfileCreate(), ctx, "runtime_profile_create", map[string]any{"name": "x", "engine": "mem"}), "仅支持 oxigraph / fuseki") {
		t.Fatal("非法引擎应拒绝")
	}
	// 无 caller 不落归因行
	out2 := call(t, f.handleProfileCreate(), ctx, "runtime_profile_create", map[string]any{"name": "无归因"})
	var c2 struct {
		Created string `json:"created"`
	}
	_ = json.Unmarshal([]byte(out2), &c2)
	if strings.Contains(call(t, f.handleProfileTrace(), ctx, "profile_trace", map[string]any{"profile_id": c2.Created}), "runtime_audit") {
		t.Fatal("无 caller 不应落归因行")
	}
}

func TestRuntimeUpdateSemantics(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	var c struct {
		Created string `json:"created"`
	}
	_ = json.Unmarshal([]byte(call(t, f.handleProfileCreate(), ctx, "runtime_profile_create", map[string]any{"name": "up", "engine": "oxigraph"})), &c)
	out := call(t, f.handleProfileUpdate(), ctx, "runtime_profile_update", map[string]any{
		"profile_id": c.Created, "name": "up-2", "caller": "归因者",
	})
	if !strings.Contains(out, "up-2") {
		t.Fatalf("update 应生效: %s", out)
	}
	if !strings.Contains(call(t, f.handleProfileTrace(), ctx, "profile_trace", map[string]any{"profile_id": c.Created}), "caller=归因者 action=runtime_profile_update") {
		t.Fatal("update 归因应落痕")
	}
	if !strings.Contains(call(t, f.handleProfileUpdate(), ctx, "runtime_profile_update", map[string]any{"profile_id": "rt_none", "name": "x"}), "不存在") {
		t.Fatal("不存在应报错")
	}
}

func TestRuntimeObservabilityTools(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	if !strings.Contains(call(t, f.handleRouteAmbiguities(), ctx, "route_ambiguities", nil), "ambiguities") {
		t.Fatal("route_ambiguities 形态")
	}
	if !strings.Contains(call(t, f.handleEnginesStatus(), ctx, "engines_status", nil), "engines") {
		t.Fatal("engines_status 形态")
	}
	// logs/trace 对已建未启动方案返回空集（对不存在方案报错——沿 rest.logs 同语义）
	var cp struct {
		Created string `json:"created"`
	}
	_ = json.Unmarshal([]byte(call(t, f.handleProfileCreate(), ctx, "runtime_profile_create", map[string]any{"name": "obs"})), &cp)
	if !strings.Contains(call(t, f.handleProfileLogs(), ctx, "profile_logs", map[string]any{"profile_id": cp.Created}), "lines") {
		t.Fatal("logs 形态（未启动返回空行不报错）")
	}
	if !strings.Contains(call(t, f.handleProfileTrace(), ctx, "profile_trace", map[string]any{"profile_id": cp.Created}), "traces") {
		t.Fatal("trace 形态")
	}
	if !strings.Contains(call(t, f.handleProfileLogs(), ctx, "profile_logs", map[string]any{"profile_id": "rt_none"}), "失败") {
		t.Fatal("不存在方案应报错")
	}
}

func TestEngineInstallUnknown(t *testing.T) {
	f := newRuntimeFixture(t)
	if !strings.Contains(call(t, f.handleEngineInstall(), context.Background(), "engine_install", map[string]any{"engine": "no-such"}), "失败") {
		t.Fatal("未知引擎应报错")
	}
}
