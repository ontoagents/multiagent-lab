package ontochat

// REQ-508/M316「从材料开始」学习开局·概念停车场（100 号 R9；承 Q-23 衍生④转正——冷启动建模）：
// 学习材料 → AI 铺候选概念清单（逐条带材料原句引用 origin）。列表形态（非画布，不触 Q-7）；
// 人工勾选/改名后由前端一键成骨架进 CustomBuildFlow S2——本包只负责 LLM 铺候选，
// 不落库不建本体（LLM=草稿生产者+人工确认铁律，REQ-269）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ConceptCandidate 停车场候选概念：name=概念名（材料原语言，与 spec.concepts[].name 直接对应，
// 骨架唯一键）、label=可选英文标识、definition=一句话定义、origin=材料原句引用
// （仅停车场呈现面，不入 spec——概念级备注随版本留痕归 REQ-510）。
type ConceptCandidate struct {
	Name       string `json:"name"`
	Label      string `json:"label,omitempty"`
	Definition string `json:"definition,omitempty"`
	Origin     string `json:"origin,omitempty"`
}

// ConceptParkResult 铺候选结果。
type ConceptParkResult struct {
	Concepts  []ConceptCandidate `json:"concepts"`
	Topic     string             `json:"topic,omitempty"`     // 建议本体名（材料主题归纳，前端可改）
	Truncated int                `json:"truncated,omitempty"` // 超上限/重复被滤除条数（中性注记）
}

// conceptParkMax 候选上限（成本护栏，沿 REQ-500 CqQuickScreen「上限+中性文案」口径）。
const conceptParkMax = 30

// conceptParkSchema 停车场输出契约。
const conceptParkSchema = `{"type":"object","required":["concepts"],"properties":{"topic":{"type":"string"},"concepts":{"type":"array","items":{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"label":{"type":"string"},"definition":{"type":"string"},"origin":{"type":"string"}}}}}}`

// conceptParkPrompt 概念停车场铺候选提示词（只铺不建——骨架落库由前端确认后走人工确认链）。
const conceptParkPrompt = `你是本体建模工程师。学习者会粘贴一段学习材料，请从中铺出「概念停车场」候选清单——即这段材料中值得建模为概念的核心名词/术语/实体类型，供学习者逐条勾选后一键生成本体骨架。

要求：
- 只提取材料中实际出现或直接指涉的概念，不虚构材料外的领域实体；
- 每条给出 name（概念名，用材料原语言，简短且彼此不同）、label（可选英文标识，没有把握就省略）、definition（一句话定义，不超过 60 字）、origin（支撑该概念的材料原句引用，尽量原文短句，不超过 80 字）；
- 最多 30 条，按材料重要性排序；宁缺毋滥，同一含义不拆成多条；
- topic 用不超过 20 字归纳材料主题（作为建议本体名）；
- 只输出 JSON 对象，不要 markdown 代码块或其他文本。

学习材料：
{material}`

// MaterialConcepts 概念停车场铺候选（REQ-508）：材料 → 候选概念清单。
// 解析失败带错误回喂重试 ≤2 轮（沿 extractCQsCore 先例）；批内按 name 去重；超上限截断+如实注记。
func (e *Engine) MaterialConcepts(ctx context.Context, material string, onProgress ...RoundProgress) (ConceptParkResult, error) {
	notify := func(msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(1, msg)
			}
		}
	}
	base := strings.Replace(conceptParkPrompt, "{material}", material, 1)
	prompt := base
	var parsed struct {
		Topic    string             `json:"topic"`
		Concepts []ConceptCandidate `json:"concepts"`
	}
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("第 %d/2 轮：铺候选概念中", round))
		raw, _, err := e.LLM.Chat(ctx, prompt, conceptParkSchema)
		if err != nil {
			return ConceptParkResult{}, err
		}
		if perr := json.Unmarshal([]byte(raw), &parsed); perr != nil {
			if round == 2 {
				return ConceptParkResult{}, fmt.Errorf("铺候选输出解析失败：%w", perr)
			}
			notify("第 1 轮输出不是合法 JSON，回喂重试")
			prompt = base + "\n\n注意：上一次输出不是合法 JSON 对象（" + perr.Error() + "）。请严格只输出 JSON 对象。"
			continue
		}
		break
	}
	if len(parsed.Concepts) == 0 {
		return ConceptParkResult{}, fmt.Errorf("未铺出候选概念，请补充或更换学习材料后重试")
	}
	// 批内 name 去重（保留首条）+ 空名剔除 + 上限截断（超限如实注记，不静默丢弃）
	seen := map[string]bool{}
	out := make([]ConceptCandidate, 0, conceptParkMax)
	truncated := 0
	for _, c := range parsed.Concepts {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			continue
		}
		if seen[c.Name] {
			truncated++
			continue
		}
		seen[c.Name] = true
		if len(out) >= conceptParkMax {
			truncated++
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return ConceptParkResult{}, fmt.Errorf("候选概念清洗后为空，请更换学习材料后重试")
	}
	return ConceptParkResult{Concepts: out, Topic: strings.TrimSpace(parsed.Topic), Truncated: truncated}, nil
}
