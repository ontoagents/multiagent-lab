package llmcreate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// REQ-271 定案③：超时默认 300s + ONTOCHAT_LLM_TIMEOUT 可配。
func TestTimeoutConfig(t *testing.T) {
	t.Setenv("ONTOCHAT_LLM_TIMEOUT", "")
	c := New("http://127.0.0.1:1")
	if c.HTTP.Timeout != 300*time.Second {
		t.Fatalf("默认超时应为 300s（原 120s 已废）: %v", c.HTTP.Timeout)
	}
	t.Setenv("ONTOCHAT_LLM_TIMEOUT", "7")
	c2 := New("http://127.0.0.1:1")
	if c2.HTTP.Timeout != 7*time.Second {
		t.Fatalf("env 覆盖失效: %v", c2.HTTP.Timeout)
	}
	t.Setenv("ONTOCHAT_LLM_TIMEOUT", "bogus")
	c3 := New("http://127.0.0.1:1")
	if c3.HTTP.Timeout != 300*time.Second {
		t.Fatalf("非法 env 应回落默认: %v", c3.HTTP.Timeout)
	}
}

// REQ-271⑤：few-shot 种子范例接线——命中种子的领域描述其生成 prompt 含范例段。
func TestBuildPromptFewShotWired(t *testing.T) {
	hit := buildPrompt("医学常识本体：症状、疾病与药物的对照关系", "", nil, nil)
	if !strings.Contains(hit, FewShotHeaderPrompt) {
		t.Fatal("命中种子的领域描述应注入 few-shot 范例段（M70 死代码转正）")
	}
	if !strings.Contains(hit, "concepts") {
		t.Fatal("范例段应含种子概念 JSON")
	}
	miss := buildPrompt("陶瓷茶具的手工拉坯工艺记录", "", nil, nil)
	if strings.Contains(miss, FewShotHeaderPrompt) {
		t.Fatal("无命中种子时不应注入范例段")
	}
}

// REQ-271：ctx 贯穿——上游挂起时取消 context，调用即时返回错误（异步 job 取消依赖）。
func TestCallGenerateContextCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":"{}"}`))
	}))
	defer srv.Close()
	defer close(block)

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, _, err := c.callGenerate(ctx, "测试"); err == nil {
		t.Fatal("ctx 取消后应返回错误")
	} else if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("取消未即时生效（%v）", elapsed)
	}
}

// 进度回调：模拟平台返回坏 JSON → 解析失败回喂应触发进度通知。
func TestDraftProgressCallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":"不是json"}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.MaxRounds = 1
	var notes []string
	_, err := c.Draft(context.Background(), "测试域", "", func(round int, msg string) {
		notes = append(notes, msg)
	})
	if err == nil {
		t.Fatal("坏 JSON 一轮后应报错")
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "第 1/1 轮") {
		t.Fatalf("进度回调未收到生成阶段通知: %v", notes)
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "回喂重试") || strings.Contains(n, "不是合法") {
			found = true
		}
	}
	if !found {
		t.Fatalf("解析失败阶段未通知: %v", notes)
	}
	_ = json.Marshal
}
