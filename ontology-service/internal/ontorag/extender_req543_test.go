package ontorag

// REQ-543/M350 单测：扩展器被引词表注入面——模板槽位（空段整槽缺席不留占位符）+
// Generate 主循环透传（prompt 携带词表段且片段照常通过两阶段校验）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

const refVocabProbe = "已登记被引本体词表（external_refs 仓内引用——探针REQ543）：\n- 组织本体（orgs） v2〔import〕概念：部门、员工"

func TestRenderExtenderPromptRefVocab_Req543(t *testing.T) {
	with := RenderExtenderPrompt("哪些工单超期？", nil, ProfileStructure, refVocabProbe)
	if !strings.Contains(with, refVocabProbe) {
		t.Fatal("refVocab 非空时 prompt 应含被引词表段")
	}
	// 注入位：在目标元素块之后、能力问题之前
	if strings.Index(with, refVocabProbe) < strings.Index(with, "目标本体已有元素") || strings.Index(with, refVocabProbe) > strings.Index(with, "能力问题：") {
		t.Fatal("被引词表段应在目标元素块与能力问题之间")
	}
	without := RenderExtenderPrompt("哪些工单超期？", nil, ProfileStructure, "")
	if strings.Contains(without, "{refVocab}") || strings.Contains(without, "已登记被引本体词表") {
		t.Fatal("refVocab 空串时整槽缺席（无占位符残留）")
	}
}

// TestGenerateRefVocabWired_Req543：Generate 主循环把 refVocab 拼进 prompt（桩捕获），
// 且注入不影响两阶段校验——片段照常 ok。
func TestGenerateRefVocabWired_Req543(t *testing.T) {
	var mu sync.Mutex
	var prompt string
	frag := `{"name":"超期工单片段","concepts":[{"name":"超期工单","label":"超期工单","definition":"超过处理期限仍未闭环的工单"}],"relations":[],"data_properties":[],"axioms":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		prompt = req.Prompt
		mu.Unlock()
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(frag) + `,"usage":null}`))
	}))
	defer srv.Close()

	target := &pkgspec.Spec{Name: "工单本体", Concepts: []pkgspec.Concept{{Name: "工单", Label: "工单", Definition: "处理过程中的单据"}}}
	res := Generate(context.Background(), llmcreate.New(srv.URL), target, "哪些工单超期？", ProfileStructure, nil, refVocabProbe, nil)
	if res.Status != "ok" || res.Fragment == nil {
		t.Fatalf("片段应生成成功（注入不影响校验）: %s — %s", res.Status, res.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(prompt, refVocabProbe) {
		t.Fatal("Generate 的 prompt 应携带被引词表段")
	}
}
