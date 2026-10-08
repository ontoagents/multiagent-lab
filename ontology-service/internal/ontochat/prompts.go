// prompts.go OntoChat 提示词集中管理与页面只读透出（REQ-271⑥，2026-10-08 定案：只显示不可修改）。
// 单一事实源：本清单既注入引擎/生成器（同包引用 + llmcreate 导出常量），也经
// GET /api/ontochat/prompts 原样序列化给前端只读渲染——页面显示的=运行时注入的同一份数据。
// REQ-272~275 新增阶段（story/抽取/聚类/测试）的提示词统一落此处续行，Source 标注 63 号 P 编号溯源。
package ontochat

import (
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

// PromptDef 单条提示词（只读透出形态；Text 为模板骨架，{占位符} 标注运行时动态段）。
type PromptDef struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Purpose string `json:"purpose"`
	Source  string `json:"source"`
	Text    string `json:"text"`
}

// Prompts 提示词清单（GET /api/ontochat/prompts 返回值；增删提示词只改本函数）。
func Prompts() []PromptDef {
	return []PromptDef{
		{
			ID:      "spec_generation",
			Label:   "草稿生成提示词",
			Purpose: "draft/refine 生成轮：领域描述 + 累积上下文 → spec_json 草稿（生成→结构校验→质量门禁错误级回喂，≤3 轮；few-shot 范例段按领域关键词注入）",
			Source:  "自研（REQ-82 生成契约）；对照 63 号 §2.2：开源 OntoChat 生成后无结构校验环",
			Text: llmcreate.SpecRolePrompt + llmcreate.SpecSchemaHint + llmcreate.SpecRulesPrompt +
				"{few-shot 范例段（无命中种子时不注入）}\n\n领域描述：\n{领域描述（首轮录入）}\n\n补充要求：\n{全部 hints——能力问题+逐轮补充+修正意见，全量拼接，无截断}" +
				"\n\n上一稿存在以下校验错误，请修正后重新输出完整 spec_json：\n{错误清单（回喂轮才有）}",
		},
		{
			ID:      "domain_sufficient",
			Label:   "领域归纳提示词",
			Purpose: "domain 补全轮：归纳本轮要点 + 判断信息是否足以生成 + 引导补充缺口",
			Source:  "自研（REQ-103 模式 A）",
			Text:    domainSufficientPrompt,
		},
		{
			ID:      "few_shot_example",
			Label:   "种子 few-shot 范例段",
			Purpose: "生成轮按领域关键词挑选最相近内置种子的紧凑片段（概念 ≤4、关系 ≤3，截断防 prompt 膨胀）；无命中种子时整段不注入",
			Source:  llmcreate.FewShotHeaderPrompt + "范例取自 internal/seed/examples 五份内置示例（med_common/gene_core/defects/orgs/failure；REQ-247/G4 设计，REQ-271⑤ 接线转正——M70 声称注入但实际未进 buildPrompt）",
			Text:    llmcreate.FewShotHeaderPrompt + "\n```json\n{\"concepts\":[最相近种子前 4 个概念完整 JSON],\"relations\":[前 3 条关系完整 JSON]}\n```",
		},
		{
			ID:      "cq_extraction",
			Label:   "CQ 抽取提示词",
			Purpose: "从领域描述与累积补充信息抽取能力问题候选，应用两净化算子（拆非原子问题/命名实体抽象）后输出 JSON 数组，供人工确认编辑后写入会话（生成草稿时回写 spec.CQ 入资产）",
			Source:  "63 号 P5 蓝本（King-s KG Lab functions.py/ontolib.py；论文 §1.3 模块 2）+中文适配——论文全英文评估，中文有效性以真机验证为准（63 号 §6.3）；v1 两算子合一次调用（论文为分步，诚实标注）",
			Text:    cqExtractPrompt,
		},
		{
			ID:      "cq_analyze",
			Label:   "CQ 去重与聚类提示词",
			Purpose: "对会话确认后的 CQ 清单做 paraphrase 去重（语义等价合并保留最清晰表述）+ 主题聚类（带中文标签簇，可选簇数）；产出经人工确认应用写回会话 CQs",
			Source:  "63 号 P6 蓝本（King-s KG Lab analysis.py::llm_cq_clustering；论文 §1.3 模块 3）+中文适配——论文 paraphrase 与聚类为两步，v1 合步；诚实边界=聚类不能单独支撑完整分析（论文 62.5% 直观度），须人工确认",
			Text:    strings.Replace(strings.Replace(cqAnalyzePrompt, "{clusterCount}", clusterCountAuto, 1), "{cqs}", "1. 问题一\n2. 问题二\n……", 1),
		},
		{
			ID:      "cq_coverage_test",
			Label:   "CQ 覆盖测试判定提示词",
			Purpose: "覆盖测试快筛（默认关，质量卡显式开启）：本体口语化文本+单条 CQ → Yes/No 判定附解释；每条 CQ 独立调用防泄漏（成本=N 次调用，入口有成本预估警告）",
			Source:  "63 号 P7/P8 蓝本（King-s KG Lab ontolib.py::cqt_prompt_a/b + verbaliser.py；论文 §1.3 模块 4）+中文适配；诚实边界=口语化判定对「可推断但不显式」需求有系统性乐观偏误（论文 87.5% 系 56 条小样本），存疑项转 REQ-255 SPARQL 精判",
			Text:    coveragePrompt,
		},
		{
			ID:      "ontology_llm_system",
			Label:   "平台结构化生成 system 提示",
			Purpose: "主平台 /api/ontology-llm/generate 通道的 system 角色：要求只输出符合 Schema 的 JSON（草稿与归纳轮共用同一代理）",
			Source:  "backend chat.GenerateStructured（模型能力归主平台；多协议连接经设置-模型连接切换）",
			Text:    "你是结构化数据生成器。请严格按给定的 JSON Schema 生成一份符合结构的 JSON 草稿。\n要求：只输出一个 JSON 对象，不要输出任何解释、Markdown 代码围栏或其他文本。\n\nJSON Schema:\n{schema}",
		},
	}
}
