package cqbench

// REQ-382/M178：执行级对齐单测——桩 LLM+桩执行通道；执行结果集含 gold URN → GoldAligned
// 且 Executed=true/GoldOverlap 计得；执行失败回落文本子串口径并如实标注。

import (
	"context"
	"os"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func execStubLLM() *llmcreate.Creator {
	plt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		draft := "SELECT ?x WHERE { ?x a <urn_o_evo_concept_计算节点> } LIMIT 10"
		if !strings.Contains(req.Prompt, "本体查询专家") {
			draft = `{"reply":"ok"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"draft_json": draft})
	}))
	return llmcreate.New(plt.URL)
}

func execDataset(t *testing.T) *Dataset {
	dir := t.TempDir()
	dsJSON := `{"name":"exec_mini","ontology":"seed_x","revision":"r1-test","cqs":[{"id":"Q01","cq":"有哪些计算节点？","gold":["urn:o:evo:concept:计算节点"],"type":"E","kind":"fact","difficulty":"easy"}]}`
	_ = os.WriteFile(filepath.Join(dir, "exec_mini.json"), []byte(dsJSON), 0o644)
	ds, err := LoadDataset(filepath.Join(dir, "exec_mini.json"))
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestExecutionLevelAlignment(t *testing.T) {
	ds := execDataset(t)
	target := &pkgspec.Spec{Name: "evo", Concepts: []pkgspec.Concept{{Name: "计算节点"}}, Relations: []pkgspec.Relation{}, Instances: []pkgspec.Instance{}}

	// 桩执行通道：结果集中含 gold URN（归一形态）
	exec := func(ctx context.Context, oid, q string) ([]map[string]any, error) {
		return []map[string]any{
			// value 为引擎存 URI 的 SanitizeURN 归一形态（与 IRI 模板一致）
			{"x": map[string]any{"type": "uri", "value": "urn_o_evo_concept_计算节点"}},
		}, nil
	}
	rep := Run(context.Background(), execStubLLM(), target, "evo", 1, ds, nil, WithExecutor(exec))
	if len(rep.Signals) != 1 || !rep.Signals[0].GoldAligned {
		t.Fatalf("执行级对齐应命中: %+v", rep.Signals)
	}
	sig := rep.Signals[0]
	if !sig.Executed || sig.GoldHitCount != 1 || sig.GoldTotal != 1 || sig.GoldOverlap != 1 {
		t.Fatalf("执行级证据不符: %+v", sig)
	}
	if rep.MetricDefinition == nil || rep.MetricDefinition.GoldAlignMode != "executed" {
		t.Fatalf("口径应标 executed: %+v", rep.MetricDefinition)
	}

	// 执行失败：回落文本子串口径（URN 不在查询文本——文本口径下不命中，但 mode 如实标注）
	execFail := func(ctx context.Context, oid, q string) ([]map[string]any, error) {
		return nil, context.DeadlineExceeded
	}
	rep2 := Run(context.Background(), execStubLLM(), target, "evo", 1, ds, nil, WithExecutor(execFail))
	if rep2.Signals[0].Executed {
		t.Fatal("执行失败不应标 executed")
	}
	if rep2.MetricDefinition.GoldAlignMode != "text-substring-fallback" {
		t.Fatalf("回落口径应标注: %+v", rep2.MetricDefinition)
	}

	// 无执行通道：旧行为（文本子串）
	rep3 := Run(context.Background(), execStubLLM(), target, "evo", 1, ds, nil)
	if rep3.MetricDefinition.GoldAlignMode != "text-substring-fallback" {
		t.Fatalf("无通道应回落口径: %+v", rep3.MetricDefinition)
	}
}

var _ = time.Second
