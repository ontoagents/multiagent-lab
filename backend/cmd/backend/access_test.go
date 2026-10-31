// REQ-209/M42 访问凭证中间件单测：启用态四象限（非 loopback 无凭证 401/带凭证放行/loopback 豁免/
// 代理头不豁免）+ 未启用零影响 + 常量时间比较路径 + 凭证三来源。
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func withToken(t *testing.T, v string) {
	t.Helper()
	old, had := os.LookupEnv("PLATFORM_TOKEN")
	os.Setenv("PLATFORM_TOKEN", v)
	t.Cleanup(func() {
		if had {
			os.Setenv("PLATFORM_TOKEN", old)
		} else {
			os.Unsetenv("PLATFORM_TOKEN")
		}
	})
}

func newReq(remote, path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.RemoteAddr = remote
	return r
}

func TestAccessAuthDisabledPassthrough(t *testing.T) {
	withToken(t, "")
	h := accessAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for _, remote := range []string{"192.168.1.5:1234", "127.0.0.1:9999"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq(remote, "/api/kb"))
		if rec.Code != 200 {
			t.Fatalf("未启用时 %s 应透传，got %d", remote, rec.Code)
		}
	}
}

func TestAccessAuthEnabledMatrix(t *testing.T) {
	withToken(t, "s3cret")
	h := accessAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	// 非 loopback 无凭证 → 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("192.168.1.5:1234", "/api/kb"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("非 loopback 无凭证 = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("401 应带 WWW-Authenticate")
	}

	// 非 loopback 三种凭证来源 → 放行
	for name, set := range map[string]func(*http.Request){
		"bearer":  func(r *http.Request) { r.Header.Set("Authorization", "Bearer s3cret") },
		"xtoken":  func(r *http.Request) { r.Header.Set("X-Platform-Token", "s3cret") },
		"queryey": func(r *http.Request) { r.URL.RawQuery = "access_token=s3cret" },
	} {
		r := newReq("192.168.1.5:1234", "/api/kb")
		set(r)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != 200 {
			t.Fatalf("%s 凭证应放行, got %d", name, rec.Code)
		}
	}

	// loopback 直连（无代理头）→ 豁免
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("127.0.0.1:9999", "/api/kb"))
	if rec.Code != 200 {
		t.Fatalf("loopback 直连应豁免, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("[::1]:9999", "/api/kb"))
	if rec.Code != 200 {
		t.Fatalf("IPv6 loopback 应豁免, got %d", rec.Code)
	}

	// loopback + 代理转发头（同机反代）→ 不豁免须凭证
	r := newReq("127.0.0.1:9999", "/api/kb")
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("同机反代（XFF）应要求凭证, got %d", rec.Code)
	}
	r = newReq("127.0.0.1:9999", "/api/kb")
	r.Header.Set("X-Real-IP", "203.0.113.7")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("X-Real-IP 同理应要求凭证, got %d", rec.Code)
	}

	// 错误凭证 → 401
	r = newReq("192.168.1.5:1234", "/api/kb")
	r.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误凭证应 401, got %d", rec.Code)
	}
}

func TestAccessAuthOnlyAPIGated(t *testing.T) {
	withToken(t, "s3cret")
	h := accessAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for _, p := range []string{"/healthz", "/mcp", "/", "/assets/app.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq("192.168.1.5:1234", p))
		if rec.Code != 200 {
			t.Fatalf("%s 不应被凭证门禁拦截（仅 /api/* 管）, got %d", p, rec.Code)
		}
	}
}

// REQ-556/M363 只读演示凭证（PLATFORM_TOKEN_RO）行为矩阵（107 号 §7.4 验收③）：
// 无凭证 401／RO GET 200／RO POST 403 人话／全权 POST 200／loopback 无凭证 200（豁免不回归）/
// RO 打敏感导出面 403（backup/export 打包主密钥）／RO 未启用时带 RO 值凭证走全权判定 401。
func TestAccessAuthROScope_Req556(t *testing.T) {
	withToken(t, "full-s3cret")
	old, had := os.LookupEnv("PLATFORM_TOKEN_RO")
	os.Setenv("PLATFORM_TOKEN_RO", "ro-s3cret")
	t.Cleanup(func() {
		if had {
			os.Setenv("PLATFORM_TOKEN_RO", old)
		} else {
			os.Unsetenv("PLATFORM_TOKEN_RO")
		}
	})
	h := accessAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	ro := func(r *http.Request) { r.Header.Set("X-Platform-Token", "ro-s3cret") }
	full := func(r *http.Request) { r.Header.Set("X-Platform-Token", "full-s3cret") }

	// RO GET → 200
	rec := httptest.NewRecorder()
	r := newReq("192.168.1.5:1234", "/api/kb")
	ro(r)
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("RO GET 应 200, got %d", rec.Code)
	}
	// RO POST → 403 人话
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/conversations", nil)
	r.RemoteAddr = "192.168.1.5:1234"
	ro(r)
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "只读凭证") {
		t.Fatalf("RO POST 应 403 人话, got %d %s", rec.Code, rec.Body.String())
	}
	// RO 敏感导出面（backup/export 打包主密钥）→ 403
	rec = httptest.NewRecorder()
	r = newReq("192.168.1.5:1234", "/api/backup/export")
	ro(r)
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "敏感导出面") {
		t.Fatalf("RO backup/export 应 403, got %d %s", rec.Code, rec.Body.String())
	}
	// 全权 POST → 200（RO 不影响全权档）
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/conversations", nil)
	r.RemoteAddr = "192.168.1.5:1234"
	full(r)
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("全权 POST 应 200, got %d", rec.Code)
	}
	// loopback 无凭证 → 200（豁免语义不回归）
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("127.0.0.1:9999", "/api/kb"))
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback 豁免应保持, got %d", rec.Code)
	}
	// RO 未启用：RO 值凭证非全权 → 401（第二档关闭零变化）
	os.Unsetenv("PLATFORM_TOKEN_RO")
	rec = httptest.NewRecorder()
	r = newReq("192.168.1.5:1234", "/api/kb")
	ro(r)
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("RO 未启用时 RO 凭证应 401, got %d", rec.Code)
	}
	// 错误 RO 凭证 → 401（非 403——不泄露 RO 档存在性）
	rec = httptest.NewRecorder()
	r = newReq("192.168.1.5:1234", "/api/kb")
	r.Header.Set("X-Platform-Token", "ro-wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误凭证应 401, got %d", rec.Code)
	}
}
