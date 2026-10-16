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

// REQ-272 + REQ-342/OC-1：两步主链——抽取轮（只抽）→ 净化轮（拆非原子+专名抽象+统计）→ 与既有 CQ 去重。
func TestExtractCQsOK(t *testing.T) {
	srv, _ := cqStubServer(t, func(n int) string {
		if n == 1 { // 抽取轮
			return `[{"cq":"青霉素可治疗哪些疾病？","origin":"抽取"},{"cq":"青霉素的发现者是谁、能治哪些疾病？","origin":"抽取"}]`
		}
		// 净化轮：拆分 1 条 + 抽象 2 条（其一与既有重复将滤除）
		return `{"refined":[{"cq":"药物可治疗哪些疾病？","origin":"抽象"},{"cq":"药物由哪些人员发现？","origin":"拆分"},{"cq":"药物可治疗哪些疾病？","origin":"抽象"}],"stats":{"compound_found":1,"named_found":2}}`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	sess := cqTestSession("药物可治疗哪些疾病？") // 既有重复应滤除
	res, err := e.ExtractCQs(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CQs) != 1 || res.CQs[0].CQ != "药物由哪些人员发现？" || res.CQs[0].Origin != "拆分" {
		t.Fatalf("净化/去重结果不符: %+v", res.CQs)
	}
	// 既有重复 1 + 批内重复 1（抽象重复输出）→ dedup=2
	if res.Dedup != 2 {
		t.Fatalf("重复计数应为 2: %d", res.Dedup)
	}
	// 净化统计在净化原始产出上计（去重前）：识别复合 1 拆出 1；含专名 2 抽象 2
	if res.Purification == nil || res.Purification.CompoundFound != 1 || res.Purification.SplitOut != 1 ||
		res.Purification.NamedFound != 2 || res.Purification.Abstracted != 2 {
		t.Fatalf("净化统计不符: %+v", res.Purification)
	}
	if res.PurificationDegraded {
		t.Fatal("正常路径不应降级")
	}
}

// REQ-342/OC-1：净化轮输出不合法→回喂重试；两轮全坏→降级用抽取原清单（degraded 如实标注）。
func TestExtractCQsPurifyDegrade(t *testing.T) {
	srv, calls := cqStubServer(t, func(n int) string {
		if n == 1 {
			return `[{"cq":"疾病如何分级？","origin":"抽取"}]`
		}
		return "净化轮不是 json"
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	res, err := e.ExtractCQs(context.Background(), cqTestSession())
	if err != nil {
		t.Fatal(err)
	}
	if !res.PurificationDegraded || res.Purification != nil {
		t.Fatalf("应降级且无统计: %+v", res)
	}
	if len(res.CQs) != 1 || res.CQs[0].CQ != "疾病如何分级？" {
		t.Fatalf("降级应保留抽取原清单: %+v", res.CQs)
	}
	if *calls != 3 { // 抽取 1 + 净化 2（重试轮耗尽）
		t.Fatalf("净化重试轮数不符: %d", *calls)
	}
}

// REQ-272：抽取轮坏 JSON 回喂重试，第二轮成功（净化轮第 3 调用正常）。
func TestExtractCQsRetryOnBadJSON(t *testing.T) {
	srv, calls := cqStubServer(t, func(n int) string {
		if n == 1 {
			return "好的，以下是能力问题：1. ……（非 JSON）"
		}
		if n == 2 {
			return `[{"cq":"药物由哪些人员发现？","origin":"抽取"}]`
		}
		return `{"refined":[{"cq":"药物由哪些人员发现？","origin":"抽取"}],"stats":{}}`
	})
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	res, err := e.ExtractCQs(context.Background(), cqTestSession())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CQs) != 1 || *calls != 3 {
		t.Fatalf("回喂重试未生效: calls=%d cqs=%+v", *calls, res.CQs)
	}
}

// REQ-272：抽取轮两轮全坏 → 错误留痕（job error 路径）。
func TestExtractCQsAllBad(t *testing.T) {
	srv, _ := cqStubServer(t, func(int) string { return "不是 json" })
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	if _, err := e.ExtractCQs(context.Background(), cqTestSession()); err == nil || !strings.Contains(err.Error(), "解析失败") {
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
	if _, err := e.ExtractCQs(context.Background(), cqTestSession()); err == nil {
		t.Fatal("空清单应报错")
	}
}
