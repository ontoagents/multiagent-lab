package chat

// REQ-452/M248：模型调用重试与故障转移单测。
// 覆盖：幂等白名单 429/rate limit 特征；ShouldRetry 决策（白名单/非白名单/部分增量/主动取消）；
// 故障转移按序切换与事件留痕（真 store 夹具，buildChatModel 仅构造客户端零网络）。

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func TestIsIdempotentRetryable_Req452(t *testing.T) {
	yes := []string{
		"Error 429, rate limit exceeded",
		"error, status code: 429",
		"Too Many Requests",
		"context deadline exceeded",
		"connection reset by peer",
	}
	for _, s := range yes {
		if !IsIdempotentRetryable(errors.New(s)) {
			t.Fatalf("%q 应命中幂等重试白名单", s)
		}
	}
	no := []string{"invalid api key", "model not found", "bad request", ""}
	for _, s := range no {
		msg := s
		if msg == "" {
			if IsIdempotentRetryable(nil) {
				t.Fatal("nil 错误不应重试")
			}
			continue
		}
		if IsIdempotentRetryable(errors.New(msg)) {
			t.Fatalf("%q 不应命中幂等重试白名单", msg)
		}
	}
}

func TestMessageHasEmittedContent_Req452(t *testing.T) {
	if messageHasEmittedContent(nil) {
		t.Fatal("nil 消息不应视为已产出")
	}
	if messageHasEmittedContent(&schema.Message{}) {
		t.Fatal("空消息不应视为已产出")
	}
	for name, m := range map[string]*schema.Message{
		"text":      {Content: "部分回答"},
		"reasoning": {ReasoningContent: "思考中"},
		"toolcall":  {ToolCalls: []schema.ToolCall{{ID: "t1"}}},
	} {
		if !messageHasEmittedContent(m) {
			t.Fatalf("%s 消息应视为已产出", name)
		}
	}
}

func TestModelRetryConfig_ShouldRetry_Req452(t *testing.T) {
	rc := modelRetryConfig()
	if rc.MaxRetries != 2 {
		t.Fatalf("默认重试次数应为 2，得到 %d", rc.MaxRetries)
	}
	cases := []struct {
		name   string
		err    error
		output *schema.Message
		want   bool
	}{
		{"限流错误可重试", errors.New("status code: 429"), nil, true},
		{"非白名单不重试", errors.New("invalid api key"), nil, false},
		{"流正常结束不重试", nil, &schema.Message{Content: "ok"}, false},
		{"已产出增量不重试", errors.New("connection reset"), &schema.Message{Content: "部分"}, false},
		{"零增量流错误可重试", errors.New("connection reset"), nil, true},
	}
	for _, c := range cases {
		ctx := context.Background()
		got := rc.ShouldRetry(ctx, &adk.RetryContext{Err: c.err, OutputMessage: c.output})
		if (got != nil && got.Retry) != c.want {
			t.Fatalf("%s: ShouldRetry=%v want %v", c.name, got != nil && got.Retry, c.want)
		}
	}
	// 主动停止：ctx 取消后白名单错误也不重试
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := rc.ShouldRetry(ctx, &adk.RetryContext{Err: errors.New("connection reset")}); got != nil && got.Retry {
		t.Fatal("ctx 已取消不应重试")
	}
}

func TestBuildModelFailover_Req452(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	mkConn := func(name, model string) *store.ModelConnection {
		c, err := st.CreateConnection(&store.ModelConnection{
			Name: name, Protocol: "openai_compat", BaseURL: "http://127.0.0.1:9", ModelName: model, ConnType: "chat", Enabled: true,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	primary := mkConn("主连接", "m0")
	b1 := mkConn("备援一", "m1")
	b2 := mkConn("备援二", "m2")
	bOff := mkConn("停用备援", "m3")
	bOffOff := *bOff
	bOffOff.Enabled = false
	if _, err := st.UpdateConnection(&bOffOff, nil); err != nil {
		t.Fatal(err)
	}

	a := &Assembler{Store: st, Box: box}
	ag := &store.Agent{Name: "fa", ModelConnID: &primary.ID, ModelFailoverConnIDs: []string{b1.ID, b2.ID}}

	// 空清单返回 nil（零回归）
	if got := a.buildModelFailover(context.Background(), &store.Agent{Name: "x"}); got != nil {
		t.Fatal("无备援清单应返回 nil")
	}

	fc := a.buildModelFailover(context.Background(), ag)
	if fc == nil {
		t.Fatal("有备援清单应返回配置")
	}
	if fc.MaxRetries != 2 {
		t.Fatalf("MaxRetries 应为备援数 2，得到 %d", fc.MaxRetries)
	}

	// ShouldFailover：错误才切；主动取消不切；已产出增量不切
	ctx := context.Background()
	if fc.ShouldFailover(ctx, nil, nil) {
		t.Fatal("无错误不应切换")
	}
	if fc.ShouldFailover(ctx, nil, errors.New("boom")) != true {
		t.Fatal("错误且零增量应切换")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if fc.ShouldFailover(cctx, nil, errors.New("boom")) {
		t.Fatal("ctx 取消不应切换")
	}
	if fc.ShouldFailover(ctx, &schema.Message{Content: "部分"}, errors.New("boom")) {
		t.Fatal("已产出增量不应切换")
	}

	// GetFailoverModel：按序切换 + run.warning 事件留痕
	var warns []string
	fctx := tool.WithEventSink(ctx, func(eventType string, data map[string]any) {
		if eventType == "run.warning" {
			warns = append(warns, data["message"].(string))
		}
	})
	in := []*schema.Message{{Role: schema.User, Content: "hi"}}
	m1, _, err := fc.GetFailoverModel(fctx, &adk.FailoverContext[*schema.Message]{FailoverAttempt: 1, InputMessages: in, LastErr: errors.New("主连接持续失败")})
	if err != nil {
		t.Fatalf("第一次切换失败: %v", err)
	}
	if m1 == nil {
		t.Fatal("应返回备援一模型")
	}
	m2, _, err := fc.GetFailoverModel(fctx, &adk.FailoverContext[*schema.Message]{FailoverAttempt: 2, InputMessages: in})
	if err != nil || m2 == nil {
		t.Fatalf("第二次切换失败: %v", err)
	}
	if _, _, err := fc.GetFailoverModel(fctx, &adk.FailoverContext[*schema.Message]{FailoverAttempt: 3, InputMessages: in}); err == nil {
		t.Fatal("超出备援数应报错终止")
	}
	if len(warns) != 2 {
		t.Fatalf("应留痕 2 次 run.warning，得到 %d: %v", len(warns), warns)
	}

	// 停用的备援连接：如实报错终止（不静默跳过）
	agOff := &store.Agent{Name: "fa2", ModelFailoverConnIDs: []string{bOff.ID}}
	fcOff := a.buildModelFailover(context.Background(), agOff)
	if fcOff == nil {
		t.Fatal("停用备援也应返回配置（失败面在切换时点）")
	}
	if _, _, err := fcOff.GetFailoverModel(fctx, &adk.FailoverContext[*schema.Message]{FailoverAttempt: 1, InputMessages: in}); err == nil {
		t.Fatal("停用备援连接应报错")
	}
}
