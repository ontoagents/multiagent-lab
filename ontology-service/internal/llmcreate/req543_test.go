package llmcreate

// REQ-543/M350 单测：仓内词表上下文注入面——buildPrompt 注入位（prompt 单源）+
// DraftWithVocab 透传（生成-校验循环全轮次携带；DraftWithCQ 旧入口零注入零破坏）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const vocabProbe = "仓内已有本体（只读参考——词表上下文探针REQ543）：\n1. 缺陷管理（defects） v3 · 概念：缺陷、模块"

func TestBuildPromptVocabSlot_Req543(t *testing.T) {
	with := buildPrompt("缺陷与模块的领域", "", nil, nil, vocabProbe)
	if !strings.Contains(with, vocabProbe) {
		t.Fatal("vocab 非空时 prompt 应含注入段")
	}
	// 注入位：词表段在领域描述之前（参考材料先于任务输入）
	if strings.Index(with, vocabProbe) > strings.Index(with, "领域描述：") {
		t.Fatal("词表段应在领域描述之前（参考材料先于任务输入）")
	}
	without := buildPrompt("缺陷与模块的领域", "", nil, nil, "")
	if strings.Contains(without, "仓内已有本体") || strings.Contains(without, vocabProbe) {
		t.Fatal("vocab 空串时整段缺席（无残留）")
	}
}

// TestDraftWithVocabThreaded_Req543：生成-校验循环每一轮 prompt 均携带词表段（回喂重建走
// buildPrompt 同一注入位）；DraftWithCQ 旧入口不注入（零破坏）。桩恒回非法 JSON——
// 循环走满 MaxRounds（3 轮）后带错误返回，捕获全部请求体断言。
func TestDraftWithVocabThreaded_Req543(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		prompts = append(prompts, req.Prompt)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"draft_json":"not-json-at-all","usage":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.DraftWithVocab(context.Background(), "缺陷与模块的领域", "", nil, vocabProbe); err == nil {
		t.Fatal("恒非法输出应带错误返回")
	}
	mu.Lock()
	n := len(prompts)
	snap := append([]string(nil), prompts...)
	mu.Unlock()
	if n != c.MaxRounds {
		t.Fatalf("应走满 %d 轮，实际捕获 %d 个请求", c.MaxRounds, n)
	}
	for i, p := range snap {
		if !strings.Contains(p, vocabProbe) {
			t.Fatalf("第 %d 轮 prompt 应携带词表段（回喂重建同一注入位）", i+1)
		}
	}

	// 旧入口 DraftWithCQ：同桩重跑，prompt 不含词表段（零注入零破坏）
	mu.Lock()
	prompts = nil
	mu.Unlock()
	if _, err := c.DraftWithCQ(context.Background(), "缺陷与模块的领域", "", nil); err == nil {
		t.Fatal("桩恒非法输出应带错误返回")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range prompts {
		if strings.Contains(p, vocabProbe) {
			t.Fatal("DraftWithCQ 旧入口不应注入词表段")
		}
	}
}
