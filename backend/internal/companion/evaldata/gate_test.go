// REQ-358/M158 C 门控单测（stub 向量——冻结基准集真实数据 + 受控向量空间）：
// 基准域健康图放行 / 期望实体缺失（recall<τ）拒绝并落归因 / 任意域无信号诚实放行。
package evaldata

import (
	"context"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/companion"
)

const gateDims = 16

// gateOrth 正交向量（与其他维度 cos=0）。
func gateOrth() []float32 { v := make([]float32, gateDims); v[gateDims-1] = 1; return v }

// stubEmbed 受控向量空间：lookup 命中=原向量；未命中=正交向量（cosine 0）。
func stubEmbed(lookup map[string][]float32) func(ctx context.Context, texts []string) ([][]float32, error) {
	return func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, 0, len(texts))
		for _, t := range texts {
			if v, ok := lookup[t]; ok {
				out = append(out, v)
			} else {
				out = append(out, gateOrth())
			}
		}
		return out, nil
	}
}

// evalOpsOpsLabels 健康基准域图标签（eval-ops 事实面全量）。
func evalOpsLabels() []string {
	var out []string
	for _, f := range Sessions[0].Facts {
		out = append(out, f.Name)
		if f.RelTarget != "" && f.RelName != "" {
			out = append(out, f.RelTarget)
		}
	}
	return out
}

func TestRecallFloorGate(t *testing.T) {
	// 受控向量空间（16 维，每题独立维度——同题期望实体与查询同向 sim=1，其余全正交 0）：
	// 保证每题 Recall@5 恰为 1（健康图）/0（期望实体缺失图），断言不受排序抖动影响。
	lookup := map[string][]float32{}
	for i, q := range Questions {
		v := make([]float32, gateDims)
		v[i] = 1
		lookup[q.Input] = v
		for _, e := range q.Expected {
			lookup[e] = v
		}
	}
	stub := stubEmbed(lookup)

	deps := GateDeps{
		LabelsOf: func(ctx context.Context, ontID string) ([]string, error) { return evalOpsLabels(), nil },
		Embed:    stub,
		EmbedOne: func(ctx context.Context, text string) ([]float32, error) {
			vs, err := stub(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			return vs[0], nil
		},
		Tau:      0.60,
	}
	gate := RecallFloorGate(deps)
	ctx := context.Background()

	// ①基准域健康图：放行
	ok, reason := gate(ctx, companion.GateCandidateInfo{OntologyID: "ont-x", Name: "任意"})
	if !ok {
		t.Fatalf("健康基准域应放行，got 拒绝：%s", reason)
	}

	// ②期望实体缺失（图退化）：recall<τ → 拒绝（归因含 recall 与 τ）
	deps2 := deps
	deps2.LabelsOf = func(ctx context.Context, ontID string) ([]string, error) {
		return []string{"无关甲", "无关乙", "配置中心", "镜像仓库"}, nil // 仅余非期望实体
	}
	gate2 := RecallFloorGate(deps2)
	ok2, reason2 := gate2(ctx, companion.GateCandidateInfo{OntologyID: "ont-x", Name: "任意"})
	if ok2 {
		t.Fatal("退化图（期望实体缺失）应拒绝")
	}
	if !strings.Contains(reason2, "benchmark_recall") || !strings.Contains(reason2, "τ") {
		t.Fatalf("拒绝归因应含 recall 与 τ：%s", reason2)
	}

	// ③任意域无重叠：诚实放行 no-benchmark-signal
	deps3 := deps
	deps3.LabelsOf = func(ctx context.Context, ontID string) ([]string, error) {
		return []string{"量子纠缠", "蛋白质折叠"}, nil
	}
	gate3 := RecallFloorGate(deps3)
	ok3, reason3 := gate3(ctx, companion.GateCandidateInfo{OntologyID: "ont-y", Name: "任意"})
	if !ok3 || !strings.Contains(reason3, "no-benchmark-signal") {
		t.Fatalf("无重叠域应诚实放行，got %v/%s", ok3, reason3)
	}

	// ④embedding 不可用：门控无信号放行（不阻断主链路）
	deps4 := GateDeps{
		LabelsOf: deps.LabelsOf,
		Embed: func(ctx context.Context, texts []string) ([][]float32, error) {
			return nil, context.DeadlineExceeded
		},
		EmbedOne: deps.EmbedOne,
		Tau:      0.60,
	}
	gate4 := RecallFloorGate(deps4)
	if ok4, _ := gate4(ctx, companion.GateCandidateInfo{OntologyID: "ont-x"}); !ok4 {
		t.Fatal("embedding 不可用应无信号放行")
	}
}
