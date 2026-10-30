package chat

// REQ-531/M338（P12）：注入预算复检——截断语义纯逻辑测试（runOnce 全链召回桩面大，
// 循环核心语义在此锚定：注入区起点=历史区之后/尾部优先删最早源/截断后入预算）。

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestInjectionBudgetTrimSemantics(t *testing.T) {
	budget := 100
	hist := []*schema.Message{schema.UserMessage(strings.Repeat("历", 50))}
	inj1 := schema.SystemMessage(strings.Repeat("甲", 80)) // 早期注入（先删）
	inj2 := schema.SystemMessage(strings.Repeat("乙", 80))
	msgs := append(append(hist, inj1), inj2)
	total := estimateMessages(msgs)
	if total <= budget {
		t.Fatalf("fixture 应超预算: %d", total)
	}
	histCount := len(hist)
	trimmed := append([]*schema.Message{}, msgs...)
	removedTokens := estimateMessages(trimmed[histCount : histCount+1])
	trimmed = append(trimmed[:histCount], trimmed[histCount+1:]...)
	if removedTokens <= 0 || estimateMessages(trimmed) >= total {
		t.Fatalf("截断后总量应下降: %d → %d", total, estimateMessages(trimmed))
	}
	if estimateMessages(trimmed) > budget {
		trimmed = append(trimmed[:histCount], trimmed[histCount+1:]...)
	}
	if estimateMessages(trimmed) > budget {
		t.Fatalf("两源全删后应入预算: %d", estimateMessages(trimmed))
	}
}
