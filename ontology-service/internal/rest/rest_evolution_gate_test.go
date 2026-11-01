package rest

// REQ-547/M354（决策结构化借鉴批④；105 号 P1-4）gateEvolutionCandidate 端点级单测——
// 第三信号（with_benchmark 双跑）+ case 案例级计分 + benchRegressed 门控回归 + 默认关回归。
// 桩 LLM+迷你数据集沿 rest_evolution_diagnose_test 夹具口径；执行通道指向不可达端口
// （执行失败→文本子串回落口径，确定性不触网）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// gateStubLLM 桩 LLM：翻译问恒返引用「计算节点」的合法 SELECT（base/patched 同形=无回归基线；
// regressed 用例改按提示词内可用概念分流——patched 含「容器节点」时改译容器节点=gold 失对齐）。
func gateStubLLM(t *testing.T, divertOnPatched bool) *llmcreate.Creator {
	t.Helper()
	plt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		draft := "SELECT ?x WHERE { ?x a <urn_o_evo_concept_计算节点> } LIMIT 10"
		if divertOnPatched && strings.Contains(req.Prompt, "容器节点") {
			draft = "SELECT ?x WHERE { ?x a <urn_o_evo_concept_容器节点> } LIMIT 10"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"draft_json": draft})
	}))
	t.Cleanup(plt.Close)
	return llmcreate.New(plt.URL)
}

// gateMiniDataset 迷你基准集：1 fact（gold=计算节点）+case 期望可答（expected=[计算节点]）+
// case 期望不可答（expected=[幽灵概念]——种子不承载，桩查询零引用即判不可答）。
func gateMiniDataset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ds := map[string]any{
		"name": "gate_mini", "ontology": "seed_x", "revision": "r1-test", "note": "门控第三信号单测",
		"cqs": []map[string]any{
			{"id": "Q01", "cq": "有哪些计算节点？", "gold": []string{"计算节点"}, "type": "E", "kind": "fact", "difficulty": "easy"},
			{"id": "C01", "cq": "哪些计算节点挂载了存储节点？", "gold": []string{}, "type": "R", "kind": "case", "difficulty": "medium",
				"expected_concepts": []string{"计算节点"}, "expected_answerable": true},
			{"id": "C02", "cq": "计算节点的维护人与值班表是什么？", "gold": []string{}, "type": "E", "kind": "case", "difficulty": "hard",
				"expected_concepts": []string{"幽灵概念"}, "expected_answerable": false},
		},
	}
	b, _ := json.Marshal(ds)
	if err := os.WriteFile(filepath.Join(dir, "gate_mini.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func gateNewServer(t *testing.T, llm *llmcreate.Creator) *httptest.Server {
	t.Helper()
	t.Setenv("RUNTIME_MGR_URL", "http://127.0.0.1:1") // 执行通道快速失败→文本回落口径（确定性）
	st, err := repo.Open(filepath.Join(t.TempDir(), "gate_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("evo", "门控测试本体", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("evo", "spec_json", diagSpecJSON, true); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, nil, llm).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// gateCandidate 经 REST 提交候选返回 id。
func gateCandidate(t *testing.T, srv *httptest.Server, patch string) string {
	t.Helper()
	body := `{"layer":"content","summary":"门控单测候选","patch":` + patch + `}`
	resp, err := http.Post(srv.URL+"/api/ontologies/evo/evolution/candidates", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("提交候选应 201，got %d", resp.StatusCode)
	}
	var c struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	if c.ID == "" {
		t.Fatal("候选 id 为空")
	}
	return c.ID
}

type gateRep struct {
	Passed   bool `json:"passed"`
	Verdict  string `json:"verdict"`
	Benchmark *struct {
		Dataset  string  `json:"dataset"`
		Total    int     `json:"total"`
		Regressed bool   `json:"regressed"`
		Base     map[string]any `json:"base"`
		Patched  map[string]any `json:"patched"`
		Note     string  `json:"note"`
	} `json:"benchmark"`
}

func gateRun(t *testing.T, srv *httptest.Server, cid, body string) gateRep {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/ontologies/evo/evolution/candidates/"+cid+"/gate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gate 应 200，got %d", resp.StatusCode)
	}
	var out struct {
		Passed     bool            `json:"passed"`
		GateReport json.RawMessage `json:"gate_report"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	var rep gateRep
	if err := json.Unmarshal(out.GateReport, &rep); err != nil {
		t.Fatalf("gate_report 解析失败: %v", err)
	}
	if rep.Passed != out.Passed {
		t.Fatal("passed 顶层与报告不一致")
	}
	return rep
}

// ① 第三信号开启：双跑三率对照+case 案例级计分（期望可答⊇/期望不可答零引用）+无回归通过。
func TestEvolutionGateCaseScoring_Req547(t *testing.T) {
	t.Setenv("CQ_BENCH_DIR", gateMiniDataset(t))
	srv := gateNewServer(t, gateStubLLM(t, false))
	cid := gateCandidate(t, srv, `{"add_concepts":[{"name":"容器节点","label":"容器节点","definition":"运行工作负载的节点","parents":["计算节点"]}]}`)
	rep := gateRun(t, srv, cid, `{"with_benchmark":true,"dataset":"gate_mini"}`)
	if rep.Benchmark == nil {
		t.Fatal("with_benchmark 开启应产出 benchmark 块")
	}
	b := rep.Benchmark
	if b.Dataset != "gate_mini" || b.Total != 3 {
		t.Fatalf("数据集透出失真: %s %d", b.Dataset, b.Total)
	}
	// 文本回落口径：fact ✓（查询引用计算节点）+case 可答 ✓（hits=1⊇1）+case 不可答 ✓（幽灵概念零引用）→ 3/3
	if got := numf(b.Base["gold_aligned"]); got != 3 {
		t.Fatalf("base gold_aligned 应 3（fact+case可答+case不可答全过），got %v", b.Base["gold_aligned"])
	}
	if got := numf(b.Patched["gold_aligned"]); got != 3 {
		t.Fatalf("patched gold_aligned 应 3（同形桩无回归），got %v", b.Patched["gold_aligned"])
	}
	if b.Regressed || !rep.Passed {
		t.Fatalf("同形桩应无回归通过: regressed=%v passed=%v verdict=%s", b.Regressed, rep.Passed, rep.Verdict)
	}
	if !strings.Contains(b.Note, "REQ-547") {
		t.Fatalf("第三信号注应含 case 口径: %s", b.Note)
	}

	// ② 默认关回归：不带 with_benchmark → benchmark 缺省（成本护栏——UI/脚本旧调用零变化）
	cid2 := gateCandidate(t, srv, `{"add_concepts":[]}`)
	rep2 := gateRun(t, srv, cid2, `{}`)
	if rep2.Benchmark != nil {
		t.Fatalf("默认关不应产出 benchmark 块: %+v", rep2.Benchmark)
	}
	if !rep2.Passed {
		t.Fatal("默认双信号门控应通过（结构完好质量不降）")
	}
}

// ③ benchRegressed：patched 译貌改指新增概念（gold 失对齐）→ 三率回退计入门控不过。
func TestEvolutionGateBenchRegressed_Req547(t *testing.T) {
	dir := t.TempDir()
	ds := map[string]any{
		"name": "gate_reg", "ontology": "seed_x", "revision": "r1-test",
		"cqs": []map[string]any{
			{"id": "Q01", "cq": "有哪些计算节点？", "gold": []string{"计算节点"}, "type": "E", "kind": "fact", "difficulty": "easy"},
		},
	}
	b, _ := json.Marshal(ds)
	if err := os.WriteFile(filepath.Join(dir, "gate_reg.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CQ_BENCH_DIR", dir)
	srv := gateNewServer(t, gateStubLLM(t, true))
	cid := gateCandidate(t, srv, `{"add_concepts":[{"name":"容器节点","label":"容器节点","definition":"运行工作负载的节点","parents":["计算节点"]}]}`)
	rep := gateRun(t, srv, cid, `{"with_benchmark":true,"dataset":"gate_reg"}`)
	if rep.Benchmark == nil || !rep.Benchmark.Regressed {
		t.Fatalf("patched gold 失对齐应判回归: %+v", rep.Benchmark)
	}
	if rep.Passed {
		t.Fatalf("三率回退应门控不过: %s", rep.Verdict)
	}
	if !strings.Contains(rep.Verdict, "三率回退") {
		t.Fatalf("verdict 应注明回退语义: %s", rep.Verdict)
	}
}

func numf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	default:
		return -1
	}
}
