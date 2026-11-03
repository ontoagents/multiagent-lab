package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestQuizCoverageRequiresSuccessfulExplicitVerdict(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"unloaded", 409, `{"error":"not loaded"}`, ""},
		{"server_error_with_bool", 500, `{"answerable":false}`, ""},
		{"invalid_json", 200, `{`, ""},
		{"trailing_json", 200, `{"answerable":true}{}`, ""},
		{"trailing_garbage", 200, `{"answerable":true}oops`, ""},
		{"missing_bool", 200, `{"coverage":0}`, ""},
		{"null_bool", 200, `{"answerable":null}`, ""},
		{"wrong_type", 200, `{"answerable":"false"}`, ""},
		{"false", 200, `{"answerable":false}`, "not_answerable"},
		{"true", 200, `{"answerable":true}`, "answerable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st := quizTestServer(t)
			rt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer rt.Close()
			t.Setenv("RUNTIME_MGR_URL", rt.URL)
			q, err := st.InsertQuizItem(&store.QuizItem{OntologyID: "coverage", Question: "Q?", Answer: "A"})
			if err != nil {
				t.Fatal(err)
			}
			s.quizCoveragePreset("coverage", []*store.QuizItem{q})
			got, err := st.GetQuizItem(q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.CoverageVerdict != tc.want {
				t.Fatalf("verdict %q want %q", got.CoverageVerdict, tc.want)
			}
		})
	}
}

func TestQuizCoverageFailureDoesNotSkipLaterQuestions(t *testing.T) {
	s, st := quizTestServer(t)
	calls := 0
	rt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(409)
			fmt.Fprint(w, `{"error":"unloaded"}`)
		} else {
			fmt.Fprint(w, `{"answerable":true}`)
		}
	}))
	defer rt.Close()
	t.Setenv("RUNTIME_MGR_URL", rt.URL)
	one, _ := st.InsertQuizItem(&store.QuizItem{OntologyID: "coverage", Question: "one", Answer: "A"})
	two, _ := st.InsertQuizItem(&store.QuizItem{OntologyID: "coverage", Question: "two", Answer: "A"})
	s.quizCoveragePreset("coverage", []*store.QuizItem{one, two})
	a, _ := st.GetQuizItem(one.ID)
	b, _ := st.GetQuizItem(two.ID)
	if a.CoverageVerdict != "" || b.CoverageVerdict != "answerable" {
		t.Fatalf("verdicts %q/%q", a.CoverageVerdict, b.CoverageVerdict)
	}
}
