package ontochat

// REQ-306 A3 domain 自由问答槽单测：isQuestion 规则组 + 问答轮零副作用（不进 Hints）+ 材料轮不受影响。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

func TestIsQuestionRules(t *testing.T) {
	yes := []string{
		"概念 A 和 B 之间该建什么关系？",
		"how to model this?", // 问号结尾的任意文本
		"什么场景下需要建实例？",
		"如何划分概念层级",
		"为什么关系要有方向",
		"是否需要建约束",
		"应该先定义概念还是关系？",
		"哪些属性值得记录",
	}
	for _, q := range yes {
		if !isQuestion(q) {
			t.Fatalf("期望判定为疑问: %q", q)
		}
	}
	no := []string{
		"补充：药物分为处方药和非处方药两类", // 陈述材料
		"生成草稿", // 生成意图（先于 isQuestion 判定，此处仅验非疑问）
		"",     // 空
	}
	for _, s := range no {
		if isQuestion(s) {
			t.Fatalf("期望判定为非疑问: %q", s)
		}
	}
}

// 问答轮：疑问输入 → LLM 回复、Hints 零增长、stage 维持 domain；随后陈述句照旧进 Hints。
func TestTurnDomainQASlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(`{"reply":"建议建立「属于」关系，父概念在前。可以继续补充材料，或说「生成草稿」。"}`) + `,"usage":null}`))
	}))
	t.Cleanup(srv.Close)
	st := newJobTestStore(t)
	e := &Engine{LLM: llmcreate.New(srv.URL)}

	sess, err := st.Create("qa-slot-test", "问答槽测试")
	if err != nil {
		t.Fatal(err)
	}
	sess.Stage = "domain"
	sess.Context.Description = "医院药物管理领域"

	// 疑问轮：回答但不进 Hints
	res, err := e.Turn(context.Background(), st, sess, "概念 A 和 B 之间该建什么关系？")
	if err != nil {
		t.Fatal(err)
	}
	if res.NextStage != "domain" || res.Reply == "" {
		t.Fatalf("问答轮应维持 domain 且有回复: %+v", res)
	}
	if len(sess.Context.Hints) != 0 {
		t.Fatalf("问答轮不应进 Hints: %v", sess.Context.Hints)
	}

	// 材料轮：陈述句照旧累积
	res2, err := e.Turn(context.Background(), st, sess, "补充：药物分为处方药和非处方药两类")
	if err != nil {
		t.Fatal(err)
	}
	if res2.NextStage != "domain" || len(sess.Context.Hints) != 1 {
		t.Fatalf("材料轮应进 Hints（1）: %v %+v", sess.Context.Hints, res2)
	}
}
