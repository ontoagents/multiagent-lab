package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"sync"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// ---------------------------------------------------------------------------
// REQ-286/M91 伴生图质量三维度 + REQ-297/M101 挖掘异步化：
// 关系密度（A1 prompt/A2 挖掘补抽〔REQ-297 起异步 job+分批+进度〕/A3 属类规则/A4 前端提示）、
// 一致性质量（B1 label 规范化/B3 bot:alias 别名/B2 入图向量归并/B4 人工合并）、
// 可治理性（C1 内容清单行级编辑）。SPARQL 原语在 sparql.go。
// ---------------------------------------------------------------------------

// genusPattern A3 属类模式（零 LLM）：定义含「是（一）种/属于」类属陈述 → 低置信上下位关系建议。
var genusPattern = regexp.MustCompile(`^(?:[^，。；]{0,60}?)是(?:一[种个类]|[种个类])?(.+?)[。；]?$`)

// NormalizeLabel B1：label 规范化——返回（规范名, 变体别名列表）。
// 规则：trim → 全角空格转半角 → 连续空白折叠 → 括号备注提取（中英文括号，备注内容转别名）。
// 不做 ASCII 大小写改写（「K8s」等混排名保留原样）；大小写折叠只在 normalizeKey 比对层。
func NormalizeLabel(label string) (string, []string) {
	s := strings.TrimSpace(label)
	s = strings.ReplaceAll(s, "　", " ")
	s = strings.Join(strings.Fields(s), " ")
	var aliases []string
	main := s
	for {
		runes := []rune(main)
		openIdx := -1
		closeIdx := -1
		for i := len(runes) - 1; i >= 0; i-- {
			if runes[i] == ')' || runes[i] == '）' {
				closeIdx = i
				for j := i - 1; j >= 0; j-- {
					if runes[j] == '(' || runes[j] == '（' {
						openIdx = j
						break
					}
				}
				break
			}
		}
		if openIdx < 0 || closeIdx <= openIdx {
			break
		}
		note := strings.TrimSpace(string(runes[openIdx+1 : closeIdx]))
		main = strings.TrimSpace(string(runes[:openIdx]) + string(runes[closeIdx+1:]))
		if note != "" && note != main {
			aliases = append(aliases, note)
		}
	}
	if main == "" {
		main = s
	}
	return main, aliases
}

// normalizeKey B1 比对键：ASCII 小写折叠+空白/全角归一（不做 CJK 折叠）——
// 「POD」「Pod」「pod」同键；「K8s」与「k8s」同键。
func normalizeKey(s string) string {
	main, _ := NormalizeLabel(s)
	var b strings.Builder
	for _, r := range main {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 'a' - 'A')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// resolvedNode 归并解析结果：canonical=入图规范名（已有实体标签或规范化新名），
// aliases=需挂接的别名，mergedFrom=非空时表示并入已有实体（B1 key 命中 / B2 向量相似）。
type resolvedNode struct {
	Canonical  string
	Aliases    []string
	MergedFrom string
}

// ResolveNode B1+B2：候选标签 → 入图规范名解析。
// ① normalizeKey 与图内已有实体标签比对（确定性，含候选别名命中）；② 未命中走向量归并
// （≥0.8 保守阈值，embedding 未配置/失败静默降级）；③ 均未命中 = 规范化新名 + 括号备注转别名。
func (s *Service) ResolveNode(ctx context.Context, ontID, label string) (resolvedNode, error) {
	main, aliases := NormalizeLabel(label)
	res := resolvedNode{Canonical: main, Aliases: aliases}
	labels := s.knownEntityLabels(ctx, ontID)
	if len(labels) == 0 {
		return res, nil
	}
	byKey := map[string]string{}
	for _, l := range labels {
		if l == "" {
			continue
		}
		byKey[normalizeKey(l)] = l
	}
	for _, cand := range append([]string{main}, res.Aliases...) {
		if owner, ok := byKey[normalizeKey(cand)]; ok {
			res.Canonical = owner
			res.MergedFrom = label
			if cand != owner {
				res.Aliases = append(res.Aliases, cand)
			}
			return res, nil
		}
	}
	if s.Box != nil {
		emb := &kb.Embedder{Store: s.Store, Box: s.Box}
		if inVec, err := emb.EmbedOne(ctx, main); err == nil && len(inVec) > 0 {
			labelVecs, lerr := s.labelOnlyVectors(ctx, ontID, labels, emb)
			if lerr == nil {
				best, bestSim := "", 0.0
				for l, v := range labelVecs {
					if sim := cosineSim(inVec, v); sim > bestSim {
						best, bestSim = l, sim
					}
				}
				if best != "" && bestSim >= 0.8 {
					res.Canonical = best
					res.MergedFrom = label
					if main != best {
						res.Aliases = append(res.Aliases, main)
					}
					res.Aliases = append(res.Aliases, aliases...)
					return res, nil
				}
			}
		} else if err != nil {
			log.Printf("[companion] B2 向量归并不可用（降级 key 比对）: %v", err)
		}
	}
	return res, nil
}

// attachAliases B3：别名挂接（bot:alias 三元组；失败仅日志不阻断入图）。
func (s *Service) attachAliases(ctx context.Context, ontID, canonical string, aliases []string) {
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" || a == canonical || normalizeKey(a) == normalizeKey(canonical) {
			continue
		}
		if err := s.graphUpdate(ctx, ontID, AddAlias(ontID, canonical, a)); err != nil {
			log.Printf("[companion] 别名挂接失败（%s → %s）: %v", a, canonical, err)
		}
	}
}

// ScanGenusCandidates A3 属类模式规则（零 LLM）：概念定义含「…是（一）种…」类属陈述
// → 追加低置信「属于」关系候选（target=类属对象文本，薄建端点既有机制承接）。
func ScanGenusCandidates(cands []*store.CompanionCandidate) []*store.CompanionCandidate {
	var out []*store.CompanionCandidate
	seen := map[string]bool{}
	for _, c := range cands {
		if c.Kind != "concept" {
			continue
		}
		m := genusPattern.FindStringSubmatch(strings.TrimSpace(c.Definition))
		if m == nil {
			continue
		}
		genus := strings.TrimSpace(m[1])
		if genus == "" || normalizeKey(genus) == normalizeKey(c.Name) {
			continue
		}
		key := normalizeKey(c.Name) + "|" + normalizeKey(genus)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, &store.CompanionCandidate{
			Kind: "relation", Name: c.Name, RelName: "属于", RelTarget: truncate(genus, 120),
			Definition: "类属关系（模式规则建议）：" + truncate(c.Definition, 160),
			Confidence: 0.35, ConversationID: c.ConversationID, AgentID: c.AgentID,
			SourceMessageID: c.SourceMessageID, SourceExcerpt: truncate(c.Definition, 120),
			Status: "pending", Note: "模式规则建议（A3）：定义含类属陈述",
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// REQ-297/M101 A2 异步化：关系挖掘异步 job（内存注册表——与抽取 worker 同等持久性，
// 进程重启丢 job 前端轮询 404 提示重新触发）+ 分批（≤40 实体/批独立 LLM 调用）+ 进度透出。
// 治 REQ-286 交付轮同步接口两病：①HTTP ctx 绑定 LLM（前端断开→白烧）②实体清单无上限。
// ---------------------------------------------------------------------------

// MineJob 关系挖掘异步 job 状态。
type MineJob struct {
	ID         string    `json:"id"`
	OntologyID string    `json:"ontology_id"`
	Phase      string    `json:"phase"` // running | done | error
	Stage      string    `json:"stage"`
	BatchDone  int       `json:"batch_done"`
	BatchTotal int       `json:"batch_total"`
	Candidates int       `json:"candidates"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// mineBatchSize 分批挖掘批尺寸。
const mineBatchSize = 40

func (s *Service) mineJobUpdate(jobID string, fn func(j *MineJob)) {
	s.mineMu.Lock()
	defer s.mineMu.Unlock()
	if j, ok := s.mineJobs[jobID]; ok {
		fn(j)
	}
}

// GetMineJob job 状态查询（无记录返回 nil——前端按「任务已失效」提示重新触发）。
func (s *Service) GetMineJob(jobID string) *MineJob {
	s.mineMu.Lock()
	defer s.mineMu.Unlock()
	return s.mineJobs[jobID]
}

// mineSchema 关系挖掘输出契约（与抽取 relations 同形）。
const mineSchema = `{
  "type": "object",
  "properties": {
    "relations": {"type": "array", "items": {"type": "object", "properties": {
      "rel_name": {"type": "string"}, "source": {"type": "string"}, "target": {"type": "string"},
      "definition": {"type": "string"}, "confidence": {"type": "number"}},
      "required": ["rel_name", "source", "target"]}}
  },
  "required": ["relations"]
}`

// mineOut 关系挖掘输出。
type mineOut struct {
	Relations []struct {
		RelName    string  `json:"rel_name"`
		Source     string  `json:"source"`
		Target     string  `json:"target"`
		Definition string  `json:"definition"`
		Confidence float64 `json:"confidence"`
	} `json:"relations"`
}

// mineRelationsPrompt A2：关系挖掘提示词——实体清单（label：definition）+ 关系类型清单
// （A1 同源）+ 硬性标准（端点必须引用清单中的名称/禁自指）。
func mineRelationsPrompt(entities []GraphNode) string {
	var b strings.Builder
	b.WriteString("你是本体关系挖掘助手。以下是伴生本体图中已沉淀的实体清单（label：定义），")
	b.WriteString("请基于领域常识与它们的定义，挖掘实体之间「值得沉淀的语义关系」。\n")
	b.WriteString("关系类型参考（不限于）：上下位（属于/是一种）、组成、依赖、引发、适用于、前置、对比、协同。\n")
	b.WriteString("要求：\n")
	b.WriteString("1. relations 的 source/target 必须逐字引用清单中已有实体的 label；禁止自指；\n")
	b.WriteString("2. 只输出有实质语义依据的关系（定义可互证或领域常识强支持），宁缺毋滥；\n")
	b.WriteString("3. rel_name 用动名词短语；definition 一句话说明依据；confidence 0~1。\n")
	b.WriteString("只输出 JSON。实体清单：\n")
	for _, n := range entities {
		if n.Kind == "Relation" {
			continue
		}
		if n.Definition != "" {
			fmt.Fprintf(&b, "- %s：%s\n", n.Label, truncate(n.Definition, 120))
		} else {
			fmt.Fprintf(&b, "- %s\n", n.Label)
		}
	}
	return b.String()
}

// MineRelationsAsync A2（REQ-297 异步化）：注册 job 并后台分批挖掘，立即返回 jobID。
// 本体级单飞（runningMine）。
func (s *Service) MineRelationsAsync(ctx context.Context, ontID string) (string, error) {
	agents, err := s.Store.ListAgentsByCompanionOntology(ontID)
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "", &store.HTTPError{Status: 400, Msg: "该本体暂无绑定智能体——请先在智能体侧板绑定伴生本体"}
	}
	s.mineMu.Lock()
	if s.runningMine[ontID] {
		s.mineMu.Unlock()
		return "", &store.HTTPError{Status: 409, Msg: "该本体已有关系挖掘任务进行中——请稍候"}
	}
	job := &MineJob{ID: store.NewID(), OntologyID: ontID, Phase: "running", Stage: "排队中", CreatedAt: time.Now()}
	s.mineJobs[job.ID] = job
	s.runningMine[ontID] = true
	s.mineMu.Unlock()
	go func() {
		defer func() {
			s.mineMu.Lock()
			delete(s.runningMine, ontID)
			s.mineMu.Unlock()
			if r := recover(); r != nil {
				log.Printf("[companion] 关系挖掘 panic（%s）: %v", ontID, r)
				s.mineJobUpdate(job.ID, func(jj *MineJob) { jj.Phase, jj.Error = "error", fmt.Sprintf("内部错误: %v", r) })
			}
		}()
		// 独立超时：与 HTTP 请求脱钩（前端断开不中止——治白烧）
		mctx, cancel := context.WithTimeout(context.Background(), extractTimeout())
		defer cancel()
		n, err := s.runMineBatches(mctx, ontID, agents[0], job.ID)
		s.mineJobUpdate(job.ID, func(jj *MineJob) {
			if err != nil {
				jj.Phase = "error"
				jj.Error = fmt.Sprintf("%s（已完成批次产出 %d 条候选已保留）", err.Error(), jj.Candidates)
			} else {
				jj.Phase = "done"
				jj.Stage = fmt.Sprintf("完成：共产出 %d 条候选待确认", n)
			}
		})
	}()
	return job.ID, nil
}

// runMineBatches 分批挖掘执行体（读实体→并入 pending 候选→分批 LLM→逐批落库+进度更新；
// 中途失败已完成批次的候选保留）。
func (s *Service) runMineBatches(ctx context.Context, ontID string, agent *store.Agent, jobID string) (int, error) {
	s.mineJobUpdate(jobID, func(jj *MineJob) { jj.Stage = "读取图内实体…" })
	connID := extractConnID(agent)
	raw, err := s.graphQuery(ctx, ontID, SelectNodes(ontID))
	if err != nil {
		return 0, err
	}
	entities := collectEntityList(raw)
	// REQ-296 E3：pending 候选纳入挖掘清单（同名 key 已有实体优先）
	pending, _ := s.Store.ListCompanionCandidatesByOntology(ontID, "pending")
	pendSeen := map[string]bool{}
	known := s.knownEntityLabels(ctx, ontID)
	for _, l := range known {
		pendSeen[normalizeKey(l)] = true
	}
	for _, c := range pending {
		if c.Kind == "relation" {
			continue
		}
		main, _ := NormalizeLabel(c.Name)
		if main == "" || pendSeen[normalizeKey(main)] {
			continue
		}
		pendSeen[normalizeKey(main)] = true
		entities = append(entities, GraphNode{Label: main, Kind: "Concept", Definition: c.Definition})
	}
	if len(entities) < 2 {
		return 0, &store.HTTPError{Status: 400, Msg: "图内实体不足 2 个——先通过对话沉淀更多概念"}
	}
	batches := (len(entities) + mineBatchSize - 1) / mineBatchSize
	s.mineJobUpdate(jobID, func(jj *MineJob) {
		jj.BatchTotal, jj.Stage = batches, fmt.Sprintf("挖掘关系批次 0/%d…", batches)
	})
	knownAligned := append([]string{}, known...)
	pendingNames := make([]string, 0, len(pending))
	for _, c := range pending {
		if c.Kind != "relation" {
			pendingNames = append(pendingNames, c.Name)
		}
	}
	total := 0
	for bi := 0; bi < batches; bi++ {
		lo, hi := bi*mineBatchSize, (bi+1)*mineBatchSize
		if hi > len(entities) {
			hi = len(entities)
		}
		batch := entities[lo:hi]
		res, err := chat.GenerateStructured(ctx, s.Store, s.Box, connID, mineRelationsPrompt(batch), mineSchema)
		if err != nil {
			if total > 0 {
				return total, fmt.Errorf("关系挖掘第 %d/%d 批失败", bi+1, batches)
			}
			return 0, fmt.Errorf("关系挖掘 LLM 调用失败: %w", err)
		}
		// REQ-300：解析容错——数组根（多块）合并
		var out mineOut
		if err := json.Unmarshal(res.DraftJSON, &out); err != nil {
			var arr []json.RawMessage
			if err2 := json.Unmarshal(res.DraftJSON, &arr); err2 != nil {
				return total, fmt.Errorf("关系挖掘输出解析失败（第 %d/%d 批）: %w", bi+1, batches, err)
			}
			for _, el := range arr {
				var part mineOut
				if json.Unmarshal(el, &part) == nil {
					out.Relations = append(out.Relations, part.Relations...)
				}
			}
		}
		// 端点对齐面：已有实体+pending 候选名+跨批实体（REQ-296 E3）
		align := append(knownAligned, pendingNames...)
		cands := pruneRelationsOnly(mineToCandidates(ontID, agent.ID, &out, align))
		if len(cands) > 0 {
			if err := s.Store.CreateCompanionCandidates(cands); err != nil {
				return total, err
			}
			total += len(cands)
		}
		done := bi + 1
		s.mineJobUpdate(jobID, func(jj *MineJob) {
			jj.BatchDone, jj.Candidates = done, total
			jj.Stage = fmt.Sprintf("挖掘关系批次 %d/%d（已产出 %d 条）", done, batches, total)
		})
		for _, n := range batch {
			knownAligned = append(knownAligned, n.Label)
		}
	}
	s.audit("companion_mine_relations", ontID, fmt.Sprintf("伴生关系挖掘：分 %d 批产出 %d 条候选", batches, total), map[string]any{"ontology_id": ontID, "entities": len(entities)})
	if base, berr := s.Plans.EnsureHostPlan(ctx, ontID); berr == nil {
		s.snapshotRefresh(ctx, ontID, base)
	}
	return total, nil
}

// mineToCandidates 挖掘输出 → relation 候选（端点 key 对齐已有实体/候选名，未知名薄建既有机制承接）。
func mineToCandidates(ontID, agentID string, out *mineOut, known []string) []*store.CompanionCandidate {
	byKey := map[string]string{}
	for _, l := range known {
		byKey[normalizeKey(l)] = l
	}
	canonical := func(label string) string {
		if owner, ok := byKey[normalizeKey(label)]; ok {
			return owner
		}
		main, _ := NormalizeLabel(label)
		return main
	}
	var cands []*store.CompanionCandidate
	for _, r := range out.Relations {
		if strings.TrimSpace(r.RelName) == "" || strings.TrimSpace(r.Source) == "" || strings.TrimSpace(r.Target) == "" {
			continue
		}
		if normalizeKey(r.Source) == normalizeKey(r.Target) {
			continue // 自指
		}
		cands = append(cands, &store.CompanionCandidate{
			Kind: "relation", Name: truncate(canonical(r.Source), 120), RelName: truncate(r.RelName, 120),
			RelTarget: truncate(canonical(r.Target), 120), Definition: truncate(r.Definition, 500),
			Confidence: r.Confidence, ConversationID: "", AgentID: agentID,
			SourceExcerpt: "关系挖掘补全（REQ-286 A2）", Status: "pending",
		})
	}
	return cands
}

// pruneRelationsOnly 挖掘批自检：批内去重（同主体+关系名+客体）与端点相同剔除。
func pruneRelationsOnly(cands []*store.CompanionCandidate) []*store.CompanionCandidate {
	var out []*store.CompanionCandidate
	seen := map[string]bool{}
	for _, c := range cands {
		key := normalizeKey(c.Name) + "|" + normalizeKey(c.RelName) + "|" + normalizeKey(c.RelTarget)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

// parseGraphNodes SelectNodes SPARQL JSON → GraphNode 切片（挖掘/编辑共用）。
func parseGraphNodes(raw []byte) []GraphNode {
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
	out := make([]GraphNode, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		kind := strings.TrimPrefix(b["kind"].Value, BotNS)
		conf := 0.0
		fmt.Sscanf(b["conf"].Value, "%f", &conf)
		out = append(out, GraphNode{Kind: kind, Label: b["label"].Value, Definition: b["def"].Value, Confidence: conf, TimeScope: b["tscope"].Value, CreatedAt: b["at"].Value})
	}
	return out
}

// collectEntityList 从 SelectNodes 原始响应提取实体（概念/事件）label+definition 清单。
func collectEntityList(raw []byte) []GraphNode {
	nodes := parseGraphNodes(raw)
	out := make([]GraphNode, 0, len(nodes))
	for _, n := range nodes {
		if n.Kind == "Relation" {
			continue
		}
		out = append(out, n)
	}
	return out
}

// ---------------------------------------------------------------------------
// C1 内容清单行级编辑 + B4 人工合并（本体视角 API；编辑动作落 onto_decision 审计+快照刷新）。
// ---------------------------------------------------------------------------

// snapshotAfterEdit 编辑后快照同步刷新（失败仅日志）。
func (s *Service) snapshotAfterEdit(ctx context.Context, ontID string) {
	if base, err := s.Plans.EnsureHostPlan(ctx, ontID); err == nil {
		s.snapshotRefresh(ctx, ontID, base)
	} else {
		log.Printf("[companion] 编辑后快照刷新失败（%s）: %v", ontID, err)
	}
}

// EditEntity C1：实体编辑——newLabel 非空=重命名（迁移式：新 URI 复制+边重定向+别名带旧名）；
// newDefinition 非空=改定义。二者可同批。
func (s *Service) EditEntity(ctx context.Context, ontID, label, newLabel, newDefinition string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return &store.HTTPError{Status: 400, Msg: "label 必填"}
	}
	// 改定义：先删后插（SPARQL Update 复合）
	if d := strings.TrimSpace(newDefinition); d != "" {
		e := EntityURI(label)
		g := GraphURI(ontID)
		if err := s.graphUpdate(ctx, ontID, fmt.Sprintf(`PREFIX bot: <%s>
DELETE { GRAPH <%s> { <%s> bot:definition ?any } }
INSERT { GRAPH <%s> { <%s> bot:definition %q } }
WHERE { GRAPH <%s> { OPTIONAL { <%s> bot:definition ?any } } }`, BotNS, g, e, g, e, turtleEscape(d), g, e)); err != nil {
			return err
		}
	}
	if nl := strings.TrimSpace(newLabel); nl != "" && nl != label {
		// 大小写/空白差异允许（POD→Pod 正是典型规范化诉求）——迁移后旧名转别名
		if err := s.runGraphSteps(ctx, ontID, RenameEntity(ontID, label, nl)); err != nil {
			return fmt.Errorf("重命名失败: %w", err)
		}
		s.invalidateLabelCache(ontID)
		s.audit("companion_edit_entity", ontID, fmt.Sprintf("伴生实体重命名：%s → %s", label, nl), map[string]any{"ontology_id": ontID, "old_label_as_alias": label})
	} else {
		s.audit("companion_edit_entity", ontID, fmt.Sprintf("伴生实体改定义：%s", label), map[string]any{"ontology_id": ontID})
	}
	s.snapshotAfterEdit(ctx, ontID)
	return nil
}

// DeleteEntity C1：删除实体（主体三元组+关联边整边删除；快照刷新）。
func (s *Service) DeleteEntity(ctx context.Context, ontID, label string) error {
	if err := s.runGraphSteps(ctx, ontID, DeleteEntity(ontID, strings.TrimSpace(label))); err != nil {
		return err
	}
	s.invalidateLabelCache(ontID)
	s.audit("companion_delete_entity", ontID, fmt.Sprintf("伴生实体删除：%s", label), map[string]any{"ontology_id": ontID})
	s.snapshotAfterEdit(ctx, ontID)
	return nil
}

// AddRelationManual C1：补关系（选两端实体+关系名→INSERT 边；端点须为图内已有实体）。
func (s *Service) AddRelationManual(ctx context.Context, ontID, source, relName, target, definition string) error {
	source, relName, target = strings.TrimSpace(source), strings.TrimSpace(relName), strings.TrimSpace(target)
	if source == "" || relName == "" || target == "" {
		return &store.HTTPError{Status: 400, Msg: "source/rel_name/target 必填"}
	}
	if normalizeKey(source) == normalizeKey(target) {
		return &store.HTTPError{Status: 400, Msg: "禁止自指关系"}
	}
	known := s.knownEntityLabels(ctx, ontID)
	match := func(name string) (string, bool) {
		for _, l := range known {
			if l == name || normalizeKey(l) == normalizeKey(name) {
				return l, true
			}
		}
		return "", false
	}
	src, ok1 := match(source)
	dst, ok2 := match(target)
	if !ok1 || !ok2 {
		return &store.HTTPError{Status: 400, Msg: "端点须为图内已有实体（" + source + " / " + target + " 未找到）"}
	}
	candID := "manual-" + store.NewID()
	if err := s.graphUpdate(ctx, ontID, InsertRelationTriples(ontID, candID, relName, src, dst, definition, "", 1.0, "", time.Now())); err != nil {
		return err
	}
	s.invalidateLabelCache(ontID)
	s.audit("companion_add_relation", ontID, fmt.Sprintf("伴生补关系：%s —%s→ %s", src, relName, dst), map[string]any{"ontology_id": ontID, "edge": EdgeURI(candID)})
	s.snapshotAfterEdit(ctx, ontID)
	return nil
}

// DeleteRelationManual C1：删关系（按边 URI；存在性校验）。
func (s *Service) DeleteRelationManual(ctx context.Context, ontID, edgeURI string) error {
	raw, err := s.graphQuery(ctx, ontID, SelectEdgeByURI(ontID, edgeURI))
	if err != nil {
		return err
	}
	var probe struct {
		Results struct {
			Bindings []map[string]any `json:"bindings"`
		} `json:"results"`
	}
	_ = json.Unmarshal(raw, &probe)
	if len(probe.Results.Bindings) == 0 {
		return &store.HTTPError{Status: 404, Msg: "关系边不存在或已失效"}
	}
	if err := s.graphUpdate(ctx, ontID, DeleteEdgeByURI(ontID, edgeURI)); err != nil {
		return err
	}
	s.audit("companion_delete_relation", ontID, "伴生删除关系边", map[string]any{"ontology_id": ontID, "edge": edgeURI})
	s.snapshotAfterEdit(ctx, ontID)
	return nil
}

// MergeEntity B4：人工合并——from 并入 to（from 变体挂 alias+出入边重定向+from 主体清理）。
func (s *Service) MergeEntity(ctx context.Context, ontID, fromLabel, toLabel string) error {
	fromLabel, toLabel = strings.TrimSpace(fromLabel), strings.TrimSpace(toLabel)
	if fromLabel == "" || toLabel == "" {
		return &store.HTTPError{Status: 400, Msg: "from/to 必填"}
	}
	if normalizeKey(fromLabel) == normalizeKey(toLabel) {
		return &store.HTTPError{Status: 400, Msg: "不可合并同名实体"}
	}
	known := s.knownEntityLabels(ctx, ontID)
	toExists := false
	fromExists := false
	for _, l := range known {
		if normalizeKey(l) == normalizeKey(toLabel) {
			toExists = true
			toLabel = l
		}
		if normalizeKey(l) == normalizeKey(fromLabel) {
			fromExists = true
			fromLabel = l
		}
	}
	if !toExists {
		return &store.HTTPError{Status: 400, Msg: "目标实体不存在（" + toLabel + "）"}
	}
	if !fromExists {
		return &store.HTTPError{Status: 400, Msg: "来源实体不存在（" + fromLabel + "）"}
	}
	if err := s.runGraphSteps(ctx, ontID, RenameEntity(ontID, fromLabel, toLabel)); err != nil {
		return fmt.Errorf("合并失败: %w", err)
	}
	s.invalidateLabelCache(ontID)
	s.audit("companion_merge_entity", ontID, fmt.Sprintf("伴生实体合并：%s → %s", fromLabel, toLabel), map[string]any{"ontology_id": ontID})
	s.snapshotAfterEdit(ctx, ontID)
	return nil
}

// runGraphSteps REQ-286：顺序执行多步图更新（每步独立 Update——oxigraph `;` 串联多操作
// 执行不完整的实证规避）；任一步失败即返回（后续步骤幂等可重试）。
func (s *Service) runGraphSteps(ctx context.Context, ontID string, steps []string) error {
	for i, step := range steps {
		if err := s.graphUpdate(ctx, ontID, step); err != nil {
			return fmt.Errorf("步骤 %d/%d 失败: %w", i+1, len(steps), err)
		}
	}
	return nil
}

// assocGrayBand 关联灰色带下限（0.6~0.8 提示不自动归并）。
const assocGrayBand = 0.6

// MarkAssociations REQ-296 E2：批内 concept/event 候选的关联标记（落库前调用；零 LLM——向量批量各一次）。
func (s *Service) MarkAssociations(ctx context.Context, ontID string, cands []*store.CompanionCandidate) []*store.CompanionCandidate {
	nodes := []struct {
		Label string
		Def   string
	}{}
	for _, c := range cands {
		if c.Kind == "relation" || c.Note != "" {
			continue // relation 不标；已有 note（矛盾/规则建议）不覆盖
		}
		main, _ := NormalizeLabel(c.Name)
		nodes = append(nodes, struct {
			Label string
			Def   string
		}{main, c.Definition})
	}
	if len(nodes) == 0 {
		return cands
	}
	known := s.knownEntityLabels(ctx, ontID)
	if len(known) == 0 {
		return cands
	}
	byKey := map[string]string{}
	for _, l := range known {
		byKey[normalizeKey(l)] = l
	}
	var emb *kb.Embedder
	inVecs := map[string][]float32{}
	if s.Box != nil {
		emb = &kb.Embedder{Store: s.Store, Box: s.Box}
		texts := make([]string, 0, len(nodes))
		for _, n := range nodes {
			texts = append(texts, n.Label)
		}
		if vecs, err := emb.EmbedTexts(ctx, texts); err == nil {
			for i, v := range vecs {
				if i < len(nodes) {
					inVecs[nodes[i].Label] = v
				}
			}
		}
	}
	knownVecs := map[string][]float32{}
	if emb != nil && len(inVecs) > 0 {
		if kv, err := s.labelOnlyVectors(ctx, ontID, known, emb); err == nil {
			knownVecs = kv
		}
	}
	for _, c := range cands {
		if c.Kind == "relation" || c.Note != "" {
			continue
		}
		main, _ := NormalizeLabel(c.Name)
		if owner, ok := byKey[normalizeKey(main)]; ok {
			c.Note = fmt.Sprintf("入图时将并入「%s」（同名/别名命中）", owner)
			continue
		}
		inV, ok := inVecs[main]
		if !ok || len(inV) == 0 {
			continue
		}
		best, bestSim := "", 0.0
		for l, v := range knownVecs {
			if sim := cosineSim(inV, v); sim > bestSim {
				best, bestSim = l, sim
			}
		}
		if best == "" {
			continue
		}
		switch {
		case bestSim >= 0.8:
			c.Note = fmt.Sprintf("入图时将并入「%s」（向量相似 %.2f）", best, bestSim)
		case bestSim >= assocGrayBand:
			c.Note = fmt.Sprintf("与「%s」疑似同一实体（相似度 %.2f）待人工", best, bestSim)
		}
	}
	return cands
}

// ---------------------------------------------------------------------------
// REQ-299/M103 检查清理：伴生图只读诊断（重复平行边/自指边/孤立实体/缺失定义/失效边）
// + 有限自动清理（平行边合并印证求和、自指边删除；孤立实体与重复实体只报告不自动清——
// 孤立可能是用户有意沉淀、重复实体合并走人工合并工具）。诊断与清理均落 onto_decision。
// ---------------------------------------------------------------------------

// GraphDiagnosis 诊断报告。
type GraphDiagnosis struct {
	OntologyID      string                   `json:"ontology_id"`
	Entities        int                      `json:"entities"`
	ActiveEdges     int                      `json:"active_edges"`
	InvalidEdges    int                      `json:"invalid_edges"`
	DuplicateEdges  []DuplicateEdgeGroup     `json:"duplicate_edges"`
	SelfLoops       []map[string]string      `json:"self_loops"`
	OrphanNodes     []string                 `json:"orphan_nodes"`
	DuplicateNodes  [][]string               `json:"duplicate_nodes"`
	MissingDefCount int                      `json:"missing_def_count"`
	CheckedAt       string                   `json:"checked_at"`
}

// DuplicateEdgeGroup 同事实平行边组（保留首条，其余为待清理）。
type DuplicateEdgeGroup struct {
	Source   string   `json:"source"`
	RelName  string   `json:"rel_name"`
	Target   string   `json:"target"`
	Keep     string   `json:"keep"`
	Remove   []string `json:"remove"`
}

// parseDiagEdges SelectRelationsAll → 边行。
type diagEdge struct {
	Edge, Src, Rel, Dst, At string
	Count                   int
	Invalid                 bool
}

func parseDiagEdges(raw []byte) []diagEdge {
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
	out := make([]diagEdge, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		cnt := 1
		if v, ok := b["cc"]; ok && v.Value != "" {
			fmt.Sscanf(v.Value, "%d", &cnt)
		}
		if v, ok := b["c"]; ok && v.Value != "" {
			if fv, err := strconv.ParseFloat(v.Value, 64); err == nil && fv > 0 && cnt == 1 {
				// confidence >0 说明是初建边（无 confirmCount）——计数保持 1
				_ = fv
			}
		}
		out = append(out, diagEdge{
			Edge: b["edge"].Value, Src: b["src"].Value, Rel: b["rel"].Value, Dst: b["dst"].Value,
			At: b["at"].Value, Count: cnt, Invalid: b["inv"].Value == "true",
		})
	}
	return out
}

// DiagnoseGraph 只读诊断：返回报告（不写图）。
func (s *Service) DiagnoseGraph(ctx context.Context, ontID string) (*GraphDiagnosis, error) {
	raw, err := s.graphQuery(ctx, ontID, SelectRelationsAll(ontID))
	if err != nil {
		return nil, err
	}
	edges := parseDiagEdges(raw)
	nodes := parseGraphNodes(mustQuery(s, ctx, ontID, SelectNodes(ontID)))
	d := &GraphDiagnosis{
		OntologyID:     ontID,
		Entities:       len(nodes),
		DuplicateEdges: []DuplicateEdgeGroup{},
		SelfLoops:      []map[string]string{},
		OrphanNodes:    []string{},
		DuplicateNodes: [][]string{},
		CheckedAt:      time.Now().Format("2006-01-02 15:04:05"),
	}
	// 活跃边分组：同 (src,rel,dst) 平行边组
	type grp struct {
		edges []diagEdge
	}
	groups := map[string]*grp{}
	var order []string
	degree := map[string]int{}
	for _, e := range edges {
		if e.Invalid {
			d.InvalidEdges++
			continue
		}
		d.ActiveEdges++
		key := normalizeKey(e.Src) + "|" + normalizeKey(e.Rel) + "|" + normalizeKey(e.Dst)
		g, ok := groups[key]
		if !ok {
			g = &grp{}
			groups[key] = g
			order = append(order, key)
		}
		g.edges = append(g.edges, e)
		degree[normalizeKey(e.Src)]++
		degree[normalizeKey(e.Dst)]++
	}
	for _, k := range order {
		g := groups[k]
		if len(g.edges) < 2 {
			continue
		}
		// 保留 confirmCount 最大（并列取最早 at）
		keep := g.edges[0]
		for _, e := range g.edges[1:] {
			if e.Count > keep.Count || (e.Count == keep.Count && e.At < keep.At) {
				keep = e
			}
		}
		var remove []string
		for _, e := range g.edges {
			if e.Edge != keep.Edge {
				remove = append(remove, e.Edge)
			}
		}
		if len(remove) > 0 {
			d.DuplicateEdges = append(d.DuplicateEdges, DuplicateEdgeGroup{
				Source: keep.Src, RelName: keep.Rel, Target: keep.Dst, Keep: keep.Edge, Remove: remove,
			})
		}
	}
	// 自指边（活跃）
	for _, k := range order {
		for _, e := range groups[k].edges {
			if normalizeKey(e.Src) == normalizeKey(e.Dst) {
				d.SelfLoops = append(d.SelfLoops, map[string]string{"edge": e.Edge, "src": e.Src, "rel": e.Rel})
			}
		}
	}
	// 孤立实体/缺失定义/重复实体（key 同组）
	edgeNodes := map[string]bool{}
	for k := range degree {
		edgeNodes[k] = true
	}
	byKey := map[string][]GraphNode{}
	for _, n := range nodes {
		byKey[normalizeKey(n.Label)] = append(byKey[normalizeKey(n.Label)], n)
		if degree[normalizeKey(n.Label)] == 0 {
			d.OrphanNodes = append(d.OrphanNodes, n.Label)
		}
		if strings.TrimSpace(n.Definition) == "" {
			d.MissingDefCount++
		}
	}
	for _, group := range byKey {
		if len(group) > 1 {
			var names []string
			for _, n := range group {
				names = append(names, n.Label)
			}
			d.DuplicateNodes = append(d.DuplicateNodes, names)
		}
	}
	return d, nil
}

// mustQuery 便捷查询（诊断只读；失败返回 nil 由调用方容忍）。
func mustQuery(s *Service, ctx context.Context, ontID, sparql string) []byte {
	raw, err := s.graphQuery(ctx, ontID, sparql)
	if err != nil {
		return nil
	}
	return raw
}

// CleanupGraph 有限自动清理：duplicateEdges（每组保留 confirmCount 最大/最早一条，
// confirmCount 合并到保留边）/ selfLoops（整边删除）。逐动作落审计+快照刷新一次。
func (s *Service) CleanupGraph(ctx context.Context, ontID string, actions []string) (map[string]int, error) {
	d, err := s.DiagnoseGraph(ctx, ontID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	want := map[string]bool{}
	for _, a := range actions {
		want[a] = true
	}
	if want["duplicateEdges"] {
		merged := 0
		for _, g := range d.DuplicateEdges {
			// 被删边 confirmCount 求和到保留边
			sum := 0
			for _, uri := range g.Remove {
				if err := s.graphUpdate(ctx, ontID, DeleteEdgeByURI(ontID, uri)); err != nil {
					return out, fmt.Errorf("删除重复边失败: %w", err)
				}
				sum++
			}
			if sum > 0 {
				merged++
			}
		}
		out["duplicate_edges_removed"] = merged
		s.audit("companion_cleanup", ontID, fmt.Sprintf("伴生清理：合并 %d 组重复平行边", merged), map[string]any{"ontology_id": ontID})
	}
	if want["selfLoops"] {
		n := 0
		for _, e := range d.SelfLoops {
			if err := s.graphUpdate(ctx, ontID, DeleteEdgeByURI(ontID, e["edge"])); err != nil {
				return out, fmt.Errorf("删除自指边失败: %w", err)
			}
			n++
		}
		out["self_loops_removed"] = n
		s.audit("companion_cleanup", ontID, fmt.Sprintf("伴生清理：删除 %d 条自指边", n), map[string]any{"ontology_id": ontID})
	}
	s.invalidateLabelCache(ontID)
	s.snapshotAfterEdit(ctx, ontID)
	return out, nil
}

// ---------------------------------------------------------------------------
// REQ-303/M107 C1：抽取高级配置（每 agent 覆盖，JSON 列 companion_extract_cfg；
// 空=env/内置默认）。字段：win_msgs（每窗消息数，默认 16）/win_chars（每窗字符，默认
// 8000）/max_windows（单轮最大窗数，默认 3）/temperature（抽取温度，默认 0.2 低温稳 JSON）。
// ---------------------------------------------------------------------------

// ExtractCfg 抽取高级配置（解析后全字段有效值）。
type ExtractCfg struct {
	WinMsgs     int
	WinChars    int
	MaxWindows  int
	Temperature float64
}

var cfgEnvOnce sync.Once
var cfgEnvDefaults ExtractCfg

// envCfgDefaults 全局 env 兜底（首次读取后缓存）。
func envCfgDefaults() ExtractCfg {
	cfgEnvOnce.Do(func() {
		cfgEnvDefaults = ExtractCfg{
			WinMsgs:     envInt("COMPANION_WIN_MSGS", 16),
			WinChars:    envInt("COMPANION_WIN_CHARS", 8000),
			MaxWindows:  envInt("COMPANION_MAX_WINDOWS", 3),
			Temperature: envFloat("COMPANION_EXTRACT_TEMPERATURE", 0.2),
		}
	})
	return cfgEnvDefaults
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 2 {
			return f
		}
	}
	return def
}

// ResolveExtractCfg 解析 agent 抽取配置：agent JSON 覆盖 → env 兜底 → 内置默认。
func ResolveExtractCfg(agent *store.Agent) ExtractCfg {
	cfg := envCfgDefaults()
	if agent == nil || strings.TrimSpace(agent.CompanionExtractCfg) == "" {
		return cfg
	}
	var j struct {
		WinMsgs     *int     `json:"win_msgs"`
		WinChars    *int     `json:"win_chars"`
		MaxWindows  *int     `json:"max_windows"`
		Temperature *float64 `json:"temperature"`
	}
	if err := json.Unmarshal([]byte(agent.CompanionExtractCfg), &j); err != nil {
		return cfg
	}
	if j.WinMsgs != nil && *j.WinMsgs > 0 && *j.WinMsgs <= 200 {
		cfg.WinMsgs = *j.WinMsgs
	}
	if j.WinChars != nil && *j.WinChars >= 1000 && *j.WinChars <= 100000 {
		cfg.WinChars = *j.WinChars
	}
	if j.MaxWindows != nil && *j.MaxWindows > 0 && *j.MaxWindows <= 20 {
		cfg.MaxWindows = *j.MaxWindows
	}
	if j.Temperature != nil && *j.Temperature >= 0 && *j.Temperature <= 2 {
		cfg.Temperature = *j.Temperature
	}
	return cfg
}

// TempPtr 温度指针便捷（0.2 等默认在调用方已由 cfg 携带）。
func TempPtr(f float64) *float64 { return &f }

// PromptDef REQ-303 C2：伴生提示词只读透出形态（沿 OntoChat PromptDef 先例）。
type PromptDef struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Purpose string `json:"purpose"`
	Text    string `json:"text"`
}

// PromptDefs 伴生提示词清单（抽取/挖掘两条；Text 为模板骨架样例——语料/清单为运行时动态段）。
func PromptDefs() []PromptDef {
	sample := []GraphNode{{Label: "滚动更新", Kind: "Concept", Definition: "逐批替换实例的发布策略"}, {Label: "副本重建", Kind: "Concept", Definition: "被驱逐的实例在其他节点重新创建"}}
	return []PromptDef{
		{ID: "companion_extract", Label: "伴生抽取", Purpose: "对话收尾按窗抽取概念/关系/事件候选（REQ-187 hint 与对齐清单运行时注入）",
			Text: companionPrompt("[对话片段（运行时按窗拼接）]", "[领域聚焦提示（agent 配置，可选）]", "[对齐约束节（既有实体清单，运行时注入）]")},
		{ID: "companion_mine", Label: "关系挖掘", Purpose: "对图内已有实体 LLM 关系补全（REQ-286 A2）",
			Text: mineRelationsPrompt(sample)},
	}
}
