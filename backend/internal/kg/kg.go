// Package kg 自研轻量 KG 抽取（D-O15/REQ-110：去-semantica 化，反转 D-O10）。
//
// 抽切 REQ-98 LLM 能力代理（chat.GenerateStructured，主路径 method=llm，KB-6 起为
// 分窗并发 + 两步抽取，见 pipeline.go）；LLM 不可用/解析失败时回退内置规则抽取
// （「A 是 B」「A 的 B」句式，method=lightweight），保证 graphrag 模式零外部进程依赖
// （验收 22：run-dev.sh 一条命令、零 Python venv）。
// 独立成包原因：chat → kb 已有依赖，抽取若放 kb 包会成环；由 api 层注入 kb.Service。
package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// GenFuncs LLM 生成注入（REQ-355 R2：反转 kg→chat 正向 import——装配层闭包注入，
// 沿 kb.SetWikiLLM「kb 包不 import chat 防环」同款先例；签名仅用 kg/store 侧形态，
// kg 摘除不再拖出 chat）。nil 项按生成失败走既有回退/降级分支，规则抽取路径不受影响。
type GenFuncs struct {
	// GenStructured 结构化生成：返回模型输出的 JSON 原文（原 chat.GenerateStructured 的 DraftJSON 投影）。
	GenStructured func(ctx context.Context, connID, prompt, schemaJSON string) (json.RawMessage, error)
	// GenText 自由文本生成（temperature=nil 沿用连接默认）。
	GenText func(ctx context.Context, connID, system, user string, temperature *float64) (string, error)
}

// genStructured 结构化生成（nil 注入如实报错，调用方沿既有失败分支降级）。
func (g GenFuncs) genStructured(ctx context.Context, connID, prompt, schemaJSON string) (json.RawMessage, error) {
	if g.GenStructured == nil {
		return nil, fmt.Errorf("KG 结构化生成未注入（装配缺失）")
	}
	return g.GenStructured(ctx, connID, prompt, schemaJSON)
}

// genText 文本生成（nil 注入如实报错）。
func (g GenFuncs) genText(ctx context.Context, connID, system, user string, temperature *float64) (string, error) {
	if g.GenText == nil {
		return "", fmt.Errorf("KG 文本生成未注入（装配缺失）")
	}
	return g.GenText(ctx, connID, system, user, temperature)
}

// Extractor KG 抽取器（api 层构造并注入 kb.Service.KGExtract）。
type Extractor struct {
	Store  *store.Store
	Box    *secrets.Box
	ConnID string // 兜底模型连接；空 = 默认 chat 连接（REQ-98 规则）
	// OntoVocab 本体词表读取注入（M36/KB-6③；main 层经构建平面 spec 只读投影，见 pipeline.go）。
	OntoVocab OntoVocabFunc
	GenFuncs  // REQ-355 R2：LLM 生成注入（两步抽取主路径）
}

// kgEntityT/kgRelT/kgClaimT 抽取输出条目（LLM JSON 契约；轻量回退同形构造）。
type kgEntityT struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type kgRelT struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type kgClaimT struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

type kgOut struct {
	Entities  []kgEntityT `json:"entities"`
	Relations []kgRelT    `json:"relations"`
	Claims    []kgClaimT  `json:"claims"`
}

// ExtractForDoc 抽取某文档（或 KB 级重建 docID=""）的 KG 并落库。
// 永不返回 error：结果语义对齐 M14 GraphragInfo（degraded 不阻断导入主流程）。
func (x *Extractor) ExtractForDoc(ctx context.Context, kbID, docID string, chunks []*store.KnowledgeChunk) *store.GraphragInfo {
	if len(chunks) == 0 {
		return &store.GraphragInfo{Degraded: true, Error: "无 chunk 语料，KG 抽取跳过"}
	}
	kbRow := x.kbRow(kbID) // M16/REQ-129① 库级配置 + M36/KB-6 预算与本体挂载
	warnings := []string{}
	method := "llm"
	connID, promptOverride := x.extractConfig(kbRow)

	wins, truncated := buildWindows(chunks, budgetFor(kbRow))
	if truncated {
		warnings = append(warnings, fmt.Sprintf("语料超预算被截断（上限 %d chunks），KG 覆盖不完整", budgetFor(kbRow)))
	}
	// KB-6③：本体约束词表（挂载本体时拉只读投影；失败降级自由抽取并如实警告）
	var vocab *OntoVocab
	if kbRow != nil && kbRow.KGOntologyID != "" && x.OntoVocab != nil {
		v, err := x.OntoVocab(ctx, kbRow.KGOntologyID)
		if err != nil || v == nil {
			warnings = append(warnings, "本体词表加载失败，本次按自由抽取（无词表约束）: "+errString(err))
			log.Printf("[kg] kb=%s 本体词表加载失败 (onto=%s): %v", kbID, kbRow.KGOntologyID, err)
		} else {
			vocab = v
			warnings = append(warnings, fmt.Sprintf("本体约束抽取：挂载本体「%s」（概念 %d / 关系 %d 词表注入）", v.Name, len(v.Concepts), len(v.Relations)))
		}
	}
	out, warns, extractErr := x.llmExtractTwoPass(ctx, kbID, wins, connID, promptOverride, vocab)
	warnings = append(warnings, warns...)
	if out == nil {
		method = "lightweight"
		out = ruleExtract(chunks)
		warnings = append(warnings, "LLM 抽取失败，已回退规则抽取（「A 是 B」「A 的 B」句式）: "+errString(extractErr))
		log.Printf("[kg] kb=%s doc=%s LLM 抽取失败回退 lightweight: %v", kbID, docID, extractErr)
	} else if len(wins) > 0 {
		// KB-6① 成本预估提示（KB-O4 口径：token 消耗如实呈现）
		warnings = append(warnings, fmt.Sprintf("两步抽取：分 %d 窗 × 2 轮 ≈ %d 次 LLM 调用（并发 %d，预算 %d chunks）",
			len(wins), len(wins)*2, kgConcurrency(), budgetFor(kbRow)))
	}

	if len(out.Entities) == 0 && len(out.Relations) == 0 && len(out.Claims) == 0 {
		return &store.GraphragInfo{Degraded: true, Method: method, Chunks: len(chunks), Warnings: warnings, Error: "未抽取到任何实体/关系（语料过短或句式不匹配）"}
	}
	if err := x.persist(kbID, docID, out, chunks); err != nil {
		return &store.GraphragInfo{Degraded: true, Method: method, Chunks: len(chunks), Error: "KG 落库失败: " + err.Error(), Warnings: warnings}
	}
	info := &store.GraphragInfo{OK: true, Method: method, Chunks: len(chunks),
		Entities: len(out.Entities), Relationships: len(out.Relations), Warnings: warnings}
	log.Printf("[kg] kb=%s doc=%s 抽取完成: method=%s entities=%d relations=%d claims=%d",
		kbID, docID, method, len(out.Entities), len(out.Relations), len(out.Claims))
	return info
}

// kbRow 读取 KB 配置行（库级抽取配置：连接/提示词/预算/本体挂载）。
func (x *Extractor) kbRow(kbID string) *store.KnowledgeBase {
	if x.Store == nil {
		return nil
	}
	if k, err := x.Store.GetKnowledgeBase(kbID); err == nil {
		return k
	}
	return nil
}

// extractConfig 解析库级抽取配置（REQ-129①）：模型连接与提示词覆写（KB 配置优先，均空回退默认）。
func (x *Extractor) extractConfig(kbRow *store.KnowledgeBase) (connID, promptOverride string) {
	connID = x.ConnID
	if kbRow != nil {
		if kbRow.KGConnID != "" {
			connID = kbRow.KGConnID
		}
		promptOverride = strings.TrimSpace(kbRow.KGPrompt)
	}
	return connID, promptOverride
}

// persist 落库（同 KB 跨 doc 同名实体归一由 ReplaceKGForDoc 事务保证；claims 反查 chunk 做溯源）。
func (x *Extractor) persist(kbID, docID string, out *kgOut, chunks []*store.KnowledgeChunk) error {
	entities := make([]*store.KGEntity, 0, len(out.Entities))
	for _, e := range out.Entities {
		entities = append(entities, &store.KGEntity{ID: store.NewID(), KBID: kbID, DocID: docID,
			Name: e.Name, Type: e.Type, Description: e.Description})
	}
	rels := make([]*store.KGRelationship, 0, len(out.Relations))
	for _, r := range out.Relations {
		rels = append(rels, &store.KGRelationship{ID: store.NewID(), KBID: kbID, DocID: docID,
			Source: r.Source, Target: r.Target, Type: r.Type})
	}
	located, total := 0, 0
	claims := make([]*store.KGClaim, 0, len(out.Claims))
	for _, c := range out.Claims {
		total++
		chunkID, start, end := LocateClaimSpan(c.Text, chunks)
		if chunkID != "" && start >= 0 {
			located++
		}
		claims = append(claims, &store.KGClaim{ID: store.NewID(), KBID: kbID, DocID: docID,
			ChunkID: chunkID, Subject: c.Subject, Text: c.Text, SpanStart: start, SpanEnd: end})
	}
	if total > 0 {
		log.Printf("[kg] doc %s claim 溯源定位 %d/%d（%.0f%%）", docID, located, total, 100*float64(located)/float64(total))
	}
	return x.Store.ReplaceKGForDoc(kbID, docID, entities, rels, claims)
}

// LocateClaimSpan claim 文本回溯出处 chunk 与内容内 rune 起止偏移（KB-17/M289 结构化溯源）。
// 两级定位：①原文精确 Contains（偏移即真相）②归一化窗口 Contains（大小写/空白差异——
// 经 rune 级映射表回原偏移）；两级皆未命中 → 首 24 rune 前缀兜底（现状语义保留，start=-1
// 表未定位到原文段——溯源失败率=chunkID 空或 start<0 计数）。
func LocateClaimSpan(text string, chunks []*store.KnowledgeChunk) (chunkID string, start, end int) {
	start, end = -1, -1
	if strings.TrimSpace(text) == "" {
		return "", -1, -1
	}
	runes := []rune(text)
	// ①原文精确
	for _, c := range chunks {
		if i := strings.Index(c.Content, text); i >= 0 {
			start := len([]rune(c.Content[:i]))
			return c.ID, start, start + len(runes)
		}
	}
	// ②归一化窗口（小写+去空白）——构建 chunk 归一化 rune 数组与原 rune 偏移映射
	nText := normalizeForSpan(text)
	for _, c := range chunks {
		content := []rune(c.Content)
		norm := make([]rune, 0, len(content))
		mapBack := make([]int, 0, len(content)) // norm 序号 → content rune 序号
		for i, r := range content {
			for _, nr := range normalizeForSpan(string(r)) {
				norm = append(norm, nr)
				mapBack = append(mapBack, i)
			}
		}
		if i := strings.Index(string(norm), nText); i >= 0 {
			// i 为 byte 偏移（string Index）——换算 norm rune 序号
			nStart := len([]rune(string(norm)[:i]))
			nEnd := nStart + len([]rune(nText))
			if nEnd <= len(mapBack) {
				return c.ID, mapBack[nStart], mapBack[nEnd-1] + 1
			}
		}
	}
	// ③首 24 rune 前缀兜底（弱定位——chunk 可查，span 不落）
	head := runes
	if len(head) > 24 {
		head = head[:24]
	}
	for _, c := range chunks {
		if strings.Contains(c.Content, string(head)) {
			return c.ID, -1, -1
		}
	}
	return "", -1, -1
}

// normalizeForSpan 归一化（小写+剥离全部空白——LLM 改写常见的空白/大小写漂移）。
func normalizeForSpan(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u3000':
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// ---- 内置规则抽取（lightweight 回退；参考 semantica worker 内置轻量抽取的句式口径） ----

// ruleExtract 「A 是 B」→ IS_A、「A 的 B」→ 具有；claims = 含已知实体的句子。
// 纯字符串规则零依赖，保证 LLM 不可用时 graphrag 模式仍可用（教学演示兜底）。
func ruleExtract(chunks []*store.KnowledgeChunk) *kgOut {
	out := &kgOut{}
	entities := map[string]bool{}
	addEntity := func(name, typ, desc string) {
		name = strings.TrimSpace(name)
		if name == "" || len([]rune(name)) > 40 || entities[name] {
			return
		}
		entities[name] = true
		out.Entities = append(out.Entities, kgEntityT{Name: name, Type: typ, Description: desc})
	}
	sents := []string{}
	for _, c := range chunks {
		sents = append(sents, splitSentences(c.Content)...)
	}
	relSeen := map[string]bool{}
	addRel := func(source, target, typ string) {
		key := typ + "|" + source + "|" + target
		if source == "" || target == "" || source == target || relSeen[key] {
			return
		}
		relSeen[key] = true
		out.Relations = append(out.Relations, kgRelT{Source: source, Target: target, Type: typ})
	}
	for _, s := range sents {
		// 「A 是(一种/一类)B」
		for _, kw := range []string{"是一种", "是一类", "是一个", "是指", "是"} {
			if i := strings.Index(s, kw); i > 0 {
				a, b := strings.TrimSpace(s[:i]), trimTail(strings.TrimSpace(s[i+len(kw):]))
				if a != "" && b != "" && len([]rune(b)) <= 30 {
					addEntity(a, "concept", s)
					addEntity(b, "concept", "")
					addRel(a, b, "IS_A")
					out.Claims = append(out.Claims, kgClaimT{Subject: a, Text: s})
					break
				}
			}
		}
		// 「A 的 B」→ A 具有 B
		if i := strings.Index(s, "的"); i > 0 && i < len(s)-3 {
			a := headOf(strings.TrimSpace(s[:i]))
			b := trimTail(strings.TrimSpace(s[i+3:]))
			if a != "" && b != "" && len([]rune(b)) <= 20 {
				addEntity(a, "concept", "")
				addEntity(b, "property", "")
				addRel(a, b, "具有")
			}
		}
	}
	// claims：含已知实体的句子补录（每实体至多 2 条）
	perEntity := map[string]int{}
	for _, s := range sents {
		for name := range entities {
			if perEntity[name] >= 2 || !strings.Contains(s, name) || len([]rune(s)) < 6 {
				continue
			}
			perEntity[name]++
			out.Claims = append(out.Claims, kgClaimT{Subject: name, Text: s})
		}
	}
	if len(out.Claims) > 60 {
		out.Claims = out.Claims[:60]
	}
	return out
}

// splitSentences 粗分句（中文标点 + 换行）。
func splitSentences(text string) []string {
	raw := strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '；' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s = strings.TrimSpace(s); len([]rune(s)) >= 4 {
			out = append(out, s)
		}
	}
	return out
}

// headOf 取短语主干（截到最后一个分隔词前；教学口径粗规则，不求完备）。
func headOf(s string) string {
	for _, sep := range []string{"的", "与", "和", "及"} {
		if i := strings.LastIndex(s, sep); i > 0 {
			return s[:i]
		}
	}
	return s
}

// trimTail 去句尾连接词/标点残留。
func trimTail(s string) string {
	s = strings.TrimRight(s, "，。、；：,.;:")
	for _, sep := range []string{"其中", "并且", "而且", "同时", "通常", "一般"} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

// errString err 空安全转字符串（警告拼接用）。
func errString(err error) string {
	if err == nil {
		return "未知原因"
	}
	return err.Error()
}
