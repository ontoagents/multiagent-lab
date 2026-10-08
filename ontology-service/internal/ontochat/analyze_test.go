package ontochat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

func analyzeSession(cqs ...string) *Session {
	return &Session{ID: "sess-an", Stage: "domain", Context: Context{Description: "医学常识本体", CQs: cqs}}
}

func analyzeStub(t *testing.T, respond func(call int) string) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconvQuote(respond(calls)) + `,"usage":null}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// REQ-273：聚类主链——簇净化（trim/去空/跨簇去重）+ dedup 计数（输入-输出）。
func TestAnalyzeCQsOK(t *testing.T) {
	srv, _ := analyzeStub(t, func(int) string {
		return `{"clusters":[
			{"label":"症状-疾病映射","cqs":["疾病通常表现出哪些典型症状？","某种症状可能提示哪些疾病？"]},
			{"label":"药物","cqs":["药物可以治疗哪些疾病？","药物可以治疗哪些疾病？",""]},
			{"label":"","cqs":["孤儿簇问题？"]}
		]}`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	sess := analyzeSession("疾病通常表现出哪些典型症状？", "某种症状可能提示哪些疾病？", "药物可以治疗哪些疾病？", "症状如何分级？")
	clusters, dedup, err := e.AnalyzeCQs(context.Background(), sess, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 2 {
		t.Fatalf("空 label 孤儿簇应被剔除: %+v", clusters)
	}
	if clusters[0].Label != "症状-疾病映射" || len(clusters[1].CQs) != 1 {
		t.Fatalf("簇内容不符: %+v", clusters)
	}
	// 输入 4 条，输出 3 条（药物簇内重复 1 条被去重）→ dedup=1
	if dedup != 1 {
		t.Fatalf("dedup 应为 1: %d", dedup)
	}
}

// REQ-273：指定簇数进 prompt；坏 JSON 回喂重试。
func TestAnalyzeCQsFixedClustersAndRetry(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		resp := `不是 json`
		if strings.Contains(gotBody, "上一次输出不是合法") {
			resp = `{"clusters":[{"label":"主题","cqs":["疾病如何按照类别进行层级划分？"]}]}`
		}
		_, _ = w.Write([]byte(`{"draft_json":` + strconvQuote(resp) + `,"usage":null}`))
	}))
	defer srv.Close()
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	cqs, _, err := e.AnalyzeCQs(context.Background(), analyzeSession("疾病如何按照类别进行层级划分？"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(cqs) != 1 || cqs[0].Label != "主题" {
		t.Fatalf("回喂重试未生效: %+v", cqs)
	}
	if !strings.Contains(gotBody, "恰好分为 3 簇") {
		t.Fatalf("指定簇数规则未进 prompt: %.200s", gotBody)
	}
}

// REQ-273：会话无 CQ → 引导报错；两轮全坏 → 解析失败留痕。
func TestAnalyzeCQsGuards(t *testing.T) {
	e := &Engine{LLM: llmcreate.New("http://127.0.0.1:1")}
	if _, _, err := e.AnalyzeCQs(context.Background(), analyzeSession(), 0); err == nil || !strings.Contains(err.Error(), "尚无能力问题") {
		t.Fatalf("空 CQ 应引导报错: %v", err)
	}
	srv, _ := analyzeStub(t, func(int) string { return "不是 json" })
	e2 := &Engine{LLM: llmcreate.New(srv.URL)}
	if _, _, err := e2.AnalyzeCQs(context.Background(), analyzeSession("药物禁忌如何建模？"), 0); err == nil || !strings.Contains(err.Error(), "解析失败") {
		t.Fatalf("应报解析失败: %v", err)
	}
}
