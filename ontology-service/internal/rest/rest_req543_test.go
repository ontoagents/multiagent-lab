package rest

// REQ-543/M350 接线单测：AI 创建（aiDraft）prompt 注入仓内清单+可复用词表提示——
// rest→vocabctx.RepoList→llmcreate.buildPrompt 全链贯通（桩捕获平台请求体断言）；
// OntoChat VocabProvider 装配面（Engine 不感知仓库的函数注入缝）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func TestAiDraftInjectsRepoVocab_Req543(t *testing.T) {
	st, err := repo.Open(filepath.Join(t.TempDir(), "req543.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOntology("defects", "缺陷管理", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("defects", "spec_json", `{"concepts":[{"name":"缺陷","label":"缺陷单"},{"name":"模块"}]}`, true); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var prompt string
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		prompt = req.Prompt
		mu.Unlock()
		_, _ = w.Write([]byte(`{"draft_json":"not-json","usage":null}`)) // 恒非法→循环走满，捕获首轮 prompt
	}))
	t.Cleanup(llmSrv.Close)

	s := New(st, nil, llmcreate.New(llmSrv.URL))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/ai-draft", strings.NewReader(`{"description":"医院药物管理"}`))
	req.Header.Set("Content-Type", "application/json")
	s.aiDraft(w, req)

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"仓内已有本体", "缺陷管理（defects） v1", "缺陷单、缺陷、模块", "不强制"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("AI 创建 prompt 应含 %q（rest→vocabctx→llmcreate 全链），得:\n%s", want, prompt)
		}
	}
}

// TestOntoChatVocabProviderWired_Req543：rest.New 装配 Engine.VocabProvider=RepoList——
// 调用即得仓内清单（Engine 自身不感知仓库，函数注入缝 D-O26）。
func TestOntoChatVocabProviderWired_Req543(t *testing.T) {
	st, err := repo.Open(filepath.Join(t.TempDir(), "req543b.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if s := New(st, nil, nil); s.OntoChat.VocabProvider == nil {
		t.Fatal("rest.New 应装配 OntoChat.VocabProvider")
	}
	if _, err := st.CreateOntology("orgs", "组织本体", ""); err != nil {
		t.Fatal(err)
	}
	s := New(st, nil, nil)
	got := s.OntoChat.VocabProvider()
	if !strings.Contains(got, "组织本体（orgs） v1") {
		t.Fatalf("VocabProvider() 应返回仓内清单:\n%s", got)
	}
}
