package ontochat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
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

const (
	paraResp2 = `{"groups":[
		{"keep":"疾病通常表现出哪些典型症状？","duplicates":[]},
		{"keep":"某种症状可能提示哪些疾病？","duplicates":[]},
		{"keep":"药物可以治疗哪些疾病？","duplicates":["药物能够治疗什么疾病？"]},
		{"keep":"症状如何分级？","duplicates":[]}
	]}`
	clusterResp2 = `{"clusters":[
		{"label":"症状-疾病映射","cqs":["疾病通常表现出哪些典型症状？","某种症状可能提示哪些疾病？"]},
		{"label":"药物","cqs":["药物可以治疗哪些疾病？"]},
		{"label":"","cqs":["孤儿簇问题？"]}
	]}`
)

// REQ-273 + REQ-342/OC-3：两步主链——paraphrase 去重（明细 groups）→ 主题聚类；
// 孤儿簇剔除+覆盖兜底（漏归簇条目进「未归类」不丢失）。
func TestAnalyzeCQsOK(t *testing.T) {
	srv, _ := analyzeStub(t, func(n int) string {
		if n == 1 { // paraphrase 步
			return paraResp2
		}
		return clusterResp2 // 聚类步
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	sess := analyzeSession("疾病通常表现出哪些典型症状？", "某种症状可能提示哪些疾病？", "药物可以治疗哪些疾病？", "药物能够治疗什么疾病？", "症状如何分级？")
	res, err := e.AnalyzeCQs(context.Background(), sess, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 簇：症状-疾病映射 / 药物 / 未归类（症状如何分级？未归簇→覆盖兜底）；空 label 孤儿簇剔除
	if len(res.Clusters) != 3 {
		t.Fatalf("簇数不符（含未归类兜底）: %+v", res.Clusters)
	}
	if res.Clusters[0].Label != "症状-疾病映射" || len(res.Clusters[0].CQs) != 2 {
		t.Fatalf("簇内容不符: %+v", res.Clusters[0])
	}
	if res.Clusters[2].Label != "未归类" || len(res.Clusters[2].CQs) != 1 {
		t.Fatalf("覆盖兜底未归类簇不符: %+v", res.Clusters[2])
	}
	// paraphrase 明细：药物组保留 1 合并 1
	if res.Dedup != 1 || len(res.MergedGroups) != 4 {
		t.Fatalf("去重明细不符: dedup=%d groups=%+v", res.Dedup, res.MergedGroups)
	}
	found := false
	for _, g := range res.MergedGroups {
		if g.Keep == "药物可以治疗哪些疾病？" && len(g.Duplicates) == 1 && g.Duplicates[0] == "药物能够治疗什么疾病？" {
			found = true
		}
	}
	if !found {
		t.Fatalf("合并明细缺失: %+v", res.MergedGroups)
	}
	if res.DedupSkipped {
		t.Fatal("正常路径不应标注降级")
	}
}

// REQ-342/OC-3：paraphrase 步失败降级（两轮全坏）→ 跳过去重直接聚类（dedup_skipped）。
func TestAnalyzeCQsDedupDegrade(t *testing.T) {
	srv, calls := analyzeStub(t, func(n int) string {
		if n <= 2 {
			return "不是 json" // paraphrase 两轮全坏
		}
		return `{"clusters":[{"label":"主题","cqs":["疾病如何按照类别进行层级划分？"]}]}`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	res, err := e.AnalyzeCQs(context.Background(), analyzeSession("疾病如何按照类别进行层级划分？"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DedupSkipped || res.Dedup != 0 || len(res.MergedGroups) != 0 {
		t.Fatalf("应降级跳过去重: %+v", res)
	}
	if len(res.Clusters) != 1 || *calls != 3 {
		t.Fatalf("降级后应直接聚类: calls=%d clusters=%+v", *calls, res.Clusters)
	}
}

// REQ-273：指定簇数进 prompt；聚类坏 JSON 回喂重试（paraphrase 正常应答）。
func TestAnalyzeCQsFixedClustersAndRetry(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		if strings.Contains(gotBody, "同义识别") { // paraphrase 步
			_, _ = w.Write([]byte(`{"draft_json":` + strconvQuote(`{"groups":[{"keep":"疾病如何按照类别进行层级划分？","duplicates":[]}]}`) + `,"usage":null}`))
			return
		}
		resp := `不是 json`
		if strings.Contains(gotBody, "上一次输出不是合法") {
			resp = `{"clusters":[{"label":"主题","cqs":["疾病如何按照类别进行层级划分？"]}]}`
		}
		_, _ = w.Write([]byte(`{"draft_json":` + strconvQuote(resp) + `,"usage":null}`))
	}))
	defer srv.Close()
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	res, err := e.AnalyzeCQs(context.Background(), analyzeSession("疾病如何按照类别进行层级划分？"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Clusters) != 1 || res.Clusters[0].Label != "主题" {
		t.Fatalf("回喂重试未生效: %+v", res.Clusters)
	}
	if !strings.Contains(gotBody, "恰好分为 3 簇") {
		t.Fatalf("指定簇数规则未进 prompt: %.200s", gotBody)
	}
}

// REQ-273：会话无 CQ → 引导报错；两步全坏 → 解析失败留痕。
func TestAnalyzeCQsGuards(t *testing.T) {
	e := &Engine{LLM: llmcreate.New("http://127.0.0.1:1")}
	if _, err := e.AnalyzeCQs(context.Background(), analyzeSession(), 0); err == nil || !strings.Contains(err.Error(), "尚无能力问题") {
		t.Fatalf("空 CQ 应引导报错: %v", err)
	}
	srv, _ := analyzeStub(t, func(int) string { return "不是 json" })
	e2 := &Engine{LLM: llmcreate.New(srv.URL)}
	if _, err := e2.AnalyzeCQs(context.Background(), analyzeSession("药物禁忌如何建模？"), 0); err == nil || !strings.Contains(err.Error(), "解析失败") {
		t.Fatalf("应报解析失败: %v", err)
	}
}
