package facade

// REQ-545/M352 分域零命中清单纯函数单测——三域独立判定/命中不计入/单字符词不参与。

import (
	"reflect"
	"testing"
)

func TestMissingByDomain_Req545(t *testing.T) {
	labels := map[string][]string{
		"concepts":  {"计算节点", "存储节点"},
		"relations": {"连接"},
		"synonyms":  {"服务器"},
	}
	words := []string{"计算", "网络", "连接", "服务器", "a"}
	got := missingByDomain(words, labels)
	// concepts：计算 命中（计算节点⊃）；网络/连接/服务器 零命中
	if !reflect.DeepEqual(got["concepts"], []string{"网络", "连接", "服务器"}) {
		t.Fatalf("concepts 域失真: %v", got["concepts"])
	}
	// relations：连接 命中；网络/服务器/计算 零命中
	if !reflect.DeepEqual(got["relations"], []string{"计算", "网络", "服务器"}) {
		t.Fatalf("relations 域失真: %v", got["relations"])
	}
	// synonyms：服务器 命中
	if !reflect.DeepEqual(got["synonyms"], []string{"计算", "网络", "连接"}) {
		t.Fatalf("synonyms 域失真: %v", got["synonyms"])
	}
	// 全命中域=空数组（非 nil）
	labels2 := map[string][]string{"concepts": {"计算节点"}}
	got2 := missingByDomain([]string{"计算"}, labels2)
	if got2["concepts"] == nil || len(got2["concepts"]) != 0 {
		t.Fatalf("全命中应为空数组: %v", got2["concepts"])
	}
}
