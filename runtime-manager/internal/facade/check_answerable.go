package facade

// REQ-415/M211：check_answerable 可回答性预检（99 号 NVIDIA AutoOntology D1 借鉴——
// 握手阶段确定性预检，治「agent 盲跑后才发现本体不含该域」）。
// 实现=question 分词（rune bigram + 显式分隔词）与概念标签集 containment 匹配，
// 覆盖度=命中标签数/标签总数 与 命中问题词数/问题词数 双向综合；零 LLM 调用。
// suggestion=建议调用序（99 号 O1 搭车：guide 紧凑指引同口径）。

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// labelQueries REQ-545/M352 分域标签查询（rdf:type 分类命中——concepts=owl:Class、
// relations=owl:ObjectProperty、synonyms=skos:altLabel 全域）。
func labelQueries() map[string]string {
	base := `PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX owl: <http://www.w3.org/2002/07/owl#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>`
	return map[string]string{
		"concepts": base + `
SELECT DISTINCT ?label WHERE { ?c rdf:type owl:Class . { ?c rdfs:label ?label } UNION { ?c skos:prefLabel ?label } UNION { ?c skos:altLabel ?label } }`,
		"relations": base + `
SELECT DISTINCT ?label WHERE { ?p rdf:type owl:ObjectProperty . { ?p rdfs:label ?label } UNION { ?p skos:prefLabel ?label } }`,
		"synonyms": base + `
SELECT DISTINCT ?label WHERE { ?c skos:altLabel ?label }`,
	}
}

// fetchDomainLabels 拉取分域标签集（查询失败域=整体报错回收——不虚报命中）。
func (f *Facade) fetchDomainLabels(ctx context.Context, tool, oid, profileID, endpoint string) (map[string][]string, error) {
	out := map[string][]string{}
	for domain, q := range labelQueries() {
		bindings, err := f.exec(ctx, tool, oid, profileID, endpoint, q)
		if err != nil {
			return nil, fmt.Errorf("%s 标签拉取失败: %w", domain, err)
		}
		for _, b := range bindings {
			if l, ok := b["label"].(map[string]any); ok {
				if v, ok2 := l["value"].(string); ok2 && strings.TrimSpace(v) != "" {
					out[domain] = append(out[domain], v)
				}
			}
		}
	}
	return out, nil
}

// missingByDomain REQ-545/M352 分域零命中清单纯函数：问题词在对应域标签集上零命中者逐域收集
// （concepts/relations/synonyms 三域独立判定——出题/检索预检可据此指认「缺哪类证据」）。
// 判定口径与主匹配一致：标签含问题词（标签⊃词）或问题含完整标签（长标签短语）。
func missingByDomain(words []string, domainLabels map[string][]string) map[string][]string {
	missing := map[string][]string{}
	for domain, labels := range domainLabels {
		missing[domain] = []string{}
		for _, w := range words {
			if len([]rune(w)) < 2 {
				continue // 与主匹配同口径：单字符词不参与
			}
			hit := false
			for _, lab := range labels {
				// 缺失判定仅用「标签含词」方向（label⊃word）——「问题含完整标签」是标签级命中信号，
				// 不应使其它词免检（否则单标签入题即全词豁免，缺口清单失真）
				if strings.Contains(lab, w) {
					hit = true
					break
				}
			}
			if !hit {
				missing[domain] = append(missing[domain], w)
			}
		}
	}
	return missing
}

// CheckAnswerableCore 程序化调用核（MCP 工具与 REST 端点共用——REQ-545② REST 口与工具同源零漂移）。
func (f *Facade) CheckAnswerableCore(ctx context.Context, oid, question string) (map[string]any, error) {
	p, ep, errRes := f.route(oid)
	if errRes != nil {
		return nil, fmt.Errorf("本体路由失败（未装载或不存在）")
	}
	domains, err := f.fetchDomainLabels(ctx, "check_answerable", oid, p.ID, ep)
	if err != nil {
		return nil, err
	}
	labels := domains["concepts"]
	if len(labels) == 0 {
		return map[string]any{
			"answerable": false, "matched_concepts": []string{},
			"coverage": 0, "label_count": 0,
			"missing":    missingByDomain(questionWords(question), domains),
			"suggestion": "本体无概念标签（空本体或未装载）——先完成构建/装载再提问",
		}, nil
	}
	words := questionWords(question)
	matched := map[string]bool{}
	hits := 0
	for _, lab := range labels {
		for _, w := range words {
			if len([]rune(w)) >= 2 && strings.Contains(lab, w) {
				if !matched[lab] {
					matched[lab] = true
					hits++
				}
				break
			}
			if !matched[lab] && len([]rune(lab)) >= 2 && strings.Contains(question, lab) {
				matched[lab] = true
				hits++
			}
		}
	}
	coverage := float64(hits) / float64(len(labels))
	// answerable 阈值：≥10% 标签命中问题词（确定性保守口径——宁可 false 让 agent 换问法）
	answerable := hits > 0 && coverage >= 0.10
	sort.Strings(matchedKeys(matched))
	suggestion := "建议调用序：list_concepts 定位 → get_concept 看定义 → neighbors/sparql_query 取数据" +
		fmt.Sprintf("（本次匹配 %d/%d 标签，覆盖度 %.0f%%）", hits, len(labels), coverage*100)
	if !answerable {
		suggestion = "覆盖度低——问题可能超出本体域；建议换问法或确认本体是否覆盖该领域。" + suggestion
	}
	mc := matchedKeys(matched)
	return map[string]any{
		"answerable": answerable, "matched_concepts": mc,
		"coverage": coverage, "label_count": len(labels), "question_words": words,
		"missing":    missingByDomain(words, domains), // REQ-545/M352：分域零命中清单（既有键全保留向后兼容）
		"suggestion": suggestion,
		"scope":      scopeBlock(p), // REQ-482/M282：组作用域透出
	}, nil
}

func (f *Facade) handleCheckAnswerable() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		question, _ := args["question"].(string)
		if oid == "" || strings.TrimSpace(question) == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 question 必填"), nil
		}
		out, err := f.CheckAnswerableCore(ctx, oid, question)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}
		return jsonResult(out), nil
	}
}

func matchedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// questionWords 确定性分词：ASCII 词（≥2 字符）+ CJK 连续段切 bigram。
func questionWords(q string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, field := range strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == '?' || r == '？' || r == '.' || r == '。' || r == '!' || r == '！' || r == '\n' || r == '\t'
	}) {
		runes := []rune(field)
		var ascii strings.Builder
		cjk := []rune{}
		flushCjk := func() {
			if len(cjk) >= 2 {
				for i := 0; i+2 <= len(cjk); i++ {
					bg := string(cjk[i : i+2])
					if !seen[bg] {
						seen[bg] = true
						out = append(out, bg)
					}
				}
			} else if len(cjk) == 1 {
				w := string(cjk[0])
				if !seen[w] {
					seen[w] = true
					out = append(out, w)
				}
			}
			cjk = nil
		}
		for _, r := range runes {
			if r > 127 {
				cjk = append(cjk, r) // CJK 连续段累积（flush 在段边界）
			} else {
				flushCjk()
				if isWordByte(byte(r)) {
					ascii.WriteRune(r)
				} else {
					flushASCII(&ascii, seen, &out)
				}
			}
		}
		flushCjk()
		flushASCII(&ascii, seen, &out)
	}
	return out
}

func flushASCII(b *strings.Builder, seen map[string]bool, out *[]string) {
	w := strings.ToLower(b.String())
	if len([]rune(w)) >= 2 && !seen[w] {
		seen[w] = true
		*out = append(*out, w)
	}
	b.Reset()
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
