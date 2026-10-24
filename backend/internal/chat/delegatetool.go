package chat

// REQ-319/M121（78 号《多智能体与子智能体执行支持与调度编排分析》P1 批）成员委派包装器：
//   A1 错误降级回喂——成员执行失败（LLM 报错/迭代耗尽等）不再终止整个 run，而是把结构化错误
//      作为 tool result 回喂协调者（可改道/重试/向用户说明）+ run.warning 事件留痕；
//      **中断类错误（审批/ask_human 挂起，CompositeInterrupt 链路）必须原样上抛**——吞掉会破坏
//      checkpoint 恢复语义（判定：adk.InterruptSignal 别名 errors.As + compose.ExtractInterruptInfo
//      + IsInterruptRerunError 三重覆盖，isInterruptErr）；
//   A2 per-member 超时——成员 agent 行 delegate_timeout_seconds>0 时对单次委派 context.WithTimeout，
//      超时走同 A1 降级路径（此前成员可无限占用协调者迭代，78 号 G2）；
//   A3 委派单事件——委派发起时发 subagent.delegate 结构化事件（成员+任务摘要），拓扑/审计一等数据
//      （此前任务描述只在 tool.call arguments 字符串里，78 号 G7）。
// 包装保持 Invokable-only 接口面不虚增流式（同 emptyArgsTool 纪律；adk AgentTool 为 Invokable-only）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

// delegateTool 成员委派包装（协调者成员工具专用；单 agent 直聊路径不经过此包装）。
type delegateTool struct {
	inner   einotool.InvokableTool
	member  string        // 成员名（委派事件与降级回喂标注用）
	timeout time.Duration // 单次委派超时；0=不限（REQ-319 A2）
}

func newDelegateTool(inner einotool.BaseTool, member string, timeout time.Duration) einotool.BaseTool {
	it, ok := inner.(einotool.InvokableTool)
	if !ok {
		return inner // 非 Invokable 形态原样返回（当前 adk AgentTool 恒为 Invokable-only）
	}
	return delegateTool{inner: it, member: member, timeout: timeout}
}

func (t delegateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}

func (t delegateTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...einotool.Option) (string, error) {
	// REQ-319 A3：委派单事件（任务摘要截断 200 字，解析失败回退原文截断）；
	// REQ-416 起 channel 区分内外（internal=平台内部成员，a2a=外部 A2A 成员）
	tool.EmitEvent(ctx, "subagent.delegate", map[string]any{
		"member":  t.member,
		"channel": "internal",
		"task":    truncateRunes(delegateTaskOf(argsInJSON), 200),
	})

	// REQ-319 A2：per-member 超时
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	text, err := t.inner.InvokableRun(ctx, argsInJSON, opts...)
	if err == nil {
		return text, nil
	}
	// REQ-319 A1：中断类（审批/ask_human 挂起）原样上抛——checkpoint 恢复依赖该错误形态穿透
	if isInterruptErr(err) {
		return "", err
	}
	detail := truncateRunes(err.Error(), 300)
	tool.EmitEvent(ctx, "run.warning", map[string]any{
		"message":       fmt.Sprintf("成员智能体 %q 执行失败，错误已回喂协调者（可换成员重试/自行完成/向用户说明）: %s", t.member, detail),
		"member_failed": map[string]any{"member": t.member, "error": detail},
	})
	payload, _ := json.Marshal(map[string]any{
		"error":  "member_failed",
		"member": t.member,
		"detail": detail,
		"hint":   "该成员本次执行失败。你可以：换其他成员重试、分解任务后再次委派、自行完成任务，或向用户如实说明。",
	})
	return string(payload), nil
}

// isInterruptErr 中断类错误判定（三重覆盖：InterruptSignal / 图级 interruptError / rerun 形态）。
// 委派包装与本包工具容错包装（toolerrors.go）共用。
func isInterruptErr(err error) bool {
	var sig *adk.InterruptSignal
	if errors.As(err, &sig) {
		return true
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	if _, ok := compose.IsInterruptRerunError(err); ok {
		return true
	}
	return false
}

// delegateTaskOf 从委派入参提取任务描述（agent_tool 默认 schema 为 {"request": "..."}；解析失败回退原文）。
func delegateTaskOf(argsInJSON string) string {
	var m map[string]any
	if json.Unmarshal([]byte(argsInJSON), &m) == nil {
		if r, ok := m["request"].(string); ok && r != "" {
			return r
		}
	}
	return argsInJSON
}
