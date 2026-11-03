package importer

import (
	"encoding/json"
	"reflect"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestMergeDecisionSourcesRenameOnce(t *testing.T) {
	in := []pkgspec.Decision{{ID: "d", DecisionQuestion: "q", RequiredSlots: []pkgspec.DecisionSlot{
		{Slot: "first", Source: "concept:A"}, {Slot: "second", Source: "concept:ext_A"},
		{Slot: "relation", Source: "relation:r"}, {Slot: "second relation", Source: "relation:ext_r"},
	}}}
	for n := 0; n < 100; n++ {
		out := mergeDecisions(nil, in, StrategyMerge, "ext", map[string]string{"A": "ext_A", "ext_A": "ext_ext_A"}, map[string]string{"r": "ext_r", "ext_r": "ext_ext_r"})
		want := []string{"concept:ext_A", "concept:ext_ext_A", "relation:ext_r", "relation:ext_ext_r"}
		for i, slot := range out[0].RequiredSlots {
			if slot.Source != want[i] {
				t.Fatalf("run %d slot %d: source=%q want=%q", n, i, slot.Source, want[i])
			}
		}
	}
}

func TestMergePreservesBothReferenceGraphs(t *testing.T) {
	target := &pkgspec.Spec{Name: "target", Concepts: []pkgspec.Concept{{Name: "A", Restrictions: []pkgspec.Restriction{{Type: "some_values_from", Property: "r", Target: "A"}}}},
		Relations: []pkgspec.Relation{{Name: "r", From: "A", To: "A"}},
		Instances: []pkgspec.Instance{{Name: "i", Concept: "A", Relations: []pkgspec.InstanceRel{{Rel: "r", Target: "i"}}}},
		Decisions: []pkgspec.Decision{{ID: "target", DecisionQuestion: "q", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "r", Source: "relation:r"}}}}}
	incoming := &pkgspec.Spec{Name: "incoming", Concepts: []pkgspec.Concept{
		{Name: "A", Restrictions: []pkgspec.Restriction{{Type: "some_values_from", Property: "r", Target: "ext_A"}}},
		{Name: "ext_A", Parents: []string{"A"}},
	}, Relations: []pkgspec.Relation{{Name: "r", From: "A", To: "ext_A", InverseOf: "ext_r"}, {Name: "ext_r", From: "ext_A", To: "A", InverseOf: "r"}},
		Instances:      []pkgspec.Instance{{Name: "i", Concept: "A", Relations: []pkgspec.InstanceRel{{Rel: "r", Target: "ext_i"}}}, {Name: "ext_i", Concept: "ext_A", Relations: []pkgspec.InstanceRel{{Rel: "ext_r", Target: "i"}}}},
		DataProperties: []pkgspec.DataProperty{{Name: "value", Domain: "A"}},
		Axioms:         []pkgspec.Axiom{{Type: "disjoint_with", Subject: "A", Targets: []string{"ext_A"}}},
		Decisions:      []pkgspec.Decision{{ID: "incoming", DecisionQuestion: "q", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "r", Source: "relation:r"}}}}}
	beforeTarget, _ := json.Marshal(target)
	beforeIncoming, _ := json.Marshal(incoming)
	pv, err := BuildMerged(target, incoming, StrategyMerge, "ext")
	if err != nil {
		t.Fatal(err)
	}
	sp := pv.MergedSpec
	if !reflect.DeepEqual(sp.Concepts[0], target.Concepts[0]) || !reflect.DeepEqual(sp.Relations[0], target.Relations[0]) || !reflect.DeepEqual(sp.Instances[0], target.Instances[0]) {
		t.Fatalf("target graph changed: concepts=%+v relations=%+v instances=%+v", sp.Concepts, sp.Relations, sp.Instances)
	}
	if sp.Concepts[1].Restrictions[0].Target != "ext_ext_A" || sp.Concepts[1].Restrictions[0].Property != "ext_r" || sp.Concepts[2].Parents[0] != "ext_A" {
		t.Fatalf("incoming concept references wrong: %+v", sp.Concepts)
	}
	wantRelations := []pkgspec.Relation{{Name: "ext_r", From: "ext_A", To: "ext_ext_A", InverseOf: "ext_ext_r"}, {Name: "ext_ext_r", From: "ext_ext_A", To: "ext_A", InverseOf: "ext_r"}}
	if !reflect.DeepEqual(sp.Relations[1:], wantRelations) {
		t.Fatalf("incoming relations=%+v want=%+v", sp.Relations[1:], wantRelations)
	}
	wantInstances := []pkgspec.Instance{{Name: "ext_i", Concept: "ext_A", Relations: []pkgspec.InstanceRel{{Rel: "ext_r", Target: "ext_ext_i"}}}, {Name: "ext_ext_i", Concept: "ext_ext_A", Relations: []pkgspec.InstanceRel{{Rel: "ext_ext_r", Target: "ext_i"}}}}
	if !reflect.DeepEqual(sp.Instances[1:], wantInstances) || sp.DataProperties[0].Domain != "ext_A" || sp.Axioms[0].Subject != "ext_A" || sp.Axioms[0].Targets[0] != "ext_ext_A" {
		t.Fatalf("incoming assertions wrong: %+v", sp)
	}
	if errs := sp.Validate(); len(errs) > 0 {
		t.Fatalf("merged graph invalid: %v", errs)
	}
	afterTarget, _ := json.Marshal(target)
	afterIncoming, _ := json.Marshal(incoming)
	if string(beforeTarget) != string(afterTarget) || string(beforeIncoming) != string(afterIncoming) {
		t.Fatal("merge changed source assets")
	}
}
