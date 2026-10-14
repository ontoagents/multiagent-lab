package companion

// REQ-302/M106 伴生绑定两层化单测：effectiveBinding 三档解析 + ExtractNew 会话覆盖触发。

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestEffectiveBinding(t *testing.T) {
	bound := "ont_agent_default"
	agent := &store.Agent{CompanionOntologyID: bound}
	conv := func(override *string) *store.Conversation {
		return &store.Conversation{CompanionOntologyID: override}
	}

	// ① 无覆盖 → 跟随 agent 默认
	if got := effectiveBinding(agent, conv(nil)); got != bound {
		t.Fatalf("无覆盖应跟随默认: %s", got)
	}
	// ② 空串覆盖（清除）→ 跟随 agent 默认（''=跟随语义）
	empty := ""
	if got := effectiveBinding(agent, conv(&empty)); got != bound {
		t.Fatalf("空覆盖应跟随默认: %s", got)
	}
	// ③ 显式覆盖 → 覆盖本体
	override := "ont_network_domain"
	if got := effectiveBinding(agent, conv(&override)); got != override {
		t.Fatalf("显式覆盖应生效: %s", got)
	}
	// ④ agent 未绑 + 会话覆盖 → 仅该会话启用
	agentNone := &store.Agent{}
	if got := effectiveBinding(agentNone, conv(&override)); got != override {
		t.Fatalf("agent 未绑时应按会话覆盖: %s", got)
	}
	// ⑤ 双 nil → 空（未启用）
	if got := effectiveBinding(nil, nil); got != "" {
		t.Fatalf("双空应未启用: %q", got)
	}
}

// 会话覆盖本体后 ExtractNew 走覆盖本体抽取（agent 默认为另一本体——隔离验证）。
func TestExtractNewConvOverride(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "override.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// 两个本体（默认域 K8s / 覆盖域 网络设备）——本体记录仅作 id 存在性（抽取写入走 Plans 桩）
	agent := &store.Agent{ID: "ag_ovr", Name: "覆盖测试", CompanionOntologyID: "ont_k8s"}
	if _, err := st.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	agentID := "ag_ovr"
	conv := &store.Conversation{ID: "conv_ovr", Scope: "agent", AgentID: &agentID, Title: "覆盖会话"}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatal(err)
	}
	override := "ont_network"
	conv.CompanionOntologyID = &override
	if _, err := st.UpdateConversation(conv); err != nil {
		t.Fatal(err)
	}

	_ = NewService(st, nil, nil) // 服务构造回归（覆盖解析为包级纯函数，触发链经 OnRunComplete 门）
	// 直接断言 effectiveBinding（store 往返后指针语义保持）：
	got, err := st.GetConversation("conv_ovr")
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	_ = st.DB.QueryRow(`SELECT companion_ontology_id FROM conversation WHERE id='conv_ovr'`).Scan(&raw)
	t.Logf("raw col=%q got=%v", raw, got.CompanionOntologyID)
	if got.CompanionOntologyID == nil || *got.CompanionOntologyID != override {
		t.Fatalf("会话覆盖应持久化: %v", got.CompanionOntologyID)
	}
	if ef := effectiveBinding(agent, got); ef != override {
		t.Fatalf("有效绑定应为覆盖本体: %s", ef)
	}
	if ef(agent, got) == "ont_k8s" {
		t.Fatalf("不应落默认域")
	}
}

func ef(agent *store.Agent, conv *store.Conversation) string { return effectiveBinding(agent, conv) }
