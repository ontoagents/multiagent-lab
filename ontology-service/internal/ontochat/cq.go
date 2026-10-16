// cq.go CQ 抽取与净化（REQ-272/S1 + REQ-342/OC-1 分步化迭代，63 号 §1.3 模块 2）。
// 流程（论文四步对齐）：抽取轮（只抽不净化）→ 净化轮（purify.go：拆非原子+命名实体抽象
// 分步单次调用+拆分/抽象率统计）→ 与既有 CQ 精确去重 → 人工确认编辑（analyze 确认步定案
// 保留）→ 写回 Context.CQs → 生成轮 DraftWithCQ 回写 spec.CQ（REQ-248 缺口闭合）。
// 诚实边界：论文全英文评估；净化轮失败降级用抽取原清单（净化是增强不是依赖）。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

// cqSchema CQ 抽取轮输出契约（只抽不净化——净化归净化轮，origin=抽取）。
const cqSchema = `{"type":"array","items":{"type":"object","required":["cq"],"properties":{"cq":{"type":"string"},"origin":{"type":"string","enum":["抽取"]}}}}`

// cqExtractPrompt CQ 抽取轮提示词（63 号 P5 抽取步；拆分/抽象两算子移交净化轮）。
const cqExtractPrompt = `你是本体需求工程师。请从下方材料中抽取能力问题（Competency Question，CQ）——即未来本体/知识库应当能够回答的问题。

要求：
- 覆盖材料中不同的建模关注点（概念、层级、关系、属性、流程、质量评价等）；
- 每条 CQ 一句话、以「？」结尾；
- 只抽取，不改写（拆分复合问题与命名实体抽象由后续净化步完成）；
- 已有能力问题清单中的问题不要重复输出；
- 只输出 JSON 数组，不要 markdown 代码块或其他文本。

已有能力问题（勿重复）：
{existing}

材料（领域描述与已补充信息）：
{material}`

// ExtractedCQ 单条抽取候选（origin=抽取|拆分|抽象；拆分/抽象由净化轮标注）。
type ExtractedCQ struct {
	CQ     string `json:"cq"`
	Origin string `json:"origin,omitempty"`
}

// CQExtractResult 抽取+净化两步的完整结果。
type CQExtractResult struct {
	CQs                  []ExtractedCQ `json:"cqs"`
	Dedup                int           `json:"dedup"`                           // 与既有/批内重复被滤除条数
	Purification         *PurifyStats  `json:"purification,omitempty"`          // 净化统计（降级时 nil）
	PurificationDegraded bool          `json:"purification_degraded,omitempty"` // 净化轮失败降级用抽取原清单
}

// ExtractCQs 抽取 CQ 候选：抽取轮（≤2 轮回喂）→ 净化轮（OC-1 分步算子）→ 与既有 CQ 精确去重。
func (e *Engine) ExtractCQs(ctx context.Context, sess *Session, onProgress ...RoundProgress) (CQExtractResult, error) {
	return extractCQsCore(ctx, e.LLM, buildCQMaterial(sess), sess.Context.CQs, onProgress...)
}

// ExtractCQsFromMaterial 无会话 CQ 抽取（REQ-308）：AI 创建路径直接消费「描述+补充材料」——
// 复用抽取+净化管线，材料组装与 OntoChat 会话解耦。
func (e *Engine) ExtractCQsFromMaterial(ctx context.Context, description string, hints, existing []string, onProgress ...RoundProgress) (CQExtractResult, error) {
	return extractCQsCore(ctx, e.LLM, buildCQMaterialFrom(description, "", hints), existing, onProgress...)
}

// extractCQsCore 两步主循环：抽取轮 existing 既有 CQ（prompt 声明勿重复+精确去重双重防线）；
// 解析失败带错误回喂重试 ≤2 轮；净化轮失败降级原清单（degraded 如实标注）。
func extractCQsCore(ctx context.Context, llm *llmcreate.Creator, material string, existing []string, onProgress ...RoundProgress) (CQExtractResult, error) {
	notify := func(msg string) {
		for _, p := range onProgress {
			if p != nil {
				p(1, msg)
			}
		}
	}
	existingStr := "（无）"
	if len(existing) > 0 {
		existingStr = joinNumbered(existing)
	}
	prompt := strings.Replace(strings.Replace(cqExtractPrompt, "{existing}", existingStr, 1), "{material}", material, 1)
	var extracted []ExtractedCQ
	for round := 1; round <= 2; round++ {
		notify(fmt.Sprintf("第 %d/2 轮：抽取能力问题中", round))
		raw, _, err := llm.Chat(ctx, prompt, cqSchema)
		if err != nil {
			return CQExtractResult{}, err
		}
		var cqs []ExtractedCQ
		if perr := json.Unmarshal([]byte(raw), &cqs); perr != nil {
			if round == 2 {
				return CQExtractResult{}, fmt.Errorf("抽取输出解析失败：%w", perr)
			}
			notify("第 1 轮输出不是合法 JSON，回喂重试")
			prompt = cqExtractPrompt + "\n\n注意：上一次输出不是合法 JSON 数组（" + perr.Error() + "）。请严格只输出 JSON 数组。"
			continue
		}
		extracted = cqs
		break
	}
	if len(extracted) == 0 {
		return CQExtractResult{}, fmt.Errorf("未抽取到能力问题候选，请补充更多领域信息后重试")
	}
	// 净化轮（OC-1 分步算子）：失败降级原清单
	out := purifyCQs(ctx, llm, extracted, notify)
	if out.err != nil {
		// 模型通道级失败：降级原清单（与输出不合法同语义——净化是增强不是依赖）
		notify("净化轮调用失败，降级使用抽取原清单")
		out = purifyOutcome{refined: extracted, degraded: true}
	}
	res, dup, err := dedupeCQs(existing, out.refined)
	dedup := dup + out.batchDup
	if err != nil {
		return CQExtractResult{Dedup: dedup, Purification: out.stats, PurificationDegraded: out.degraded}, err
	}
	return CQExtractResult{CQs: res, Dedup: dedup, Purification: out.stats, PurificationDegraded: out.degraded}, nil
}

// buildCQMaterial 抽取材料：领域描述 + 逐轮补充信息（story 制品 REQ-275 落地后并入）。
func buildCQMaterial(sess *Session) string {
	return buildCQMaterialFrom(sess.Context.Description, sess.Context.DraftStory, sess.Context.Hints)
}

// buildCQMaterialFrom 参数化材料组装（REQ-308 解 sess 化：AI 创建路径无会话复用）。
func buildCQMaterialFrom(description, story string, hints []string) string {
	var b strings.Builder
	if ds := strings.TrimSpace(story); ds != "" {
		b.WriteString("用户故事（访谈制品）：\n" + ds + "\n\n")
	}
	b.WriteString(strings.TrimSpace(description))
	if len(hints) > 0 {
		b.WriteString("\n\n已补充信息：")
		for i, h := range hints {
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
