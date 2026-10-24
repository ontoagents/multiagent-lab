package kb

// REQ-414/M210：KB 富格式摄入——PDF/DOCX **文本层**抽取（零新依赖、D-O15 不接 Python）。
// 选型结论（详设评估口径落档）：
//   - PDF：纯 Go 标准库不可行——PDF 二进制格式（xref/流压缩/字体编码）解析需完整第三方库
//     （pdfcpu/rsc.io/pdf 等），引入属新依赖决策。本件实现**朴素文本层抽取**：解析未压缩/
//     Flate 压缩的 BT..ET 文本对象流（Tj/TJ 操作符），覆盖「文本层未加密、无复杂字体编码」
//     的主流导出 PDF（Word/WPS/LaTeX pdflatex 直出）；加密/扫描件/复杂 CMap 编码返回明确
//     错误（如实边界——OCR 与完整解析维持池内）。
//   - DOCX：Office Open XML=zip 内 word/document.xml，w:t 节点即文本 run——archive 标准库
//     + XML 节点遍历即可完整覆盖文本层（零第三方依赖，覆盖率高）。
// 抽取均为**文本层**：图片内文字（OCR）/表格结构还原维持 KB-2 池触发驱动。

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ExtractText 抹平入口：按魔数分发 PDF/DOCX 文本层抽取。
func ExtractText(filename string, content []byte) (string, error) {
	switch {
	case strings.HasSuffix(strings.ToLower(filename), ".pdf") || (len(content) > 4 && string(content[:4]) == "%PDF"):
		return ExtractPDFText(content)
	case strings.HasSuffix(strings.ToLower(filename), ".docx"):
		return ExtractDOCXText(content)
	default:
		return "", fmt.Errorf("不支持的富格式（仅 .pdf/.docx 文本层）")
	}
}

// ---- DOCX：zip → word/document.xml → w:t 文本 run 拼接（段落 w:p 转换行）----

var xmlTagRe = regexp.MustCompile(`<[^>]*>`)

func ExtractDOCXText(content []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("DOCX 解析失败（非 zip 形态）: %w", err)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("DOCX 缺 word/document.xml（非 Word 文档？）")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	xmlBytes, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	xmlText := string(xmlBytes)
	// 段落边界：<w:p ...> → \n；其余标签剔除后即文本 run 连接
	var b strings.Builder
	pos := 0
	for {
		i := strings.Index(xmlText[pos:], "<w:p ")
		j := strings.Index(xmlText[pos:], "<w:p>")
		ni := -1
		nl := 0
		if i >= 0 && (j < 0 || i < j) {
			ni, nl = pos+i, len("<w:p ")
		} else if j >= 0 {
			ni, nl = pos+j, len("<w:p>")
		}
		if ni < 0 {
			b.WriteString(textOf(xmlText[pos:]))
			break
		}
		b.WriteString(textOf(xmlText[pos : ni+nl]))
		b.WriteString("\n")
		pos = ni + nl
	}
	return strings.TrimSpace(b.String()), nil
}

// textOf 剔除 XML 标签 + 还原基础实体（w:t 内即文本）。
func textOf(s string) string {
	s = strings.ReplaceAll(s, "</w:p>", "\n")
	s = xmlTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	return s
}

// ---- PDF：BT..ET 文本对象流抽取（未压缩/Flate；Tj 与 TJ 数组操作符）----

var (
	pdfStreamRe   = regexp.MustCompile(`(?s)stream\r?\n(.*?)(?:endstream|$)`)
	pdfTextObjRe  = regexp.MustCompile(`(?s)\[(.*?)\]\s*TJ|\(((?:[^()\\]|\\.)*)\)\s*Tj`)
	pdfEscapes    = strings.NewReplacer(`\(`, `(`, `\)`, `)`, `\\`, `\`)
)

func ExtractPDFText(content []byte) (string, error) {
	if len(content) > 100 && strings.Contains(string(content[:600]), "/Encrypt") {
		return "", fmt.Errorf("PDF 已加密——不支持（OCR/解密维持池内）")
	}
	var b strings.Builder
	for _, m := range pdfStreamRe.FindAllSubmatch(content, -1) {
		raw := m[1]
		data := raw
		if decoded, err := inflate(raw); err == nil {
			data = decoded
		}
		// 仅取 BT..ET 文本对象
		i := bytes.Index(data, []byte("BT"))
		for i >= 0 {
			j := bytes.Index(data[i:], []byte("ET"))
			if j < 0 {
				break
			}
			extractTextOps(data[i:i+j+2], &b)
			data = data[i+j+2:]
			i = bytes.Index(data, []byte("BT"))
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("PDF 文本层抽取为空——可能为扫描件/加密/复杂字体编码（如实边界：完整解析与 OCR 维持池内）")
	}
	return out, nil
}

// extractTextOps 从 BT..ET 块提取 Tj/TJ 字符串（ naive：不处理 Tf 字体编码映射）。
func extractTextOps(block []byte, b *strings.Builder) {
	for _, m := range pdfTextObjRe.FindAllSubmatch(block, -1) {
		if m[1] != nil { // TJ 数组：取其中 (...) 字符串片段
			for _, sm := range regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\)`).FindAllSubmatch(m[1], -1) {
				b.WriteString(pdfEscapes.Replace(string(sm[1])))
			}
		} else if m[2] != nil {
			b.WriteString(pdfEscapes.Replace(string(m[2])))
		}
		b.WriteString("\n")
	}
}

// inflate Flate 解压（容错：非压缩流原样返回失败由调用方当未压缩处理）。
func inflate(raw []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
