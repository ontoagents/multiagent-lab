package tool

// REQ-433/M229 stdio MCP 桥接——平台托管 spawn 进程接入 npx 型本地 MCP server（生态最大存量）。
//
// 形态：连接器 kind=stdio（config.command/args_json/env_json）——装配期 spawn 一次取工具清单
// （schema/annotations），调用期**每调用独立 spawn**（initialize→tools/call→Close；stdin 关闭
// 即服务端退出沿 MCP stdio 约定——零孤儿进程，等效「懒启动+即用即回收」；进程空闲常驻形态
// 随需要评估）。命令白名单 npx/node 先行；uvx/Python 依赖触 D-O15 先裁定（不放开）。
// npx 冷启动（每调用 1~3s）为诚实边界。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
)

// StdioAllowedBins 命令白名单（REQ-433：npx/node 先行——uvx/Python 触 D-O15 先裁定，不放开）。
var StdioAllowedBins = map[string]bool{"npx": true, "node": true}

// StdioConfig stdio 连接器 config 解析形态。
type StdioConfig struct {
	Command string
	Args    []string
	Env     map[string]string
}

func ParseStdioConfig(cfg map[string]any) (*StdioConfig, error) {
	command, _ := cfg["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("command 必填")
	}
	if !StdioAllowedBins[command] {
		return nil, fmt.Errorf("command 须为 npx | node（REQ-433 先行范围；uvx/Python 触 D-O15 需先裁定）")
	}
	out := &StdioConfig{Command: command, Env: map[string]string{}}
	if raw, ok := cfg["args_json"].(string); ok && strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &out.Args); err != nil {
			return nil, fmt.Errorf("args_json \u987b\u4e3a JSON \u6570\u7ec4: %w", err)
		}
	}
	if raw, ok := cfg["env_json"].(string); ok && strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &out.Env); err != nil {
			return nil, fmt.Errorf("env_json \u987b\u4e3a JSON \u5bf9\u8c61: %w", err)
		}
	}
	return out, nil
}

// spawnStdio spawn 进程并完成 MCP initialize（进程生命周期归调用方——用毕 Close）。
func spawnStdio(ctx context.Context, cfg *StdioConfig) (*mcpclient.Client, error) {
	envList := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		envList = append(envList, k+"="+v)
	}
	cli, err := mcpclient.NewStdioMCPClient(cfg.Command, envList, cfg.Args...)
	if err != nil {
		return nil, fmt.Errorf("spawn %s: %w", cfg.Command, err)
	}
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := cli.Initialize(initCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: "2025-03-26",
			ClientInfo:      mcp.Implementation{Name: "eino-multiagent-lab", Version: "0.1.0"},
		},
	}); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	return cli, nil
}

// StdioToolDef 装配期探明的工具定义（schema/annotations）。
type StdioToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	ReadOnly    bool
}

// ProbeStdioMCP spawn→initialize→tools/list 探测（连接器注册/测试闸门共用）——
// 返回工具名清单与定义（能力入库沿 REQ-214 P2 口径），用毕 Close。
func ProbeStdioMCP(ctx context.Context, cfg *StdioConfig, timeout time.Duration) (names []string, defs []StdioToolDef, err error) {
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cli, err := spawnStdio(sctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = cli.Close() }()
	lt, err := cli.ListTools(sctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("mcp list tools: %w", err)
	}
	for _, mt := range lt.Tools {
		names = append(names, mt.Name)
		defs = append(defs, StdioToolDef{
			Name: mt.Name, Description: mt.Description,
			InputSchema: func() map[string]any {
				b, _ := json.Marshal(mt.InputSchema)
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				return m
			}(),
			ReadOnly: mt.Annotations.ReadOnlyHint != nil && *mt.Annotations.ReadOnlyHint,
		})
	}
	return names, defs, nil
}

// stdioInvokable 每调用独立 spawn 的调用包装（零孤儿进程；npx 冷启动成本见文件头诚实边界）。
type stdioInvokable struct {
	def  StdioToolDef
	cfg  *StdioConfig
	name string // {server}__{tool}
}

func (t *stdioInvokable) Info(context.Context) (*schema.ToolInfo, error) {
	inputSchema := &jsonschema.Schema{}
	if b, err := json.Marshal(t.def.InputSchema); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, inputSchema)
	}
	return &schema.ToolInfo{
		Name:        t.name,
		Desc:        t.def.Description,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(inputSchema),
	}, nil
}

func (t *stdioInvokable) InvokableRun(ctx context.Context, arguments string, _ ...einotool.Option) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cli, err := spawnStdio(callCtx, t.cfg)
	if err != nil {
		return "", err
	}
	defer func() { _ = cli.Close() }()
	var args json.RawMessage = json.RawMessage(arguments)
	if strings.TrimSpace(arguments) == "" {
		args = json.RawMessage("{}")
	}
	res, err := cli.CallTool(callCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: t.def.Name, Arguments: args},
	})
	if err != nil {
		return "", fmt.Errorf("stdio tool call: %w", err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if res.IsError {
		return strings.TrimSpace(b.String()), fmt.Errorf("工具返回错误: %s", strings.TrimSpace(b.String()))
	}
	return b.String(), nil
}

// FetchStdioMCPTools 装配期装载：spawn 一次取工具清单（定义+annotations 注册），随后进程关闭；
// 返回的成员工具每调用独立 spawn 执行（生命周期见文件头）。服务端不可达返回 error 由装配层降级告警。
func FetchStdioMCPTools(ctx context.Context, serverName string, cfg *StdioConfig, timeout time.Duration) ([]einotool.BaseTool, error) {
	_, defs, err := ProbeStdioMCP(ctx, cfg, timeout)
	if err != nil {
		return nil, err
	}
	prefix := serverName + "__"
	out := make([]einotool.BaseTool, 0, len(defs))
	for _, d := range defs {
		if d.ReadOnly {
			setMCPReadOnlyHint(prefix+d.Name, true) // REQ-392 同口径：官方 readOnlyHint 注册（danger 收窄判定源）
		}
		out = append(out, &stdioInvokable{def: d, cfg: cfg, name: prefix + d.Name})
	}
	return out, nil
}
