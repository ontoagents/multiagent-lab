package api

// REQ-524/M331 handler 级单测：LLM 桩注入（quizLLM 字段）——出题概念校验+批改后掌握度真链
// 落表（冒烟 mock submit 旁路端点，掌握度真链由本测承载）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/ontology"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func quizTestServer(t *testing.T) (*Server, *store.Store) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret")) // LoadKeyFile 自动生成密钥文件
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Store: st, Box: box, DBPath: filepath.Join(dir, "platform.db")}, st
}

func TestQuizSubmitUpdatesMastery(t *testing.T) {
	s, st := quizTestServer(t)
	orig := quizLLM
	t.Cleanup(func() { quizLLM = orig })
	quizLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		return `{"score":0.9,"verdict":"correct","feedback":"回答正确"}`, nil
	}
	q, err := st.InsertQuizItem(&store.QuizItem{OntologyID: "oq1", ConceptNames: []string{"概念A", "概念B"}, Question: "Q?", Answer: "A"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"answer":"用户作答"}`))
	req.SetPathValue("id", "oq1")
	req.SetPathValue("qid", q.ID)
	rec := httptest.NewRecorder()
	s.quizSubmit(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	ms, err := st.ListMastery("oq1")
	if err != nil || len(ms) != 2 {
		t.Fatalf("批改应落两概念掌握度: %v %d", err, len(ms))
	}
	for _, m := range ms {
		if m.AskedCount != 1 || m.CorrectCount != 1 || m.Score != 0.9 {
			t.Fatalf("掌握度值不符: %+v", m)
		}
	}
}

func TestQuizGenerateValidatesConcepts(t *testing.T) {
	s, _ := quizTestServer(t)
	// 途经构建平面——不可达时 502 人话（LLM 桩不被调到也不影响断言）
	orig := quizLLM
	t.Cleanup(func() { quizLLM = orig })
	quizLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		return `[{"question":"Q","answer":"A","concepts":["真实概念","编造概念"]}]`, nil
	}
	s.Ontology = ontology.NewService()
	s.Ontology.BuildURL = "http://127.0.0.1:1" // 必不可达端口
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"count":2}`))
	req.SetPathValue("id", "oq-none")
	rec := httptest.NewRecorder()
	s.quizGenerate(rec, req)
	if rec.Code != 502 {
		t.Fatalf("构建平面不可达应 502，got %d body=%s", rec.Code, rec.Body.String())
	}
}

// quizSpecStub 构建平面 spec 桩（httptest——REQ-572 出题定向/子图端点需可读 spec）。
func quizSpecStub(t *testing.T, specJSON string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(specJSON))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const quizStubSpec = `{"concepts":[
	{"name":"订单","label":"订单","parents":["单据"]},
	{"name":"单据","label":"单据"},
	{"name":"客户","label":"客户"}],
 "relations":[{"name":"下单","from":"客户","to":"订单","label":"下单"}]}`

// TestQuizSubmitObjectiveDeterministic REQ-572：客观题确定性判定不走 LLM（choice 对/错两路）+
// 批改落档 quiz_attempt 两路各一条。
func TestQuizSubmitObjectiveDeterministic(t *testing.T) {
	s, st := quizTestServer(t)
	orig := quizLLM
	t.Cleanup(func() { quizLLM = orig })
	quizLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		t.Fatal("客观题判定不应调用 LLM")
		return "", context.Canceled
	}
	q, err := st.InsertQuizItem(&store.QuizItem{OntologyID: "oq2", ConceptNames: []string{"订单"},
		Question: "订单状态有哪些?", Answer: "已下单", Difficulty: "basic",
		QuestionType: "choice", Options: []string{"已下单", "已发货", "已收货"}})
	if err != nil {
		t.Fatal(err)
	}
	post := func(answer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"answer":"`+answer+`"}`))
		req.SetPathValue("id", "oq2")
		req.SetPathValue("qid", q.ID)
		rec := httptest.NewRecorder()
		s.quizSubmit(rec, req)
		return rec
	}
	rec := post("已下单")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Score   float64 `json:"score"`
		Verdict string  `json:"verdict"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Score != 1 || out.Verdict != "correct" {
		t.Fatalf("客观题答对应 correct/1: %+v", out)
	}
	rec = post("已发货")
	var out2 struct {
		Score    float64 `json:"score"`
		Verdict  string  `json:"verdict"`
		Feedback string  `json:"feedback"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out2)
	if out2.Verdict != "wrong" || out2.Score != 0 || !strings.Contains(out2.Feedback, "已下单") {
		t.Fatalf("客观题答错应 wrong/0 且反馈含正确答案: %+v", out2)
	}
	attempts, err := st.ListQuizAttempts("oq2", 10)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("两次提交应落档两条: %v %d", err, len(attempts))
	}
	if attempts[0].Verdict != "wrong" || attempts[1].Verdict != "correct" {
		t.Fatalf("落档序不符（时间倒序）: %+v", attempts)
	}
}

// TestQuizGenerateDedupAndNormalize REQ-572：历史+本批题干归一禁重复、非法 choice 降级简答、
// judge 归一、定向概念进提示词。
func TestQuizGenerateDedupAndNormalize(t *testing.T) {
	s, st := quizTestServer(t)
	s.Ontology = ontology.NewService()
	s.Ontology.BuildURL = quizSpecStub(t, quizStubSpec).URL
	// 既有历史题（"什么是订单?"）+ 薄弱掌握度（订单 0.2）——定向与禁重复数据源
	if _, err := st.InsertQuizItem(&store.QuizItem{OntologyID: "oq3", ConceptNames: []string{"订单"},
		Question: "什么是订单?", Answer: "单据"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMastery("oq3", "订单", false, 0.2); err != nil {
		t.Fatal(err)
	}
	var capturedUser string
	orig := quizLLM
	t.Cleanup(func() { quizLLM = orig })
	quizLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		capturedUser = user
		return `[{"type":"short_answer","question":"什么是 订单?","answer":"同考点换说法","concepts":["订单"]},
			{"type":"choice","question":"订单属于哪类?","answer":"不在选项里的答案","options":["单据","凭证"],"concepts":["订单"]},
			{"type":"judge","question":"订单是单据的一种，对吗?","answer":"正确","options":["正确","错误"],"concepts":["订单","单据"]},
			{"type":"short_answer","question":"客户的定义是什么?","answer":"参与下单的主体","concepts":["客户"]}]`, nil
	}
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"count":4}`))
	req.SetPathValue("id", "oq3")
	rec := httptest.NewRecorder()
	s.quizGenerate(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []*store.QuizItem `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Items) != 3 {
		t.Fatalf("重复题应被过滤（余 3 题）: %d", len(out.Items))
	}
	for _, it := range out.Items {
		if it.Question == "什么是 订单?" {
			t.Fatal("历史题换说法仍应被禁重复过滤")
		}
		if it.Question == "订单属于哪类?" && it.QuestionType != "short_answer" {
			t.Fatalf("答案不在选项内的 choice 应降级 short_answer: %s", it.QuestionType)
		}
	}
	if !strings.Contains(capturedUser, "订单") || !strings.Contains(capturedUser, "定向") {
		t.Fatal("薄弱概念应进入定向提示词")
	}
	if !strings.Contains(capturedUser, "什么是订单?") {
		t.Fatal("历史题干应进入禁重复提示段")
	}
}

// TestQuizMasteryGraph REQ-572：spec concepts/relations 与 mastery 合并出节点/边
// （covered/uncovered 标注+关系/父继承两族边+悬空关系防御）。
func TestQuizMasteryGraph(t *testing.T) {
	s, st := quizTestServer(t)
	s.Ontology = ontology.NewService()
	s.Ontology.BuildURL = quizSpecStub(t, quizStubSpec).URL
	if err := st.UpsertMastery("oq4", "订单", true, 0.9); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.SetPathValue("id", "oq4")
	rec := httptest.NewRecorder()
	s.quizMasteryGraph(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Nodes []struct {
			Name    string   `json:"name"`
			Covered bool     `json:"covered"`
			Score   float64  `json:"score"`
			Parents []string `json:"parents"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Kind string `json:"kind"`
		} `json:"edges"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Nodes) != 3 {
		t.Fatalf("3 概念 3 节点: %d", len(out.Nodes))
	}
	covered := map[string]bool{}
	for _, n := range out.Nodes {
		covered[n.Name] = n.Covered
		if n.Name == "订单" && (!n.Covered || n.Score != 0.9) {
			t.Fatalf("订单应 covered/0.9: %+v", n)
		}
	}
	if covered["客户"] {
		t.Fatal("客户从未考察应 uncovered")
	}
	if len(out.Edges) != 2 { // 下单 relation + 订单→单据 parent
		t.Fatalf("应 2 边（relation+parent）: %+v", out.Edges)
	}
	kinds := map[string]int{}
	for _, e := range out.Edges {
		kinds[e.Kind]++
	}
	if kinds["relation"] != 1 || kinds["parent"] != 1 {
		t.Fatalf("边族不符: %+v", out.Edges)
	}
}

// TestQuizTargetConcepts REQ-572 定向选点纯函数：薄弱优先、从未考察次之、近期已出不再定向。
func TestQuizTargetConcepts(t *testing.T) {
	spec := &quizSpec{Concepts: []quizConcept{
		{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"},
	}}
	mastery := []*store.MasteryLevel{
		{ConceptName: "A", Score: 0.2},
		{ConceptName: "B", Score: 0.9},
	}
	recent := []*store.QuizItem{{ConceptNames: []string{"C"}}} // C 已出未批改
	targets := quizTargetConcepts(spec, mastery, recent)
	if len(targets) != 2 || targets[0] != "A" || targets[1] != "D" {
		t.Fatalf("薄弱 A+从未考察 D 应入选，B 已掌握/C 已出除外: %v", targets)
	}
}

// TestQuizBoundedConceptSet REQ-608-f4bb：超限概念集有界化——定向保留/采样补足/截断。
func TestQuizBoundedConceptSet(t *testing.T) {
	orig := quizMaxPromptConcepts
	t.Cleanup(func() { quizMaxPromptConcepts = orig })
	quizMaxPromptConcepts = 8
	all := make([]quizConcept, 0, 30)
	long := strings.Repeat("义", 300)
	for i := 0; i < 30; i++ {
		all = append(all, quizConcept{Name: fmt.Sprintf("概念%02d", i), Definition: long,
			Synonyms: []string{"甲", "乙", "丙", "丁"}})
	}
	// 出题链组合与 handler 同序：先统一截断（quizTrimConcept）再有界化。
	trimmed := make([]quizConcept, len(all))
	for i, c := range all {
		trimmed[i] = quizTrimConcept(c)
	}
	got := quizBoundedConceptSet(trimmed, []string{"概念00", "概念29"})
	if len(got) != 8 {
		t.Fatalf("应截到上限 8: %d", len(got))
	}
	names := map[string]bool{}
	for _, c := range got {
		names[c.Name] = true
		if rn := []rune(c.Definition); len(rn) != 201 || !strings.HasSuffix(c.Definition, "…") {
			t.Fatalf("definition 应截断为 200 rune+省略号: %d", len(rn))
		}
		if len(c.Synonyms) != 3 {
			t.Fatalf("synonyms 应截前 3: %d", len(c.Synonyms))
		}
	}
	if !names["概念00"] || !names["概念29"] {
		t.Fatalf("定向概念应原样保留: %v", names)
	}
	if len(names) != 8 {
		t.Fatal("概念集应无重复")
	}
}

// TestQuizGenerateBoundedPrompt REQ-608-f4bb：超限本体出题——提示词概念数有界、
// 薄弱定向概念必入集、响应如实透出 concept_total/concept_used。
func TestQuizGenerateBoundedPrompt(t *testing.T) {
	orig := quizMaxPromptConcepts
	t.Cleanup(func() { quizMaxPromptConcepts = orig })
	quizMaxPromptConcepts = 10
	concepts := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		concepts = append(concepts, fmt.Sprintf(`{"name":"概念%02d","label":"概念%02d","definition":"%s","synonyms":[],"parents":[]}`,
			i, i, strings.Repeat("def", 100)))
	}
	spec := `{"concepts":[` + strings.Join(concepts, ",") + `],"relations":[]}`
	s, st := quizTestServer(t)
	s.Ontology = ontology.NewService()
	s.Ontology.BuildURL = quizSpecStub(t, spec).URL
	if err := st.UpsertMastery("oqb", "概念00", false, 0.2); err != nil { // 薄弱定向必入集
		t.Fatal(err)
	}
	var capturedUser string
	origLLM := quizLLM
	t.Cleanup(func() { quizLLM = origLLM })
	quizLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		capturedUser = user
		return `[{"type":"short_answer","question":"概念01 的定义是什么?","answer":"def","concepts":["概念01"]}]`, nil
	}
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"count":3}`))
	req.SetPathValue("id", "oqb")
	rec := httptest.NewRecorder()
	s.quizGenerate(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items        []*store.QuizItem `json:"items"`
		ConceptTotal int               `json:"concept_total"`
		ConceptUsed  int               `json:"concept_used"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ConceptTotal != 40 || out.ConceptUsed != 10 {
		t.Fatalf("概念面应如实透出 40/10: %d/%d", out.ConceptTotal, out.ConceptUsed)
	}
	if n := strings.Count(capturedUser, `"name"`); n != 10 {
		t.Fatalf("提示词概念数应有界=10: %d", n)
	}
	if !strings.Contains(capturedUser, "概念00") {
		t.Fatal("薄弱定向概念必入出题集")
	}
	if strings.Contains(capturedUser, strings.Repeat("def", 100)) {
		t.Fatal("definition 应已截断不入全文")
	}
}
