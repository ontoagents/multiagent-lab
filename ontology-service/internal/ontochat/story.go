// story.go 用户故事共创（REQ-275/S4+S5，63 号 §1.3 模块 1 + P1~P4 蓝本中文适配）。
// v1 实现口径（诚实标注）：访谈问题采用**脚本化一问一轮**（P4 预定义问题五问，严格满足
// 论文交互不变式：一问一轮/未确认不进下一题/可回退/跳过显式标注），LLM 用于 one-shot 汇总
// 与精修轮（P1 汇总蓝本）；「LLM 依当前故事推荐下一步」v1 以汇总时的 suggest 字段落点。
// 引导卡=P3 参与式提示 10 模板（中文适配）经 GET /api/ontochat/story-templates 只读透出，
// 点击填入输入框（模板是辅助非必填——2408.15256 Discussion：模板机制不应成为认知负担）。
package ontochat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// StoryQuestions 访谈五问（P4 预定义问题子集：Persona/Goal/Scenario/Data/Outcomes）。
var StoryQuestions = []string{
	"第 1/5 问 · 角色（Persona）：典型用户是谁？姓名/职业/技能/兴趣（可虚构，例：「实验员小王，3 年测序经验，熟悉 FASTQ 质控」）",
	"第 2/5 问 · 目标（Goal）：这位用户要用本体达成什么目标？为什么重要？",
	"第 3/5 问 · 场景（Scenario）：现在是怎么完成这件事的？哪个环节最痛？",
	"第 4/5 问 · 数据（Data）：手头有哪些数据/文档/记录可以作为本体素材？（没有可回复「跳过」）",
	"第 5/5 问 · 期望（Outcomes）：希望建好的本体最终能回答哪些问题、支撑什么产出？",
}

// storyTemplates 引导卡模板（P3 参与式提示 10 条中文适配；**必填**/*可选* 占位符语法沿论文）。
var StoryTemplates = []struct{ Label, Text string }{
	{"领域", "本本体服务的领域是 **[领域名称]**，涉及 **[关键主题 1~3 个]**。"},
	{"角色", "典型用户是 **[姓名/角色]**，职业为 **[职业]**，具备 **[技能/兴趣]**。"},
	{"用户目标", "该用户希望 **[达成什么]**，因为 **[为什么重要]**。"},
	{"行为", "用户日常会执行 **[动作 1/动作 2/动作 3]** 等操作。"},
	{"关键词", "领域核心关键词：**[词 1]、[词 2]、[词 3]**。"},
	{"现状方法", "目前用户通过 **[现有方法/工具]** 完成工作，存在 **[不足]**。"},
	{"挑战", "当前最大的挑战是 *[[简要描述痛点，可选：一两句话]]*。"},
	{"新方法", "期望借助本体实现 **[新方法/改进]**。"},
	{"产出", "期望产出：*[要点式/段落式，可选]* **[查询能力/报告/决策支持]**。"},
	{"数据资源", "可用数据资源：**[文档/数据库/表格]**，覆盖 *[范围，可选]*。"},
}

// storySummaryPrompt one-shot 汇总（P1 蓝本：Persona/Goal/Scenario/Data 结构化用户故事）。
const storySummaryPrompt = `你是本体需求工程师。请把访谈收集到的信息整理成一份结构化用户故事（中文，Markdown 小节）：
## 角色（Persona）
## 目标（Goal）
## 场景（Scenario，含痛点）
## 数据资源（Data）
## 期望产出（Outcomes）

要求：只使用访谈中用户给出的信息（标注「跳过」的小节写「（本次未提及）」），不臆造；语言凝练（每节 1~3 句）。
只输出 JSON 对象：{"story":"完整 Markdown 用户故事","suggest":"给后续 CQ 抽取与建模的一句话建议"}`

const storySummarySchema = `{"type":"object","required":["story","suggest"],"properties":{"story":{"type":"string"},"suggest":{"type":"string"}}}`

// StoryStepQuestion 当前步骤对应的提问文案（step 越界返回空）。
func StoryStepQuestion(step int) string {
	if step < 0 || step >= len(StoryQuestions) {
		return ""
	}
	return StoryQuestions[step]
}

// summarizeStory 访谈答案 one-shot 汇总（LLM）。
func (e *Engine) summarizeStory(ctx context.Context, sess *Session) (story, suggest string, err error) {
	var b strings.Builder
	for i, q := range StoryQuestions {
		ans := "（跳过）"
		if i < len(sess.Context.StoryAnswers) && strings.TrimSpace(sess.Context.StoryAnswers[i]) != "" {
			ans = sess.Context.StoryAnswers[i]
		}
		fmt.Fprintf(&b, "\n%s\n回答：%s\n", q, ans)
	}
	if sess.Context.Description != "" {
		b.WriteString("\n领域描述（首轮录入）：" + sess.Context.Description + "\n")
	}
	prompt := storySummaryPrompt + "\n\n访谈记录：" + b.String()
	raw, _, err := e.LLM.Chat(ctx, prompt, storySummarySchema)
	if err != nil {
		return "", "", err
	}
	var out struct {
		Story   string `json:"story"`
		Suggest string `json:"suggest"`
	}
	if perr := json.Unmarshal([]byte(raw), &out); perr != nil || strings.TrimSpace(out.Story) == "" {
		return "", "", fmt.Errorf("用户故事汇总解析失败：%v", perr)
	}
	return strings.TrimSpace(out.Story), strings.TrimSpace(out.Suggest), nil
}

// reviseStory 精修轮：按用户意见修订既有用户故事。
func (e *Engine) reviseStory(ctx context.Context, sess *Session, feedback string) (string, error) {
	prompt := "你是本体需求工程师。请按用户意见修订以下用户故事（保持 Markdown 小节结构，不臆造未提及的信息）：\n\n用户意见：" +
		feedback + "\n\n当前用户故事：\n" + sess.Context.DraftStory +
		"\n\n只输出 JSON 对象：{\"story\":\"修订后的完整用户故事\"}"
	raw, _, err := e.LLM.Chat(ctx, prompt, `{"type":"object","required":["story"],"properties":{"story":{"type":"string"}}}`)
	if err != nil {
		return "", err
	}
	var out struct {
		Story string `json:"story"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil || strings.TrimSpace(out.Story) == "" {
		return "", fmt.Errorf("精修输出解析失败")
	}
	return strings.TrimSpace(out.Story), nil
}
