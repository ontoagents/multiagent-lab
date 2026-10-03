package ontobuild

import (
	"strings"
	"testing"
)

// M-O14 P2⑤：结构化→骨架映射推导——CSV/JSON 双源 + 目标本体命中标注 + 实例采样上限
func TestInferStructuredDraftCSV(t *testing.T) {
	csv := "设备编号,设备名称,功率\nEQ-001,空压机A,75\nEQ-002,水泵B,15\n"
	targets := [][2]string{{"设备名称", "Device Name"}}
	d, err := InferStructuredDraft("devices.csv", csv, targets)
	if err != nil {
		t.Fatal(err)
	}
	if d.SourceKind != "csv" || d.MainConcept != "设备实体" {
		t.Fatalf("主概念推导不符: %s / %s", d.SourceKind, d.MainConcept)
	}
	if len(d.Mapping) != 3 || d.Mapping[0].Role != "instance-name" || d.Mapping[1].InferType != "string" || d.Mapping[2].InferType != "number" {
		t.Fatalf("映射/类型推断不符: %+v", d.Mapping)
	}
	if len(d.Mapping[1].MatchedCon) != 1 || d.Mapping[1].MatchedCon[0] != "设备名称" {
		t.Fatalf("目标概念命中不符: %+v", d.Mapping[1])
	}
	if len(d.Draft.Instances) != 2 || d.Draft.Instances[0].Name != "EQ-001" {
		t.Fatalf("实例骨架不符: %+v", d.Draft.Instances)
	}
	if d.Draft.Instances[0].Attributes["功率"] != "75" {
		t.Fatalf("属性映射不符: %+v", d.Draft.Instances[0].Attributes)
	}
}

func TestInferStructuredDraftJSON(t *testing.T) {
	js := `[{"host":"web-1","cpu":4},{"host":"db-1","cpu":16}]`
	d, err := InferStructuredDraft("hosts.json", js, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.SourceKind != "json" || d.MainConcept != "host实体" || len(d.Draft.Instances) != 2 {
		t.Fatalf("JSON 推导不符: %s / %s / %d", d.SourceKind, d.MainConcept, len(d.Draft.Instances))
	}
	if d.Mapping[1].InferType != "number" {
		t.Fatalf("number 推断不符: %+v", d.Mapping)
	}
	if !strings.Contains(d.Notes[0], "REQ-82") {
		t.Fatalf("诚实注记缺失: %v", d.Notes)
	}
}

// REQ-256（60 号 H4）：模板模式——枚举列→概念层级批量生成。
func TestInferTemplateHierarchy(t *testing.T) {
	csv := "资产编号,类别,子类别,状态,说明\n" +
		"SV-001,服务,数据库,运行中,主库\n" +
		"SV-002,服务,缓存,运行中,Redis\n" +
		"HW-001,硬件,服务器,停机,机架1\n" +
		"HW-002,硬件,服务器,运行中,机架2\n"
	d, err := InferStructuredDraftMode("assets.csv", csv, InferStructuredDraftOpts{Mode: "template", HierarchyColumns: []string{"类别", "子类别"}})
	if err != nil {
		t.Fatalf("template 推导失败: %v", err)
	}
	if d.Mode != "template" {
		t.Fatalf("Mode 应为 template，实际 %q", d.Mode)
	}
	// 显式层级列：类别→子类别（状态列不参与链，留 attributes）
	if len(d.Draft.Concepts) != 5 {
		t.Fatalf("层级概念应 5（服务/硬件 + 数据库/缓存/服务器），实际 %d", len(d.Draft.Concepts))
	}
	// 子类别父链：数据库→服务、缓存→服务、服务器→硬件
	parents := map[string][]string{}
	for _, c := range d.Draft.Concepts {
		parents[c.Name] = c.Parents
	}
	if len(parents["数据库"]) != 1 || parents["数据库"][0] != "服务" {
		t.Fatalf("数据库 父应为 服务，实际 %v", parents["数据库"])
	}
	if len(parents["服务器"]) != 1 || parents["服务器"][0] != "硬件" {
		t.Fatalf("服务器 父应为 硬件，实际 %v", parents["服务器"])
	}
	// 首列高基数 → 实例挂最细层概念（数据库/缓存/服务器）
	if len(d.Draft.Instances) != 4 {
		t.Fatalf("应生成 4 实例，实际 %d", len(d.Draft.Instances))
	}
	for _, inst := range d.Draft.Instances {
		if inst.Concept == "" {
			t.Fatalf("实例 %s 应挂最细层概念", inst.Name)
		}
		if _, has := inst.Attributes["状态"]; !has {
			t.Fatalf("非层级列应保留 attributes: %v", inst.Attributes)
		}
	}
	// mapping 报告：层级列 role=concept-level 带 level
	levels := map[string]int{}
	for _, m := range d.Mapping {
		if m.Role == "concept-level" {
			levels[m.Column] = m.Level
		}
	}
	if levels["类别"] != 1 || levels["子类别"] != 2 {
		t.Fatalf("层级序应 类别=1/子类别=2，实际 %v", levels)
	}
}

// REQ-256：无枚举列自动回落实例骨架模式。
func TestInferTemplateFallsBackToInstance(t *testing.T) {
	csv := "资产编号,单价,说明\nA-1,100,甲\nA-2,200,乙\nA-3,300,丙\n"
	d, err := InferStructuredDraftMode("items.csv", csv, InferStructuredDraftOpts{Mode: "template"})
	if err != nil {
		t.Fatalf("推导失败: %v", err)
	}
	if d.Mode != "instance" {
		t.Fatalf("无枚举列应回落 instance 模式，实际 %q", d.Mode)
	}
	if len(d.Draft.Concepts) != 1 {
		t.Fatalf("回落骨架应仅主概念，实际 %d", len(d.Draft.Concepts))
	}
}

// REQ-268/M77：结构化推导属性列 → 数据属性声明（range=推断类型映射；层级列不发声明）
func TestStructuredDataPropsEmitted(t *testing.T) {
	csv := "设备编号,设备名称,功率,在线\nEQ-001,空压机A,75,true\nEQ-002,水泵B,15,false\n"
	d, err := InferStructuredDraftMode("devices.csv", csv, InferStructuredDraftOpts{Mode: "instance"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Draft.DataProperties) != 3 {
		t.Fatalf("3 个属性列应发 3 条声明，实际 %+v", d.Draft.DataProperties)
	}
	byName := map[string]buildDataProperty{}
	for _, dp := range d.Draft.DataProperties {
		byName[dp.Name] = dp
	}
	if byName["功率"].Range != "number" || byName["设备名称"].Range != "string" {
		t.Fatalf("range 推断映射不符: %+v", byName)
	}
	if _, ok := byName["设备编号"]; ok {
		t.Fatalf("首列（实例名）不应发声明: %+v", byName)
	}
	// 模板层级模式：层级列 role=concept-level 不发声明
	tpl := "类别,子类,设备名称,功率\n设备,动力,空压机A,75\n设备,静止,水泵B,15\n仪器,检测,流量计,8\n"
	d2, err := InferStructuredDraftMode("t.csv", tpl, InferStructuredDraftOpts{Mode: "template", HierarchyColumns: []string{"类别", "子类"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, dp := range d2.Draft.DataProperties {
		if dp.Name == "类别" || dp.Name == "子类" {
			t.Fatalf("层级列不应发声明: %+v", d2.Draft.DataProperties)
		}
	}
}
