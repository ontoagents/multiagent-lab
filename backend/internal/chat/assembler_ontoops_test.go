// REQ-366/M164 装配门控集成单测：本体 facade 扩展工具（spec CRUD/校验/CQ 基准）仅在
// Agent.OntoOps 开启时并入装配清单（默认关=存量零变化）；原 6 查询工具挂载即并入不变；
// 能力纪律段（source=onto_ops）随开关注入且 instructionSegments 可切分。
package chat

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ontoagents/ontoagents/backend/internal/ontology"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// stubOntoFacade 桩本体 facade：既有查询工具 1 件 + 扩展工具 2 件（读/写各一）。
func stubOntoFacade(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("ontology-facade-stub", "1.0.0", server.WithToolCapabilities(false))
	s.AddTool(mcp.NewTool("get_concept", mcp.WithDescription("stub")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("{}"), nil
	})
	s.AddTool(mcp.NewTool("spec_get", mcp.WithDescription("stub")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("{}"), nil
	})
	s.AddTool(mcp.NewTool("spec_concept_upsert", mcp.WithDescription("stub")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("{}"), nil
	})
	srv := httptest.NewServer(server.NewStreamableHTTPServer(s))
	t.Cleanup(srv.Close)
	return srv
}

func namesOf(tb *toolBundle) []string {
	out := []string{}
	for _, bt := range tb.Tools {
		if ti, err := bt.Info(context.Background()); err == nil && ti != nil {
			out = append(out, ti.Name)
		}
	}
	return out
}

func hasTool(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// assembleWithMount 带本体挂载 scope 的装配（assembleOnly 固定空 scope，此处需 Mount）。
func (a *Assembler) assembleWithMount(ctx context.Context, t *testing.T, ag *store.Agent, sc assembleScope) *toolBundle {
	t.Helper()
	tb, err := a.assembleTools(ctx, ag, sc)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestAssembleOntoOpsGate 关态/开态装配清单对照（REQ-366 验收：关态装配零本体操作工具）。
func TestAssembleOntoOpsGate(t *testing.T) {
	asm, _, _ := newAssemblerFixture(t)
	asm.Ontology = &ontology.Service{MCPURL: stubOntoFacade(t).URL, DialTimeout: 2 * time.Second}
	sc := assembleScope{Mount: "p1"} // 挂载本体（非空即走 facade 合并分支）

	// 关态（默认）：查询工具并入，扩展工具零装配
	off := &store.Agent{Name: "a"}
	tbOff := asm.assembleWithMount(context.Background(), t, off, sc)
	names := namesOf(tbOff)
	if !hasTool(names, "ontology__get_concept") {
		t.Fatalf("挂载即并入语义应保留（ontology__get_concept 应在装配清单）: %v", names)
	}
	for _, n := range names {
		if tool.IsOntoOperationTool(n) {
			t.Fatalf("关态装配不应含扩展工具 %s: %v", n, names)
		}
	}
	if hasTool(names, "ontology__spec_get") || hasTool(names, "ontology__spec_concept_upsert") {
		t.Fatalf("关态装配不应含 spec_* 工具: %v", names)
	}

	// 开态：扩展工具并入，SourceOf 沿 ontology:facade
	on := &store.Agent{Name: "b", OntoOps: true}
	tbOn := asm.assembleWithMount(context.Background(), t, on, sc)
	namesOn := namesOf(tbOn)
	if !hasTool(namesOn, "ontology__spec_get") || !hasTool(namesOn, "ontology__spec_concept_upsert") {
		t.Fatalf("开态装配应含扩展工具: %v", namesOn)
	}
	if tbOn.SourceOf["ontology__spec_get"] != "ontology:facade" {
		t.Fatalf("来源标注应 ontology:facade: %v", tbOn.SourceOf)
	}
}

// TestOntoOpsDisciplineSegment 纪律段注入与透镜切分（关态零注入）。
func TestOntoOpsDisciplineSegment(t *testing.T) {
	asm, _, _ := newAssemblerFixture(t)
	off := &store.Agent{Name: "a"}
	instOff := asm.composeInstructionWithScope(off, assembleScope{})
	if strings.Contains(instOff, "本体操作纪律（OntoOps）") {
		t.Fatal("关态不应注入 OntoOps 纪律段")
	}
	on := &store.Agent{Name: "b", OntoOps: true}
	instOn := asm.composeInstructionWithScope(on, assembleScope{})
	if !strings.Contains(instOn, "本体操作纪律（OntoOps）") {
		t.Fatal("开态应注入 OntoOps 纪律段")
	}
	segs := instructionSegments(instOn)
	found := false
	for _, seg := range segs {
		if seg["source"] == "onto_ops" {
			found = true
		}
	}
	if !found {
		t.Fatalf("instructionSegments 应切出 onto_ops 段: %v", segs)
	}
}
