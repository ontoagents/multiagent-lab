package tool

// REQ-433/M229 单测：stdio 桥接——ParseStdioConfig 白名单/双 JSON 形态/真 spawn MCP server
// （node 内联 script 实现 initialize+tools/list+tools/call 最小协议回环）。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
)

func TestParseStdioConfig(t *testing.T) {
	// 白名单
	if _, err := ParseStdioConfig(map[string]any{"command": "uvx"}); err == nil || !strings.Contains(err.Error(), "D-O15") {
		t.Fatalf("uvx 应触 D-O15 拒绝: %v", err)
	}
	if _, err := ParseStdioConfig(map[string]any{}); err == nil {
		t.Fatal("缺 command 应报错")
	}
	// 合法 + args/env 双形态
	cfg, err := ParseStdioConfig(map[string]any{
		"command":   "npx",
		"args_json": `["-y","@modelcontextprotocol/server-memory"]`,
		"env_json":  `{"API_KEY":"k1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != "npx" || len(cfg.Args) != 2 || cfg.Env["API_KEY"] != "k1" {
		t.Fatalf("解析形态: %+v", cfg)
	}
	// 坏 JSON
	if _, err := ParseStdioConfig(map[string]any{"command": "npx", "args_json": "[坏"}); err == nil {
		t.Fatal("坏 args_json 应报错")
	}
}

// stubStdioServer 最小 stdio MCP server（node 内联：initialize/tools/list/tools/call JSON-RPC over LSP 分帧）。
func stubStdioServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `const lines = [];
process.stdin.setEncoding('utf8');
let buf = '';
process.stdin.on('data', (d) => {
  buf += d;
  let i;
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i).trim();
    buf = buf.slice(i + 1);
    if (!line) continue;
    const msg = JSON.parse(line);
    if (msg.id === undefined) continue;
    let result = null;
    if (msg.method === 'initialize') {
      result = { protocolVersion: msg.params.protocolVersion || '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: 'stub', version: '1' } };
    } else if (msg.method === 'tools/list') {
      result = { tools: [{ name: 'echo', description: '回显输入', inputSchema: { type: 'object', properties: { text: { type: 'string' } }, required: ['text'] } }] };
    } else if (msg.method === 'tools/call') {
      result = { content: [{ type: 'text', text: 'echo:' + (msg.params.arguments || {}).text }], isError: false };
    }
    process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result }) + '\n');
  }
});
`
	p := filepath.Join(dir, "stub-mcp.js")
	if err := os.WriteFile(p, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProbeStdioMCPLive(t *testing.T) {
	if _, err := os.Stat("/usr/bin/node"); err != nil {
		t.Skip("无 node——跳过真 spawn 回环")
	}
	p := stubStdioServer(t)
	cfg := &StdioConfig{Command: "node", Args: []string{p}, Env: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	names, defs, err := ProbeStdioMCP(ctx, cfg, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "echo" {
		t.Fatalf("工具清单: %v", names)
	}
	if defs[0].InputSchema == nil {
		t.Fatal("schema 应捕获")
	}
}

func TestFetchStdioMCPToolsLiveCall(t *testing.T) {
	if _, err := os.Stat("/usr/bin/node"); err != nil {
		t.Skip("无 node——跳过真 spawn 回环")
	}
	p := stubStdioServer(t)
	cfg := &StdioConfig{Command: "node", Args: []string{p}, Env: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bts, err := FetchStdioMCPTools(ctx, "stub", cfg, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(bts) != 1 {
		t.Fatalf("应 1 件工具: %d", len(bts))
	}
	// 经 einotool.InvokableTool 接口调用（每调用独立 spawn）
	inv, ok := bts[0].(einotool.InvokableTool)
	if !ok {
		t.Fatal("应为 InvokableTool")
	}
	out, err := inv.InvokableRun(ctx, `{"text":"你好"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "echo:你好") {
		t.Fatalf("调用结果: %s", out)
	}
}
