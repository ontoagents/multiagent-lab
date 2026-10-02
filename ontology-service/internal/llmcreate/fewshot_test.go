package llmcreate

// REQ-247/G4 单测：few-shot 种子挑选（关键词命中/未命中空串）与范例段形态（概念截断防膨胀）。

import (
	"strings"
	"testing"
)

func TestPickSeed(t *testing.T) {
	cases := []struct {
		desc string
		want string
	}{
		{"做一个医学常识本体，覆盖疾病与症状", "med_common.json"},
		{"软件项目缺陷管理领域", "defects.json"},
		{"Kubernetes 集群运维，容器与部署", "onto_k8s_ops.json"},
		{"完全无关的领域描述xyz", ""},
	}
	for _, c := range cases {
		if got := pickSeed(c.desc); got != c.want {
			t.Errorf("pickSeed(%q) = %q, want %q", c.desc, got, c.want)
		}
	}
}

func TestBuildFewShot(t *testing.T) {
	seg := buildFewShot("医学常识本体，疾病/症状/药物关系网")
	if seg == "" {
		t.Fatal("命中种子时应产出范例段")
	}
	if !strings.Contains(seg, "参考范例") || !strings.Contains(seg, "concepts") {
		t.Fatalf("范例段形态不符: %.200s", seg)
	}
	// 截断：概念 ≤4
	if got := strings.Count(seg, `"name"`); got > 10 {
		t.Fatalf("范例段应截断防膨胀，name 出现 %d 次", got)
	}
	if buildFewShot("完全无关xyz") != "" {
		t.Fatal("未命中种子应返回空串")
	}
}
