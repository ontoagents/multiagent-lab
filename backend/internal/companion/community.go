package companion

// REQ-288/M93 伴生社区摘要（对齐 Graphiti community summaries；复用 KB GraphRAG REQ-130
// 管线形态）：Louvain 社区检测（kg 包，gonum 固定种子）→ LLM 摘要（失败回退骨架）→
// companion_community 表缓存；成本口径沿 KB-O4——eager ≤12 全量即时，超出 pending 按需生成；
// 成员不变的社区重建时保留既有摘要不重烧 LLM。检索零命中时社区摘要兜底注入（可选检索注入臂）。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/kg"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// communityEagerLimit 重建时即时生成摘要的社区数上限（超出=pending 按需；沿 KB eagerSummaryLimit）。
const communityEagerLimit = 12

// communityFallbackTopN 检索零命中兜底注入的社区数上限。
const communityFallbackTopN = 2

// communitySumSchema 摘要 JSON 契约（沿 kg.communitySumSchema 口径，面向全貌抽象问题）。
const communitySumSchema = `{"summary":"用 2~3 句话概括该社区整体在讲什么（面向「这个伴生本体整体沉淀了什么」的全貌问题）"}`

// companionRel 伴生活跃关系边三元组（社区建图与内部关系类型统计共用）。
type companionRel struct{ Src, Rel, Dst string }

// queryEntityRels 活跃关系边清单（SelectRelationEdges：失效边不返回；LIMIT 300 沿图读口径）。
func (s *Service) queryEntityRels(ctx context.Context, ontologyID string) ([]companionRel, error) {
	raw, err := s.graphQuery(ctx, ontologyID, SelectRelationEdges(ontologyID))
	if err != nil {
		return nil, err
	}
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil, nil
	}
	out := make([]companionRel, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		out = append(out, companionRel{Src: b["src"].Value, Rel: b["rel"].Value, Dst: b["dst"].Value})
	}
	return out, nil
}

// BuildCommunities 伴生图社区检测+摘要+持久化（重建全量替换；成员不变保留既有摘要）。
// 实体/关系取自伴生子图（活跃边）；connID 空=默认 chat 连接（资产侧重建无 agent 上下文，
// 诚实标注：不走伴生抽取连接，成本为「社区数 ≤12 时 N 次 LLM 调用」）。
func (s *Service) BuildCommunities(ctx context.Context, ontologyID string) (int, string, error) {
	if s == nil || s.Plans == nil {
		return 0, "", fmt.Errorf("伴生服务未装配")
	}
	defs := s.entityDefs(ctx, ontologyID)
	entities := make([]string, 0, len(defs))
	for l := range defs {
		entities = append(entities, l)
	}
	if len(entities) < 3 {
		return 0, "", fmt.Errorf("伴生图实体不足 3 个（当前 %d）——先通过对话沉淀更多概念", len(entities))
	}
	rels, err := s.queryEntityRels(ctx, ontologyID)
	if err != nil {
		return 0, "", err
	}
	pairs := make([][2]string, 0, len(rels))
	for _, r := range rels {
		pairs = append(pairs, [2]string{r.Src, r.Dst})
	}
	comms := kg.DetectCommunitiesLouvain(entities, pairs)
	detectMethod := "louvain"
	if len(comms) == 0 {
		comms = kg.DetectCommunities(entities, pairs)
		detectMethod = "label-propagation"
	}
	eager := len(comms) <= communityEagerLimit
	out := make([]*store.CompanionCommunity, 0, len(comms))
	for _, c := range comms {
		summary, method := "", "pending"
		if eager {
			summary, method = s.summarizeCommunity(ctx, "", c.Label, c.Members, rels)
		}
		out = append(out, &store.CompanionCommunity{OntologyID: ontologyID, Label: c.Label, Summary: summary, Method: method, Members: c.Members})
	}
	if _, err := s.Store.ReplaceCompanionCommunities(ontologyID, out); err != nil {
		return 0, detectMethod, err
	}
	return len(out), detectMethod, nil
}

// summarizeCommunity 单社区摘要（LLM 主路径失败回退骨架；签名对齐 kg.Summarizer 口径）。
func (s *Service) summarizeCommunity(ctx context.Context, connID, label string, members []string, rels []companionRel) (summary, method string) {
	relTypes := internalRelTypes(members, rels)
	skeleton := fmt.Sprintf("该社区以「%s」为核心，共 %d 个成员实体：%s；内部关系 %d 条（主要类型：%s）。",
		label, len(members), strings.Join(members, "、"), len(relTypes), strings.Join(relTypes, "、"))
	prompt := "以下是伴生本体（由对话沉淀的轻量知识图谱）中一个社区（一组紧密关联的实体）的成员与关系。请用 2~3 句话概括这个社区整体在讲什么，" +
		"回答面向全貌性问题（如「这个伴生本体整体沉淀了什么」）。\n成员实体：" + strings.Join(members, "、") +
		"\n内部关系类型：" + strings.Join(relTypes, "、") + "\n只输出 JSON。"
	res, err := chat.GenerateStructured(ctx, s.Store, s.Box, connID, prompt, communitySumSchema)
	if err != nil {
		return skeleton, "skeleton"
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(res.DraftJSON), &out); err != nil || strings.TrimSpace(out.Summary) == "" {
		return skeleton, "skeleton"
	}
	return strings.TrimSpace(out.Summary), "llm"
}

// internalRelTypes 社区内部关系类型去重清单（截 5；伴生边 relName 即关系名）。
func internalRelTypes(members []string, rels []companionRel) []string {
	set := map[string]bool{}
	for _, m := range members {
		set[m] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range rels {
		if set[r.Src] && set[r.Dst] && !seen[r.Rel] {
			seen[r.Rel] = true
			out = append(out, r.Rel)
			if len(out) >= 5 {
				break
			}
		}
	}
	return out
}

// EnsureCommunitySummary pending 社区按需生成（对话兜底/显式按钮首触发后缓存——KB-O4 口径）。
func (s *Service) EnsureCommunitySummary(ctx context.Context, ontologyID, label string) (string, string, error) {
	comms, err := s.Store.ListCompanionCommunities(ontologyID)
	if err != nil {
		return "", "", err
	}
	rels, _ := s.queryEntityRels(ctx, ontologyID)
	for _, c := range comms {
		if c.Label != label {
			continue
		}
		if c.Summary != "" && c.Method != "pending" {
			return c.Summary, c.Method, nil
		}
		summary, method := s.summarizeCommunity(ctx, "", label, c.Members, rels)
		if err := s.Store.UpdateCompanionCommunitySummary(ontologyID, label, summary, method); err != nil {
			return summary, method, err
		}
		return summary, method, nil
	}
	return "", "", fmt.Errorf("社区不存在: %s", label)
}

// CommunityFallback 检索零命中兜底（可选检索注入臂）：查询词与社区摘要/成员中文 2-gram
// 覆盖评分 TopN（零 embedding 依赖，沿 kg.GlobalSearch 口径）。无命中返回空。
func (s *Service) CommunityFallback(ctx context.Context, ontologyID, query string) ([]*store.CompanionCommunity, error) {
	comms, err := s.Store.ListCompanionCommunities(ontologyID)
	if err != nil || len(comms) == 0 || strings.TrimSpace(query) == "" {
		return nil, err
	}
	terms := localGramTerms(query)
	type scored struct {
		c     *store.CompanionCommunity
		score int
	}
	var cands []scored
	for _, c := range comms {
		if c.Summary == "" {
			continue
		}
		hay := strings.ToLower(c.Summary + " " + strings.Join(c.Members, " "))
		score := 0
		for _, t := range terms {
			if strings.Contains(hay, strings.ToLower(t)) {
				score++
			}
		}
		if score > 0 {
			cands = append(cands, scored{c, score})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > communityFallbackTopN {
		cands = cands[:communityFallbackTopN]
	}
	out := make([]*store.CompanionCommunity, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.c)
	}
	return out, nil
}

// localGramTerms 查询分词：ASCII 整词 ≥2 字符 + CJK 单字与相邻 2-gram（kg.gramTerms 的
// 伴生本地版，不引包内未导出符号；上限 16 短语）。
func localGramTerms(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var out []string
	var ascii []rune
	flush := func() {
		if len(ascii) >= 2 {
			out = append(out, string(ascii))
		}
		ascii = ascii[:0]
	}
	var cjk []string
	for _, r := range []rune(q) {
		switch {
		case r > 127:
			flush()
			cjk = append(cjk, string(r))
		case r == ' ' || r == '\t' || r == ',' || r == '，' || r == '。' || r == '?' || r == '？':
			flush()
		default:
			ascii = append(ascii, r)
		}
	}
	flush()
	out = append(out, cjk...) // CJK 单字
	for i := 0; i+1 < len(cjk); i++ {
		out = append(out, cjk[i]+cjk[i+1]) // 相邻 2-gram
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}
