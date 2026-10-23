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
	L := make([]string, 0, 16+len(sp.Concepts)+len(sp.Relations)+len(sp.DataProperties)+len(sp.Axioms)+len(sp.Instances))
	L = append(L,
		fmt.Sprintf("@prefix o: <%s> .", w.base),
		"@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .",
		"@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .",
		"@prefix owl: <http://www.w3.org/2002/07/owl#> .",
		"@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .",
		"",
	)
	n := 0
	for _, c := range sp.Concepts {
		u := w.concept(c.Name)
		line := fmt.Sprintf("<%s> rdf:type owl:Class ; rdfs:label \"%s\"", u, esc(c.Name))
		n += 2 // 头部 = rdf:type + rdfs:label 两条
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
		if r.From != "" {
			line += fmt.Sprintf(" ;\n    rdfs:domain <%s>", w.concept(r.From))
			n++
		}
		if r.To != "" {
			line += fmt.Sprintf(" ;\n    rdfs:range <%s>", w.concept(r.To))
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
			line += fmt.Sprintf(" ;\n    <%s> \"%s\"", w.attr(k), esc(ttlPyStr(it.Attributes[k])))
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
