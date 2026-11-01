// REQ-209/M42 远程部署访问凭证：单一共享静态 token 做非 loopback 访问的粗粒度准入。
//
// 与 Q-5「不做登录/账户体系」不同层：无用户/角色/会话隔离，纯平台级准入（10 号 §11.1 拆出）。
// - PLATFORM_TOKEN 未设置/为空 = 不启用，本机开发零影响（原有行为，零中间件开销）；
// - 启用后仅管 /api/* 数据面；/healthz（存活探测）、/mcp（REQ-131 自带 Bearer 鉴权）、
//   静态资源（无数据）放行；
// - loopback 豁免：RemoteAddr 为回环地址**且无代理转发头**（同机反代会带 X-Forwarded-For，
//   此时请求源自远程客户端，须持凭证——防「反代同机部署令 token 形同虚设」的豁免漏洞）；
// - 凭证来源：Authorization: Bearer / X-Platform-Token / ?access_token=（EventSource 兼容）；
//   比较用 subtle.ConstantTimeCompare 防时序侧信道。
package main

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
)

func accessToken() string { return strings.TrimSpace(os.Getenv("PLATFORM_TOKEN")) }

// accessROToken 只读演示凭证（REQ-556/M363；107 号 §7.3 权限收窄选型）：PLATFORM_TOKEN_RO
// 环境变量（未设=不启用）。第二档作用域=GET 类全通、非 GET 类 /api/* 一律 403（人话），
// 敏感导出面按甄别结果排除（/api/backup/export 打包 data/.secret 主密钥——必须 403；
// /api/settings/export 明文不出密钥可放行）。仍是平台级共享凭证（非账户/非 RBAC——Q-5 边界维持），
// 仅作用域分档；loopback 直连豁免语义不变（本机全权沿 REQ-209）；RO 不补位 /mcp（handlers_mcp
// 补位面维持仅全权 token——工具写面不因只读档扩大）。
func accessROToken() string { return strings.TrimSpace(os.Getenv("PLATFORM_TOKEN_RO")) }

// roSensitivePaths RO 档敏感导出面排除清单（GET 也不放行——逐面核实：backup/export 打包
// 主密钥 secret 文件；后续新增导出面按同法核实后追加）。
var roSensitivePaths = []string{"/api/backup/export"}

func accessAuth(next http.Handler) http.Handler {
	token := accessToken()
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") { // 仅 /api/* 数据面（见包注）
			next.ServeHTTP(w, r)
			return
		}
		if loopbackDirect(r) { // 本机直连豁免（无代理转发头）
			next.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(presentedToken(r)), []byte(token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		// REQ-556：只读演示凭证（PLATFORM_TOKEN_RO）——全权不匹配后按 RO 档分域；
		// RO 对 /mcp 不补位（本中间件仅管 /api/*，/mcp 自带 Bearer 鉴权不受影响）。
		if ro := accessROToken(); ro != "" && subtle.ConstantTimeCompare([]byte(presentedToken(r)), []byte(ro)) == 1 {
			if isROSensitive(r.URL.Path) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"只读凭证无权访问该资源（敏感导出面）——如需导出请使用全权凭证 PLATFORM_TOKEN"}`))
				return
			}
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			default:
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"只读凭证（PLATFORM_TOKEN_RO）仅允许 GET/HEAD/OPTIONS——写操作请使用全权凭证 PLATFORM_TOKEN"}`))
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="platform"`)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"需要访问凭证：本平台已启用 PLATFORM_TOKEN（非本机访问须携带 Bearer 凭证 / X-Platform-Token 头或 ?access_token= 参数）"}`))
	})
}

// isROSensitive RO 档敏感路径判定（前缀精确匹配清单）。
func isROSensitive(path string) bool {
	for _, p := range roSensitivePaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// loopbackDirect 回环直连判定：RemoteAddr 为 loopback 且未经过代理转发（无 X-Forwarded-For /
// X-Real-IP 头）。转发头由反代注入，出现即视为远程来源，不豁免。
func loopbackDirect(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// presentedToken 提取请求凭证（Bearer 头 > X-Platform-Token 头 > access_token 查询参数）。
func presentedToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if h := strings.TrimSpace(r.Header.Get("X-Platform-Token")); h != "" {
		return h
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}
