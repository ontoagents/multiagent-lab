package chat

// REQ-453/M249：callbacks 统一执行观测面单测。
// 覆盖：Needed 门控（level 0/1）/model 路径 duration+usage（含 cache tokens 非零透出）/
// tool 路径 name/错误路径/无关组件跳过/流式拷贝关闭。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

type mockTokenUsage = model.TokenUsage

func collectObs(t *testing.T) (context.Context, *[]map[string]any) {
	t.Helper()
	var got []map[string]any
	ctx := tool.WithEventSink(context.Background(), func(eventType string, data map[string]any) {
		if eventType == "obs.timing" {
			got = append(got, data)
		}
	})
	return ctx, &got
}

func TestObsHandler_Needed_Req453(t *testing.T) {
	h0 := &obsHandler{level: DebugOff}
	h1 := &obsHandler{level: DebugDetailed}
	if h0.Needed(context.Background(), nil, callbacks.TimingOnStart) {
		t.Fatal("level=0 不应激活（旁路流拷贝开销）")
	}
	if !h1.Needed(context.Background(), nil, callbacks.TimingOnStart) {
		t.Fatal("level>0 应激活")
	}
}

func TestObsHandler_ModelTiming_Req453(t *testing.T) {
	ctx, got := collectObs(t)
	h := &obsHandler{level: DebugDetailed}
	ri := &callbacks.RunInfo{Name: "agent-a", Type: "OpenAI", Component: components.ComponentOfChatModel}
	cctx := h.OnStart(ctx, ri, nil)
	time.Sleep(5 * time.Millisecond)
	h.OnEnd(cctx, ri, &model.CallbackOutput{
		Message: &schema.Message{Content: "hi"},
		TokenUsage: &mockTokenUsage{
			PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120,
			PromptTokenDetails: model.PromptTokenDetails{CachedTokens: 80, CacheWriteTokens: 10},
		},
	})
	if len(*got) != 1 {
		t.Fatalf("应发出 1 条 obs.timing，得到 %d", len(*got))
	}
	d := (*got)[0]
	if d["component"] != "model" || d["name"] != "agent-a" {
		t.Fatalf("component/name 不符: %v", d)
	}
	if dm, ok := d["duration_ms"].(int64); !ok || dm < 4 {
		t.Fatalf("duration_ms 缺失或过小: %v", d["duration_ms"])
	}
	u, ok := d["usage"].(map[string]any)
	if !ok || u["total_tokens"] != 120 {
		t.Fatalf("usage 缺失: %v", d)
	}
	if u["cached_tokens"] != 80 || u["cache_write_tokens"] != 10 {
		t.Fatalf("cache tokens 未透出: %v", u)
	}
}

func TestObsHandler_CacheTokensZeroOmitted_Req453(t *testing.T) {
	ctx, got := collectObs(t)
	h := &obsHandler{level: DebugFull}
	ri := &callbacks.RunInfo{Name: "a", Type: "OpenAI", Component: components.ComponentOfChatModel}
	cctx := h.OnStart(ctx, ri, nil)
	h.OnEnd(cctx, ri, &model.CallbackOutput{
		Message:    &schema.Message{Content: "x"},
		TokenUsage: &mockTokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6},
	})
	u := (*got)[0]["usage"].(map[string]any)
	if _, has := u["cached_tokens"]; has {
		t.Fatal("cached_tokens=0 不应透出（省默认体积）")
	}
}

func TestObsHandler_ToolAndError_Req453(t *testing.T) {
	ctx, got := collectObs(t)
	h := &obsHandler{level: DebugDetailed}
	tri := &callbacks.RunInfo{Name: "web_fetch", Type: "ConcreteTool", Component: components.ComponentOfTool}
	cctx := h.OnStart(ctx, tri, nil)
	h.OnError(cctx, tri, errors.New("boom"))
	if len(*got) != 1 {
		t.Fatalf("tool 错误应发出 1 条，得到 %d", len(*got))
	}
	d := (*got)[0]
	if d["component"] != "tool" || d["name"] != "web_fetch" {
		t.Fatalf("tool component/name 不符: %v", d)
	}
	if d["error"] != "boom" {
		t.Fatalf("error 不符: %v", d)
	}
	// 无关组件（agent/graph/lambda）跳过
	ari := &callbacks.RunInfo{Name: "coord", Type: "ChatModelAgent", Component: "ChatModelAgent"}
	h.OnStart(ctx, ari, nil)
	h.OnEnd(cctx, ari, nil)
	if len(*got) != 1 {
		t.Fatalf("agent 级组件不应发 obs.timing，得到 %d", len(*got))
	}
}

func TestObsHandler_StreamVariantsClose_Req453(t *testing.T) {
	ctx, got := collectObs(t)
	h := &obsHandler{level: DebugDetailed}
	ri := &callbacks.RunInfo{Name: "a", Type: "OpenAI", Component: components.ComponentOfChatModel}
	// 流式输入/输出拷贝须关闭（接口契约；不读取）——经 Pipe 构造真实空流验证 Close 安全
	inR, _ := schema.Pipe[callbacks.CallbackInput](1)
	_ = h.OnStartWithStreamInput(ctx, ri, inR)
	outR, _ := schema.Pipe[callbacks.CallbackOutput](1)
	octx := h.OnEndWithStreamOutput(ctx, ri, outR)
	if len(*got) != 1 {
		t.Fatalf("流式 OnEnd 应发 1 条 obs.timing，得到 %d", len(*got))
	}
	if (*got)[0]["component"] != "model" {
		t.Fatalf("流式 component 不符: %v", (*got)[0])
	}
	_ = octx
}
