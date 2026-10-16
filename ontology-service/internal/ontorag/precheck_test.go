package ontorag

// REQ-344/OE-5：CQ 质量预检——信号对齐（模型漏项保守计）与降级（无信号不阻断）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func precheckStub(t *testing.T, respond func(call int) string) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		b, _ := json.Marshal(respond(calls))
		_, _ = w.Write([]byte(`{"draft_json":` + string(b) + `,"usage":null}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestPrecheckAlignment(t *testing.T) {
	srv, _ := precheckStub(t, func(n int) string {
		// 漏评第 2 条（模型漏项）——按不通过保守计
		return `{"items":[{"cq":"Q1？","answerable":true,"atomic":true,"specific":true,"note":"ok"}]}`
	})
	llm := llmcreate.New(srv.URL)
	sp := &pkgspec.Spec{Concepts: []pkgspec.Concept{{Name: "疾病", Label: "D", Definition: "d"}}}
	rep := Precheck(context.Background(), llm, sp, []string{"Q1？", "Q2？"}, nil)
	if rep == nil {
		t.Fatal("应产出报告")
	}
	if len(rep.Items) != 2 {
		t.Fatalf("对齐后应 2 条: %d", len(rep.Items))
	}
	if !rep.Items[0].Answerable {
		t.Fatal("Q1 信号应保留")
	}
	if rep.Items[1].Note == "" {
		t.Fatal("漏评条应保守标注")
	}
}

func TestPrecheckDegrade(t *testing.T) {
	srv, _ := precheckStub(t, func(int) string { return "不是 json" })
	llm := llmcreate.New(srv.URL)
	sp := &pkgspec.Spec{}
	if rep := Precheck(context.Background(), llm, sp, []string{"Q？"}, nil); rep != nil {
		t.Fatal("两轮全坏应降级 nil（无信号不阻断）")
	}
}
