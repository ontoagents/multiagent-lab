package rest

// REQ-621/M429 SPARQL 代理超时可配置与超时错误人话化单测。零外部依赖。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSparqlTimeoutFromEnv(t *testing.T) {
	t.Setenv("SPARQL_PROXY_TIMEOUT", "240")
	if got := sparqlTimeoutFromEnv(); got != 240*time.Second {
		t.Fatalf("env 覆盖未生效: %v", got)
	}
	t.Setenv("SPARQL_PROXY_TIMEOUT", "abc")
	if got := sparqlTimeoutFromEnv(); got != 120*time.Second {
		t.Fatalf("非法值应回落默认 120s: %v", got)
	}
	t.Setenv("SPARQL_PROXY_TIMEOUT", "0")
	if got := sparqlTimeoutFromEnv(); got != 120*time.Second {
		t.Fatalf("非正数应回落默认 120s: %v", got)
	}
	t.Setenv("SPARQL_PROXY_TIMEOUT", "")
	if got := sparqlTimeoutFromEnv(); got != 120*time.Second {
		t.Fatalf("缺省应 120s: %v", got)
	}
}

func TestHumanizeSparqlErr(t *testing.T) {
	// ①裸 deadline：附人话提示（超时时长+处置建议+env 指引）
	got := humanizeSparqlErr(context.DeadlineExceeded, 120*time.Second)
	if got == context.DeadlineExceeded.Error() {
		t.Fatal("超时错误应附人话提示")
	}
	for _, want := range []string{"2m0s", "SPARQL_PROXY_TIMEOUT", "无推理基线"} {
		if !strings.Contains(got, want) {
			t.Fatalf("提示缺「%s」: %s", want, got)
		}
	}
	// ②引擎错误串包裹（复刻用户报障形态）——errors.Is 穿透 wrap 命中
	wrapped := fmt.Errorf("Post \"http://127.0.0.1:9201/ds/\": %w", context.DeadlineExceeded)
	got2 := humanizeSparqlErr(wrapped, 120*time.Second)
	if !strings.Contains(got2, "Post \"http://127.0.0.1:9201/ds/\"") || !strings.Contains(got2, "引擎在 2m0s") {
		t.Fatalf("wrapped 超时应保留原始串并附提示: %s", got2)
	}
	// ③Client.Timeout 形态同样命中
	clientTimeout := errors.New(`Post "http://x": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`)
	if !strings.Contains(humanizeSparqlErr(clientTimeout, 120*time.Second), "SPARQL_PROXY_TIMEOUT") {
		t.Fatal("Client.Timeout 形态应命中人话提示")
	}
	// ④非超时错误原样透传
	plain := errors.New("connect: connection refused")
	if humanizeSparqlErr(plain, 120*time.Second) != plain.Error() {
		t.Fatal("非超时错误应原样透传")
	}
}
