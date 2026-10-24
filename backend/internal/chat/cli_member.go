package chat

// REQ-426/M222：外部 CLI 智能体成员委派工具——kind=cli 连接器（注册时 PATH 探测真握手）
// 作为 agent_as_tool 协作面的「外部成员」进入协调者工具面：执行复用 inference 适配器
// 子进程与 stream-json 解析层（claude-code 首件，opencode/aider/deepseek-harness 随矩阵），
// 最终文本回传作成员结果；失败/超时降级回喂沿 REQ-319 语义（CLI 子进程无平台内部中断
// 语义——审批/ask_human 挂起只存在于平台内部成员，无穿透分支）。委派单事件 channel=cli
//（a2a=channel a2a / 内部=channel internal），拓扑/泳道/委派面板据此区分内部/A2A/CLI 三类。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

// cliMemberDeps 外部 CLI 成员工具构建参数。
type cliMemberDeps struct {
	Name       string // 工具名（=连接器实例名；冲突时装配层加 -cli 后缀）
	Backend    string // inference 注册表内外部 CLI 后端名（claude-code 首件）
	Version    string // 注册探测入库的版本（工具描述合成用，可空）
	TaskPrefix string // config.task_prefix 角色化指令前缀（可空——成员约束随任务拼接）
	Run        func(ctx context.Context, task string) (string, int, error) // 成员执行（inference.CLIMemberRun 适配；taskPrefix 已并入）
}

// cliMemberDescription 工具描述合成（backend/版本/角色前缀如实透出给协调者）。
func cliMemberDescription(d cliMemberDeps) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("委派任务给外部 CLI 智能体（协议 CLI 子进程，后端 %s", d.Backend))
	if d.Version != "" {
		b.WriteString(" " + d.Version)
	}
	b.WriteString("）。")
	if p := strings.TrimSpace(d.TaskPrefix); p != "" {
		if len(p) > 200 {
			p = p[:200] + "…"
		}
		b.WriteString("该成员角色约束：" + p + " ")
	}
	b.WriteString(fmt.Sprintf("本工具名 %s。任务描述须自包含（外部成员看不到当前对话上下文，也看不到项目内其他资产）；结果是纯文本回传。", d.Name))
	return b.String()
}

// newCLIMemberTool 外部 CLI 成员委派工具（执行=同步等待子进程结束，上限沿 DelegateRunTimeout）。
func newCLIMemberTool(d cliMemberDeps) (einotool.BaseTool, error) {
	return utils.InferTool(d.Name, cliMemberDescription(d),
		func(ctx context.Context, in struct {
			Request string `json:"request" jsonschema:"委派任务描述（自包含——外部成员看不到当前对话上下文）"`
		}) (string, error) {
			// A3 同款委派单事件：channel=cli 区分内部/A2A/CLI（拓扑/泳道/委派面板消费）
			tool.EmitEvent(ctx, "subagent.delegate", map[string]any{
				"member":  d.Name,
				"channel": "cli",
				"backend": d.Backend,
				"task":    truncateRunes(in.Request, 200),
			})
			ctx, cancel := context.WithTimeout(ctx, DelegateRunTimeout)
			defer cancel()

			out, _, err := d.Run(ctx, in.Request)
			if err == nil {
				if strings.TrimSpace(out) == "" {
					out = fmt.Sprintf("（外部 CLI 成员 %s 执行完成，但未产生文本输出）", d.Name)
				}
				return out, nil
			}
			// REQ-319 同语义降级回喂：外部成员失败/超时不终止协调者 run——改道/重试/自答/
			// 如实说明由协调者裁量（CLI 子进程无平台内部中断语义，无穿透分支）
			detail := truncateRunes(err.Error(), 300)
			tool.EmitEvent(ctx, "run.warning", map[string]any{
				"message":       fmt.Sprintf("外部 CLI 成员 %q 执行失败，错误已回喂协调者（可换成员重试/自行完成/向用户说明）: %s", d.Name, detail),
				"member_failed": map[string]any{"member": d.Name, "channel": "cli", "backend": d.Backend, "error": detail},
			})
			payload, _ := json.Marshal(map[string]any{
				"error":    "member_failed",
				"member":   d.Name,
				"channel":  "cli",
				"backend":  d.Backend,
				"detail":   detail,
				"hint":     "该外部成员本次执行失败。你可以：换其他成员重试、分解任务后再次委派、自行完成任务，或向用户如实说明。",
			})
			return string(payload), nil
		})
}

// cliProbeVersion 注册探测 JSON 中的版本号（card 字段 cli-probe 载荷；非该形态返回空）。
func cliProbeVersion(cardJSON string) string {
	if cardJSON == "" {
		return ""
	}
	var p struct {
		Type    string `json:"type"`
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(cardJSON), &p) != nil || p.Type != "cli-probe" {
		return ""
	}
	return p.Version
}
