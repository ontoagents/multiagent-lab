package importer

// REQ-262/M256：owl:imports 文本级探测单测（装载侧解析登记源；D-O27）。

import "testing"

func TestDetectExternalImports(t *testing.T) {
	ttl := `@prefix owl: <http://www.w3.org/2002/07/owl#> .
<> a owl:Ontology ;
    owl:imports <http://example.org/vocab/core> , <http://example.org/vocab/ext#2> .
# 注释里的 owl:imports <http://fake.example/comment> 也会命中（登记态可摘除，边界在案）
<http://x> owl:imports <http://example.org/third> .`
	got := detectExternalImports(ttl)
	if len(got) != 4 {
		t.Fatalf("TTL 应探测 4 条 imports，得到 %v", got)
	}
	if got[0] != "http://example.org/vocab/core" || got[2] != "http://fake.example/comment" {
		t.Fatalf("顺序/内容不符: %v", got)
	}
	xml := `<?xml version="1.0"?><rdf:RDF xmlns:owl="http://www.w3.org/2002/07/owl#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<owl:Ontology><owl:imports rdf:resource="http://example.org/xml-dep"/></owl:Ontology></rdf:RDF>`
	got2 := detectExternalImports(xml)
	if len(got2) != 1 || got2[0] != "http://example.org/xml-dep" {
		t.Fatalf("RDF/XML 应探测 1 条: %v", got2)
	}
	if len(detectExternalImports("no imports here")) != 0 {
		t.Fatal("无 imports 应返回空")
	}
}
