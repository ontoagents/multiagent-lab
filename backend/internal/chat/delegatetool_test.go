package chat

// REQ-319/M121 单测：成员委派包装器三行为（A1 错误降级回喂 / 中断类穿透 / A2 超时降级）+ A3 委派单事件。
// 中断错误用 components/tool.StatefulInterrupt 构造——与 adk/agent_tool.go 成员挂起时上抛的是同一构造链
//（tool.CompositeInterrupt→StatefulInterrupt→*core.InterruptSignal），保证 errors.As 判定对真实形态成立。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

// stubTool 可编程的 Invokable 工具桩。
type stubTool struct {
	info  *schema.ToolInfo
	run   func(ctx context.Context, args string) (string, error)
	calls int
}

func (s *stubTool) Info(context.Context) (*schema.ToolInfo, error) {
	if s.info != nil {
		return s.info, nil
	}
	return &schema.ToolInfo{Name: "stub_member"}, nil
}

func (s *stubTool) InvokableRun(ctx context.Context, args string, _ ...einotool.Option) (string, error) {
	s.calls++
	return s.run(ctx, args)
}

type capturedEvent struct {
	typ  string
	data map[string]any
}

func withSink(ctx context.Context) (context.Context, *[]capturedEvent) {
	evts := &[]capturedEvent{}
	return tool.WithEventSink(ctx, func(t string, d map[string]any) { *evts = append(*evts, capturedEvent{t, d}) }), evts
}

func findEvent(evts *[]capturedEvent, typ string) *capturedEvent {
	for i := range *evts {
		if (*evts)[i].typ == typ {
			return &(*evts)[i]
		}
	}
	return nil
}

// A1：成员普通错误 → 降级为结构化 tool result（err=nil）+ run.warning 留痕。
func TestDelegateToolErrorDegrade(t *testing.T) {
	ctx, evts := withSink(context.Background())
	inner := &stubTool{run: func(context.Context, string) (string, error) {
		return "", errors.New("LLM connection reset")
	}}
	dt := newDelegateTool(inner, "网络排障手", 0).(einotool.InvokableTool)

	out, err := dt.InvokableRun(ctx, `{"request":"排查 pod 重启"}`)
	if err != nil {
		t.Fatalf("成员失败应降级回喂而非报错: %v", err)
	}
	if !strings.Contains(out, `"member_failed"`) || !strings.Contains(out, "网络排障手") || !strings.Contains(out, "LLM connection reset") {
		t.Fatalf("降级回喂缺关键字段: %s", out)
	}
	w := findEvent(evts, "run.warning")
	if w == nil {
		t.Fatal("缺 run.warning 事件")
	}
	mf, _ := w.data["member_failed"].(map[string]any)
	if mf == nil || mf["member"] != "网络排障手" {
		t.Fatalf("run.warning 缺 member_failed 结构: %v", w.data)
	}
}

// 中断穿透：成员审批/ask_human 挂起错误必须原样上抛（不降级、不发 run.warning）。
func TestDelegateToolInterruptPassthrough(t *testing.T) {
	ctx, evts := withSink(context.Background())
	interruptErr := einotool.StatefulInterrupt(context.Background(), "工具调用等待审批", nil)
	inner := &stubTool{run: func(context.Context, string) (string, error) {
		return "", interruptErr
	}}
	dt := newDelegateTool(inner, "审批成员", 0).(einotool.InvokableTool)

	_, err := dt.InvokableRun(ctx, `{"request":"写文件"}`)
	if err == nil {
		t.Fatal("中断错误必须上抛（挂起/checkpoint 恢复依赖）")
	}
	if !isMemberInterruptErr(err) {
		t.Fatalf("StatefulInterrupt 产物应被判定为中断类: %v", err)
	}
	if findEvent(evts, "run.warning") != nil {
		t.Fatal("中断不得发 run.warning（挂起非失败）")
	}
}

// A2：per-member 超时 → 超时错误同样降级回喂（不炸 run）。
func TestDelegateToolTimeoutDegrade(t *testing.T) {
	ctx, evts := withSink(context.Background())
	inner := &stubTool{run: func(ctx context.Context, _ string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
			return "done", nil
		}
	}}
	dt := newDelegateTool(inner, "慢成员", 50*time.Millisecond).(einotool.InvokableTool)

	out, err := dt.InvokableRun(ctx, `{"request":"长任务"}`)
	if err != nil {
		t.Fatalf("超时应降级回喂: %v", err)
	}
	if !strings.Contains(out, "member_failed") || !strings.Contains(out, "context deadline exceeded") {
		t.Fatalf("超时降级文本不符: %s", out)
	}
	if findEvent(evts, "run.warning") == nil {
		t.Fatal("超时缺 run.warning")
	}
}

// A3：委派单事件（request 提取 + 解析失败回退原文）。
func TestDelegateToolDelegateEvent(t *testing.T) {
	ctx, evts := withSink(context.Background())
	inner := &stubTool{run: func(context.Context, string) (string, error) { return "ok", nil }}
	dt := newDelegateTool(inner, "成员A", 0).(einotool.InvokableTool)

	if _, err := dt.InvokableRun(ctx, `{"request":"调研 K8s HPA"}`); err != nil {
		t.Fatal(err)
	}
	d := findEvent(evts, "subagent.delegate")
	if d == nil {
		t.Fatal("缺 subagent.delegate 事件")
	}
	if d.data["member"] != "成员A" || d.data["task"] != "调研 K8s HPA" {
		t.Fatalf("委派单字段不符: %v", d.data)
	}

	if _, err := dt.InvokableRun(ctx, `not-json`); err != nil {
		t.Fatal(err)
	}
	var delegates []capturedEvent
	for _, e := range *evts {
		if e.typ == "subagent.delegate" {
			delegates = append(delegates, e)
		}
	}
	if len(delegates) != 2 || delegates[1].data["task"] != "not-json" {
		t.Fatalf("第二次委派（非 JSON 入参）应回退原文: %v", delegates)
	}
}

// Info 透传（工具名/描述来自内层成员工具）。
func TestDelegateToolInfoPassthrough(t *testing.T) {
	dt := newDelegateTool(&stubTool{}, "成员B", 0).(einotool.InvokableTool)
	info, err := dt.Info(context.Background())
	if err != nil || info == nil || info.Name != "stub_member" {
		t.Fatalf("Info 应透传内层: %v %v", info, err)
	}
	_ = fmt.Sprint // 占位保持 fmt 引用（错误文案构建在实现内验证）
}
