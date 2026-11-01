---
module: 知识体系
topic: 决策·为什么选 Eino ADK
desc: 智能体编排框架选型——Eino/DeepSeek-Harness/Python 语义栈三路线对比与取舍后果
synced: 2026-10-28
kind: decision
domain: agent
prereq: ["03_概念_五层域总纲.md"]
sources:
  - platform-knowledge/02_智能体/08_调研预研/方案与设计分析/51_本体对话Agent技术选型_Eino_vs_DeepSeekHarness.md
  - platform-knowledge/02_智能体/08_调研预研/外部平台调研/44_Eino框架深度分析_实现原理与应用场景.md
check:
  - 三条备选路线各自的胜负手是什么？
  - 选 Eino 后，「编排语义」的哪些部分仍要平台自己定案？
---

**背景**：本体对话验证 Agent 需要一套编排框架，候选三条路线：DeepSeek-Harness（现成的编码智能体 harness）、Eino+ADK（字节 Go 框架）、Python 语义工具栈（rdflib 系）。

**备选对比**（51 号结论）：**Harness 赢快和现成**——开箱即用的长时程任务执行与审计，但编排语义黑盒、Go 集成靠进程协议转译；**Python 赢语义工具链贴合**——rdflib/OWL 生态全，但引入 Python 运行时违 D-O15（零 venv 纪律）；**Eino 赢稳、可控、长期**——组件化（Model/Tool/Retriever 可替换）、ADK 编排原语官方维护、Go 单进程与平台零缝。

**取舍**：主链押 Eino ADK。代价：ADK 不是智能体全家桶——装配链、审批、沙箱、事件契约仍要平台自建（这正是 Harness/Loop 层大量自研件的原因）；且 ADK 部分原语有官方风险标注（transfer=NOT RECOMMENDED、workflow 三构造器 NOT RECOMMENDED），平台必须自己定案编排语义。

**后果**：形成了「agent_as_tool 主推、transfer 保留对照+UI 风险标注」的定案（44 号取证+REQ-208 治理），以及 workflow 模式的告警注记——框架给了原语，语义责任在平台。学习要点：选框架不是选能力清单，是选「哪些语义自己担」的分界线。
