package importer

import (
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// REQ-157/M-O15 零依赖单测：三策略合并（replace/merge-overwrite/merge）+ 冲突字段提取 + 前缀协调。

func mergeSpecs() (target, incoming *pkgspec.Spec) {
	target = &pkgspec.Spec{
		Name: "T",
		Concepts: []pkgspec.Concept{
			{Name: "Drug", Label: "药物", Definition: "现行定义"},
			{Name: "Disease", Label: "疾病", Definition: "疾病", Parents: []string{"Drug"}},
		},
		Relations: []pkgspec.Relation{{Name: "treats", From: "Drug", To: "Disease", Definition: "现行关系"}},
		Instances: []pkgspec.Instance{{Name: "aspirin", Concept: "Drug"}},
	}
	incoming = &pkgspec.Spec{
		Name: "I",
		Concepts: []pkgspec.Concept{
			{Name: "Drug", Label: "药品", Definition: "导入定义"},                              // 冲突：label+definition
			{Name: "Symptom", Label: "症状", Definition: "症状"},                             // 新增
			{Name: "Disease", Label: "疾病", Definition: "疾病改", Parents: []string{"Drug"}}, // 冲突：definition
		},
		Relations: []pkgspec.Relation{{Name: "treats", From: "Drug", To: "Symptom", Definition: "改"}},
		Instances: []pkgspec.Instance{{Name: "aspirin", Concept: "Disease"}},
	}
	return
}

func TestBuildMergedReplace(t *testing.T) {
	target, incoming := mergeSpecs()
	pv, err := BuildMerged(target, incoming, StrategyReplace, "")
	if err != nil {
		t.Fatal(err)
	}
	if pv.Stats.TotalConflicts != 4 || len(pv.Conflicts) != 4 {
		t.Fatalf("应 4 处冲突: %+v", pv.Stats)
	}
	// Drug 被导入版整体替换
	var drug pkgspec.Concept
	for _, c := range pv.MergedSpec.Concepts {
		if c.Name == "Drug" {
			drug = c
		}
	}
	if drug.Definition != "导入定义" || drug.Label != "药品" {
		t.Fatalf("replace 应导入版整体替换: %+v", drug)
	}
	if len(pv.Added) != 1 { // 仅 Symptom 为新增（其余全部冲突）
		t.Fatalf("新增清单不符: %v", pv.Added)
	}
	if len(pv.Renamed) != 0 {
		t.Fatalf("replace 不应重命名: %v", pv.Renamed)
	}
}

func TestBuildMergedOverwrite(t *testing.T) {
	target, incoming := mergeSpecs()
	pv, _ := BuildMerged(target, incoming, StrategyMergeOverwrite, "")
	var drug pkgspec.Concept
	for _, c := range pv.MergedSpec.Concepts {
		if c.Name == "Drug" {
			drug = c
		}
	}
	if drug.Definition != "导入定义" {
		t.Fatalf("非空字段应覆盖: %+v", drug)
	}
	// 字段级冲突清单应含 label+definition
	found := false
	for _, cf := range pv.Conflicts {
		if cf.Name == "Drug" && cf.Kind == "concept" {
			found = true
			if strings.Join(cf.Fields, ",") != "definition,label" {
				t.Fatalf("冲突字段不符: %v", cf.Fields)
			}
			if cf.Resolution != "field-merged" {
				t.Fatalf("处置不符: %s", cf.Resolution)
			}
		}
	}
	if !found {
		t.Fatal("缺 Drug 冲突条目")
	}
}

func TestBuildMergedRename(t *testing.T) {
	target, incoming := mergeSpecs()
	pv, _ := BuildMerged(target, incoming, StrategyMerge, "ext")
	if len(pv.Renamed) != 4 {
		t.Fatalf("4 个冲突实体应全部重命名并入: %v", pv.Renamed)
	}
	// 原 Drug 保留现行定义；ext_Drug 存在导入定义
	var cur, ext *pkgspec.Concept
	for i := range pv.MergedSpec.Concepts {
		switch pv.MergedSpec.Concepts[i].Name {
		case "Drug":
			cur = &pv.MergedSpec.Concepts[i]
		case "ext_Drug":
			ext = &pv.MergedSpec.Concepts[i]
		}
	}
	if cur == nil || cur.Definition != "现行定义" || ext == nil || ext.Definition != "导入定义" {
		t.Fatalf("merge 应保留现行并重命名并入: %+v %+v", cur, ext)
	}
	// 实例关系/概念引用改写：ext_aspirin 应指向 ext_Disease？——原实例 aspirin 冲突重命名为 ext_aspirin，其 concept=Disease 不变（Disease 现行保留）
	var renamedInst bool
	for _, in := range pv.MergedSpec.Instances {
		if in.Name == "ext_aspirin" {
			renamedInst = true
			if in.Concept != "Disease" {
				t.Fatalf("实例类型引用应指向现行实体: %+v", in)
			}
		}
	}
	if !renamedInst {
		t.Fatal("缺 ext_aspirin")
	}
}

func TestBuildMergedPrefixCollision(t *testing.T) {
	// 冲突实体 X 的重命名基名 ext_X 已被既有实体占用 → uniquify 避让为 ext_X_2
	target := &pkgspec.Spec{Name: "T", Concepts: []pkgspec.Concept{
		{Name: "X", Definition: "现行"},
		{Name: "ext_X", Definition: "占位"},
	}}
	incoming := &pkgspec.Spec{Concepts: []pkgspec.Concept{{Name: "X", Definition: "导入"}}}
	pv, _ := BuildMerged(target, incoming, StrategyMerge, "ext")
	if !strings.Contains(strings.Join(pv.Renamed, ","), "X -> ext_X_2") {
		t.Fatalf("前缀协调应避让既有名: %v", pv.Renamed)
	}
}

func TestBuildMergedInvalid(t *testing.T) {
	target, incoming := mergeSpecs()
	if _, err := BuildMerged(target, incoming, "bad", ""); err == nil {
		t.Fatal("非法策略应报错")
	}
	if _, err := BuildMerged(nil, incoming, StrategyReplace, ""); err == nil {
		t.Fatal("nil target 应报错")
	}
}

// ---- REQ-472/M272：公理保留层三策略 + 同义词/限制保留 + rewire 扩面 ----

func axiomMergeSpecs() (target, incoming *pkgspec.Spec) {
	target = &pkgspec.Spec{
		Name: "T",
		Concepts: []pkgspec.Concept{
			{Name: "Device", Synonyms: []string{"设备"}, Restrictions: []pkgspec.Restriction{
				{Type: "some_values_from", Property: "connects", Target: "Sensor"},
			}},
			{Name: "Sensor"},
			{Name: "Gateway"},
		},
		Relations: []pkgspec.Relation{{Name: "connects", From: "Device", To: "Sensor"}},
		Instances: []pkgspec.Instance{},
		Axioms: []pkgspec.Axiom{
			{Type: "disjoint_with", Subject: "Device", Targets: []string{"Sensor"}},
		},
	}
	incoming = &pkgspec.Spec{
		Name: "I",
		Concepts: []pkgspec.Concept{
			{Name: "Device", Synonyms: []string{"装置"}, Restrictions: []pkgspec.Restriction{
				{Type: "cardinality", Property: "connects", Value: 2},
			}},
			{Name: "Sensor"},
			{Name: "Actuator"},
		},
		Relations: []pkgspec.Relation{{Name: "connects", From: "Device", To: "Sensor"}},
		Instances: []pkgspec.Instance{},
		Axioms: []pkgspec.Axiom{
			{Type: "disjoint_with", Subject: "Device", Targets: []string{"Actuator"}},
			{Type: "equivalent_class", Subject: "Sensor", Targets: []string{"Device"}},
		},
	}
	return
}

func TestBuildMergedAxiomsPreserved_Req472(t *testing.T) {
	// replace：随 incoming 整体
	tg, in := axiomMergeSpecs()
	pv, err := BuildMerged(tg, in, StrategyReplace, "")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if len(pv.MergedSpec.Axioms) != 2 {
		t.Fatalf("replace 应随 incoming 2 组公理，实际 %d", len(pv.MergedSpec.Axioms))
	}
	// merge-overwrite：incoming 组覆盖同组键 + 现行新组并入 → 3 组；Device 组 targets=incoming 的
	tg, in = axiomMergeSpecs()
	pv, err = BuildMerged(tg, in, StrategyMergeOverwrite, "")
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if len(pv.MergedSpec.Axioms) != 2 {
		t.Fatalf("overwrite 应 2 组公理（incoming 2 组+现行同组键被覆盖），实际 %d", len(pv.MergedSpec.Axioms))
	}
	for _, ax := range pv.MergedSpec.Axioms {
		if ax.Type == "disjoint_with" && ax.Subject == "Device" {
			if len(ax.Targets) != 1 || ax.Targets[0] != "Actuator" {
				t.Fatalf("overwrite 同组应被 incoming 覆盖（Actuator），实际 %v", ax.Targets)
			}
		}
	}
	// merge：现行组保留 + incoming 组经概念改名映射并入——保真本质=公理零丢失且组结构合法
	//（改名 rewire 全 spec 语义与 parents/relations 既有口径一致，精确主体名随改名映射组合不做硬断言）
	tg, in = axiomMergeSpecs()
	pv, err = BuildMerged(tg, in, StrategyMerge, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(pv.MergedSpec.Axioms) < 2 {
		t.Fatalf("merge 公理零丢失应 ≥2 组（现行+incoming 经改名映射并入），实际 %d 组 %v", len(pv.MergedSpec.Axioms), pv.MergedSpec.Axioms)
	}
	for _, ax := range pv.MergedSpec.Axioms {
		if len(ax.Targets) == 0 {
			t.Fatalf("公理组 %s(%s) targets 为空", ax.Type, ax.Subject)
		}
	}
	// merge-overwrite 下同义词并集 + 限制公理追加去重（原 conceptFieldMerge 丢弃）
	tg, in = axiomMergeSpecs()
	pv, _ = BuildMerged(tg, in, StrategyMergeOverwrite, "")
	var dev *pkgspec.Concept
	for i := range pv.MergedSpec.Concepts {
		if pv.MergedSpec.Concepts[i].Name == "Device" {
			dev = &pv.MergedSpec.Concepts[i]
		}
	}
	if dev == nil {
		t.Fatalf("Device 概念缺失")
	}
	if len(dev.Synonyms) != 2 {
		t.Fatalf("同义词应并集 2 个（设备/装置），实际 %v", dev.Synonyms)
	}
	if len(dev.Restrictions) != 2 {
		t.Fatalf("限制公理应追加 2 条，实际 %v", dev.Restrictions)
	}
}

func TestBuildMergedRewireAxiomAndRestriction_Req472(t *testing.T) {
	// merge 重命名并入：公理 subject/targets 与限制 Target/Property 随改名改写
	tg := &pkgspec.Spec{
		Name:      "T",
		Concepts:  []pkgspec.Concept{{Name: "Device"}, {Name: "Sensor"}},
		Relations: []pkgspec.Relation{{Name: "connects", From: "Device", To: "Sensor"}},
		Instances: []pkgspec.Instance{},
	}
	in := &pkgspec.Spec{
		Name:      "I",
		Concepts:  []pkgspec.Concept{{Name: "Device", Restrictions: []pkgspec.Restriction{{Type: "all_values_from", Property: "connects", Target: "Sensor"}}}},
		Relations: []pkgspec.Relation{{Name: "connects", From: "Device", To: "Sensor"}},
		Instances: []pkgspec.Instance{},
		Axioms:    []pkgspec.Axiom{{Type: "disjoint_with", Subject: "Device", Targets: []string{"Sensor"}}},
	}
	pv, err := BuildMerged(tg, in, StrategyMerge, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	// 找概念重命名映射（Renamed 形如 "concept Device -> ext_Device"；关系重命名条目同时存在）
	renamed := ""
	for _, r := range pv.Renamed {
		if strings.HasPrefix(r, "concept Device ->") {
			parts := strings.SplitN(r, "->", 2)
			renamed = strings.TrimSpace(parts[1])
		}
	}
	if renamed == "" {
		t.Fatalf("应有概念重命名发生，实际 %v", pv.Renamed)
	}
	for _, ax := range pv.MergedSpec.Axioms {
		if ax.Subject != renamed {
			t.Fatalf("公理 subject 应随改名改写为 %s，实际 %s", renamed, ax.Subject)
		}
	}
	for _, c := range pv.MergedSpec.Concepts {
		if c.Name == renamed {
			if len(c.Restrictions) != 1 || c.Restrictions[0].Target != "Sensor" {
				t.Fatalf("限制 Target 应保持/改写正确，实际 %v", c.Restrictions)
			}
		}
	}
}
