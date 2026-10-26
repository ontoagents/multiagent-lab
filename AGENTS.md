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
- **Protégé 手动建模兑现批 REQ-448~449 已交付 ✅·M244~M245（2026-10-26，立项即开发同轮收口——立项自本会话「手动建本体完整性核查」：03 v1.96 两项「与 M221 同窗」承诺脱窗悬空〔HierarchyPane 只读零 mutate/AddInstanceModal 裸 JSON 实锤〕，开发者指令「补齐未实现的部分，开始开发」）**：①**REQ-448 树上编辑**（REQ-363 P2③ 兑现——HierarchyPane 类树 hover 钮「＋子〔挂选定父〕/挂父〔多选并入去重〕/删〔Popconfirm+悬空 400 如实回喂〕」+根概念钮+DescPanel 父 Tag ✕ 摘父+实例树删除，全走 REQ-436 元素端点 kind=concept〔presence 语义只传 parents 即改父集〕；「重排 parents」=挂父/摘父，拖拽 DAG 复现语义模糊留观察；**爬坑=ops 钮 opacity:0 仍命中点击**〔测试误触隐形 Modal 实锤〕→visibility:hidden 产品测试双修）②**REQ-449 实例表单化**（AddInstanceModal 按声明属性〔直接 Domain+Parents 继承收集环防护〕生成 range 五型控件〔string/integer/number/boolean/date→DatePicker〕，提交合并 attributes〔表单为声明属性正源〕，JSON 降「高级」；顺修添加实例 resetFields 全清防 dpVals 串实例）。验证=tsc+dist 绿+headless **12/12** smoke/req448 两轮（真全栈：树上建 B 挂父 A→API parents=["A"]/摘父→[]/实例表单 year=2024 经「保存 Spec」落库断言/零 pageerror；冒烟爬坑=getByText first 命中隐藏「保存 Spec」→button 定位）。诚实边界=概念字段编辑沿既有面板（树管层级画布管拓扑）/实例编辑沿 InstancePanel（本件收新增路径）。03 v2.06/02 v0.341/18 v4.79/02_构建 synced/AGENTS 置顶。
- **A2A 协议深化批 REQ-441~445 已交付 ✅·M237~M241（2026-10-25，立项即开发同轮收口——立项自「A2A 深化优化分析」〔a2a.go/client.go/a2a_member.go 代码级精读〕，开发者指令「建议内容全都立项，按优先级排序开发」；REQ-446/447 维持 📋 挂 M242/M243）**：①**REQ-441 状态机与取消可靠性**（cancel 后台完成覆写竞态/cancel 不落库重启对账漂移/client 从不调远端 tasks/cancel 弃单白跑/并发 send 双跑无闸——四点全修，Busy 闸 -32003）②**REQ-442 server 侧 message/stream**（streaming 拉真+SSE 快照流〔运行内联请求生命周期/500ms 节流〕+RunStreamFunc=chat.Run delta 管线；**平台自吃回环单测**；REQ-432 client 半边对称收口）③**REQ-443 交互语义闭环**（审批闸门 -32004 沿 /mcp 先例+ask_human 挂起→input-required 终态问题文本外发+同 contextId 携答案 Resume 续跑——跨协议人机交互闭环）④REQ-444 card provider·securitySchemes+错误脱敏+**status.message wire 顺修**（ErrorText 此前从不上 wire）⑤REQ-445 map 淘汰+30 天保留窗口+回读不回插（爬坑=rows 未关同池 Exec 死锁 600s 实证）。单测十一组+a2a/store/api 三包全量绿。01 v2.04（§3.12 前五行 ✅）/02 v0.339（M237~M241 ✅）/18 v4.77（五行+迭代表）/00_智能体 synced/AGENTS 置顶。
- **REQ-325/332 Windows 待办销账轮已交付（2026-10-25，「拉取最新代码，领取开发 Windows 环境相关需求」批——盘点=挂起 📋 均为触发驱动项，Windows 环境待办即三件销账；零占号纯迭代，详 18 迭代表 v4.74）**：①REQ-325 待办销账=**Windows 桌面端 sidecar python 探测**——Tauri 壳 probe_sidecar_python 跨平台化（py/python/python3 真执行 import rdflib 滤 WindowsApps 空壳+where 解析绝对路径+CREATE_NO_WINDOW）+legacy Go launcher 同款注入（单测 1 组）；已设 SIDECAR_PYTHON 则尊重（对齐 run-dev.sh，unix 侧此前无条件覆盖微调如实注）；NSIS 装机 RDF/XML owl_rdfxml 导入实证全链（×0.4/0.5 双包）——Windows 桌面端 OWL/RDF-XML·JSON-LD 导入+REQ-328 canon 随之打通；②REQ-332 Windows 真机两件销账=装机直落 8180/8191/8190（NSIS /S→4s 三 healthz 零顺延）+覆盖升级保留 launcher.json（显式 18080 系精确生效〔C 段 Windows 补证〕→同包重装逐字节保留→升级后仍生效，两轮实证）；③REQ-184 漏网顺修=Tauri 壳 RESEARCH_ROOT env+resources/research 映射第三处（backend 零消费；gitignored 目录掩盖下**干净检出/CI 构建会失败**）删除+debris 清理+0.5 复装复验。20 号 S8 现役 Tauri/NSIS 口径顺修+8.7 sidecar 检查点。诚实边界=py 候选依赖 python.org 装机默认；未探测到不注入人话指引；硬杀壳 pidfile 竞态碎屑（--stop/正常退出清理）。03 v2.04/01 v2.02/16 号 v0.29/20 v3.43/deploy README sidecar 注/AGENTS 置顶。
- **REQ-440 KB 索引异步化已交付·M236 ✅（2026-10-25，KB-8③ 提级「按照建议立项，开发」——REQ-439 交付边界点名项）**：①导入/重建索引端点**落库即返回**（status=indexing，HTTP 不再等 embedding/KG 抽取——graphrag 大文档导入 6ms 秒回，同步阻塞数分钟病灶根除）②kb 包后台**串行索引队列**（NewService 起单 worker；ctx=Background+10min 脱离请求生命周期；与同步语义等价防 SQLite 并发写）③**GraphragInfo 落库回读**（迁移 067 graphrag_json——顺修「仅导入当次回显、刷新即丢」，KG 徽标/失败原因持久可见）④前端 useDocsPolling（2s 轮询至稳定，三类型页共用）+toast 真实化。单测三组+backend 全量绿+tsc/dist+headless 9/9 smoke/req440。诚实边界=整库重建三端点维持 KB-8 池；并行多 worker 留池；队列进程内（重启丢任务=停留 indexing，重试=重建按钮）。11 v1.7/12 v0.16/02 v0.337（M236 ✅）/18 v4.75/20 v3.44（5.9）/00_知识库 synced/AGENTS 置顶。
- **REQ-439 KB 基础可用收口已交付·M235 ✅（2026-10-25，「review 知识库模块方案和实现，补齐基础可用的能力」）**：review 结论=知识库文档面需求基本全交付，实查缺口集中在已立项能力 UI 断链与需求字段 UI 未展示——①REQ-414 富格式**前端断链**收口（后端 richtext.go 已交付但 UI accept 仅 txt/md/file.text() 读二进制失真/client 不传 filename）：accept 扩 .pdf/.docx+**content_b64 二进制通道**+filename 透传②REQ-54 字段级补齐：doc.dim 迁移 066 落库（12 §3.3 口径实现成立）+文档表「维度/更新时间」列③上传文案诚实化（「索引在后台进行」实为同步）。单测三组+headless 13/13 smoke/req439+doccheck 绿。诚实边界=KB-8 索引/KG 抽取异步化维持 P3 触发池；OCR/表格维持 KB-2 池。11 v1.6/12 v0.15/02 v0.336（M235 ✅）/18 v4.73/20 v3.42（5.8）/00_知识库 synced/AGENTS 置顶。


> **滚动窗口规则（G-7/REQ-375，2026-10-22）**：本节仅保留**最近 5 条**交付摘要——新条目置顶、超出窗口的旧条目直接移除，历史由 git 历史 + 18 号注册表 + 各档迭代表承载；本节是「最近动态速览」非事实源（事实源=01/03/11+18 号）。条目篇幅遵循纪律 #8：细节落迭代表与 20 号，此处只写指针级概述。

