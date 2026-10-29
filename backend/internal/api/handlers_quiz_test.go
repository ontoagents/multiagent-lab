package api

// REQ-524/M331 handler 级单测：LLM 桩注入（quizLLM 字段）——出题概念校验+批改后掌握度真链
// 落表（冒烟 mock submit 旁路端点，掌握度真链由本测承载）。

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/ontology"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
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
