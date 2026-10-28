package chat

// REQ-453/M249：eino callbacks 统一执行观测面。
// obsHandler（callbacks.Handler：model/tool 的 OnStart/OnEnd/OnError/流式同型）经 Runner 级
// adk.WithCallbacks 挂接，耗时/错误/token（含 v0.9.21 cache write/read tokens）落 run_event
// （obs.timing 事件，SSE+落库同源）——tool 级含 MCP/委派计时补全（callbacks 计的是实际执行窗，
// 区别于 runner toolCallAt 墙钟差值含审批挂起时长）。
//
// debugModel 评估结论（REQ-453「退役或降级随实现评估」）：**退役不做、降级不做，维持现状**——
// 装配期包装覆盖全部 agent 含嵌套成员（agent_as_tool 子调用的模型调用），callbacks 经顶层
// Runner 注入不达嵌套 agent（eino filterCallbackHandlersForNestedAgents 设计），替代反缩覆盖面。
// 两者互补：debugModel=model.step 产品面（REQ-117 观测级别门控），obsHandler=obs.timing 审计面。

import (
	"context"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	platool "github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

const (
	obsComponentModel = "model"
	obsComponentTool  = "tool"
)

// obsHandler 统一执行观测 handler（REQ-453）。
type obsHandler struct {
	level DebugLevel
}

// Needed TimingChecker：level>0 才激活（与 model.step 同门控——level=0 旁路流拷贝开销）。
func (h *obsHandler) Needed(_ context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackTiming) bool {
	return h.level > 0
}

func (h *obsHandler) OnStart(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
	return context.WithValue(ctx, obsStartKey{}, time.Now())
}

func (h *obsHandler) OnEnd(ctx context.Context, ri *callbacks.RunInfo, out callbacks.CallbackOutput) context.Context {
	h.report(ctx, ri, out, nil)
	return ctx
}

func (h *obsHandler) OnError(ctx context.Context, ri *callbacks.RunInfo, err error) context.Context {
	h.report(ctx, ri, nil, err)
	return ctx
}

func (h *obsHandler) OnStartWithStreamInput(ctx context.Context, _ *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	input.Close() // 不读取：接口契约要求关闭本 handler 的拷贝（不关泄漏整条管线）
	return context.WithValue(ctx, obsStartKey{}, time.Now())
}

func (h *obsHandler) OnEndWithStreamOutput(ctx context.Context, ri *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	output.Close() // 流式 usage 随 debugModel model.step 已有，不重复消费流拷贝
	h.report(ctx, ri, nil, nil)
	return ctx
}

type obsStartKey struct{}

// report 输出一次 obs.timing（component/name/type/duration_ms/usage 含 cache tokens/error）。
// usage 仅非流式路径提取（model.CallbackOutput.TokenUsage）；tool 名取 RunInfo.Name。
func (h *obsHandler) report(ctx context.Context, ri *callbacks.RunInfo, out callbacks.CallbackOutput, err error) {
	if ri == nil {
		return
	}
	var component string
	switch ri.Component {
	case components.ComponentOfChatModel:
		component = obsComponentModel
	case components.ComponentOfTool:
		component = obsComponentTool
	default:
		return // agent/lambda/graph 级事件不在此面（agent 生命周期由 run.*/subagent.* 事件承载）
	}
	data := map[string]any{"component": component, "name": ri.Name, "type": ri.Type}
	if t0, ok := ctx.Value(obsStartKey{}).(time.Time); ok {
		data["duration_ms"] = time.Since(t0).Milliseconds()
	}
	if err != nil {
		data["error"] = err.Error()
	}
	if component == obsComponentModel && out != nil {
		if mo := model.ConvCallbackOutput(out); mo != nil && mo.TokenUsage != nil {
			u := mo.TokenUsage
			usage := map[string]any{
				"prompt_tokens":     u.PromptTokens,
				"completion_tokens": u.CompletionTokens,
				"total_tokens":      u.TotalTokens,
			}
			// v0.9.21+：cache 命中观测（非零才透出，省默认体积）
			if u.PromptTokenDetails.CachedTokens > 0 {
				usage["cached_tokens"] = u.PromptTokenDetails.CachedTokens
			}
			if u.PromptTokenDetails.CacheWriteTokens > 0 {
				usage["cache_write_tokens"] = u.PromptTokenDetails.CacheWriteTokens
			}
			data["usage"] = usage
		}
	}
	platool.EmitEvent(ctx, "obs.timing", data)
}
