package workflow

// REQ-552/M377 DAG 计划与调度——规划者产计划 JSON（Synthesize）→复用 WF-① Validate+
// WF-② Runner 执行（113 号归一轮路线 A）。dag.plan.created/finished 事件经 OnEvent 透出
// （degraded=降级线性链标注；cache 命中=跳过合成）。委派型节点（delegate）经 ExecLLM
// 执行体承载 v1（子会话后端形态随装配层扩展）。
import (
	"context"
	"fmt"
)

// PlanAndExecute REQ-552 核心管线：Synthesize（规划者）→ Validate → Runner 执行。
// 返回合成结果（含 degraded/from_cache）+执行结果。
func PlanAndExecute(ctx context.Context, task string, llm ComposeLLM, cache ComposeCache, execLLM, execTool Executor, onEvent func(string, map[string]any), parallelLimit int) (*ComposeResult, *RunResult, error) {
	// ① 规划者产计划 JSON（WF-⑤ Synthesize——回喂 ≤2 轮+降级线性链）
	cr, err := Synthesize(ctx, task, llm, cache)
	if err != nil {
		return nil, nil, fmt.Errorf("计划合成失败: %w", err)
	}
	// ② dag.plan.created（计划层事件——degraded/cache 标注）
	if onEvent != nil {
		onEvent("dag.plan.created", map[string]any{
			"degraded": cr.Degraded, "from_cache": cr.FromCache,
			"node_count": len(cr.Spec.Nodes), "task": truncStr(task, 100),
		})
	}
	// ③ WF-① Validate 已在 Synthesize 内过——Runner 再次校验（防御性）
	// ④ WF-② Runner 执行
	runner := NewRunner(RunnerDeps{
		Spec: cr.Spec, Vars: NewVarTable(), OnEvent: onEvent,
		ExecLLM: execLLM, ExecTool: execTool, ParallelLimit: parallelLimit,
	})
	res, err := runner.Run(ctx, task)
	if onEvent != nil {
		finished := "completed"
		if err != nil {
			finished = "failed"
		}
		onEvent("dag.plan.finished", map[string]any{
			"degraded": cr.Degraded, "status": finished,
			"skipped": len(res.Skipped), "failed": len(res.Failed),
		})
	}
	return cr, res, err
}
