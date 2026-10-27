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
- **本体前端梳理批 REQ-458~459 已交付 ✅·M254~M255（2026-10-27，立项自本会话「本体页面功能堆叠梳理优化」——开发者指令：详情类多标签页整体梳理〔合并整合或拆分〕+创建导入完成后返回上一步旧内容残留收口；立项即开发同轮收口）**：①**REQ-458 资产详情分区 17→11**（AssetsPage sectionGroups 普通型模板重构——关系类型+数据属性→「关系与属性」/公理+透传公理→「公理」〔两处内部 Segmented 分段切换〕、校验+质量卡→「校验与质量」/进化+影响分析→「演化与影响」/被引用+产物+TTL 导出→「引用与产物」〔纵向并置 onto-sec 子标题〕；大分区〔Spec 编辑/层级浏览/图形编辑/CSV 灌装/版本与源码/可视化〕与伴生型 6 分区不动；分区键仅页内消费〔外部深链只传本体 id〕零外部影响；顺修两处重复 setTabKey(null) 重置 effect 并一）②**REQ-459 构建流完成即新会话**（CustomBuildFlow handleCreated 清 custom-import-report·custom-ai-draft 双草稿+S1Source key 重挂载——治「导入创建完成返回上一步旧内容还在」〔四步面板常驻挂载+完成路径零清理+draft 持久化三因叠加〕；BuildFromKBFlow 入库成功清 result·specText·ontoName+savedName 承接展示；OntoExtendFlow apply 成功清 extend-rag-data 草稿+lovPicked·odpId·targetCQs；分治口径=表单内容与生成产物完成即清、用户偏好〔s1tab/KB 选择〕与工作对象〔目标本体〕保留）。明确不动在案=详情头部动作区 small 化已收敛/RuntimePage Drawer 5 页签与 AuditPage 3 页签合理粒度/LearnPage 三层折叠（学习内容非操作面）。验证=tsc+dist 绿+headless **43/43 两轮** smoke/req458（对运行中 dev 栈 :5173/:8080/:8091：A 段 34 断言〔导航恰 11 项+旧标签无+三合并区子区齐+两分段切换〕/B 段 6 断言〔粘贴导入跳②后返回①表单与报告全新+草稿键已清〕+teardown diff 自清+零 pageerror；爬坑=旧标签断言须整项比对防子串误命中〔「校验与质量」含「校验」〕/S1 重挂载后 importMode 回默认档〔重置生效旁证〕）。03 v2.08（§2.61 新节）/02 v0.349（M254~M255）/14 v0.93/18 v4.89/20 v3.50（4.126）/platform-knowledge 04_本体 02_构建·03_资产 synced/AGENTS 置顶。
- **Eino 能力深化批立项 REQ-452~457 📋·M248~M253（2026-10-27，立项自本会话「分析当前智能体实现是否充分使用了 eino 的能力，哪些能力没有使用且值得使用」——backend import 面×全部 adk/compose 调用点×eino v0.9.22 能力清单三方对照，开发者指令「将建议内容立即全部立项排期，后期按需触发开发」；只立项不开发按需触发；前置=eino 同日升 v0.9.22 c00a013 21 包全量绿）**：六件=REQ-452 模型调用重试与故障转移〔P1——adk RetryChatModel 幂等白名单 429/5xx/deadline/reset+FailoverChatModel 多连接备援切换 warning；缺口实锤=自研 retry.go DoWithRetry 仅盖 MCP 拉取两处、模型推理零重试零备援〕/REQ-453 eino callbacks 统一执行观测面〔P2——model/tool OnStart/End 计时审计进 run_event 替代 debugModel 模型单点包装〔tool 级含 MCP/委派零计时〕+TokenUsage cache write tokens 透出〕/REQ-454 中断恢复悬空 ToolCalls 修补〔P3——adk patchtoolcalls 中间件管运行内恢复层，与自研 context.go A1 历史投影层互补〕/REQ-455 计划执行工作流模板〔P3——adk prebuilt planexecute 接入装配面〕/REQ-456 上下文准入链 Retriever 接口化〔P4——recallChain 形式化 retriever 接口投影零行为变更，沿 REQ-355 反转先例〕/REQ-457 KB 文档解析器开源组件化评估〔P3 落 11 v1.8——eino-ext document parsers 对比自研 richtext.go 评估先行，D-O5 对齐〕。**明确不引入在案（防过度引入）**=summarization 中间件（自研 A3 持久化压缩更强）/adk filesystem·agentsmd·skill（与技能模块+基座十工具重叠）/compose.Graph（Q-7 不做画布）/flow multiagent（ADK 前旧 API）/prebuilt supervisor（自研协调者已集成审批审计）。**顺修=AGENTS.md 整文件重复副本删除+01/02/18 头部陈旧残留行删除（v2.07/v0.343/v4.82 并行写坏遗留）**；18 号撞号注=v4.86 被并行 REQ-133 桌面轮占用顺延 v4.87。01 v2.11（§3.13 新节）/02 v0.347（M248~M253）/11 v1.8/18 v4.87（六行+迭代表）/AGENTS 置顶。
- **REQ-133 桌面壳原生选目录+Windows 服务日志黑洞已交付（2026-10-27，10-19/20 并行会话在途 WIP 一周未动由本会话核出后代为收口——远端无等价物、两缺陷仍在最新 main，落地校验三件全过）**：①tauri-plugin-dialog 壳侧注册+capabilities/default.json（core/dialog 默认权限+**remote.urls 127.0.0.1:* 关键**——壳窗口终态加载 backend 托管 http 源，不声明则 __TAURI__ 不注入）+withGlobalTauri+前端 native.ts 运行时补丁（浏览器环境自识别回落后端端点行为不变；取消静默处理）——治桌面壳 CREATE_NO_WINDOW 无头 backend 进程 powershell 目录弹窗不可靠；②set_hidden 双平台统一日志重定向（原 Windows 分支 Stdio::null() 排障黑洞→数据根 logs/<svc>.log）；③顺带 format! 改 String 拼接两处+lib.rs 头注数据根名顺修。**验证**=apply 干净落 c00a013（REQ-325 轮 lib.rs 改动区域零重叠）+tsc 绿+cargo check 绿（干净 worktree 补桩）+REQ133-desktop-pick-dir-fix.md 新档（根因/构建/验证清单 7 项）；真机装包验证留 Windows 下一轮。18 v4.86 迭代行/AGENTS 置顶。
- **REQ-401③ ChatWindow 组件主体四域拆分已交付·M197 全项收口 ✅（2026-10-27，本会话「继续执行」领取件）**：四抽件 prop 化零行为变更——**ConvSettingsPanel**〔会话设置 ⋮ 面板：renameVal 状态留主件/伴生 toast 语义经 onCompanionChange 上抛保真〕/**ObservePanel**〔过程面板：显示档+采集档同面板+关系说明〕/**ComparePane**〔对比窗格 section：Splitter.Panel 留主件 map 回调/paneBadgeSelect 复用 chat/bits 既有 helper〕/**Composer**〔输入区整块：chips 元素构造留主件传 ReactNode/slash 面板+附件回显+隐藏 input〕——状态与 effect 全留主件，纯渲染+回调上抛；主件 2241→**1861** 行、四件均 ≤257（NFR-O-6 ≤400 达标）。验证=tsc+dist 绿+headless **17/17** smoke/req401b 两轮稳定（A 会话设置四断言含 patchConv 落库/B 过程面板三断言/C 对比五断言/D 输入区四断言+零 pageerror；发送真模型链路沿既有 M 档冒烟承载——零行为变更重构交互骨架逐点验收）。**REQ-401 全项收口**（①事件注册表 ②五域辅助 2026-10-24 在案）。01 v2.10/02 v0.346/18 v4.85/20 v3.49（2.84）/AGENTS 置顶。
- **REQ-400①③ 配置保存链路收口已交付·M196 全项 ✅（2026-10-27，本会话「继续执行」领取件；①选型单源载荷构造器非 PATCH——full-replace 保持零破坏面）**：①**单源载荷构造器**=web/src/api/payload.ts 新件 agentFullPayload/projectFullPayload——实体基座=服务端返回对象剔除 id/created_at/updated_at/is_builtin 后整体透传（后端新增字段自动防清零，根治手写清单漂移），表单值仅覆盖其管理字段（undefined 跳过/null 除 model_conn_id 显式允许外回落实体；mcp_serve token 原样保留/hooks 扁平键收拢实体形态）③**四处手写清单收敛**=AgentSidePanel 主保存+伴生绑定路径（本地构造器删除）/ProjectSidePanel 配置+协作两视图/HybridTeamTemplate 全接入，If-Match 补挂伴生与项目配置两路径。**两处现网清零 bug 顺修实锤**=手写清单漏 companion_extract_cfg〔伴生抽取高级设置每次保存被清〕+伴生绑定路径漏 connectors/tool_approval/approval_exempt/approval_timeout_hours/onchat_mode/onto_ops/hooks 七字段〔伴生保存清零审批档/连接器授权/能力开关〕。验证=tsc/dist 绿+headless **16/16** smoke/req400b 两轮稳定（隔离 8099 全栈；D4~D6 协作视图下拉沿 REQ-311 P2 antd v6 headless 限制口径 skip——保存语义同走 projectFullPayload 已由 D1~D3 实证）；冒烟环境爬坑入库=backend WEB_DIST 相对 CWD 解析〔错 CWD 静默服务旧 dist 假阳性实锤〕/端口变量 ADDR/teardown 必须清伴生本体〔堆积顶出下拉选项〕。台账顺修=01 §3.10 REQ-400 行立项态滞后一并转正。01 v2.09/02 v0.345/18 v4.84/20 v3.48（2.83）/AGENTS 置顶。
