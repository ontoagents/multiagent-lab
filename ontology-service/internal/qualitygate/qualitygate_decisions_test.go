package qualitygate

// REQ-544/M351 决策层 warning 检查单测——无必答槽/无拒绝路径两类软命中 + 完备种子零误报。

import (
	"strings"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestDecisionChecks_Req544(t *testing.T) {
	sp := &pkgspec.Spec{
		Concepts:  []pkgspec.Concept{{Name: "概念甲"}, {Name: "概念乙"}},
		Relations: []pkgspec.Relation{{Name: "关联", From: "概念甲", To: "概念乙"}},
		Decisions: []pkgspec.Decision{
			{ID: "d-empty", DecisionQuestion: "无槽无路"}, // 两类全命中
			{ID: "d-ok", DecisionQuestion: "完备", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "s", Source: "relation:关联"}}, RejectRoutes: []pkgspec.DecisionRoute{{Route: "r", Reason: "x"}}}, // 零命中
		},
	}
	rep := Check(sp, nil)
	byID := map[string]*Finding{}
	for i := range rep.Findings {
		f := &rep.Findings[i]
		byID[f.CheckID] = f
	}
	noSlots, ok1 := byID["decision_no_slots"]
	if !ok1 || noSlots.Count != 1 || !strings.Contains(strings.Join(noSlots.Samples, ","), "decisions[0]") {
		t.Fatalf("decision_no_slots 应命中 1 条: %+v", noSlots)
	}
	noRoutes, ok2 := byID["decision_no_routes"]
	if !ok2 || noRoutes.Count != 1 {
		t.Fatalf("decision_no_routes 应命中 1 条: %+v", noRoutes)
	}
	if rep.Stats.Decisions != 2 {
		t.Fatalf("Stats.Decisions 应=2: %d", rep.Stats.Decisions)
	}
}
