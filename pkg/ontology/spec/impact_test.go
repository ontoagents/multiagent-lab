package spec

// REQ-278/M103 影响分析单测：闭包波及面/风险分级/B1 疑似蕴含边（67 号扩注判据）。

import (
	"strings"
	"testing"
)

func TestAnalyzeImpact(t *testing.T) {
	from := &Spec{
		Concepts:       []Concept{{Name: "药物"}, {Name: "心血管用药", Parents: []string{"药物"}}, {Name: "抗凝药", Parents: []string{"心血管用药"}}},
		Relations:      []Relation{{Name: "作用于", From: "药物", To: "疾病"}, {Name: "直达边", From: "药物", To: "疾病"}},
		Instances:      []Instance{{Name: "阿司匹林", Concept: "药物"}},
		DataProperties: []DataProperty{{Name: "剂量", Domain: "药物"}},
	}
	to := &Spec{
		// 药物被删除但仍被引用（TO 保留悬空引用=草稿态真实场景）→ removed+被引用=high
		Concepts:       []Concept{{Name: "心血管用药"}, {Name: "抗凝药", Parents: []string{"心血管用药"}}},
		Relations:      []Relation{{Name: "作用于", From: "药物", To: "疾病"}},
		Instances:      []Instance{{Name: "阿司匹林", Concept: "药物"}},
		DataProperties: []DataProperty{{Name: "剂量", Domain: "药物"}},
	}
	rep := AnalyzeImpact(from, to)
	if len(rep.Changes["concept"].Removed) != 1 || rep.Changes["concept"].Removed[0] != "药物" {
		t.Fatalf("删除概念集不符: %v", rep.Changes["concept"])
	}
	highFound := false
	joined := ""
	for _, b := range rep.Blast {
		if b.Name == "药物" && b.Risk == RiskHigh && b.Action == ActionRemoved {
			highFound = true
			joined += b.ReasonsString()
		}
	}
	if highFound {
		for _, want := range []string{"作用于", "阿司匹林", "剂量"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("判词缺引用者 %s: %s", want, joined)
			}
		}
	}
	if !highFound {
		t.Fatalf("被引用概念删除应标 high: %v", rep.Blast)
	}
	if rep.Summary.High == 0 {
		t.Fatal("summary.high 应>0")
	}
}

func TestAnalyzeImpactImplied(t *testing.T) {
	to := &Spec{
		Concepts:  []Concept{{Name: "A"}, {Name: "B"}, {Name: "C"}},
		Relations: []Relation{{Name: "A到B", From: "A", To: "B"}, {Name: "B到C", From: "B", To: "C"}, {Name: "A到C直达", From: "A", To: "C"}},
	}
	rep := AnalyzeImpact(to, to)
	if len(rep.Implied) != 1 || !strings.Contains(rep.Implied[0], "A→B→C") {
		t.Fatalf("B1 蕴含边检测不符: %v", rep.Implied)
	}
}

func TestAnalyzeImpactLowRisk(t *testing.T) {
	from := &Spec{Concepts: []Concept{{Name: "孤岛"}}}
	to := &Spec{Concepts: []Concept{{Name: "新概念"}}}
	rep := AnalyzeImpact(from, to)
	if len(rep.Changes["concept"].Added) != 1 || len(rep.Changes["concept"].Removed) != 1 {
		t.Fatalf("变更集不符: %v", rep.Changes["concept"])
	}
	lowFound := false
	for _, b := range rep.Blast {
		if b.Name == "孤岛" && b.Risk == RiskLow {
			lowFound = true
		}
	}
	if !lowFound {
		t.Fatalf("无引用删除应标 low: %v", rep.Blast)
	}
}
