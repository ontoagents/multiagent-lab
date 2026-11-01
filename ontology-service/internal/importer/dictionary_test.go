package importer

// REQ-424/M384 概念词典导出单测：md/html 两形态内容断言+空 spec 不 panic。

import (
	"strings"
	"testing"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

func dictStubSpec() *pkgspec.Spec {
	return &pkgspec.Spec{
		Name:        "订单域本体",
		Description: "订单业务概念词典样例",
		Concepts: []pkgspec.Concept{
			{Name: "订单", Label: "Order", Definition: "客户购买行为的凭证", Parents: []string{"单据"}, Synonyms: []string{"采购单"}},
			{Name: "单据", Definition: "凭证统称"},
		},
		Relations: []pkgspec.Relation{
			{Name: "开具", Label: "issue", From: "单据", To: "订单", Definition: "单据开具为订单"},
		},
		DataProperties: []pkgspec.DataProperty{
			{Name: "金额", Domain: "订单", Range: "number", Definition: "订单金额"},
		},
		Instances: []pkgspec.Instance{
			{Name: "SO-001", Concept: "订单", Attributes: map[string]any{"金额": 99.5}},
		},
	}
}

func TestDictionaryMarkdown(t *testing.T) {
	md := ExportDictionaryMarkdown(dictStubSpec())
	for _, want := range []string{
		"# 订单域本体 概念词典", "订单业务概念词典样例",
		"概念 2 · 关系 1 · 数据属性 1 · 实例 1",
		"| 订单（Order） | 采购单 | 客户购买行为的凭证 | 单据 |",
		"| 开具（issue） | 单据 | 订单 | 单据开具为订单 |",
		"| 金额 | 订单 | number | 订单金额 |",
		"| SO-001 | 订单 | 金额=99.5 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown 缺 %q\n---\n%s", want, md)
		}
	}
}

func TestDictionaryHTML(t *testing.T) {
	h := ExportDictionaryHTML(dictStubSpec())
	for _, want := range []string{
		"<title>订单域本体 概念词典</title>", "<h1>订单域本体 概念词典</h1>",
		"<td>订单（Order）</td><td>采购单</td><td>客户购买行为的凭证</td><td>单据</td>",
		"<td>开具（issue）</td><td>单据</td><td>订单</td>",
		"<td>金额</td><td>订单</td><td>number</td>",
	} {
		if !strings.Contains(h, want) {
			t.Fatalf("html 缺 %q\n---\n%s", want, h)
		}
	}
	// HTML 转义：定义含 <>& 不破坏结构
	sp := dictStubSpec()
	sp.Concepts[0].Definition = "a<b>&\"c\""
	h2 := ExportDictionaryHTML(sp)
	if !strings.Contains(h2, "a&lt;b&gt;&amp;&#34;c&#34;") && !strings.Contains(h2, "a&lt;b&gt;&amp;") {
		t.Fatalf("HTML 转义未生效: %s", h2)
	}
}

func TestDictionaryEmptySpec(t *testing.T) {
	sp := &pkgspec.Spec{Name: ""}
	md := ExportDictionaryMarkdown(sp)
	if !strings.Contains(md, "# 未命名本体 概念词典") || !strings.Contains(md, "（本本体暂无概念）") {
		t.Fatalf("空 spec md 空态不符: %s", md)
	}
	h := ExportDictionaryHTML(sp)
	if !strings.Contains(h, "（本本体暂无概念）") {
		t.Fatalf("空 spec html 空态不符: %s", h)
	}
}
