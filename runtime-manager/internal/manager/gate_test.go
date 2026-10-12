package manager

// REQ-293/M97 物化与装载原子对账单测（67 号 B3/B7）：
//   ①X-Onto-Triples 响应头解析（缺头=-1 跳过硬闸口径）；
//   ②countTriples 引擎计数解析（sparql-results+json bindings）；
//   ③计数硬闸的语义在 Start 内联（依赖完整引擎进程，硬闸对账由冒烟真机承载）。
// 沿 publish_meta_test.go 先例：httptest 假构建平面/假引擎，零外部依赖。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchTTLWithCountHeader(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Onto-Triples", "42")
		_, _ = w.Write([]byte("<urn:a> <urn:b> <urn:c> ."))
	}))
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL}
	ttl, n, err := m.FetchTTLWithCount("o1")
	if err != nil || n != 42 || ttl == "" {
		t.Fatalf("期望 ttl+计数 42，实际 n=%d err=%v", n, err)
	}
}

func TestFetchTTLWithoutCountHeader(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("raw turtle")) // original turtle：无计数头
	}))
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL}
	_, n, err := m.FetchTTLWithCount("o1")
	if err != nil || n != -1 {
		t.Fatalf("缺头期望 -1（跳过硬闸），实际 n=%d err=%v", n, err)
	}
}

func TestCountTriples(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("期望 POST SPARQL，实际 %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(`{"results":{"bindings":[{"c":{"value":"42"}}]}}`))
	}))
	defer svc.Close()
	n, err := countTriples(context.Background(), svc.URL)
	if err != nil || n != 42 {
		t.Fatalf("期望计数 42，实际 n=%d err=%v", n, err)
	}
}
