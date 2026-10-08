// coverage.go CQ 覆盖测试快筛（REQ-274/S3，63 号 §1.3 模块 4 + P7/P8 蓝本中文适配）。
// 两步：①spec 口语化（概念/关系/实例三段式纯文本，P8 算法适配 spec_json 形态）；
// ②逐 CQ 独立 Yes/No 判定附解释（P7——每条单独调用防泄漏/保独立性，论文不变式）。
// 与 REQ-255 并轨：口语化快筛→存疑项 SPARQL 精判（质量卡「CQ 验收」），通过率统一入质量卡。
// 诚实边界（63 号 §6.2③）：口语化判定对「可推断但不显式」的需求有系统性乐观偏误；
// 定案④默认关——质量卡显式开启（成本=N 次 LLM 调用，入口成本预估警告）。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// coverageVerdictSchema 单条 CQ 判定输出契约（P7 的 Yes/No+解释结构化）。
const coverageVerdictSchema = `{"type":"object","required":["verdict","explanation"],"properties":{"verdict":{"type":"string","enum":["Yes","No"]},"explanation":{"type":"string"}}}`

// coveragePrompt 单条 CQ 判定（P7 cqt_prompt_a/b 中文适配：判定+解释）。
const coveragePrompt = `你是本体评审专家。以下是一份本体的纯文本描述（口语化），请推断该本体能否回应这条能力问题（Competency Question）。

本体描述：
{verbalisation}

能力问题：{cq}

判定标准：本体中已建模的概念/关系/实例（或其结构）足以结构化地回应该问题=「Yes」；缺少必要的概念、关系或属性=「No」。
只输出 JSON 对象：{"verdict":"Yes"或"No","explanation":"一句话中文说明（指出依据的概念/关系，或缺失什么）"}`

// CQVerdict 单条覆盖判定结果。
type CQVerdict struct {
	CQ          string `json:"cq"`
	Verdict     string `json:"verdict"` // Yes | No | Unknown（调用失败如实标注，不静默放行）
	Explanation string `json:"explanation"`
}

// VerbaliseSpec spec 口语化（P8 算法适配：类（含父链+定义）/关系（域→值域+定义）/实例三段式；
// 论文假设本体注释良好——缺定义/域值域的条目以「（未填写）」如实标注，不臆造）。
func VerbaliseSpec(sp *pkgspec.Spec) string {
	var b strings.Builder
	b.WriteString("本体名称：" + sp.Name)
	if sp.Description != "" {
		b.WriteString("——" + sp.Description)
	}
	b.WriteString("\n\n主要概念（类）：\n")
	for _, c := range sp.Concepts {
		b.WriteString("- " + c.Name)
		if c.Label != "" && c.Label != c.Name {
			b.WriteString("（" + c.Label + "）")
		}
		if len(c.Parents) > 0 {
			b.WriteString("，是 " + strings.Join(c.Parents, "、") + " 的子概念")
		}
		if c.Definition != "" {
			b.WriteString("：" + c.Definition)
		} else {
			b.WriteString("：（未填写定义）")
		}
		b.WriteString("\n")
	}
	if len(sp.Relations) > 0 {
		b.WriteString("\n主要关系：\n")
		for _, r := range sp.Relations {
			b.WriteString("- " + r.Name)
			if r.Label != "" && r.Label != r.Name {
				b.WriteString("（" + r.Label + "）")
			}
			b.WriteString("：" + r.From + " → " + r.To)
			if r.Definition != "" {
				b.WriteString("，" + r.Definition)
			}
			b.WriteString("\n")
		}
	}
	if len(sp.Instances) > 0 {
		b.WriteString("\n主要实例（个体）：\n")
		for _, inst := range sp.Instances {
			b.WriteString("- " + inst.Name + "，是 " + inst.Concept + " 的实例")
			if len(inst.Attributes) > 0 {
				keys := make([]string, 0, len(inst.Attributes))
				for k := range inst.Attributes {
					keys = append(keys, k)
				}
				b.WriteString("，属性：" + strings.Join(keys, "、"))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n（以上为本体全部建模内容的口语化；「未填写」处表示该注释缺失。）")
	return b.String()
}

// TestCoverage 逐 CQ 独立判定：每条单独一次调用（防 prompt 泄漏/保独立性，论文不变式）。
// 单条调用失败记 Unknown 并继续（报告如实呈现），全部 Unknown 时返回错误。
func (e *Engine) TestCoverage(ctx context.Context, sp *pkgspec.Spec, cqs []string, onProgress ...func(done, total int, msg string)) ([]CQVerdict, error) {
	if len(cqs) == 0 {
		return nil, fmt.Errorf("本体无能力问题（spec.CQ 为空）：请先在构建会话中抽取并确认 CQ，或显式传入 cqs")
	}
	verbalisation := VerbaliseSpec(sp)
	notify := func(done int, msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(done, len(cqs), msg)
			}
		}
	}
	verdicts := make([]CQVerdict, 0, len(cqs))
	unknown := 0
	for i, cq := range cqs {
		notify(i, fmt.Sprintf("第 %d/%d 条：判定中", i+1, len(cqs)))
		prompt := strings.Replace(strings.Replace(coveragePrompt, "{verbalisation}", verbalisation, 1), "{cq}", cq, 1)
		v := CQVerdict{CQ: cq, Verdict: "Unknown", Explanation: ""}
		raw, _, err := e.LLM.Chat(ctx, prompt, coverageVerdictSchema)
		if err == nil {
			var out struct {
				Verdict     string `json:"verdict"`
				Explanation string `json:"explanation"`
			}
			if perr := json.Unmarshal([]byte(raw), &out); perr == nil &&
				(out.Verdict == "Yes" || out.Verdict == "No") {
				v.Verdict = out.Verdict
				v.Explanation = strings.TrimSpace(out.Explanation)
			} else {
				v.Explanation = "输出解析失败"
				unknown++
			}
		} else {
			v.Explanation = "调用失败：" + err.Error()
			unknown++
		}
		verdicts = append(verdicts, v)
	}
	notify(len(cqs), "判定完成")
	if unknown == len(cqs) {
		return nil, fmt.Errorf("全部 %d 条判定失败，请检查模型连接后重试", len(cqs))
	}
	return verdicts, nil
}

// CoverageSummary 快筛汇总。
type CoverageSummary struct {
	Verdicts []CQVerdict `json:"verdicts"`
	Passed   int         `json:"passed"`
	Total    int         `json:"total"`
	PassRate float64     `json:"pass_rate"`
}

// SummarizeCoverage 汇总判定（Unknown 不计通过）。
func SummarizeCoverage(verdicts []CQVerdict) CoverageSummary {
	passed := 0
	for _, v := range verdicts {
		if v.Verdict == "Yes" {
			passed++
		}
	}
	total := len(verdicts)
	rate := 0.0
	if total > 0 {
		rate = float64(passed) / float64(total)
	}
	return CoverageSummary{Verdicts: verdicts, Passed: passed, Total: total, PassRate: rate}
}
