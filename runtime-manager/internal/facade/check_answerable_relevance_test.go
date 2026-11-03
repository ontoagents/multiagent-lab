package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

func TestAnswerableRelevanceSurvivesUnrelatedGrowth(t *testing.T) {
	for _, size := range []int{1, 100} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			f := newHistFacade(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				raw := string(body)
				bindings := []map[string]any{}
				if strings.Contains(raw, "owl:Class") {
					labels := []string{"Kubernetes"}
					for i := 1; i < size; i++ {
						labels = append(labels, fmt.Sprintf("unrelated%03d", i))
					}
					for _, l := range labels {
						bindings = append(bindings, map[string]any{"label": map[string]any{"value": l, "type": "literal"}})
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": bindings}})
			}))
			for _, question := range []string{"kubernetes 是什么？", "Kubernetes 是什么？"} {
				got, err := f.CheckAnswerableCore(context.Background(), "ont", question)
				if err != nil {
					t.Fatal(err)
				}
				if got["answerable"] != true || !reflect.DeepEqual(got["matched_concepts"], []string{"Kubernetes"}) {
					t.Fatalf("size=%d result=%+v", size, got)
				}
			}
		})
	}
}

func TestAnswerableUnloadedIsError(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "rt.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f := New(st, func(string) (string, error) { return "", nil })
	if _, err := f.CheckAnswerableCore(context.Background(), "missing", "Kubernetes"); err == nil {
		t.Fatal("unloaded ontology must remain unknown")
	}
}

func TestAnswerableMissingLabelsIgnoreCase(t *testing.T) {
	got := missingByDomain([]string{"kubernetes", "api"}, map[string][]string{"concepts": {"Kubernetes API"}})
	if len(got["concepts"]) != 0 {
		t.Fatalf("case mismatch reported as missing: %+v", got)
	}
}
