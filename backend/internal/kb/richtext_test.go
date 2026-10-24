package kb

// REQ-414/M210：富格式文本层抽取单测——DOCX（真 zip 形态）+ PDF（Flate BT/Tj 流）+ 边界。
import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"testing"
)

func TestExtractDOCXText(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<?xml version="1.0"?><w:document><w:body><w:p><w:r><w:t>第一段：本体工程</w:t></w:r></w:p><w:p><w:r><w:t>第二段：知识库 &amp; RAG</w:t></w:r></w:p></w:body></w:document>`))
	zw.Close()

	text, err := ExtractDOCXText(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(text), []byte("第一段：本体工程")) || !bytes.Contains([]byte(text), []byte("知识库 & RAG")) {
		t.Fatalf("文本抽取不符: %q", text)
	}
}

func TestExtractPDFTextFlate(t *testing.T) {
	// 构造含 Flate 压缩 BT/Tj 流的最小 PDF 内容载荷
	var content bytes.Buffer
	stream := "BT /F1 12 Tf (计算节点定义) Tj ET\nBT (存储节点) Tj ET"
	zw := zlib.NewWriter(&content)
	zw.Write([]byte(stream))
	zw.Close()

	pdf := append([]byte("%PDF-1.4\nstream\n"), content.Bytes()...)	// 真 PDF 形态：stream 关键字 + Flate 数据
	text, err := ExtractPDFText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(text), []byte("计算节点定义")) || !bytes.Contains([]byte(text), []byte("存储节点")) {
		t.Fatalf("PDF 文本抽取不符: %q", text)
	}
}

func TestExtractPDFTJArray(t *testing.T) {
	var content bytes.Buffer
	stream := "BT [(计) -100 (算)] TJ ET"
	zw := zlib.NewWriter(&content)
	zw.Write([]byte(stream))
	zw.Close()
	text, err := ExtractPDFText(append([]byte("%PDF-1.4\nstream\n"), content.Bytes()...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(text), []byte("计算")) {
		t.Fatalf("TJ 数组抽取不符: %q", text)
	}
}

func TestExtractTextErrors(t *testing.T) {
	if _, err := ExtractText("x.txt", []byte("纯文本")); err == nil {
		t.Fatal("非富格式应报错")
	}
	// 扫描件/空抽取：PDF 无文本流 → 空抽取报错
	if _, err := ExtractPDFText([]byte("%PDF-1.4\nnothing")); err == nil {
		t.Fatal("空抽取应报错")
	}
}
