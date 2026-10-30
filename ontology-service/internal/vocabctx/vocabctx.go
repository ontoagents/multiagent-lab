// Package vocabctx REQ-543/M350：LLM 生成注入的仓内词表上下文组装（103 号 §6.5 步骤二后半
// /§7.5-3，修痛点 P5）——FetchSpecVocab 先例扩面（backend/internal/ontology/ontology.go
// OntoVocab 投影口径同构：label 优先、名兜底、去重保序、条数截断），把只读词表投影从
// KB 抽取面扩到 LLM prompt 面。
//
// 两个出料（数据段由本包组装，prompt 拼接单源仍在消费方：llmcreate.buildPrompt /
// ontorag.extenderUser——llmcreate/ontorag 不感知仓库，经参数注入，D-O26 可插拔口径）：
//   - RepoList：AI 辅助创建用——仓内本体清单+可复用词表提示（命名与已有本体对齐、
//     高度重叠时提示用户可考虑复用/分叉）；
//   - RefVocab：扩展器用——目标本体已登记 external_refs（仓内引用）被引本体词表；
//     pin 按 REQ-540 口径解析（同目标多登记取最低 pin=最保守，与运行面 expandClosure/
//     ListPinEdges 收敛一致；无 pin 浮动头版本）。
//
// 成本护栏三件沿 REQ-500 CqQuickScreen 模式（103 号 §7.5-3）：
//
//	①注入上限——本体数/每本体词条数/总字符三重截断，截断处如实注记不静默丢弃；
//	②中性文案——只读参考、是否复用对齐由模型依语义判断不强制；且扩展片段的可引用
//	  名册仍限「目标本体∪片段」（被引词表是对齐参考不是可引用名册——保住两阶段
//	  校验 ValidateAgainstTarget 硬约束不被注入面破坏）；
//	③同版本缓存——版本指纹（head 版本集合 / pin 解析版本集合）不变直接回缓存，
//	  不重读 spec 工件重组装；指纹变才重建（本件花费=工件读+组装，无 LLM 调用）。
//
// 术语来源登记面（vocabsearch 选中→external_refs）归 REQ-504 命名联想批承接，本包不重复建设。
package vocabctx

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

// 注入上限（护栏①；Req543 测试钉桩——调参须同步测试）。
const (
	MaxRepoOntos  = 6    // RepoList 至多列出本体数（最近更新优先）
	MaxListTerms  = 8    // RepoList 每本体至多词条数
	MaxRefOntos   = 6    // RefVocab 至多被引本体数
	MaxRefTerms   = 12   // RefVocab 每本体至多概念词条数
	MaxRefRels    = 6    // RefVocab 每本体至多关系词条数
	MaxTotalChars = 4000 // 注入段总字符上限（含标题行；超限截断+注记）
)

const (
	cacheMax = 64 // 缓存条目容量兜底（超限整体重置；vocabsearch 缓存同思路）
	// capNote 截断注记（如实注记不静默丢弃）。
	capNote = "（已截断——注入上限护栏）"
)

// 中性文案段（护栏②）：只读参考、不强制对齐；扩展面额外钉住「可引用名册」边界，
// 保住片段两阶段校验（对目标∪片段引用完整性）不被注入面破坏。
const (
	repoHeader = "仓内已有本体（只读参考——命名与建模风格尽量与其一致，避免同义异名；若与某个已有本体高度重叠，请在本体 description 中提示用户可考虑复用或分叉该本体；是否采用由你依语义判断，不强制）："
	refHeader  = "已登记被引本体词表（external_refs 仓内引用——新建元素命名与概念措辞尽量与其对齐，避免同义异名；若某被引概念与本 CQ 语义匹配，优先复用目标本体中的同名/对齐元素；跨本体的复用/对齐由人工在资产页「引用与产物」登记 external_refs 落地。只读参考，是否采用由语义判断决定，不强制；片段中出现的元素名仍必须来自目标本体已有元素或本片段定义——被引词表是对齐参考，不是可引用名册）："
)

// Builder 词表上下文组装器（repo 只读消费；缓存进程内——单机单用户无需跨进程）。
type Builder struct {
	Store *repo.Store

	mu     sync.Mutex
	cache  map[string]ctxEntry
	hits   int
	misses int
}

type ctxEntry struct {
	fp  string // 版本指纹（head 版本集合 / pin 解析版本集合）
	txt string
}

// New 构造（st 只读使用；构造零 I/O，装配层可安全在 New(rest) 时即建）。
func New(st *repo.Store) *Builder {
	return &Builder{Store: st, cache: map[string]ctxEntry{}}
}

// CacheStats 同版本缓存命中统计（护栏③可观测；Req543 测试钉桩）。
func (b *Builder) CacheStats() (hits, misses int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hits, b.misses
}

// RepoList AI 辅助创建的仓内清单段（空仓/读失败返回空串——整段不进 prompt，增强非依赖，
// llmcreate.buildFewShot 同口径）。缓存 key="repo"、fp=全仓 id@version@name 有序指纹。
// 诚实边界：description 编辑不 bump 版本，缓存窗口内清单里的描述性内容（概念词条随版本
// 走不滞后）之外的名称已入指纹；仅 description 文本可能滞后（身份字段 id/name/version 均在指纹内）。
func (b *Builder) RepoList() string {
	heads, err := b.Store.ListOntologyHeads()
	if err != nil || len(heads) == 0 {
		return ""
	}
	fp := make([]string, 0, len(heads))
	for _, h := range heads {
		fp = append(fp, fmt.Sprintf("%s@%d@%s", h.ID, h.Version, h.Name))
	}
	sort.Strings(fp)
	return b.cached("repo", strings.Join(fp, "|"), func() string {
		var w strings.Builder
		w.WriteString(repoHeader)
		shown := 0
		for _, h := range heads {
			if shown >= MaxRepoOntos {
				fmt.Fprintf(&w, "\n（其余 %d 个本体略——注入上限护栏）", len(heads)-shown)
				break
			}
			t := projectSpec(b.specRaw(h.ID, 0), MaxListTerms, 0)
			line := fmt.Sprintf("\n%d. %s（%s） v%d", shown+1, h.Name, h.ID, h.Version)
			if len(t.Concepts) > 0 {
				line += " · 概念：" + strings.Join(t.Concepts, "、") + termsTotalSuffix(t.Concepts, t.ConceptTotal)
			}
			w.WriteString(line)
			shown++
		}
		return capTotal(w.String())
	})
}

// RefVocab 扩展器被引词表段（ontoID 无仓内引用/全为外部 IRI 登记态/目标全不可解析 → 空串，
// 整段不进 prompt）。pin 解析 REQ-540：同目标多登记取最低 pin（>0 中最小；全 0=浮动头）；
// 目标本体已删（missing_ontology 孤儿登记行）与工件/快照缺失的被引目标如实跳过不虚构。
// 缓存 key="refs:"+ontoID、fp=被引目标「解析后版本」有序指纹——head 演进（被引方升级）与
// pin 刷新都会改指纹；同指纹回缓存=同版本不重复花费。
func (b *Builder) RefVocab(ontoID string) string {
	refs, err := b.Store.ListExternalRefs(ontoID)
	if err != nil || len(refs) == 0 {
		return ""
	}
	type refTarget struct {
		pin    int
		hasPin bool
		kinds  map[string]bool
	}
	targets := map[string]*refTarget{}
	for _, r := range refs {
		if r.ToOntoID == "" {
			continue // 外部 IRI 登记态无仓内词表（D-O27 不拉取远端）
		}
		t := targets[r.ToOntoID]
		if t == nil {
			t = &refTarget{kinds: map[string]bool{}}
			targets[r.ToOntoID] = t
		}
		t.kinds[r.RefKind] = true
		if r.PinVersion > 0 && (!t.hasPin || r.PinVersion < t.pin) {
			t.pin, t.hasPin = r.PinVersion, true
		}
	}
	if len(targets) == 0 {
		return ""
	}
	heads := map[string]repo.OntologyHead{}
	if hs, herr := b.Store.ListOntologyHeads(); herr == nil {
		for _, h := range hs {
			heads[h.ID] = h
		}
	}
	// 解析后版本（pin>0 取 pin，否则头版本；目标不在 heads=已删，解析版本记 -1 仍入指纹——
	// 删除/重建都会改指纹，缓存不穿帮）
	resolved := make(map[string]int, len(targets))
	fps := make([]string, 0, len(targets))
	for id, t := range targets {
		v := -1
		if h, ok := heads[id]; ok {
			if t.hasPin {
				v = t.pin
			} else {
				v = h.Version
			}
		}
		resolved[id] = v
		fps = append(fps, fmt.Sprintf("%s@%d", id, v))
	}
	sort.Strings(fps)
	return b.cached("refs:"+ontoID, strings.Join(fps, "|"), func() string {
		ids := make([]string, 0, len(targets))
		for id := range targets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var w strings.Builder
		w.WriteString(refHeader)
		shown := 0
		for _, id := range ids {
			if shown >= MaxRefOntos {
				fmt.Fprintf(&w, "\n（其余 %d 个被引本体略——注入上限护栏）", len(ids)-shown)
				break
			}
			t := targets[id]
			v := resolved[id]
			if v < 0 {
				continue // 已删目标如实跳过（missing_ontology 口径同构，不虚构词表）
			}
			// 取数口径=REQ-540 引用解析点：pin>0 取该版本快照（GetVersionSpec），
			// 浮动头取头版本工件（GetArtifact——头版本号≠快照存在，快照只在保存点落）。
			raw := ""
			if t.hasPin {
				raw = b.specRaw(id, t.pin)
			} else {
				raw = b.specRaw(id, 0)
			}
			if strings.TrimSpace(raw) == "" {
				continue // 工件/快照缺失如实跳过不虚构
			}
			tm := projectSpec(raw, MaxRefTerms, MaxRefRels)
			h := heads[id]
			tag := kindTag(t.kinds)
			at := ""
			if t.hasPin {
				at = fmt.Sprintf(" · pin v%d", t.pin)
			}
			line := fmt.Sprintf("\n- %s（%s） v%d〔%s%s〕", h.Name, id, v, tag, at)
			if len(tm.Concepts) > 0 {
				line += "概念：" + strings.Join(tm.Concepts, "、") + termsTotalSuffix(tm.Concepts, tm.ConceptTotal)
			}
			if len(tm.Relations) > 0 {
				line += "｜关系：" + strings.Join(tm.Relations, "、") + termsTotalSuffix(tm.Relations, tm.RelTotal)
			}
			w.WriteString(line)
			shown++
		}
		if shown == 0 {
			return "" // 全部被引目标不可解析：整段缺席（增强非依赖）
		}
		return capTotal(w.String())
	})
}

// specRaw 取 spec_json 内容：version>0 取版本快照（REQ-540 pin 解析点），否则头版本工件。
// 读失败返回空串（调用方如实跳过）。
func (b *Builder) specRaw(ontoID string, version int) string {
	if version > 0 {
		raw, err := b.Store.GetVersionSpec(ontoID, version)
		if err != nil {
			return ""
		}
		return raw
	}
	raw, _, err := b.Store.GetArtifact(ontoID, "spec_json")
	if err != nil {
		return ""
	}
	return raw
}

// kindTag 登记种类标签（import/reuse/alignment 升序去重）。
func kindTag(kinds map[string]bool) string {
	ks := make([]string, 0, len(kinds))
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, "/")
}

// termsTotalSuffix 截断注记尾缀（「等 N 个」——只在实际截断时出现）。
func termsTotalSuffix(shown []string, total int) string {
	if total > len(shown) {
		return fmt.Sprintf(" 等 %d 个", total)
	}
	return ""
}

// capTotal 注入段总字符上限兜底（护栏①第三重；按 rune 截断避免切半个汉字）。
func capTotal(s string) string {
	rs := []rune(s)
	if len(rs) <= MaxTotalChars {
		return s
	}
	return string(rs[:MaxTotalChars]) + "\n" + capNote
}

// termItem 词条源字段（concepts/relations 共用投影）。
type termItem struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// specTerms spec 词表投影结果（概念/关系分开；总数供截断注记）。
type specTerms struct {
	Concepts     []string
	ConceptTotal int
	Relations    []string
	RelTotal     int
}

// projectSpec spec_json → 词条投影（FetchSpecVocab 口径同构：label 优先、名兜底、
// 去重保序；概念至多 capC 条、关系至多 capR 条，capR<=0 表示不取关系）。
// 解析失败返回零值（工件损坏不虚构词条）。
func projectSpec(raw string, capC, capR int) specTerms {
	if strings.TrimSpace(raw) == "" {
		return specTerms{}
	}
	var sp struct {
		Concepts  []termItem `json:"concepts"`
		Relations []termItem `json:"relations"`
	}
	if json.Unmarshal([]byte(raw), &sp) != nil {
		return specTerms{}
	}
	seen := map[string]bool{}
	project := func(items []termItem, capN int) ([]string, int) {
		var all []string
		for _, it := range items {
			for _, v := range []string{it.Label, it.Name} {
				if v = strings.TrimSpace(v); v != "" && !seen[v] {
					seen[v] = true
					all = append(all, v)
				}
			}
		}
		total := len(all)
		if total > capN {
			all = all[:capN]
		}
		return all, total
	}
	c, ct := project(sp.Concepts, capC)
	out := specTerms{Concepts: c, ConceptTotal: ct}
	if capR > 0 {
		r, rt := project(sp.Relations, capR)
		out.Relations, out.RelTotal = r, rt
	}
	return out
}

// cached 指纹命中回缓存（护栏③同版本不重复花费），未命中重建并写回（build 在锁外跑——
// Store 读不持本锁，无死锁面）。
func (b *Builder) cached(key, fp string, build func() string) string {
	b.mu.Lock()
	if e, ok := b.cache[key]; ok && e.fp == fp {
		b.hits++
		b.mu.Unlock()
		return e.txt
	}
	b.misses++
	b.mu.Unlock()
	txt := build()
	b.mu.Lock()
	if len(b.cache) >= cacheMax {
		b.cache = map[string]ctxEntry{}
	}
	b.cache[key] = ctxEntry{fp: fp, txt: txt}
	b.mu.Unlock()
	return txt
}
