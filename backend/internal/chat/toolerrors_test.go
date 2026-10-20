package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// 工具容错包装（toolerrors.go）单测：执行失败回喂不炸 run；中断/取消原样透传；
// 流式调用错误与流中途错误同口径；与空参数归一化包装（toolargs.go）链序兼容。

type tolerIn struct {
	Q string `json:"q"`
}

func TestTolerantToolFeedsBackExecutionError(t *testing.T) {
	boom := errors.New("write file: permission denied")
	it, err := utils.InferTool("feed_tool", "d", func(context.Context, tolerIn) (string, error) {
		return "", boom
	})
	if err != nil {
		t.Fatal(err)
	}
	w, ok := tolerantTool(it).(einotool.InvokableTool)
	if !ok {
		t.Fatal("Invokable-only 工具包装后应保持 InvokableTool 断言")
	}
	if _, streamable := tolerantTool(it).(einotool.StreamableTool); streamable {
		t.Fatal("Invokable-only 包装不应虚增 StreamableTool")
	}
	out, err := w.InvokableRun(context.Background(), `{"q":"x"}`)
	if err != nil {
		t.Fatalf("执行失败应回喂而非上抛: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("回喂载荷应为 JSON: %v\n%s", err, out)
	}
	if p["error"] != "tool_failed" || p["tool"] != "feed_tool" {
		t.Fatalf("载荷字段不符: %s", out)
	}
	if !strings.Contains(p["detail"].(string), "permission denied") {
		t.Fatalf("detail 应携带原始错误: %s", out)
	}
	if !strings.Contains(p["hint"].(string), "修正参数") {
		t.Fatalf("hint 缺失: %s", out)
	}
}

func TestTolerantToolUnmarshalErrorHint(t *testing.T) {
	it, err := utils.InferTool("feed_tool2", "d", func(context.Context, tolerIn) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w := tolerantTool(it).(einotool.InvokableTool)
	out, err := w.InvokableRun(context.Background(), `{"q": "截断`) // 畸形 JSON
	if err != nil {
		t.Fatalf("参数解析失败应回喂而非上抛: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(out), &p)
	if !strings.Contains(p["hint"].(string), "max_tokens") {
		t.Fatalf("解析失败分支应给截断调参指引: %s", out)
	}
}

func TestTolerantToolPassthroughInterruptAndCancel(t *testing.T) {
	sigErr := &adk.InterruptSignal{ID: "sig-test"}
	it2, _ := utils.InferTool("feed_tool3", "d", func(context.Context, tolerIn) (string, error) {
		return "", sigErr
	})
	w := tolerantTool(it2).(einotool.InvokableTool)
	if _, err := w.InvokableRun(context.Background(), `{"q":"x"}`); !errors.Is(err, sigErr) {
		t.Fatalf("中断类错误应原样上抛，得到 %v", err)
	}
	it3, _ := utils.InferTool("feed_tool3", "d", func(context.Context, tolerIn) (string, error) {
		return "", context.Canceled
	})
	w = tolerantTool(it3).(einotool.InvokableTool)
	if _, err := w.InvokableRun(context.Background(), `{"q":"x"}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("用户取消应原样上抛，得到 %v", err)
	}
	// 成功路径零改动回归
	it4, _ := utils.InferTool("feed_tool3", "d", func(_ context.Context, in tolerIn) (string, error) {
		return "答:" + in.Q, nil
	})
	w = tolerantTool(it4).(einotool.InvokableTool)
	if out, err := w.InvokableRun(context.Background(), `{"q":"hi"}`); err != nil || out != "答:hi" {
		t.Fatalf("成功路径应原样透传: %q %v", out, err)
	}
}

func TestTolerantToolChainWithEmptyArgsNormalize(t *testing.T) {
	var gotArgs string
	it, _ := utils.InferTool("feed_tool4", "d", func(_ context.Context, in tolerIn) (string, error) {
		gotArgs = in.Q
		return "ok", nil
	})
	// 装配链序：normalize 内层（补 "{}"），tolerant 外层（兜错误）
	w := tolerantTool(normalizeEmptyArgsTool(it)).(einotool.InvokableTool)
	if _, err := w.InvokableRun(context.Background(), "  "); err != nil {
		t.Fatalf("空参数经链序应正常执行: %v", err)
	}
	if gotArgs != "" { // "{}" 解析为零值结构体
		t.Fatalf("空参数应归一化为零值: %q", gotArgs)
	}
}

func TestTolerantStreamToolCallErrorFeedsBack(t *testing.T) {
	st, err := utils.InferStreamTool[tolerIn, string]("feed_stream", "d",
		func(context.Context, tolerIn) (*schema.StreamReader[string], error) {
			return nil, errors.New("connect refused")
		})
	if err != nil {
		t.Fatal(err)
	}
	w, ok := tolerantTool(st).(einotool.StreamableTool)
	if !ok {
		t.Fatal("流式工具包装后应保持 StreamableTool 断言")
	}
	sr, err := w.StreamableRun(context.Background(), `{"q":"x"}`)
	if err != nil {
		t.Fatalf("流式调用失败应转回喂流: %v", err)
	}
	defer sr.Close()
	var sb strings.Builder
	for {
		chunk, rerr := sr.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("回喂流不应带错误: %v", rerr)
		}
		sb.WriteString(chunk)
	}
	if !strings.Contains(sb.String(), "tool_failed") || !strings.Contains(sb.String(), "feed_stream") {
		t.Fatalf("回喂流内容不符: %s", sb.String())
	}
}

func TestTolerantStreamToolMidStreamError(t *testing.T) {
	// 流中途普通错误：已收内容保留 + 尾注回喂，Recv 不见错误
	st, _ := utils.InferStreamTool[tolerIn, string]("feed_stream2", "d",
		func(_ context.Context, _ tolerIn) (*schema.StreamReader[string], error) {
			pr, pw := schema.Pipe[string](2)
			go func() {
				defer pw.Close()
				pw.Send("部分输出", nil)
				pw.Send("", errors.New("stream reset"))
			}()
			return pr, nil
		})
	sr, err := tolerantTool(st).(einotool.StreamableTool).StreamableRun(context.Background(), `{"q":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	var sb strings.Builder
	for {
		chunk, rerr := sr.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("流中途普通错误应转尾注而非上抛: %v", rerr)
		}
		sb.WriteString(chunk)
	}
	if !strings.Contains(sb.String(), "部分输出") || !strings.Contains(sb.String(), "输出流中断") {
		t.Fatalf("应保留已收内容并附尾注: %s", sb.String())
	}

	// 流中途中断类错误：原样穿透（Recv 收到同一错误）
	sigErr := &adk.InterruptSignal{ID: "sig-stream"}
	st2, _ := utils.InferStreamTool[tolerIn, string]("feed_stream3", "d",
		func(_ context.Context, _ tolerIn) (*schema.StreamReader[string], error) {
			pr, pw := schema.Pipe[string](2)
			go func() {
				defer pw.Close()
				pw.Send("", sigErr)
			}()
			return pr, nil
		})
	sr2, err := tolerantTool(st2).(einotool.StreamableTool).StreamableRun(context.Background(), `{"q":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer sr2.Close()
	_, rerr := sr2.Recv()
	if !errors.Is(rerr, sigErr) {
		t.Fatalf("流中途中断错误应原样穿透，得到 %v", rerr)
	}
}
