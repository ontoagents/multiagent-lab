package spec

// REQ-249/G3 单测：跨域查重（概念↔实例同名=错误）与 CQ 字段往返（仅 spec 层）。

import (
	"encoding/json"
	"testing"
)

func TestValidateCrossDomainNames(t *testing.T) {
	s := &Spec{
		Name:      "t",
		Concepts:  []Concept{{Name: "服务器"}},
		Instances: []Instance{{Name: "服务器", Concept: "服务器"}},
	}
	errs := s.Validate()
	found := false
	for _, e := range errs {
		if contains(e.Message, "概念与实例同名") {
			found = true
		}
	}
	if !found {
		t.Fatalf("概念与实例同名应报错，得到 %v", errs)
	}
	// 改名后通过
	s.Instances[0].Name = "服务器A"
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("改名后应通过，得到 %v", errs)
	}
}

func TestCQRoundTrip(t *testing.T) {
	s := Spec{Name: "t", CQ: []string{"Q1", "Q2"}, Concepts: []Concept{}, Relations: []Relation{}, Instances: []Instance{}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(b), `"cq"`) {
		t.Fatalf("cq 应序列化: %s", b)
	}
	var back Spec
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.CQ) != 2 {
		t.Fatalf("cq 往返失真: %v", back.CQ)
	}
	// 空 cq 不序列化（omitempty）
	s2 := Spec{Name: "t2"}
	b2, _ := json.Marshal(s2)
	if contains(string(b2), `"cq"`) {
		t.Fatalf("空 cq 不应出现在 JSON: %s", b2)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
