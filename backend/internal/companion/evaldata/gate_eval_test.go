//go:build eval

// REQ-358/M158 C：注入前后对比度量真机跑落（evaldata 离线评测面——「图改动效果」量化，
// Gate −11.2 思想的最小落地：干扰候选批注入后冻结验证集 Recall@5 变化 + 门控决策）。
// 运行：go test -tags eval ./internal/companion/evaldata/ -run TestIngestGateEval -v
package evaldata

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestIngestGateEval(t *testing.T) {
	st, err := store.Open(filepath.Join("..", "..", "..", "data", "platform.db"))
	if err != nil {
		t.Skipf("开发库不在位，跳过: %v", err)
	}
	defer st.Close()
	box, err := secrets.LoadKeyFile(filepath.Join("..", "..", "..", "data", ".secret"))
	if err != nil {
		t.Skipf("密钥文件不在位，跳过: %v", err)
	}
	emb := &kb.Embedder{Store: st, Box: box}
	if _, err := emb.EmbedOne(context.Background(), "连通性探针"); err != nil {
		t.Skipf("embedding 连接不可用，跳过: %v", err)
	}
	embed := func(ctx context.Context, texts []string) ([][]float32, error) { return emb.EmbedTexts(ctx, texts) }

	// 冻结基准：eval-ops 域问题与期望实体（stub 事实面全量标签=基线图）
	var qTexts, baseLabels, expected []string
	for _, q := range Questions {
		if q.Conv != "eval-ops" {
			continue
		}
		qTexts = append(qTexts, q.Input)
		expected = append(expected, q.Expected...)
	}
	for _, f := range Sessions[0].Facts {
		baseLabels = append(baseLabels, f.Name)
	}
	// 扩展候选批（模拟低质自动入图：同义混淆实体挤占 top5）
	injected := []string{"水平扩展策略", "自动伸缩机制", "发布流程管理", "健康检查配置", "渐进式交付"}
	current := append(append([]string{}, baseLabels...), injected...)

	before, after, degraded, err := InjectDeltaReport(embed, qTexts, baseLabels, current, expected, injected)
	if err != nil {
		t.Fatalf("delta 度量失败: %v", err)
	}
	t.Logf("════ 注入前后 Recall@5 对比（eval-ops 域，注入 %d 条干扰候选）════", len(injected))
	t.Logf("注入前: %.3f", before)
	t.Logf("注入后: %.3f", after)
	t.Logf("退化期望实体: %v", degraded)
	decision := "allow"
	if after < before-0.05 {
		decision = "reject（图改动负向）"
	}
	t.Logf("门控决策（delta 口径 ≥-0.05）: %s", decision)
	// 结构断言：度量值域合法；对照结论如实输出不硬门禁（收益以落档为准）
	if before < 0 || before > 1 || after < 0 || after > 1 {
		t.Fatalf("recall 越界: %f→%f", before, after)
	}
}
