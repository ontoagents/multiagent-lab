package chat

// REQ-393/M189：会话级上下文预算档合并序单测。
import "testing"

func TestEffectiveContextMode(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		agent      string
		conv       *string
		wantMode   string
		wantSource string
	}{
		{"", nil, "standard", "default"},                 // 全局默认
		{"compact", nil, "compact", "agent"},             // Agent 级
		{"compact", str("full"), "full", "conversation"}, // 会话覆盖
		{"compact", str(""), "compact", "agent"},         // 显式清除跟随 Agent
		{"", str("compact"), "compact", "conversation"},  // Agent 空时覆盖生效
		{"", str(""), "standard", "default"},             // 双空=全局默认
	}
	for _, c := range cases {
		mode, src := EffectiveContextMode(c.agent, c.conv)
		if mode != c.wantMode || src != c.wantSource {
			t.Errorf("EffectiveContextMode(%q,%v) = (%q,%q), want (%q,%q)", c.agent, c.conv, mode, src, c.wantMode, c.wantSource)
		}
	}
}
