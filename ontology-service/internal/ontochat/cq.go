// cq.go CQ 抽取与净化（REQ-272/S1，63 号 §1.3 模块 2 + P5 蓝本中文适配）。
// 流程：领域描述+累积补充信息 → LLM 抽取候选并应用两个净化算子（拆非原子问题/命名实体
// 抽象）→ 与既有 CQ 精确去重 → 人工确认编辑（analyze 确认步定案保留）→ 写回 Context.CQs
// → 生成轮 DraftWithCQ 回写 spec.CQ（REQ-248 缺口闭合）。
// 诚实边界：论文为「抽取→拆分→抽象」分步且全英文评估；v1 两算子合一次调用，中文有效性
// 以真机验证为准（63 号 §6.3 待验证项）。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// cqSchema CQ 抽取输出契约。
const cqSchema = `{"type":"array","items":{"type":"object","required":["cq"],"properties":{"cq":{"type":"string"},"origin":{"type":"string","enum":["抽取","拆分","抽象"]}}}}`

// cqExtractPrompt CQ 抽取提示词（63 号 P5 蓝本中文适配；两净化算子内联为输出要求）。
const cqExtractPrompt = `你是本体需求工程师。请从下方材料中抽取能力问题（Competency Question，CQ）——即未来本体/知识库应当能够回答的问题。

按三步处理（一次性完成，输出净化后的最终清单）：
1. 抽取：从材料中找出值得建模回答的问题；
2. 拆非原子问题：复合问题拆成多条单一焦点的问题（例：「青霉素的发现者是谁、能治哪些病？」拆为「药物由哪些人员发现？」「药物可治疗哪些疾病？」）；
3. 命名实体抽象：把具体专名替换为类概念词，使问题成为可复用的模式（例：「青霉素可治疗哪些疾病？」→「药物可治疗哪些疾病？」）。

要求：
- 每条 CQ 一句话、单一焦点、以「？」结尾；
- 数量 3~10 条，覆盖材料中不同的建模关注点（概念、层级、关系、属性、流程、质量评价等）；
- origin 标注来源：直接抽取=「抽取」；由复合问题拆出=「拆分」；经专名抽象=「抽象」；
- 已有能力问题清单中的问题不要重复输出；
- 只输出 JSON 数组，不要 markdown 代码块或其他文本。

已有能力问题（勿重复）：
{existing}

材料（领域描述与已补充信息）：
{material}`

// ExtractedCQ 单条抽取候选（origin=抽取|拆分|抽象）。
type ExtractedCQ struct {
	CQ     string `json:"cq"`
	Origin string `json:"origin,omitempty"`
}

// ExtractCQs 抽取 CQ 候选：解析失败带错误回喂重试 ≤2 轮；与既有 CQ 精确去重。
// 返回（候选清单, 与既有重复被滤除的条数, error）。
func (e *Engine) ExtractCQs(ctx context.Context, sess *Session, onProgress ...RoundProgress) ([]ExtractedCQ, int, error) {
	notify := func(msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(1, msg)
			}
		}
	}
	material := buildCQMaterial(sess)
	existing := "（无）"
	if len(sess.Context.CQs) > 0 {
		existing = joinNumbered(sess.Context.CQs)
	}
	prompt := strings.Replace(strings.Replace(cqExtractPrompt, "{existing}", existing, 1), "{material}", material, 1)
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("第 %d/2 轮：抽取能力问题中", round))
		raw, _, err := e.LLM.Chat(ctx, prompt, cqSchema)
		if err != nil {
			return nil, 0, err
		}
		var cqs []ExtractedCQ
		if perr := json.Unmarshal([]byte(raw), &cqs); perr != nil {
			if round == 2 {
				return nil, 0, fmt.Errorf("抽取输出解析失败：%w", perr)
			}
			notify("第 1 轮输出不是合法 JSON，回喂重试")
			prompt = cqExtractPrompt + "\n\n注意：上一次输出不是合法 JSON 数组（" + perr.Error() + "）。请严格只输出 JSON 数组。"
			continue
		}
		return dedupeCQs(sess.Context.CQs, cqs)
	}
	return nil, 0, fmt.Errorf("抽取循环异常退出")
}

// buildCQMaterial 抽取材料：领域描述 + 逐轮补充信息（story 制品 REQ-275 落地后并入）。
func buildCQMaterial(sess *Session) string {
	var b strings.Builder
	if ds := strings.TrimSpace(sess.Context.DraftStory); ds != "" {
		b.WriteString("用户故事（访谈制品）：\n" + ds + "\n\n")
	}
	b.WriteString(strings.TrimSpace(sess.Context.Description))
	if len(sess.Context.Hints) > 0 {
		b.WriteString("\n\n已补充信息：")
		for i, h := range sess.Context.Hints {
			fmt.Fprintf(&b, "\n%d. %s", i+1, h)
		}
	}
	return b.String()
}

// dedupeCQs 净化输出：trim/去空/批内去重 + 与既有 CQ 精确去重（ REQ-273 将做 paraphrase 级）。
func dedupeCQs(existing []string, cqs []ExtractedCQ) ([]ExtractedCQ, int, error) {
	seen := map[string]bool{}
	for _, e := range existing {
		seen[strings.TrimSpace(e)] = true
	}
	out := make([]ExtractedCQ, 0, len(cqs))
	dup := 0
	for _, c := range cqs {
		q := strings.TrimSpace(c.CQ)
		if q == "" {
			continue
		}
		c.CQ = q
		if seen[q] {
			dup++
			continue
		}
		seen[q] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, dup, fmt.Errorf("未抽取到能力问题候选，请补充更多领域信息后重试")
	}
	return out, dup, nil
}
