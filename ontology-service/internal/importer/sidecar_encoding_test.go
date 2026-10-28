package importer

// REQ-465 sidecar 导入编码根治回归：
// Windows PYTHONUTF8/UTF-8 模式运行时 stdin=surrogateescape，文件中的非 UTF-8 字节被读成
// 孤立代理字符（\udcXX），RDF/XML 在 rdflib 严格 utf-8 编码处崩「surrogates not allowed」
// （真机实锤：64KB 文件第 64071 字节单个 0x93 残留即崩）；cp936 严格环境则整读直接
// UnicodeDecodeError。根治=sidecar.py stdin 字节自读三级解码（utf-8 严格 → 稀疏坏字节
// utf-8+replace 保正文 → gb18030 全篇 GBK 无损）。依赖 python3+rdflib，缺失自动 skip
// （沿 sidecar_skos_test.go 口径）。

import (
	"strings"
	"testing"
)

// 受损 UTF-8 RDF/XML：合法 UTF-8 正文 + 单个 0x93 残留字节（cp1252 智能引号/GBK 尾字节典型残留，真实案例形态）。
const damagedRDFXML = `<?xml version="1.0" encoding="utf-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="http://example.org/e#Dev"><rdfs:label>` + "\x93" + `设备</rdfs:label></rdfs:Class>
</rdf:RDF>
`

// 全篇 GBK 中文 turtle：「网络设备」= GBK 字节 \xcd\xf8\xc2\xe7\xc9\xe8\xb1\xb8。
const gbkTTL = "@prefix ex: <http://example.org/g#> .\n" +
	"@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n" +
	"ex:dev a rdfs:Class ; rdfs:label \"\xcd\xf8\xc2\xe7\xc9\xe8\xb1\xb8\" .\n"

func TestSidecarDecodesDamagedUTF8RDFXML(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := parseViaSidecar(sc, FormatOWLRDF, damagedRDFXML)
	if err != nil {
		t.Fatalf("受损 UTF-8 RDF/XML 应降级成功而非崩溃（修复前=surrogates not allowed）: %v", err)
	}
	if len(sp.Concepts) != 1 {
		t.Fatalf("期望 1 概念，实际 %d", len(sp.Concepts))
	}
	name := sp.Concepts[0].Name
	if !strings.Contains(name, "设备") || !strings.Contains(name, "\ufffd") {
		t.Fatalf("正文应按 UTF-8 保留且坏字节落 U+FFFD，实际 %q", name)
	}
	if !rep.Lossy {
		t.Fatalf("坏字节替换应标记 lossy: %v", rep.Warnings)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "坏字节") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应含坏字节降级 warning: %v", rep.Warnings)
	}
}

func TestSidecarDecodesGBKFile(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := parseViaSidecar(sc, FormatTurtle, gbkTTL)
	if err != nil {
		t.Fatalf("全 GBK 文件应经 GB18030 无损解码: %v", err)
	}
	if len(sp.Concepts) != 1 || sp.Concepts[0].Name != "网络设备" {
		t.Fatalf("GBK 中文应无损还原为「网络设备」，实际 %v", sp.Concepts)
	}
	if rep.Lossy {
		t.Fatalf("GB18030 无损解码不应标记 lossy: %v", rep.Warnings)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "GB18030") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应含 GB18030 解码 warning: %v", rep.Warnings)
	}
	// canon 路径共享 stdin_text：GBK 输入同样不崩（REQ-328/C 运行装载双源回归）
	if _, n, err := CanonicalTTL(sc, FormatTurtle, gbkTTL); err != nil || n < 1 {
		t.Fatalf("canon 对 GBK 输入应成功: n=%d err=%v", n, err)
	}
}

func TestSidecarCleanUTF8NoDecodeWarning(t *testing.T) {
	sc := sidecarAvailable(t)
	_, rep, err := parseViaSidecar(sc, FormatTurtle, skosFixture)
	if err != nil {
		t.Fatalf("SKOS 导入失败: %v", err)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "UTF-8") || strings.Contains(w, "GB18030") {
			t.Fatalf("纯 UTF-8 输入不应出现解码 warning: %v", rep.Warnings)
		}
	}
}
