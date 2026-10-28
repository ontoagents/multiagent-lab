package importer

// TTLGoExport spec_json → Turtle 直出（REQ-321 迭代：导出链路脱离 python）。
// sidecar.py export 的字节级一致 Go 移植：IRI 方案（urn:o:{id}:concept|relation|instance|attr:*）、
// esc/sanitize 转义、字段拼接顺序、XSD range 映射逐行对齐（对拍测试 ttlexport_test.go 锁定，
// python+rdflib 在场时逐字节比对 sidecar 输出）。差异口径=实例 attributes 键序：两侧统一排序
// （sidecar 同步改为 sorted，消除 python dict 插入序 vs Go map 遍历序的不确定）。
// X-Onto-Triples 计数改算术精确累计（模板每分支可数），对拍测试断言与 sidecar rdflib len(g) 一致。

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// ttlSanitize 对齐 sidecar.py sanitize：strip 后逐字符，alnum/_-. 保留、其余替换 _。
// python str.isalnum() 为 unicode 语义（任意文字字母+数字+数字字符），Go 用 Letter/Digit/Number 近似。
func ttlSanitize(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsNumber(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ttlEsc 对齐 sidecar.py esc：str(v) 后反斜杠与双引号转义（换行不转义，沿 sidecar 既有语义）。
func ttlEsc(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, `"`, `\"`)
}

// ttlPyStr 对齐 python str() 的 JSON 值字符串化主形态：bool → True/False、nil → None、
// 整值浮点按整形输出（python json "42"→int→"42" 同形）。浮点 ".0" 尾数与科学计数为已知边缘差异
// （属性值实际以字符串为主，见 16 号 v0.20 诚实边界）。
// typedLiteral REQ-474/M274：按数据属性 range 短名输出类型化字面量——
// integer→裸整数 / number→xsd:double / boolean→真小写 true/false（修 python str() 的 True/False
// 非法字面量）/ date→"…"^^xsd:date；string/未声明/未知 range→引号字符串（含 python str() 形态的
// 数值/布尔字符串原样引号保底——ttlPyStr 仍兜底非 JSON 形态）。
func typedLiteral(v any, rng string) string {
	switch rng {
	case "integer":
		switch t := v.(type) {
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		case string:
			if _, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
				return strings.TrimSpace(t)
			}
		}
	case "number":
		switch t := v.(type) {
		case float64:
			s := strconv.FormatFloat(t, 'f', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0" // xsd:double 需小数点或指数形态
			}
			return s
		}
	case "boolean":
		switch t := v.(type) {
		case bool:
			if t {
				return "true"
			}
			return "false"
		case string:
			switch strings.TrimSpace(t) {
			case "true", "True", "1":
				return "true"
			case "false", "False", "0":
				return "false"
			}
		}
	case "date":
		return fmt.Sprintf("\"%s\"^^xsd:date", ttlPyStr(v))
	}
	return fmt.Sprintf("\"%s\"", ttlPyStr(v))
}

func ttlPyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return t
	case float64:
		s := strconv.FormatFloat(t, 'f', -1, 64)
		if !strings.ContainsAny(s, ".eE") && math.Abs(t) < 1e16 {
			return s
		}
		return s
	default:
		return fmt.Sprint(v)
	}
}

type ttlIRIs struct{ base string }

func (w ttlIRIs) concept(n string) string  { return w.base + "concept:" + ttlSanitize(n) }
func (w ttlIRIs) relation(n string) string { return w.base + "relation:" + ttlSanitize(n) }
func (w ttlIRIs) instance(n string) string { return w.base + "instance:" + ttlSanitize(n) }
func (w ttlIRIs) attr(k string) string     { return w.base + "attr:" + ttlSanitize(k) }

// ttlRangeIRI 对齐 sidecar XSD_IRI：短名映射 xsd、http(s) IRI 尖括号原样、未知回落 xsd:string。
func ttlRangeIRI(rng string) string {
	r := strings.TrimSpace(rng)
	switch r {
	case "string":
		return "xsd:string"
	case "number":
		return "xsd:double"
	case "integer":
		return "xsd:integer"
	case "boolean":
		return "xsd:boolean"
	case "date":
		return "xsd:date"
	}
	if strings.HasPrefix(r, "http://") || strings.HasPrefix(r, "https://") {
		return "<" + r + ">"
	}
	return "xsd:string"
}

// TTLGoExport spec_json → Turtle（字节级对齐 sidecar.py export），返回 TTL 与精确三元组数。
// REQ-216：空 spec（伴生绑定自动创建）导出为仅前缀的合法 TTL，计数 0（0=0 硬闸恒过）。
func TTLGoExport(ontologyID string, sp *pkgspec.Spec) (string, int, error) {
	if sp == nil {
		sp = &pkgspec.Spec{}
	}
	w := ttlIRIs{base: "urn:o:" + ttlSanitize(ontologyID) + ":"}
	var esc = ttlEsc
	// REQ-419：skos 前缀条件性——仅存在同义词时引入（TTLGoExport 与 sidecar 字节级对拍锁，
	// 无同义词的存量 spec 输出必须零变化）
	hasSynonyms := false
	for _, c := range sp.Concepts {
		if len(c.Synonyms) > 0 {
			hasSynonyms = true
			break
		}
	}
	L := make([]string, 0, 16+len(sp.Concepts)+len(sp.Relations)+len(sp.DataProperties)+len(sp.Axioms)+len(sp.Instances))
	prefixes := []string{
		fmt.Sprintf("@prefix o: <%s> .", w.base),
		"@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .",
		"@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .",
		"@prefix owl: <http://www.w3.org/2002/07/owl#> .",
	}
	if hasSynonyms {
		prefixes = append(prefixes, "@prefix skos: <http://www.w3.org/2004/02/skos/core#> .")
	}
	prefixes = append(prefixes, "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .", "")
	L = append(L, prefixes...)
	n := 0
	// REQ-474/M274：owl:Ontology 合成头——Protégé 打开可识别本体实体（IRI 沿 base 命名空间合成：
	// <base__ontology>；rdfs:label=名称恒有，rdfs:comment=描述有则出）。空名 spec（伴生空本体）
	// 不出头——保空 spec 仅前缀、计数 0 的装载对账不变量。
	if sp.Name != "" {
		head := fmt.Sprintf("<%s> rdf:type owl:Ontology ; rdfs:label \"%s\"", w.base+"__ontology", esc(sp.Name))
		n += 2
		if sp.Description != "" {
			head += fmt.Sprintf(" ;\n    rdfs:comment \"%s\"", esc(sp.Description))
			n++
		}
		L = append(L, head+" .")
	}
	for _, c := range sp.Concepts {
		u := w.concept(c.Name)
		line := fmt.Sprintf("<%s> rdf:type owl:Class ; rdfs:label \"%s\"", u, esc(c.Name))
		n += 2 // 头部 = rdf:type + rdfs:label 两条
		// REQ-419：同义词写出 skos:altLabel（round-trip 写出端——读入端 altLabel 捕获对偶；
		// 引擎侧 check_answerable 预检查询 UNION skos:altLabel 即命中同义词）。
		// 有同义词时补 skos:prefLabel 锚名——读入端 labelOf 优先级 prefLabel>altLabel>label，
		// 无锚名时首个 altLabel 会被误取为名（名称漂移）。
		if len(c.Synonyms) > 0 {
			line += fmt.Sprintf(" ;\n    skos:prefLabel \"%s\"", esc(c.Name))
			n++
		}
		// REQ-474/M274：展示名 rdfs:label 导出（原只写 Name——Label 字段从不导出的保真断裂；
		// 与 Name 同值不重复出，字节级对拍锁下无 Label 存量输出零变化）
		if c.Label != "" && c.Label != c.Name {
			line += fmt.Sprintf(" ;\n    rdfs:label \"%s\"", esc(c.Label))
			n++
		}
		for _, syn := range c.Synonyms {
			if strings.TrimSpace(syn) != "" {
				line += fmt.Sprintf(" ;\n    skos:altLabel \"%s\"", esc(syn))
				n++
			}
		}
		if c.Definition != "" {
			line += fmt.Sprintf(" ;\n    rdfs:comment \"%s\"", esc(c.Definition))
			n++
		}
		for _, p := range c.Parents {
			line += fmt.Sprintf(" ;\n    rdfs:subClassOf <%s>", w.concept(p))
			n++
		}
		// REQ-376①：限制公理写出——owl:Restriction 匿名父类形态（round-trip 写出端；
		// Go 读入端 ttlread 对偶投影）。计数=每限制 3 条（type/onProperty + 值元）。
		for _, rst := range c.Restrictions {
			if len(c.Parents) == 0 && rst == c.Restrictions[0] {
				// 无具名父类时限制仍是匿名节点（subClassOf 指向 BNode）——行内续写
			}
			line += fmt.Sprintf(" ;\n    rdfs:subClassOf [ rdf:type owl:Restriction ; owl:onProperty <%s>", w.relation(rst.Property))
			n += 2
			switch rst.Type {
			case "some_values_from":
				line += fmt.Sprintf(" ; owl:someValuesFrom <%s>", w.concept(rst.Target))
				n++
			case "all_values_from":
				line += fmt.Sprintf(" ; owl:allValuesFrom <%s>", w.concept(rst.Target))
				n++
			case "cardinality":
				line += fmt.Sprintf(" ; owl:cardinality \"%d\"^^xsd:nonNegativeInteger", rst.Value)
				n++
			case "min_cardinality":
				line += fmt.Sprintf(" ; owl:minCardinality \"%d\"^^xsd:nonNegativeInteger", rst.Value)
				n++
			case "max_cardinality":
				line += fmt.Sprintf(" ; owl:maxCardinality \"%d\"^^xsd:nonNegativeInteger", rst.Value)
				n++
			}
			line += " ]"
		}
		L = append(L, line+" .")
	}
	for _, r := range sp.Relations {
		u := w.relation(r.Name)
		line := fmt.Sprintf("<%s> rdf:type owl:ObjectProperty ; rdfs:label \"%s\"", u, esc(r.Name))
		n += 2 // 头部 = rdf:type + rdfs:label 两条
		if r.Definition != "" {
			line += fmt.Sprintf(" ;\n    rdfs:comment \"%s\"", esc(r.Definition))
			n++
		}
		// REQ-474/M274：关系展示名 rdfs:label 导出（同值不重复出）
		if r.Label != "" && r.Label != r.Name {
			line += fmt.Sprintf(" ;\n    rdfs:label \"%s\"", esc(r.Label))
			n++
		}
		if r.From != "" {
			line += fmt.Sprintf(" ;\n    rdfs:domain <%s>", w.concept(r.From))
			n++
		}
		if r.To != "" {
			line += fmt.Sprintf(" ;\n    rdfs:range <%s>", w.concept(r.To))
			n++
		}
		// REQ-473/M273：对象属性特性——附加 rdf:type 行（owl:Transitive/Symmetric/Functional/InverseFunctionalProperty）
		for _, ch := range r.Characteristics {
			var pref string
			switch ch {
			case "transitive":
				pref = "owl:TransitiveProperty"
			case "symmetric":
				pref = "owl:SymmetricProperty"
			case "functional":
				pref = "owl:FunctionalProperty"
			case "inverse_functional":
				pref = "owl:InverseFunctionalProperty"
			default:
				continue
			}
			line += fmt.Sprintf(" ;\n    rdf:type %s", pref)
			n++
		}
		if r.InverseOf != "" {
			line += fmt.Sprintf(" ;\n    owl:inverseOf <%s>", w.relation(r.InverseOf))
			n++
		}
		L = append(L, line+" .")
	}
	for _, dp := range sp.DataProperties {
		u := w.attr(dp.Name)
		line := fmt.Sprintf("<%s> rdf:type owl:DatatypeProperty ; rdfs:label \"%s\"", u, esc(dp.Name))
		n += 2 // 头部 = rdf:type + rdfs:label 两条
		if dp.Definition != "" {
			line += fmt.Sprintf(" ;\n    rdfs:comment \"%s\"", esc(dp.Definition))
			n++
		}
		if dp.Domain != "" {
			line += fmt.Sprintf(" ;\n    rdfs:domain <%s>", w.concept(dp.Domain))
			n++
		}
		line += fmt.Sprintf(" ;\n    rdfs:range %s", ttlRangeIRI(dp.Range))
		n++
		L = append(L, line+" .")
	}
	for _, ax := range sp.Axioms {
		pred := "owl:equivalentClass"
		if ax.Type == "disjoint_with" {
			pred = "owl:disjointWith"
		}
		if len(ax.Targets) == 0 {
			continue
		}
		tgts := make([]string, 0, len(ax.Targets))
		for _, t := range ax.Targets {
			tgts = append(tgts, fmt.Sprintf("<%s>", w.concept(t)))
		}
		L = append(L, fmt.Sprintf("<%s> %s %s .", w.concept(ax.Subject), pred, strings.Join(tgts, ", ")))
		n += len(ax.Targets)
	}
	// REQ-474/M274：实例数据断言按数据属性声明 range 类型化（原全字符串化+True/False/None
	// 非法 Turtle 字面量——按 range 出 xsd 类型化字面量；未声明/未知 range 回落字符串）。
	dpRange := map[string]string{}
	for _, dp := range sp.DataProperties {
		dpRange[dp.Name] = dp.Range
	}
	for _, it := range sp.Instances {
		u := w.instance(it.Name)
		line := fmt.Sprintf("<%s> rdf:type owl:NamedIndividual", u)
		n++
		if it.Concept != "" {
			line += fmt.Sprintf(" ;\n    rdf:type <%s>", w.concept(it.Concept))
			n++
		}
		line += fmt.Sprintf(" ;\n    rdfs:label \"%s\"", esc(it.Name))
		n++
		// 键序：两侧统一排序（sidecar.py 同步 sorted，见文件头口径注）
		keys := make([]string, 0, len(it.Attributes))
		for k := range it.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			line += fmt.Sprintf(" ;\n    <%s> %s", w.attr(k), typedLiteral(it.Attributes[k], dpRange[k]))
			n++
		}
		for _, ir := range it.Relations {
			line += fmt.Sprintf(" ;\n    <%s> <%s>", w.relation(ir.Rel), w.instance(ir.Target))
			n++
		}
		L = append(L, line+" .")
	}
	return strings.Join(L, "\n") + "\n", n, nil
}
