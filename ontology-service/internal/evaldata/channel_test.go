package evaldata

// REQ-257 渠道派生规则单测（纯函数）。

import "testing"

func TestDeriveChannel(t *testing.T) {
	cases := []struct {
		name        string
		id, forked  string
		hasOriginal bool
		want        string
	}{
		{"种子前缀", "onto_seed_defects", "", false, "seed"},
		{"内置三份", "onto_k8s_ops", "", false, "seed"},
		{"fork", "onto_x", "onto_k8s_ops", false, "fork"},
		{"导入（有 original）", "onto_imported", "", true, "import"},
		{"手工/AI", "onto_manual", "", false, "custom"},
		{"fork 优先于 import", "onto_x", "onto_a", true, "fork"},
	}
	for _, c := range cases {
		if got := DeriveChannel(c.id, c.forked, c.hasOriginal); got != c.want {
			t.Fatalf("%s: 期望 %s，实际 %s", c.name, c.want, got)
		}
	}
}
