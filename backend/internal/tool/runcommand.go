package tool

// REQ-225②/M53 沙箱执行纵深：run_command 通用命令原语（sh -c）。
// 定位：沙箱执行世界（agentd 容器内，SANDBOX_EXEC_WORLD=1）的受控命令执行——三前置
// （沙箱✅/绑定目录✅/审批✅，REQ-202 B1 口径）由装配层把关（chat/assembler.go），本工具
// 只负责执行与输出治理：cmdGuard 危险模式拒绝（hooks.go 规则表，pre-hook 消费 REQ-232③）、
// 超时（默认 30s 上限 120s）、输出截断（头部保留，防 stdout 洪泛撑爆上下文）。
// 平台主进程（inprocess 世界）恒不装配——宿主机执行世界禁入，装配层硬闸 + 本工具 env 双保险。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	utils "github.com/cloudwego/eino/components/tool/utils"

	"github.com/ontoagents/ontoagents/backend/internal/fsutil"
)

// RunCommandDeps run_command 装配依赖（WorkDir=绑定目录，空=容器工作目录）。
type RunCommandDeps struct {
	WorkDir   string        // 命令工作目录（=volume 绑定的 agent work_dir；须绝对路径）
	Timeout   time.Duration // 单命令超时（空=30s；上限 120s）
	MaxOutput int           // 输出保留 rune 数（空=8K，头部截断）
}

// sandboxExecWorld 当前进程是否处于沙箱执行世界（agentd 容器内装配期置位）。
func sandboxExecWorld() bool { return os.Getenv("SANDBOX_EXEC_WORLD") == "1" }

// SandboxExecWorld 导出判断（装配层闸门用；测试可经 t.Setenv 控制）。
func SandboxExecWorld() bool { return sandboxExecWorld() }

type runCommandIn struct {
	Command string `json:"command" jsonschema:"description=要执行的命令（sh -c 语义，可含管道与重定向）"`
}

// NewRunCommandTool 构造 run_command 工具（仅沙箱执行世界可实例化——双保险）。
func NewRunCommandTool(d RunCommandDeps) (einotool.BaseTool, error) {
	if !sandboxExecWorld() {
		return nil, fmt.Errorf("run_command 仅在沙箱执行世界可用（SANDBOX_EXEC_WORLD 未置位）")
	}
	if d.WorkDir != "" && !fsutil.IsAbsDir(d.WorkDir) { // 跨平台绝对判定（REQ-321：POSIX "/" 与 Windows 盘符均合法；原 strings.HasPrefix 仅认 "/"）
		return nil, fmt.Errorf("run_command 工作目录须为绝对路径: %q", d.WorkDir)
	}
	if d.Timeout <= 0 {
		d.Timeout = 30 * time.Second
	}
	if d.Timeout > 120*time.Second {
		d.Timeout = 120 * time.Second
	}
	if d.MaxOutput <= 0 {
		d.MaxOutput = 8 * 1024
	}
	desc := fmt.Sprintf("在沙箱工作目录内执行一条 shell 命令（sh -c），返回 JSON {exit_code,output}。危险命令会被 cmdGuard 拒绝；超时 %ds；输出超长截断。用于构建、测试、查看环境等本地任务。", int(d.Timeout.Seconds()))
	return utils.InferTool("run_command", desc, func(ctx context.Context, in runCommandIn) (map[string]any, error) {
		if strings.TrimSpace(in.Command) == "" {
			return map[string]any{"error": "缺少 command 参数"}, nil
		}
		// cmdGuard 双保险（装配链 pre-hook 已查一次；此处兜底直调路径）
		if ok, why := CommandGuard(in.Command); !ok {
			return map[string]any{"denied": why + "（cmdGuard）"}, nil
		}
		cctx, cancel := context.WithTimeout(ctx, d.Timeout)
		defer cancel()
		cmd := exec.CommandContext(cctx, "sh", "-c", in.Command)
		if d.WorkDir != "" {
			cmd.Dir = d.WorkDir
		}
		out, err := cmd.CombinedOutput()
		exit := 0
		if err != nil {
			// 超时优先判定（进程被 kill 也包成 ExitError，先查 deadline 再取退出码）
			if cctx.Err() == context.DeadlineExceeded {
				exit = 124
				out = append(out, []byte("\n[run_command] 超时（"+d.Timeout.String()+"）")...)
			} else if ee, ok := err.(*exec.ExitError); ok {
				exit = ee.ExitCode()
			} else {
				return map[string]any{"error": err.Error()}, nil
			}
		}
		text := strings.TrimSpace(string(out))
		rn := []rune(text)
		if len(rn) > d.MaxOutput {
			text = string(rn[:d.MaxOutput]) + fmt.Sprintf("\n…[输出截断，共 %d 字符]", len(rn))
		}
		return map[string]any{"exit_code": exit, "output": text}, nil
	})
}
