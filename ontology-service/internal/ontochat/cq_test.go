package ontochat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

func cqTestSession(cqs ...string) *Session {
	raw := json.RawMessage(`{}`)
	return &Session{ID: "sess-cq", Stage: "domain", Context: Context{Description: "医学常识本体：症状、疾病与药物", CQs: cqs, DraftSpec: &raw}}
}

func cqStubServer(t *testing.T, respond func(call int) string) (*httptest.Server, *int) {
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

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// REQ-272：抽取主链——解析成功 + 与既有 CQ 精确去重。
func TestExtractCQsOK(t *testing.T) {
	srv, _ := cqStubServer(t, func(int) string {
		return `[{"cq":"药物可治疗哪些疾病？","origin":"抽取"},{"cq":"疾病簇由哪些症状组成？","origin":"抽象"},{"cq":"药物可治疗哪些疾病？","origin":"抽取"}]`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	sess := cqTestSession("药物可治疗哪些疾病？") // 既有重复应滤除
	cqs, dup, err := e.ExtractCQs(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(cqs) != 1 || cqs[0].CQ != "疾病簇由哪些症状组成？" || cqs[0].Origin != "抽象" {
		t.Fatalf("去重/字段不符: %+v", cqs)
	}
	if dup != 2 {
		// 既有重复 1 + 批内重复 1：dedupeCQs 统一计数（输出只留 1 条）
		t.Fatalf("重复计数应为 2（既有+批内）: %d", dup)
	}
}

// REQ-272：首轮坏 JSON 回喂重试，第二轮成功。
func TestExtractCQsRetryOnBadJSON(t *testing.T) {
	srv, calls := cqStubServer(t, func(n int) string {
		if n == 1 {
			return "好的，以下是能力问题：1. ……（非 JSON）"
		}
		return `[{"cq":"药物由哪些人员发现？","origin":"抽取"}]`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	cqs, _, err := e.ExtractCQs(context.Background(), cqTestSession())
	if err != nil {
		t.Fatal(err)
	}
	if len(cqs) != 1 || *calls != 2 {
		t.Fatalf("回喂重试未生效: calls=%d cqs=%+v", *calls, cqs)
	}
}

// REQ-272：两轮全坏 → 错误留痕（job error 路径）。
func TestExtractCQsAllBad(t *testing.T) {
	srv, _ := cqStubServer(t, func(int) string { return "不是 json" })
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	if _, _, err := e.ExtractCQs(context.Background(), cqTestSession()); err == nil || !strings.Contains(err.Error(), "解析失败") {
		t.Fatalf("应报解析失败: %v", err)
	}
}

// REQ-272：材料拼装含领域描述与 hints（story 制品落地后并入的入口已就位）。
func TestBuildCQMaterial(t *testing.T) {
	sess := cqTestSession()
	sess.Context.Hints = []string{"关注常见病症状簇", "用药禁忌需建模"}
	m := buildCQMaterial(sess)
	if !strings.Contains(m, "医学常识本体") || !strings.Contains(m, "1. 关注常见病症状簇") || !strings.Contains(m, "2. 用药禁忌需建模") {
		t.Fatalf("材料拼装不符: %q", m)
	}
}

// REQ-272：空产出报错（引导补充而非静默空清单）。
func TestExtractCQsEmpty(t *testing.T) {
	srv, _ := cqStubServer(t, func(int) string { return `[]` })
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	if _, _, err := e.ExtractCQs(context.Background(), cqTestSession()); err == nil {
		t.Fatal("空清单应报错")
	}
}
