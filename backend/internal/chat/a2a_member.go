package chat

// REQ-416/M212：外部 A2A 成员委派工具——kind=a2a 连接器（注册时 agent card 真握手）作为
// agent_as_tool 协作面的「外部成员」进入协调者工具面：执行走 message/send + tasks/get 轮询
// 至终态（无 message/stream 订阅，与 server 端 capabilities.streaming=false 对称如实），
// artifacts text parts 回传作成员结果；失败/超时降级回喂沿 REQ-319 语义（A2A 无中断语义，
// 无中断穿透分支——审批/ask_human 挂起只存在于平台内部成员）。委派单事件 channel=a2a
//（内部委派 channel=internal），拓扑/泳道/委派面板据此区分内外。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/a2a"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

// a2aMemberDeps 外部成员工具构建参数。
type a2aMemberDeps struct {
	Name     string // 工具名（=连接器实例名；冲突时装配层改名）
	Endpoint string // A2A JSON-RPC 端点（config.url）
	Headers  map[string]string
	CardJSON string  // 注册握手落库的 agent card 原文（工具描述合成源）
	PollSec  float64 // tasks/get 轮询间隔（config.poll_interval_seconds，缺省 2s）
	// DecorateTask REQ-430/M226：任务装饰闭包（context_injection=auto 时前置平台项目上下文；
	// nil=原样——委派单事件记录原始任务，装饰只影响外发内容）。
	DecorateTask func(ctx context.Context, task string) string
	// Streaming REQ-432/M228：对端 capabilities.streaming（card 能力随行——true 走 message/stream
	// SSE 订阅、失败回落 send+轮询；false 纯轮询沿现状）。
	Streaming bool
}

// a2aMemberDescription 卡片合成工具描述（skills+description 如实透出给协调者）。
func a2aMemberDescription(d a2aMemberDeps) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("委派任务给外部 A2A 智能体（协议 A2A，端点 %s）。", d.Endpoint))
	if d.CardJSON != "" {
		var card a2a.AgentCard
		if json.Unmarshal([]byte(d.CardJSON), &card) == nil {
			if card.Name != "" {
				b.WriteString(fmt.Sprintf("对端智能体名 %q。", card.Name))
			}
			desc := strings.TrimSpace(card.Description)
			if len(desc) > 200 {
				desc = desc[:200] + "…"
			}
			if desc != "" {
				b.WriteString("该智能体自述：" + desc + "。")
			}
			if len(card.Skills) > 0 {
				names := make([]string, 0, len(card.Skills))
				for _, sk := range card.Skills {
					if strings.TrimSpace(sk.Name) != "" {
						names = append(names, sk.Name)
					}
				}
				if len(names) > 0 {
					b.WriteString("技能：" + strings.Join(names, "、") + "。")
				}
			}
		}
	}
	b.WriteString(fmt.Sprintf("本工具名 %s。任务描述须自包含（外部成员看不到当前对话上下文）；结果是纯文本回传。", d.Name))
	return b.String()
}

// newA2AMemberTool 外部 A2A 成员委派工具（执行=同步等待远端终态，上限沿 DelegateRunTimeout）。
func newA2AMemberTool(d a2aMemberDeps) (einotool.BaseTool, error) {
	return utils.InferTool(d.Name, a2aMemberDescription(d),
		func(ctx context.Context, in struct {
			Request string `json:"request" jsonschema:"委派任务描述（自包含——外部成员看不到当前对话上下文）"`
		}) (string, error) {
			return a2aMemberInvoke(ctx, d, in.Request)
		})
}

// a2aMemberInvoke 外部 A2A 成员执行外壳（REQ-446/M242：agent_as_tool 委派工具与 workflow 节点/
// transfer 转移目标两轨共用——装配判定与执行语义单源防漂移）：A3 委派单事件 channel=a2a →
// DelegateRunTimeout 上限 → REQ-428 过程转译 hook → REQ-430 任务装饰 → 执行 → REQ-319 同语义
// 降级回喂（member_failed JSON 作结果文本不炸 run；工作流节点无协调者裁量面，降级载荷作节点
// 输出继续流转+run.warning 事件——下游节点/最终回答可见失败信息）。
func a2aMemberInvoke(ctx context.Context, d a2aMemberDeps, request string) (string, error) {
	// A3 同款委派单事件：channel=a2a 区分内部/外部（拓扑/泳道/委派面板消费）
	tool.EmitEvent(ctx, "subagent.delegate", map[string]any{
		"member":   d.Name,
		"channel":  "a2a",
		"endpoint": d.Endpoint,
		"task":     truncateRunes(request, 200),
	})
	ctx, cancel := context.WithTimeout(ctx, DelegateRunTimeout)
	defer cancel()

	// REQ-428/M224：过程转译——远端任务 history（agent 消息）逐条转 external.audit
	// run_event（沿 REQ-206 S3 先例；上限 20 条防刷屏，超出以尾注如实标注）
	hook := func(protoEvent, summary string) {
		tool.EmitEvent(ctx, "external.audit", map[string]any{
			"source": "a2a_member", "member": d.Name, "channel": "a2a",
			"proto_event": protoEvent, "summary": truncateRunes(summary, 200),
		})
	}
	task := request
	if d.DecorateTask != nil {
		task = d.DecorateTask(ctx, task)
	}
	out, err := a2aDelegateRun(ctx, d, task, hook)
	if err == nil {
		return out, nil
	}
	// REQ-319 同语义降级回喂：外部成员失败/超时不终止协调者 run——改道/重试/自答/
	// 如实说明由协调者裁量（A2A 无平台内部中断语义，无穿透分支）
	detail := truncateRunes(err.Error(), 300)
	tool.EmitEvent(ctx, "run.warning", map[string]any{
		"message":       fmt.Sprintf("外部 A2A 成员 %q 执行失败，错误已回喂协调者（可换成员重试/自行完成/向用户说明）: %s", d.Name, detail),
		"member_failed": map[string]any{"member": d.Name, "channel": "a2a", "error": detail},
	})
	payload, _ := json.Marshal(map[string]any{
		"error":    "member_failed",
		"member":   d.Name,
		"channel":  "a2a",
		"endpoint": d.Endpoint,
		"detail":   detail,
		"hint":     "该外部成员本次执行失败。你可以：换其他成员重试、分解任务后再次委派、自行完成任务，或向用户如实说明。",
	})
	return string(payload), nil
}

// newA2AMemberAgent 外部 A2A 成员的 adk.Agent 适配（REQ-446/M242）：workflow 节点与 transfer
// 转移目标消费外部成员——执行走同一 a2aMemberInvoke 外壳，描述复用工具轨卡片合成（transfer
// 协调者的转移决策按子代理描述路由，与工具描述同一信息面）。
func newA2AMemberAgent(d a2aMemberDeps) adk.Agent {
	return &externalMemberAgent{
		name:        d.Name,
		description: a2aMemberDescription(d),
		invoke: func(ctx context.Context, task string) (string, error) {
			return a2aMemberInvoke(ctx, d, task)
		},
	}
}

// a2aSendDone message/send 即回终态（同步型远端）判定。
func a2aSendDone(rt *a2a.RemoteTask) (done, failed bool) {
	if rt == nil {
		return true, true
	}
	switch rt.State {
	case "completed":
		return true, false
	case "failed", "canceled", "rejected", "input-required":
		return true, true
	}
	return false, false
}

// a2aDelegateRun 委派执行：对端声明流式能力时走 message/stream SSE 订阅（REQ-432①，失败回落
// send+轮询），否则纯轮询（send 即回终态的同步型远端跳过轮询）。
// REQ-428：hook 非 nil 时任务 history/stream 快照逐条转译过程事件（history 上限 20 条，超出尾注标注）。
func a2aDelegateRun(ctx context.Context, d a2aMemberDeps, task string, hook func(protoEvent, summary string)) (string, error) {
	poll := time.Duration(d.PollSec * float64(time.Second))
	// REQ-441/M237：ctx 超时/打断时 best-effort 取消远端任务（治弃单后远端白跑至自身超时——
	// 协议有 tasks/cancel 且平台 server 面已实现，client 面此前从未调用）
	var remoteID string
	defer func() {
		if ctx.Err() != nil && remoteID != "" {
			cctx, cc := context.WithTimeout(context.Background(), 10*time.Second)
			defer cc()
			_ = a2a.CancelTask(cctx, d.Endpoint, d.Headers, remoteID)
		}
	}()
	var rt *a2a.RemoteTask
	var err error
	if d.Streaming {
		lastArt := 0
		rt, err = a2a.SendStream(ctx, d.Endpoint, d.Headers, task, func(u *a2a.RemoteTask) {
			if hook == nil || u == nil {
				return
			}
			// 流式快照转译：产物增量 + 状态文本变化各一条（快照全量、按增量发防刷屏）
			if len(u.ArtifactText) > lastArt {
				hook("a2a_stream", u.ArtifactText[lastArt:])
				lastArt = len(u.ArtifactText)
			} else if strings.TrimSpace(u.ErrorText) != "" {
				hook("a2a_stream", "["+u.State+"] "+u.ErrorText)
			}
		})
		if err != nil {
			// 回落 send+轮询（对端实际不支持流式/中途断流——能力声明与实现的差异如实回落）
			rt, err = a2a.Send(ctx, d.Endpoint, d.Headers, task)
			if err != nil {
				return "", err
			}
		}
	} else {
		rt, err = a2a.Send(ctx, d.Endpoint, d.Headers, task)
		if err != nil {
			return "", err
		}
	}
	// REQ-432②：input-required 交互通道——外部成员人工介入请求不再按失败降级，结构化透传
	// 协调者（向用户转述问题获取答案后附答案再次委派；协调者有 ask_human 工具时可先走审批面/
	// 追问语义）。非失败态：不 emit run.warning；不进轮询（等人工输入的任务对端不会自行推进）。
	if rt.State == "input-required" {
		remoteID = rt.ID
		q := rt.ErrorText
		if q == "" {
			q = "（对端未附问题文本）"
		}
		payload, _ := json.Marshal(map[string]any{
			"status":   "input_required",
			"member":   d.Name,
			"question": truncateRunes(q, 400),
			"hint":     "外部成员请求人工输入。请向用户转述问题并获取答案，然后把答案附在任务描述中再次委派本成员（或改派其他成员/自行完成）。",
		})
		return string(payload), nil
	}
	if done, failed := a2aSendDone(rt); !done || failed {
		if rt.ID == "" {
			return "", err
		}
		remoteID = rt.ID
		rt, err = a2a.AwaitTerminal(ctx, d.Endpoint, d.Headers, rt.ID, poll)
		if err != nil {
			return "", err
		}
	}
	if hook != nil {
		const maxHistory = 20
		n := len(rt.History)
		for i, h := range rt.History {
			if i >= maxHistory {
				hook("a2a_history", fmt.Sprintf("（history 共 %d 条，仅转译前 %d 条）", n, maxHistory))
				break
			}
			if strings.TrimSpace(h.Text) != "" {
				role := h.Role
				if role == "" {
					role = "agent"
				}
				hook("a2a_history", "["+role+"] "+h.Text)
			}
		}
	}
	if rt.State != "completed" {
		detail := rt.ErrorText
		if detail == "" {
			detail = "远端任务终态 " + rt.State
		}
		return "", fmt.Errorf("远端任务 %s（%s）", rt.State, detail)
	}
	out := strings.TrimSpace(rt.ArtifactText)
	if out == "" {
		out = fmt.Sprintf("（外部 A2A 成员 %s 任务完成，但未回传文本产物）", d.Name)
	}
	return out, nil
}
