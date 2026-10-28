// Package importer 导入流（REQ-80/69）：格式嗅探 → 原样存档 → 解析为 spec_json → 导入报告。
// spec 无损；csv/graphml 有损（规则透明写入报告）；OWL/TTL 解析经 Python rdflib sidecar（不自研）。
package importer

import (
	"regexp"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os/exec"

	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// triplesRe sidecar export stderr 的三元组计数标记（REQ-293① 装载对账数据源）。
// \r? 容忍 windows CRLF（REQ-325 真机实锤：python stderr 为 \r\n，(?m)$ 不含 \r 前——
// 计数恒丢 → X-Onto-Triples 头恒缺 → 装载对账硬闸在 windows 全线 skipped）。
var triplesRe = regexp.MustCompile(`(?m)^TRIPLES (\d+)\r?$`)

// Format 支持的形态格式。
const (
	FormatSpecJSON = "spec_json"
	FormatCSV      = "csv"
	FormatGraphML  = "graphml"
	FormatOWLRDF   = "owl_rdfxml"
	FormatTurtle   = "turtle"
	FormatJSONLD   = "jsonld" // REQ-235③/M62：JSON-LD（sidecar rdflib json-ld 插件）
)

// Report 导入报告（统计 + warnings + 有损映射说明）。
type Report struct {
	Format    string   `json:"format"`
	Lossy     bool     `json:"lossy"`
	Warnings  []string `json:"warnings"`
	LossyNote string   `json:"lossy_note,omitempty"`
	// ExternalImports 导入内容中探测到的 owl:imports 目标 IRI（REQ-262/M256 D-O27 装载侧
	// 解析登记源——文本级提取零 sidecar 改动；mergeApply 时自动登记 external_refs kind=import）。
	ExternalImports []string `json:"external_imports,omitempty"`
	// Unprojected 未投影内容全量余量账本（REQ-328/B，原 REQ-318 未投影公理透传升级）：
	// 凡未投影且非纯系统词汇三元组全部 N-Triples 入账（无上限），导入侧存 artifact axioms-raw、
	// 导出拼接还原（77 号 §6 设计；确定性规则零 LLM）。
	Unprojected []string `json:"unprojected,omitempty"`
	// Reconciliation 对账块（REQ-328/B）：恒等式 triples_in = vocabulary + projected + remainder
	// （unaccounted=0 结构性保证）+ capture_rate（语义三元组捕获率——哨兵数据源）。
	Reconciliation *Reconciliation `json:"reconciliation,omitempty"`
}

// Reconciliation 导入对账块（REQ-328/B，77 号 §6）。
type Reconciliation struct {
	TriplesIn   int      `json:"triples_in"`
	Vocabulary  int      `json:"vocabulary"`  // 纯系统词汇三元组（rdf/rdfs/owl/skos/xsd 全系统主谓宾）
	Projected   int      `json:"projected"`   // 投影消费（结构+断言）
	Remainder   int      `json:"remainder"`   // 余量账本（未投影且非纯系统——BNode 结构/头部元数据/自定义谓词非实例断言）
	Unaccounted int      `json:"unaccounted"` // 恒等式差额（结构性为 0；非 0=sidecar 内部缺陷，前端红标）
	Inferred    []string `json:"inferred_notes,omitempty"`
	CaptureRate float64  `json:"capture_rate,omitempty"` // (projected+推断声明数)/语义三元组
}

type Sidecar struct {
	Python string // python3 可执行路径
	Script string // sidecar.py 绝对路径
}

// Sniff 格式嗅探：扩展名优先，内容特征兜底。
func Sniff(filename, content string) (string, error) {
	lf := strings.ToLower(filename)
	trimmed := strings.TrimSpace(content)
	switch {
	case strings.HasSuffix(lf, ".ttl"):
		return FormatTurtle, nil
	case strings.HasSuffix(lf, ".owl"), strings.HasSuffix(lf, ".rdf"), strings.HasSuffix(lf, ".rdfxml"), strings.HasSuffix(lf, ".xml"):
		isXML := strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<rdf:RDF")
		if !isXML && (strings.HasPrefix(trimmed, "@prefix") || strings.Contains(trimmed, "\n@prefix")) {
			return FormatTurtle, nil // .owl 后缀常装 Turtle 内容（实测 HOCC 即如此）
		}
		if strings.Contains(trimmed, "<graphml") {
			return FormatGraphML, nil
		}
		return FormatOWLRDF, nil
	case strings.HasSuffix(lf, ".graphml"):
		return FormatGraphML, nil
	case strings.HasSuffix(lf, ".jsonld"):
		return FormatJSONLD, nil // REQ-235③/M62
	case strings.HasSuffix(lf, ".csv"):
		return FormatCSV, nil
	case strings.HasSuffix(lf, ".json"):
		if looksLikeSpec(trimmed) {
			return FormatSpecJSON, nil
		}
		// REQ-235③：.json 里含 @context 的 JSON-LD 文档（@graph/@id RDF 语义，非 spec 三要素）
		if strings.Contains(trimmed, `"@context"`) {
			return FormatJSONLD, nil
		}
		return "", fmt.Errorf("json 内容不是 spec_json（缺少 concepts/relations/instances 三要素）")
	default:
		// 内容特征兜底
		if strings.HasPrefix(trimmed, "@prefix") || strings.Contains(trimmed, "@prefix") {
			return FormatTurtle, nil
		}
		if strings.Contains(trimmed, "<rdf:RDF") {
			return FormatOWLRDF, nil
		}
		if strings.Contains(trimmed, "<graphml") {
			return FormatGraphML, nil
		}
		if strings.Contains(trimmed, `"@context"`) {
			return FormatJSONLD, nil
		}
		if looksLikeSpec(trimmed) {
			return FormatSpecJSON, nil
		}
		return "", fmt.Errorf("无法识别格式（支持 spec_json / csv / graphml / owl-rdfxml / turtle / jsonld）")
	}
}

func looksLikeSpec(s string) bool {
	var probe struct {
		Concepts  *[]json.RawMessage `json:"concepts"`
		Relations *[]json.RawMessage `json:"relations"`
		Instances *[]json.RawMessage `json:"instances"`
	}
	return json.Unmarshal([]byte(s), &probe) == nil &&
		probe.Concepts != nil && probe.Relations != nil && probe.Instances != nil
}

// Import 解析为 spec_json + 报告。original 由调用方原样存档。
func Import(sidecar *Sidecar, filename, content string) (*pkgspec.Spec, *Report, error) {
	format, err := Sniff(filename, content)
	if err != nil {
		return nil, nil, err
	}
	switch format {
	case FormatSpecJSON:
		var sp pkgspec.Spec
		if err := json.Unmarshal([]byte(content), &sp); err != nil {
			return nil, nil, fmt.Errorf("spec_json 解析失败: %w", err)
		}
		rep := &Report{Format: format, ExternalImports: detectExternalImports(content)} // REQ-262/M256 装载侧解析登记源
		if errs := sp.Validate(); len(errs) > 0 {
			for _, e := range errs {
				rep.Warnings = append(rep.Warnings, "校验问题: "+e.Error())
			}
		}
		return &sp, rep, nil
	case FormatCSV:
		sp, rep := parseCSV(content)
		return sp, rep, nil
	case FormatGraphML:
		sp, rep := parseGraphML(content)
		return sp, rep, nil
	case FormatTurtle:
		// REQ-378①/M174：Turtle 先走 Go 读入端（零 Python，round-trip+教学主路径）；
		// 解析失败回落 sidecar（可选组件——复杂形态承载），双路错误如实透出
		if res, gerr := ReadTurtleSpec(content); gerr == nil {
			rep := &Report{Format: format, Reconciliation: &res.Recon, ExternalImports: detectExternalImports(content)}
			rep.Warnings = append(rep.Warnings, res.Warning...)
			rep.Warnings = append(rep.Warnings, "TTL 读入端=Go 原生（REQ-378，零 Python；余量为未投影计数非精确 N-Triples 账本）")
			return &res.Spec, rep, nil
		} else {
			sp, rep, serr := parseViaSidecar(sidecar, format, content)
			if serr != nil {
				return nil, nil, fmt.Errorf("Go TTL 读入端解析失败（%v）；sidecar 回落亦失败: %w", gerr, serr)
			}
			return sp, rep, nil
		}
	case FormatOWLRDF, FormatJSONLD:
		return parseViaSidecar(sidecar, format, content)
	}
	return nil, nil, fmt.Errorf("unsupported format %q", format)
}

// ---- csv：单文件列驱动（kind 列区分 concept/relation/instance/instance_rel）----

func parseCSV(content string) (*pkgspec.Spec, *Report) {
	rep := &Report{Format: FormatCSV, Lossy: true, LossyNote: "csv 为扁平表格，仅映射名称/标签/定义与引用，其他列丢弃计入 warnings"}
	rows := splitCSV(content)
	if len(rows) < 1 {
		return nil, rep
	}
	header := rows[0]
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(h))] = i
	}
	get := func(row []string, key string) string {
		i, ok := col[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	sp := &pkgspec.Spec{}
	cn := map[string]bool{}
	rn := map[string]bool{}
	in := map[string]bool{}
	for _, row := range rows[1:] {
		kind := strings.ToLower(get(row, "kind"))
		name := get(row, "name")
		switch kind {
		case "concept":
			if name == "" || cn[name] {
				rep.warn("跳过无效/重复概念行: " + name)
				continue
			}
			c := pkgspec.Concept{Name: name, Label: get(row, "label"), Definition: get(row, "definition")}
			if p := get(row, "parents"); p != "" {
				for _, x := range strings.Split(p, ";") {
					if x = strings.TrimSpace(x); x != "" {
						c.Parents = append(c.Parents, x)
					}
				}
			}
			sp.Concepts = append(sp.Concepts, c)
			cn[name] = true
		case "relation":
			if name == "" || rn[name] {
				rep.warn("跳过无效/重复关系行: " + name)
				continue
			}
			sp.Relations = append(sp.Relations, pkgspec.Relation{Name: name, Label: get(row, "label"), Definition: get(row, "definition"), From: get(row, "from"), To: get(row, "to")})
			rn[name] = true
		case "instance":
			if name == "" || in[name] {
				rep.warn("跳过无效/重复实例行: " + name)
				continue
			}
			it := pkgspec.Instance{Name: name, Concept: get(row, "concept"), Attributes: map[string]any{}}
			for k, i := range col {
				if k == "kind" || k == "name" || k == "concept" || strings.HasPrefix(k, "rel_") {
					continue
				}
				if i < len(row) && strings.TrimSpace(row[i]) != "" {
					it.Attributes[k] = strings.TrimSpace(row[i])
				}
			}
			if len(it.Attributes) == 0 {
				it.Attributes = nil
			}
			sp.Instances = append(sp.Instances, it)
			in[name] = true
		case "instance_rel":
			src, rel, tgt := get(row, "source"), get(row, "rel"), get(row, "target")
			idx := indexOfInstance(sp, src)
			if idx < 0 || rel == "" || tgt == "" {
				rep.warn(fmt.Sprintf("跳过无效实例关系行: %s -[%s]-> %s", src, rel, tgt))
				continue
			}
			sp.Instances[idx].Relations = append(sp.Instances[idx].Relations, pkgspec.InstanceRel{Rel: rel, Target: tgt})
		default:
			rep.warn("未知 kind 行已跳过: " + kind)
		}
	}
	return sp, rep
}

func indexOfInstance(sp *pkgspec.Spec, name string) int {
	for i := range sp.Instances {
		if sp.Instances[i].Name == name {
			return i
		}
	}
	return -1
}

// splitCSV 最小 CSV 解析（RFC4180 引号支持，无多行字段）。
func splitCSV(content string) [][]string {
	var rows [][]string
	var cur []string
	var field strings.Builder
	inQuote := false
	flush := func() {
		cur = append(cur, field.String())
		field.Reset()
	}
	endRow := func() {
		flush()
		rows = append(rows, cur)
		cur = nil
	}
	for _, r := range content {
		switch {
		case inQuote:
			if r == '"' {
				inQuote = false
			} else {
				field.WriteRune(r)
			}
		case r == '"':
			inQuote = true
		case r == ',':
			flush()
		case r == '\n' || r == '\r':
			if r == '\r' {
				continue
			}
			endRow()
		default:
			field.WriteRune(r)
		}
	}
	if field.Len() > 0 || len(cur) > 0 {
		endRow()
	}
	// 去掉全空行
	out := rows[:0]
	for _, row := range rows {
		empty := true
		for _, f := range row {
			if strings.TrimSpace(f) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, row)
		}
	}
	return out
}

// ---- graphml：node→实例（data key: concept），edge→实例关系（data key: rel/label）----

type graphmlDoc struct {
	Keys []struct {
		ID   string `xml:"id,attr"`
		For  string `xml:"for,attr"`
		Name string `xml:"attr.name,attr"`
	} `xml:"key"`
	Graph struct {
		Nodes []struct {
			ID    string `xml:"id,attr"`
			Datas []struct {
				Key string `xml:"key,attr"`
				Val string `xml:",chardata"`
			} `xml:"data"`
		} `xml:"node"`
		Edges []struct {
			Source string `xml:"source,attr"`
			Target string `xml:"target,attr"`
			Datas  []struct {
				Key string `xml:"key,attr"`
				Val string `xml:",chardata"`
			} `xml:"data"`
		} `xml:"edge"`
	} `xml:"graph"`
}

func parseGraphML(content string) (*pkgspec.Spec, *Report) {
	rep := &Report{Format: FormatGraphML, Lossy: true, LossyNote: "graphml 无本体语义：node→实例、edge→实例关系；未标注概念/关系的元素跳过并计入 warnings；未出现的概念/关系定义不生成"}
	var doc graphmlDoc
	if err := xml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, rep
	}
	// key 名映射
	nodeKeys, edgeKeys := map[string]string{}, map[string]string{}
	for _, k := range doc.Keys {
		if k.For == "node" || (k.For == "" && k.Name != "") {
			nodeKeys[k.ID] = k.Name
		}
		if k.For == "edge" {
			edgeKeys[k.ID] = k.Name
		}
	}
	sp := &pkgspec.Spec{}
	rn := map[string]bool{}
	in := map[string]bool{}
	for _, n := range doc.Graph.Nodes {
		name := strings.TrimSpace(n.ID)
		if name == "" || in[name] {
			rep.warn("跳过无效/重复节点: " + name)
			continue
		}
		it := pkgspec.Instance{Name: name, Attributes: map[string]any{}}
		for _, d := range n.Datas {
			key := nodeKeys[d.Key]
			switch strings.ToLower(key) {
			case "concept", "type":
				it.Concept = strings.TrimSpace(d.Val)
			case "label", "name":
				it.Name = strings.TrimSpace(d.Val)
				name = it.Name
			default:
				if v := strings.TrimSpace(d.Val); v != "" {
					it.Attributes[key] = v
				}
			}
		}
		if it.Name == "" {
			it.Name = name
		}
		sp.Instances = append(sp.Instances, it)
		in[it.Name] = true
	}
	for _, e := range doc.Graph.Edges {
		var rel string
		for _, d := range e.Datas {
			if k := strings.ToLower(edgeKeys[d.Key]); k == "rel" || k == "relation" || k == "label" {
				if rel == "" {
					rel = strings.TrimSpace(d.Val)
				}
			}
		}
		if rel == "" {
			rep.warn(fmt.Sprintf("边 %s->%s 无关系标注，跳过", e.Source, e.Target))
			continue
		}
		if !rn[rel] {
			rep.warn("关系未定义: " + rel + "（保留实例关系，校验时会报引用缺失）")
			rn[rel] = true
		}
		idx := indexOfInstance(sp, e.Source)
		if idx < 0 {
			rep.warn("边源节点不存在: " + e.Source)
			continue
		}
		sp.Instances[idx].Relations = append(sp.Instances[idx].Relations, pkgspec.InstanceRel{Rel: rel, Target: e.Target})
	}
	return sp, rep
}

// ---- OWL/TTL：经 sidecar（Python rdflib，不自研）----

type sidecarParseOut struct {
	Spec        pkgspec.Spec    `json:"spec"`
	Warnings    []string        `json:"warnings"`
	Lossy       bool            `json:"lossy"`
	Note        string          `json:"note"`
	Unprojected []string        `json:"unprojected,omitempty"` // REQ-328/B：全量余量账本（N-Triples 行）
	Recon       *Reconciliation `json:"reconciliation,omitempty"`
}

func parseViaSidecar(sc *Sidecar, format, content string) (*pkgspec.Spec, *Report, error) {
	if sc == nil || sc.Python == "" || sc.Script == "" {
		// REQ-378②/M174：sidecar 可选组件化——缺失给人话指引（缺什么/怎么装/当前哪些格式仍可用）
		return nil, nil, fmt.Errorf("RDF 解析组件（sidecar）未配置，无法解析该格式。缺什么：python3 + rdflib 库（环境变量 SIDECAR_PYTHON 指定解释器、SIDECAR_SCRIPT 指定 tools/rdf-sidecar/sidecar.py）。怎么装：安装 python3 后执行 pip install rdflib，或改用 run-dev.sh 启动（自动探测可用解释器）。当前仍可用格式：spec_json / Turtle（Go 原生读入端 REQ-378）/ CSV / GraphML；OWL/RDF-XML 与 JSON-LD 需 sidecar 在场")
	}
	cmd := exec.Command(sc.Python, sc.Script, "parse", "--format", format)
	cmd.Stdin = strings.NewReader(content)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("sidecar parse: %w; stderr: %s", err, strings.TrimSpace(errb.String()))
	}
	var res sidecarParseOut
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		return nil, nil, fmt.Errorf("sidecar 输出解析失败: %w; stdout: %.200s", err, out.String())
	}
	rep := &Report{Format: format, Lossy: res.Lossy, Warnings: res.Warnings, LossyNote: res.Note, ExternalImports: detectExternalImports(content)}
	rep.Unprojected = res.Unprojected
	rep.Reconciliation = res.Recon
	return &res.Spec, rep, nil
}

// CanonicalTTL REQ-328/C 运行装载双源：original 全图经 sidecar canon 直出 canonical Turtle
// （rdflib 图同构 100% 无损）+ 精确三元组计数（X-Onto-Triples 装载硬闸对账数据源）。
func CanonicalTTL(sc *Sidecar, format, content string) (string, int, error) {
	if sc == nil || sc.Python == "" || sc.Script == "" {
		return "", -1, fmt.Errorf("sidecar 未配置（SIDECAR_PYTHON / SIDECAR_SCRIPT）")
	}
	cmd := exec.Command(sc.Python, sc.Script, "canon", "--format", format)
	cmd.Stdin = strings.NewReader(content)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", -1, fmt.Errorf("sidecar canon: %w; stderr: %s", err, strings.TrimSpace(errb.String()))
	}
	m := triplesRe.FindStringSubmatch(errb.String())
	if m == nil {
		return "", -1, fmt.Errorf("sidecar canon 计数标记缺失")
	}
	n := 0
	_, _ = fmt.Sscanf(m[1], "%d", &n)
	return out.String(), n, nil
}

// Materialize REQ-481/M281 装载期跨本体推理物化：stdin TTL 全集并置 → sidecar materialize
//（owlrl OWL 2 RL 数据级闭包，axiomatic 词汇不入输出）→ 仅推断三元组 Turtle + TRIPLES 精确计数
//（owlrl 未安装 exit 3 → 显式错误，运行平面降级跳过不阻断装载）。
func Materialize(sc *Sidecar, format, content string) (string, int, error) {
	if sc == nil || sc.Python == "" || sc.Script == "" {
		return "", -1, fmt.Errorf("sidecar 未配置（SIDECAR_PYTHON / SIDECAR_SCRIPT）")
	}
	cmd := exec.Command(sc.Python, sc.Script, "materialize", "--format", format)
	cmd.Stdin = strings.NewReader(content)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if strings.Contains(errb.String(), "owlrl 未安装") {
			return "", -1, fmt.Errorf("owlrl 未安装（pip install owlrl）——物化档依赖缺失")
		}
		return "", -1, fmt.Errorf("sidecar materialize: %w; stderr: %s", err, strings.TrimSpace(errb.String()))
	}
	m := triplesRe.FindStringSubmatch(errb.String())
	if m == nil {
		return "", -1, fmt.Errorf("sidecar materialize 计数标记缺失")
	}
	n := 0
	_, _ = fmt.Sscanf(m[1], "%d", &n)
	return out.String(), n, nil
}

// ExportTTL spec_json → Turtle（Go 原生直出，REQ-321 迭代：导出链路脱离 python）。
// REQ-479/M279：refs 为 external_refs 导出桥投影（variadic 可选——不传即存量无桥形态）。
func ExportTTL(ontologyID string, sp *pkgspec.Spec, refs ...ExtRefBridge) (string, error) {
	ttl, _, err := ExportTTLCount(ontologyID, sp, refs...)
	return ttl, err
}

// ExportTTLCount 同 ExportTTL，另回三元组精确计数（REQ-293① 装载计数硬闸对账数据源）。
// 导出走 TTLGoExport（sidecar.py export 的字节级一致 Go 移植，对拍测试锁定），python 不再是
// 导出依赖——sidecar 消费面收窄为导入解析与 owlrl 推理检查两个。
func ExportTTLCount(ontologyID string, sp *pkgspec.Spec, refs ...ExtRefBridge) (string, int, error) {
	return TTLGoExport(ontologyID, sp, refs...)
}

func (r *Report) warn(s string) { r.Warnings = append(r.Warnings, s) }


// detectExternalImports 文本级提取 owl:imports 目标（REQ-262/M256 装载侧解析登记源）：
// Turtle 形态 `owl:imports <IRI>`（含 `a owl:Ontology` 头部声明）与 RDF/XML 形态
// `owl:imports` 元素 rdf:resource 属性；纯文本正则零 sidecar 改动（误报面=注释行，可接受——
// 登记态可手动摘除）。去重保序。
func detectExternalImports(content string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(iri string) {
		iri = strings.TrimSpace(iri)
		if iri == "" || seen[iri] {
			return
		}
		seen[iri] = true
		out = append(out, iri)
	}
	for _, m := range reTTLImports.FindAllStringSubmatch(content, -1) {
		// Turtle 谓词宾语列表形态（`owl:imports <a>, <b>`）——整段捕获再逐 IRI 拆分
		for _, iri := range reIRIToken.FindAllStringSubmatch(m[1], -1) {
			add(iri[1])
		}
	}
	for _, m := range reXMLImports.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	return out
}

var (
	reTTLImports = regexp.MustCompile(`owl\s*:\s*imports\s*((?:<\s*[^>\s]+\s*>\s*,?\s*)+)`)
	reIRIToken   = regexp.MustCompile(`<\s*([^>\s]+)\s*>`)
	reXMLImports = regexp.MustCompile(`<owl:\s*imports[^>]*rdf:\s*resource\s*=\s*"([^"]+)"`)
)
