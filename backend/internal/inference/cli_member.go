package inference

// REQ-426/M222：外部 CLI 智能体成员执行底座——kind=cli 连接器的成员委派复用既有
// cliAdapter 子进程骨架与 stream-json/纯文本解析层（不另起执行面，claude-code 首件，
// opencode/aider/deepseek-harness 随适配器矩阵）。

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PathProber CLI 适配器可选能力：PATH 文件级探测（exec.LookPath，无 --version 子进程）——
// 装配期轻量检查用（候选可执行名依次尝试，与 Probe 同一口径但不额外 spawn）。
type PathProber interface {
	LookPath() (string, error)
}

// LookPath 实现 cliAdapter 的 PathProber。
func (c *cliAdapter) LookPath() (string, error) {
	for _, b := range c.bin {
		if p, err := exec.LookPath(b); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("PATH 中未找到 %q", c.bin[0])
}

// CLIMemberRun 外部 CLI 成员一次委派执行：走适配器 Run（同一子进程与解析层），收集文本输出。
// 收集口径：stream-json 型（claude-code）result 行带 final 标记→取 result 为最终文本
// （避免 assistant 中间文本与 result 重复拼接）；纯文本型拼接全部 delta。
// 返回（结果文本, CLI 侧工具调用次数, 错误）。
func CLIMemberRun(ctx context.Context, b Backend, req *RunRequest) (string, int, error) {
	ca, ok := b.(*cliAdapter)
	if !ok {
		return "", 0, fmt.Errorf("后端 %q 不是外部 CLI 型（成员通道仅承载 CLI 适配器）", b.Name())
	}
	var text strings.Builder
	final := ""
	toolCalls := 0
	err := ca.Run(ctx, req, func(ev Event) {
		switch ev.Type {
		case "message.delta":
			if v, _ := ev.Data["final"].(bool); v {
				if d, _ := ev.Data["delta"].(string); d != "" {
					final = strings.TrimSpace(d)
				}
				return
			}
			if d, _ := ev.Data["delta"].(string); d != "" {
				text.WriteString(d)
			}
		case "tool.call":
			toolCalls++
		}
	})
	if err != nil {
		return "", toolCalls, err
	}
	out := final
	if out == "" {
		out = strings.TrimSpace(text.String())
	}
	return out, toolCalls, nil
}
