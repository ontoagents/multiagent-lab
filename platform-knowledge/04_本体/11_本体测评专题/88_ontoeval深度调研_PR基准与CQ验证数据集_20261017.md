---
module: 本体
topic: 本体测评·ontoeval 双实体深度调研
desc: ai4curation/ontoeval（PR 基准库）与 OntoEval 数据集/OE-Assist（CQ 验证）拆解；本体测评外部基准谱系定位；可借鉴点 K1~K6 与接入判定
synced: 2026-10-17
---

# 88 · ontoeval 深度调研：AI agent 本体任务基准与 CQ 验证数据集（2026-10-17）

> 动因：开发者指令「在平台知识/本体 下新建 本体测评 专题，将相关文档加入其中。再深入调研一下 ontoeval」。本档为「本体测评」专题第 5 份成员，87 号 OntoGPT 调研的姊妹篇（同 Monarch/OBO 生态，方法论 idea-to-feasible-plan）。
> ⚠️ **命名澄清先行**：「ontoeval」一名对应三个实体，调研前必须分清——
> ① **ai4curation/ontoeval**（Chris Mungall，PyPI 0.1.0）：评 AI agent 做本体任务的 PR 基准库——本档 §2 主实体；
> ② **OntoEval 数据集 + OE-Assist**（Lippolis/Saeedizade 等，arXiv 2507.14552）：LLM 辅助本体功能评估（CQ 验证）的数据集与人机协同框架——本档 §3；
> ③ **本项目 `ontology-service/internal/evaldata`**（REQ-207/M43 起 build tag 基准包）：项目内文档（85 号等）惯称「ontoeval 本体基准」——**沿用了外部之名**，非外部项目本身。85 号表格「ontoeval 本体集 10 份」指 ③。

## 一、ai4curation/ontoeval：用 GitHub issue+PR 当本体任务基准

### 1.1 项目名片
- 仓库 github.com/ai4curation/ontoeval，fork 自 hrshdhgd/llm-change-agent；Python 90.8%；v0.1.0（2025-07-08）；86 commits，**此后一年多未更新**；2 star；维护者 4 人（cmungall/justaddcoffee/kltm/matentzn——OBO 核心圈）。文档站 ai4curation.github.io/ontoeval/。
- 自述定位：*«A python library for evaluating the performance of AI agents on ontology tasks, using GitHub issues and pull requests as a benchmark»*。
- 生态位：GO-AI / AI4curation 计划——用 LLM agent 维护 GO/Mondo 等大型 OBO 本体（提 PR 改 *-edit.obo），ontoeval 负责回答「**agent 改得跟人类专家一样好吗**」。实例仓 odk-ai-ontoeval。

### 1.2 机制五步（PR 重放式基准）
1. **爬取**：给定「PR↔issue 关联」的仓库，逐 PR 抓元数据+关联 issue+before/after commit ID（commit 通常为 *-edit.obo 简单变更）；
2. **重放**：重建 PR 创建时刻的仓库状态与 issue tracker 状态——**遮蔽原始 PR 作者当时可见的评论**，保证 agent 与人类信息对等；
3. **执行**：AI agent 经 **runner** 跑 issue 指令。支持 runner：Claude Code、Goose（CLI）。实验 yaml 配置模型/提示词/环境（prompt 模板含「不要用 git/gh、结束时不要 commit——我会先评审你的修改」类护栏）；
4. **取diff**：agent 产出 diff vs 人类原始 diff（新术语数字部分先归一化到新范围，避免「都是新 ID」的假差异）；
5. **评分**：双 evaluator（§1.3）。运行形态 `ontoeval run-all -c experiments/mondo-1.yaml -I src/ontology/mondo-edit.obo`——已见 Mondo/GO 实例。

### 1.3 双评分器
| 评分器 | 机制 | 优点 | 自认局限 |
| --- | --- | --- | --- |
| **metadiff** | diff 相似度：overlap/F1/precision/recall，按行/公理 all-or-nothing 对齐 | 客观可复现；归一化后可比 | 差一个字符即同时计 FP+FN；长文本定义建议 BLEU 类相似度，**未实现** |
| **llm_judge** | LLM as judge：issue+人类改动+agent 改动三段输入，rubric 多准则打分输出结构化 JSON（overall_score / instruction_following / logical_consistency / issue_clarity / difficulty / confidence / similarity / suggestions_for_users；示例：左 0.7 右 0.6、confidence 0.9） | 可评「语义等价但写法不同」；suggestions 字段直接面向人 | 依赖 judge 稳定性；需固定 rubric 防漂移 |

### 1.4 自认局限（论文式诚实标注）
- **不可并行**：重放用共享工作目录，同本体并发评测会互相污染；
- **单 issue 假设**：假设 issue↔PR 一一对应；一 issue 多 PR（标记 epic）直接忽略不评——真实工作流常更复杂。

## 二、OntoEval 数据集与 OE-Assist：LLM 辅助 CQ 功能验证（arXiv 2507.14552，2025-07）

- **团队**：Lippolis（Bologna/ISTC-CNR）+ Saeedizade/Keskisärkkä/Blomqvist（Linköping）+ Gangemi + Nuzzolse——欧洲联盟，与 81 号 CQ4OE（OEG-UPM）不同团队、同一问题域。作者自认是**首个 LLM 辅助本体功能评估的系统研究**。
- **OntoEval 数据集（1,393 CQ）**：57 份人工本体（欧洲项目 Polifonia 音乐/WHOW 水与健康/Onto-DESIDE 循环经济/IKS/AcquaDiva 等）+ 1,336 份 LLM 生成本体（GPT-4 家族/Llama-3.1-405B/o1-preview/DeepSeek-V3），每条配 user story+gold 标注（**Yes / No / No-minor**〔只缺一个属性即完整〕，双本体工程师交叉核验）+难度标签（**Simple=≤2 类+1 对象属性，否则 Complex**）。刻意保留有噪声的 LLM 生成本体——「真实评估工具必须处理不完美模型」。
- **OntoEval-small**：均衡子集（只留 Complex+每项目≤3 CQ+LLM 正确率作软过滤保持分布）——供用户研究。
- **自动评估（RQ1）**：LLM 对「本体+CQ」产 Yes/No 判定+一条佐证 SPARQL；o1-preview / o3-mini / GPT-4o 三模型对照 gold 算 Macro-F1/Accuracy。**结论：o1-preview 与 o3-mini 的自动评估水平≈人类知识工程师平均用户水平**（具体指标表见论文 §6）。
- **半自动 OE-Assist（RQ2）**：Protégé 插件给 CQ 验证中的工程师实时建议；**19 名知识工程师、8 个国际机构**对照实验。结论三件：①建议正确时用户准确率提升；②感知任务难度显著下降；③**LLM 给出错误引导时人类表现反而下降**——效率与准确的关键权衡，混合人机不是免费午餐。

## 三、本体测评外部基准谱系（专题定位图）

| 基准 | 评什么 | 信号形态 | 与本项目对应 |
| --- | --- | --- | --- |
| **OntoEval**（本档 §二） | 本体对 CQ 的**功能满足度**+人机协同评估 | gold 标注 CQ 判定集（1393 条）+LLM label/SPARQL 佐证 | 85 号/REQ-345 CQ 基准集（50 条三层均衡，86 号 N2 已清账）= 同思路缩小版；gold terms+R/E/I 口径 |
| **ai4curation/ontoeval**（本档 §一） | **AI agent 改本体的行为质量** | 历史 PR 重放+metadiff/llm_judge | 本项目暂无 agent 自动改本体的消费场景；REQ-339 演化动线/REQ-257 版本对比可借其「变更基准化」思想 |
| **CQ4OE**（[81 号](./81_CQ4OE深度调研_CQ2Term与CQ2Onto原理与借鉴_20261016.md)） | LLM **生成**本体的质量 | CQ2Term/CQ2Onto 五层指标+gold 对齐（w3id.org/cq4oe，ftr:Metric 体系；EmbeddingDiagnostic 软相似诊断） | llmcreate/REQ-171 六路径生成质量门禁的评估口径参照 |
| **OntoURL**（arXiv 2505.11031） | LLM 的**本体理解/推理/学习**能力 | 40 本体 57,303 题 15 任务，Bloom 三层 taxonomy | 远期：学习平台「本体能力」评测叙事可借其任务分层 |
| **OAEI** | 本体**对齐/匹配**系统 | 年度竞赛+reference alignment | REQ-347 外部标准基准引入的挂靠对象（85 号已注） |
| **OOPS!** | 本体**结构缺陷** | 41 类 pitfall 扫描 | REQ-344 OE-3 移除法评估器结构维未引入，如实标注留后续 |

谱系小结：**评估对象四层**——本体本身的质量（78 号五层模型/OOPS!）、本体生成过程（CQ4OE）、本体功能满足（OntoEval/本项目 CQ 基准集）、改本体 agent 的行为（ai4curation/ontoeval）。本项目 85 号选择先建「功能判定集」（OntoEval 式），与「教学→工程化产品」定位一致。

## 四、可借鉴点清单 K1~K6

| # | 借鉴点 | 本项目落点 | 状态建议 |
| --- | --- | --- | --- |
| K1 | **metadiff 归一化对比法**：LLM 产物 vs gold 的结构性 diff，新实体数字段归一化后算 overlap/F1/P/R；行级 all-or-nothing 局限→文本字段加软相似（CQ4OE EmbeddingDiagnostic 同思路） | REQ-346 基准报告可比化（版本/渠道对比）的直接方法论；86 号 N4 rest_ontochat 的 CQ 信号报告 | ⏸ 挂 REQ-346（P3 触发驱动，v1 已交付管理面动线） |
| K2 | **llm_judge rubric 结构化**：多准则+confidence+difficulty+suggestions_for_users 的 JSON 契约 | agenteval Judge 复用契同（REQ-346 后续明确留「Judge 共契与对比视图」）；72 号评估反馈卡的 suggestion 交互 | ⏸ 挂 REQ-346 后续评估 |
| K3 | **PR 重放式变更基准**：变更前后状态重建+人类/机器 diff 对比+信息对等遮蔽 | REQ-207 进化门控/REQ-257 版本对比的「变更基准化」增强；REQ-339 演化动线的评测面参照 | 观察（本项目暂无 agent 自动改本体场景） |
| K4 | **CQ 验证人机协同双信号**：自动判定+建议制人工复核；「错误引导拉低人类表现」警示——建议必须带 confidence 且可一键驳回 | 72 号 C 面（评估→消费反馈闭环）与 78 号 Findings 修复动线的交互设计参照 | ⏸ 已有排期参照（REQ-312/331 均已立项） |
| K5 | **难度分层标签法**：Simple（≤2 类+1 对象属性）/Complex 二分 | 85 号 CQ 基准集（50 条已三层均衡）可加难度维度，扩统计口径防回归噪声（86 号 N2 复盘延伸） | ⏸ 数据集已扩至 50 条，难度分层作下次扩条选项 |
| K6 | **OntoURL 三层任务 taxonomy**（understanding/reasoning/learning） | 学习平台本体能力评测叙事（远期）；REQ-109 外部资源导航可收录其论文页 | 观察 |

## 五、接入判定

- **运行时 ⛔ 不引入**：ai4curation/ontoeval 为 Python+GitHub PR 工作流特化+一年多停更，与 D-O15 轻量自研原则及 Go 技术栈冲突；其价值全部在方法论层（K1~K5），现有排期（REQ-346/257/207）可直接消费。
- **OntoEval 数据集 ✅ 远期候选**：1393 CQ 公开数据集是 REQ-347（外部标准基准引入）的候选项之一，与 CQ4OE HF、OAEI 并列；其「人工+LLM 生成混合、刻意含噪声」的构成哲学对种子本体基准集扩条有直接参考价值。
- **与 87 号 OntoGPT 的关系**：同生态不同环节——OntoGPT 管「抽得准+grounding 实」（构建期），ontoeval/OntoEval 管「评得准」（评估期）；两者在「LLM 不可信环节给确定性工具兜底」的哲学上一致（SPIRES 的 OAKlib grounding ↔ metadiff 的确定性 diff、K4 的 confidence 建议）。

## 七、谱系×项目实现完备度对照检查（2026-10-17 增补，开发者指令）

> 对 §三表格逐行核对：当前项目实现是否完备、产品设计界面提示是否清晰有条理。依据=代码盘点（qualitygate/evaldata/cqbench/removal_eval 五包+前端 QualityCardPane/QualityReportCard/ProductionGuide/AuditPage）+18 号注册表 REQ 状态。

| 谱系体系 | 项目实现 | 完备度 | 界面承载与提示 | 结论 |
| --- | --- | --- | --- | --- |
| OntoEval（CQ 功能满足） | REQ-345 ✅ 基准集（gold terms+E/I/R 三层，扩容后 50 条三层均衡）+cqsignal_eval 三率信号；REQ-346 ✅ cqbench 三端点（datasets/run/report） | ✅ 缩小版完备（v1）；差距=无难度标签/答案级评估留 Judge 共契 | 质量卡「CQ 基准集测我」卡：选集+三率说明+「跨域 gold 对齐率低为预期负信号」解释+报告历史三率表+低对齐行展开明细+版本超前过期提示重跑 | 提示清晰有条理（机制/口径/预期行为三层齐） |
| ai4curation/ontoeval（agent 行为） | 无对应场景 | ➖ 诚实不建（正确） | 无 | 一致 |
| CQ4OE（生成质量） | qualitygate 15 项三维（完备/一致/可维护，扣分制权重透明）+REQ-342 ✅ OC-2 覆盖测试站（快筛+SPARQL 精判双轨+回归基准+三级覆盖衔接 81 号）；REQ-347 📋 CQ4OE 数据集适配未领取 | ◐ 结构与功能覆盖在位；CQ4OE 式 gold 对齐五层指标挂 REQ-347 | 覆盖测试站双轨说明（L633）+QualityReportCard 扣分制权重说明+新旧对照 Alert | 清晰；口径与 CQ4OE 的关系由 81 号档承载，界面不重复背 |
| OntoURL（LLM 本体能力） | 无（远期） | ➖ 正确不做 | 无 | 一致 |
| OAEI（对齐） | REQ-347 📋（OAEI 挂 REQ-79 对齐链） | 📋 立项未开发（触发驱动纪律内） | 无 | 一致，谱系唯一剩余落地缺口 |
| OOPS!（结构缺陷） | qualitygate 15 项自研检查覆盖部分同型 pitfall（层级循环/悬空端点/未连接等）+REQ-344 ✅ 交付注「结构维 OOPS! 不引入如实标注」；REQ-331 P2 留「OOPS! 检查档」触发 | ◐ 自研替代部分覆盖，41 类 taxonomy 对标有意不做（已登记去向） | 质量档案：Findings 严重级分组+15 项「怎么改」人话指引+error 前往图形修复+info 方法论入口（REQ-331 修复动线三段跳转） | 提示清晰有条理（评估→动作闭环） |

**总评**：①六个体系没有「该做没做」的悬空——2 个落地（OntoEval 缩小版/OOPS 自研替代）、1 个部分覆盖（CQ4OE 挂 REQ-347）、1 个立项待领取（OAEI 同属 REQ-347）、2 个诚实不做；②界面提示普遍做到「机制说明+口径+预期行为解释」，REQ-334 生产动线「②质量门禁」步骤把评估串进交付路径，条理成立。③可改进三件：
- **K-界面①（唯一具体缺口）**：基准测我卡未透出「gold 对齐=查询引用 gold 概念 URN，非答案内容比对」语义（REQ-345 诚实边界，仅代码注释 cqbench.go 承载）——用户易误读为答案正确率；建议报告历史表加一句 Tooltip，随 REQ-346 后续轮或 Judge 共契落地。
- **K-界面②**：基准集无难度分层标签（§四 K5），统计口径防回归噪声可扩展——留基准集下次扩条。
- **K-界面③**：qualitygate 检查项↔OOPS 41 类映射表不存在（文档/界面均无）——REQ-331 P2「OOPS! 检查档」触发时一并补，当前不预付。

## 八、维护说明
- 来源：github.com/ai4curation/ontoeval（README+docs 站）、pypi.org/project/ontoeval、arxiv.org/abs/2507.14552（§3/§4/§6/摘要）、arxiv.org/abs/2505.11031、ai4curation.github.io/ontoeval docs（Evaluators/Limitations 节引文为原文直译）。OntoEval 精确指标表未逐值摘录，以论文 §6 为准。
- 关联：[87 号 OntoGPT](../08_调研预研/项目深度调研/87_OntoGPT深度调研_SPIRES原理与开源实现与借鉴_20261017.md)、[81 号 CQ4OE](./81_CQ4OE深度调研_CQ2Term与CQ2Onto原理与借鉴_20261016.md)、[85 号](./85_测试数据集与基准测试模块必要性分析_20261016.md)、[86 号](../08_调研预研/产品与设计分析/86_本体实现现状体检与不足清单_20261017.md)；登记簿 §六 ai4curation/ontoeval 行+T1-16；15 号 v2.42。
- 维护者：董奎 × 协作 Agent ｜ 2026-10-17 建
