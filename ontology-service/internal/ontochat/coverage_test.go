package ontochat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func covSpec() *pkgspec.Spec {
	return &pkgspec.Spec{
		Name:        "药物本体",
		Description: "药物与疾病对照",
		Concepts: []pkgspec.Concept{
			{Name: "Drug", Label: "药物", Definition: "用于预防治疗诊断疾病的物质"},
			{Name: "Disease", Label: "疾病", Parents: []string{"MedicalCondition"}, Definition: "机体异常状态"},
		},
		Relations: []pkgspec.Relation{{Name: "treats", Label: "治疗", From: "Drug", To: "Disease", Definition: "药物对疾病的治疗关系"}},
		Instances: []pkgspec.Instance{{Name: "Aspirin", Concept: "Drug", Attributes: map[string]any{"禁忌": "胃溃疡"}}},
	}
}

// REQ-274：口语化三段式——概念（label+父链+定义）/关系（from→to）/实例（概念+属性）全在位；
// 缺定义如实标注不臆造。
func TestVerbaliseSpec(t *testing.T) {
	v := VerbaliseSpec(covSpec())
	for _, want := range []string{"药物本体", "Disease（疾病），是 MedicalCondition 的子概念", "机体异常状态", "treats（治疗）：Drug → Disease", "Aspirin，是 Drug 的实例", "属性：禁忌"} {
		if !strings.Contains(v, want) {
			t.Fatalf("口语化缺段: %q", want)
		}
	}
	bare := covSpec()
	bare.Concepts[0].Definition = ""
	if !strings.Contains(VerbaliseSpec(bare), "Drug（药物）：（未填写定义）") {
		t.Fatal("缺定义应如实标注")
	}
}

func coverageStub(t *testing.T, respond func(cq string) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fullBytes, _ := io.ReadAll(r.Body)
		full := string(fullBytes)
		// 从请求 prompt 中抠出能力问题（{cq} 占位在末段）
		idx := strings.LastIndex(full, "能力问题：")
		cq := "未知"
		if idx >= 0 {
			cq = strings.TrimSpace(full[idx+len("能力问题："):])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconvQuote(respond(cq)) + `,"usage":null}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// REQ-274：逐条独立判定主链 + Unknown 鲁棒（单条失败不拖垮整批）。
func TestCoverageRun(t *testing.T) {
	srv := coverageStub(t, func(cq string) string {
		if strings.Contains(cq, "禁忌") {
			return `{"verdict":"No","explanation":"缺少禁忌建模"}`
		}
		return `{"verdict":"Yes","explanation":"treats 关系可回应"}`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	cqs := []string{"药物可治疗哪些疾病？", "药物的禁忌人群有哪些？", "药物如何分类？"}
	var lastDone, lastTotal int
	verdicts, err := e.TestCoverage(context.Background(), covSpec(), cqs, func(done, total int, msg string) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 3 {
		t.Fatalf("应 3 条判定: %d", len(verdicts))
	}
	if verdicts[0].Verdict != "Yes" || verdicts[1].Verdict != "No" {
		t.Fatalf("判定不符: %+v", verdicts)
	}
	if lastDone != 3 || lastTotal != 3 {
		t.Fatalf("进度未达完成: %d/%d", lastDone, lastTotal)
	}
	sum := SummarizeCoverage(verdicts)
	if sum.Passed != 2 || sum.Total != 3 {
		t.Fatalf("汇总不符: %+v", sum)
	}
	// 单条调用失败 → Unknown 且继续（混合成败）；全部失败才整体报错
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "失败") {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":"{\"verdict\":\"Yes\",\"explanation\":\"ok\"}","usage":null}`))
	}))
	defer srv2.Close()
	e2 := &Engine{LLM: llmcreate.New(srv2.URL)}
	verdicts2, err := e2.TestCoverage(context.Background(), covSpec(), []string{"药物禁忌？", "药物如何分类（失败样例）？"})
	if err != nil || len(verdicts2) != 2 || verdicts2[0].Verdict != "Yes" || verdicts2[1].Verdict != "Unknown" {
		t.Fatalf("单条失败应记 Unknown 继续: %+v %v", verdicts2, err)
	}
	if _, err := e2.TestCoverage(context.Background(), covSpec(), []string{"全部失败一", "全部失败二"}); err == nil {
		t.Fatal("全部失败应整体报错")
	}
	if _, err := e2.TestCoverage(context.Background(), covSpec(), nil); err == nil {
		t.Fatal("空 CQ 应报错引导")
	}
}
