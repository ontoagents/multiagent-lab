package rest

// REQ-207 余项：进化诊断→归因→补丁端点单测（桩 LLM 平台 + 迷你基准集；沿 rest_test 夹具口径）。
// 覆盖：全链产出候选（proposed+layer/evidence/patch）/全绿不出候选/Evidence 未锚定拒绝/轮次预算 409。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/evolution"
	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

const diagSpecJSON = `{"name":"evo","concepts":[
 {"name":"计算节点","label":"计算节点","definition":"执行计算的物理或虚拟节点"},
 {"name":"存储节点","label":"存储节点","definition":"提供持久化存储的节点"}],
 "relations":[{"name":"挂载","from":"计算节点","to":"存储节点"}],"instances":[]}`

// diagStubLLM 桩 LLM 平台：翻译问（本体查询专家）返回合法 SELECT；归因问（本体进化专家）
// 按 attribFn 产出（可注入 Evidence 未锚定形态）。
func diagStubLLM(t *testing.T, attribFn func() string) *llmcreate.Creator {
	t.Helper()
	plt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var draft string
		switch {
		case strings.Contains(req.Prompt, "本体查询专家"):
			draft = "SELECT ?x WHERE { ?x a <urn_o_evo_concept_计算节点> } LIMIT 10" // URN 经 SanitizeURN 归一（冒号→下划线），gold 对齐按归一形态 containment
		case strings.Contains(req.Prompt, "本体进化专家"):
			draft = attribFn()
		default:
			draft = `{"reply":"ok"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"draft_json": draft})
	}))
	t.Cleanup(plt.Close)
	return llmcreate.New(plt.URL)
}

// diagMiniDataset 迷你基准集（1 条 gold 指向本体不存在的概念——翻译过/白名单过/gold 失败=失评测信号）。
func diagMiniDataset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ds := map[string]any{
		"name": "diag_mini", "ontology": "seed_x", "revision": "r1-test", "note": "诊断单测",
		"cqs": []map[string]any{{
			"id": "Q01", "cq": "有哪些容器节点负责运行负载？", "gold": []string{"urn:o:evo:concept:容器节点"},
			"type": "E", "kind": "fact", "difficulty": "easy",
		}},
	}
	b, _ := json.Marshal(ds)
	if err := os.WriteFile(filepath.Join(dir, "diag_mini.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func diagNewServer(t *testing.T, attribFn func() string) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "evo_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("evo", "进化诊断测试本体", ""); err != nil {
		t.Fatalf("建本体失败: %v", err)
	}
	if err := st.PutArtifact("evo", "spec_json", diagSpecJSON, true); err != nil {
		t.Fatalf("写 spec 失败: %v", err)
	}
	mux := http.NewServeMux()
	New(st, nil, diagStubLLM(t, attribFn)).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func diagPollJob(t *testing.T, srv *httptest.Server, jobID string) *draftJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(srv.URL + "/api/ai-draft-jobs/" + jobID)
		if err != nil {
			t.Fatalf("轮询失败: %v", err)
		}
		var job draftJob
		_ = json.NewDecoder(resp.Body).Decode(&job)
		resp.Body.Close()
		if job.Status == "done" || job.Status == "failed" {
			return &job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job 超时未完成")
	return nil
}

func TestEvolutionDiagnoseFullChain(t *testing.T) {
	t.Setenv("CQ_BENCH_DIR", diagMiniDataset(t))
	attrib := `{"layer":"content","summary":"本体缺「容器节点」概念，Q01 指向的覆盖缺口","evidence":"基准 Q01（有哪些容器节点负责运行负载？）gold 对齐失败——查询无法引用答案依赖的容器节点概念，指向内容层缺口","patch":{"add_concepts":[{"name":"容器节点","label":"容器节点","definition":"运行工作负载的节点","parents":["计算节点"]}]}}`
	srv, st := diagNewServer(t, func() string { return attrib })

	resp, err := http.Post(srv.URL+"/api/ontology/evolution/diagnose", "application/json", strings.NewReader(`{"ontology_id":"evo","dataset":"diag_mini"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("期望 202 得到 %d", resp.StatusCode)
	}
	var started struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&started)
	job := diagPollJob(t, srv, started.JobID)
	if job.Status != "done" {
		t.Fatalf("job 应 done: %s %s", job.Status, job.Error)
	}
	d := job.Diagnose
	if d == nil || d.Candidate == nil {
		t.Fatalf("应产出候选: %+v", d)
	}
	if d.Candidate.Status != "proposed" || d.Candidate.Layer != "content" {
		t.Fatalf("候选形态不符: %+v", d.Candidate)
	}
	if !strings.Contains(d.Candidate.Evidence, "Q01") {
		t.Fatalf("Evidence 应锚定 Q01: %s", d.Candidate.Evidence)
	}
	var patch evolution.Patch
	if err := json.Unmarshal([]byte(d.Candidate.PatchJSON), &patch); err != nil || len(patch.AddConcepts) != 1 {
		t.Fatalf("补丁形态不符: %s %v", d.Candidate.PatchJSON, err)
	}
	if d.Report == nil || d.Report.Total != 1 || d.Report.GoldAlign != 0 {
		t.Fatalf("诊断报告不符: %+v", d.Report)
	}
	// 候选可在既有列表端点检索（同库）
	cands, err := evolution.New(st.DB()).ListByOntology("evo", "")
	if err != nil || len(cands) != 1 {
		t.Fatalf("候选应落库: %v %d", err, len(cands))
	}
}

func TestEvolutionDiagnoseAllGreen(t *testing.T) {
	// gold 指向既有概念=全绿路径：不出候选（不为改而改）
	t.Setenv("CQ_BENCH_DIR", func() string {
		dir := t.TempDir()
		ds := map[string]any{
			"name": "diag_green", "ontology": "seed_x", "revision": "r1-test",
			"cqs": []map[string]any{{
				"id": "Q01", "cq": "有哪些计算节点？", "gold": []string{"urn:o:evo:concept:计算节点"},
				"type": "E", "kind": "fact", "difficulty": "easy",
			}},
		}
		b, _ := json.Marshal(ds)
		_ = os.WriteFile(filepath.Join(dir, "diag_green.json"), b, 0o644)
		return dir
	}())
	srv, _ := diagNewServer(t, func() string { return "{}" })
	resp, _ := http.Post(srv.URL+"/api/ontology/evolution/diagnose", "application/json", strings.NewReader(`{"ontology_id":"evo"}`))
	var started struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&started)
	job := diagPollJob(t, srv, started.JobID)
	if job.Status != "done" || job.Diagnose == nil || job.Diagnose.Candidate != nil {
		t.Fatalf("全绿应 done 且无候选: %s err=%s", job.Status, job.Error)
	}
	if !strings.Contains(job.Diagnose.Note, "无进化建议") {
		t.Fatalf("全绿注记不符: %s", job.Diagnose.Note)
	}
}

func TestEvolutionDiagnoseEvidenceNotAnchored(t *testing.T) {
	t.Setenv("CQ_BENCH_DIR", diagMiniDataset(t))
	attrib := `{"layer":"content","summary":"我觉得该加概念","evidence":"凭经验加一个概念比较好","patch":{"add_concepts":[{"name":"容器节点"}]}}`
	srv, _ := diagNewServer(t, func() string { return attrib })
	resp, _ := http.Post(srv.URL+"/api/ontology/evolution/diagnose", "application/json", strings.NewReader(`{"ontology_id":"evo"}`))
	var started struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&started)
	job := diagPollJob(t, srv, started.JobID)
	if job.Status != "failed" || !strings.Contains(job.Error, "Evidence 未锚定") {
		t.Fatalf("Evidence 未锚定应 failed: %s %s", job.Status, job.Error)
	}
}

func TestEvolutionDiagnoseBudgetExhausted(t *testing.T) {
	t.Setenv("CQ_BENCH_DIR", diagMiniDataset(t))
	srv, st := diagNewServer(t, func() string { return "{}" })
	es := evolution.New(st.DB())
	for i := 1; i <= evolution.MaxRoundsPerPass; i++ {
		if _, err := es.Create(&evolution.Candidate{OntologyID: "evo", BaseVersion: 1, Layer: "content", Summary: "s", PatchJSON: `{}`, Round: i}); err != nil {
			t.Fatal(err)
		}
	}
	resp, _ := http.Post(srv.URL+"/api/ontology/evolution/diagnose", "application/json", strings.NewReader(`{"ontology_id":"evo"}`))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("预算用尽应 409，得 %d", resp.StatusCode)
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if !strings.Contains(e.Error, fmt.Sprint(evolution.MaxRoundsPerPass)) {
		t.Fatalf("409 信息应含预算: %s", e.Error)
	}
}
