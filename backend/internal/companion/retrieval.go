package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ---------------------------------------------------------------------------
// REQ-170 P2「KG 检索源并入」+ REQ-194/M34 批次一②召回增强（治 G1 词法召回恰是隐式召回失败）：
// 召回链——a) 向量主（embedding 余弦，REQ-287② 起 embed 语料=标签+定义）→ b) 词法兜底
// （recallEntities 双向包含）→ c) BM25/FTS5 兜底臂（REQ-287①，SQLite 投影惰性回填）；
// REQ-287③ 三臂 RRF 融合（k=60 沿 kb hybrid 先例）取代向量优先占坑式合并。
// 命中实体拉 2 跳邻域（bot:Relation 边节点两跳，每实体 ≤8 边，2 跳边标 hop=2）；
// retrieval 事件明细带 match 字段（vector/lexical/bm25 及「+」组合，诚实呈现召回来源）。
// 降级链：embedding/FTS5 未配置或失败 → 静默降级其余臂 + 日志（与 KB 召回同口径，不发 run.warning）；
// 全部臂空 = 空上下文不阻断主链路。chat.CompanionSource 接口与 SSE 契约零改动。
// ---------------------------------------------------------------------------

// maxRecallEntities 单次并入检索的命中实体上限（薄版口径：少而准，避免上下文淹没）。
const maxRecallEntities = 5

// vecMatchThreshold 向量召回相似度阈值（可调常量；经验起点 0.35，基准 evaldata 可校准）。
const vecMatchThreshold = 0.35

// maxEntityEdges 命中实体邻域边总量限流（1 跳优先，2 跳补位）。
const maxEntityEdges = 8

// REQ-287④ 缓存治理常量：embed 语料版本（定义并入=2，变更即全桶重建）、桶 TTL、单桶实体上限。
const (
	embedTextVersion  = 2
	vecCacheTTL       = 10 * time.Minute
	vecCacheMaxLabels = 2000
	rrfK              = 60.0 // REQ-287③ RRF 常数（沿 kb hybrid rrfK=60 先例）
)

// vecBucket 向量缓存桶（REQ-287④）：embed 文本随定义变化，版本不符或过期即整桶重建。
type vecBucket struct {
	version int
	textOf  map[string]string // label → embed 语料（label+"\n"+definition）
	vecs    map[string][]float32
	updated time.Time
}

// edgeRow 一条活跃关系边的渲染行（hop=2 为 2 跳链式边，relName 为拼链文本）。
type edgeRow struct {
	Rel   string `json:"rel"`
	Other string `json:"other"`
	Dir   string `json:"dir"` // out=主体→他者 | in=他者→主体
	Hop   int    `json:"hop,omitempty"`
}

// entityHit 命中实体明细（retrieval 事件与上下文渲染共用）。
type entityHit struct {
	Label       string    `json:"label"`
	Definition  string    `json:"definition,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`
	TimeScope   string    `json:"time_scope,omitempty"` // REQ-229②：事件时点提示
	Match       string    `json:"match,omitempty"`      // vector | lexical | bm25 及「+」组合（REQ-194②/287 如实呈现）
	HasEdgeInfo bool      `json:"-"`
	Edges       []edgeRow `json:"relations,omitempty"`
}

// RetrievalContext 实现 chat.CompanionSource（接口反转注入，companion→chat 包环约束）。
// 返回值：注入文本（空=无命中）、实体明细（retrieval 事件用）、错误（仅引擎通信失败）。
// REQ-216：召回作用域=绑定本体的伴生子图（同本体多 agent 共享沉淀——项目协作语义升维）。
func (s *Service) RetrievalContext(ctx context.Context, conv *store.Conversation, agent *store.Agent, input string) (string, []map[string]any, error) {
	if s == nil || s.Plans == nil || agent == nil || strings.TrimSpace(input) == "" {
		return "", nil, nil
	}
	ontID := effectiveBinding(agent, conv)
	if ontID == "" {
		return "", nil, nil
	}
	labels, err := s.queryLabels(ctx, ontID)
	if err != nil {
		return "", nil, err
	}
	defs := s.entityDefs(ctx, ontID)
	// a) 向量主召回（embedding 未配置/失败 → 静默降级其余臂 + 日志）
	vecHits, vecErr := s.vectorRecall(ctx, ontID, labels, defs, input)
	if vecErr != nil {
		log.Printf("[companion] 向量召回不可用，降级其余臂（本体 %s）: %v", ontID, vecErr)
	}
	// b) 词法兜底
	lexHits := recallEntities(labels, input)
	// c) BM25/FTS5 兜底臂（REQ-287①：SQLite 投影惰性回填；FTS 不可用静默降级）
	bm25Hits := s.bm25Recall(ontID, labels, defs, input)
	// REQ-287③：三臂 RRF 融合 + 别名回溯主实体（REQ-286 B3 空挂修复）
	hits, matchOf := fuseRecallRRF(vecHits, bm25Hits, lexHits)
	hits = s.resolveHitAliases(ctx, ontID, hits, defs)
	if len(hits) == 0 {
		// REQ-288 可选检索注入臂：实体零命中时社区摘要兜底——全貌性问题（「这个伴生本体整体
		// 在讲什么」级）常无实体词锚点，社区摘要是对症形态；pending 按需生成沿 KB-O4 成本口径
		comms, cerr := s.CommunityFallback(ctx, ontID, input)
		if cerr != nil || len(comms) == 0 {
			return "", nil, nil
		}
		var b strings.Builder
		b.WriteString("【伴生图检索】未命中具体实体，以下为伴生本体社区摘要（一组紧密关联实体的整体概括，供参考）：")
		out := make([]map[string]any, 0, len(comms))
		for _, c := range comms {
			fmt.Fprintf(&b, "\n- 社区「%s」（%d 个成员）：%s", c.Label, len(c.Members), c.Summary)
			out = append(out, map[string]any{"label": c.Label, "summary": c.Summary, "members": c.Members, "match": "community"})
		}
		return b.String(), out, nil
	}
	out := make([]entityHit, 0, len(hits))
	for _, label := range hits {
		h := entityHit{Label: label, Match: matchOf[label]}
		def, conf, tscope, err := s.queryEntityInfo(ctx, ontID, label)
		if err == nil {
			h.Definition, h.Confidence, h.TimeScope = def, conf, tscope
		}
		if edges, err := s.queryEntityNeighborhood(ctx, ontID, label); err == nil {
			h.Edges, h.HasEdgeInfo = edges, true
		}
		out = append(out, h)
	}
	return renderCompanionContext(out), entityDetails(out), nil
}

// entityDefs 实体定义投影（SelectEntityDefs；进程内缓存，失效同 vecCache——REQ-287②④）。
func (s *Service) entityDefs(ctx context.Context, ontologyID string) map[string]string {
	s.vecMu.Lock()
	if defs, ok := s.defCache[ontologyID]; ok {
		s.vecMu.Unlock()
		return defs
	}
	s.vecMu.Unlock()
	defs := map[string]string{}
	raw, err := s.graphQuery(ctx, ontologyID, SelectEntityDefs(ontologyID))
	if err == nil {
		var res struct {
			Results struct {
				Bindings []map[string]struct {
					Value string `json:"value"`
				} `json:"bindings"`
			} `json:"results"`
		}
		if json.Unmarshal(raw, &res) == nil {
			for _, b := range res.Results.Bindings {
				if l := strings.TrimSpace(b["label"].Value); l != "" {
					defs[l] = b["def"].Value
				}
			}
		}
	} else {
		log.Printf("[companion] 定义投影查询失败（FTS 语料降级仅标签）: %v", err)
	}
	s.vecMu.Lock()
	if s.defCache == nil {
		s.defCache = map[string]map[string]string{}
	}
	s.defCache[ontologyID] = defs
	s.vecMu.Unlock()
	return defs
}

// vectorRecall 向量召回：embed 语料=标签+定义（REQ-287②——定义是区分同义实体的关键信号），
// 标签向量（进程内桶缓存：版本+TTL+容量，图写入失效）与输入向量余弦 topK≤5 阈值 0.35。
func (s *Service) vectorRecall(ctx context.Context, ontologyID string, labels []string, defs map[string]string, input string) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	emb := &kb.Embedder{Store: s.Store, Box: s.Box}
	labelVecs, err := s.labelVectors(ctx, ontologyID, labels, defs, emb)
	if err != nil {
		return nil, err
	}
	inVec, err := emb.EmbedOne(ctx, input)
	if err != nil {
		return nil, err
	}
	type scored struct {
		label string
		sim   float64
	}
	var cands []scored
	for l, v := range labelVecs {
		if sim := cosineSim(inVec, v); sim >= vecMatchThreshold {
			cands = append(cands, scored{l, sim})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
	if len(cands) > maxRecallEntities {
		cands = cands[:maxRecallEntities]
	}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.label)
	}
	return out, nil
}

// labelVectors 标签批量向量（REQ-287④：桶版本/TTL/容量治理；缺失标签一次批量补齐后写缓存）。
// embed 语料=标签+定义（textOf），定义变更随图写入失效自然重建。
func (s *Service) labelVectors(ctx context.Context, ontologyID string, labels []string, defs map[string]string, emb *kb.Embedder) (map[string][]float32, error) {
	s.vecMu.Lock()
	b := s.vecCache[ontologyID]
	if b == nil || b.version != embedTextVersion || time.Since(b.updated) > vecCacheTTL {
		b = &vecBucket{version: embedTextVersion, textOf: map[string]string{}, vecs: map[string][]float32{}, updated: time.Now()}
		s.vecCache[ontologyID] = b
	}
	// 语料构建：全部标签进 textOf（容量上限截断——超限实体不参与向量召回，log 一次语义由调用方日志承载）
	for _, l := range labels {
		if _, ok := b.textOf[l]; ok {
			continue
		}
		if len(b.textOf) >= vecCacheMaxLabels {
			break
		}
		text := l
		if d := defs[l]; d != "" {
			text = l + "\n" + d
		}
		b.textOf[l] = text
	}
	var missing []string
	for l := range b.textOf {
		if _, ok := b.vecs[l]; !ok {
			missing = append(missing, l)
		}
	}
	s.vecMu.Unlock()
	if len(missing) > 0 {
		sort.Strings(missing) // 确定性批量顺序
		vecs, err := emb.EmbedTexts(ctx, missing)
		if err != nil {
			return nil, err
		}
		s.vecMu.Lock()
		// 失效竞态防护：若期间缓存被图写入清空，仍以本次结果重建（标签随后会再失效）
		cur := s.vecCache[ontologyID]
		if cur == nil || cur.version != embedTextVersion {
			cur = &vecBucket{version: embedTextVersion, textOf: map[string]string{}, vecs: map[string][]float32{}, updated: time.Now()}
			s.vecCache[ontologyID] = cur
		}
		for i, l := range missing {
			if i < len(vecs) {
				cur.vecs[l] = vecs[i]
			}
		}
		cur.updated = time.Now()
		s.vecMu.Unlock()
	}
	s.vecMu.Lock()
	defer s.vecMu.Unlock()
	out := make(map[string][]float32, len(labels))
	for _, l := range labels {
		if v, ok := s.vecCache[ontologyID].vecs[l]; ok {
			out[l] = v
		}
	}
	return out, nil
}

// cosineSim 余弦相似度（维度不齐按短边截断；零向量返回 0）。
func cosineSim(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// bm25Recall REQ-287①：FTS5 兜底臂——投影（label+别名+定义）惰性回填（桶空时）后 BM25 检索。
// 别名与主标签各自成文档（别名命中后由 resolveHitAliases 回溯主实体）；FTS 不可用/无 Store 返回 nil。
func (s *Service) bm25Recall(ontologyID string, labels []string, defs map[string]string, input string) []string {
	if s == nil || s.Store == nil || strings.TrimSpace(input) == "" || len(labels) == 0 {
		return nil
	}
	n, err := s.Store.CompanionFTSCount(ontologyID)
	if err == nil && n == 0 {
		docs := make([]store.CompanionFTSDoc, 0, len(labels))
		for _, l := range labels {
			content := l
			if d := defs[l]; d != "" {
				content = l + "\n" + d
			}
			docs = append(docs, store.CompanionFTSDoc{Label: l, Content: content})
		}
		if err := s.Store.ReplaceCompanionFTS(ontologyID, docs); err != nil {
			log.Printf("[companion] FTS 投影回填失败（降级其余臂）: %v", err)
			return nil
		}
	}
	hits, err := s.Store.SearchCompanionFTS(ontologyID, store.FTSMatchQuery(input), maxRecallEntities)
	if err != nil {
		// 投影陈旧（实体删除后 MATCH 无果不进此路径）或 FTS 不可用——静默降级
		return nil
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Label)
	}
	return out
}

// resolveHitAliases 别名回溯主实体（REQ-286 B3 resolveAliasOwner 空挂修复）：命中别名时
// 以主标签替换（match 保留原臂标记）；defs 投影缺失（查询失败）时跳过回溯不阻断。
func (s *Service) resolveHitAliases(ctx context.Context, ontologyID string, hits []string, defs map[string]string) []string {
	if defs == nil || len(hits) == 0 {
		return hits
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		target := h
		if _, isPrimary := defs[h]; !isPrimary {
			if owner := s.resolveAliasOwner(ctx, ontologyID, h); owner != "" {
				target = owner
			}
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
		if len(out) >= maxRecallEntities {
			break
		}
	}
	return out
}

// fuseRecallRRF REQ-287③：三臂 RRF 融合（纯函数）——score=Σ 1/(k+rank)，k=60（kb hybrid 先例）；
// match 标记=命中臂并集按固定序拼接（vector/lexical/bm25）。取代原「向量优先占坑」合并。
func fuseRecallRRF(vecHits, bm25Hits, lexHits []string) ([]string, map[string]string) {
	arms := []struct {
		name string
		hits []string
	}{{"vector", vecHits}, {"lexical", lexHits}, {"bm25", bm25Hits}}
	score := map[string]float64{}
	armOf := map[string]map[string]bool{}
	for _, arm := range arms {
		for rank, l := range arm.hits {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			score[l] += 1 / (rrfK + float64(rank+1))
			if armOf[l] == nil {
				armOf[l] = map[string]bool{}
			}
			armOf[l][arm.name] = true
		}
	}
	type scored struct {
		label string
		score float64
	}
	cands := make([]scored, 0, len(score))
	for l, sc := range score {
		cands = append(cands, scored{l, sc})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].label < cands[j].label // 同分字典序：结果不依赖 map 迭代序
	})
	if len(cands) > maxRecallEntities {
		cands = cands[:maxRecallEntities]
	}
	order := []string{"vector", "lexical", "bm25"}
	matchOf := map[string]string{}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		var parts []string
		for _, a := range order {
			if armOf[c.label][a] {
				parts = append(parts, a)
			}
		}
		matchOf[c.label] = strings.Join(parts, "+")
		out = append(out, c.label)
	}
	return out, matchOf
}

// resolveAliasOwner REQ-286 B3：别名 → 主实体标签（非别名返回空串）。
func (s *Service) resolveAliasOwner(ctx context.Context, ontID, alias string) string {
	raw, err := s.graphQuery(ctx, ontID, SelectAliasOwner(ontID, alias))
	if err != nil {
		return ""
	}
	owners := parseLabelValues(raw)
	if len(owners) > 0 {
		return owners[0]
	}
	return ""
}

// queryLabels 本体伴生子图实体标签清单（REQ-216：读侧兜底拉起——graphQuery 内 Ensure
// 确保宿主方案 running；宿主方案不可达返回空，不阻断主链路）。
func (s *Service) queryLabels(ctx context.Context, ontologyID string) ([]string, error) {
	raw, err := s.graphQuery(ctx, ontologyID, SelectLabels(ontologyID))
	if err != nil {
		if strings.Contains(err.Error(), "未绑定伴生本体") {
			return nil, nil
		}
		log.Printf("[companion] 伴生图标签查询失败（按空召回）: %v", err)
		return nil, nil
	}
	return parseLabelValues(raw), nil
}

func (s *Service) queryEntityInfo(ctx context.Context, ontologyID, label string) (string, float64, string, error) {
	raw, err := s.graphQuery(ctx, ontologyID, SelectEntityInfo(ontologyID, label))
	if err != nil {
		return "", 0, "", err
	}
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &res) != nil || len(res.Results.Bindings) == 0 {
		return "", 0, "", nil
	}
	b := res.Results.Bindings[0]
	conf := 0.0
	if v, ok := b["conf"]; ok {
		fmt.Sscanf(v.Value, "%f", &conf)
	}
	return b["def"].Value, conf, b["tscope"].Value, nil
}

// queryEntityNeighborhood 命中实体 2 跳邻域（1 跳优先，2 跳补位；每实体限流 ≤8 边）。
func (s *Service) queryEntityNeighborhood(ctx context.Context, ontologyID, label string) ([]edgeRow, error) {
	raw, err := s.graphQuery(ctx, ontologyID, SelectEntityNeighborhood(ontologyID, label))
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
	edges := make([]edgeRow, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		hop := 1
		if v, ok := b["hop"]; ok {
			fmt.Sscanf(v.Value, "%d", &hop)
		}
		if len(edges) >= maxEntityEdges {
			break
		}
		edges = append(edges, edgeRow{Rel: b["relName"].Value, Other: b["otherLabel"].Value, Dir: b["dir"].Value, Hop: hop})
	}
	return edges, nil
}

// recallEntities 标签→输入包含匹配（双向包含，短标签 <2 字符跳过防误召），保序去重限量。
func recallEntities(labels []string, input string) []string {
	lc := strings.ToLower(strings.TrimSpace(input))
	if lc == "" {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, maxRecallEntities)
	for _, l := range labels {
		t := strings.TrimSpace(l)
		if len([]rune(t)) < 2 || seen[t] {
			continue
		}
		lt := strings.ToLower(t)
		if strings.Contains(lc, lt) || strings.Contains(lt, lc) && len([]rune(lc)) >= 4 {
			seen[t] = true
			out = append(out, t)
			if len(out) >= maxRecallEntities {
				break
			}
		}
	}
	return out
}

// renderCompanionContext 检索上下文渲染（纯函数；System 注入口径对齐 kb.RenderContext）。
// REQ-194②：match 来源与 2 跳边如实标注（诚实原则——LLM 与人看到同一份事实来源）。
func renderCompanionContext(hits []entityHit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("【伴生图检索】以下内容来自与本智能体的历史对话（含项目协作）中确认生成的伴生轻量本体（供参考，非权威知识）：")
	for _, h := range hits {
		fmt.Fprintf(&b, "\n- 实体「%s」", h.Label)
		if h.Match != "" {
			fmt.Fprintf(&b, "（%s 召回）", matchLabel(h.Match))
		}
		if h.Definition != "" {
			fmt.Fprintf(&b, "：%s", h.Definition)
		}
		if h.TimeScope != "" {
			fmt.Fprintf(&b, "（时点：%s）", h.TimeScope)
		}
		for _, e := range h.Edges {
			hopTag := ""
			if e.Hop == 2 {
				hopTag = "（2跳）"
			}
			if e.Dir == "out" {
				fmt.Fprintf(&b, "；「%s」→ %s%s", e.Rel, e.Other, hopTag)
			} else {
				fmt.Fprintf(&b, "；%s →「%s」%s", e.Other, e.Rel, hopTag)
			}
		}
	}
	return b.String()
}

// matchLabel match 值 → 中文标注（事件与上下文共用）；REQ-287 起 match 为「+」组合，逐段映射。
func matchLabel(m string) string {
	parts := strings.Split(m, "+")
	mapped := make([]string, 0, len(parts))
	for _, p := range parts {
		switch p {
		case "vector":
			mapped = append(mapped, "向量")
		case "lexical":
			mapped = append(mapped, "词法")
		case "bm25":
			mapped = append(mapped, "全文")
		default:
			mapped = append(mapped, p)
		}
	}
	return strings.Join(mapped, "+")
}

// entityDetails 实体明细 → retrieval 事件数据（map 切片，chat 层透传；含 match/hop 如实呈现）。
func entityDetails(hits []entityHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		rels := make([]map[string]any, 0, len(h.Edges))
		for _, e := range h.Edges {
			rels = append(rels, map[string]any{"rel": e.Rel, "other": e.Other, "dir": e.Dir, "hop": e.Hop})
		}
		out = append(out, map[string]any{"label": h.Label, "definition": h.Definition, "confidence": h.Confidence, "time_scope": h.TimeScope, "match": h.Match, "relations": rels})
	}
	return out
}

// parseLabelValues 从 SelectLabels 结果提取 label 值数组。
func parseLabelValues(raw []byte) []string {
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	out := make([]string, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		if v, ok := b["label"]; ok {
			out = append(out, v.Value)
		}
	}
	return out
}

// labelOnlyVectors REQ-286 对齐/关联比对专用（quality.go 消费）：仅标签向量、独立缓存空间——
// 与检索的定义增广向量（labelVectors）语义互不干扰（0.8 归并阈值按纯标签相似度标定）。
// 原缓存结构原样保留（REQ-287④ 治理仅作用于检索桶）。
func (s *Service) labelOnlyVectors(ctx context.Context, ontologyID string, labels []string, emb *kb.Embedder) (map[string][]float32, error) {
	s.labelVecMu.Lock()
	cache := s.labelVecCache[ontologyID]
	if cache == nil {
		cache = map[string][]float32{}
		s.labelVecCache[ontologyID] = cache
	}
	var missing []string
	for _, l := range labels {
		if _, ok := cache[l]; !ok {
			missing = append(missing, l)
		}
	}
	s.labelVecMu.Unlock()
	if len(missing) > 0 {
		vecs, err := emb.EmbedTexts(ctx, missing)
		if err != nil {
			return nil, err
		}
		s.labelVecMu.Lock()
		cur := s.labelVecCache[ontologyID]
		if cur == nil {
			cur = map[string][]float32{}
			s.labelVecCache[ontologyID] = cur
		}
		for i, l := range missing {
			if i < len(vecs) {
				cur[l] = vecs[i]
			}
		}
		s.labelVecMu.Unlock()
	}
	s.labelVecMu.Lock()
	defer s.labelVecMu.Unlock()
	out := make(map[string][]float32, len(labels))
	for _, l := range labels {
		if v, ok := s.labelVecCache[ontologyID][l]; ok {
			out[l] = v
		}
	}
	return out, nil
}
