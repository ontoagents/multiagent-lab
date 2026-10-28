package chat

// REQ-454/M250：中断恢复悬空 ToolCalls 修补单测。
// 覆盖：合成结果文案对齐 A1 unknownResultText（结果未知+重试边界纪律）/溯源尾注/
// newChatModelAgent 挂接 patchtoolcalls 中间件后装配成功（全装配点生效）。

import (
	"context"
	"strings"
	"testing"
)

func TestPatchedUnknownToolResult_Req454(t *testing.T) {
	r := patchedUnknownToolResult("write_file", "call-1")
	if r == nil || r.ToolResult != nil {
		t.Fatal("应为纯文本合成结果（Content 形态）")
	}
	if !strings.HasPrefix(r.Content, "（结果未知：该运行中断") {
		t.Fatalf("合成文案未对齐 A1 unknownResultText: %q", r.Content[:40])
	}
	if !strings.Contains(r.Content, "勿盲目重试") {
		t.Fatal("合成文案缺重试边界纪律（A1 口径）")
	}
	if !strings.Contains(r.Content, "patchtoolcalls 合成") || !strings.Contains(r.Content, "tool=write_file") || !strings.Contains(r.Content, "id=call-1") {
		t.Fatalf("缺溯源尾注: %q", r.Content)
	}
}

func TestNewChatModelAgent_WithPatchMiddleware_Req454(t *testing.T) {
	fake := &fakeInner{}
	inst, err := newChatModelAgent(context.Background(), "t", "d", "inst", 3, fake, nil, nil)
	if err != nil {
		t.Fatalf("挂 patchtoolcalls 中间件后装配失败: %v", err)
	}
	if inst == nil {
		t.Fatal("装配产物为空")
	}
}
