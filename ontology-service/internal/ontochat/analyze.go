// analyze.go CQ 分析：paraphrase 去重 + 主题聚类（REQ-273/S2 + REQ-342/OC-3 分步化深化，
// 63 号 §1.3 模块 3 + P6 蓝本中文适配）。
// 论文两步对齐：①paraphrase detection（LLM 判等价分组——保留最清晰表述，产出合并明细）
// ②主题聚类（对去重后清单聚类带标签）。REQ-273 v1 合步（诚实标注），本轮分步并把
// 去重明细（保留哪条、合并了哪些）透出（「补完整语义」）。
// 纯 LLM 路线不引句向量（D-O23，沿论文反 IDEA/RevOnt 立场）。
// 诚实边界（63 号 §6.2⑤）：聚类不能单独支撑完整分析（论文评估仅 62.5% 认为交互直观）——
// 产出须经人工确认（analyze 确认步定案）后应用写回；paraphrase 步失败降级跳过去重直接聚类。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// clusterSchema 聚类输出契约（去重已由 paraphrase 步承载，聚类只分组）。
const clusterSchema = `{"type":"object","required":["clusters"],"properties":{"clusters":{"type":"array","items":{"type":"object","required":["label","cqs"],"properties":{"label":{"type":"string"},"cqs":{"type":"array","items":{"type":"string"}}}}}}}`

// paraphraseSchema paraphrase 去重输出契约（OC-3 分步：groups 明细=保留哪条合并了哪些）。
const paraphraseSchema = `{"type":"object","required":["groups"],"properties":{"groups":{"type":"array","items":{"type":"object","required":["keep"],"properties":{"keep":{"type":"string"},"duplicates":{"type":"array","items":{"type":"string"}}}}}}}`

// paraphrasePrompt paraphrase 去重提示词（P6 paraphrase detection 步中文适配）。
const paraphrasePrompt = `你是本体需求分析师。请对下方能力问题（CQ）清单做同义识别与去重（paraphrase detection）：

- 语义等价或高度相似的问题归为一组，保留表述最清晰、最完整的一句为 keep，其余列入 duplicates；
- 不要丢失任何不同的信息点——语义有实质差异的问题不得合并；
- 未与他项等价的问题各自成组（duplicates 留空数组）；
- 全部问题都必须出现在某个组里，不要遗漏、不要新增；
- 只输出 JSON 对象，不要 markdown 代码块或其他文本。

CQ 清单：
{cqs}`

// cqClusterPrompt 主题聚类提示词（P6 clustering 步；去重已前置完成）。
const cqClusterPrompt = `你是本体需求分析师。请把下方能力问题（CQ）按建模主题聚类分组：

1. 每簇给一个简短中文标签（如「症状-疾病映射」「药物禁忌」「层级分类」），同一簇内主题粒度相近；
{clusterCount}
2. 全部问题都必须归入某个簇，不要遗漏、不要新增。

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

// MergeGroup paraphrase 去重明细（保留表述 + 被合并的等价表述）。
type MergeGroup struct {
	Keep       string   `json:"keep"`
	Duplicates []string `json:"duplicates"`
}

// AnalyzeResult CQ 分析两步完整结果。
type AnalyzeResult struct {
	Clusters     []CQCluster  `json:"clusters"`
	MergedGroups []MergeGroup `json:"merged_groups,omitempty"` // paraphrase 去重明细（OC-3）
	Dedup        int          `json:"dedup"`                   // 被合并条数（Σ duplicates；降级时 0）
	DedupSkipped bool         `json:"dedup_skipped,omitempty"` // paraphrase 步失败降级（只聚类未去重）
}

// AnalyzeCQs CQ 分析两步：paraphrase 去重（明细透出）→ 主题聚类。会话 CQs 为空时报错引导。
// maxClusters>0 指定簇数；=0 由模型自定。
func (e *Engine) AnalyzeCQs(ctx context.Context, sess *Session, maxClusters int, onProgress ...RoundProgress) (AnalyzeResult, error) {
	notify := func(msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(1, msg)
			}
		}
	}
	cqs := sess.Context.CQs
	if len(cqs) == 0 {
		return AnalyzeResult{}, fmt.Errorf("会话尚无能力问题：请先「抽取 CQ」或手动补充后再分析")
	}
	// 步①paraphrase 去重（失败降级：跳过去重直接聚类，dedup_skipped 如实标注）
	kept, merged, dedup, skipped := e.paraphraseCQs(ctx, cqs, notify)
	// 步②主题聚类
	clusters, err := e.clusterCQs(ctx, kept, maxClusters, notify)
	if err != nil {
		return AnalyzeResult{}, err
	}
	return AnalyzeResult{Clusters: clusters, MergedGroups: merged, Dedup: dedup, DedupSkipped: skipped}, nil
}

// paraphraseCQs 步①：等价分组（≤2 轮回喂；失败降级返回原清单 skipped=true）。
func (e *Engine) paraphraseCQs(ctx context.Context, cqs []string, notify func(string)) ([]string, []MergeGroup, int, bool) {
	prompt := strings.Replace(paraphrasePrompt, "{cqs}", joinNumbered(cqs), 1)
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("去重步（第 %d/2 轮）：同义识别", round))
		raw, _, err := e.LLM.Chat(ctx, prompt, paraphraseSchema)
		if err != nil {
			notify("去重步调用失败，跳过去重直接聚类")
			return cqs, nil, 0, true
		}
		var out struct {
			Groups []MergeGroup `json:"groups"`
		}
		if perr := json.Unmarshal([]byte(raw), &out); perr != nil || len(out.Groups) == 0 {
			if round == 2 {
				notify("去重步输出不合法，跳过去重直接聚类")
				return cqs, nil, 0, true
			}
			notify("去重步输出不是合法 JSON，回喂重试")
			prompt += "\n\n注意：上一次输出不是合法的 {\"groups\":[{\"keep\":\"...\",\"duplicates\":[...]}]} JSON 对象，请严格只输出该 JSON 结构。"
			continue
		}
		// 净化：keep/duplicates trim 去空；只认清单内的原文（模型改写原文一律丢弃——不得新增/篡改）
		valid := map[string]bool{}
		for _, q := range cqs {
			valid[q] = true
		}
		kept := make([]string, 0, len(cqs))
		merged := make([]MergeGroup, 0, len(out.Groups))
		seen := map[string]bool{}
		dedup := 0
		for _, g := range out.Groups {
			k := strings.TrimSpace(g.Keep)
			if !valid[k] || seen[k] {
				continue
			}
			seen[k] = true
			kept = append(kept, k)
			dups := make([]string, 0, len(g.Duplicates))
			for _, d := range g.Duplicates {
				d = strings.TrimSpace(d)
				if valid[d] && d != k && !seen[d] {
					seen[d] = true
					dups = append(dups, d)
					dedup++
				}
			}
			merged = append(merged, MergeGroup{Keep: k, Duplicates: dups})
		}
		// 覆盖兜底：模型漏掉的清单原文原样保留（不得丢失任何条目）
		for _, q := range cqs {
			if !seen[q] {
				kept = append(kept, q)
				merged = append(merged, MergeGroup{Keep: q})
			}
		}
		return kept, merged, dedup, false
	}
	return cqs, nil, 0, true
}

// clusterCQs 步②：主题聚类（净化口径沿 REQ-273：trim/去空/跨簇去重/空 label 孤儿簇剔除；
// 覆盖兜底=未归簇的原文并入「未归类」簇不丢失）。
func (e *Engine) clusterCQs(ctx context.Context, cqs []string, maxClusters int, notify func(string)) ([]CQCluster, error) {
	countRule := clusterCountAuto
	if maxClusters > 0 {
		countRule = fmt.Sprintf(clusterCountFixed, maxClusters)
	}
	prompt := strings.Replace(strings.Replace(cqClusterPrompt, "{clusterCount}", countRule, 1), "{cqs}", joinNumbered(cqs), 1)
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("聚类步（第 %d/2 轮）：主题分组", round))
		raw, _, err := e.LLM.Chat(ctx, prompt, clusterSchema)
		if err != nil {
			return nil, err
		}
		var out struct {
			Clusters []CQCluster `json:"clusters"`
		}
		if perr := json.Unmarshal([]byte(raw), &out); perr != nil || len(out.Clusters) == 0 {
			if round == 2 {
				if perr != nil {
					return nil, fmt.Errorf("聚类输出解析失败：%w", perr)
				}
				return nil, fmt.Errorf("聚类输出为空")
			}
			notify("聚类步输出不合法，回喂重试")
			prompt += "\n\n注意：上一次输出不是合法的 {\"clusters\":[{\"label\":\"...\",\"cqs\":[...]}]} JSON 对象，请严格只输出该 JSON 结构。"
			continue
		}
		seen := map[string]bool{}
		cleaned := make([]CQCluster, 0, len(out.Clusters))
		for _, cl := range out.Clusters {
			label := strings.TrimSpace(cl.Label)
			if label == "" {
				continue // 空 label 簇整簇剔除（其问题由覆盖兜底承接）
			}
			qs := make([]string, 0, len(cl.CQs))
			for _, q := range cl.CQs {
				q = strings.TrimSpace(q)
				if q == "" || seen[q] {
					continue
				}
				seen[q] = true
				qs = append(qs, q)
			}
			if len(qs) == 0 {
				continue
			}
			cleaned = append(cleaned, CQCluster{Label: label, CQs: qs})
		}
		if len(cleaned) == 0 {
			return nil, fmt.Errorf("聚类输出无有效簇")
		}
		// 覆盖兜底：未归簇的原文并入「未归类」簇（聚类失败不丢条目）
		var missed []string
		for _, q := range cqs {
			if !seen[q] {
				missed = append(missed, q)
			}
		}
		if len(missed) > 0 {
			cleaned = append(cleaned, CQCluster{Label: "未归类", CQs: missed})
		}
		return cleaned, nil
	}
	return nil, fmt.Errorf("分析循环异常退出")
}
