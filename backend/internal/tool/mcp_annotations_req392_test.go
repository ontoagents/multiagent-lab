package tool

// REQ-392/M188：MCP tool annotations 只读提示——fetch 注册 + danger 全审收窄判定单测。
import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestMCPAnnotationsFetchAndApproval 端到端：stub MCP 带 readOnlyHint 工具→fetch 注册→
// IsDangerousTool 收窄判定（readOnly 免审/写类全审/无标注回退全审）。
func TestMCPAnnotationsFetchAndApproval(t *testing.T) {
	ResetMCPReadOnlyHintsForTest()
	t.Cleanup(ResetMCPReadOnlyHintsForTest)
	srv := mcpserver.NewMCPServer("stub", "0.1.0")
	// readOnlyHint=true 工具（REQ-392 判定源）
	srv.AddTool(mcp.NewTool("search_docs", mcp.WithReadOnlyHintAnnotation(true)), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	// 未标注工具
	srv.AddTool(mcp.NewTool("mutate_thing"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	ts := httptest.NewServer(mcpserver.NewStreamableHTTPServer(srv))
	defer ts.Close()

	if _, err := FetchMCPToolsWithHeaders(context.Background(), "conn1", ts.URL+"/mcp", nil, 5*time.Second); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !HasMCPReadOnlyAnnotation("conn1__search_docs") {
		t.Fatal("readOnly 工具应注册标注")
	}
	if !MCPReadOnlyHint("conn1__search_docs") {
		t.Fatal("readOnlyHint 应为 true")
	}
	if IsDangerousTool("conn1__search_docs") {
		t.Fatal("readOnlyHint=true 应收窄免审")
	}
	// mcp-go v0.43 按规范为未标注工具补默认 annotations（readOnlyHint=false）——
	// 判定语义不变：显式 false 与「无标注」同样落入全审（诚实默认）
	if !IsDangerousTool("conn1__mutate_thing") {
		t.Fatal("未标注/显式 false 应全审")
	}
	// 显式 false：全审
	setMCPReadOnlyHint("conn1__explicit_write", false)
	if !IsDangerousTool("conn1__explicit_write") {
		t.Fatal("readOnlyHint=false 应全审")
	}
	// 无标注未知工具
	if !IsDangerousTool("other__anything") {
		t.Fatal("未注册前缀维持全审")
	}
}

var _ = http.StatusOK
