package agenteval

// C4 Evaluator（REQ-388/M387——运行质量评估器）：对任意终态 run 做质量归因。
// 复用 REQ-223 Judge 契约（rubric 三维骨架/输出解析）不开新面——55 号既定口径；
// 评估输入=对话末轮 goal/answer 快照 + run 工具轨迹/警告摘要（store 侧组装，见 handlers_runs）。

import (
	"context"
	"fmt"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// runJudgeLLM 注入点（默认 chat.GenerateText；handler 级单测桩——沿 quizLLM 先例）。
var runJudgeLLM = chat.GenerateText

// SetRunJudge 测试注入钩子（仅测试装配——返回还原函数）。
func SetRunJudge(f func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error)) func() {
	old := runJudgeLLM
	runJudgeLLM = f
	return func() { runJudgeLLM = old }
}

const runJudgeSystem = `你是智能体运行质量评估器（C4）。你会收到：一次运行的目标（用户当轮输入）、智能体的最终回答、以及该运行的工具轨迹与警告摘要。
请只依据给定材料评估这次运行的质量，三维权衡：
1) 目标达成：最终回答是否正确且完整地回应了目标；
2) 过程合理性：工具调用是否服务于目标、收敛合理（无空转循环/无关调用）；无工具调用时按回答本身判断；
3) 诚实性：对做不到/不知道的部分是否如实声明，而非编造。
评分：2=pass（达成且过程合理）；1=partial（主体达成但有遗漏/小瑕疵/过程欠合理，或诚实性欠缺但未编造关键事实）；0=fail（答错、编造、拒答一个本可完成的任务、或过程严重失控）。
只输出一个 JSON 对象，不要输出任何其他文字：{"score": 0|1|2, "verdict": "pass|partial|fail", "reason": "一句话理由"}`

// RunEvalInput C4 评估输入（handlers 侧从 message/run_event 双源组装）。
type RunEvalInput struct {
	Goal     string
	Answer   string
	Tools    []string
	Warnings []string
	ErrMsg   string
}

// EvaluateRun C4 真机评估：默认模型连接（connID 空=GetDefaultConnection("chat")）。
// 输出契约与 JudgeAnswer 同形（ParseJudgeOutput）。
func EvaluateRun(ctx context.Context, st *store.Store, box *secrets.Box, connID string, in RunEvalInput) (JudgeResult, error) {
	out, err := runJudgeLLM(ctx, st, box, connID, runJudgeSystem, runJudgeUserPrompt(in), nil)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("run judge generate: %w", err)
	}
	return ParseJudgeOutput(out)
}

func runJudgeUserPrompt(in RunEvalInput) string {
	var b strings.Builder
	b.WriteString("## 运行目标（用户当轮输入）\n" + in.Goal + "\n")
	b.WriteString("\n## 工具调用轨迹\n")
	if len(in.Tools) == 0 {
		b.WriteString("（无工具调用）\n")
	} else {
		for _, name := range in.Tools {
			b.WriteString("- " + name + "\n")
		}
	}
	if len(in.Warnings) > 0 {
		b.WriteString("\n运行警告：" + strings.Join(in.Warnings, "；") + "\n")
	}
	if in.ErrMsg != "" {
		b.WriteString("\n运行错误：" + in.ErrMsg + "\n")
	}
	b.WriteString("\n## 智能体最终回答\n" + in.Answer + "\n")
	return b.String()
}
