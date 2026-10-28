package importer

// TTLGoRead Turtle 方言 Go 读入端（REQ-378①/M174——与 ttlexport.go 写出端对偶，D-O15 纯 Go
// 单机叙事收尾）。覆盖两档：
//   ① spec 导出 round-trip——ttlexport 产出形态（concepts/relations/instances/data_properties
//      的 label/definition/parents/from/to/attributes/instance-relations）解析还原；
//   ② 教学导入主路径——常见 OWL/TTL 形态（owl:Class/rdfs:Class/skos:Concept 声明+rdfs:subClassOf
//      层级+rdfs:label/comment+owl:ObjectProperty domain/range+owl:DatatypeProperty+个体 rdf:type
//      断言与对象属性关系），含用法推断补声明（sidecar 同口径：类型使用/subClassOf 两端/domain·range
//      宾语 → 推断概念）。
// 诚实边界：全量余量账本/匿名公理 BNode 链/OWL 限制等复杂形态不在 Go 读入端范围（解析层面
// 容忍不失败——未投影三元组计入 reconciliation.remainder，但精确 N-Triples 账本与 lossy 语义
// 沿 sidecar）——解析失败时调用方回落 sidecar（REQ-378② 可选组件），复杂文件由 sidecar 承载。

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

const (
	nsRDF  = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	nsRDFS = "http://www.w3.org/2000/01/rdf-schema#"
	nsOWL  = "http://www.w3.org/2002/07/owl#"
	nsSKOS = "http://www.w3.org/2004/02/skos/core#"
	nsXSD  = "http://www.w3.org/2001/XMLSchema#"
)

// ttlTerm 词法 term（kind: iri|bnode|literal）。
type ttlTerm struct {
	Kind     string
	Value    string
	Lang     string
	Datatype string
}

// ttlTriple 一条三元组。
type ttlTriple struct{ S, P, O ttlTerm }

// TTLReadResult Go 读入端产物。
type TTLReadResult struct {
	Spec    pkgspec.Spec
	Recon   Reconciliation
	Warning []string
}

// ---- 词法与解析 ----

type ttlParser struct {
	src     []rune
	pos     int
	prefix  map[string]string
	base    string
 triples []ttlTriple
}

// ParseTurtleGo Turtle 文本 → 三元组（语法子集见文件头；解析失败返回 error 由调用方回落 sidecar）。
func ParseTurtleGo(src string) ([]ttlTriple, error) {
	p := &ttlParser{src: []rune(src), prefix: map[string]string{}}
	if err := p.parseDoc(); err != nil {
		return nil, err
	}
	return p.triples, nil
}

func (p *ttlParser) parseDoc() error {
	for {
		p.skipWS()
		if p.pos >= len(p.src) {
			return nil
		}
		if p.tryDirective() {
			continue
		}
		if err := p.parseTriplesStmt(); err != nil {
			return err
		}
	}
}

func (p *ttlParser) tryDirective() bool {
	rest := strings.ToLower(string(p.src[min2(p.pos, len(p.src)):min2(p.pos+10, len(p.src))]))
	at := strings.HasPrefix(string(p.src[p.pos:]), "@")
	if strings.HasPrefix(rest, "prefix") && !at {
		p.pos += 6
		p.skipWS()
		pn := p.readPNPrefix()
		p.skipOptColon()
		p.skipWS()
		iri := p.readIRI()
		p.skipWS()
		p.acceptDotSPARQL()
		if pn != "" && iri != nil {
			p.prefix[pn] = iri.Value
		}
		return true
	}
	if strings.HasPrefix(rest, "base") && !at {
		p.pos += 4
		p.skipWS()
		iri := p.readIRI()
		p.skipWS()
		p.acceptDotSPARQL()
		if iri != nil {
			p.base = iri.Value
		}
		return true
	}
	if at && strings.HasPrefix(rest, "@prefix") {
		p.pos += 7
		p.skipWS()
		pn := p.readPNPrefix()
		p.skipOptColon()
		p.skipWS()
		iri := p.readIRI()
		p.skipWS()
		p.expectDot()
		if pn != "" && iri != nil {
			p.prefix[pn] = iri.Value
		}
		return true
	}
	if at && strings.HasPrefix(rest, "@base") {
		p.pos += 5
		p.skipWS()
		iri := p.readIRI()
		p.skipWS()
		p.expectDot()
		if iri != nil {
			p.base = iri.Value
		}
		return true
	}
	return false
}

func (p *ttlParser) acceptDotSPARQL() {
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
	}
}

func (p *ttlParser) expectDot() error {
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		return nil
	}
	return fmt.Errorf("Turtle 语法错误：位置 %d 期望 '.'", p.pos)
}

// parseTriplesStmt subject predicateObjectList '.'（blank node property list 主语形态含入 subject）。
func (p *ttlParser) parseTriplesStmt() error {
	subj, err := p.parseSubject()
	if err != nil {
		return err
	}
	p.skipWS()
	// [ ... ] 主语后可能直接接谓词列表或 '.'
	if err := p.parsePredicateObjectList(subj); err != nil {
		return err
	}
	p.skipWS()
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		return nil
	}
	return fmt.Errorf("Turtle 语法错误：位置 %d 期待语句结束 '.'", p.pos)
}

func (p *ttlParser) parseSubject() (ttlTerm, error) {
	p.skipWS()
	if p.pos >= len(p.src) {
		return ttlTerm{}, fmt.Errorf("Turtle 意外结束")
	}
	switch p.src[p.pos] {
	case '[': // 匿名节点主语
		return p.parseAnonNode()
	default:
		return p.parseTerm(true)
	}
}

// parseAnonNode '[' predicateObjectList ']' → bnode（内联谓词对象落三元组）。
func (p *ttlParser) parseAnonNode() (ttlTerm, error) {
	p.pos++ // '['
	bn := ttlTerm{Kind: "bnode", Value: fmt.Sprintf("genid%d", p.pos)}
	p.skipWS()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.pos++
		return bn, nil
	}
	if err := p.parsePredicateObjectList(bn); err != nil {
		return ttlTerm{}, err
	}
	p.skipWS()
	if p.pos >= len(p.src) || p.src[p.pos] != ']' {
		return ttlTerm{}, fmt.Errorf("Turtle 语法错误：位置 %d 期望 ']'", p.pos)
	}
	p.pos++
	return bn, nil
}

func (p *ttlParser) parsePredicateObjectList(subj ttlTerm) error {
	for {
		p.skipWS()
		verb, err := p.parseVerb()
		if err != nil {
			return err
		}
		for {
			p.skipWS()
			obj, err := p.parseObject()
			if err != nil {
				return err
			}
			p.triples = append(p.triples, ttlTriple{S: subj, P: verb, O: obj})
			p.skipWS()
			if p.pos < len(p.src) && p.src[p.pos] == ',' {
				p.pos++
				continue
			}
			break
		}
		p.skipWS()
		if p.pos < len(p.src) && p.src[p.pos] == ';' {
			p.pos++
			p.skipWS()
			// 尾分号合法：';' 后接 '.' 直接返回
			if p.pos < len(p.src) && (p.src[p.pos] == '.' || p.src[p.pos] == ']' || p.src[p.pos] == '[') {
				return nil
			}
			continue
		}
		return nil
	}
}

func (p *ttlParser) parseVerb() (ttlTerm, error) {
	p.skipWS()
	if p.pos < len(p.src) && p.src[p.pos] == 'a' && (p.pos+1 >= len(p.src) || isTTLWS(p.src[p.pos+1])) {
		p.pos++
		return ttlTerm{Kind: "iri", Value: nsRDF + "type"}, nil
	}
	return p.parseTerm(false)
}

func (p *ttlParser) parseObject() (ttlTerm, error) {
	p.skipWS()
	if p.pos >= len(p.src) {
		return ttlTerm{}, fmt.Errorf("Turtle 意外结束（期待对象）")
	}
	switch p.src[p.pos] {
	case '<', '"', '\'':
		return p.parseTerm(false)
	case '[':
		return p.parseAnonNode()
	case '(':
		return p.parseCollection()
	default:
		return p.parseTerm(false)
	}
}

// parseCollection '(' object* ')' → bnode 链（rdf:first/rest，nil 收尾）——限制公理等形态
// 解析层容忍（三元组落账，投影层未消费计余量）。
func (p *ttlParser) parseCollection() (ttlTerm, error) {
	p.pos++ // '('
	var items []ttlTerm
	for {
		p.skipWS()
		if p.pos >= len(p.src) {
			return ttlTerm{}, fmt.Errorf("Turtle 意外结束（集合未闭合）")
		}
		if p.src[p.pos] == ')' {
			p.pos++
			break
		}
		it, err := p.parseObject()
		if err != nil {
			return ttlTerm{}, err
		}
		items = append(items, it)
	}
	nilT := ttlTerm{Kind: "iri", Value: nsRDF + "nil"}
	first := ttlTerm{Kind: "iri", Value: nsRDF + "first"}
	rest := ttlTerm{Kind: "iri", Value: nsRDF + "rest"}
	if len(items) == 0 {
		return nilT, nil
	}
	head := ttlTerm{Kind: "bnode", Value: fmt.Sprintf("coll%d", p.pos)}
	cur := head
	for i, it := range items {
		p.triples = append(p.triples, ttlTriple{S: cur, P: first, O: it})
		if i == len(items)-1 {
			p.triples = append(p.triples, ttlTriple{S: cur, P: rest, O: nilT})
		} else {
			nx := ttlTerm{Kind: "bnode", Value: fmt.Sprintf("coll%d_%d", p.pos, i)}
			p.triples = append(p.triples, ttlTriple{S: cur, P: rest, O: nx})
			cur = nx
		}
	}
	return head, nil
}

// parseTerm IRI / prefixed name / literal / bnode label / 数字与布尔。
func (p *ttlParser) parseTerm(asSubject bool) (ttlTerm, error) {
	if p.pos >= len(p.src) {
		return ttlTerm{}, fmt.Errorf("Turtle 意外结束")
	}
	switch p.src[p.pos] {
	case '<':
		t := p.readIRI()
		if t == nil {
			return ttlTerm{}, fmt.Errorf("IRI 解析失败")
		}
		return *t, nil
	case '"':
		return p.readLongOrNormalString('"')
	case '\'':
		return p.readLongOrNormalString('\'')
	case '_':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == ':' {
			p.pos += 2
			label := p.readPNLocal()
			return ttlTerm{Kind: "bnode", Value: label}, nil
		}
	case '@': // 直接 '@lang'（对象位置由字面量处理器处理；此处防御）
		return ttlTerm{}, fmt.Errorf("Turtle 语法错误：位置 %d 意外 '@'", p.pos)
	}
	// 数字/布尔
	if t, ok := p.tryNumberOrBool(); ok {
		return t, nil
	}
	// prefixed name
	pn := p.readPNPrefix()
	local := ""
	if p.pos < len(p.src) && p.src[p.pos] == ':' {
		p.pos++
		local = p.readPNLocal()
		ns, ok := p.prefix[pn]
		if !ok {
			return ttlTerm{}, fmt.Errorf("Turtle 未声明前缀 %q（位置 %d）", pn, p.pos)
		}
		return ttlTerm{Kind: "iri", Value: ns + local}, nil
	}
	return ttlTerm{}, fmt.Errorf("Turtle 语法错误：位置 %d 无法识别的 token 起始 %q", p.pos, string(p.src[p.pos]))
}

func (p *ttlParser) tryNumberOrBool() (ttlTerm, bool) {
	rest := string(p.src[p.pos:])
	for _, b := range []string{"true", "false"} {
		if strings.HasPrefix(rest, b) && (p.pos+len(b) >= len(p.src) || isTTLWS(p.src[p.pos+len(b)]) || punctTTL(p.src[p.pos+len(b)])) {
			p.pos += len(b)
			return ttlTerm{Kind: "literal", Value: b, Datatype: nsXSD + "boolean"}, true
		}
	}
	i := p.pos
	if i < len(p.src) && (p.src[i] == '+' || p.src[i] == '-') {
		i++
	}
	digits := false
	dot := false
	for i < len(p.src) && (runeIsDigit(p.src[i]) || p.src[i] == '.') {
		if runeIsDigit(p.src[i]) {
			digits = true
		}
		if p.src[i] == '.' {
			// 数字内嵌点（ex:1.5）与语句结束点区分：点后仍是数字则属数字
			if i+1 < len(p.src) && runeIsDigit(p.src[i+1]) {
				dot = true
			} else {
				break
			}
		}
		i++
	}
	if !digits {
		return ttlTerm{}, false
	}
	raw := string(p.src[p.pos:i])
	p.pos = i
	dt := nsXSD + "integer"
	if strings.ContainsAny(raw, ".eE") {
		dt = nsXSD + "decimal"
		if strings.ContainsAny(raw, "eE") {
			dt = nsXSD + "double"
		}
	}
	_ = dot
	return ttlTerm{Kind: "literal", Value: raw, Datatype: dt}, true
}

func (p *ttlParser) readIRI() *ttlTerm {
	if p.pos >= len(p.src) || p.src[p.pos] != '<' {
		return nil
	}
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) && p.src[p.pos] != '>' {
		b.WriteRune(p.src[p.pos])
		p.pos++
	}
	if p.pos < len(p.src) {
		p.pos++ // '>'
	}
	v := b.String()
	if p.base != "" && !strings.Contains(v, ":") {
		v = p.base + v
	}
	return &ttlTerm{Kind: "iri", Value: v}
}

func (p *ttlParser) readLongOrNormalString(q rune) (ttlTerm, error) {
	// 长字符串 """ ''' 
	if p.pos+2 < len(p.src) && p.src[p.pos+1] == q && p.src[p.pos+2] == q {
		p.pos += 3
		var b strings.Builder
		for p.pos < len(p.src) {
			if p.src[p.pos] == q && p.pos+2 < len(p.src) && p.src[p.pos+1] == q && p.src[p.pos+2] == q {
				p.pos += 3
				return ttlTerm{Kind: "literal", Value: b.String()}, nil
			}
			b.WriteRune(p.src[p.pos])
			p.pos++
		}
		return ttlTerm{}, fmt.Errorf("Turtle 长字符串未闭合")
	}
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\\' {
			p.pos++
			if p.pos >= len(p.src) {
				break
			}
			e := p.src[p.pos]
			switch e {
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case 'r':
				b.WriteRune('\r')
			case '"', '\'', '\\':
				b.WriteRune(e)
			case 'u':
				if p.pos+4 < len(p.src) {
					if n, err := strconv.ParseUint(string(p.src[p.pos+1:p.pos+5]), 16, 32); err == nil {
						b.WriteRune(rune(n))
						p.pos += 4
					}
				}
			default:
				b.WriteRune(e)
			}
			p.pos++
			continue
		}
		if c == q {
			p.pos++
			t := ttlTerm{Kind: "literal", Value: b.String()}
			// @lang / ^^datatype
			p.skipInline()
			if p.pos < len(p.src) && p.src[p.pos] == '@' {
				p.pos++
				var lb strings.Builder
				for p.pos < len(p.src) && !isTTLWS(p.src[p.pos]) && !punctTTL(p.src[p.pos]) {
					lb.WriteRune(p.src[p.pos])
					p.pos++
				}
				t.Lang = lb.String()
			} else if p.pos+1 < len(p.src) && p.src[p.pos] == '^' && p.src[p.pos+1] == '^' {
				p.pos += 2
				dt := p.readIRI()
				if dt != nil {
					t.Datatype = dt.Value
				} else {
					pn := p.readPNPrefix()
					if p.pos < len(p.src) && p.src[p.pos] == ':' {
						p.pos++
						local := p.readPNLocal()
						t.Datatype = p.prefix[pn] + local
					}
				}
			}
			return t, nil
		}
		b.WriteRune(c)
		p.pos++
	}
	return ttlTerm{}, fmt.Errorf("Turtle 字符串未闭合")
}

func (p *ttlParser) readPNPrefix() string {
	var b strings.Builder
	for p.pos < len(p.src) && isPNCharU(p.src[p.pos]) {
		b.WriteRune(p.src[p.pos])
		p.pos++
	}
	return b.String()
}

// readPNLocal 本地名：字母数字 _ - .（尾点回退语句界）与 % 转义容忍。
func (p *ttlParser) readPNLocal() string {
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if isPNCharU(c) || runeIsDigit(c) || c == '-' || c == '.' {
			b.WriteRune(c)
			p.pos++
			continue
		}
		break
	}
	v := b.String()
	// 尾点属语句界（Turtle 本地名不以点结尾）
	for strings.HasSuffix(v, ".") {
		v = strings.TrimSuffix(v, ".")
		p.pos--
	}
	return v
}

func (p *ttlParser) skipWS() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		if c == '#' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		return
	}
}

// skipOptColon 前缀声明/前缀名后的分隔冒号（readPNPrefix 不再消费冒号）。
func (p *ttlParser) skipOptColon() {
	p.skipWS()
	if p.pos < len(p.src) && p.src[p.pos] == ':' {
		p.pos++
	}
}

func (p *ttlParser) skipInline() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

// containsStr 轻量去重判含（REQ-419 同义词捕获用）。
func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func isTTLWS(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func punctTTL(c rune) bool { return c == '.' || c == ',' || c == ';' || c == '<' || c == '>' }
func runeIsDigit(c rune) bool { return c >= '0' && c <= '9' }
func isPNCharU(c rune) bool {
	return unicodeTTL(c)
}
func unicodeTTL(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c > 127
}
func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---- 投影：三元组 → spec（sidecar 核心规则的对偶实现）----

// ReadTurtleSpec Turtle → spec + 对账（REQ-378①：round-trip 与教学主路径）。
func ReadTurtleSpec(src string) (*TTLReadResult, error) {
	triples, err := ParseTurtleGo(src)
	if err != nil {
		return nil, err
	}
	res := &TTLReadResult{Spec: pkgspec.Spec{Concepts: []pkgspec.Concept{}, Relations: []pkgspec.Relation{}, Instances: []pkgspec.Instance{}, DataProperties: []pkgspec.DataProperty{}}}
	type termKey = ttlTerm
	isSys := func(t ttlTerm) bool {
		return t.Kind == "iri" && (strings.HasPrefix(t.Value, nsRDF) || strings.HasPrefix(t.Value, nsRDFS) ||
			strings.HasPrefix(t.Value, nsOWL) || strings.HasPrefix(t.Value, nsSKOS) || strings.HasPrefix(t.Value, nsXSD))
	}
	eq := func(a, b ttlTerm) bool { return a.Kind == b.Kind && a.Value == b.Value }

	typeT := ttlTerm{Kind: "iri", Value: nsRDF + "type"}
	subClassT := ttlTerm{Kind: "iri", Value: nsRDFS + "subClassOf"}
	labelT := ttlTerm{Kind: "iri", Value: nsRDFS + "label"}
	commentT := ttlTerm{Kind: "iri", Value: nsRDFS + "comment"}
	prefLabelT := ttlTerm{Kind: "iri", Value: nsSKOS + "prefLabel"}
	altLabelT := ttlTerm{Kind: "iri", Value: nsSKOS + "altLabel"}
	definitionT := ttlTerm{Kind: "iri", Value: nsSKOS + "definition"}
	domainT := ttlTerm{Kind: "iri", Value: nsRDFS + "domain"}
	rangeT := ttlTerm{Kind: "iri", Value: nsRDFS + "range"}
	broaderT := ttlTerm{Kind: "iri", Value: nsSKOS + "broader"}
	narrowerT := ttlTerm{Kind: "iri", Value: nsSKOS + "narrower"}

	classOf := func(t ttlTerm) bool {
		return t.Kind == "iri" && (t.Value == nsOWL+"Class" || t.Value == nsRDFS+"Class" || t.Value == nsSKOS+"Concept")
	}

	// 索引
	predObjs := map[termKey][]ttlTerm{} // s+pred -> objects
	objs := func(s, pred ttlTerm) []ttlTerm {
		var out []ttlTerm
		for _, t := range triples {
			if eq(t.S, s) && eq(t.P, pred) {
				out = append(out, t.O)
			}
		}
		_ = predObjs
		return out
	}

	declared := map[string]bool{} // iri
	usedClass := map[string]bool{}
	for _, t := range triples {
		if eq(t.P, typeT) && t.O.Kind == "iri" && !isSys(t.O) && t.S.Kind == "iri" && !isSys(t.S) {
			usedClass[t.O.Value] = true
		}
		if eq(t.P, subClassT) {
			if t.S.Kind == "iri" && !isSys(t.S) {
				usedClass[t.S.Value] = true
			}
			if t.O.Kind == "iri" && !isSys(t.O) {
				usedClass[t.O.Value] = true
			}
		}
		if (eq(t.P, domainT) || eq(t.P, rangeT)) && t.O.Kind == "iri" && !isSys(t.O) {
			usedClass[t.O.Value] = true
		}
	}
	for _, t := range triples {
		if eq(t.P, typeT) && classOf(t.O) && t.S.Kind == "iri" {
			declared[t.S.Value] = true
		}
	}
	classSet := map[string]bool{}
	for u := range declared {
		classSet[u] = true
	}
	for u := range usedClass {
		classSet[u] = true
	}

	labelOf := func(s ttlTerm) string {
		for _, pred := range []ttlTerm{prefLabelT, altLabelT, labelT} {
			for _, o := range objs(s, pred) {
				if o.Kind == "literal" && strings.TrimSpace(o.Value) != "" {
					return strings.TrimSpace(o.Value)
				}
			}
		}
		return ""
	}
	localOf := func(iri string) string {
		for _, sep := range []string{"#", "/"} {
			if i := strings.LastIndex(iri, sep); i >= 0 && i+1 < len(iri) {
				return iri[i+1:]
			}
		}
		// URN 冒号形态（ttlexport IRI 方案 urn:o:{id}:attr:{key} / relation:{name}——取尾段）
		if i := strings.LastIndex(iri, ":"); i >= 0 && i+1 < len(iri) {
			return iri[i+1:]
		}
		return iri
	}
	defOf := func(s ttlTerm) string {
		for _, pred := range []ttlTerm{commentT, definitionT} {
			for _, o := range objs(s, pred) {
				if o.Kind == "literal" && strings.TrimSpace(o.Value) != "" {
					return strings.TrimSpace(o.Value)
				}
			}
		}
		return ""
	}
	// 概念注册（确定性序：IRI 排序）
	iris := make([]string, 0, len(classSet))
	for u := range classSet {
		if strings.HasPrefix(u, nsRDFS) || strings.HasPrefix(u, nsOWL) || strings.HasPrefix(u, nsRDF) || strings.HasPrefix(u, nsSKOS) {
			continue
		}
		iris = append(iris, u)
	}
	sort.Strings(iris)
	nameByURI := map[string]string{}
	seenName := map[string]bool{}
	concepts := []pkgspec.Concept{}
	for _, u := range iris {
		t := ttlTerm{Kind: "iri", Value: u}
		name := labelOf(t)
		if name == "" {
			name = localOf(u)
		}
		if name == "" || seenName[name] {
			res.Warning = append(res.Warning, fmt.Sprintf("概念 %s 名称缺省或重复（%s）跳过", name, u))
			continue
		}
		seenName[name] = true
		nameByURI[u] = name
		c := pkgspec.Concept{Name: name}
		// REQ-419/M215：altLabel 捕获回写——labelOf 取首个作名/标签，其余同义值不再丢弃入余量，
		// 收进 Synonyms（TBox 一等公民；check_answerable 预检与业务口语查询的命中面）。
		for _, o := range objs(t, altLabelT) {
			if o.Kind == "literal" {
				if v := strings.TrimSpace(o.Value); v != "" && v != name && !containsStr(c.Synonyms, v) {
					c.Synonyms = append(c.Synonyms, v)
				}
			}
		}
		if d := defOf(t); d != "" {
			c.Definition = d
		}
		concepts = append(concepts, c)
	}
	// 第二轮：parents（subClassOf + skos:broader / narrower 反向；仅注册内目标）
	for i := range concepts {
		var u string
		for uri, n := range nameByURI {
			if n == concepts[i].Name {
				u = uri
				break
			}
		}
		if u == "" {
			continue
		}
		t := ttlTerm{Kind: "iri", Value: u}
		parents := []string{}
		addParent := func(pn string) {
			if pn != "" && pn != concepts[i].Name {
				for _, ex := range parents {
					if ex == pn {
						return
					}
				}
				parents = append(parents, pn)
			}
		}
		for _, o := range objs(t, subClassT) {
			if o.Kind == "iri" {
				addParent(nameByURI[o.Value])
			}
		}
		for _, o := range objs(t, broaderT) {
			if o.Kind == "iri" {
				addParent(nameByURI[o.Value])
			}
		}
		for _, t2 := range triples { // narrower 反向：?x skos:narrower t
			if eq(t2.P, narrowerT) && eq(t2.O, t) && t2.S.Kind == "iri" {
				addParent(nameByURI[t2.S.Value])
			}
		}
		if len(parents) > 0 {
			concepts[i].Parents = parents
		}
	}
	res.Spec.Concepts = concepts

	// 关系：owl:ObjectProperty
	propType := ttlTerm{Kind: "iri", Value: nsOWL + "ObjectProperty"}
	relSet := []string{}
	for _, t := range triples {
		if eq(t.P, typeT) && eq(t.O, propType) && t.S.Kind == "iri" &&
			!strings.HasPrefix(t.S.Value, nsRDF) && !strings.HasPrefix(t.S.Value, nsRDFS) && !strings.HasPrefix(t.S.Value, nsOWL) && !strings.HasPrefix(t.S.Value, nsSKOS) {
			relSet = append(relSet, t.S.Value)
		}
	}
	sort.Strings(relSet)
	relNameByURI := map[string]string{}
	for _, u := range relSet {
		t := ttlTerm{Kind: "iri", Value: u}
		name := labelOf(t)
		if name == "" {
			name = localOf(u)
		}
		if name == "" || seenName[name] {
			continue
		}
		seenName[name] = true
		relNameByURI[u] = name
		rel := pkgspec.Relation{Name: name, Evidence: "declared"}
		if d := defOf(t); d != "" {
			rel.Definition = d
		}
		doms := objs(t, domainT)
		rngs := objs(t, rangeT)
		if len(doms) > 0 {
			rel.From = nameByURI[doms[0].Value]
		}
		if len(rngs) > 0 {
			rel.To = nameByURI[rngs[0].Value]
		}
		if rel.From == "" || rel.To == "" {
			res.Warning = append(res.Warning, fmt.Sprintf("关系 %s 定义域或值域缺失/未注册为概念", name))
		}
		res.Spec.Relations = append(res.Spec.Relations, rel)
	}

	// 数据属性：owl:DatatypeProperty
	dpType := ttlTerm{Kind: "iri", Value: nsOWL + "DatatypeProperty"}
	dpSet := []string{}
	for _, t := range triples {
		if eq(t.P, typeT) && eq(t.O, dpType) && t.S.Kind == "iri" &&
			!strings.HasPrefix(t.S.Value, nsRDF) && !strings.HasPrefix(t.S.Value, nsRDFS) && !strings.HasPrefix(t.S.Value, nsOWL) && !strings.HasPrefix(t.S.Value, nsSKOS) {
			dpSet = append(dpSet, t.S.Value)
		}
	}
	sort.Strings(dpSet)
	dpNameByURI := map[string]string{}
	for _, u := range dpSet {
		t := ttlTerm{Kind: "iri", Value: u}
		name := labelOf(t)
		if name == "" {
			name = localOf(u)
		}
		if name == "" || seenName[name] {
			continue
		}
		seenName[name] = true
		dpNameByURI[u] = name
		dp := pkgspec.DataProperty{Name: name}
		if lbl := labelOf(t); lbl != "" && lbl != name {
			dp.Label = lbl
		}
		if d := defOf(t); d != "" {
			dp.Definition = d
		}
		if doms := objs(t, domainT); len(doms) > 0 {
			dp.Domain = nameByURI[doms[0].Value]
		}
		if rngs := objs(t, rangeT); len(rngs) > 0 && rngs[0].Kind == "iri" {
			dp.Range = xsdShort(rngs[0].Value)
		}
		res.Spec.DataProperties = append(res.Spec.DataProperties, dp)
	}

	// REQ-376①：限制公理投影——BNode[rdf:type owl:Restriction; onProperty; 值元] →
	// subClassOf 指向它的具名概念.Constraints（五型；导出端 ttlexport 对偶 round-trip）
	restrType := ttlTerm{Kind: "iri", Value: nsOWL + "Restriction"}
	onPropT := ttlTerm{Kind: "iri", Value: nsOWL + "onProperty"}
	svfT := ttlTerm{Kind: "iri", Value: nsOWL + "someValuesFrom"}
	avfT := ttlTerm{Kind: "iri", Value: nsOWL + "allValuesFrom"}
	cardT := ttlTerm{Kind: "iri", Value: nsOWL + "cardinality"}
	minT := ttlTerm{Kind: "iri", Value: nsOWL + "minCardinality"}
	maxT := ttlTerm{Kind: "iri", Value: nsOWL + "maxCardinality"}
	// BNode → Restriction
	restrByBNode := map[string]pkgspec.Restriction{}
	for _, t := range triples {
		if !eq(t.P, typeT) || !eq(t.O, restrType) || t.S.Kind != "bnode" {
			continue
		}
		rst := pkgspec.Restriction{}
		for _, tp := range triples {
			if !eq(tp.S, t.S) {
				continue
			}
			switch {
			case eq(tp.P, onPropT) && tp.O.Kind == "iri":
				rst.Property = relNameByURI[tp.O.Value]
				if rst.Property == "" {
					rst.Property = localOf(tp.O.Value)
				}
			case eq(tp.P, svfT) && tp.O.Kind == "iri":
				rst.Type, rst.Target = "some_values_from", nameByURI[tp.O.Value]
			case eq(tp.P, avfT) && tp.O.Kind == "iri":
				rst.Type, rst.Target = "all_values_from", nameByURI[tp.O.Value]
			case eq(tp.P, cardT) && tp.O.Kind == "literal":
				rst.Type = "cardinality"
				rst.Value, _ = strconv.Atoi(tp.O.Value)
			case eq(tp.P, minT) && tp.O.Kind == "literal":
				rst.Type = "min_cardinality"
				rst.Value, _ = strconv.Atoi(tp.O.Value)
			case eq(tp.P, maxT) && tp.O.Kind == "literal":
				rst.Type = "max_cardinality"
				rst.Value, _ = strconv.Atoi(tp.O.Value)
			}
		}
		if rst.Type != "" && rst.Property != "" {
			restrByBNode[t.S.Value] = rst
		}
	}
	if len(restrByBNode) > 0 {
		subClassOfT := subClassT
		for _, t := range triples {
			if !eq(t.P, subClassOfT) || t.O.Kind != "bnode" || t.S.Kind != "iri" {
				continue
			}
			if rst, ok := restrByBNode[t.O.Value]; ok {
				if cName := nameByURI[t.S.Value]; cName != "" {
					for i := range res.Spec.Concepts {
						if res.Spec.Concepts[i].Name == cName {
							res.Spec.Concepts[i].Restrictions = append(res.Spec.Concepts[i].Restrictions, rst)
							break
						}
					}
				}
			}
		}
	}

	// 实例：rdf:type 指向已注册概念的主体（非类自身）
	instNameByURI := map[string]string{}
	type instDraft struct {
		name string
		con  string
		uri  string
		atts map[string]string
		rels []pkgspec.InstanceRel
	}
	var drafts []*instDraft
	for _, t := range triples {
		if !eq(t.P, typeT) || t.S.Kind != "iri" || t.O.Kind != "iri" {
			continue
		}
		conName := nameByURI[t.O.Value]
		if conName == "" || nameByURI[t.S.Value] != "" {
			continue // 类型未注册/主体自身是类
		}
		name := labelOf(t.S)
		if name == "" {
			name = localOf(t.S.Value)
		}
		if name == "" {
			continue
		}
		d := &instDraft{name: name, con: conName, uri: t.S.Value, atts: map[string]string{}}
		instNameByURI[t.S.Value] = name
		drafts = append(drafts, d)
	}
	for _, d := range drafts {
		t := ttlTerm{Kind: "iri", Value: d.uri}
		for _, tr := range triples {
			if !eq(tr.S, t) || eq(tr.P, typeT) || isSys(tr.P) || tr.P.Kind != "iri" {
				continue
			}
			pn := relNameByURI[tr.P.Value]
			if pn == "" {
				pn = dpNameByURI[tr.P.Value]
			}
			if pn == "" {
				pn = localOf(tr.P.Value)
			}
			switch {
			case tr.O.Kind == "literal":
				d.atts[pn] = tr.O.Value
			case tr.O.Kind == "iri" && instNameByURI[tr.O.Value] != "":
				d.rels = append(d.rels, pkgspec.InstanceRel{Rel: pn, Target: instNameByURI[tr.O.Value]})
			}
		}
		inst := pkgspec.Instance{Name: d.name, Concept: d.con, Attributes: map[string]any{}}
		for k, v := range d.atts {
			inst.Attributes[k] = v
		}
		inst.Relations = d.rels
		res.Spec.Instances = append(res.Spec.Instances, inst)
	}

	// 对账：vocabulary（纯系统词汇三元组）/ projected（投影消费）/ remainder（未投影）
	vocabCount, projCount := 0, 0
	for _, t := range triples {
		if isSys(t.S) && isSys(t.P) && isSys(t.O) {
			vocabCount++
			continue
		}
		if eq(t.P, typeT) && classOf(t.O) {
			projCount++
			continue
		}
		if eq(t.P, subClassT) || eq(t.P, broaderT) || eq(t.P, narrowerT) {
			projCount++
			continue
		}
		if eq(t.P, labelT) || eq(t.P, prefLabelT) || eq(t.P, altLabelT) || eq(t.P, commentT) || eq(t.P, definitionT) {
			projCount++
			continue
		}
		if eq(t.P, domainT) || eq(t.P, rangeT) {
			projCount++
			continue
		}
		if eq(t.P, typeT) && nameByURI[t.O.Value] != "" && t.S.Kind == "iri" { // 实例类型断言
			projCount++
			continue
		}
		if t.S.Kind == "iri" && instNameByURI[t.S.Value] != "" { // 实例断言
			projCount++
			continue
		}
		if t.P.Kind == "iri" && (relNameByURI[t.P.Value] != "" || dpNameByURI[t.P.Value] != "") {
			projCount++
			continue
		}
	}
	res.Recon = Reconciliation{TriplesIn: len(triples), Vocabulary: vocabCount, Projected: projCount, Remainder: len(triples) - vocabCount - projCount}
	// 概念锚名兜底（spec.Name 空时取首概念——REQ-262 交付轮 vet 顺修：原 self-assignment 无操作删除）
	if res.Spec.Name == "" && len(concepts) > 0 {
		res.Spec.Name = concepts[0].Name
	}
	return res, nil
}

// xsdShort xsd IRI → 短名（sidecar 同口径）。
func xsdShort(iri string) string {
	if strings.HasPrefix(iri, nsXSD) {
		return strings.TrimPrefix(iri, nsXSD)
	}
	return iri
}
