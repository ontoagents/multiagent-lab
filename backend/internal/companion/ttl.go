package companion

// ttl.go REQ-285④：伴生子图 TTL 导出——SPARQL JSON 绑定 → Turtle 序列化。
// 定位：让对话生长产物可进 Protégé 等外部工具（与普通本体「进得来出得去」的互操作面对齐）。
// 语法取 N-Triples 兼容形态（合法 Turtle 子集）+ @prefix 头美化谓词；主宾实体走完整 IRI
// （动态 slug 无前缀价值）。sparqlTerm/termToTurtle 复用 migrate216.go 既有定义。
// 图名不入文件（named graph 归属由导出方语境承载）。

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// prefixNS TTL 导出 @prefix 表（顺序即输出顺序；谓词与 xsd/prov 类型缩写用）。
var prefixNS = [][2]string{
	{"rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
	{"rdfs", "http://www.w3.org/2000/01/rdf-schema#"},
	{"bot", BotNS},
	{"prov", "http://www.w3.org/ns/prov#"},
	{"xsd", "http://www.w3.org/2001/XMLSchema#"},
}

// abbreviateIRI 命中 @prefix 表 → 缩写（xsd:string 等）；未命中 ok=false。
func abbreviateIRI(iri string) (string, bool) {
	for _, p := range prefixNS {
		if strings.HasPrefix(iri, p[1]) {
			return p[0] + ":" + strings.TrimPrefix(iri, p[1]), true
		}
	}
	return "", false
}

// predTurtle 谓词缩写（命中前缀表用缩写，否则原样尖括号）。
func predTurtle(iri string) string {
	if abbr, ok := abbreviateIRI(iri); ok {
		return abbr
	}
	return "<" + iri + ">"
}

// abbrevTerm term 缩写安全层：整项 IRI（主/宾 uri）与 `^^<iri>` datatype 位走前缀缩写；
// 字面量内容不触碰（引号内的 <...> 与末尾非 `> ` 形态均不命中）。
var iriTermRe = regexp.MustCompile(`\A<[^<>\s"]+>$`)

func abbrevTerm(t string) string {
	if iriTermRe.MatchString(t) {
		iri := t[1 : len(t)-1]
		if abbr, ok := abbreviateIRI(iri); ok {
			return abbr
		}
		return t
	}
	if strings.HasSuffix(t, ">") {
		if i := strings.LastIndex(t, "^^<"); i >= 0 {
			iri := t[i+3 : len(t)-1]
			if abbr, ok := abbreviateIRI(iri); ok {
				return t[:i] + "^^" + abbr + ">" // i 指向 ^^<，替换含 < 的尾段
			}
		}
	}
	return t
}

// SPARQLBindingsToTTL SPARQL SELECT JSON（s/p/o 绑定）→ Turtle 文档。
// 排序输出保证同图导出字节稳定（快照对账/测试断言友好）。
func SPARQLBindingsToTTL(raw []byte) ([]byte, error) {
	var res struct {
		Results struct {
			Bindings []map[string]sparqlTerm `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("SPARQL 结果解析失败: %w", err)
	}
	var b strings.Builder
	b.WriteString("# 伴生子图导出（OntoAgents REQ-285）——对话生长沉淀，薄本体词表 bot:\n")
	for _, p := range prefixNS {
		fmt.Fprintf(&b, "@prefix %s: <%s> .\n", p[0], p[1])
	}
	b.WriteString("\n")
	lines := make([]string, 0, len(res.Results.Bindings))
	for _, bind := range res.Results.Bindings {
		sTerm, pTerm, oTerm := bind["s"], bind["p"], bind["o"]
		if sTerm.Type == "" || pTerm.Type == "" || oTerm.Type == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s %s .\n", abbrevTerm(termToTurtle(sTerm)), predTurtle(pTerm.Value), abbrevTerm(termToTurtle(oTerm))))
	}
	sort.Strings(lines)
	for _, l := range lines {
		b.WriteString(l)
	}
	return []byte(b.String()), nil
}

// ExportTTL 宿主方案伴生子图全量导出为 Turtle（EnsureHost 读侧兜底拉起同口径；
// 空 named graph 返回仅头部的空文档——诚实空）。
func (s *Service) ExportTTL(ctx context.Context, ontologyID string) ([]byte, error) {
	base, err := s.EnsureHost(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	raw, err := s.Plans.Query(ctx, base, SelectGraphAllTriples(ontologyID))
	if err != nil {
		s.Plans.Invalidate(ontologyID)
		return nil, err
	}
	return SPARQLBindingsToTTL(raw)
}
