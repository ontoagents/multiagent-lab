// S3 真机协议回归：驱动**真实** dsh --profile sdk（PATH 探测到即跑，否则跳过）经
// sdkAdapter 全链——initialize 必填路由、session/prompt 入队回执、session.event 封套
// 转译、bash 工具真实执行、turn/end 结算、shutdown。模型轮经 DEEPSEEK_BASE_URL 指向
// 进程内 OpenAI 兼容桩（S0 同口径：协议事件与工具链真实，模型为脚本化桩）。
// 默认跳过；DSH_S3_LIVE=1 显式开启（dsh 缺席时恒跳过——Windows/无 dsh 环境零影响）。
package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// sdkLiveStub OpenAI 兼容桩：①无 tool 结果 → 调第一个 bash 类工具（echo 实证脚本）；
// ②bash 结果已回 → 终文 "sdk-live-ok"。支持流式 SSE 与非流式（dsh-llm-deepseek 实际形态）。
func sdkLiveStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		hasToolResult := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasToolResult = true
			}
		}
		var call *struct {
			Name string
			Args string
		}
		if !hasToolResult {
			for _, tl := range body.Tools {
				if strings.Contains(strings.ToLower(tl.Function.Name), "bash") {
					call = &struct {
						Name string
						Args string
					}{Name: tl.Function.Name, Args: `{"command":"echo sdk-live-bash-ok","description":"S3 live 验证用 echo"}`}
					break
				}
			}
		}
		finalText := "sdk-live-ok"
		if r.Header.Get("Accept") == "text/event-stream" || body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			chunk := func(delta map[string]any, finish any) {
				b, _ := json.Marshal(map[string]any{
					"id": "chatcmpl-live", "object": "chat.completion.chunk",
					"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
				})
				_, _ = w.Write(append([]byte("data: "), append(b, []byte("\n\n")...)...))
				flusher.Flush()
			}
			if call != nil {
				chunk(map[string]any{"role": "assistant"}, nil)
				chunk(map[string]any{"tool_calls": []map[string]any{{
					"index": 0, "id": "call_live_1", "type": "function",
					"function": map[string]any{"name": call.Name, "arguments": ""},
				}}}, nil)
				chunk(map[string]any{"tool_calls": []map[string]any{{
					"index": 0, "function": map[string]any{"arguments": call.Args},
				}}}, nil)
				chunk(map[string]any{}, "tool_calls")
			} else {
				for i := 0; i < len(finalText); i += 4 {
					end := i + 4
					if end > len(finalText) {
						end = len(finalText)
					}
					chunk(map[string]any{"content": finalText[i:end]}, nil)
				}
				chunk(map[string]any{}, "stop")
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		msg := map[string]any{"role": "assistant", "content": finalText}
		finish := "stop"
		if call != nil {
			msg = map[string]any{"role": "assistant", "content": nil, "tool_calls": []map[string]any{{
				"id": "call_live_1", "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": call.Args},
			}}}
			finish = "tool_calls"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-live", "object": "chat.completion",
			"choices": []map[string]any{{"index": 0, "message": msg, "finish_reason": finish}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSDKAdapterLiveRealDSH(t *testing.T) {
	if os.Getenv("DSH_S3_LIVE") != "1" {
		t.Skip("DSH_S3_LIVE=1 显式开启真机协议回归（真实 dsh --profile sdk + 桩 LLM）")
	}
	if _, err := exec.LookPath("dsh"); err != nil {
		t.Skip("PATH 无 dsh，跳过真机回归")
	}
	srv := sdkLiveStub(t)
	t.Setenv("DEEPSEEK_BASE_URL", srv.URL)
	t.Setenv("DEEPSEEK_API_KEY", "stub")

	a := newSDKAdapter()
	if pr := a.Probe(context.Background()); !pr.Available {
		t.Fatalf("probe: %+v", pr)
	}
	var (
		evs       []Event
		text      strings.Builder
		audits    = map[string]int{}
		toolName  string
		toolHasOK bool
	)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	err := a.Run(ctx, &RunRequest{
		Prompt:    "执行一次 bash echo 命令回显 sdk-live-bash-ok，然后简短总结。",
		SessionID: "conv-live-s3",
		Cwd:       "/tmp",
	}, func(e Event) {
		evs = append(evs, e)
		switch e.Type {
		case "message.delta":
			if d, ok := e.Data["delta"].(string); ok {
				text.WriteString(d)
			}
		case "tool.call":
			if n, _ := e.Data["name"].(string); strings.Contains(strings.ToLower(n), "bash") {
				toolName = n
			}
		case "tool.result":
			if c, _ := e.Data["content"].(string); strings.Contains(c, "sdk-live-bash-ok") {
				toolHasOK = true
			}
		case "external.audit":
			if s, _ := e.Data["event"].(string); s != "" {
				audits[s]++
			}
		}
	})
	if err != nil {
		t.Fatalf("live run: %v", err)
	}
	if toolName == "" || !toolHasOK {
		t.Fatalf("bash 工具未真实执行：name=%q result-ok=%v", toolName, toolHasOK)
	}
	if !strings.Contains(text.String(), "sdk-live-ok") {
		t.Fatalf("终文未回流: %q", text.String())
	}
	for _, want := range []string{"turn/start", "turn/end", "step/start", "request/header"} {
		if audits[want] == 0 {
			t.Fatalf("审计事件缺 %s：实得 %v", want, audits)
		}
	}
	t.Logf("live 审计事件分布: %v（message.delta=%q）", audits, text.String())
}
