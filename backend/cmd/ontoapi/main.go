// ontoapi 单二进制（REQ-561/M368 平台助手管理面 CLI；111 号方案档）——manifest 驱动生成
// 命名子命令 + 透传兜底（未知端点默认 deny）+ --help 可发现 + JSON 输出与 --limit/--fields
// 裁剪（防上下文刷爆）+ HTTP 错误人话归一（重名 409/If-Match 过期指引）+ --from 自动 GET→
// PUT 取 If-Match 基线。标准库 flag 兜底不引 CLI 框架（零新依赖——15 号登记免触发，D-O5）。
// 仅打 127.0.0.1 后端（信任级别同助手后端自身——不开远程管理面，Q-5 边界不动）。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/ontoapi"
)

func main() {
	fs := flag.NewFlagSet("ontoapi", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	baseURL := fs.String("base", envOr("ONTOAPI_BASE", "http://127.0.0.1:8080"), "后端基址（默认 127.0.0.1——CLI 不开远程管理面）")
	limit := fs.Int("limit", 0, "数组结果截断条数（防上下文刷爆）")
	fields := fs.String("fields", "", "数组对象字段裁剪（逗号分隔；如 id,name）")
	from := fs.String("from", "", "GET 源路径（自动取 updated_at 作 If-Match 基线——PUT 组合）")
	token := fs.String("token", envOr("ONTOAPI_TOKEN", ""), "访问凭证（非本机部署时；loopback 免凭证）")
	help := fs.Bool("help", false, "列出全部子命令")
	timeout := fs.Duration("timeout", 30*time.Second, "请求超时")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	args := fs.Args()
	if *help || len(args) == 0 {
		printHelp()
		return
	}
	if *from != "" {
		// --from 组合：GET 基线 → updated_at 作 If-Match（412 过期走人话指引归一）
		if ua := fetchUpdated(*baseURL, *from, *token, *timeout); ua != "" {
			args = append(args, "--ifmatch", ua)
		}
	}
	if len(args) == 0 {
		printHelp()
		return
	}
	name := args[0]
	ep, ok := ontoapi.Lookup(name)
	if !ok && name == "api" && len(args) >= 3 {
		// 透传兜底：api <METHOD> <PATH> [JSON]（未知 method+path 组合默认 deny——分级不旁路）
		ep, ok = passthrough(args[1], args[2])
	}
	if !ok {
		fmt.Fprintln(os.Stderr, ontoapi.DenyReason(name))
		os.Exit(3)
	}
	if ep.Access == ontoapi.Deny {
		// deny 级 CLI 侧硬闸（platform_cli 工具侧同判——双闸防绕）
		fmt.Fprintln(os.Stderr, ontoapi.DenyReason(name))
		os.Exit(3)
	}
	rest := args[1:]
	path := ep.Path
	var body []byte
	// 路径参数（:id/:run_id 按出现顺序用位置参数替换）
	for {
		i := strings.Index(path, ":")
		if i < 0 {
			break
		}
		end := i + 1
		for end < len(path) && path[end] != '/' {
			end++
		}
		if len(rest) == 0 {
			fmt.Fprintf(os.Stderr, "缺路径参数 %s\n", path[i:end])
			os.Exit(2)
		}
		path = path[:i] + rest[0] + path[end:]
		rest = rest[1:]
	}
	// 查询参数（k=v 形态；首个 JSON body 停止）；--ifmatch 特殊旗标
	qs := []string{}
	var ifMatch string
	for len(rest) > 0 {
		if rest[0] == "--ifmatch" && len(rest) >= 2 {
			ifMatch = rest[1]
			rest = rest[2:]
			continue
		}
		if strings.Contains(rest[0], "=") && !strings.HasPrefix(rest[0], "{") {
			qs = append(qs, rest[0])
			rest = rest[1:]
			continue
		}
		break
	}
	if len(qs) > 0 {
		path += "?" + strings.Join(qs, "&")
	}
	if len(rest) > 0 {
		body = []byte(strings.Join(rest, " "))
	}
	out, code, err := request(*baseURL, ep.Method, path, body, ifMatch, *token, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "请求失败（后端不可达？）: "+err.Error())
		os.Exit(4)
	}
	if code >= 300 {
		fmt.Fprintln(os.Stderr, humanizeError(code, out))
		os.Exit(5)
	}
	fmt.Println(trimJSON(out, *limit, *fields))
}

// passthrough 透传兜底（api <METHOD> <PATH>）——未知 method+path 组合默认 deny：
// 仅 manifest 覆盖的端点形态放行（分级不旁路——111 号「未知端点默认 deny」）。
func passthrough(method, rawPath string) (ontoapi.Endpoint, bool) {
	method = strings.ToUpper(method)
	path := rawPath
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	for _, e := range ontoapi.Manifest {
		if e.Method == method && matchPath(e.Path, path) {
			return e, true
		}
	}
	return ontoapi.Endpoint{}, false
}

// matchPath 模板匹配（:id 段通配）。
func matchPath(tpl, path string) bool {
	tp := strings.Split(strings.Trim(tpl, "/"), "/")
	pp := strings.Split(strings.Trim(path, "/"), "/")
	if len(tp) != len(pp) {
		return false
	}
	for i, seg := range tp {
		if strings.HasPrefix(seg, ":") {
			continue
		}
		if seg != pp[i] {
			return false
		}
	}
	return true
}

// fetchUpdated GET 基线取 updated_at（--from 组合；取不到如实空串——PUT 侧 412 走人话指引）。
func fetchUpdated(base, path, token string, timeout time.Duration) string {
	out, code, err := request(base, "GET", path, nil, "", token, timeout)
	if err != nil || code >= 300 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(out, &m) != nil {
		return ""
	}
	ua, _ := m["updated_at"].(string)
	return ua
}

func request(base, method, path string, body []byte, ifMatch, token string, timeout time.Duration) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	if token != "" {
		req.Header.Set("X-Platform-Token", token)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return buf, resp.StatusCode, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// trimJSON JSON 输出 + --limit 数组截断 + --fields 字段裁剪（防上下文刷爆——111 号验收）。
func trimJSON(out []byte, limit int, fields string) string {
	var v any
	if json.Unmarshal(out, &v) != nil {
		return string(out)
	}
	if fields != "" {
		want := map[string]bool{}
		for _, f := range strings.Split(fields, ",") {
			if f = strings.TrimSpace(f); f != "" {
				want[f] = true
			}
		}
		trimFields(v, want)
	}
	if limit > 0 {
		v = limitAny(v, limit)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func trimFields(v any, want map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, it := range t {
			if want[k] {
				trimFields(it, want)
			} else {
				delete(t, k)
			}
		}
	case []any:
		for _, it := range t {
			trimFields(it, want)
		}
	}
}

// limitAny 数组截断（重建切片——JSON 重封只含前 limit 条；包装键递归）。
func limitAny(v any, limit int) any {
	switch t := v.(type) {
	case []any:
		if len(t) > limit {
			return t[:limit]
		}
		return t
	case map[string]any:
		for k, it := range t {
			t[k] = limitAny(it, limit)
		}
		return t
	default:
		return v
	}
}

func humanizeError(code int, out []byte) string {
	msg := strings.TrimSpace(string(out))
	var m map[string]any
	if json.Unmarshal(out, &m) == nil {
		if e, ok := m["error"].(string); ok {
			msg = e
		}
	}
	switch {
	case code == http.StatusConflict:
		return fmt.Sprintf("409 冲突：%s——重名时改用其他名称，或先用 list 查看现有条目", msg)
	case code == http.StatusPreconditionFailed:
		return fmt.Sprintf("412 基线过期：%s——资源已被并发更新；PUT 请加 --from <GET 路径> 自动取 If-Match 基线后重试", msg)
	case code == http.StatusNotFound:
		return fmt.Sprintf("404 不存在：%s——先用 list/GET 确认 id", msg)
	case code == http.StatusUnauthorized:
		return fmt.Sprintf("401 需要访问凭证：%s——非本机部署须设 --token 或 ONTOAPI_TOKEN", msg)
	default:
		return fmt.Sprintf("HTTP %d：%s", code, msg)
	}
}

func printHelp() {
	fmt.Println("ontoapi — 平台助手管理面 CLI（REQ-561；仅打 127.0.0.1 后端）")
	fmt.Println("用法: ontoapi <子命令> [路径参数...] [k=v 查询...] [JSON body]")
	fmt.Println("全局: --base --limit N --fields a,b --from <GET 路径> --token --timeout")
	fmt.Println("\n子命令（分级 ro=直行/rw=审批/deny=拒绝）:")
	for _, e := range ontoapi.Manifest {
		fmt.Printf("  %-24s %-6s %-6s %s\n", e.Name, e.Access.String(), e.Method, e.Desc)
	}
	fmt.Println("  api                     passthrough（<METHOD> <PATH> [JSON]；未知端点默认 deny）")
}
