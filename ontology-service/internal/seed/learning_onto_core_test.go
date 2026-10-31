package seed

// REQ-544/M351 决策层 dogfooding 种子单测——onto_core.json 过 LoadLearningExample（Validate 全链）
// +5 条决策结构完整（id 唯一/question 非空/槽位与拒绝路径齐备）。

import (
	"testing"
)

func TestOntoCoreSeedDecisions_Req544(t *testing.T) {
	sp, err := LoadLearningExample("onto_core")
	if err != nil {
		t.Fatalf("onto_core 种子应过 Validate 全链: %v", err)
	}
	if len(sp.Decisions) != 5 {
		t.Fatalf("应有 5 条决策: %d", len(sp.Decisions))
	}
	ids := map[string]bool{}
	for i, d := range sp.Decisions {
		if d.ID == "" || d.DecisionQuestion == "" {
			t.Fatalf("决策 %d 结构不完整: %+v", i, d)
		}
		if ids[d.ID] {
			t.Fatalf("决策 id 重复: %s", d.ID)
		}
		ids[d.ID] = true
		if len(d.RequiredSlots) == 0 || len(d.RejectRoutes) == 0 {
			t.Fatalf("决策 %s 槽位/拒绝路径不应为空（dogfooding 种子须完备示范）", d.ID)
		}
	}
	want := []string{"dec_add_synonyms", "dec_run_cq_screen", "dec_companion_promote", "dec_split_ontology", "dec_rename_via_chat"}
	for _, w := range want {
		if !ids[w] {
			t.Fatalf("缺少预期决策: %s", w)
		}
	}
	// LearningExamples 清单自动收录（embed 目录扫描——登记零手工表）
	found := false
	for _, e := range LearningExamples() {
		if e.Key == "onto_core" {
			found = true
		}
	}
	if !found {
		t.Fatal("onto_core 应出现在学习示例清单")
	}
}
