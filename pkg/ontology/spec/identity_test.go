package spec

import (
	"strings"
	"testing"
)

func TestValidateIdentityCollisions(t *testing.T) {
	for _, kind := range []string{"concepts", "relations", "instances", "data_properties"} {
		t.Run(kind, func(t *testing.T) {
			sp := &Spec{Name: "identity", Concepts: []Concept{{Name: "Device"}}}
			switch kind {
			case "concepts":
				sp.Concepts = []Concept{{Name: "A/B"}, {Name: "A_B"}}
			case "relations":
				sp.Relations = []Relation{{Name: "A/B", From: "Device", To: "Device"}, {Name: "A_B", From: "Device", To: "Device"}}
			case "instances":
				sp.Instances = []Instance{{Name: "A/B", Concept: "Device"}, {Name: "A_B", Concept: "Device"}}
			case "data_properties":
				sp.DataProperties = []DataProperty{{Name: "A/B"}, {Name: "A_B"}}
			}
			found := false
			for _, err := range sp.Validate() {
				if err.Path == kind+"[1].name" && strings.Contains(err.Message, "IRI") {
					found = true
				}
			}
			if !found {
				t.Fatalf("distinct %s names collide in RDF but pass validation: %v", kind, sp.Validate())
			}
		})
	}
}

func TestValidateDistinctNamesAndNamespaces(t *testing.T) {
	sp := &Spec{Name: "valid", Concepts: []Concept{{Name: "设备-A"}, {Name: "设备_A"}}, Relations: []Relation{{Name: "设备-A", From: "设备-A", To: "设备_A"}}, Instances: []Instance{{Name: "设备1", Concept: "设备-A"}}, DataProperties: []DataProperty{{Name: "设备-A"}}}
	if errs := sp.Validate(); len(errs) != 0 {
		t.Fatalf("distinct RDF identities should remain valid: %v", errs)
	}
}
