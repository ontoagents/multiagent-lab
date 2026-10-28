---
module: 知识体系
topic: LLM 可读聚合视图（llms.txt 形态）
desc: 全部知识卡的机器可读索引——生成器从卡 frontmatter 聚合（生成而非手维，漂移由常跑测试强制）
synced: 2026-10-28
sources:
  - platform-knowledge/08_调研预研/58_核心知识体系提取可行方案.md
  - REQ-372
---

# 平台核心知识体系（LLM 聚合视图）

> REQ-372 知识工程三层资产：三型知识卡（概念/决策/实践）× 三线学习路径的机器可读聚合。本文件为生成物（勿手改）——卡 frontmatter 为事实源，改卡后 `KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync` 重新生成。人读入口 [00_导读.md](platform-knowledge/09_知识体系/00_导读.md)。
>
> 共 30 卡：概念 12 · 决策 8 · 实践 10 ｜ 卡 synced 最新 2026-10-28

## 01_本体线（13 卡）

- [本体与知识工程谱系](platform-knowledge/09_知识体系/01_本体线/cards/01_本体与知识工程谱系.md): concept｜ontology｜本体是什么——从 DIKW 谱系到与数据库 schema/面向对象类的对照，以及开放世界假设｜前置 0·源指针 3
- [CQ 驱动建模](platform-knowledge/09_知识体系/01_本体线/cards/02_CQ驱动建模.md): concept｜ontology｜能力问题（Competency Question）是什么、为什么 CQ 先行、以及它在平台里如何变成可执行的验收｜前置 1·源指针 5
- [RDF/OWL/SPARQL 三件套](platform-knowledge/09_知识体系/01_本体线/cards/03_RDF_OWL_SPARQL三件套.md): concept｜ontology｜三元组数据模型、语义层与查询语言的分工，以及平台内的落点（TTL 导出与只读查询白名单）｜前置 1·源指针 4
- [TBox 与 ABox 分层](platform-knowledge/09_知识体系/01_本体线/cards/04_TBox与ABox分层.md): concept｜ontology｜术语层与断言层的分工——平台 spec 的双层映射、装载顺序依赖与治理面差异｜前置 1·源指针 3
- [三平面架构——构建/运行/消费审计](platform-knowledge/09_知识体系/01_本体线/cards/05_三平面架构.md): concept｜ontology｜知识资产的职责分层：spec 门禁构建、引擎装载运行、KG 与 PROV-O 消费审计，与平台五栏 IA 一一对应｜前置 1·源指针 3
- [决策·单引擎 Oxigraph（D-O5）](platform-knowledge/09_知识体系/01_本体线/cards/06_决策_单引擎Oxigraph.md): decision｜ontology｜为什么运行平面只押一个引擎——背景、备选对比、取舍与对冲（Fuseki 候选序）｜前置 1·源指针 3
- [决策·spec 归一化形态（为什么编辑态不是 OWL/TTL）](platform-knowledge/09_知识体系/01_本体线/cards/07_决策_spec归一化形态.md): decision｜ontology｜自研 spec_json 归一化形态的背景、备选、取舍与后果——校验管线、编辑保护三层与导出投影｜前置 1·源指针 5
- [决策·不接 Python 运行时（D-O15）](platform-knowledge/09_知识体系/01_本体线/cards/08_决策_不接Python运行时.md): decision｜ontology｜semantica worker 的引入与退役——零 venv 一键启动、Go 原生 TTL 导出对齐，以及边界里保留了什么｜前置 1·源指针 3
- [实践·六路径构建](platform-knowledge/09_知识体系/01_本体线/cards/09_实践_六路径构建.md): practice｜ontology｜六条建本体的动线各自怎么走、代码落点在哪、按什么选路径，以及共同的写入口纪律｜前置 1·源指针 3
- [实践·质量门禁与进化](platform-knowledge/09_知识体系/01_本体线/cards/10_实践_质量门禁与进化.md): practice｜ontology｜三维分怎么算、strict 门禁何时拦截、CQ 基准「测我」三率怎么读、以及进化候选的状态机｜前置 2·源指针 5
- [实践·消费与审计三件套](platform-knowledge/09_知识体系/01_本体线/cards/11_实践_消费与审计三件套.md): practice｜ontology｜facade 工具面（20 件）读、KG 抽取与治理、PROV-O 决策审计——消费面代码落点与一致性来源｜前置 1·源指针 4
- [决策·MCP 工具面扩展（REQ-366，6→19）](platform-knowledge/09_知识体系/01_本体线/cards/12_决策_MCP工具面扩展.md): decision｜ontology｜为什么把本体操作从 UI/REST 专有消费升级为 MCP 标准工具面——证据、门控取舍与写路径纪律｜前置 1·源指针 4
- [双时序知识图（vt×tt）](platform-knowledge/09_知识体系/01_本体线/cards/13_概念_双时序知识图.md): concept｜ontology｜有效性时间与事务时间的区别、失效语义，以及平台伴生图的落点（companion_history 三模式）｜前置 1·源指针 4

## 02_智能体线（12 卡）

- [概念·智能体与 LLM 应用工程谱系](platform-knowledge/09_知识体系/02_智能体线/cards/01_概念_智能体工程谱系.md): concept｜agent｜智能体在 LLM 应用工程谱系中的位置——从提示词到多智能体，智能体与聊天机器人的分界线｜前置 0·源指针 2
- [概念·五阶段范式演进（提示词→Graph 工程）](platform-knowledge/09_知识体系/02_智能体线/cards/02_概念_五阶段范式演进.md): concept｜agent｜AI 应用工程的五个阶段——受控单元不断外扩，越往后越靠外部状态质量而非模型能力取胜｜前置 1·源指针 2
- [概念·五层域总纲（Model/Context/Harness/Loop/Graph）](platform-knowledge/09_知识体系/02_智能体线/cards/03_概念_五层域总纲.md): concept｜agent｜平台智能体架构的五层域划分——各层职责、代码承载与「配置聚合呈现≠层职责混淆」口径｜前置 1·源指针 5
- [概念·外部智能体平台观察（坐标系）](platform-knowledge/09_知识体系/02_智能体线/cards/04_概念_外部智能体平台观察.md): concept｜agent｜六篇外部平台调研构成的对照坐标系——DeepSeek-Harness/Eino/扣子/Fleet/Dots/Jev/FutureAGI 各自验证什么｜前置 1·源指针 6
- [决策·为什么选 Eino ADK](platform-knowledge/09_知识体系/02_智能体线/cards/05_决策_选型EinoADK.md): decision｜agent｜智能体编排框架选型——Eino/DeepSeek-Harness/Python 语义栈三路线对比与取舍后果｜前置 1·源指针 2
- [决策·多后端推理注册表与外部 CLI 边界](platform-knowledge/09_知识体系/02_智能体线/cards/06_决策_多后端推理注册表.md): decision｜agent｜推理后端注册表矩阵（eino-adk 内置+外部 CLI）与「外部后端旁路装配链」的诚实边界｜前置 1·源指针 3
- [决策·A2A 双向协议与外部成员轨](platform-knowledge/09_知识体系/02_智能体线/cards/07_决策_A2A双向与外部成员轨.md): decision｜agent｜平台既是 A2A server 也是 client——协议面取舍（tasks 落库/审批闸门/input-required）与语言无关主张｜前置 1·源指针 5
- [实践·Harness 层装配链与防护](platform-knowledge/09_知识体系/02_智能体线/cards/08_实践_Harness装配链.md): practice｜agent｜assembler 装配管线、hooks 防护链、审批 gating、verify 背压与沙箱——一次运行的系统怎么搭｜前置 1·源指针 3
- [实践·Context 层供给（预算/压缩/剪枝/召回）](platform-knowledge/09_知识体系/02_智能体线/cards/09_实践_Context层供给.md): practice｜agent｜模型每轮真正看到什么——context_mode 三档预算、工具轮次保全、压缩持久化与召回链｜前置 1·源指针 4
- [实践·Loop 层循环治理与挂起恢复](platform-knowledge/09_知识体系/02_智能体线/cards/10_实践_Loop层循环与恢复.md): practice｜agent｜迭代上限、checkpoint 持久化、挂起恢复/放弃/超时、定时续跑与 usage 成本护栏｜前置 1·源指针 4
- [实践·Graph 层多智能体编排](platform-knowledge/09_知识体系/02_智能体线/cards/11_实践_Graph层多智能体编排.md): practice｜agent｜agent_as_tool 主推/transfer 对照/workflow 五模式/后台委派——多个智能体怎么组队与流转｜前置 2·源指针 3
- [实践·事件面与回放观测](platform-knowledge/09_知识体系/02_智能体线/cards/12_实践_事件面与回放观测.md): practice｜agent｜SSE 事件契约、展示级别与 debug_persist、obs.timing 观测、回放过滤——运行过程如何可见可审计｜前置 1·源指针 5

## 03_结合线（5 卡）

- [概念·知识三形态对照（RAG 片段/KG/本体）](platform-knowledge/09_知识体系/03_结合线/cards/01_概念_知识三形态对照.md): concept｜bridge｜同一栈的三个层次——本体=模式层、KG=事实层、知识库=交付层，以及「何时用哪个」的工程判据｜前置 2·源指针 2
- [概念·本体赋能 Agent 的结合模式](platform-knowledge/09_知识体系/03_结合线/cards/02_概念_本体赋能Agent结合模式.md): concept｜bridge｜四模式框架——GraphRAG/Ontology-as-Tools/混合推理/AgentO，各自回答什么问题｜前置 2·源指针 2
- [决策·知识工程三层资产（卡→图谱→服务）](platform-knowledge/09_知识体系/03_结合线/cards/03_决策_知识工程三层资产.md): decision｜bridge｜知识体系自身的架构决策——md 卡为事实源、KG 为投影、服务层为消费面，确定性摄取优先于 LLM 摄取｜前置 1·源指针 3
- [实践·平台实证——本体增强智能体动线](platform-knowledge/09_知识体系/03_结合线/cards/04_实践_本体增强智能体动线.md): practice｜bridge｜Ontology-as-Tools 在平台的落地——facade 工具面、伴生绑定、check_answerable 与消费贡献度量｜前置 2·源指针 5
- [实践·平台实证——自研 KG 管线与 GraphRAG 问答](platform-knowledge/09_知识体系/03_结合线/cards/05_实践_KG管线与GraphRAG问答.md): practice｜bridge｜internal/kg 两步抽取与社区摘要、KB graphrag 三步检索、伴生检索三臂——GraphRAG 模式的零依赖落地｜前置 1·源指针 6

