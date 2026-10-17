package buildflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// captureTracer 捕获留痕供断言（SetTracer 接管口径同 REQ-328 对账持久化）。
type captureTracer struct {
	mu     sync.Mutex
	traces []SegmentTrace
}

func (c *captureTracer) Emit(t SegmentTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traces = append(c.traces, t)
}

func withCapture(t *testing.T) *captureTracer {
	t.Helper()
	c := &captureTracer{}
	old := tracer
	SetTracer(c)
	t.Cleanup(func() { SetTracer(nil); tracer = old }) // 恢复默认 LogTracer
	return c
}

func TestRunSequencesAndTraces(t *testing.T) {
	cap := withCapture(t)
	var order []Stage
	err := Run(context.Background(), "ai-draft",
		Segment{Stage: StageSource, Run: func(context.Context) (string, error) { order = append(order, StageSource); return "描述摘要", nil }},
		Segment{Stage: StageExtract, Skip: true},
		Segment{Stage: StageDraft, Run: func(context.Context) (string, error) { order = append(order, StageDraft); return "rounds=2", nil }},
		Segment{Stage: StageGate, Run: func(context.Context) (string, error) { order = append(order, StageGate); return "overall=90.0", nil }},
	)
	if err != nil {
		t.Fatalf("Run 不应报错: %v", err)
	}
	if len(order) != 3 || order[0] != StageSource || order[1] != StageDraft {
		t.Fatalf("执行序不符: %v", order)
	}
	if len(cap.traces) != 4 {
		t.Fatalf("留痕应 4 条（含 skip）: %d", len(cap.traces))
	}
	if cap.traces[1].Status != "skip" {
		t.Fatalf("extract 段应 skip: %+v", cap.traces[1])
	}
	if cap.traces[0].Pipeline != "ai-draft" || cap.traces[0].Note != "描述摘要" {
		t.Fatalf("首段留痕字段不符: %+v", cap.traces[0])
	}
}

func TestRunFailsFastAndTraces(t *testing.T) {
	cap := withCapture(t)
	called := false
	err := Run(context.Background(), "kb-extract",
		Segment{Stage: StageSource, Run: func(context.Context) (string, error) { return "KB x", nil }},
		Segment{Stage: StageExtract, Run: func(context.Context) (string, error) { return "", errors.New("LLM 超时") }},
		Segment{Stage: StageDraft, Run: func(context.Context) (string, error) { called = true; return "", nil }},
	)
	if err == nil {
		t.Fatal("应透出段错误")
	}
	if called {
		t.Fatal("失败段后不应继续执行")
	}
	var fail, after int
	for _, tr := range cap.traces {
		if tr.Status == "fail" {
			fail++
			if tr.Note != "LLM 超时" {
				t.Fatalf("失败留痕应带错误 note: %+v", tr)
			}
		}
		if tr.Stage == StageDraft {
			after++
		}
	}
	if fail != 1 || after != 0 {
		t.Fatalf("fail=%d 失败后段=%d（应 1/0）", fail, after)
	}
}

func TestRegistryCovers(t *testing.T) {
	for _, p := range []string{"ai-draft", "ontoextend-draft", "kb-extract", "cq-rag-draft"} {
		if !Covers(p, StageSource) || !Covers(p, StageIngest) {
			t.Fatalf("%s 应覆盖 source/ingest 全段", p)
		}
	}
	// REQ-346/M146：cq-benchmark 为评测管线（报告落 artifact 不动仓库资产）——只声明 source/draft，无 ingest
	if !Covers("cq-benchmark", StageSource) || !Covers("cq-benchmark", StageDraft) || Covers("cq-benchmark", StageIngest) {
		t.Fatal("cq-benchmark 应只覆盖 source/draft（评测管线无入库段）")
	}
	if Covers("未知管线", StageSource) {
		t.Fatal("未登记管线不应覆盖任何段")
	}
	if len(Registered()) != 5 {
		t.Fatalf("注册表应 5 实例: %v", Registered())
	}
	// 未声明段：Run 跳过不留痕
	cap := withCapture(t)
	if err := Run(context.Background(), "未知管线", Segment{Stage: StageSource, Run: func(context.Context) (string, error) { return "", nil }}); err != nil {
		t.Fatalf("未声明段不应报错: %v", err)
	}
	if len(cap.traces) != 0 {
		t.Fatalf("未声明段不应留痕: %d", len(cap.traces))
	}
}

func TestEmitAtPreservesRemoteTiming(t *testing.T) {
	cap := withCapture(t)
	started := time.Now().UTC().Add(-1500 * time.Millisecond)
	EmitAt(SegmentTrace{Pipeline: "kb-extract", Stage: StageDraft, Status: "", StartedAt: started, DurationMS: 1500, Note: "概念 12 · 关系 8"})
	if len(cap.traces) != 1 {
		t.Fatalf("应透传 1 条: %d", len(cap.traces))
	}
	tr := cap.traces[0]
	if tr.Status != "ok" { // 空 status 补 ok
		t.Fatalf("空 status 应补 ok: %s", tr.Status)
	}
	if tr.DurationMS != 1500 || !tr.StartedAt.Equal(started) {
		t.Fatalf("远端计时应原样保留: %+v", tr)
	}
}
