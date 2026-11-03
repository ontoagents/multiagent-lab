package rest

import (
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func TestElementRenameRewiresDecisionSources(t *testing.T) {
	for _, kind := range []string{"concept", "relation"} {
		t.Run(kind, func(t *testing.T) {
			sp := &pkgspec.Spec{Name: "rename", Concepts: []pkgspec.Concept{{Name: "Device"}}, Relations: []pkgspec.Relation{{Name: "owns", From: "Device", To: "Device"}}, Decisions: []pkgspec.Decision{{ID: "d", DecisionQuestion: "Who owns the device?", RequiredSlots: []pkgspec.DecisionSlot{{Slot: "device", Source: "concept:Device"}, {Slot: "owner", Source: "relation:owns"}}}}}
			old, to, slot := "Device", "Equipment", 0
			if kind == "relation" {
				old, to, slot = "owns", "maintains", 1
			}
			if _, err := applyElementRename(kind, old, to, sp); err != nil {
				t.Fatal(err)
			}
			if got := sp.Decisions[0].RequiredSlots[slot].Source; got != kind+":"+to {
				t.Fatalf("decision source=%q want=%q", got, kind+":"+to)
			}
			if errs := sp.Validate(); len(errs) != 0 {
				t.Fatalf("rename left dangling decision: %v", errs)
			}
		})
	}
}
