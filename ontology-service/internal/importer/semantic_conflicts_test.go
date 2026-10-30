package importer

// REQ-259/M258：语义级对齐检测单测——父类分歧/定义漂移/label 同义提示/零噪声边界。

import (
	"strings"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestSemanticConflicts_Req259(t *testing.T) {
	target := &pkgspec.Spec{
		Concepts: []pkgspec.Concept{
			{Name: "缺陷", Label: "Defect", Definition: "需求的遗漏或错误", Parents: []string{"问题"}},
			{Name: "模块", Label: "Module", Definition: "软件组成单元"},
			{Name: "客户", Label: "Customer"},
		},
	}
	incoming := &pkgspec.Spec{
		Concepts: []pkgspec.Concept{
			// ①父类分歧：现行父=[问题] vs 导入父=[需求项]
			{Name: "缺陷", Label: "Defect", Definition: "需求的遗漏或错误", Parents: []string{"需求项"}},
			// ②定义漂移：双方非空互异
			{Name: "模块", Label: "Module", Definition: "系统功能划分单位"},
			// ③label 同义：label=Customer 与现行「客户」同名不同概念名
			{Name: "甲方", Label: "Customer"},
			// 无冲突：同名同定义同父
			{Name: "客户", Label: "Customer"},
		},
	}
	got := semanticConflicts(target, incoming)
	kinds := map[string]int{}
	for _, c := range got {
		kinds[c.Kind]++
		if c.Severity == "conflict" && strings.TrimSpace(c.Detail) == "" {
			t.Fatalf("%s 缺 detail: %+v", c.Kind, c)
		}
	}
	if kinds["parent_divergence"] != 1 || kinds["definition_drift"] != 1 || kinds["label_synonym"] != 1 {
		t.Fatalf("三类各恰 1 条，得到 %v (%+v)", kinds, got)
	}
	// label 同义应指向 owner=客户
	if got[2].Entity != "甲方" || !strings.Contains(got[2].Detail, "客户") {
		t.Fatalf("label_synonym 指向不符: %+v", got[2])
	}
}

func TestSemanticConflicts_NoiseSafe_Req259(t *testing.T) {
	target := &pkgspec.Spec{Concepts: []pkgspec.Concept{
		{Name: "A", Definition: "", Parents: []string{"P"}}, // 现行有父
		{Name: "B", Definition: "定义B"},
	}}
	incoming := &pkgspec.Spec{Concepts: []pkgspec.Concept{
		// 单方空父=增补不算分歧；单方空定义=增补不算漂移；无 label 零提示
		{Name: "A", Definition: "", Parents: []string{"P", "Q"}},
		{Name: "B", Definition: ""},
	}}
	if got := semanticConflicts(target, incoming); len(got) != 0 {
		t.Fatalf("增补噪声不应报冲突: %+v", got)
	}
}

func TestBuildMergedSemanticHook_Req259(t *testing.T) {
	target := &pkgspec.Spec{Concepts: []pkgspec.Concept{{Name: "X", Definition: "旧定义"}}}
	incoming := &pkgspec.Spec{Concepts: []pkgspec.Concept{{Name: "X", Definition: "新定义（显著不同）"}}}
	pv, err := BuildMerged(target, incoming, StrategyMerge, "ext")
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.SemanticConflicts) == 0 {
		t.Fatal("MergePreview 应附语义冲突清单")
	}
	if pv.SemanticConflicts[0].Kind != "definition_drift" {
		t.Fatalf("kind 不符: %+v", pv.SemanticConflicts[0])
	}
	// 策略行为零变化：merge 保留现行
	if pv.MergedSpec.Concepts[0].Definition != "旧定义" {
		t.Fatal("merge 策略行为被语义预检改变（零变化口径破坏）")
	}
}
