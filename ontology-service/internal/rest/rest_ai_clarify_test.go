package rest

// REQ-306 A1 ai-clarify 澄清轮单测：description 校验 + LLM 桩 200 回显。

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

func TestAiClarify(t *testing.T) {
	// description 必填
	s := newImportGuardServer(t)
	w := httptest.NewRecorder()
	s.aiClarify(w, importJSONReq(""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空 description 期望 400，得 %d", w.Code)
	}

	// LLM 桩：RawChat 代理响应 → reply 透传
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(`{"reply":"理解范围：医院药物管理。概念覆盖：缺——主要实体类型未列出。可以生成。"}`) + `,"usage":null}`))
	}))
	t.Cleanup(llmSrv.Close)
	s2 := New(s.Store, nil, llmcreate.New(llmSrv.URL))
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/ai-clarify", strings.NewReader(`{"description":"医院药物管理","hints":["药物有处方与非处方两类"]}`))
	req.Header.Set("Content-Type", "application/json")
	s2.aiClarify(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("期望 200，得 %d body=%s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "reply") {
		t.Fatalf("响应应含 reply: %s", w2.Body.String())
	}
}
