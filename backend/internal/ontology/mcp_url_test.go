package ontology

// REQ-518/M325：ONTOLOGY_MCP_URL 缺省自 RUNTIME_MGR_URL 派生（同服务 /mcp 路径）——
// 治「容器编排缺 env→默认 127.0.0.1:8090/mcp 打进 backend 自身容器→onto_* 工具静默缺失」。

import (
	"os"
	"testing"
)

func TestNewServiceMCPURLDerived(t *testing.T) {
	t.Setenv("RUNTIME_MGR_URL", "http://runtime-manager:8090")
	os.Unsetenv("ONTOLOGY_MCP_URL")
	s := NewService()
	if s.MCPURL != "http://runtime-manager:8090/mcp" {
		t.Fatalf("MCPURL 应自 RUNTIME_MGR_URL 派生 /mcp，got %q", s.MCPURL)
	}
	if s.RuntimeURL != "http://runtime-manager:8090" {
		t.Fatalf("RuntimeURL 不应受派生影响: %q", s.RuntimeURL)
	}
}

func TestNewServiceMCPURLExplicitWins(t *testing.T) {
	t.Setenv("RUNTIME_MGR_URL", "http://runtime-manager:8090")
	t.Setenv("ONTOLOGY_MCP_URL", "http://mcp-proxy:9000/mcp")
	s := NewService()
	if s.MCPURL != "http://mcp-proxy:9000/mcp" {
		t.Fatalf("显式 ONTOLOGY_MCP_URL 应优先，got %q", s.MCPURL)
	}
}

func TestNewServiceMCPURLDefaultBackwardCompat(t *testing.T) {
	os.Unsetenv("RUNTIME_MGR_URL")
	os.Unsetenv("ONTOLOGY_MCP_URL")
	s := NewService()
	if s.MCPURL != "http://127.0.0.1:8090/mcp" {
		t.Fatalf("宿主直跑缺省派生应与旧默认一致（127.0.0.1:8090/mcp），got %q", s.MCPURL)
	}
}
