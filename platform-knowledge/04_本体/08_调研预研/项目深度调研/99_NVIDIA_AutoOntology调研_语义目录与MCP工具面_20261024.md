---
module: 本体
topic: NVIDIA Auto Ontology 调研（语义目录与 MCP 工具面）
desc: 开发者指令「调研这篇文章中涉及到的项目」——DataFun《NVIDIA 开源 Auto Ontology，本体创建时间缩短 90%》（2026-10-23）：NVIDIA 2026-10-21 开源的语义目录（semantic catalog）框架（Agent Knowledge Foundation 首件），FastAPI+Next.js+pgvector+NIM 栈，Discover/Ground/Validate/Expose/Learn 五环节，九只读 MCP 工具（术语中心+ask_question+可回答性预检），定义 candidate/reviewed/certified 三态认证；与 Palantir（74 号 E10）/Snowflake（74 号专档）/Databricks 三方格局对比（新增多源中立开源目录层选手）；与本项目十项映射（6 ✅ 同构 / 2 🟡 粒度差 / 2 缺口→借鉴决策 D1 预检工具·D2 实体级认证状态）
synced: 2026-10-24
状态: v1.0 调研归档（🔭 对照参考不引入——Python/NIM 重栈 D-O15 不符；借鉴点 D1 入需求池候选、D2 观察池，待拍板）
动因: 开发者指令「调研这篇文章中涉及到的项目，已有调研文档的项目在原文档中扩充，不要新建重复文档。分析当前项目可以借鉴的地方」（2026-10-24，文章 https://mp.weixin.qq.com/s/DXdqHAA-5enaRKvY5bkt-g）
---

# NVIDIA Auto Ontology 调研（语义目录与 MCP 工具面）

## 0. 结论速览

**问题**：调研 NVIDIA 2026-10-21 开源的 Auto Ontology 及文章涉及的三家对比平台（Palantir/Snowflake/Databricks），评估对本项目的借鉴价值。

**Auto Ontology 一句话**：在**已有数据之上**构建一层「可审核的语义目录」（术语/指标的可信业务定义 + 映射到物理列/SQL 属性），经 MCP/CLI/API 以**九个只读工具**暴露给任意 agent harness——它是 NVIDIA Agent Knowledge Foundation（Agent Toolkit 生态）的开源首件，Apache-2.0，2026-10-21 官宣。

**对本项目映射结论**：十项映射 **6 项同构已有**（含两个路线更强的：Learn 环节=REQ-207 进化闭环且我们的 Evidence 锚定+人工采纳链更严；不搬数据建目录层=D-O1 哲学同源）、**2 项粒度差**（定义认证状态停在资产/版本级 vs 其实体级；工具面纪律差一个握手 instructions）、**2 项真缺口**——①`check_answerable` 形态的**可回答性预检工具**（我们 facade 20 工具全是执行类，无「先判断本体能否回答」的便宜预检）②**实体级定义认证**（candidate/reviewed/certified）。核心判断：**该项目验证了我们两条主线**——「本体作为 Agent 可信上下文 + MCP 薄暴露」的形态，与「Learn=认证纠正回流」的自进化方向；其差异化在**业务术语/指标语义层**（BI 域），与本项目知识本体域不同赛道，整体 🔭 对照参考不引入（Python/FastAPI/NIM 重栈与 D-O15 单机纯 Go 冲突）。

---

## 1. Auto Ontology 全景

### 1.1 定位与背景

- **解决问题**：Agent 分析可靠性瓶颈——"a revenue metric answered faster is worth nothing if the number is wrong"。从企业元数据构建上下文层（数据在哪/含义/是否如何使用/来源/相似性），供任意 harness（Cursor/Claude Desktop/自建 agent）消费。
- **生态位**：NVIDIA **Agent Knowledge Foundation**（agent 知识基建）开源首件，与 Agent Toolkit 同生态；官方叙事=让 agent 拥有「对数据资产的共享、可信、可审计理解」。
- **仓库态**：Apache-2.0（SPDX 头），79★/1,329 commits（新开源）；**不接受外部贡献**（外部 issue/PR 不审查不合并，安全问题走 SECURITY.md）。

### 1.2 架构与部署

| 组件 | 技术 | 端口 |
| --- | --- | --- |
| 前端 | Next.js 16（React 19/TS/Tailwind 4）+ Better Auth（SSO+RBAC）+ Prisma | 3000 |
| 后端 | FastAPI（Python 3.12+）+ LangChain（聊天/NL-to-SQL、目录、数据源 API） | 3001 |
| 摄取 worker | FastAPI（表格数据摄取+嵌入写入） | 3002 |
| MCP server | 见 §1.4（零凭证薄层，默认 0.0.0.0:3003） | 3003 |
| 存储 | Postgres + pgvector（应用元数据/目录与本体/向量库） | 5432 |
| 可选 | HashiCorp Vault（连接凭据安全存储；未配时**明文 JSON 存目录行**——回退路径） | — |
| 模型 | NVIDIA NIM 三元组：LLM nemotron-3-nano-30b-a3b / 嵌入 nemotron-3-embed-1b / 重排 llama-nemotron-rerank-vl-1b-v2（可回退任意 OpenAI 兼容端点） | — |

部署：Docker Compose v2 / K8s（Helm）；数据源连接器 **Databricks / Postgres / Snowflake / DuckDB**——**不搬数据，只建目录层**（语义定义+映射落在 pgvector 库，查询时下推到源库执行）。

### 1.3 五环节生命周期（Discover → Ground → Validate → Expose → Learn）

1. **Discover**：从元数据生成企业本体；接入已有本体/语义模型；构建 AI-ready 知识图谱；
2. **Ground**：为指标/实体/关系提供可信业务含义，跨系统调和不一致定义；
3. **Validate**：用**认证定义与血缘**校验生成的查询，每个答案附带溯源；
4. **Expose**：MCP / CLI / API 向 agent harness 提供上下文；
5. **Learn**：**认证过的纠正与使用情况回流本体**，共享上下文持续改进。

配套治理：语义目录中每个定义带 **candidate / reviewed / certified** 三态（文章口径：候选→审核→认证，认证态才作为 Agent 查询的权威依据）。

### 1.4 MCP 工具面（九件全只读）

> 原文声明："Everything here reads. Nothing modifies the glossary, the catalog, or the underlying databases."

| 分层 | 工具 | 用途 |
| --- | --- | --- |
| 术语中心 | `search_terms` / `get_term` / `get_term_columns` / `get_term_sql_attributes` / `get_sql_attribute` | 检索业务词汇表；术语详情（描述/同义词/相关术语）；映射物理列；术语下 SQL 属性；单属性表达式 |
| 表结构 | `describe_table` | 列/相关术语/SQL 属性 |
| 问答主工具 | `ask_question` | 自然语言问答，返回 **answer + SQL + rows 三字段**（SQL 随答案透出供代理复用） |
| 预检 | `check_answerable` / `check_readiness` | **语义层能否覆盖该问题**（廉价预检）/ 部署是否具备回答条件（一次性报告全部阻塞项） |

工具面纪律：结果上限 100 行 + `row_count`/`truncated` 截断标注；请求超时 30s、聊天超时 900s（双档）；**握手 instructions 说明工具调用顺序**（防模型抢用 ask_question）；**故意不提供浏览数据库/schema/原始列的工具**——迫使消费方经语义层访问。

### 1.5 认证与安全设计

- MCP server 本身**零凭证**（无数据库凭证/模型配置）；Auto Ontology 为授权服务器，客户端 OAuth 注册、用户 SSO 登录；**会话权限=登录者本人**（viewer 只得 viewer 答案，改角色/封禁即时生效）；token 每次调用经 UserInfo 端点校验。
- API token 只存 SHA-256 哈希、只显示一次。
- **边界警示**（官方自认）：FastAPI 端口是 trusted internal service boundary——`x-auto-ontology-user-id` 头不被独立验证，**不得公网暴露**。

### 1.6 「90% 工时缩短」声明的批判阅读

官方口径=本体创建时间缩短 90%；文章同时给出批判视角：**语义工程并未消失而是转移**——字段说明与指标映射的治理人工仍需前置，自动化的是「从元数据起草」而非「定义可信」。该口径与本项目诚实边界文化一致（见 L2）。

---

## 2. 三方格局对比（文章对照 + 74 号既有档交叉）

| 平台 | 形态 | 一句话定位 | 既有覆盖 |
| --- | --- | --- | --- |
| **NVIDIA Auto Ontology** | 开源（Apache-2.0） | **多源中立的开源目录层**：不绑单一仓，连接器接 Databricks/Snowflake/Postgres/DuckDB，语义定义+MCP 暴露自成一体 | 本档 |
| **Palantir Foundry Ontology** | 商用闭源 | **运营型**：Object+Link+Action+Function 四元组（Action 写回业务操作=数字孪生驱动应用）；双 MCP（构建者 70+ 工具/消费者 OMCP） | 15 号已登记；74 号 §2/E10（分析型 vs 运营型分野+2025-10 Iceberg 互操作） |
| **Snowflake Horizon Context** | 商用闭源 | **分析型仓内语义层**：Semantic Views 五要素+Autopilot 反推+KG 表模型+Flattened GraphRAG（~80% 增益来自画像预计算） | **74 号专档**（benchmark 四档/十项映射全在）；本档仅补事件对照（见 74 号 §9 增补） |
| **Databricks Unity Catalog Pages / Genie Ontology** | 商用闭源 | 湖仓一体语义层：Metric Views 指标定义+Pages 治理+Genie 消费（文章口径，未深查——置信 medium，不展开） | 无专档，暂不建（商业平台、非开源，对照点已被上两家覆盖；后续有真实对标诉求再补） |

**格局判断**：「企业语义层 for Agent」已是三家商业平台+一家开源新选手的 converging 赛道；Auto Ontology 的差异牌=开源+多源中立+MCP 优先。对本项目的意义见 §4/§5。

---

## 3. 证据台账

| ID | 事实/结论 | 来源 | 新鲜度 | 置信度 |
| --- | --- | --- | --- | --- |
| E1 | 项目定位/五环节/架构组件/部署/NIM 配置/不接受外部贡献/Vault 明文回退 | [GitHub README](https://github.com/NVIDIA/auto-ontology)（一手） | 新 | high |
| E2 | 九只读工具清单与分工/100 行截断+row_count·truncated/30s·900s 双超时/握手 instructions/故意不提供裸库浏览 | [docs/mcp.md](https://github.com/NVIDIA/auto-ontology/blob/main/docs/mcp.md)（一手） | 新 | high |
| E3 | MCP 零凭证+OAuth 用户级身份/会话权限=登录者/API token 哈希/FastAPI 内部信任边界警示 | 同 E2 | 新 | high |
| E4 | 2026-10-21 开源官宣/纳入 Agent Knowledge Foundation/「90% 缩短」官方叙事/三方对比叙述/定义三态认证 | [DataFun 文章](https://mp.weixin.qq.com/s/DXdqHAA-5enaRKvY5bkt-g)（2026-10-23，二手转述） | 新 | medium（二手；关键事实与 E1/E2 互证，三态认证与 90% 声明未在一手 README 逐字核对） |
| E5 | 79★/13 forks/22 open issues/1,329 commits 仓库现状 | E1 页面侧栏 | 新 | high（时点值） |
| E6 | Palantir/Snowflake/Databricks 对比细节 | 文章（E4）+ 74 号 E8/E10 交叉 | 中 | medium（Databricks 部分未深查） |

---

## 4. 与本项目映射（十项）

> 盘点基准：03 v1.90、04 v1.33、facade 20 工具（REQ-366/368）、REQ-382 执行级对齐已交付、REQ-377 伴生图持久化已交付。

| # | Auto Ontology 要素 | 本项目对位资产 | 状态 | 说明 |
| --- | --- | --- | --- | --- |
| M1 | 语义目录（术语/指标可审核定义） | spec_json 概念/关系/数据属性 + CQ | ✅ 同构 | 域不同：业务术语/指标（BI）vs 知识本体（OWL/RDF）；指标语义层不在本项目域（见不做清单） |
| M2 | Discover 从元数据起草本体 | 六路径构建（AI 创建/OntoChat/KB 三策略/权威库/OntoExtend）+ 用法推断（REQ-328） | ✅ 已有且更全 | 其「从元数据反推」≈我们「从导入/KB 反推」；其 Autopilot 式自动化叙事见 74 号 M6 结论（我们已有门禁更完整） |
| M3 | Ground 跨系统定义调和 | 别名/人工合并（REQ-286）+ REQ-79 本体对齐（P2 池） | 🟡 部分 | 跨本体对齐留池未动——与 Auto Ontology 的多源调和诉求同族，维持触发驱动 |
| M4 | Validate 认证定义校验+答案溯源 | qualitygate+进化配对门控（REQ-207/382 三信号）+trace_log 翻译透视（REQ-94）+PROV-O 审计 | ✅ 已有 | 其「答案附溯源」≈我们 SQL 透出（trace）+决策审计；粒度差见 M8 |
| M5 | Expose MCP/CLI/API | facade 20 工具（MCP Streamable HTTP，SELECT/ASK 白名单+limit 200+10s 超时+truncated） | ✅ 已有 | 工具面纪律同构度极高（见 O1 小增量） |
| M6 | Learn 认证纠正回流 | REQ-207 进化闭环（诊断→归因→补丁+Evidence 锚定机器门+人工采纳链+门控三信号） | ✅ 已有（更严） | 业界同构验证；其「认证过的纠正才回流」≈我们「Evidence 锚定+gate/accept 人工链」 |
| M7 | check_answerable / check_readiness 预检 | 无对位（facade 全是执行类；check_readiness 形态≈引擎自检/healthz/loaded_status 已有） | ❌ 缺 | **D1 借鉴点**：可回答性预检 |
| M8 | 定义三态认证（candidate/reviewed/certified，实体级） | 资产级发布态+版本级候选 vN-cK（REQ-207/239） | 🟡 粒度差 | **D2 借鉴点**：实体级认证粒度 |
| M9 | 工具面纪律（截断/双超时/握手 instructions/术语中心分层） | limit 200/10s/truncated 已有；guide 紧凑指引（REQ-215/357） | 🟡 小增量 | O1：guide 补「建议调用序」一句话 |
| M10 | 不搬数据建目录层（多源连接器） | D-O1 仓库=唯一事实源+original 直挂（REQ-328 运行双源） | ✅ 同哲学 | 业界同构验证（「语义层不绑单一仓」=74 号 E8 跨系统批评的回应方向） |

**映射结论**：🆕 0 个模块级引入、2 个借鉴决策（D1 入池候选 / D2 观察池）、1 个零立项顺修级小增量（O1）、2 项学习素材（L1/L2）。

---

## 5. 借鉴决策

### D1（🆕 候选，P3 需求池，待拍板）：facade「可回答性预检」工具

- **借鉴点**：`check_answerable` 形态——在执行 SPARQL/工具前，按 **CQ 关键词/概念覆盖**判定「本本体能否回答该问题」（便宜预检，避免 agent 盲跑长查询后才发现本体不含该域）；`check_readiness` 形态已有对位（引擎自检/healthz/loaded_status/质量快评）不另立。
- **项目落点**：runtime-manager facade 新只读工具 `check_answerable(question)`——实现候选=spec.CQ 关键词命中 + concepts/labels 检索覆盖度，返回 {answerable, matched_concepts, suggestion}；与 cqbench（离线评测）和 REQ-382（执行级对齐）互补，不引入 LLM 调用（确定性词法匹配起步）。
- **触发条件**：消费侧真实出现「盲问失败」案例（agent 对不含该域的本体反复提问失败）再领取；当前 guide 紧凑指引+list_concepts 已部分兜底。

### D2（⏸ 观察池）：实体级定义认证状态

- **借鉴点**：candidate/reviewed/certified 三态落到**每个术语/定义**（实体级），消费时可声明「答案基于已认证定义」。
- **对位差距**：我们的治理状态停在资产级（发布态）与版本级（进化候选 vN-cK、qualitygate 版本档案）；概念/数据属性级无认证标记。
- **评估时机**：待 REQ-405（构建路径决策化）交付、构建动线稳定后，随「消费侧溯源声明」诉求一起评估；当前 qualitygate Findings+Evidence 三档（REQ-295）已部分承载「定义可信度」信号，不急。

### O1（顺修级，零立项）：guide 补「建议调用序」

Auto Ontology 握手 instructions 说明工具调用顺序（防模型抢用主工具）；我们 facade 挂载 guide（REQ-215 紧凑口径+REQ-357 IRI 模板）已近似——可在 guide 文案补一句「先 list_concepts/get_concept 定位再 sparql_query 执行」（随 facade 任一轮交付顺带，不单独立项）。

### L1（📚 学习素材，零实现）：Learn 环节=REQ-207 的业界同构

Auto Ontology 五环节把 Learn 列为一等环节（认证纠正回流本体）——与本项目进化闭环（诊断→归因→补丁，Evidence 锚定防本体幻觉，gate/accept 人工链）**同构且本项目更严**（其公开材料未见对位 Evidence 机械锚定）。价值：①方向验证（自进化不是我们独创的过度设计）②教学叙述素材——「语义层是 agent 的记忆系统，Learn 就是记忆的固化机制」可进 05 消费与审计/学习中心叙事。

### L2（📚 学习素材，零实现）：「90% 缩短」的批判性案例

官方「本体创建时间缩短 90%」与文章自我批判（语义工程转移而非消失）是一组现成的**诚实边界教学案例**：自动化的是起草，不消失的是治理。可进 31 号建模规范/学习中心方法论卡作批判性阅读素材——与本项目「LLM 生成必过门禁 ≤3 轮铁律」「不为改而改」的口径互证。

### 不做清单（对照即止）

- **指标语义层/BI 域**（metrics/facts/dimensions 组织单元）——非知识本体域，与平台定位不符（沿 74 号 M1 结论）；
- **NIM/FastAPI/Next.js 全栈引入**——D-O15 单机纯 Go 冲突；
- **SSO+RBAC 多用户体系**——Q-5 单用户边界；
- **text-to-SQL 生成主体路线**——我们 SPARQL+确定性翻译路线（REQ-94）为其对照，其 ask_question=NL-to-SQL+执行一体，与我们 NL2SPARQL 受控生成（REQ-333）形态不同但已有。

---

## 6. 风险对策映射（源项目自认风险 → 对本项目借鉴的约束）

| # | 风险/边界 | 对策 |
| --- | --- | --- |
| R1 | 不接受外部贡献（外部 PR 不审查） | 仅作对照参考，不形成依赖；借鉴以形态思想为主不引代码 |
| R2 | NIM 绑定（默认模型族）+ 明文凭据回退路径 | D-O15 独立评估，不受其栈选择影响 |
| R3 | 新项目成熟度（79★/22 open issues，发布 3 天） | 观察池项（D2）等其稳定后再评估；D1 只借形态不依赖其实现 |
| R4 | 「90%」营销口径 | L2 批判性采纳，不进任何决策依据 |

---

## 7. 开放问题

| 优先级 | 问题 | 影响 | 需要谁确认 |
| --- | --- | --- | --- |
| P2 | D1（可回答性预检）是否入 03 需求池 | 唯一立项决策点 | 董奎拍板 |
| P3 | Databricks Genie/Metric Views 是否值得专档（当前被 74 号+本档对比节覆盖） | 对比面完整性 | 有真实对标诉求再议 |

---

## 8. 版本记录

| 版本 | 日期 | 变更 |
| --- | --- | --- |
| v1.0 | 2026-10-24 | 初版：全景（架构/五环节/九工具/安全）+ 三方对比（引用 74 号防重复）+ 证据台账六条 + 十项映射 + D1/D2/O1/L1/L2 决策 + 不做清单；74 号同步增补 §9；15 号登记第七类 |
