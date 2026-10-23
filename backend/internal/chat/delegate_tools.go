package chat

// REQ-208/M42 余项：可继续子 Agent（continuation）协调者工具族。
// agent_as_tool 项目模式下与一次性同步委派（成员名直调工具，REQ-319 包装）并列：
//   delegate_background  后台委派——发起后协调者不阻塞，立即拿 delegate_id 继续
//   delegate_status      查进度
//   delegate_steer       中途引导——向运行中子智能体追加指令（轮界接续消费）
//   delegate_collect     收取结果（摘要 ≤2000 rune，全文在子会话）
//   delegate_interrupt   打断
//   delegate_continue    冷恢复/续跑（重启孤儿委派从子会话历史接续）
// 归属守卫：delegate_id 必须属于装配时绑定的父会话（跨会话操作拒绝）。

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// DelegateToolDeps 后台委派工具族依赖（装配期绑定父会话与可委派成员）。
type DelegateToolDeps struct {
	Manager      *DelegateManager
	Store        *store.Store
	ParentConvID string
	Members      []*store.Agent // agent_as_tool 子代理集合（成员名校验）
}

// delegateBackgroundTools 产出六件工具（协调者工具面；Manager 未装配时返回 nil）。
func delegateBackgroundTools(deps DelegateToolDeps) []einotool.BaseTool {
	if deps.Manager == nil || deps.Store == nil || deps.ParentConvID == "" || len(deps.Members) == 0 {
		return nil
	}
	memberNames := ""
	for i, m := range deps.Members {
		if i > 0 {
			memberNames += "、"
		}
		memberNames += m.Name
	}
	belong := func(delegateID string) (*store.Delegate, error) {
		d, err := deps.Store.GetDelegate(delegateID)
		if err != nil {
			return nil, fmt.Errorf("委派 %s 不存在", delegateID)
		}
		if d.ParentConversationID != deps.ParentConvID {
			return nil, fmt.Errorf("委派 %s 不属于当前会话（归属守卫）", delegateID)
		}
		return d, nil
	}

	// 1) delegate_background
	backgroundTool, err := utils.InferTool("delegate_background",
		fmt.Sprintf("后台委派任务给成员智能体（%s 之一）：立即返回不阻塞，之后用 delegate_status 查进度、delegate_steer 插话引导、delegate_collect 收结果、delegate_interrupt 打断。任务描述须自包含。", memberNames),
		func(ctx context.Context, in struct {
			Member string `json:"member" jsonschema:"成员智能体名（须为本项目成员）"`
			Task   string `json:"task" jsonschema:"委派任务描述（自包含——子智能体看不到当前对话上下文）"`
		}) (struct {
			Error                string `json:"error,omitempty"`
			DelegateID           string `json:"delegate_id,omitempty"`
			ChildConversationID  string `json:"child_conversation_id,omitempty"`
			Member               string `json:"member,omitempty"`
			Status               string `json:"status,omitempty"`
			Receipt              string `json:"receipt,omitempty"`
		}, error) {
			out := struct {
				Error                string `json:"error,omitempty"`
				DelegateID           string `json:"delegate_id,omitempty"`
				ChildConversationID  string `json:"child_conversation_id,omitempty"`
				Member               string `json:"member,omitempty"`
				Status               string `json:"status,omitempty"`
				Receipt              string `json:"receipt,omitempty"`
			}{}
			var member *store.Agent
			for _, m := range deps.Members {
				if m.Name == in.Member {
					member = m
					break
				}
			}
			if member == nil {
				out.Error = fmt.Sprintf("成员 %q 不在可委派清单（%s）", in.Member, memberNames)
				return out, nil
			}
			parent, err := deps.Store.GetConversation(deps.ParentConvID)
			if err != nil {
				out.Error = "父会话不存在"
				return out, nil
			}
			d, err := deps.Manager.Start(parent, member, in.Task)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.DelegateID, out.ChildConversationID, out.Member, out.Status = d.ID, d.ChildConversationID, d.MemberName, d.Status
			out.Receipt = "已在后台发起（协调者可继续其他工作）；delegate_id=" + d.ID + "，用 delegate_status/delegate_steer/delegate_collect 跟进"
			return out, nil
		})
	if err != nil {
		return nil
	}

	// 2) delegate_status
	statusTool, err := utils.InferTool("delegate_status",
		"查询后台委派的当前状态（running|completed|failed|stopped）与结果摘要头部。",
		func(ctx context.Context, in struct {
			DelegateID string `json:"delegate_id" jsonschema:"required delegate_background 返回的委派 id"`
		}) (struct {
			Error        string `json:"error,omitempty"`
			Status       string `json:"status,omitempty"`
			Member       string `json:"member,omitempty"`
			Task         string `json:"task,omitempty"`
			ResultDigest string `json:"result_digest,omitempty"`
			SteerPending bool   `json:"steer_pending,omitempty"`
		}, error) {
			out := struct {
				Error        string `json:"error,omitempty"`
				Status       string `json:"status,omitempty"`
				Member       string `json:"member,omitempty"`
				Task         string `json:"task,omitempty"`
				ResultDigest string `json:"result_digest,omitempty"`
				SteerPending bool   `json:"steer_pending,omitempty"`
			}{}
			d, err := belong(in.DelegateID)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.Status, out.Member, out.Task, out.ResultDigest = d.Status, d.MemberName, d.Task, truncateRunesBG(d.ResultDigest)
			out.SteerPending = d.SteerNote != ""
			return out, nil
		})
	if err != nil {
		return nil
	}

	// 3) delegate_steer
	steerTool, err := utils.InferTool("delegate_steer",
		"中途引导：向后台委派的子智能体追加指令。运行中→本轮结束后自动续跑一轮消费插话；已结束→直接以插话续跑。",
		func(ctx context.Context, in struct {
			DelegateID string `json:"delegate_id" jsonschema:"required 委派 id"`
			Message    string `json:"message" jsonschema:"required 追加给子智能体的指令"`
		}) (struct {
			Error   string `json:"error,omitempty"`
			Status  string `json:"status,omitempty"`
			Receipt string `json:"receipt,omitempty"`
		}, error) {
			out := struct {
				Error   string `json:"error,omitempty"`
				Status  string `json:"status,omitempty"`
				Receipt string `json:"receipt,omitempty"`
			}{}
			if _, err := belong(in.DelegateID); err != nil {
				out.Error = err.Error()
				return out, nil
			}
			d, err := deps.Manager.Steer(in.DelegateID, in.Message)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.Status = d.Status
			if d.Status == "running" {
				out.Receipt = "插话已受理：当前轮结束后自动续跑一轮消费（子智能体将看到该指令）"
			} else {
				out.Receipt = "委派已收束：已以插话为输入续跑一轮"
			}
			return out, nil
		})
	if err != nil {
		return nil
	}

	// 4) delegate_collect
	collectTool, err := utils.InferTool("delegate_collect",
		"收取后台委派结果：返回状态与最终回复摘要（全文在子会话，可引用子会话 id 指向用户）。",
		func(ctx context.Context, in struct {
			DelegateID string `json:"delegate_id" jsonschema:"required 委派 id"`
		}) (struct {
			Error        string `json:"error,omitempty"`
			Status       string `json:"status,omitempty"`
			ResultDigest string `json:"result_digest,omitempty"`
			ChildConv    string `json:"child_conversation_id,omitempty"`
		}, error) {
			out := struct {
				Error        string `json:"error,omitempty"`
				Status       string `json:"status,omitempty"`
				ResultDigest string `json:"result_digest,omitempty"`
				ChildConv    string `json:"child_conversation_id,omitempty"`
			}{}
			if _, err := belong(in.DelegateID); err != nil {
				out.Error = err.Error()
				return out, nil
			}
			d, full, err := deps.Manager.Collect(in.DelegateID)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.Status, out.ChildConv = d.Status, d.ChildConversationID
			if d.Error != "" {
				out.Error = d.Error
			}
			out.ResultDigest = truncateRunesBG(full)
			return out, nil
		})
	if err != nil {
		return nil
	}

	// 5) delegate_interrupt
	interruptTool, err := utils.InferTool("delegate_interrupt",
		"打断后台委派：停止子智能体当前轮（stopped 留档，可后续 delegate_continue 恢复）。",
		func(ctx context.Context, in struct {
			DelegateID string `json:"delegate_id" jsonschema:"required 委派 id"`
		}) (struct {
			Error   string `json:"error,omitempty"`
			Status  string `json:"status,omitempty"`
			Receipt string `json:"receipt,omitempty"`
		}, error) {
			out := struct {
				Error   string `json:"error,omitempty"`
				Status  string `json:"status,omitempty"`
				Receipt string `json:"receipt,omitempty"`
			}{}
			if _, err := belong(in.DelegateID); err != nil {
				out.Error = err.Error()
				return out, nil
			}
			d, err := deps.Manager.Interrupt(in.DelegateID)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.Status = d.Status
			out.Receipt = "已打断（子会话历史完整，可 delegate_continue 恢复）"
			return out, nil
		})
	if err != nil {
		return nil
	}

	// 6) delegate_continue
	continueTool, err := utils.InferTool("delegate_continue",
		"恢复/续跑后台委派（冷恢复）：从中断点接续子智能体上下文继续任务（backend 重启后的委派恢复同此入口）。",
		func(ctx context.Context, in struct {
			DelegateID  string `json:"delegate_id" jsonschema:"required 委派 id"`
			Instruction string `json:"instruction" jsonschema:"可选续跑指令（空=默认「继续此前任务」）"`
		}) (struct {
			Error   string `json:"error,omitempty"`
			Status  string `json:"status,omitempty"`
			Receipt string `json:"receipt,omitempty"`
		}, error) {
			out := struct {
				Error   string `json:"error,omitempty"`
				Status  string `json:"status,omitempty"`
				Receipt string `json:"receipt,omitempty"`
			}{}
			if _, err := belong(in.DelegateID); err != nil {
				out.Error = err.Error()
				return out, nil
			}
			d, err := deps.Manager.Continue(in.DelegateID, in.Instruction)
			if err != nil {
				out.Error = err.Error()
				return out, nil
			}
			out.Status = d.Status
			out.Receipt = "已发起续跑（从子会话历史接续）"
			return out, nil
		})
	if err != nil {
		return nil
	}

	return []einotool.BaseTool{backgroundTool, statusTool, steerTool, collectTool, interruptTool, continueTool}
}
