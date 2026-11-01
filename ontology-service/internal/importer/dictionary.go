package importer

// REQ-424/M384 本体业务文档导出——概念词典（业务可读，03 §2.59 B6）：
// spec_json → markdown / HTML 两形态。受众=业务同事（术语-同义词-定义-层级表 + 关系表 +
// 数据属性表），非技术 TTL/JSON 形态的对偶交付物；沿 REQ-113 数据导出先例（服务端直出、
// Content-Disposition 下载）。同义词面=REQ-419 TBox 一等公民（Concept.Synonyms）。

import (
	"fmt"
	"html"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

// dictConceptRow 概念词典行（术语/同义词/定义/上位）——md 与 html 共用取数逻辑。
type dictConceptRow struct {
	Term     string
	Label    string
	Synonyms []string
	Def      string
	Parents  []string
}

// ExportDictionaryMarkdown 概念词典 markdown 形态。
func ExportDictionaryMarkdown(sp *pkgspec.Spec) string {
	var b strings.Builder
	name := strings.TrimSpace(sp.Name)
	if name == "" {
		name = "未命名本体"
	}
	b.WriteString(fmt.Sprintf("# %s 概念词典\n\n", name))
	if d := strings.TrimSpace(sp.Description); d != "" {
		b.WriteString(d + "\n\n")
	}
	b.WriteString(fmt.Sprintf("> 概念 %d · 关系 %d · 数据属性 %d · 实例 %d\n\n",
		len(sp.Concepts), len(sp.Relations), len(sp.DataProperties), len(sp.Instances)))

	b.WriteString("## 概念词典\n\n")
	if len(sp.Concepts) == 0 {
		b.WriteString("（本本体暂无概念）\n\n")
	} else {
		b.WriteString("| 术语 | 同义词 | 定义 | 上位概念 |\n| --- | --- | --- | --- |\n")
		for _, c := range sp.Concepts {
			term := c.Name
			if strings.TrimSpace(c.Label) != "" && c.Label != c.Name {
				term = fmt.Sprintf("%s（%s）", c.Name, c.Label)
			}
			row := dictConceptRow{Term: term, Label: c.Label, Synonyms: c.Synonyms, Def: c.Definition, Parents: c.Parents}
			def := strings.TrimSpace(row.Def)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				row.Term, dictJoin(row.Synonyms), def, dictJoin(row.Parents)))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 关系\n\n")
	if len(sp.Relations) == 0 {
		b.WriteString("（本本体暂无关系）\n\n")
	} else {
		b.WriteString("| 关系 | 从（定义域） | 到（值域） | 定义 |\n| --- | --- | --- | --- |\n")
		for _, r := range sp.Relations {
			def := strings.TrimSpace(r.Definition)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", dictRelLabel(r), r.From, r.To, def))
		}
		b.WriteString("\n")
	}

	if len(sp.DataProperties) > 0 {
		b.WriteString("## 数据属性\n\n| 属性 | 定义域 | 值域 | 定义 |\n| --- | --- | --- | --- |\n")
		for _, dp := range sp.DataProperties {
			def := strings.TrimSpace(dp.Definition)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", dp.Name, dp.Domain, dp.Range, def))
		}
		b.WriteString("\n")
	}

	if len(sp.Instances) > 0 {
		b.WriteString("## 实例\n\n| 实例 | 概念 | 属性 |\n| --- | --- | --- |\n")
		for _, ins := range sp.Instances {
			kvs := make([]string, 0, len(ins.Attributes))
			for k, v := range ins.Attributes {
				kvs = append(kvs, fmt.Sprintf("%s=%v", k, v))
			}
			attr := strings.Join(kvs, "；")
			if attr == "" {
				attr = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ins.Name, ins.Concept, attr))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ExportDictionaryHTML 概念词典 HTML 形态（独立生成不经 md 转换——零新增依赖，D-O5）。
func ExportDictionaryHTML(sp *pkgspec.Spec) string {
	e := html.EscapeString
	name := strings.TrimSpace(sp.Name)
	if name == "" {
		name = "未命名本体"
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString(fmt.Sprintf("<title>%s 概念词典</title>\n", e(name)))
	b.WriteString("<style>body{font-family:system-ui,-apple-system,'Segoe UI','PingFang SC','Microsoft YaHei',sans-serif;max-width:960px;margin:32px auto;padding:0 16px;color:#1f2430;line-height:1.7}h1{font-size:22px}h2{font-size:17px;border-bottom:1px solid #e5e7f0;padding-bottom:6px;margin-top:28px}table{border-collapse:collapse;width:100%;font-size:13.5px}th,td{border:1px solid #e5e7f0;padding:6px 10px;text-align:left;vertical-align:top}th{background:#f6f7fb}.meta{color:#6b7280;font-size:13px}</style>\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString(fmt.Sprintf("<h1>%s 概念词典</h1>\n", e(name)))
	if d := strings.TrimSpace(sp.Description); d != "" {
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", e(d)))
	}
	b.WriteString(fmt.Sprintf("<p class=\"meta\">概念 %d · 关系 %d · 数据属性 %d · 实例 %d</p>\n",
		len(sp.Concepts), len(sp.Relations), len(sp.DataProperties), len(sp.Instances)))

	b.WriteString("<h2>概念词典</h2>\n")
	if len(sp.Concepts) == 0 {
		b.WriteString("<p>（本本体暂无概念）</p>\n")
	} else {
		b.WriteString("<table><tr><th>术语</th><th>同义词</th><th>定义</th><th>上位概念</th></tr>\n")
		for _, c := range sp.Concepts {
			term := e(c.Name)
			if strings.TrimSpace(c.Label) != "" && c.Label != c.Name {
				term = fmt.Sprintf("%s（%s）", e(c.Name), e(c.Label))
			}
			def := strings.TrimSpace(c.Definition)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				term, e(dictJoin(c.Synonyms)), e(def), e(dictJoin(c.Parents))))
		}
		b.WriteString("</table>\n")
	}

	b.WriteString("<h2>关系</h2>\n")
	if len(sp.Relations) == 0 {
		b.WriteString("<p>（本本体暂无关系）</p>\n")
	} else {
		b.WriteString("<table><tr><th>关系</th><th>从（定义域）</th><th>到（值域）</th><th>定义</th></tr>\n")
		for _, r := range sp.Relations {
			def := strings.TrimSpace(r.Definition)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				e(dictRelLabel(r)), e(r.From), e(r.To), e(def)))
		}
		b.WriteString("</table>\n")
	}

	if len(sp.DataProperties) > 0 {
		b.WriteString("<h2>数据属性</h2>\n<table><tr><th>属性</th><th>定义域</th><th>值域</th><th>定义</th></tr>\n")
		for _, dp := range sp.DataProperties {
			def := strings.TrimSpace(dp.Definition)
			if def == "" {
				def = "—"
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				e(dp.Name), e(dp.Domain), e(dp.Range), e(def)))
		}
		b.WriteString("</table>\n")
	}

	if len(sp.Instances) > 0 {
		b.WriteString("<h2>实例</h2>\n<table><tr><th>实例</th><th>概念</th><th>属性</th></tr>\n")
		for _, ins := range sp.Instances {
			kvs := make([]string, 0, len(ins.Attributes))
			for k, v := range ins.Attributes {
				kvs = append(kvs, fmt.Sprintf("%s=%v", k, v))
			}
			attr := strings.Join(kvs, "；")
			if attr == "" {
				attr = "—"
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n", e(ins.Name), e(ins.Concept), e(attr)))
		}
		b.WriteString("</table>\n")
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// dictRelLabel 关系显示名（label 优先，括注 name）。
func dictRelLabel(r pkgspec.Relation) string {
	lbl := strings.TrimSpace(r.Label)
	if lbl == "" || lbl == r.Name {
		return r.Name
	}
	return fmt.Sprintf("%s（%s）", r.Name, lbl)
}

// dictJoin 列表单元（同义词/上位概念）——空=—。
func dictJoin(list []string) string {
	parts := make([]string, 0, len(list))
	for _, s := range list {
		if t := strings.TrimSpace(s); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "、")
}
