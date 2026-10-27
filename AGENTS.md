# AGENTS.md — AI 编码助手项目引导

> 本文件面向 AI 编码工具（zcode / Claude Code / Cursor 等）。人类用户请看 README.md。

## 项目定位

「智能体构建平台」——Go + CloudWeGo Eino/ADK 的智能体构建平台，**工程化产品定位**（2026-10-24 Q-21：主定位由学习平台叙事升级为工程化产品——优先级裁判=工程化成熟度〔可靠性/安全/可运维/性能〕，学习价值〔学习中心/任务卡/示例〕保留为产品能力；单机可部署与既有边界决策〔单用户 Q-5/不做画布 Q-7 等〕不自动反转），六模块 + 设置（智能体 / 项目 / 本体 / 知识库 / 技能 / 📖平台知识 REQ-116/161 / ⚙设置；OpenOntologies 双轨已退役〔2026-10-08，66 号方案 B，REQ-100 ⛔〕）。双核心：**智能体工程**（对话 / 多智能体协作 / 知识·技能·本体增强，01）+ **本体工程**（构建 → 资产 → 运行 → 消费审计闭环，03），能力边界诚实标注（如 KG 增强检索为轻量实现）。

## 仓库结构

- `backend/` 主平台后端（Hertz :8080，Eino ADK 编排、SQLite、SSE 流式）
- `ontology-service/` 本体构建平面（:8091，spec_json 多形态资产、LLM 辅助创建、自研 KG：SQLite 三表 + PROV-O 导出）
- `runtime-manager/` 本体运行平面（:8090，多引擎方案管理：Oxigraph / Fuseki）
- `web/` React 前端（Vite + AntD；本体模块五栏 IA：学习中心 / 构建 / 资产 / 运行 / 消费与审计）
- `tools/` 工具脚本（`tools/semantica-worker/` 已归档休眠——代码保留、**勿重新启用**，D-O15）
- `website/` 项目官网（GitHub Pages，REQ-238：vite+React 独立小站；构建期内联 platform-knowledge 导读档与种子 spec——内容单源；`.github/workflows/website-deploy.yml` 推送 main 自动发布，首次启用需 Settings→Pages 选 GitHub Actions）；- `website/` 项目官网（GitHub Pages，REQ-238：vite+React 独立小站；构建期内联 platform-knowledge 导读档与种子 spec——内容单源；`.github/workflows/website-deploy.yml` 推送 main 自动发布，首次启用需 Settings→Pages 选 GitHub Actions）；- `deploy/` Docker Compose + Helm + Windows/macOS 桌面客户端（REQ-321/REQ-324）；`seeds/` 种子数据与学习中心内容包（学习包/方法论/外部资源单源）
- `docs/` **需求与开发进度统一管理目录**（G-5，2026-09-27；文档内维护人称谓一律用「开发者」，G-6）：需求事实源（01/03/11）+ 工程活文档（02/04/12/14 方案与里程碑进度）+ 治理台账（15/18）+ 冒烟清单（20）；`platform-knowledge/` **平台知识目录**（REQ-161/G-5）：按模块子目录放调研/开发/产品设计文档与模块导读——**页面「平台知识」与目录一致，agent 维护的文档人在页面可见**（REQ-161/169/184）；**收录规则（2026-10-01 重组，开发者指令「文档自顶向下按内容分模块组织管理」）**：各模块目录下 `08_调研预研/` 收录该模块的资料与调研预研（开源分析/调研报告/未拍板方案/已交付方案依据档）；模块根放**子模块实现状态文档**（每子模块一档记录当前实现+演进，本体=01_学习中心~05_消费与审计 五档已落、智能体=53~56 号先例，其余模块按需补齐）；全局 `08_调研预研/` 只收跨模块调研并承载收录规则总则（判定口径见 `platform-knowledge/08_调研预研/00_调研预研.md`；原 09-29「未拍板归全局 08」规则已演进，档案已按内容分流）；**模块生效功能总账补则（2026-10-24）**：各模块 `00_XX模块.md` 维护「已交付能力总表」（横切/模块级生效功能，状态权威=docs/02 §12+18 号），子模块实现状态档维护栏内能力表——交付回写**默认必写其一**（原「按需补齐」口径收紧）；跨子模块横切件指定唯一归属档（本体 facade 工具面=04_运行）

## 开发前必读（按需读，不必全读；版本号不在此 pin，以各文档头部为准）

| 文档 | 内容 |
|---|---|
| `docs/18_REQ编号注册表.md` | **REQ 编号唯一分配权威**——新需求立项先查此表；含 90~96 冲突裁决（本体学习增强保留 90~96；智能体侧技能=REQ-120、挂载=121、沙箱=122、Agent 级配置=123、折叠树=124、项目文件=125、产物=126）与文档编号约定 |
| `docs/01_智能体_需求文档_PRD.md` | 智能体平台**唯一需求事实源**：REQ 全表 + 决策 Q-1~20 + 验收（§6）+ 需求池（§9，含触发条件） |
| `docs/03_本体_需求文档.md` | 本体模块**唯一需求事实源**：REQ 全表 + 决策记录 D-O1~16（§5）+ 验收要点（§7）+ 迭代记录（§8） |
| `platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md` | **产品/界面口径事实源**（2026-09-25 自 docs/ 迁入平台知识）：北极星与产品原则、五栏/导航 IA、构建路径分类学、用户旅程 J1~J4、REQ-111/112 |
| `platform-knowledge/02_智能体/53~56 号` | **智能体五层域事实源**（2026-10-01 沉淀）：[53 Harness] / [54 Context] / [55 Loop] / [56 Graph] 各一档——层职责+代码地图+已交付能力表（REQ·里程碑映射）+开放问题；新增各层演进默认落对应档；模块全域导航见 00_智能体模块「模块文档地图」 |
| `docs/04_本体_方案设计.md` | 本体模块方案设计（需求与方案分离） |
| `docs/02_智能体_技术方案设计.md` | 智能体平台技术方案、API 面、里程碑（§12 含状态列） |
| `docs/12_知识库_方案设计.md` / `docs/11_知识库_需求文档.md` | KB 模块（RAG/GraphRAG 双子模块，KG 自研内置 backend） |
| `docs/14_本体_前端改造方案.md` | 前端结构与改造史（当前态以此为准） |
| `platform-knowledge/01_整体设计/16_部署与运行.md` | 本地 / Docker / K8s 三条部署路径 |
| `docs/20_回归冒烟清单.md` | **交付质量资产**：每个独立任务交付后按清单跑核心动线（纪律 #5 的落地载体） |
| `docs/15_开源项目及论文登记簿.md` | 全局台账：**新引入开源项目/论文必须先登记**（`platform-knowledge/04_本体/08_调研预研/方法论与理论/19_本体_semantica集成方案.md` 为其存档，原 15 号；2026-10-01 归入本体调研预研专区） |
| `platform-knowledge/04_本体/08_调研预研/07`（工具链清单）/ `08_调研预研/08`（自研边界）/ `08_调研预研/13`（学习缺口分析）/ `platform-knowledge/04_本体/90`（底层实现原理代码地图） | 决策依据（G-5 已迁入平台知识，编号不变；2026-10-01 本体重组为子模块实现状态文档+调研预研专区） |
| `platform-knowledge/`（各模块子目录） | **调研研究已整体并入**（G-5，原 `research/` 18 份按域迁入 智能体/本体/总览 子目录，文件名不变）——各决策的 why（为何选 Eino、为何不自研 GraphRAG 等），映射见 `platform-knowledge/README.md` |
| `platform-knowledge/` | **平台知识目录**（REQ-161/G-5）：整体设计/调研/开发文档 + 各模块导读，七个子目录与页面树 1:1（整体设计/智能体/项目/本体/知识库/技能/设置）；**收录与归属目录驱动，页面展示顺序按导航模块规划**（REQ-169 交付轮）；维护约定见 `platform-knowledge/README.md` |

## 硬性纪律

1. **需求先行**：新需求/需求变更先在对应需求档落档（01/03/11 按归属；编号先查 `docs/18` 注册表；版本递增 + 迭代表留痕），再实现；实现交付后回写相关文档（回写前先读各文档头部最新版本号，避免撞号）。
2. **git**：只 add 本次实际修改的文件路径（禁止 `git add .` / `git add -A`）；commit message 概述变更要点；commit 后 push origin/main。**任务边界即提交**：每完成一个独立任务、或开始新独立任务前，不留未提交改动。
3. **权威 = origin/main**；本仓库（macOS 正身）是唯一开发地，云盘副本只读。
4. **自研边界**：开源优先、避免重复造轮子（D-O5）；**不接入 Python 运行时依赖**（D-O15：`run-dev.sh` 一条命令启动零 venv，验收 22）。
5. **验收**以 03 §7 / 01 §6 验收要点为准；前端改动用 headless Chrome（CDP）按 **`docs/20_回归冒烟清单`** 冒烟验证（选改动面对应模块节；★ 主链路建议全跑）。冒烟截图留本地 `smoke/`（.gitignore 不入库，勿 `git add -f`——2026-09-27 治理：二进制截图不再进仓库）。
6. **活文档**：已确认需求不删除，变更标注 `（已变更/已废弃，见 vX.X）`；文档正文与实现不符时，以实现为准并回修文档。已完成 REQ 的行内交付注压缩为指针、AGENTS.md 当前状态旧条目滚动移出**不属于删除**（全量落点在各档迭代表/18 号注册表/20 号，git 历史兜底；G-7/REQ-375 口径增补）。
7. **知识同步**：需求/方案档发生**语义级变更**（新增/变更 REQ 行、新增/反转 D-* 决策、口径退役）的同一轮交付里，必须同步更新 `sources` 命中的 `platform-knowledge/` 模块导读页（每篇头部有源指针 frontmatter；原 seeds/learning/reference/，2026-09-25 迁入）；纯笔误/版本递增不触发。知识分层职责见 17 §1.4，页面渲染检查在 20 号冒烟清单 S1。
8. **交付详录单源（G-7/REQ-375）**：交付细节（实现清单/验证数据/爬坑/诚实边界）全量只落**两处**——对应档迭代表 + `docs/20` 冒烟清单；AGENTS.md 当前状态、REQ 表行、02 里程碑行只写**一行状态+指针**（「交付注见迭代表 vX.X」沿 REQ-214 行先例）；每次交付回写时，顺手把本次触碰文档内**已完成 REQ 的行内长交付注压缩为指针**（滚动压缩防膨胀）。
9. **大档按需读（G-7/REQ-375）**：01/02/03/18/20 等大档先读头部导读与版本号，grep 定位行号后按 offset 分段读，**禁止无目的全文通读**；纯分析/盘点类任务派 Explore 子代理读档回传结论。`tools/docsize.sh` 交付回写时顺手跑，单档超阈值即触发指针化/压缩。

## 当前状态（滚动窗口·最近 5 条，由协作 Agent 维护）
- **REQ-451 平台知识阅读补全与全员化已交付 ✅·M247（2026-10-27，立项自本会话「如何为智能体（特别是平台助手）赋予平台知识阅读能力」分析——REQ-186 三级管道〔sync→search→doc_read〕四缺口盘点实锤，开发者指令「按建议立项开发」；立项即开发同轮收口）**：①**启动自动同步**（SyncPlatformKBStartup 后台 goroutine mtime 对账——治懒触发陈旧窗口；真机实证冷启建库 149 篇登记+official=true，本机 embedding 未配置=诚实 failed 态恰凸显词法链价值）②**doc_list 目录发现**（遍历 docs/+platform-knowledge/ 两根列 .md+frontmatter topic/desc 摘要+prefix 过滤——治基座 prompt 静态目录树漂移，prompt 目录降速览 doc_list 为正源）③**doc_grep 词法定位**（正则/字面量 file:line:text+非法正则降级字面量——治 embedding 单点依赖降级链断裂，词法全链恒可用；resolveDocPath 抽取三工具共用白名单解析）④**doc_read 分段读取**（offset_line/max_lines 分页+续读偏移+截断附「共 N 行」——治 48KB 截断大档后半不可达；爬坑=文件尾换行多计一行→行数归一）⑤**全员化产品面**（tool.Entry.Group+/api/tools+前端「平台知识」分组徽标〔AgentModal+侧板〕/KB official 标注〔chat.PlatformKBName 单源判定〕+知识库页与对话 KB 选择「官方」徽标）；基座八→十工具三处同步+**AssistantPromptHistory 历史基座清单**+SyncBuiltinAssistantPrompt 启动覆写（行内=历史基座→当前基座、真实微调永不动——治 prompt_tuned 误报）。验证=单测九组新增（chat/store/api 三包）+backend 21 包全量绿+tsc+dist+headless **14/14** smoke/req451 两轮（A 段 API 六断言/B 段真实 UI 六断言/C 段 route 改写机器无关；冒烟爬坑=tools 字段在「高级配置」Collapse 内默认折叠不渲染→先展开/Escape 只收下拉不关 Modal→两级 Escape+遮罩 detach/B3 断言对创建弹窗错位〔禁选是侧板编辑态行为〕→改分组缺失断言；WSL libasound 沿 stub 先例）。诚实边界=向量检索质量依赖 embedding 配置（未配置词法链兜底如实）/official 为 api 层判定非持久化列（名称即幂等键）/基座 prompt 历史清单手工维护。01 v2.08/02 v0.344/18 v4.83/20 v3.47/02_智能体 synced/AGENTS 置顶。
- **REQ-372 P2 知识图谱层缺陷修复与交互补全已交付 ✅（2026-10-27，「按建议全部修复」批——知识沉淀模块审计〔P0 功能失效+P1 布局崩坏实锤〕全数收口，零占号迭代）**：P0 三件=①ParseCardText 单行内联数组解析（真实卡 prereq `["a.md"]` 形态此前不被解析——13 卡 0 边、学习路径退化单卡，单测行式 fixture 掩盖）+校验器同口径修②节点 id 双前缀幂等修复（前端按示例 id 查询必失败根除）③面板布局崩坏修复（顶层渲染成并列 flex 子项挤阅读区 1084→210px——移入内容列）。P1 三件=断链随读随检（ScanBroken）+空图摄取失败 ingestError 透出（原静默「节点 0」无解释）+断链警示竞态消除+导读/主档不入图（15→13 节点）。P2 交互四件=卡选择器+路径/分型卡点击直达阅读区+刷新 message 反馈+收起持久。验证=单测四组新增+backend 全量绿+tsc+dist+headless **17/17** smoke/req372/graph.mjs（冒烟补课 20 v3.46 4.125——P2 原无冒烟行为带病交付根因）+L1 9/9 回归。诚实边界=KNOWLEDGE_ROOT cwd 敏感未根治（错误文案已三要素化含解析路径）；sources 面板不可见与 GraphRAG/llms.txt 维持 58 号 §11.3 演进项。01 v2.07/18 v4.82/20 v3.46/58 v1.4/00_导读 synced/AGENTS 置顶。
- **REQ-446 workflow/transfer 消费外部成员已交付·M242 ✅（2026-10-27，A2A 协议深化批⑥收口——本会话「检查还有哪些需求没有实现继续执行」领取件：状态列扫描定位深化批最后可领取项〔447 触发驱动维持〕）**：①**外部成员 adk.Agent 适配桥**（external_member_agent.go 新件——Eino Agent 三方法最小实现/Run 单事件 assistant 输出/任务取末条用户消息〔上下文隔离语义两轨一致〕/事件带 AgentName 流转）②**装配判定 specs 单源**（a2aMemberSpecs/cliMemberSpecs——工具轨〔agent_as_tool〕与节点轨共用判定与 deps 防漂移；CLI docker 沙箱三前置闸两轨同款）③**invoke 外壳共用**（a2aMemberInvoke/cliMemberInvoke——委派单/超时/REQ-319 降级两轨一致，工作流节点降级载荷作输出继续流转）④**两轨接入**（workflow 外部节点连接器序追加本地成员后〔WorkflowMeta.Nodes 含外部名——节点事件/回放自动覆盖〕+transfer 进 SetSubAgents 转移面〔描述复用卡片/CLI 合成〕；诚实边界告警=外部节点无审批面）。验证=单测七组（适配器执行/降级流转/任务提取/workflow A2A·CLI 装配/transfer/沙箱闸/**ADK Sequential 全外部节点真工作流集成**）+backend 21 包全量绿（TestACPAdapterResumeFlow 存量偶发与本批无关——干净基线复现实锤）。**顺修台账滞后两行**=REQ-392/403 状态列（02 M188/M199 ✅ 为准）。开发环境注=主工作树被并行 WIP 挡编译，沿 v3.94 先例走 git worktree 于 origin/main 独立提交。18 v4.81/01 v2.06/02 v0.343/20 v3.45（2.82）/00_智能体 synced/AGENTS 置顶。
- **REQ-450 创建流程可发现性与推理后端安装引导已交付·M246 ✅（2026-10-27，立项自本会话「创建智能体交互引导与推理后端安装支持」分析——开发者指令「按照建议开发」；立项即开发同轮收口）**：①**创建表单外部智能体可发现性**（AgentModal「后端身份」分区提示行+「设置→连接器」深链沿 goConnectorSettings 先例——外部 A2A/CLI 智能体非创建形态〔连接器注册→入项目授权为委派成员〕，Q-22/REQ-132② 定位不变只补可发现性）②**未安装推理后端安装引导**（inference registry installHints 单源：claude-code/opencode/aider 安装命令+官方文档、dsh 系无公开命令仅凭据说明不臆造；Status install_cmd/install_docs/install_note 透出；前端 InferenceInstallHint 新件「安装指引」Popover=命令可复制+文档链接+「重新探测」POST reprobe 即点即刷〔AgentModal+侧板共用〕+未发现选项 hover title）③**探测失败保底**（inferenceBackendOptions 空清单保底注入 eino-adk——原 catch 静默下拉空）。**引导安装非代为安装**：安装器异构+长时下载不进创建主链（REQ-146 oxigraph 单一资产先例/fuseki 仅指引对照），未发现选项维持置灰（选择只允许已发现后端）。验证=inference 单测+backend 全量绿+tsc+dist+headless **23/23** smoke/req450 两轮（API 三型断言/弹窗提示+深链落连接器页/aider 置灰+Popover+重新探测 200/route 拦截→保底/route 改写→命令型+说明型同屏；冒烟爬坑=本机已装 claude-code/opencode/dsh 仅 aider 缺——Popover 只列未装项是正确行为，断言改真实数据+route 改写双路）。01 v2.05/02 v0.342/18 v4.80/20 2.72/00_智能体 synced/AGENTS 置顶。
- **Protégé 手动建模兑现批 REQ-448~449 已交付 ✅·M244~M245（2026-10-26，立项即开发同轮收口——立项自本会话「手动建本体完整性核查」：03 v1.96 两项「与 M221 同窗」承诺脱窗悬空〔HierarchyPane 只读零 mutate/AddInstanceModal 裸 JSON 实锤〕，开发者指令「补齐未实现的部分，开始开发」）**：①**REQ-448 树上编辑**（REQ-363 P2③ 兑现——HierarchyPane 类树 hover 钮「＋子〔挂选定父〕/挂父〔多选并入去重〕/删〔Popconfirm+悬空 400 如实回喂〕」+根概念钮+DescPanel 父 Tag ✕ 摘父+实例树删除，全走 REQ-436 元素端点 kind=concept〔presence 语义只传 parents 即改父集〕；「重排 parents」=挂父/摘父，拖拽 DAG 复现语义模糊留观察；**爬坑=ops 钮 opacity:0 仍命中点击**〔测试误触隐形 Modal 实锤〕→visibility:hidden 产品测试双修）②**REQ-449 实例表单化**（AddInstanceModal 按声明属性〔直接 Domain+Parents 继承收集环防护〕生成 range 五型控件〔string/integer/number/boolean/date→DatePicker〕，提交合并 attributes〔表单为声明属性正源〕，JSON 降「高级」；顺修添加实例 resetFields 全清防 dpVals 串实例）。验证=tsc+dist 绿+headless **12/12** smoke/req448 两轮（真全栈：树上建 B 挂父 A→API parents=["A"]/摘父→[]/实例表单 year=2024 经「保存 Spec」落库断言/零 pageerror；冒烟爬坑=getByText first 命中隐藏「保存 Spec」→button 定位）。诚实边界=概念字段编辑沿既有面板（树管层级画布管拓扑）/实例编辑沿 InstancePanel（本件收新增路径）。03 v2.06/02 v0.341/18 v4.79/02_构建 synced/AGENTS 置顶。
