package tool

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"sync"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// renamedInvokable 改写暴露给模型的工具名并保留 Invoke 能力（§6.11）。
type renamedInvokable struct {
	einotool.InvokableTool
	name string
}

func (r *renamedInvokable) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ti, err := r.InvokableTool.Info(ctx)
	if err != nil {
		return nil, err
	}
	c := *ti // 浅拷贝，避免改写共享 ToolInfo
	c.Name = r.name
	return &c, nil
}

// renamedStreamable 改写暴露给模型的流式工具名并保留 StreamInvoke 能力。
type renamedStreamable struct {
	einotool.StreamableTool
	name string
}

func (r *renamedStreamable) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ti, err := r.StreamableTool.Info(ctx)
	if err != nil {
		return nil, err
	}
	c := *ti
	c.Name = r.name
	return &c, nil
}

// ---- REQ-356/M156：MCP 工具清单装配层进程内缓存 ----
// 每轮消息 run 重做装配（runner Assemble）时，ontology facade FetchTools 与 Agent 级连接器
// fetchEndpoint 都走 FetchMCPToolsWithHeaders——每次重做 Streamable HTTP 连接+Initialize+tools/list
// 全流程，属每轮固定开销+失败面（MCP 抖动→单次失败即降级的误报放大）。缓存 key=serverName+
// URL+认证头指纹（FNV-1a，凭据变更即失效）；TTL 默认 60s，MCPTOOLS_TTL 环境变量可调。
// 语义零变化：缓存只加速成功路径——失败不缓存（下轮照旧直连重试，降级/ontology__ 前缀改名/
// 审批面/SourceOf 标注均沿旧）；listChanged 长连接订阅不做（TTL 收敛——缓存期内 facade 工具集
// 变更最迟一个 TTL 生效，诚实边界）。
type mcpToolsCacheEntry struct {
	tools   []einotool.BaseTool
	expires time.Time
}

var (
	mcpToolsCacheMu sync.RWMutex
	mcpToolsCache   = map[string]mcpToolsCacheEntry{}
	// McpToolsCacheTTL 缓存生存期（包变量便于测试与调参；init 读 MCPTOOLS_TTL 覆盖默认 60s）
	McpToolsCacheTTL = 60 * time.Second
	// mcpToolsCacheCap 防御性上限（连接器/挂载方案数量级远小于此；满时先清过期，仍满清最早过期）
	mcpToolsCacheCap = 64
)

func init() {
	if v := os.Getenv("MCPTOOLS_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			McpToolsCacheTTL = d
		}
	}
}

// mcpCacheKey serverName+URL+认证头指纹（头名排序拼接，凭据变更即缓存失效）
func mcpCacheKey(serverName, url string, headers map[string]string) string {
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	h := fnv.New64a()
	_, _ = h.Write([]byte(serverName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(url))
	for _, k := range names {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte("="))
		_, _ = h.Write([]byte(headers[k]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ResetMCPToolsCacheForTest 测试辅助：清空缓存并重置 TTL（跨包单测用）。
func ResetMCPToolsCacheForTest(ttl time.Duration) {
	mcpToolsCacheMu.Lock()
	defer mcpToolsCacheMu.Unlock()
	mcpToolsCache = map[string]mcpToolsCacheEntry{}
	McpToolsCacheTTL = ttl
}

// FetchMCPTools 连接 streamable HTTP MCP server 并拉取全部工具（§6.11）。
// 工具以 {server}__{tool} 前缀重命名后返回；连接/初始化/列工具失败返回 error，
// 由装配层降级记告警（§13 风险：MCP 不可用不阻断运行，不做重试风暴）。
func FetchMCPTools(ctx context.Context, serverName, url string, timeout time.Duration) ([]einotool.BaseTool, error) {
	return FetchMCPToolsWithHeaders(ctx, serverName, url, nil, timeout)
}

// FetchMCPToolsWithHeaders REQ-214 P2：带自定义请求头的变体（mcp 直通连接器的认证头，
// 凭据态 headers 经装配层解密注入；不经此函数的头一律不带）。
// REQ-356/M156：命中进程内缓存直接返回（TTL 内零握手）；未命中直连取数，成功才写缓存
//（失败不缓存——装配层降级语义沿旧，下轮照旧直连）。ontology facade 与 Agent 级连接器
// 同一装载函数，双双受益。
func FetchMCPToolsWithHeaders(ctx context.Context, serverName, url string, headers map[string]string, timeout time.Duration) ([]einotool.BaseTool, error) {
	key := mcpCacheKey(serverName, url, headers)
	mcpToolsCacheMu.RLock()
	if e, ok := mcpToolsCache[key]; ok && time.Now().Before(e.expires) {
		mcpToolsCacheMu.RUnlock()
		return e.tools, nil
	}
	mcpToolsCacheMu.RUnlock()

	bts, err := fetchMCPToolsDirect(ctx, serverName, url, headers, timeout)
	if err != nil {
		return nil, err
	}

	mcpToolsCacheMu.Lock()
	if len(mcpToolsCache) >= mcpToolsCacheCap {
		now := time.Now()
		for k, e := range mcpToolsCache { // 先清过期
			if now.After(e.expires) {
				delete(mcpToolsCache, k)
			}
		}
		for len(mcpToolsCache) >= mcpToolsCacheCap { // 仍满：清最早过期
			var oldestKey string
			var oldest time.Time
			first := true
			for k, e := range mcpToolsCache {
				if first || e.expires.Before(oldest) {
					oldestKey, oldest, first = k, e.expires, false
				}
			}
			delete(mcpToolsCache, oldestKey)
		}
	}
	mcpToolsCache[key] = mcpToolsCacheEntry{tools: bts, expires: time.Now().Add(McpToolsCacheTTL)}
	mcpToolsCacheMu.Unlock()
	return bts, nil
}

// fetchMCPToolsDirect 直连装载（连接+Initialize+tools/list+前缀改名），REQ-356 起为缓存未命中路径。
func fetchMCPToolsDirect(ctx context.Context, serverName, url string, headers map[string]string, timeout time.Duration) ([]einotool.BaseTool, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := []transport.StreamableHTTPCOption{}
	if len(headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}
	cli, err := mcpclient.NewStreamableHttpClient(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("mcp client create: %w", err)
	}
	if err := cli.Start(cctx); err != nil {
		return nil, fmt.Errorf("mcp start: %w", err)
	}
	if _, err := cli.Initialize(cctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: "2025-03-26",
			ClientInfo:      mcp.Implementation{Name: "eino-multiagent-lab", Version: "0.1.0"},
		},
	}); err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}

	bts, err := mcpp.GetTools(cctx, &mcpp.Config{Cli: cli})
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}
	// REQ-392/M188：读取 MCP 官方 tool annotations（readOnlyHint）入注册表——danger 全审
	// 收窄的判定源（approval.go IsDangerousTool 查询）。ListTools 与 GetTools 并行口径：
	// 注释读取为轻量元数据往返（装配 TTL 缓存窗口内一次），无标注/读取失败=无注册（维持全审默认）。
	if ltRes, lerr := cli.ListTools(cctx, mcp.ListToolsRequest{}); lerr == nil {
		for _, mt := range ltRes.Tools {
			full := serverName + "__" + mt.Name
			if mt.Annotations.ReadOnlyHint != nil {
				setMCPReadOnlyHint(full, *mt.Annotations.ReadOnlyHint)
			}
		}
	}

	out := make([]einotool.BaseTool, 0, len(bts))
	prefix := serverName + "__"
	for _, bt := range bts {
		ti, err := bt.Info(ctx)
		if err != nil || ti == nil || ti.Name == "" {
			continue
		}
		// 保留底层工具的调用能力：按 Invokable / Streamable 分别包装，仅改写名字。
		switch t := bt.(type) {
		case einotool.InvokableTool:
			out = append(out, &renamedInvokable{InvokableTool: t, name: prefix + ti.Name})
		case einotool.StreamableTool:
			out = append(out, &renamedStreamable{StreamableTool: t, name: prefix + ti.Name})
		default:
			out = append(out, bt)
		}
	}
	return out, nil
}
