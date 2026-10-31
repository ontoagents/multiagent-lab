// platform_cli 内置工具（REQ-561/M368 平台助手管理面 CLI 化；111 号方案档）——
// 专用 invokable 工具直 exec 可信自家二进制 ontoapi（manifest 驱动管理面命令表）。
// 安全口径：仅打 127.0.0.1 后端（信任级别同助手后端自身——**不 require 沙箱**，区别
// run_command 三前提 REQ-202 B1）；读写分级与 CLI 同轮交付（不先通后管）：ro 直行/
// rw danger 审批（approvalTool 包装复用审批卡）/deny 拒绝（工具侧+CLI 侧双闸防绕）。
package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/compose"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/ontoapi"
)

// PlatformCLIBin ontoapi 二进制路径（装配层注入——main.go 侧 PATH 查找 or 同目录部署）。
var PlatformCLIBin string

func init() {
	schema.RegisterName[*PlatformCLIState]("eino_multiagent_lab_tool_platform_cli_state")
}

type platformCLIIn struct {
	Command string   `json:"command" jsonschema_description:"ontoapi 子命令（agent_create/conversation_list/stats_usage 等——先 platform_cli command=help 查全部命令与分级；未知命令默认拒绝）"`
	Args    []string `json:"args,omitempty" jsonschema_description:"子命令参数（路径参数按序；k=v 查询；JSON body 字符串）"`
}

type platformCLIOut struct {
	Output string `json:"output" jsonschema_description:"命令输出（JSON 或人话错误）"`
}

// NewPlatformCLITool 构造 platform_cli 工具（REQ-561；二进制缺失不装配——诚实降级）。
func NewPlatformCLITool() (einotool.BaseTool, error) {
	bin := PlatformCLIBin
	if bin == "" {
		if p, err := exec.LookPath("ontoapi"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		return nil, fmt.Errorf("platform_cli: ontoapi 二进制不可达（PATH 或 PlatformCLIBin 未配置）")
	}
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("platform_cli: ontoapi 二进制不可达（%s）", bin)
	}
	return utils.InferTool("platform_cli",
		"平台管理面 CLI（对话执行任何页面可做的管理操作乃至组合操作）。command=ontoapi 子命令；args=参数数组。分级：ro 只读直行；rw 写操作会挂起等待用户审批（审批卡附完整命令）；deny 类（删除/密文导出）不可用。先 command=help 查看全部命令、参数与分级。",
		func(ctx context.Context, in platformCLIIn) (*platformCLIOut, error) {
			return platformCLIFn(ctx, in, bin)
		})
}

func platformCLIFn(ctx context.Context, in platformCLIIn, bin string) (*platformCLIOut, error) {
	// 恢复重入路径：审批通过 → 执行；拒绝 → 诚实回执（沿 approvalTool 口径）
	if was, _, st := compose.GetInterruptState[*PlatformCLIState](ctx); was {
		if isTarget, hasData, dec := compose.GetResumeContext[string](ctx); isTarget && hasData {
			if strings.TrimSpace(dec) == ApprovalApprove {
				EmitEvent(ctx, "approval.granted", map[string]any{
					"tool_name": "platform_cli", "args_digest": argsDigest(in.Command),
					"decision": "approve", "decision_source": "manual",
				})
				return runCLI(bin, in.Command, in.Args)
			}
			EmitEvent(ctx, "approval.denied", map[string]any{
				"tool_name": "platform_cli", "args_digest": argsDigest(in.Command),
				"decision": "deny", "decision_source": "manual",
			})
			return &platformCLIOut{Output: `{"approved":false,"message":"用户拒绝了本次管理操作；请勿重复调用，请询问用户希望如何调整。"}`}, nil
		}
		if st != nil {
			// 未被定向恢复 → 原样再中断
			return nil, compose.StatefulInterrupt(ctx, &ApprovalInfo{ToolName: "platform_cli", Arguments: st.Command}, st)
		}
	}
	cmd := strings.TrimSpace(in.Command)
	if cmd == "" {
		return nil, fmt.Errorf("command 必填（先 command=help 查看可用命令）")
	}
	// help 直行（可发现性——助手按需查询不占常驻上下文）
	if cmd == "help" {
		return runCLI(bin, "--help", nil)
	}
	// 工具侧分级判读（deny 双闸之一；CLI 侧同判）——ro 直行；rw 走审批包装外层
	ep, access, known := ontoapi.Grade(cmd)
	if !known || access == ontoapi.Deny {
		EmitEvent(ctx, "hook.denied", map[string]any{
			"guard": "platform_cli_grade", "tool_name": "platform_cli",
			"reason": ontoapi.DenyReason(cmd),
		})
		return &platformCLIOut{Output: ontoapi.DenyReason(cmd)}, nil
	}
	if access == ontoapi.Rw {
		// rw：StatefulInterrupt 挂起审批（审批卡附完整命令+参数摘要——111 号口径）
		full := strings.TrimSpace(cmd + " " + strings.Join(in.Args, " "))
		return nil, compose.StatefulInterrupt(ctx, &ApprovalInfo{ToolName: "platform_cli", Arguments: full}, &PlatformCLIState{Command: full})
	}
	_ = ep
	return runCLI(bin, cmd, in.Args)
}

func runCLI(bin, cmd string, args []string) (*platformCLIOut, error) {
	full := append([]string{cmd}, args...)
	c := exec.Command(bin, full...)
	out, err := c.CombinedOutput()
	if err != nil {
		// CLI 侧人话错误（deny/HTTP 归一）走 stderr+非零退出——透传不包装
		_ = err
		return &platformCLIOut{Output: strings.TrimSpace(string(out))}, nil
	}
	return &platformCLIOut{Output: strings.TrimSpace(string(out))}, nil
}

// PlatformCLIState 审批挂起持久化（checkpoint gob）。
type PlatformCLIState struct {
	Command string `json:"command"`
}
