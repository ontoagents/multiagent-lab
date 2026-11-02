package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

// cloneMergeSpec keeps all spec fields and nested source assets intact during preview.
func cloneMergeSpec(sp *pkgspec.Spec) (*pkgspec.Spec, error) {
	b, err := json.Marshal(sp)
	if err != nil {
		return nil, fmt.Errorf("本体合并载荷无法复制: %w", err)
	}
	var out pkgspec.Spec
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func cloneDecision(d pkgspec.Decision) pkgspec.Decision {
	d.RequiredSlots = append([]pkgspec.DecisionSlot(nil), d.RequiredSlots...)
	d.RejectRoutes = append([]pkgspec.DecisionRoute(nil), d.RejectRoutes...)
	return d
}

// mergeDecisions resolves conflicts by decision ID, preserving unique entries from both assets.
func mergeDecisions(target, incoming []pkgspec.Decision, strategy, prefix string, concepts, relations map[string]string) []pkgspec.Decision {
	merged := make([]pkgspec.Decision, 0, len(target)+len(incoming))
	taken := map[string]int{}
	for _, d := range target {
		taken[d.ID] = len(merged)
		merged = append(merged, cloneDecision(d))
	}
	// Reserve incoming IDs before generating conflict IDs, so a later unique decision stays unique.
	reserved := map[string]int{}
	for id, i := range taken {
		reserved[id] = i
	}
	for _, d := range incoming {
		reserved[d.ID] = 0
	}
	for _, value := range incoming {
		d := cloneDecision(value)
		if strategy == StrategyMerge {
			for i := range d.RequiredSlots {
				kind, name, _ := strings.Cut(d.RequiredSlots[i].Source, ":")
				var renamed string
				switch kind {
				case "concept":
					renamed = concepts[name]
				case "relation":
					renamed = relations[name]
				}
				if renamed != "" {
					d.RequiredSlots[i].Source = kind + ":" + renamed
				}
			}
		}
		if i, conflict := taken[d.ID]; conflict {
			switch strategy {
			case StrategyReplace:
				merged[i] = d
			case StrategyMergeOverwrite:
				cur := &merged[i]
				if d.DecisionQuestion != "" {
					cur.DecisionQuestion = d.DecisionQuestion
				}
				if len(d.RequiredSlots) > 0 {
					cur.RequiredSlots = d.RequiredSlots
				}
				if len(d.RejectRoutes) > 0 {
					cur.RejectRoutes = d.RejectRoutes
				}
				if d.ProofScope != "" {
					cur.ProofScope = d.ProofScope
				}
			default:
				d.ID = uniquify(prefix+"_"+d.ID, reserved)
				reserved[d.ID] = len(merged)
				taken[d.ID] = len(merged)
				merged = append(merged, d)
			}
		} else {
			taken[d.ID] = len(merged)
			merged = append(merged, d)
		}
	}
	return merged
}

func rewireDecisionSources(decisions []pkgspec.Decision, kind, oldName, newName string) {
	for i := range decisions {
		for j := range decisions[i].RequiredSlots {
			if decisions[i].RequiredSlots[j].Source == kind+":"+oldName {
				decisions[i].RequiredSlots[j].Source = kind + ":" + newName
			}
		}
	}
}
