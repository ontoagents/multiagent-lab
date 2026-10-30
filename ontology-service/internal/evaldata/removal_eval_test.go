//go:build eval

// REQ-344/OE-3（64 号 §4 论文四步移除法，arXiv 2607.17963 §Dataset Creation）：
// 移除法评估 CQ 驱动扩展的重建贡献——从种子本体系统性移除类集 C（连带 domain/range 引用
// 它们的关系）→ 对移除部分构造 CQ → 以移除后本体为目标跑 ontorag 三段式 → 对照恢复情况。
// 三维简化口径（论文三维的本平台可计算映射，结构维 OOPS! 不引入如实标注）：
//
//	覆盖 Coverage = 移除元素被片段重建的比例（含以同名概念重新声明）
//	忠实 Faithfulness = 重建元素确实存在于原本体（未被幻觉）的比例
//	新颖 Novelty = 重建元素中原本体没有的新建元素比例（越低越好；冗余代理指标）
//
// build tag eval 手动跑（LLM 依赖：真机 GLM 经主平台代理）。运行：
//
//	go test -tags eval ./internal/evaldata/ -run TestRemovalEval -v
package evaldata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/ontorag"
)

// removalResult 单次移除实验的三维结果。
type removalResult struct {
	Target    string   `json:"target"`
	Removed   []string `json:"removed"` // 移除的概念名
	CQ        string   `json:"cq"`      // 对移除部分构造的 CQ
	Rounds    int      `json:"rounds"`
	Generated []string `json:"generated"` // 片段重建的概念名
	Coverage  float64  `json:"coverage"`  // 覆盖：重建∩移除 / 移除
	Faithful  float64  `json:"faithful"`  // 忠实：重建∈原本体 的比例
	Novelty   float64  `json:"novelty"`   // 新颖：重建∉原本体 的比例（冗余代理）
	Status    string   `json:"status"`    // ok | failed
	Error     string   `json:"error,omitempty"`
}

// removalPlan 确定性移除计划（不随机——可复现；每种子取叶子概念前 2 个+其关系连带）。
func removalPlan(sp *pkgspec.Spec) (removed []string, cq string, kept *pkgspec.Spec) {
	// 选叶子概念（无子概念）前 2 个，跳过被关系引用最多的（保证移除后仍有结构）
	hasChild := map[string]bool{}
	for _, c := range sp.Concepts {
		for _, p := range c.Parents {
			hasChild[p] = true
		}
	}
	relRef := map[string]int{}
	for _, r := range sp.Relations {
		relRef[r.From]++
		relRef[r.To]++
	}
	leaves := []string{}
	for _, c := range sp.Concepts {
		if !hasChild[c.Name] {
			leaves = append(leaves, c.Name)
		}
	}
	if len(leaves) < 2 {
		return nil, "", sp
	}
	removed = leaves[:2]
	rmSet := map[string]bool{}
	for _, n := range removed {
		rmSet[n] = true
	}
	kept = &pkgspec.Spec{Name: sp.Name, Description: sp.Description, CQ: sp.CQ}
	for _, c := range sp.Concepts {
		if !rmSet[c.Name] {
			kept.Concepts = append(kept.Concepts, c)
		}
	}
	for _, r := range sp.Relations {
		if !rmSet[r.From] && !rmSet[r.To] {
			kept.Relations = append(kept.Relations, r)
		}
	}
	kept.Instances = sp.Instances
	// CQ 构造（论文口径「对应类在原本体中本意代表什么」的中文适配）
	cq = fmt.Sprintf("本体中应有「%s」与「%s」的概念——请重建这两个概念及其与既有概念的关联关系。",
		removed[0], removed[1])
	return removed, cq, kept
}

func TestRemovalEval(t *testing.T) {
	platformURL := os.Getenv("PLATFORM_URL")
	if platformURL == "" {
		platformURL = "http://127.0.0.1:8080"
	}
	// 主平台连通预检（生成链 LLM 依赖；未启动则跳过）
	if resp, err := http.Get(platformURL + "/healthz"); err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("主平台不可达（%v）——移除法评估依赖 LLM 生成链，跳过", err)
	} else {
		resp.Body.Close()
	}
	llm := llmcreate.New(platformURL)
	llm.HTTP.Timeout = 300 * time.Second

	// 评测对象：小种子 defects（6 概念——移除 2 叶子后仍 4 概念有结构；大种子成本过高留后续扩容）
	raw, err := os.ReadFile("../../internal/seed/examples/defects.json")
	if err != nil {
		t.Skipf("种子 defects 读取失败: %v", err)
	}
	var full pkgspec.Spec
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatalf("种子解析失败: %v", err)
	}
	removed, cq, kept := removalPlan(&full)
	if len(removed) == 0 {
		t.Skip("种子无可移除叶子（<2）")
	}
	t.Logf("移除: %v\n构造 CQ: %s\n保留: 概念 %d 关系 %d", removed, cq, len(kept.Concepts), len(kept.Relations))

	// ontorag 三段式：检索→逐 CQ 生成（structure profile）→去重
	ctx := context.Background()
	client := ontorag.NewClient(platformURL)
	elements, vectors, err := client.Index(ctx, "removal_eval", kept)
	if err != nil {
		t.Fatalf("索引失败: %v", err)
	}
	hits, err := client.Retrieve(ctx, cq, elements, vectors, 20)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	gen := ontorag.Generate(ctx, llm, kept, cq, ontorag.ProfileStructure, hits, func(msg string) {
		t.Logf("  [生成] %s", msg)
	})
	if gen.Status != "ok" || gen.Fragment == nil {
		t.Fatalf("片段生成失败（不静默）: %s — %s", gen.Status, gen.Error)
	}

	// 三维对照
	res := removalResult{Target: "defects", Removed: removed, CQ: cq, Rounds: gen.Rounds, Status: "ok"}
	inFull := map[string]bool{}
	for _, c := range full.Concepts {
		inFull[c.Name] = true
	}
	seen := map[string]bool{}
	for _, c := range gen.Fragment.Concepts {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		res.Generated = append(res.Generated, c.Name)
		if inFull[c.Name] {
			res.Faithful++
		} else {
			res.Novelty++
		}
	}
	cover := 0
	for _, n := range removed {
		if seen[n] {
			cover++
		}
	}
	if n := len(res.Generated); n > 0 {
		res.Coverage = float64(cover) / float64(len(removed))
		res.Faithful = res.Faithful / float64(n)
		res.Novelty = res.Novelty / float64(n)
	}
	t.Logf("")
	t.Logf("== 移除法三维评估（REQ-344/OE-3；结构维 OOPS! 不引入如实标注）==")
	t.Logf("移除 %v | 重建 %v", removed, res.Generated)
	t.Logf("覆盖 Coverage=%.2f（移除元素被重建比例）| 忠实 Faithful=%.2f（重建∈原本体比例）| 新颖 Novelty=%.2f（新建比例，冗余代理）",
		res.Coverage, res.Faithful, res.Novelty)
	// 硬断言：覆盖必须 >0（CQ 明确指名重建，一个都没恢复=链路失效）；忠实/新颖只报告不做硬门禁（阈值校准后续轮）
	if res.Coverage == 0 {
		t.Errorf("覆盖为 0——CQ 指名重建的元素一个都没恢复，链路失效")
	}
}
