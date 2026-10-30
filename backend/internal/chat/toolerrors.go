package chat

// 工具容错包装（2026-10-20 报障迭代）：工具执行失败不再终止整个 run，错误作为 tool result
// 回喂模型（可修正参数重试/换方法/向用户说明）——把 REQ-319 A1 成员委派的降级口径推广到
// 全部工具。背景（Windows 桌面端项目会话报障）：代码修复类任务 save_file 等工具 Go 层失败
// （典型：输出达 max_tokens 上限截断致参数 JSON 解析 eof、写文件 I/O 失败）此前一律
// NodeRunError 致命中止整轮运行，用户观感是「任务执行未完成就停止」。
// 透传白名单（吞掉会破坏语义，原样上抛）：
//   - 中断类（审批/ask_human 挂起）：isInterruptErr 三重判定——checkpoint 恢复依赖该错误形态穿透；
//   - context.Canceled（用户停止）：回喂会让 run 在已取消上下文里继续，必须上抛保持 rc.stopped 语义。
// 输出流中途错误经 Pipe.Send 的错误位原样穿透中断/取消；其余错误保留已收内容+尾注回喂
// （平台当前无流式中断型工具：审批/ask_human/委派均 Invokable-only，尾注降级不破坏挂起语义）。
// 接口面纪律同 toolargs.go/delegatetool.go：Invokable-only 不虚增 StreamableTool
// （compose ToolsNode 按断言优先走流式路径），流式工具经 safeStreamTool 保持断言。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// tolerantTool 包装单个工具：执行失败→错误回喂；重复包裹透传，仅实现 BaseTool 的罕见形态原样返回。
func tolerantTool(bt einotool.BaseTool) einotool.BaseTool {
	switch bt.(type) {
	case safeTool, safeStreamTool:
		return bt
	}
	if st, ok := bt.(einotool.StreamableTool); ok {
		return safeStreamTool{StreamableTool: st}
	}
	if it, ok := bt.(einotool.InvokableTool); ok {
		return safeTool{InvokableTool: it}
	}
	return bt
}

// passthroughErr 必须原样上抛的错误：中断类（挂起恢复语义）+ 用户取消（停止语义）。
func passthroughErr(err error) bool {
	return isInterruptErr(err) || errors.Is(err, context.Canceled)
}

// toolErrorFeedback 把工具错误转为回喂输出（JSON 载荷，口径同 delegatetool 成员失败回喂），
// 并发 run.warning 留痕（无事件汇的调用场景静默跳过）。
func toolErrorFeedback(ctx context.Context, name string, err error) string {
	detail := truncateRunes(err.Error(), 300)
	tool.EmitEvent(ctx, "run.warning", map[string]any{
		"message":     fmt.Sprintf("工具 %q 执行失败，错误已回喂模型（可修正参数重试/换方法/向用户说明）: %s", name, detail),
		"tool_failed": map[string]any{"tool": name, "error": detail},
	})
	hint := "你可以修正参数后重试、换一种方式完成任务，或向用户如实说明。"
	if strings.Contains(err.Error(), "failed to unmarshal arguments") {
		hint = "参数 JSON 不完整或非法（常见成因：输出达到 max_tokens 上限被截断）。可缩短单次产出" +
			"（如分多次写文件）后重试，或调大智能体「最大回复 tokens」。"
	}
	payload, _ := json.Marshal(map[string]any{
		"error":  "tool_failed",
		"tool":   name,
		"detail": detail,
		"hint":   hint,
	})
	return string(payload)
}

// baseToolName 错误回喂标注用工具名（Info 失败降级 unknown）。
func baseToolName(ctx context.Context, bt einotool.BaseTool) string {
	if ti, err := bt.Info(ctx); err == nil && ti != nil && ti.Name != "" {
		return ti.Name
	}
	return "unknown"
}

// safeTool Invokable-only 工具的容错包装。
type safeTool struct {
	einotool.InvokableTool
}

func (t safeTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...einotool.Option) (string, error) {
	out, err := t.InvokableTool.InvokableRun(ctx, argsInJSON, opts...)
	if err == nil {
		return out, nil
	}
	if passthroughErr(err) {
		return "", err
	}
	return toolErrorFeedback(ctx, baseToolName(ctx, t.InvokableTool), err), nil
}

// safeStreamTool 流式工具的容错包装（保持 StreamableTool 断言成立）。
type safeStreamTool struct {
	einotool.StreamableTool
}

func (t safeStreamTool) StreamableRun(ctx context.Context, argsInJSON string, opts ...einotool.Option) (*schema.StreamReader[string], error) {
	sr, err := t.StreamableTool.StreamableRun(ctx, argsInJSON, opts...)
	name := baseToolName(ctx, t.StreamableTool)
	if err != nil {
		if passthroughErr(err) {
			return nil, err
		}
		return oneShotStream(toolErrorFeedback(ctx, name, err)), nil
	}
	return guardedToolStream(sr, name), nil
}

// guardedToolStream 包装输出流：流中途错误不炸 run——中断/取消经 Pipe 错误位原样穿透，
// 其余错误保留已收内容+尾注回喂。
func guardedToolStream(sr *schema.StreamReader[string], name string) *schema.StreamReader[string] {
	pr, pw := schema.Pipe[string](4)
	go func() {
		defer sr.Close()
		defer pw.Close()
		for {
			chunk, err := sr.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if passthroughErr(err) {
					pw.Send("", err)
					return
				}
				pw.Send(fmt.Sprintf("\n（工具 %s 输出流中断，错误已回喂）%s", name, truncateRunes(err.Error(), 300)), nil)
				return
			}
			if pw.Send(chunk, nil) {
				return // 下游已提前关闭
			}
		}
	}()
	return pr
}

// oneShotStream 单条文本转流（cap=1 发送不阻塞，免 goroutine）。
func oneShotStream(text string) *schema.StreamReader[string] {
	pr, pw := schema.Pipe[string](1)
	pw.Send(text, nil)
	pw.Close()
	return pr
}
