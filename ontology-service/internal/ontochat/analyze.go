// analyze.go CQ 分析：paraphrase 去重 + 主题聚类（REQ-273/S2，63 号 §1.3 模块 3 + P6 蓝本中文适配）。
// 论文主张纯 LLM 路线（不依赖句向量——本项目 D-O23 同口径不引 sentence-transformers）；
// paraphrase 去重与聚类在论文中为两步，v1 合一次调用（同 REQ-272 合步诚实标注）：
// 语义等价的问题合并保留最清晰表述（=去冗余），其余按主题聚成带标签簇。
// 诚实边界（63 号 §6.2⑤）：聚类不能单独支撑完整分析（论文评估仅 62.5% 认为交互直观）——
// 产出须经人工确认（analyze 确认步定案）后应用写回。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// clusterSchema CQ 聚类输出契约。
const clusterSchema = `{"type":"object","required":["clusters"],"properties":{"clusters":{"type":"array","items":{"type":"object","required":["label","cqs"],"properties":{"label":{"type":"string"},"cqs":{"type":"array","items":{"type":"string"}}}}}}}`

// cqAnalyzePrompt CQ 去重与聚类提示词（P6 蓝本中文适配）。
const cqAnalyzePrompt = `你是本体需求分析师。请对下方能力问题（CQ）清单做去冗余与主题聚类：

1. 去重（paraphrase 合并）：语义等价或高度相似的问题合并为一条，保留表述最清晰、最完整的一句；不要丢失任何不同的信息点；
2. 主题聚类：把去重后的问题按建模主题分组，每簇给一个简短中文标签（如「症状-疾病映射」「药物禁忌」「层级分类」），同一簇内主题粒度相近；
{clusterCount}
3. 全部问题都必须归入某个簇，不要遗漏、不要新增。

只输出 JSON 对象，不要 markdown 代码块或其他文本。

CQ 清单：
{cqs}`

const clusterCountAuto = `簇数由你根据主题数量决定（通常 2~6 簇）；`
const clusterCountFixed = "恰好分为 %d 簇；"

// CQCluster 单个主题簇。
type CQCluster struct {
	Label string   `json:"label"`
	CQs   []string `json:"cqs"`
}

// AnalyzeCQs CQ 去重与聚类：会话 CQs 为空时报错引导；解析失败带错误回喂 ≤2 轮。
// maxClusters>0 指定簇数；=0 由模型自定。返回（簇清单, 去重合并掉的条数, error）。
func (e *Engine) AnalyzeCQs(ctx context.Context, sess *Session, maxClusters int, onProgress ...RoundProgress) ([]CQCluster, int, error) {
	notify := func(msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(1, msg)
			}
		}
	}
	cqs := sess.Context.CQs
	if len(cqs) == 0 {
		return nil, 0, fmt.Errorf("会话尚无能力问题：请先「抽取 CQ」或手动补充后再分析")
	}
	countRule := clusterCountAuto
	if maxClusters > 0 {
		countRule = fmt.Sprintf(clusterCountFixed, maxClusters)
	}
	prompt := strings.Replace(strings.Replace(cqAnalyzePrompt, "{clusterCount}", countRule, 1), "{cqs}", joinNumbered(cqs), 1)
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("第 %d/2 轮：去重与聚类中", round))
		raw, _, err := e.LLM.Chat(ctx, prompt, clusterSchema)
		if err != nil {
			return nil, 0, err
		}
		var out struct {
			Clusters []CQCluster `json:"clusters"`
		}
		if perr := json.Unmarshal([]byte(raw), &out); perr != nil || len(out.Clusters) == 0 {
			if round == 2 {
				if perr != nil {
					return nil, 0, fmt.Errorf("聚类输出解析失败：%w", perr)
				}
				return nil, 0, fmt.Errorf("聚类输出为空")
			}
			notify("第 1 轮输出不合法，回喂重试")
			prompt += "\n\n注意：上一次输出不是合法的 {\"clusters\":[{\"label\":\"...\",\"cqs\":[...]}]} JSON 对象，请严格只输出该 JSON 结构。"
			continue
		}
		// 净化：trim/去空行；校验覆盖（输出条数不得多于输入——LLM 不得新增问题）
		seen := map[string]bool{}
		cleaned := make([]CQCluster, 0, len(out.Clusters))
		total := 0
		for _, cl := range out.Clusters {
			label := strings.TrimSpace(cl.Label)
			if label == "" {
				continue // 空 label 簇整簇剔除（其问题不计入覆盖统计——无主题归属不可用）
			}
			qs := make([]string, 0, len(cl.CQs))
			for _, q := range cl.CQs {
				q = strings.TrimSpace(q)
				if q == "" || seen[q] {
					continue
				}
				seen[q] = true
				qs = append(qs, q)
				total++
			}
			if len(qs) == 0 {
				continue
			}
			cleaned = append(cleaned, CQCluster{Label: label, CQs: qs})
		}
		if len(cleaned) == 0 {
			return nil, 0, fmt.Errorf("聚类输出无有效簇")
		}
		dedup := len(cqs) - total
		if dedup < 0 {
			dedup = 0
		}
		return cleaned, dedup, nil
	}
	return nil, 0, fmt.Errorf("分析循环异常退出")
}
