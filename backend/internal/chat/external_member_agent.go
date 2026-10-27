package chat

// REQ-446/M242：外部成员（CLI/A2A）的 adk.Agent 适配桥——workflow 节点与 transfer 转移目标
// 消费外部成员（REQ-416 诚实边界收口：混合编排 agent_as_tool/workflow/transfer 全轨成立）。
// agent_as_tool 轨外部成员=委派工具（REQ-416/426），本轨=确定性节点/转移目标；执行复用同一
// invoke 外壳（a2aMemberInvoke/cliMemberInvoke——委派单事件/超时/REQ-319 降级回喂语义两轨一致）。
// 事件沿 AgentName 流转：runner trackAgent 按事件 AgentName 自动发 subagent.enter/exit 与
// workflow.node started/completed（WorkflowMeta.Nodes 含外部成员名即得节点级进度）。

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// externalMemberAgent 外部成员 adk.Agent 适配（Eino Agent 三方法最小实现）。
type externalMemberAgent struct {
	name        string
	description string
	invoke      func(ctx context.Context, task string) (string, error)
}

func (a *externalMemberAgent) Name(ctx context.Context) string        { return a.name }
func (a *externalMemberAgent) Description(ctx context.Context) string { return a.description }

// Run 同步执行外部成员并把结果作单条 assistant 消息输出（非流式——外部成员结果为纯文本回传）。
// invoke 极少返回 error（成员失败已在 invoke 内降级为载荷文本）；此分支兜底防 nil 输出。
func (a *externalMemberAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		out, err := a.invoke(ctx, externalTaskFromMessages(input.Messages))
		if err != nil {
			gen.Send(&adk.AgentEvent{AgentName: a.name, Err: err})
			return
		}
		gen.Send(&adk.AgentEvent{AgentName: a.name, Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: false,
				Message:     &schema.Message{Role: schema.Assistant, Content: out},
			},
		}})
	}()
	return iter
}

// externalTaskFromMessages 取末条用户消息文本作外部成员任务（退化：任一非空内容→空串）。
// 外部成员上下文隔离语义不变——工作流/transfer 轨同样只拿到一段自包含任务文本，看不到
// 平台会话其他节点产物全文（与 agent_as_tool 轨「任务描述须自包含」同一口径）。
func externalTaskFromMessages(msgs []*schema.Message) string {
	lastAny := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		if lastAny == "" {
			lastAny = m.Content
		}
		if m.Role == schema.User {
			return m.Content
		}
	}
	return lastAny
}
