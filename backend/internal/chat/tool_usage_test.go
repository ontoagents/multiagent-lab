package chat

// REQ-397/M193：per-tool 差值归因单测（settleUsageDelta 纯逻辑）。
import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSettleUsageDelta(t *testing.T) {
	rc := &runConsumer{toolCallUsage: map[string]*usageSnapshot{}, toolUsageAgg: map[string]int{}, toolAgg: map[int]*pendingToolCall{}}
	rc.toolAgg[0] = &pendingToolCall{ID: "c1", Name: "http_fetch"}
	rc.toolAgg[1] = &pendingToolCall{ID: "c2", Name: "read_file"}

	// 起步 usage=100；发两调用（快照=100）；下一步 usage=400 → delta=300 均摊 150/150
	rc.toolCallUsage["c1"] = &usageSnapshot{Total: 100}
	rc.toolCallUsage["c2"] = &usageSnapshot{Total: 100}
	rc.settleUsageDelta(&usageSnapshot{Total: 100}, &schema.TokenUsage{TotalTokens: 400})
	if rc.toolUsageAgg["http_fetch"] != 150 || rc.toolUsageAgg["read_file"] != 150 {
		t.Fatalf("均摊不符: %+v", rc.toolUsageAgg)
	}
	if len(rc.toolCallUsage) != 0 {
		t.Fatal("未决应清空")
	}

	// 余数归首个：3 调用 delta=10 → 4/3/3
	rc2 := &runConsumer{toolCallUsage: map[string]*usageSnapshot{}, toolUsageAgg: map[string]int{}, toolAgg: map[int]*pendingToolCall{}}
	rc2.toolAgg[0] = &pendingToolCall{ID: "a", Name: "t1"}
	rc2.toolAgg[1] = &pendingToolCall{ID: "b", Name: "t2"}
	rc2.toolAgg[2] = &pendingToolCall{ID: "c", Name: "t3"}
	for _, id := range []string{"a", "b", "c"} {
		rc2.toolCallUsage[id] = &usageSnapshot{Total: 0}
	}
	rc2.settleUsageDelta(&usageSnapshot{Total: 0}, &schema.TokenUsage{TotalTokens: 10})
	if rc2.toolUsageAgg["t1"] != 4 || rc2.toolUsageAgg["t2"] != 3 || rc2.toolUsageAgg["t3"] != 3 {
		t.Fatalf("余数归首个不符: %+v", rc2.toolUsageAgg)
	}

	// 负 delta（重试/缓存场景）：钳 0
	rc3 := &runConsumer{toolCallUsage: map[string]*usageSnapshot{}, toolUsageAgg: map[string]int{}, toolAgg: map[int]*pendingToolCall{}}
	rc3.toolAgg[0] = &pendingToolCall{ID: "x", Name: "t"}
	rc3.toolCallUsage["x"] = &usageSnapshot{Total: 500}
	rc3.settleUsageDelta(&usageSnapshot{Total: 500}, &schema.TokenUsage{TotalTokens: 100})
	if rc3.toolUsageAgg["t"] != 0 {
		t.Fatalf("负 delta 应钳 0: %+v", rc3.toolUsageAgg)
	}
}
