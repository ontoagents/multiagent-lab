// REQ-356/M156 MCP 工具清单装配缓存单测：TTL 内零重复握手（计数器实锤）/TTL 过期自动重拉/
// 认证头指纹进 key（凭据变更即失效）/失败不缓存（修复后下一轮直连成功）。
// stub = mcp-go 进程内 Streamable HTTP server（真实 Initialize+tools/list 全流程）。
// 注：一次抓取含多个 HTTP 请求（initialize+notification+tools/list），断言一律用相对比较。
package tool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// newCountingMCPServer 起一个带请求计数的 stub MCP server；返回 (URL, 计数指针, 切换真实/坏 handler)。
func newCountingMCPServer(t *testing.T) (string, *atomic.Int64, func(good bool)) {
	t.Helper()
	srv := mcpserver.NewMCPServer("stub", "0.1.0")
	srv.AddTool(mcp.NewTool("ping"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pong"), nil
	})
	good := mcpserver.NewStreamableHTTPServer(srv)
	var hits atomic.Int64
	counting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		good.ServeHTTP(w, r)
	})
	var cur atomic.Value
	cur.Store(http.Handler(counting))
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur.Load().(http.Handler).ServeHTTP(w, r)
	}))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts.URL + "/mcp", &hits, func(goodHandler bool) {
		if goodHandler {
			cur.Store(http.Handler(counting))
		} else {
			cur.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				http.Error(w, "stub down", http.StatusInternalServerError)
			}))
		}
	}
}

func toolName(t *testing.T, bt einotool.BaseTool) string {
	t.Helper()
	ti, err := bt.Info(context.Background())
	if err != nil || ti == nil {
		t.Fatalf("取工具 Info 失败: %v", err)
	}
	return ti.Name
}

// TTL 语义：TTL 内二取零握手；过期后自动重拉。
func TestFetchMCPToolsCacheTTL(t *testing.T) {
	url, hits, _ := newCountingMCPServer(t)
	ResetMCPToolsCacheForTest(400 * time.Millisecond)
	t.Cleanup(func() { ResetMCPToolsCacheForTest(60 * time.Second) })
	ctx := context.Background()

	bts1, err := FetchMCPTools(ctx, "facade", url, 5*time.Second)
	if err != nil {
		t.Fatalf("首取失败: %v", err)
	}
	if len(bts1) != 1 || toolName(t, bts1[0]) != "facade__ping" {
		t.Fatalf("工具应带 {server}__ 前缀，got %q", toolName(t, bts1[0]))
	}
	afterFirst := hits.Load()

	// TTL 内二取：零重复握手（缓存命中实锤）
	bts2, err := FetchMCPTools(ctx, "facade", url, 5*time.Second)
	if err != nil {
		t.Fatalf("二取失败: %v", err)
	}
	if hits.Load() != afterFirst {
		t.Fatalf("TTL 内二取应零握手，计数 %d→%d", afterFirst, hits.Load())
	}
	if toolName(t, bts2[0]) != "facade__ping" {
		t.Fatalf("缓存返回应同工具集")
	}

	// TTL 过期：自动重拉
	time.Sleep(500 * time.Millisecond)
	if _, err := FetchMCPTools(ctx, "facade", url, 5*time.Second); err != nil {
		t.Fatalf("过期后取失败: %v", err)
	}
	if hits.Load() <= afterFirst {
		t.Fatalf("TTL 过期应重拉，计数 %d→%d", afterFirst, hits.Load())
	}
}

// 认证头指纹进 key：换头=缓存失效重新握手；同头=命中。
func TestFetchMCPToolsCacheHeadersKey(t *testing.T) {
	url, hits, _ := newCountingMCPServer(t)
	ResetMCPToolsCacheForTest(5 * time.Second)
	t.Cleanup(func() { ResetMCPToolsCacheForTest(60 * time.Second) })
	ctx := context.Background()

	hA := map[string]string{"Authorization": "Bearer a"}
	hB := map[string]string{"Authorization": "Bearer b"}
	if _, err := FetchMCPToolsWithHeaders(ctx, "facade", url, hA, 5*time.Second); err != nil {
		t.Fatalf("带头首取失败: %v", err)
	}
	n1 := hits.Load()
	// 同头再取：命中（计数不变）
	if _, err := FetchMCPToolsWithHeaders(ctx, "facade", url, hA, 5*time.Second); err != nil {
		t.Fatalf("同头再取失败: %v", err)
	}
	if hits.Load() != n1 {
		t.Fatalf("同头应命中缓存，计数 %d→%d", n1, hits.Load())
	}
	// 换头：缓存失效重新握手
	if _, err := FetchMCPToolsWithHeaders(ctx, "facade", url, hB, 5*time.Second); err != nil {
		t.Fatalf("换头取失败: %v", err)
	}
	if hits.Load() <= n1 {
		t.Fatalf("认证头变更应缓存失效重握手，计数 %d→%d", n1, hits.Load())
	}
}

// 失败不缓存：坏 handler 报错后修复，下一轮直连成功（若失败被缓存则仍报错）。
func TestFetchMCPToolsFailureNotCached(t *testing.T) {
	url, hits, setGood := newCountingMCPServer(t)
	setGood(false) // 坏 handler（500）
	ResetMCPToolsCacheForTest(60 * time.Second)
	t.Cleanup(func() { ResetMCPToolsCacheForTest(60 * time.Second) })
	ctx := context.Background()

	if _, err := FetchMCPTools(ctx, "facade", url, 5*time.Second); err == nil {
		t.Fatal("坏 handler 应报错")
	}
	n := hits.Load()
	setGood(true)
	bts, err := FetchMCPTools(ctx, "facade", url, 5*time.Second)
	if err != nil {
		t.Fatalf("修复后取应成功（失败不得缓存）: %v", err)
	}
	if hits.Load() <= n {
		t.Fatalf("修复后应重新握手，计数 %d→%d", n, hits.Load())
	}
	if len(bts) == 0 {
		t.Fatal("修复后应返回工具")
	}
}
