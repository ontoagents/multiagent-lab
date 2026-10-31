package workflow

// REQ-566/M373 WF-⑤ 拓扑合成器单测（109 号验收①②③④钉桩）：
// 合成过 Validate/非法回喂 ≤2/连败降级线性链 degraded/同指纹缓存命中/超护栏中性文案。
import (
	"context"
	"strings"
	"testing"
)

const goodJSON = `{"name":"计划","nodes":[{"id":"a","type":"llm","name":"做"},{"id":"b","type":"llm","name":"核","join":"all"}],"edges":[{"from":"a","to":"b"}],"vars":[]}`

func TestSynthesizeOK_Req566(t *testing.T) {
	calls := 0
	res, err := Synthesize(context.Background(), "调研并写报告",
		func(ctx context.Context, prompt string) (string, error) {
			calls++
			return goodJSON, nil
		}, NewMemComposeCache())
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded || res.FromCache {
		t.Fatalf("首轮成功不应降级/命中: %+v", res)
	}
	if err := res.Spec.Validate(); err != nil {
		t.Fatalf("合成输出应过 Validate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("应一次合成: %d", calls)
	}
}

func TestSynthesizeFeedbackAndDegrade_Req566(t *testing.T) {
	// ① 非法输出回喂修复：首轮坏 JSON、次轮好 JSON
	calls := 0
	res, err := Synthesize(context.Background(), "任务甲",
		func(ctx context.Context, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return "不是 JSON", nil
			}
			return goodJSON, nil
		}, NewMemComposeCache())
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded {
		t.Fatal("回喂修复不应降级")
	}
	if calls != 2 {
		t.Fatalf("回喂后应 2 次调用: %d", calls)
	}
	// ② 连败 3 轮（初+2 回喂）→ 降级线性链 degraded（验收④）
	calls2 := 0
	res2, err := Synthesize(context.Background(), "任务乙",
		func(ctx context.Context, prompt string) (string, error) {
			calls2++
			return `{"nodes":[{"id":"a","type":"llm"},{"id":"b","type":"llm"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`, nil // 环
		}, NewMemComposeCache())
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Degraded {
		t.Fatal("连败应降级线性链")
	}
	if len(res2.Notes) == 0 || !strings.Contains(res2.Notes[0], "降级") {
		t.Fatalf("degraded 注记失真: %v", res2.Notes)
	}
	if calls2 != ComposeMaxFeedback+1 {
		t.Fatalf("连败调用数应 %d: %d", ComposeMaxFeedback+1, calls2)
	}
	// 降级链本身合法（线性三节点）
	if err := res2.Spec.Validate(); err != nil {
		t.Fatalf("降级线性链应合法: %v", err)
	}
}

func TestSynthesizeCache_Req566(t *testing.T) {
	cache := NewMemComposeCache()
	calls := 0
	llm := func(ctx context.Context, prompt string) (string, error) {
		calls++
		return goodJSON, nil
	}
	// 首次合成写缓存
	if _, err := Synthesize(context.Background(), "同一任务描述", llm, cache); err != nil {
		t.Fatal(err)
	}
	// 二次同描述 → 指纹命中不重合成（验收③）
	res, err := Synthesize(context.Background(), "同一任务描述", llm, cache) // 完全同描述
	if err != nil {
		t.Fatal(err)
	}
	if !res.FromCache {
		t.Fatal("同指纹应命中缓存")
	}
	if calls != 1 {
		t.Fatalf("命中后不应重合成: %d", calls)
	}
	// 不同描述 → 未命中
	res2, err := Synthesize(context.Background(), "另一任务", llm, cache)
	if err != nil {
		t.Fatal(err)
	}
	if res2.FromCache {
		t.Fatal("不同描述不应命中")
	}
	// 指纹归一语义由 Fingerprint 函数直测（下方单测）
}

func TestSynthesizeGuardrails_Req566(t *testing.T) {
	// 超护栏（节点数超上限）→ 回喂后仍超 → 降级（中性文案在 Validate 人话——验收②）
	var big strings.Builder
	big.WriteString(`{"nodes":[`)
	for i := 0; i < MaxNodes+5; i++ {
		if i > 0 {
			big.WriteString(",")
		}
		big.WriteString(`{"id":"n`)
		big.WriteString(strings.Repeat("x", 0) + itoa(i))
		big.WriteString(`","type":"llm"}`)
	}
	big.WriteString(`],"edges":[]}`)
	res, err := Synthesize(context.Background(), "超大计划",
		func(ctx context.Context, prompt string) (string, error) { return big.String(), nil },
		NewMemComposeCache())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Degraded {
		t.Fatal("超护栏连败应降级")
	}
	// 空任务/无执行体中性人话
	if _, err := Synthesize(context.Background(), "", nil, nil); err == nil || !strings.Contains(err.Error(), "任务描述为空") {
		t.Fatalf("空任务人话: %v", err)
	}
	if _, err := Synthesize(context.Background(), "t", nil, nil); err == nil {
		t.Fatal("无执行体应报错")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

func TestFingerprint_Req566(t *testing.T) {
	if Fingerprint("同一 任务描述") != Fingerprint("同一   任务描述") {
		t.Fatal("空白归一后应同指纹")
	}
	if Fingerprint("任务甲") == Fingerprint("任务乙") {
		t.Fatal("不同描述应不同指纹")
	}
}
