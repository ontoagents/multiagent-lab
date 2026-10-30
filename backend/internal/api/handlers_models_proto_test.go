package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// REQ-172：Anthropic 探测请求形态（路径/头/体）与响应解析——httptest 桩。
func TestProbeAnthropicChat(t *testing.T) {
	var gotPath, gotKey, gotVer, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "role": "assistant",
			"content": []map[string]any{{"type": "text", "text": "ok"}},
		})
	}))
	defer srv.Close()

	// 用户误填 OpenAI 习惯的 /v1 后缀也须归一到 /v1/messages
	if msg := probeAnthropicChat(context.Background(), srv.URL+"/v1", "claude-sonnet-4-5", "sk-ant-test"); msg != "" {
		t.Fatalf("probeAnthropicChat 返回错误: %s", msg)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("请求路径 = %q, want /v1/messages", gotPath)
	}
	if gotKey != "sk-ant-test" {
		t.Errorf("x-api-key = %q", gotKey)
	}
	if gotVer == "" {
		t.Error("缺少 anthropic-version 头")
	}
	if gotAuth != "" {
		t.Error("anthropic 探测不应携带 Authorization: Bearer 头")
	}
	if gotBody["max_tokens"].(float64) != 16 {
		t.Errorf("max_tokens = %v, want 16（Anthropic 必填）", gotBody["max_tokens"])
	}
	if gotBody["model"] != "claude-sonnet-4-5" {
		t.Errorf("model = %v", gotBody["model"])
	}
}

func TestProbeAnthropicChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]any{"type": "authentication_error", "message": "invalid x-api-key"}})
	}))
	defer srv.Close()

	msg := probeAnthropicChat(context.Background(), srv.URL, "claude-sonnet-4-5", "bad-key")
	if msg == "" {
		t.Fatal("401 应返回错误信息")
	}
	want := "HTTP 401: invalid x-api-key"
	if msg != want {
		t.Errorf("err = %q, want %q", msg, want)
	}
}

func TestListAnthropicModels(t *testing.T) {
	var gotPath, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"type": "model", "id": "claude-sonnet-4-5"},
				{"type": "model", "id": "claude-haiku-4-5"},
				{"type": "model", "id": "claude-sonnet-4-5"}, // 重复项应去重
			},
			"has_more": false,
		})
	}))
	defer srv.Close()

	models, err := listAnthropicModels(context.Background(), srv.URL, "sk-ant-test")
	if err != nil {
		t.Fatalf("listAnthropicModels: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Errorf("请求路径 = %q, want /v1/models", gotPath)
	}
	if gotKey != "sk-ant-test" {
		t.Errorf("x-api-key = %q", gotKey)
	}
	if len(models) != 2 {
		t.Errorf("模型数 = %d, want 2（去重后）: %v", len(models), models)
	}
}

func TestValidateConnProtocol(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		connType string
		wantErr  bool
	}{
		{"空协议默认放行", "", "chat", false},
		{"openai_compat chat", "openai_compat", "chat", false},
		{"openai_compat embedding", "openai_compat", "embedding", false},
		{"anthropic chat", "anthropic", "chat", false},
		{"anthropic embedding 拒绝", "anthropic", "embedding", true},
		{"未知协议拒绝", "grpc", "chat", true},
	}
	for _, c := range cases {
		conn := &store.ModelConnection{Protocol: c.protocol, ConnType: c.connType}
		if got := validateConnProtocol(conn); (got != "") != c.wantErr {
			t.Errorf("%s: validateConnProtocol = %q, wantErr=%v", c.name, got, c.wantErr)
		}
	}
}
