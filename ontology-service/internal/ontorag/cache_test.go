package ontorag

import (
	"reflect"
	"testing"
)

// 缓存往返：同键命中返回同一元素集与向量（84 号 A2/86 号 N3——同 spec 重跑不重调 embedding）。
func TestVectorCacheRoundTrip(t *testing.T) {
	c := NewVectorCache(0)
	sp := testSpec()
	els := ElementsFromSpec("ont_x", sp)
	vecs := [][]float64{{1, 0}, {0, 1}}
	if _, _, ok := c.Get("ont_x", "fp1"); ok {
		t.Fatal("空缓存不应命中")
	}
	c.Put("ont_x", "fp1", els, vecs)
	gotEls, gotVecs, ok := c.Get("ont_x", "fp1")
	if !ok || !reflect.DeepEqual(gotEls, els) || !reflect.DeepEqual(gotVecs, vecs) {
		t.Fatalf("同键往返不一致: ok=%v", ok)
	}
	// 其他本体/其他指纹互不可见
	if _, _, ok := c.Get("ont_y", "fp1"); ok {
		t.Fatal("不同本体不应命中")
	}
	if _, _, ok := c.Get("ont_x", "fp2"); ok {
		t.Fatal("不同指纹不应命中")
	}
}

// spec 变更即新键：同本体新旧指纹并存互不污染（失效语义=内容寻址，无需显式失效通道）。
func TestVectorCacheInvalidation(t *testing.T) {
	c := NewVectorCache(0)
	c.Put("ont_x", "fp_v1", []Element{{LocalName: "旧"}}, [][]float64{{1}})
	c.Put("ont_x", "fp_v2", []Element{{LocalName: "新"}}, [][]float64{{2}})
	if els, _, _ := c.Get("ont_x", "fp_v1"); els[0].LocalName != "旧" {
		t.Fatal("旧指纹条目应保持不变")
	}
	if els, _, _ := c.Get("ont_x", "fp_v2"); els[0].LocalName != "新" {
		t.Fatal("新指纹条目应保持不变")
	}
}

// LRU 容量淘汰：超容量时最旧未访问条目被逐出，最近访问的保留。
func TestVectorCacheLRUEviction(t *testing.T) {
	c := NewVectorCache(2)
	c.Put("o", "k1", []Element{{LocalName: "1"}}, nil)
	c.Put("o", "k2", []Element{{LocalName: "2"}}, nil)
	c.Get("o", "k1") // 刷新 k1 访问序
	c.Put("o", "k3", []Element{{LocalName: "3"}}, nil)
	// k2 最旧被逐出，k1/k3 保留
	if _, _, ok := c.Get("o", "k2"); ok {
		t.Fatal("超容量应淘汰最旧的 k2")
	}
	if _, _, ok := c.Get("o", "k1"); !ok {
		t.Fatal("最近访问的 k1 应保留")
	}
	if _, _, ok := c.Get("o", "k3"); !ok {
		t.Fatal("最新写入的 k3 应保留")
	}
}

// 指纹：确定性 + 变更敏感（同文同键，任何变更即新键）。
func TestSpecFingerprint(t *testing.T) {
	a := SpecFingerprint(`{"name":"医疗"}`)
	if a != SpecFingerprint(`{"name":"医疗"}`) {
		t.Fatal("同文指纹应一致")
	}
	if a == SpecFingerprint(`{"name":"外科"}`) {
		t.Fatal("异文指纹应不同")
	}
}
