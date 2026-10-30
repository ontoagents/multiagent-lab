package ontorag

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func testSpec() *pkgspec.Spec {
	return &pkgspec.Spec{
		Name: "医疗",
		Concepts: []pkgspec.Concept{
			{Name: "疾病", Label: "Disease", Definition: "机体在病因作用下出现的异常生命活动"},
			{Name: "症状", Label: "Symptom", Definition: "疾病表现出的异常状态"},
			{Name: "感染性疾病", Label: "InfectiousDisease", Definition: "由病原体引起的疾病", Parents: []string{"疾病"}},
		},
		Relations: []pkgspec.Relation{
			{Name: "表现为", Label: "hasSymptom", Definition: "疾病呈现的症状", From: "疾病", To: "症状"},
		},
		DataProperties: []pkgspec.DataProperty{
			{Name: "发病率", Label: "incidence", Definition: "单位人群新发病例比例", Domain: "疾病", Range: "number"},
		},
		Instances: []pkgspec.Instance{{Name: "流感", Concept: "疾病"}},
	}
}

// 元素抽取：类/对象关系/数据属性三 kinds 齐全；实例不进索引（A-Box 禁令）。
func TestElementsFromSpec(t *testing.T) {
	els := ElementsFromSpec("onto1", testSpec())
	if len(els) != 5 { // 3 概念 + 1 关系 + 1 数据属性（实例「流感」不入）
		t.Fatalf("元素数期望 5 得到 %d", len(els))
	}
	kinds := map[string]int{}
	for _, e := range els {
		kinds[e.Kind]++
		if e.Snippet == "" {
			t.Fatalf("元素 %s 缺 snippet", e.LocalName)
		}
	}
	if kinds["class"] != 3 || kinds["object_property"] != 1 || kinds["data_property"] != 1 {
		t.Fatalf("kind 分布不符: %v", kinds)
	}
	// 关系 domain/range 映射
	for _, e := range els {
		if e.LocalName == "表现为" {
			if len(e.Domain) != 1 || e.Domain[0] != "疾病" || len(e.Range) != 1 || e.Range[0] != "症状" {
				t.Fatalf("关系 domain/range 映射错误: %+v", e)
			}
		}
		if e.LocalName == "感染性疾病" {
			if len(e.Super) != 1 || e.Super[0] != "疾病" {
				t.Fatalf("概念 parents 未映射 Super: %+v", e)
			}
		}
		if e.URI == "" || !json.Valid([]byte(e.Snippet)) {
			t.Fatalf("元素 URI/snippet 异常: %+v", e)
		}
	}
}

// 管道符序列化：N2 口径——字段顺序 localName|label|comment|Type|Domain|Range，含 comments。
func TestSearchText(t *testing.T) {
	els := ElementsFromSpec("onto1", testSpec())
	var rel, cls string
	for _, e := range els {
		if e.LocalName == "表现为" {
			rel = e.SearchText()
		}
		if e.LocalName == "疾病" {
			cls = e.SearchText()
		}
	}
	want := "表现为 | hasSymptom | 疾病呈现的症状 | Type: 对象关系 | Domain: 疾病 | Range: 症状"
	if rel != want {
		t.Fatalf("关系序列化不符:\n得到 %s\n期望 %s", rel, want)
	}
	if want2 := "疾病 | Disease | 机体在病因作用下出现的异常生命活动 | Type: 概念 | 上位: "; cls != want2 && cls != "疾病 | Disease | 机体在病因作用下出现的异常生命活动 | Type: 概念" {
		t.Fatalf("概念序列化不符: %s", cls)
	}
}

// 内积 top-k：归一后内积=余弦；排序正确、维度不齐向量跳过。
func TestTopK(t *testing.T) {
	els := []Element{
		{LocalName: "a", Kind: "class"},
		{LocalName: "b", Kind: "class"},
		{LocalName: "c", Kind: "class"},
		{LocalName: "bad", Kind: "class"},
	}
	vecs := [][]float64{
		{1, 0},
		{0.9, 0.1},
		{0, 1},
		{1, 1, 1}, // 维度不齐——跳过
	}
	hits := TopK([]float64{1, 0.05}, els, vecs, 2)
	if len(hits) != 2 {
		t.Fatalf("top-k 数期望 2 得到 %d", len(hits))
	}
	if hits[0].Element.LocalName != "a" || hits[1].Element.LocalName != "b" {
		t.Fatalf("排序错误: %s, %s", hits[0].Element.LocalName, hits[1].Element.LocalName)
	}
	if hits[0].Score <= hits[1].Score {
		t.Fatalf("得分应递减: %v <= %v", hits[0].Score, hits[1].Score)
	}
	if math.IsNaN(hits[0].Score) || hits[0].Score <= 0.99 {
		t.Fatalf("归一内积应≈1: %v", hits[0].Score)
	}
}

// Integrator 去重：同名概念/关系/数据属性/公理剔除并留清单；片段内批内同口径。
func TestDedupe(t *testing.T) {
	target := testSpec()
	frag := &FragmentSpec{
		Concepts: []pkgspec.Concept{
			{Name: "疾病", Label: "重复声明"}, // 与目标撞名——剔除
			{Name: "治疗方案", Label: "Therapy", Definition: "疾病的治疗计划"},
		},
		Relations: []pkgspec.Relation{
			{Name: "表现为", From: "疾病", To: "症状"}, // 撞名——剔除
			{Name: "采用", From: "治疗方案", To: "药物"},
		},
		DataProperties: []pkgspec.DataProperty{{Name: "发病率", Domain: "疾病"}},
	}
	cleaned, removed := Dedupe(target, frag)
	if len(cleaned.Concepts) != 1 || cleaned.Concepts[0].Name != "治疗方案" {
		t.Fatalf("去重后概念不符: %+v", cleaned.Concepts)
	}
	if len(cleaned.Relations) != 1 || cleaned.Relations[0].Name != "采用" {
		t.Fatalf("去重后关系不符: %+v", cleaned.Relations)
	}
	if len(cleaned.DataProperties) != 0 {
		t.Fatalf("数据属性应被去重剔除: %+v", cleaned.DataProperties)
	}
	if len(removed) != 3 {
		t.Fatalf("去重清单期望 3 得到 %d: %v", len(removed), removed)
	}
}

// 两阶段校验①：合并视图校验——复用目标概念引用不误报；重复声明撞名报错；片段内悬空引用报错。
func TestValidateAgainstTarget(t *testing.T) {
	target := testSpec()
	// 合法片段：复用「疾病」「症状」+ 新建「药物」「作用于」
	ok := &FragmentSpec{
		Concepts:  []pkgspec.Concept{{Name: "药物", Label: "Drug", Definition: "用于预防治疗诊断疾病的物质"}},
		Relations: []pkgspec.Relation{{Name: "作用于", Label: "actsOn", Definition: "药物的靶点关系", From: "药物", To: "疾病"}},
	}
	if errs := ValidateAgainstTarget(target, ok); len(errs) != 0 {
		t.Fatalf("合法片段不应报错: %v", errs)
	}
	// 重复声明：概念「疾病」再声明 → 撞名错误
	dup := &FragmentSpec{Concepts: []pkgspec.Concept{{Name: "疾病", Label: "Disease", Definition: "重复"}}}
	if errs := ValidateAgainstTarget(target, dup); len(errs) == 0 {
		t.Fatalf("重复声明应报名称重复")
	}
	// 悬空引用：from 引用不存在的概念
	dangling := &FragmentSpec{Relations: []pkgspec.Relation{{Name: "测试", From: "不存在", To: "疾病"}}}
	if errs := ValidateAgainstTarget(target, dangling); len(errs) == 0 {
		t.Fatalf("悬空引用应报错")
	}
}

// prompt 组装：命中元素只读上下文渲染（含 snippet）与 profile 指令位。
func TestRenderExtenderPrompt(t *testing.T) {
	els := ElementsFromSpec("onto1", testSpec())
	hits := []Hit{{Element: els[0], Score: 0.87}}
	p := RenderExtenderPrompt("药物可治疗哪些疾病？", hits, ProfileRestriction, "")
	for _, want := range []string{"药物可治疗哪些疾病？", "限制型", "只读上下文", "Disease"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt 缺少 %q", want)
		}
	}
	if !strings.Contains(RenderExtenderPrompt("q", nil, "", ""), "检索无命中") {
		t.Fatalf("空命中应给无命中引导")
	}
}
