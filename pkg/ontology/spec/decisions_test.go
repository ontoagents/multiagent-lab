package spec

// REQ-544/M351 决策层 spec 校验单测（仿 dataprop_test.go）：
// 合法 decisions 零错误 / id 重复 / question 空 / 槽位 source 前缀形态与悬空引用 / route·reason 空。

import (
	"strings"
	"testing"
)

func baseDecisionSpec() *Spec {
	return &Spec{
		Concepts:  []Concept{{Name: "概念甲"}, {Name: "概念乙"}},
		Relations: []Relation{{Name: "关联", From: "概念甲", To: "概念乙"}},
	}
}

func TestDecisionsValidate_Req544(t *testing.T) {
	// 合法：零错误
	ok := baseDecisionSpec()
	ok.Decisions = []Decision{{
		ID:               "d1",
		DecisionQuestion: "何时合并甲乙？",
		RequiredSlots:    []DecisionSlot{{Slot: "引用面", Source: "relation:关联", MissingAction: "先补关系"}},
		RejectRoutes:     []DecisionRoute{{Route: "语义有偏差", Reason: "走同义词登记"}},
	}}
	if errs := ok.Validate(); len(errs) != 0 {
		t.Fatalf("合法决策不应报错: %+v", errs)
	}
	// concept: 引用形态
	ok.Decisions[0].RequiredSlots[0].Source = "concept:概念甲"
	if errs := ok.Validate(); len(errs) != 0 {
		t.Fatalf("concept 引用合法: %+v", errs)
	}

	// id 重复
	dup := baseDecisionSpec()
	dup.Decisions = []Decision{{ID: "d1", DecisionQuestion: "q"}, {ID: "d1", DecisionQuestion: "q2"}}
	errs := dup.Validate()
	foundDup := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "决策 id 重复") {
			foundDup = true
		}
	}
	if !foundDup {
		t.Fatalf("id 重复应报错: %+v", errs)
	}

	// id 空
	empty := baseDecisionSpec()
	empty.Decisions = []Decision{{DecisionQuestion: "q"}}
	errs = empty.Validate()
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "决策 id 不能为空") {
		t.Fatalf("id 空应报错: %+v", errs)
	}

	// question 空
	q := baseDecisionSpec()
	q.Decisions = []Decision{{ID: "d1"}}
	errs = q.Validate()
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "decision_question") {
		t.Fatalf("question 空应报错: %+v", errs)
	}

	// source 形态与悬空
	src := baseDecisionSpec()
	src.Decisions = []Decision{{
		ID:               "d1",
		DecisionQuestion: "q",
		RequiredSlots:    []DecisionSlot{{Slot: "s", Source: "relation:不存在"}},
	}}
	errs = src.Validate()
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "关系未在本 spec 声明") {
		t.Fatalf("悬空 relation 引用应报错: %+v", errs)
	}
	src.Decisions[0].RequiredSlots[0].Source = "axiom:某物"
	errs = src.Validate()
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "relation:NAME 或 concept:NAME") {
		t.Fatalf("非法前缀应报错: %+v", errs)
	}
	src.Decisions[0].RequiredSlots[0].Source = "concept:概念乙"
	if errs := src.Validate(); len(errs) != 0 {
		t.Fatalf("concept 合法引用: %+v", errs)
	}

	// route/reason 空
	rr := baseDecisionSpec()
	rr.Decisions = []Decision{{ID: "d1", DecisionQuestion: "q", RejectRoutes: []DecisionRoute{{Route: "r"}}}}
	errs = rr.Validate()
	foundReason := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "reason 不能为空") {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("reason 空应报错: %+v", errs)
	}
}
