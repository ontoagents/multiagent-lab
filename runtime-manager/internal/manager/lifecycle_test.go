package manager

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

// Only the external engine and build service are replaced. Lifecycle state and
// profile status use the real Manager and SQLite store.
type lifecycleEngine struct {
	mu        sync.Mutex
	starts    map[string]int
	stops     map[string]int
	onStart   func(context.Context, string) error
	onStop    func(string) error
	healthErr error
}

func (e *lifecycleEngine) Start(ctx context.Context, id string, _ int, _ map[string]string) (*engine.Process, error) {
	e.mu.Lock()
	e.starts[id]++
	fn := e.onStart
	e.mu.Unlock()
	if fn != nil {
		if err := fn(ctx, id); err != nil {
			return nil, err
		}
	}
	return engine.NewProcess("http://127.0.0.1:1/sparql", 42, func() error {
		e.mu.Lock()
		e.stops[id]++
		fn := e.onStop
		e.mu.Unlock()
		if fn != nil {
			return fn(id)
		}
		return nil
	}), nil
}

func (e *lifecycleEngine) HealthCheck(context.Context, string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.healthErr
}

func (e *lifecycleEngine) counts(id string) (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.starts[id], e.stops[id]
}

func lifecycleFixture(t *testing.T) (*Manager, *lifecycleEngine, context.Context) {
	t.Helper()
	build := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/external-refs"):
			_, _ = w.Write([]byte(`{"refs":[],"dangling":[]}`))
		case strings.HasSuffix(r.URL.Path, "/export"):
			_, _ = w.Write([]byte("<urn:s> <urn:p> <urn:o> ."))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(build.Close)
	m := New(newStore(t), build.URL, t.TempDir(), t.TempDir())
	if err := m.Store.SetConfig(store.RuntimeConfig{ExecutionMethod: "native"}); err != nil {
		t.Fatal(err)
	}
	e := &lifecycleEngine{starts: map[string]int{}, stops: map[string]int{}}
	m.RegisterEngine("fuseki", e)
	m.HealthTries = 1
	for _, id := range []string{"p1", "p2"} {
		if err := m.Store.Create(&store.Profile{ID: id, Name: id, Engine: "fuseki", OntologyIDs: []string{"o1"}, Config: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return m, e, ctx
}

func lifecycleResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("lifecycle operation did not finish")
		return nil
	}
}

func TestLifecycleRejectsOverlappingStart(t *testing.T) {
	for _, op := range []string{"start", "stop", "reload", "remove"} {
		t.Run(op, func(t *testing.T) {
			m, e, ctx := lifecycleFixture(t)
			entered, release := make(chan struct{}, 4), make(chan struct{})
			var once sync.Once
			var workers sync.WaitGroup
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(func() { unblock(); workers.Wait() })
			e.onStart = func(ctx context.Context, id string) error {
				if id != "p1" {
					return nil
				}
				select {
				case entered <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			first := make(chan error, 1)
			workers.Add(1)
			go func() { defer workers.Done(); first <- m.Start(ctx, "p1") }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			// An unrelated profile remains usable while p1's engine is blocked.
			if err := m.Start(ctx, "p2"); err != nil {
				t.Fatalf("independent profile: %v", err)
			}
			second := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				switch op {
				case "start":
					second <- m.Start(ctx, "p1")
				case "stop":
					second <- m.Stop("p1")
				case "reload":
					second <- m.Reload(ctx, "p1")
				case "remove":
					second <- m.Remove("p1")
				}
			}()
			err := lifecycleResult(t, second)
			if err == nil || !strings.Contains(err.Error(), "操作进行中") {
				t.Fatalf("overlap must explicitly fail as busy: %v", err)
			}
			unblock()
			if err := lifecycleResult(t, first); err != nil {
				t.Fatal(err)
			}
			if starts, stops := e.counts("p1"); starts != 1 || stops != 0 {
				t.Fatalf("unexpected side effects: starts=%d stops=%d", starts, stops)
			}
			if err := m.Stop("p1"); err != nil {
				t.Fatal(err)
			}
			p, _ := m.Store.Get("p1")
			if p.Status != "stopped" {
				t.Fatalf("successful stop resurrected profile: %+v", p)
			}
		})
	}
}

func TestLifecycleStopFailureCanRetry(t *testing.T) {
	m, e, ctx := lifecycleFixture(t)
	stopErr := errors.New("engine stop failed")
	e.onStop = func(id string) error {
		_, stops := e.counts(id)
		if stops == 1 {
			return stopErr
		}
		return nil
	}
	if err := m.Start(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("p1"); !errors.Is(err, stopErr) {
		t.Fatalf("stop error must reach caller: %v", err)
	}
	p, _ := m.Store.Get("p1")
	if p.Status != "error" || !strings.Contains(p.LastError, stopErr.Error()) {
		t.Fatalf("failed stop falsely reported success: %+v", p)
	}
	if _, err := m.ProcEndpoint("p1"); err != nil {
		t.Fatalf("stop failure lost handle: %v", err)
	}
	if err := m.Start(ctx, "p1"); err == nil {
		t.Fatal("must not replace a handle whose stop failed")
	}
	if err := m.Stop("p1"); err != nil {
		t.Fatalf("retry stop: %v", err)
	}
	if _, err := m.ProcEndpoint("p1"); err == nil {
		t.Fatal("successful retry retained handle")
	}
	if starts, stops := e.counts("p1"); starts != 1 || stops != 2 {
		t.Fatalf("retry must stop the same process: starts=%d stops=%d", starts, stops)
	}
}

func TestLifecycleReloadOwnsStopAndStart(t *testing.T) {
	m, e, ctx := lifecycleFixture(t)
	if err := m.Start(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	stopEntered, startEntered := make(chan struct{}, 1), make(chan struct{}, 1)
	releaseStop, releaseStart := make(chan struct{}), make(chan struct{})
	var stopOnce, startOnce sync.Once
	unblockStop := func() { stopOnce.Do(func() { close(releaseStop) }) }
	unblockStart := func() { startOnce.Do(func() { close(releaseStart) }) }
	var workers sync.WaitGroup
	t.Cleanup(func() { unblockStop(); unblockStart(); workers.Wait() })
	e.mu.Lock()
	e.onStop = func(string) error {
		stopEntered <- struct{}{}
		select {
		case <-releaseStop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.onStart = func(ctx context.Context, _ string) error {
		startEntered <- struct{}{}
		select {
		case <-releaseStart:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.mu.Unlock()
	result := make(chan error, 1)
	workers.Add(1)
	go func() { defer workers.Done(); result <- m.Reload(ctx, "p1") }()
	for phase, entered := range []<-chan struct{}{stopEntered, startEntered} {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if err := m.Stop("p1"); err == nil || !strings.Contains(err.Error(), "操作进行中") {
			t.Fatalf("reload phase %d released lifecycle gate: %v", phase, err)
		}
		unblockStop()
	}
	unblockStart()
	if err := lifecycleResult(t, result); err != nil {
		t.Fatal(err)
	}
	p, err := m.Store.Get("p1")
	if err != nil || p.Status != "running" {
		t.Fatalf("reload outcome: profile=%+v err=%v", p, err)
	}
	if starts, stops := e.counts("p1"); starts != 2 || stops != 1 {
		t.Fatalf("reload must replace engine exactly once: starts=%d stops=%d", starts, stops)
	}
}

func TestLifecycleFailedStartCleanupCanRetry(t *testing.T) {
	m, e, ctx := lifecycleFixture(t)
	e.healthErr = errors.New("engine unhealthy")
	stopErr := errors.New("cleanup failed")
	e.onStop = func(id string) error {
		_, stops := e.counts(id)
		if stops == 1 {
			return stopErr
		}
		return nil
	}
	if err := m.Start(ctx, "p1"); !errors.Is(err, stopErr) || !strings.Contains(err.Error(), "engine unhealthy") {
		t.Fatalf("startup and cleanup errors must reach caller: %v", err)
	}
	p, err := m.Store.Get("p1")
	if err != nil || p.Status != "error" || !strings.Contains(p.LastError, "cleanup failed") {
		t.Fatalf("failed startup state: profile=%+v err=%v", p, err)
	}
	if _, err := m.ProcEndpoint("p1"); err != nil {
		t.Fatalf("failed startup cleanup lost process handle: %v", err)
	}
	if err := m.Start(ctx, "p1"); err == nil {
		t.Fatal("failed cleanup must not allow another engine")
	}
	if err := m.Stop("p1"); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	if starts, stops := e.counts("p1"); starts != 1 || stops != 2 {
		t.Fatalf("cleanup retry counts: starts=%d stops=%d", starts, stops)
	}
}

func TestLifecycleReloadAndRemovePreserveFailedStop(t *testing.T) {
	for _, op := range []string{"reload", "remove"} {
		t.Run(op, func(t *testing.T) {
			m, e, ctx := lifecycleFixture(t)
			stopErr := errors.New("stop refused")
			e.onStop = func(string) error { return stopErr }
			if err := m.Start(ctx, "p1"); err != nil {
				t.Fatal(err)
			}
			var err error
			if op == "reload" {
				err = m.Reload(ctx, "p1")
			} else {
				err = m.Remove("p1")
			}
			if !errors.Is(err, stopErr) {
				t.Fatalf("stop failure must abort %s: %v", op, err)
			}
			p, err := m.Store.Get("p1")
			if err != nil || p.Status != "error" {
				t.Fatalf("failed %s lost profile: profile=%+v err=%v", op, p, err)
			}
			if _, err := m.ProcEndpoint("p1"); err != nil {
				t.Fatalf("failed %s lost handle: %v", op, err)
			}
			if starts, stops := e.counts("p1"); starts != 1 || stops != 1 {
				t.Fatalf("failed %s must not start another engine: starts=%d stops=%d", op, starts, stops)
			}
			e.mu.Lock()
			e.onStop = nil
			e.mu.Unlock()
			if err := m.Remove("p1"); err != nil {
				t.Fatalf("successful cleanup then remove: %v", err)
			}
			if _, err := m.Store.Get("p1"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("successful remove retained profile: %v", err)
			}
		})
	}
}

func TestLifecycleCanceledStartDoesNotBecomeRunning(t *testing.T) {
	m, e, ctx := lifecycleFixture(t)
	ctx, cancel := context.WithCancel(ctx)
	e.onStart = func(context.Context, string) error { cancel(); return nil }
	if err := m.Start(ctx, "p1"); err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("canceled startup must fail: %v", err)
	}
	p, err := m.Store.Get("p1")
	if err != nil || p.Status != "error" {
		t.Fatalf("canceled startup state: profile=%+v err=%v", p, err)
	}
	if _, err := m.ProcEndpoint("p1"); err == nil {
		t.Fatal("canceled startup retained successfully stopped engine")
	}
	if starts, stops := e.counts("p1"); starts != 1 || stops != 1 {
		t.Fatalf("cancellation cleanup: starts=%d stops=%d", starts, stops)
	}
}
