// purify.go CQ 净化轮（REQ-342/OC-1，63 号 §1.3 模块 2 步 3 的分步化落地）：
// 抽取轮（cq.go，只抽不净化）之后，净化轮单独一次 LLM 调用应用两算子——
// ①拆非原子问题（few-shot：复合问题拆成单一焦点多条）②命名实体抽象（专名替换为类概念词）。
// 论文为「抽取→拆分→抽象」分步；REQ-272 v1 合步（诚实标注），本轮对齐论文分步并把
// 拆分/抽象率统计透出（83 号 OC-1 验收口径：复合 CQ 拆分率+专名抽象率人工核验）。
// 净化是增强不是依赖：净化轮重试后仍失败→降级用抽取原清单（purification_degraded 如实标注）。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// purifySchema 净化轮输出契约：refined 清单 + stats 统计（拆分率/抽象率的分子分母）。
const purifySchema = `{"type":"object","required":["refined","stats"],"properties":{
"refined":{"type":"array","items":{"type":"object","required":["cq","origin"],"properties":{"cq":{"type":"string"},"origin":{"type":"string","enum":["抽取","拆分","抽象"]}}}},
"stats":{"type":"object","properties":{"compound_found":{"type":"integer"},"named_found":{"type":"integer"}}}}}`

// purifyPrompt 净化轮提示词（63 号 P5 两算子 few-shot 中文适配，独立成步）。
const purifyPrompt = `你是本体需求工程师。请对下方能力问题（CQ）候选清单做净化（两个算子一次性应用）：

算子一·拆非原子问题：复合问题（一问多焦点）拆成多条单一焦点的问题。
例：「青霉素的发现者是谁、能治哪些疾病？」拆为「药物由哪些人员发现？」「药物可治疗哪些疾病？」。

算子二·命名实体抽象：把具体专名替换为类概念词，使问题成为可复用的模式。
例：「青霉素可治疗哪些疾病？」→「药物可治疗哪些疾病？」。

要求：
- 净化后逐条输出，不丢失任何信息点（拆分产生的新条目 origin=「拆分」；经专名抽象的条目 origin=「抽象」；未变动的 origin=「抽取」）；
- 每条 CQ 一句话、单一焦点、以「？」结尾；
- stats.compound_found=识别出的复合问题条数；stats.named_found=含具体专名的条数；
- 不要新增清单之外的问题，不要遗漏任何条目；
- 只输出 JSON 对象，不要 markdown 代码块或其他文本。

CQ 候选清单：
{cqs}`

// PurifyStats 净化统计（拆分率/抽象率的分子分母；透出前端与 job result 供人工核验）。
type PurifyStats struct {
	CompoundFound int `json:"compound_found"` // 识别复合问题数（拆分算子分母）
	SplitOut      int `json:"split_out"`      // 拆分产出条数（拆分算子分子）
	NamedFound    int `json:"named_found"`    // 含专名条数（抽象算子分母）
	Abstracted    int `json:"abstracted"`     // 抽象产出条数（抽象算子分子）
}

// purifyOutcome 净化轮结果（batchDup=净化输出批内去重滤除数——并入抽取结果的总 dedup 口径）。
type purifyOutcome struct {
	refined  []ExtractedCQ
	stats    *PurifyStats
	batchDup int
	degraded bool
	err      error
}

// purifyCQs 净化轮主循环：解析失败带错误回喂重试 ≤2 轮，仍失败降级返回原清单（degraded=true）。
func purifyCQs(ctx context.Context, llm purifyLLM, cqs []ExtractedCQ, notify func(string)) purifyOutcome {
	items := make([]string, len(cqs))
	for i, c := range cqs {
		items[i] = fmt.Sprintf("%d. %s", i+1, c.CQ)
	}
	prompt := strings.Replace(purifyPrompt, "{cqs}", strings.Join(items, "\n"), 1)
	for round := 1; round <= 2; round++ {
		if notify != nil {
			notify(fmt.Sprintf("净化轮（第 %d/2 轮）：拆非原子+命名实体抽象", round))
		}
		raw, _, err := llm.Chat(ctx, prompt, purifySchema)
		if err != nil {
			return purifyOutcome{degraded: true, err: err}
		}
		var out struct {
			Refined []ExtractedCQ `json:"refined"`
			Stats   PurifyStats   `json:"stats"`
		}
		if perr := json.Unmarshal([]byte(raw), &out); perr != nil || len(out.Refined) == 0 {
			if round == 2 {
				// 净化失败降级：抽取原清单保留（净化是增强不是依赖——结果如实标注降级）
				if notify != nil {
					notify("净化轮输出不合法，降级使用抽取原清单")
				}
				return purifyOutcome{refined: cqs, degraded: true}
			}
			if notify != nil {
				notify("净化轮输出不是合法 JSON，回喂重试")
			}
			prompt += "\n\n注意：上一次输出不是合法的 {\"refined\":[...],\"stats\":{...}} JSON 对象，请严格只输出该 JSON 结构。"
			continue
		}
		// 净化输出净化：trim/去空/批内去重（沿用 dedupeCQs 口径——模型漏报/重复行兜底）
		cleaned, dup, _ := dedupeCQs(nil, out.Refined)
		if len(cleaned) == 0 {
			if notify != nil {
				notify("净化轮无有效条目，降级使用抽取原清单")
			}
			return purifyOutcome{refined: cqs, degraded: true}
		}
		// 净化统计在净化原始产出上计（去重前——反映净化算子本身的工作量，与后续既有重复滤除无关）
		stats := out.Stats
		for _, c := range out.Refined {
			switch c.Origin {
			case "拆分":
				stats.SplitOut++
			case "抽象":
				stats.Abstracted++
			}
		}
		return purifyOutcome{refined: cleaned, stats: &stats, batchDup: dup}
	}
	return purifyOutcome{refined: cqs, degraded: true}
}

// purifyLLM 净化轮模型通道（llmcreate.Creator 的 Chat 子集，测试可桩）。
type purifyLLM interface {
	Chat(ctx context.Context, prompt, schema string) (draft string, usage any, err error)
}
