package workflow

import (
	"context"
	"testing"
)

func TestPlanAndExecute_Req552(t *testing.T) {
	ce := &collectEvents{}
	res, runRes, err := PlanAndExecute(
		context.Background(),
		"调研并写报告",
		func(ctx context.Context, prompt string) (string, error) { return goodJSON, nil },
		NewMemComposeCache(),
		func(ctx context.Context, n Node, in string) (string, error) { return n.ID + "完成", nil },
		nil,
		ce.on, 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded {
		t.Fatal("不应降级")
	}
	if runRes == nil || runRes.FinalOutput == "" {
		t.Fatalf("执行结果失真: %+v", runRes)
	}
	if ce.count("dag.plan.created") != 1 || ce.count("dag.plan.finished") != 1 {
		t.Fatalf("计划层事件失真: created=%d finished=%d",
			ce.count("dag.plan.created"), ce.count("dag.plan.finished"))
	}
	// 降级路径
	res2, runRes2, err2 := PlanAndExecute(
		context.Background(), "任务乙",
		func(ctx context.Context, prompt string) (string, error) { return `{"nodes":[{"id":"a","type":"llm"},{"id":"b","type":"llm"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`, nil },
		NewMemComposeCache(),
		func(ctx context.Context, n Node, in string) (string, error) { return n.ID + "ok", nil },
		nil, ce.on, 2,
	)
	if err2 != nil {
		t.Fatal(err2)
	}
	if !res2.Degraded {
		t.Fatal("连败应降级")
	}
	if runRes2 == nil || runRes2.FinalOutput == "" {
		t.Fatalf("降级链执行失真: %+v", runRes2)
	}
}
