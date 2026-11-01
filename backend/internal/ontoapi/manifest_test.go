package ontoapi

// REQ-561/M368 manifest 单测：分级判读（ro 直行/rw 审批/deny 拒绝）/未知命令 deny/透传兜底口径。
import "testing"

func TestManifestGrade_Req561(t *testing.T) {
	// ro 直行
	if e, a, ok := Grade("agent_list"); !ok || a != Ro || e.Method != "GET" {
		t.Fatalf("agent_list 应 ro: %+v %v %v", e, a, ok)
	}
	// rw 审批
	if _, a, ok := Grade("agent_create"); !ok || a != Rw {
		t.Fatalf("agent_create 应 rw: %v %v", a, ok)
	}
	if _, a, ok := Grade("kb_create"); !ok || a != Rw {
		t.Fatalf("kb_create 应 rw: %v %v", a, ok)
	}
	// deny：删除类
	if _, a, ok := Grade("conversation_delete"); !ok || a != Deny {
		t.Fatalf("conversation_delete 应 deny: %v %v", a, ok)
	}
	if _, a, ok := Grade("kb_delete"); !ok || a != Deny {
		t.Fatalf("kb_delete 应 deny: %v %v", a, ok)
	}
	// deny：密文/配置面排除
	if _, a, ok := Grade("settings_export"); !ok || a != Deny {
		t.Fatalf("settings_export 应 deny: %v %v", a, ok)
	}
	// 未知命令 → deny（透传兜底=未知端点默认拒绝）
	if _, a, ok := Grade("no_such_command"); ok || a != Deny {
		t.Fatalf("未知命令应 deny: %v %v", a, ok)
	}
	// 带参形态取首词
	if _, a, ok := Grade("agent_create {\"name\":\"x\"}"); !ok || a != Rw {
		t.Fatalf("带参命令应按首词判读: %v %v", a, ok)
	}
	// DenyReason 人话（验收「deny 不可达+错误人话」）
	if r := DenyReason("conversation_delete"); r == "" || !contains(r, "删除类") {
		t.Fatalf("deny 理由应含口径: %s", r)
	}
	if r := DenyReason("nope"); !contains(r, "未知命令") {
		t.Fatalf("未知命令理由: %s", r)
	}
	// manifest 覆盖面自检：至少 ro/rw/deny 三档齐备（分级三档冒烟的结构性前提）
	seen := map[Access]bool{}
	for _, e := range Manifest {
		seen[e.Access] = true
	}
	if !seen[Ro] || !seen[Rw] || !seen[Deny] {
		t.Fatalf("manifest 应三档齐备: %v", seen)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
