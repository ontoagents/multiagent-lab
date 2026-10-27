package store

// REQ-451 单测：①基座工具清单扩容（EnsureAssistantTools 并入 doc_list/doc_grep）；
// ②SyncBuiltinAssistantPrompt 基座升级同步（历史基座覆写/真实微调不动/幂等零写入）。

import (
	"path/filepath"
	"testing"
)

func TestEnsureAssistantToolsReq451Base(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 存量形态：仅旧基座八工具（REQ-451 前的 030 形态）
	a, _ := st.GetAgent("builtin-assistant")
	a.Tools = []string{"doc_read", "list_model_connections", "list_agents", "list_kbs",
		"get_assistant_config", "sync_platform_kb", "search_platform_kb", "propose_assistant_config"}
	if _, err := st.UpdateAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureAssistantTools(); err != nil {
		t.Fatal(err)
	}
	a, _ = st.GetAgent("builtin-assistant")
	has := map[string]bool{}
	for _, tl := range a.Tools {
		has[tl] = true
	}
	for _, want := range []string{"doc_list", "doc_grep"} {
		if !has[want] {
			t.Fatalf("基座自愈应并入 %s：%v", want, a.Tools)
		}
	}
	if len(a.Tools) != 10 {
		t.Fatalf("基座应为 10 工具：%v", a.Tools)
	}
}

func TestSyncBuiltinAssistantPrompt(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.EnsureBuiltinAssistantInstruction("BASE-V2"); err != nil {
		t.Fatal(err)
	}

	// 行内 = 历史基座 → 覆写为当前基座
	a, _ := st.GetAgent("builtin-assistant")
	a.Instruction = "BASE-V1"
	if _, err := st.UpdateAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncBuiltinAssistantPrompt("BASE-V2", []string{"BASE-V1"}); err != nil {
		t.Fatal(err)
	}
	a, _ = st.GetAgent("builtin-assistant")
	if a.Instruction != "BASE-V2" {
		t.Fatalf("历史基座应覆写为当前基座：%q", a.Instruction)
	}

	// 幂等：已是当前基座零写入（无变化）
	if err := st.SyncBuiltinAssistantPrompt("BASE-V2", []string{"BASE-V1"}); err != nil {
		t.Fatal(err)
	}
	if a, _ = st.GetAgent("builtin-assistant"); a.Instruction != "BASE-V2" {
		t.Fatalf("幂等调用不应改写：%q", a.Instruction)
	}

	// 用户真实微调（不等于任何历史基座）永不动
	a, _ = st.GetAgent("builtin-assistant")
	a.Instruction = "我的自定义提示词"
	if _, err := st.UpdateAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncBuiltinAssistantPrompt("BASE-V2", []string{"BASE-V1"}); err != nil {
		t.Fatal(err)
	}
	if a, _ = st.GetAgent("builtin-assistant"); a.Instruction != "我的自定义提示词" {
		t.Fatalf("用户微调不应被覆写：%q", a.Instruction)
	}
}
