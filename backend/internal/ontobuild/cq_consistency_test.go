package ontobuild

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func modelBuildFixture(t *testing.T, drafts []string) (*Service, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	index := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if index >= len(drafts) {
			w.WriteHeader(500)
			return
		}
		draft := drafts[index]
		index++
		json.NewEncoder(w).Encode(map[string]any{"id": "test", "object": "chat.completion", "model": "stub", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": draft}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(model.Close)
	conn, err := st.CreateConnection(&store.ModelConnection{Name: "stub", ConnType: "chat", Protocol: "openai_compat", BaseURL: model.URL + "/v1", ModelName: "stub", Enabled: true, IsDefault: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, nil, nil)
	return svc, conn.ID
}

func TestGeneratedCQPersistsInFormalSpec(t *testing.T) {
	for _, tc := range []struct {
		name, draft string
		want        []string
	}{
		{"legacy", `{"name":"test","cqs":["legacy question"],"concepts":[{"name":"A","synonyms":["Alias"],"note":"keep me"}]}`, []string{"legacy question"}},
		{"formal", `{"name":"test","cq":["edited question"],"cqs":["old question"],"concepts":[{"name":"A"}]}`, []string{"edited question"}},
		{"explicit_empty", `{"name":"test","cq":[],"cqs":["old question"],"concepts":[{"name":"A"}]}`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, conn := modelBuildFixture(t, []string{tc.draft})
			sp, err := svc.generateSpec(context.Background(), "test", conn)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(sp)
			var saved struct {
				CQ       []string `json:"cq"`
				Concepts []struct {
					Synonyms []string `json:"synonyms"`
					Note     string   `json:"note"`
				} `json:"concepts"`
			}
			if err := json.Unmarshal(b, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.CQ) != len(tc.want) || (len(tc.want) > 0 && !reflect.DeepEqual(saved.CQ, tc.want)) {
				t.Fatalf("formal CQ missing/stale: %s", b)
			}
			if tc.name == "legacy" && (saved.Concepts[0].Note != "keep me" || len(saved.Concepts[0].Synonyms) != 1) {
				t.Fatalf("formal fields lost: %s", b)
			}
		})
	}
}

func TestRepairedBuildQualityAndCQUseFinalDraft(t *testing.T) {
	svc, conn := modelBuildFixture(t, []string{
		`{"name":"test","cqs":["before CQ"],"concepts":[{"name":"A","parents":["missing"]}]}`,
		`{"name":"test","cqs":["after CQ"],"concepts":[{"name":"A"}]}`,
	})
	k, err := svc.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "corpus", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: k.ID, Title: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.InsertKnowledgeChunks([]*store.KnowledgeChunk{{KBID: k.ID, DocID: d.ID, Seq: 0, Content: "A is a concept."}}); err != nil {
		t.Fatal(err)
	}
	qualityCalls := 0
	q := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qualityCalls++
		var body struct {
			Spec struct {
				CQ       []string `json:"cq"`
				Concepts []struct {
					Parents []string `json:"parents"`
				} `json:"concepts"`
			} `json:"spec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(400)
			return
		}
		if len(body.Spec.Concepts[0].Parents) > 0 {
			fmt.Fprint(w, `{"report":{"error_count":1,"score":{"overall":10}}}`)
		} else if reflect.DeepEqual(body.Spec.CQ, []string{"after CQ"}) {
			fmt.Fprint(w, `{"report":{"error_count":0,"score":{"overall":95}}}`)
		} else {
			fmt.Fprint(w, `{"report":{"error_count":0,"score":{"overall":20}}}`)
		}
	}))
	defer q.Close()
	svc.BuildPlaneURL = q.URL
	gateNotes := []string{}
	svc.OnStage = func(stage, note string, _ int64, _ string) {
		if stage == "gate" {
			gateNotes = append(gateNotes, note)
		}
	}
	res, err := svc.BuildFromKB(context.Background(), k.ID, "chunk-llm", "auto", nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rounds != 2 || !res.Report.OK || res.Quality.Overall != 95 || !reflect.DeepEqual(res.CQs, []string{"after CQ"}) {
		t.Fatalf("inconsistent result: %+v quality=%+v CQs=%v", res, res.Quality, res.CQs)
	}
	if qualityCalls != 1 || len(gateNotes) != 1 || !strings.Contains(gateNotes[0], "95.0") {
		t.Fatalf("gate uses pre-repair draft: calls=%d notes=%v", qualityCalls, gateNotes)
	}
}

func TestBuildCQModePreservesSelectedIntent(t *testing.T) {
	for _, mode := range []string{"custom", "skip"} {
		t.Run(mode, func(t *testing.T) {
			svc, conn := modelBuildFixture(t, []string{`{"name":"test","cqs":["model CQ"],"concepts":[{"name":"A"}]}`})
			k, err := svc.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "corpus", Mode: "rag"})
			if err != nil {
				t.Fatal(err)
			}
			d, err := svc.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: k.ID, Title: "doc"})
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Store.InsertKnowledgeChunks([]*store.KnowledgeChunk{{KBID: k.ID, DocID: d.ID, Seq: 0, Content: "A is a concept."}}); err != nil {
				t.Fatal(err)
			}
			svc.BuildPlaneURL = "http://127.0.0.1:1"
			res, err := svc.BuildFromKB(context.Background(), k.ID, "chunk-llm", mode, []string{"user CQ"}, conn)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{}
			if mode == "custom" {
				want = []string{"user CQ"}
			}
			b, _ := json.Marshal(res.Spec)
			var saved struct {
				CQ []string `json:"cq"`
			}
			json.Unmarshal(b, &saved)
			if len(saved.CQ) != len(want) || !reflect.DeepEqual(res.CQs, want) {
				t.Fatalf("mode %s result CQ=%v spec=%s", mode, res.CQs, b)
			}
		})
	}
}

func TestBuildValidationIncludesFormalRules(t *testing.T) {
	sp := &buildSpec{Name: "test", Concepts: []buildConcept{{Name: "A/B"}, {Name: "A_B"}}}
	report := ValidateBuildSpec(sp)
	if report.OK || len(report.Errors) == 0 || !strings.Contains(report.Errors[0].Message, "IRI") {
		t.Fatalf("draft validation differs from formal save: %+v", report)
	}
	// KB-only rules still reject an empty name and an empty concept list.
	if report := ValidateBuildSpec(&buildSpec{}); report.OK || len(report.Errors) != 2 {
		t.Fatalf("KB requirements lost: %+v", report)
	}
}

func TestBuildRepairsFormalIdentityConflict(t *testing.T) {
	svc, conn := modelBuildFixture(t, []string{
		`{"name":"test","cq":["original CQ"],"concepts":[{"name":"A/B"},{"name":"A_B"}]}`,
		`{"name":"test","concepts":[{"name":"A/B"},{"name":"B"}]}`,
	})
	k, err := svc.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "corpus", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: k.ID, Title: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.InsertKnowledgeChunks([]*store.KnowledgeChunk{{KBID: k.ID, DocID: d.ID, Seq: 0, Content: "A/B and B are distinct concepts."}}); err != nil {
		t.Fatal(err)
	}
	svc.BuildPlaneURL = "http://127.0.0.1:1"
	res, err := svc.BuildFromKB(context.Background(), k.ID, "chunk-llm", "auto", nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rounds != 2 || !res.Report.OK || len(res.Spec.Validate()) != 0 || res.Spec.Concepts[1].Name != "B" || !reflect.DeepEqual(res.CQs, []string{"original CQ"}) {
		t.Fatalf("formal validation did not repair draft consistently: %+v", res)
	}
}
