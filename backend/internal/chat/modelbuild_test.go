package chat

import (
	"context"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/claude"

	"github.com/ontoagents/ontoagents/backend/internal/modelproto"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// buildChatModel 协议分支（REQ-172）：构造期不触网，验证两通道均可离线实例化且类型正确。
func TestBuildChatModelByProtocol(t *testing.T) {
	ctx := context.Background()
	temp := 1.7 // 故意超 Anthropic 温度域，验证钳制路径不报错

	anthropicConn := &store.ModelConnection{Protocol: modelproto.Anthropic, BaseURL: "https://api.anthropic.com/v1/", ModelName: "claude-sonnet-4-5"}
	cm, err := buildChatModel(ctx, anthropicConn, "sk-test", &temp, nil)
	if err != nil {
		t.Fatalf("anthropic 分支构造失败: %v", err)
	}
	if _, ok := cm.(*claude.ChatModel); !ok {
		t.Errorf("anthropic 分支应返回 *claude.ChatModel，得到 %T", cm)
	}

	openaiConn := &store.ModelConnection{Protocol: modelproto.OpenAICompat, BaseURL: "https://api.deepseek.com/v1", ModelName: "deepseek-chat"}
	cm, err = buildChatModel(ctx, openaiConn, "sk-test", nil, nil)
	if err != nil {
		t.Fatalf("openai_compat 分支构造失败: %v", err)
	}
	if cm == nil {
		t.Error("openai_compat 分支应返回非 nil 模型")
	}

	legacyConn := &store.ModelConnection{Protocol: "", BaseURL: "https://api.deepseek.com/v1", ModelName: "deepseek-chat"}
	if cm, err = buildChatModel(ctx, legacyConn, "", nil, nil); err != nil || cm == nil {
		t.Errorf("空协议应走 openai 兼容默认分支，err=%v", err)
	}
}
