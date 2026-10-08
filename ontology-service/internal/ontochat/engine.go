// engine.go OntoChat 多轮引导引擎（REQ-103 模式 A）。
// 职责：按会话阶段组装 prompt → 经 llmcreate.Creator 调主平台生成 → 解析意图 → 推进状态机。
// 阶段语义（与 14 号方案 §5 OntoChat 流程页一致）：
//
//	cq     首轮：用户给领域描述 + 能力问题列表 → 引擎确认并提示下一步
//	domain 补全轮：用户逐轮补充领域信息 → 引擎归纳要点，询问是否足够生成
//	draft  生成轮：把累积上下文整体喂给生成器产出 spec 草稿（复用 llmcreate 校验循环）
//	refine 修正轮：草稿校验错误回喂 → 重新生成（引擎内已含，最多 3 轮）
//
// REQ-271/M80：ctx 贯穿（异步 job 取消依赖）；Turn 增可选 onProgress 回调（生成-校验环
// 轮次透出给异步 job）；WillGenerate 导出供 REST 层判定同步/异步路径。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

// Engine 多轮引导引擎（复用 llmcreate.Creator 的平台代理与校验循环）。
type Engine struct {
	LLM *llmcreate.Creator
}

// TurnResult 单轮交互结果：assistant 回复 + 可选草稿（draft/refine 阶段产出）。
type TurnResult struct {
	Reply     string        // assistant 消息
	NextStage string        // 推进后的阶段
	Draft     *pkgspec.Spec // 非 nil 表示本轮产出了草稿（含校验警告）
	Warning   string        // 草稿带警告（如达最大修正轮数仍有错误）
}

// RoundProgress(round, note) 生成-校验环轮次进度回调（异步 job 进度透出，REQ-271）。
type RoundProgress func(round int, note string)

// domainSufficientPrompt 补全轮的归纳 prompt：让模型归纳已给信息并判断是否可生成。
const domainSufficientPrompt = `你是本体建模访谈专家。用户正在逐步补充领域信息，请：
1. 用 2~4 条要点归纳用户本轮补充的信息（不要重复已有要点）；
2. 判断当前信息是否足以生成一个可用的本体草稿；
3. 若不足，明确列出还缺什么（如：关键概念间的关系、实例来源、层级深度），用一句话引导用户补充；
4. 若已足够，回复以「可以生成」开头，并简述你准备建模的概念范围。
要求：中文、简洁（150 字内）、不要输出 JSON。`

// WillGenerate 判定该轮将进入生成器（REST 层同步/异步路径判定依据，REQ-271）：
// draft/refine 阶段任意输入（修正意见/空文本重生成）都触发生成；domain 阶段命中生成意图才生成；
// cq 阶段纯本地解析不生成。
func WillGenerate(stage, userText string) bool {
	switch stage {
	case "draft", "refine":
		return true
	case "domain":
		return isGenerateIntent(strings.TrimSpace(userText))
	default:
		return false
	}
}

// Turn 处理一轮用户输入，推进状态机并落库。
// ctx：平台代理调用取消（异步 job cancel / 请求断连）；onProgress：可选生成轮次进度回调。
func (e *Engine) Turn(ctx context.Context, st *Store, sess *Session, userText string, onProgress ...RoundProgress) (*TurnResult, error) {
	switch sess.Stage {
	case "story":
		return e.turnStory(ctx, st, sess, userText)
	case "cq":
		return e.turnCQ(st, sess, userText)
	case "domain":
		return e.turnDomain(ctx, st, sess, userText, onProgress...)
	case "draft", "refine":
		// REQ-246/G5 修复「refine 前端断链」：此阶段用户文本此前被 turnDraft 完全忽略——
		// 界面承诺「回复修改意见进入修正轮」实际不生效。现非空文本一律作为修正意见走 Refine
		// （并入 Hints 重生成）；纯「生成」意图或空文本才直接重生成。
		if fb := strings.TrimSpace(userText); fb != "" && !isGenerateIntent(fb) {
			return e.Refine(ctx, st, sess, fb, onProgress...)
		}
		return e.turnDraft(ctx, st, sess, onProgress...)
	default:
		return nil, fmt.Errorf("会话已结束（stage=done），如需继续请新建会话")
	}
}

// turnStory 访谈轮（REQ-275）：脚本化一问一轮（StoryQuestions，交互不变式照 63 号 §5.3——
// 一问一轮/未确认不进下一题/可回退由 /story-back 承载/「跳过」显式留痕）；五问毕 one-shot
// 汇总（LLM）生成用户故事；此后该阶段文本一律为精修意见（LLM 修订 DraftStory）。
func (e *Engine) turnStory(ctx context.Context, st *Store, sess *Session, userText string) (*TurnResult, error) {
	text := strings.TrimSpace(userText)
	// 精修轮：已有用户故事 → 文本即修订意见
	if sess.Context.DraftStory != "" {
		if text == "" {
			return nil, fmt.Errorf("请输入对用户故事的修改意见，或点击「完成并抽取 CQ」进入能力问题抽取")
		}
		revised, err := e.reviseStory(ctx, sess, text)
		if err != nil {
			return nil, err
		}
		sess.Context.DraftStory = revised
		reply := "用户故事已按意见修订。可继续回复修改意见，或点击「完成并抽取 CQ」进入能力问题抽取。"
		if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, nil, nil, &sess.Context); err != nil {
			return nil, err
		}
		return &TurnResult{Reply: reply, NextStage: "story"}, nil
	}
	// 访谈轮：记录回答 → 下一问 / 汇总
	if text == "" {
		return nil, fmt.Errorf("请回答当前问题（或点「跳过」）")
	}
	step := sess.Context.StoryStep
	if step < 0 || step >= len(StoryQuestions) {
		step = len(StoryQuestions) - 1
	}
	for len(sess.Context.StoryAnswers) <= step {
		sess.Context.StoryAnswers = append(sess.Context.StoryAnswers, "")
	}
	sess.Context.StoryAnswers[step] = text
	sess.Context.StoryStep = step + 1
	var reply string
	if sess.Context.StoryStep < len(StoryQuestions) {
		reply = StoryQuestions[sess.Context.StoryStep] + "\n\n（回答完自动进入下一问；点「上一步」回退，无相关信息可点「跳过」）"
	} else {
		story, suggest, err := e.summarizeStory(ctx, sess)
		if err != nil {
			// 汇总失败：退回本问重答（错误经 500 留痕，用户重发即重试汇总）
			sess.Context.StoryStep = step
			sess.Context.StoryAnswers = sess.Context.StoryAnswers[:step]
			return nil, err
		}
		sess.Context.DraftStory = story
		reply = "用户故事已生成（见右侧卡片）：\n\n" + truncate(story, 600) +
			"\n\n后续建议：" + truncate(suggest, 160) +
			"\n\n可直接回复修改意见精修；满意后点「完成并抽取 CQ」进入能力问题抽取（复用 REQ-272 确认卡）。"
	}
	stage := "story"
	round := sess.Round + 1
	if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, &stage, &round, &sess.Context); err != nil {
		return nil, err
	}
	return &TurnResult{Reply: reply, NextStage: stage}, nil
}

// StoryBack 访气回退一步（REQ-275 交互不变式③：可回退）：清草稿、步数-1 并重发该问。
func (e *Engine) StoryBack(st *Store, sess *Session) (*TurnResult, error) {
	step := sess.Context.StoryStep
	if sess.Context.DraftStory != "" {
		sess.Context.DraftStory = ""
		step = len(StoryQuestions) - 1 // 汇总后回退=回末问重答
	} else if step > 0 {
		step--
	}
	sess.Context.StoryStep = step
	reply := StoryStepQuestion(step) + "\n\n（已回退，请重新回答）"
	stage := "story"
	if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, &stage, nil, &sess.Context); err != nil {
		return nil, err
	}
	return &TurnResult{Reply: reply, NextStage: stage}, nil
}

// turnCQ 首轮：领域描述（必填）+ 能力问题（每行一个，可选）。
func (e *Engine) turnCQ(st *Store, sess *Session, userText string) (*TurnResult, error) {
	text := strings.TrimSpace(userText)
	if text == "" {
		return nil, fmt.Errorf("请先描述要建模的领域")
	}
	// 约定格式：首行为领域描述，其余行（或「能力问题：」后的行）为 CQ
	lines := strings.Split(text, "\n")
	desc := strings.TrimSpace(lines[0])
	cqs := []string{}
	inCQ := false
	for _, ln := range lines[1:] {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "能力问题") {
			inCQ = true
			t = strings.TrimPrefix(strings.TrimPrefix(t, "能力问题"), "：")
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
		}
		if inCQ {
			cqs = append(cqs, strings.TrimLeft(t, "0123456789.、) "))
		} else if desc == "" {
			desc = t
		} else {
			cqs = append(cqs, strings.TrimLeft(t, "0123456789.、) "))
		}
	}
	if desc == "" {
		return nil, fmt.Errorf("未能识别领域描述，请把描述放在第一行")
	}
	sess.Context.Description = desc
	sess.Context.CQs = cqs

	var b strings.Builder
	b.WriteString("已记录领域描述：")
	b.WriteString(truncate(desc, 80))
	if len(cqs) > 0 {
		fmt.Fprintf(&b, "\n已记录能力问题 %d 条：", len(cqs))
		for i, q := range cqs {
			fmt.Fprintf(&b, "\n%d. %s", i+1, truncate(q, 60))
		}
	} else {
		b.WriteString("\n尚未提供能力问题（可选）。建议列 3~5 个本体要回答的问题，能显著提升建模质量；也可以直接进入补全阶段。")
	}
	b.WriteString("\n\n下一步：请继续补充领域信息（关键概念、层级、关系、实例来源等），补充充分后点击「生成草稿」即可产出 spec_json。")
	reply := b.String()
	stage := "domain"
	round := 0
	if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, &stage, &round, &sess.Context); err != nil {
		return nil, err
	}
	return &TurnResult{Reply: reply, NextStage: stage}, nil
}

// turnDomain 补全轮：归纳要点；用户说「生成草稿」则直接进入 draft（onProgress 透传，REQ-271）。
func (e *Engine) turnDomain(ctx context.Context, st *Store, sess *Session, userText string, onProgress ...RoundProgress) (*TurnResult, error) {
	text := strings.TrimSpace(userText)
	if text == "" {
		return nil, fmt.Errorf("请输入内容：补充领域信息，或点击「生成草稿」")
	}
	if isGenerateIntent(text) {
		return e.turnDraft(ctx, st, sess, onProgress...)
	}
	sess.Context.Hints = append(sess.Context.Hints, text)

	// 归纳 prompt：把已累积上下文给模型，要要点归纳 + 是否足够判断
	var b strings.Builder
	b.WriteString(domainSufficientPrompt)
	b.WriteString("\n\n领域描述：\n" + sess.Context.Description)
	if len(sess.Context.CQs) > 0 {
		b.WriteString("\n\n能力问题：")
		for i, q := range sess.Context.CQs {
			fmt.Fprintf(&b, "\n%d. %s", i+1, q)
		}
	}
	if len(sess.Context.Hints) > 0 {
		b.WriteString("\n\n已补充的信息：")
		for i, h := range sess.Context.Hints {
			fmt.Fprintf(&b, "\n%d. %s", i+1, h)
		}
	}
	reply, _, err := e.LLM.RawChat(ctx, b.String())
	if err != nil {
		return nil, err
	}
	round := sess.Round + 1
	stage := "domain"
	if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, &stage, &round, &sess.Context); err != nil {
		return nil, err
	}
	return &TurnResult{Reply: reply, NextStage: stage}, nil
}

// turnDraft 生成轮：累积上下文 → llmcreate.Draft（内含校验回喂循环，最多 3 轮）。
func (e *Engine) turnDraft(ctx context.Context, st *Store, sess *Session, onProgress ...RoundProgress) (*TurnResult, error) {
	// 组装 extraHint：逐轮补全要点（CQ 不再拼入 hints——改经 DraftWithCQ 的 cqs 参数进
	// CQ 强调块并回写 spec.CQ，REQ-272 闭合 REQ-248 OntoChat 路径缺口；避免双份重复）
	hints := append([]string(nil), sess.Context.Hints...)
	cqs := sess.Context.CQs
	rounds := make([]func(int, string), 0, len(onProgress))
	for _, p := range onProgress {
		if p != nil {
			rounds = append(rounds, func(r int, msg string) { p(r, msg) })
		}
	}
	res, err := e.LLM.DraftWithCQ(ctx, sess.Context.Description, strings.Join(hints, "\n\n"), cqs, rounds...)
	if err != nil && res == nil {
		return nil, err
	}
	raw, _ := json.Marshal(res.Spec)
	rm := json.RawMessage(raw)
	sess.Context.DraftSpec = &rm

	stage := "draft"
	var warn string
	if err != nil {
		warn = err.Error() // 达最大修正轮数仍有错误 → 草稿供预览参考
		stage = "refine"
	}
	reply := fmt.Sprintf("草稿已生成：概念 %d、关系 %d、实例 %d（生成-校验循环 %d 轮）。\n请在右侧预览确认：可直接入库，或回复修改意见进入修正轮。",
		len(res.Spec.Concepts), len(res.Spec.Relations), len(res.Spec.Instances), res.Rounds)
	if warn != "" {
		reply += "\n注意：" + warn
	}
	round := sess.Round + 1
	if err := st.Append(sess.ID, Message{Role: "assistant", Content: reply}, &stage, &round, &sess.Context); err != nil {
		return nil, err
	}
	return &TurnResult{Reply: reply, NextStage: stage, Draft: res.Spec, Warning: warn}, nil
}

// Refine 修正轮：用户修改意见 + 上稿校验错误回喂重新生成（由 turnDraft 复用：把意见并入 Hints 后再生成）。
func (e *Engine) Refine(ctx context.Context, st *Store, sess *Session, feedback string, onProgress ...RoundProgress) (*TurnResult, error) {
	if fb := strings.TrimSpace(feedback); fb != "" {
		sess.Context.Hints = append(sess.Context.Hints, "修正意见："+fb)
	}
	return e.turnDraft(ctx, st, sess, onProgress...)
}

// isGenerateIntent 生成意图识别（REQ-271 放宽：去 ≤12 rune 硬阈）。
// 规则：精确短指令直接命中；含「生成」且 ≤12 rune 的短句命中（沿 REQ-246/G5 口径）；
// 含「生成」且出现「草稿/出稿/draft/spec」目标词的长句也命中（如「信息差不多了，帮我生成草稿吧」）；
// 含「生成」但无目标词的长句视为普通建模输入（如「生成关系的设计思路」，避免误触发分钟级生成）。
func isGenerateIntent(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	if t == "" {
		return false
	}
	if t == "generate" || t == "draft" || t == "出稿" || t == "出草稿" {
		return true
	}
	if !strings.Contains(t, "生成") {
		return false
	}
	if len([]rune(t)) <= 12 {
		return true
	}
	return strings.Contains(t, "草稿") || strings.Contains(t, "出稿") ||
		strings.Contains(t, "draft") || strings.Contains(t, "spec")
}

func joinNumbered(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = fmt.Sprintf("%d. %s", i+1, s)
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
