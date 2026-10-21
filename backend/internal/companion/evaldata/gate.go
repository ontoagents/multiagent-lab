// gate.go REQ-358/M158 C（EvoOntology 增量借鉴·93 号 §五 P2④/§7.1「Gate −11.2 最大承重件」）
// 伴生自动入图基准门控（旁路包）：冻结验证集（本包 Sessions/Questions，stub 事实面）上的
// Recall@5 地板门控 + 注入前后对比度量（图改动效果）。旁挂语义——决策函数只读（不改图、
// 不改入图写路径），装配层显式开启（COMPANION_BENCHMARK_GATE=1），默认 nil=现行为零变化。
//
// 门控阈值 τ：宽平台 [0.60,0.80]（EvoOntology 实证口径，93 号 §7.2）；默认 0.60，
// COMPANION_GATE_TAU 可调。诚实边界：任意域伴生本体与冻结基准无重叠时门控无信号（诚实
// 放行 allow+no-benchmark-signal）——per-domain 冻结验证集扩充前门控仅对基准域有效。
package evaldata

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/companion"
)

// GateTauDefault 门控地板默认值（宽平台 [0.60,0.80] 下限——宁松勿误杀，93 号 §7.2）。
const GateTauDefault = 0.60

// GateTau 环境变量覆盖（COMPANION_GATE_TAU，0~1）。
func GateTau() float64 {
	if v := os.Getenv("COMPANION_GATE_TAU"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			return f
		}
	}
	return GateTauDefault
}

// GateDeps 门控装配窄面（装配层以 Service 只读口与 KB 默认 embedding 连接填充）。
type GateDeps struct {
	// LabelsOf 取宿主伴生图实体标签（companion.Service.LabelsFor 只读口）
	LabelsOf func(ctx context.Context, ontologyID string) ([]string, error)
	// Embed 批量向量（kb.Embedder.EmbedTexts）
	Embed func(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedOne 单条向量（kb.Embedder.EmbedOne）
	EmbedOne func(ctx context.Context, text string) ([]float32, error)
	Tau      float64 // 地板（≤0 取 GateTau()）
}

// RecallFloorGate 伴生自动入图基准门控（旁挂、只读）：冻结验证集在当前图上的 Recall@5
// 低于 τ → 拒绝自动入图（图质量退化期的写入保护——Gate −11.2 思想最小落地）；与冻结基准
// 无重叠（任意域）→ 诚实放行（no-benchmark-signal）。冻结问题向量进程内缓存一次。
func RecallFloorGate(deps GateDeps) companion.GateFunc {
	var (
		qmu      sync.Mutex
		qvecs    map[string][]float32
		tauOf    = func() float64 {
			if deps.Tau > 0 {
				return deps.Tau
			}
			return GateTau()
		}
	)
	ensureQVecs := func(ctx context.Context) error {
		qmu.Lock()
		defer qmu.Unlock()
		if qvecs != nil {
			return nil
		}
		texts := make([]string, 0, len(Questions))
		for _, q := range Questions {
			texts = append(texts, q.Input)
		}
		vecs, err := deps.Embed(ctx, texts)
		if err != nil {
			return err
		}
		qvecs = map[string][]float32{}
		for i, q := range Questions {
			if i < len(vecs) {
				qvecs[q.ID] = vecs[i]
			}
		}
		return nil
	}
	return func(ctx context.Context, cand companion.GateCandidateInfo) (bool, string) {
		tau := tauOf()
		labels, err := deps.LabelsOf(ctx, cand.OntologyID)
		if err != nil || len(labels) == 0 {
			return true, "" // 图不可达/空图：门控无信号，诚实放行（阈值面已把守）
		}
		if err := ensureQVecs(ctx); err != nil {
			return true, "" // embedding 不可用：门控无信号，诚实放行（不阻断伴生主链路）
		}
		labelVecs, err := deps.Embed(ctx, labels)
		if err != nil {
			return true, ""
		}
		lv := map[string][]float32{}
		for i, l := range labels {
			if i < len(labelVecs) {
				lv[l] = labelVecs[i]
			}
		}
		// 域重叠判定：冻结期望实体出现在图标签中才视为基准域（任意域诚实放行）
		labelSet := map[string]bool{}
		for _, l := range labels {
			labelSet[l] = true
		}
		active := map[string]bool{} // 活跃域（有重叠的会话）
		for _, q := range Questions {
			for _, e := range q.Expected {
				if labelSet[e] {
					active[q.Conv] = true
					break
				}
			}
		}
		if len(active) == 0 {
			return true, "no-benchmark-signal"
		}
		// Recall@5（向量口径同评测：余弦 top5 阈 0.35）在活跃域问题上平均
		hit, total := 0.0, 0.0
		for _, q := range Questions {
			if !active[q.Conv] {
				continue
			}
			qv := qvecs[q.ID]
			type scored struct {
				label string
				sim   float64
			}
			var cands []scored
			for l, v := range lv {
				if sim := companion.EvalCosine(qv, v); sim >= 0.35 {
					cands = append(cands, scored{l, sim})
				}
			}
			sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
			if len(cands) > 5 {
				cands = cands[:5]
			}
			set := map[string]bool{}
			for _, c := range cands {
				set[c.label] = true
			}
			h, n := 0, len(q.Expected)
			for _, e := range q.Expected {
				if set[e] {
					h++
				}
			}
			hit += float64(h)
			total += float64(n)
		}
		if total == 0 {
			return true, "no-benchmark-signal"
		}
		recall := hit / total
		if recall < tau {
			return false, fmt.Sprintf("benchmark_recall@5=%.2f < τ=%.2f（冻结验证集地板门控；候选暂缓自动入图，转人工确认）", recall, tau)
		}
		return true, fmt.Sprintf("benchmark_recall@5=%.2f ≥ τ=%.2f", recall, tau)
	}
}

// InjectDeltaReport 注入前后对比度量（evaldata 离线评测面，TestIngestGateEval 消费）：
// 在给定标签/期望实体上计算注入前后的 Recall@5 变化（图改动效果——Gate −11.2 的量化口径）。
// embed 为向量函数（离线评测注入 kb.Embedder 适配）；baseline/current 为注入前后图标签全集。
func InjectDeltaReport(embed func(ctx context.Context, texts []string) ([][]float32, error), questionTexts, baselineLabels, currentLabels, expected []string, injected []string) (before, after float64, degraded []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	all := append(append([]string{}, questionTexts...), baselineLabels...)
	all = append(all, currentLabels...)
	vecs, err := embed(ctx, dedupe(all))
	if err != nil {
		return 0, 0, nil, err
	}
	byText := map[string][]float32{}
	for i, t := range dedupe(all) {
		byText[t] = vecs[i]
	}
	recall := func(labels []string) float64 {
		lv := map[string][]float32{}
		for _, l := range labels {
			if v, ok := byText[l]; ok {
				lv[l] = v
			}
		}
		hit, n := 0.0, 0.0
		for _, qt := range questionTexts {
			qv, ok := byText[qt]
			if !ok {
				continue
			}
			type scored struct {
				label string
				sim   float64
			}
			var cands []scored
			for l, v := range lv {
				if sim := companion.EvalCosine(qv, v); sim >= 0.35 {
					cands = append(cands, scored{l, sim})
				}
			}
			sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
			if len(cands) > 5 {
				cands = cands[:5]
			}
			set := map[string]bool{}
			for _, c := range cands {
				set[c.label] = true
			}
			for _, e := range expected {
				if set[e] {
					hit++
				}
			}
			n += float64(len(expected))
		}
		if n == 0 {
			return 0
		}
		return hit / n
	}
	before = recall(baselineLabels)
	after = recall(currentLabels)
	for _, e := range expected {
		inB, inA := contains(baselineLabels, e), contains(currentLabels, e)
		if inB && !inA {
			degraded = append(degraded, e)
		}
	}
	_ = injected
	return before, after, degraded, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
