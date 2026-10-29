package kg

// KB-17/M289 单测：claim 结构化溯源 span——①原文精确定位（偏移即真相）②归一化窗口
//（大小写/空白漂移，映射回原偏移）③前缀兜底（chunk 可查 span 不落）④未命中（溯源缺失）。

import (
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func chunkOf(id, content string) *store.KnowledgeChunk {
	return &store.KnowledgeChunk{ID: id, Content: content}
}

func TestLocateClaimSpanExact(t *testing.T) {
	content := "阿司匹林是一种非甾体抗炎药。它常用于解热镇痛。"
	chunks := []*store.KnowledgeChunk{chunkOf("c1", content), chunkOf("c2", "无关内容")}
	cid, start, end := LocateClaimSpan("它常用于解热镇痛", chunks)
	if cid != "c1" {
		t.Fatalf("chunk 定位错: %s", cid)
	}
	r := []rune(content)
	if string(r[start:end]) != "它常用于解热镇痛" {
		t.Fatalf("span 切片不符: [%d,%d)=%q", start, end, string(r[start:end]))
	}
}

func TestLocateClaimSpanNormalized(t *testing.T) {
	// LLM 改写常见：空白/大小写漂移——归一化窗口命中并映射回原偏移
	content := "The Drug ASPIRIN is widely used."
	chunks := []*store.KnowledgeChunk{chunkOf("c1", content)}
	cid, start, end := LocateClaimSpan("drug aspirin is widely", chunks)
	if cid != "c1" || start < 0 {
		t.Fatalf("归一化定位失败: cid=%s start=%d", cid, start)
	}
	r := []rune(content)
	// 映射窗口应覆盖原文中含原大小写与空白的片段（去空白归一——span 端点为原 rune 位）
	if !strings.Contains(strings.ToLower(string(r[start:end])), "drug aspirin is widely") {
		t.Fatalf("span 窗口不含目标段: [%d,%d)=%q", start, end, string(r[start:end]))
	}
}

func TestLocateClaimSpanPrefixFallbackAndMiss(t *testing.T) {
	long := strings.Repeat("背景铺垫内容。", 10) + "阿司匹林是常用药，具有抗血小板聚集的作用与明确的适应症人群。"
	chunks := []*store.KnowledgeChunk{chunkOf("c1", long)}
	// 前缀兜底：claim 首 24 rune 恰为原文子串但全文不在（LLM 续写）——chunk 可查 span 不落
	lr := []rune(long)
	claim := string(lr[50:74]) + "（此外还有其他适应症与禁忌情形说明内容延续。）"
	cid, start, end := LocateClaimSpan(claim, chunks)
	if cid != "c1" {
		t.Fatalf("前缀兜底应定位 chunk: %s", cid)
	}
	if start != -1 || end != -1 {
		t.Fatalf("兜底级不落 span（弱定位如实）: %d %d", start, end)
	}
	// 完全未命中：溯源缺失如实
	cid2, s2, e2 := LocateClaimSpan("完全无关的虚构声明内容", chunks)
	if cid2 != "" || s2 != -1 || e2 != -1 {
		t.Fatalf("未命中应空三元组: %s %d %d", cid2, s2, e2)
	}
}
