package importer

import (
	"encoding/json"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestBuildMergedDecisions(t *testing.T) {
	target := &pkgspec.Spec{Name: "target", Concepts: []pkgspec.Concept{{Name: "Device"}},
		Decisions: []pkgspec.Decision{{ID: "same", DecisionQuestion: "old question", ProofScope: "existing scope"}, {ID: "old", DecisionQuestion: "keep existing"}}}
	incoming := &pkgspec.Spec{Name: "incoming", Concepts: []pkgspec.Concept{{Name: "Fault"}},
		Decisions: []pkgspec.Decision{{ID: "same", DecisionQuestion: "new question"}, {ID: "new", DecisionQuestion: "keep incoming"}}}
	for _, tc := range []struct {
		strategy, question, scope string
		count                     int
	}{
		{StrategyReplace, "new question", "", 3},
		{StrategyMergeOverwrite, "new question", "existing scope", 3},
		{StrategyMerge, "old question", "existing scope", 4},
	} {
		t.Run(tc.strategy, func(t *testing.T) {
			pv, err := BuildMerged(target, incoming, tc.strategy, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(pv.MergedSpec.Decisions) != tc.count {
				t.Fatalf("decision count=%d want=%d", len(pv.MergedSpec.Decisions), tc.count)
			}
			seen := map[string]bool{}
			for _, d := range pv.MergedSpec.Decisions {
				if seen[d.ID] {
					t.Fatalf("duplicate decision id %q", d.ID)
				}
				seen[d.ID] = true
				if d.ID == "same" && (d.DecisionQuestion != tc.question || d.ProofScope != tc.scope) {
					t.Fatalf("wrong decision merge: %+v", d)
				}
			}
			if !seen["old"] || !seen["new"] {
				t.Fatalf("unique decisions lost: %v", seen)
			}
		})
	}
}

func TestBuildMergedDecisionSourcesFollowIncomingRename(t *testing.T) {
	target := &pkgspec.Spec{Name: "target", Concepts: []pkgspec.Concept{{Name: "Device"}}, Relations: []pkgspec.Relation{{Name: "owns", From: "Device", To: "Device"}},
		Decisions: []pkgspec.Decision{{ID: "target", DecisionQuestion: "target question", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "device", Source: "concept:Device"}, {Slot: "owner", Source: "relation:owns"}}}}}
	incoming := &pkgspec.Spec{Name: "incoming", Concepts: []pkgspec.Concept{{Name: "Device"}}, Relations: []pkgspec.Relation{{Name: "owns", From: "Device", To: "Device"}},
		Decisions: []pkgspec.Decision{{ID: "incoming", DecisionQuestion: "incoming question", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "device", Source: "concept:Device"}, {Slot: "owner", Source: "relation:owns"}}}}}
	beforeTarget, _ := json.Marshal(target)
	beforeIncoming, _ := json.Marshal(incoming)
	pv, err := BuildMerged(target, incoming, StrategyMerge, "ext")
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.MergedSpec.Decisions) != 2 {
		t.Fatalf("decisions lost: %+v", pv.MergedSpec.Decisions)
	}
	want := map[string][]string{"target": {"concept:Device", "relation:owns"}, "incoming": {"concept:ext_Device", "relation:ext_owns"}}
	for _, d := range pv.MergedSpec.Decisions {
		for i, slot := range d.RequiredSlots {
			if slot.Source != want[d.ID][i] {
				t.Fatalf("%s source=%q want=%q", d.ID, slot.Source, want[d.ID][i])
			}
		}
	}
	afterTarget, _ := json.Marshal(target)
	afterIncoming, _ := json.Marshal(incoming)
	if string(beforeTarget) != string(afterTarget) || string(beforeIncoming) != string(afterIncoming) {
		t.Fatal("merge mutated an input asset")
	}
}
