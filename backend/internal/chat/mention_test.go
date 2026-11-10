package chat

import (
	"context"
	"reflect"
	"testing"
)

// REQ-622/M432 单测：@ 点名解析（文本兜底路径）与 ctx 注入往返。
func TestParseMentionNames(t *testing.T) {
	nameToID := map[string]string{"翻译": "a1", "翻译官": "a2", "审查者": "a3"}
	extNames := map[string]bool{"外部审查": true}

	cases := []struct {
		name  string
		input string
		want  []MentionTarget
	}{
		{"无@快速返回", "帮我总结这份文档", nil},
		{"单成员", "@翻译 这段话什么意思", []MentionTarget{{AgentID: "a1"}}},
		{"最长名优先", "@翻译官 和 @翻译 各翻一遍", []MentionTarget{{AgentID: "a2"}, {AgentID: "a1"}}},
		{"多@全部命中按提及序", "@审查者 先审 @翻译官 再翻", []MentionTarget{{AgentID: "a3"}, {AgentID: "a2"}}},
		{"重复去重保首序", "@翻译 @翻译 再来一遍", []MentionTarget{{AgentID: "a1"}}},
		{"外部成员命中", "@外部审查 检查一下", []MentionTarget{{ExtName: "外部审查"}}},
		{"未知名忽略", "@不存在 你在吗", nil},
		{"邮箱内@不误报", "联系 user@demo.com", nil},
		{"词中@需前缀边界后仍按前缀命中", "x@翻译 你好", []MentionTarget{{AgentID: "a1"}}},
	}
	for _, c := range cases {
		got := ParseMentionNames(c.input, nameToID, extNames)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// 空成员表防炸
	if got := ParseMentionNames("@任何人", nil, nil); got != nil {
		t.Errorf("空成员表应返回 nil，got %v", got)
	}
}

func TestMentionTargetsContext(t *testing.T) {
	ctx := context.Background()
	if got := MentionTargetsFromCtx(ctx); got != nil {
		t.Errorf("未注入应返回 nil，got %v", got)
	}
	ts := []MentionTarget{{AgentID: "a1"}, {ExtName: "外部审查"}}
	ctx2 := WithMentionTargets(ctx, ts)
	if got := MentionTargetsFromCtx(ctx2); !reflect.DeepEqual(got, ts) {
		t.Errorf("往返不一致：got %v, want %v", got, ts)
	}
	if WithMentionTargets(ctx, nil) != ctx {
		t.Error("空目标应原样返回 ctx")
	}
}
