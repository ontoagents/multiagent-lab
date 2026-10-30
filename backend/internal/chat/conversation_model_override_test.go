package chat

import (
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// REQ-291：会话级模型覆盖合并序——请求级（RunInput）> 会话级（''=跟随）> 智能体默认。
func TestApplyConversationModelOverride(t *testing.T) {
	convID := "conv-1"
	agentConn := "conn-agent"
	base := &store.Agent{ID: "a1", Name: "t", ModelConnID: &agentConn}
	strPtr := func(s string) *string { return &s }

	t.Run("未设置/空串（跟随）原样返回", func(t *testing.T) {
		if got := ApplyConversationModelOverride(&store.Conversation{ID: convID}, base); got != base {
			t.Fatal("无覆盖字段应原样返回")
		}
		if got := ApplyConversationModelOverride(&store.Conversation{ID: convID, ModelConnID: strPtr("")}, base); got != base {
			t.Fatal("''=跟随应原样返回")
		}
		if got := ApplyConversationModelOverride(nil, base); got != base {
			t.Fatal("nil 会话应原样返回")
		}
	})

	t.Run("会话覆盖改写副本不改原值", func(t *testing.T) {
		got := ApplyConversationModelOverride(&store.Conversation{ID: convID, ModelConnID: strPtr("conn-conv")}, base)
		if got == base {
			t.Fatal("应返回副本")
		}
		if got.ModelConnID == nil || *got.ModelConnID != "conn-conv" {
			t.Fatalf("副本应为会话覆盖连接: %#v", got.ModelConnID)
		}
		if *base.ModelConnID != "conn-agent" {
			t.Fatal("原 Agent 不得被改写")
		}
	})

	t.Run("请求级叠加仍最高（REQ-174 优先序不回归）", func(t *testing.T) {
		overridden := ApplyConversationModelOverride(&store.Conversation{ID: convID, ModelConnID: strPtr("conn-conv")}, base)
		got := WithModelOverride(overridden, "conn-request")
		if got.ModelConnID == nil || *got.ModelConnID != "conn-request" {
			t.Fatalf("请求级应最高: %#v", got.ModelConnID)
		}
	})

	t.Run("对比窗格 '' 继承会话级、显式选择最高", func(t *testing.T) {
		// 基座已被会话级改写（handler 层物化），窗格 ''=继承 → 保持会话级
		base2 := ApplyConversationModelOverride(&store.Conversation{ID: convID, ModelConnID: strPtr("conn-conv")}, base)
		inherited := WithModelOverride(base2, "")
		if inherited.ModelConnID == nil || *inherited.ModelConnID != "conn-conv" {
			t.Fatalf("窗格空覆盖应继承会话级: %#v", inherited.ModelConnID)
		}
		// 窗格显式选择 → 覆盖会话级
		picked := WithModelOverride(base2, "conn-pane")
		if picked.ModelConnID == nil || *picked.ModelConnID != "conn-pane" {
			t.Fatalf("窗格显式选择应最高: %#v", picked.ModelConnID)
		}
	})
}
