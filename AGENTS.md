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
- **REQ-420 手动建模完整性已交付·M216 ✅（2026-10-25，「领取剩余全部立项需求开发」批第七件——业务本体批 B2 v1.95 扩容）**：①DataPropertiesPane 只读→CRUD（新增/编辑/删除走 REQ-436 元素端点 kind=attribute——Validate+版本门禁服务端单源；表单 name 锚定/domain 概念下拉/range 五类型；空态同入口）②RelationTypesPane 关系声明编辑补全（kind=relation——新增全字段；编辑=三元组锚 name:from:to 锁定+诚实提示改端点删除重建；evidence 保留透传）③client.ts elementUpsert/elementDelete（REQ-436 端点 UI 消费首例）。headless smoke/req420 **7/7**（真全栈反代串联）。爬坑=删除按钮作用域防误匹配本体级删除+空态提前返回挡入口。诚实边界=画布渲染公理/数据属性随可视化轮。03 v2.02/02 v0.332（M216 ✅）/18 v4.69/20 v3.38（2.78）/02_构建 synced/AGENTS 置顶。
- **REQ-419 概念同义词 TBox 一等公民已交付·M215 ✅（2026-10-25，「领取剩余全部立项需求开发」批第六件——业务本体批 B1 优先领取项）**：①spec Concept.Synonyms 词表②TTL 双向=altLabel 读入逐值捕获（不再丢弃入余量）+写出 skos:altLabel 对偶+prefLabel 锚名防名称漂移+skos 前缀条件性（sidecar 字节级对拍锁保持——存量输出零变化）③check_answerable 查询 UNION altLabel（业务口语 TBox 层命中断裂修复，REQ-415 命中面地基）④操作面=元素端点/工具 synonyms 参+图形编辑器（面板/新增 Modal）编辑。round-trip 测试+对拍锁保持+三服务全量绿+tsc/dist。诚实边界=命中随 Reload 生效；ABox alias 互补。03 v2.01/02 v0.331（M215 ✅）/18 v4.68/20 v3.37（2.77）/02_构建 synced/AGENTS 置顶。
- **REQ-436 元素级语义上收与 facade 薄化已交付·M232 ✅（2026-10-25，「领取剩余全部立项需求开发」批第五件——本体接口统一主线）**：①构建面一等 REST=rest_elements.go 八端点（elements/{kind}[/name] 四类按名 upsert/delete——mutation 语义逐字移植 presence 口径）+persistSpec 保存核单源（从 saveSpec 提取：Validate+strict+overlay+版本推进+入库评分，整包保存与元素端点共用）②facade 转薄=elementsDo 透传（八 handler 改元素端点；AX 适配层保留；specMutate/putSpec 客户端编排删除；21 件工具行为零变化）。双服务测试全绿+真机串联冒烟 5/5（真实双服务 facade 工具→元素端点→落盘核实）。诚实边界=页面整包 PUT 未切（择机）；OpenAPI 导出触发驱动。03 v2.00/02 v0.330（M232 ✅）/18 v4.67/20 v3.36（2.76）/04_构建·04_运行 synced/AGENTS 置顶。
- **REQ-435 运行面能力工具化已交付·M231 ✅（2026-10-25，「领取剩余全部立项需求开发」批第四件——本体接口统一批①，治 facade「运行面零覆盖」最大缺口）**：①facade_runtime.go 三组 12 件=方案管理 7（list 只读+create/update/delete/start/stop/reload 写类六件——生命周期语义与 rest 同源）+引擎运维 3（engines_status+engine_install 工具内轮询沿 cq_benchmark_run+profile_logs）+观测 2（profile_trace+route_ambiguities）；Manager 未接=不注册构造兼容，runtimed main 共用实例②backend 门控=OntoOperationTools 13→25 件（onto_ops 白名单）+写类七件入 danger 档③caller 归因搭车=写类工具可选 caller 参数→trace_log runtime_audit 行（profile_trace 读回；版本快照 agent 列仍预留）。单测四组 facade 绿+backend 21 包绿+**真机 MCP 冒烟 19/19**（tools/list 12 件+tools/call 全链归因读回）。诚实边界=真实引擎生命周期随全栈轮；caller 自报形态。03 v1.99/02 v0.329（M231 ✅）/18 v4.66/20 v3.35（2.75）/04_运行 synced/AGENTS 置顶。
- **REQ-432 A2A client 交互补强已交付·M228 ✅（2026-10-25，「领取剩余全部立项需求开发」批第三件——演进批⑦，REQ-426~432 全批收口）**：①a2a.SendStream（message/stream SSE：capabilities.streaming 随行+对端不支持/断流回落 send+轮询+快照增量 a2a_stream 过程转译沿 external.audit）②input-required 即判交互通道（结构化透传协调者 question+hint「转述问题附答案再委派」；非失败态不 run.warning 不轮询）③pushNotification 评估不做（单机无公网回调——SSE 已给实时性，回写在案）。单测三组+a2a 直测 21 包绿。诚实边界=server 侧 streaming=false 维持如实声明；真实 SSE 对端随凭证批。01 v1.99/02 v0.328（M228 ✅）/18 v4.65/20 v3.34（2.74）/00_智能体 synced/AGENTS 置顶。


> **滚动窗口规则（G-7/REQ-375，2026-10-22）**：本节仅保留**最近 5 条**交付摘要——新条目置顶、超出窗口的旧条目直接移除，历史由 git 历史 + 18 号注册表 + 各档迭代表承载；本节是「最近动态速览」非事实源（事实源=01/03/11+18 号）。条目篇幅遵循纪律 #8：细节落迭代表与 20 号，此处只写指针级概述。

