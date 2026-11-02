---
module: 智能体
topic: DeepSeek Super-agent 原型调研与借鉴分析
desc: dsh-superagent-prototype（2026-10-31 开源，MIT，TypeScript）工单化主动式智能体编排层对照拆解：Source→Trigger/Thread→Eval(gate+assessment)→Mandate/Ticket→Assignment→Effects/Outbox 五支柱 vs 本项目五层域逐层对照；借鉴清单三级=A 可直接落 3 件/B 触发驱动进池观察 8 件/C 工程文化 2 件+不借鉴边界；暂不立项——采纳任何一项时另轮领号并先过 docs/15 登记
synced: 2026-11-02
docs: [docs/research/02_智能体/08_调研预研/外部平台调研/41_DeepSeek-Harness_实现方案深度调研.md, docs/research/02_智能体/31_智能体对接DeepSeek-Harness_可行性及方案分析.md, docs/research/02_智能体/36_智能体开发架构深度对比_本地实现与DeepSeekHarness.md, docs/research/02_智能体/57_运行时借用dsh能力三路径决策分析_20261019.md, docs/research/02_智能体/112_多智能体协作场景专题.md, docs/research/02_智能体/08_调研预研/外部平台调研/115_主流多智能体框架对比报告大纲_20261101.md]
---

# DeepSeek Super-agent 原型调研与借鉴分析

> **状态：v1.0（2026-11-02，首版）**——开发者指令调研落档，**暂不立项**：借鉴清单仅作观察储备，本文不做任何能力主张；采纳任何一项时另轮领号（docs/18）并先过 docs/15 开源登记。前版：无。

## 0. 结论先行

1. **拍板：暂不立项。** 它是「单仓库主动式编码机器人」原型，与本项目「单机自部署个人助手」（01 §8 Q-23）定位不同轨；但其工单化编排层有 13 个可借鉴点，已分级沉淀于 §4，全部触发驱动。
2. **一句话画像**：它优化的是「人需要出现几次」而非「模型能做多少」——把人的注意力当预算（attention budget），授权/进展/提问全部结构化显式交接，明确拒绝从自由文本猜意图（其 docs/001：从报告自由文本推断「等待人工」被列为 rejected concept）。
3. **最大借鉴价值是四个我们尚无的机制**：①授权对象（Mandate，不可变授权+恢复前再验证）；②必达副作用通道（事务性 Outbox+回执+保序）；③影子档（observe：主动推断只记录 "would accept" 不开工）；④「未接住的消息不丢」的输入已读会计。

## 1. 调研对象与事实底座

| 项 | 事实（2026-11-02 核实） |
| --- | --- |
| 仓库 | github.com/deepseek-ai/dsh-superagent-prototype——9.3k star / MIT / TypeScript ~97% / 仅 1 个 commit / 2026-10-31 开源（DeepSeek Harness 团队） |
| 定位 | 面向**单个 GitHub 仓库**的主动式编码智能体，打包为 DeepSeek Harness 的 bundle；监听 GitHub（可选飞书）事件，被授权者提需求即接活，在独立会话与目录中处理，回原对话交付（开 PR） |
| 方法 | 逐篇读官方编号设计文档 docs/000~008（自称按序阅读、每篇只依赖前文）+ src/ 源码结构（core 五分区/effects/outbox/policy/sources/executor）；仓库浅克隆到本地临时目录比对（不入库） |
| 方法论 | README 设计哲学原句：「take as little human attention as possible, and give as much value as possible」——每条消息/通知/提问/设置项/面板元素都计注意力成本，价值不抵成本即移除 |

**诚实边界（引用本档必读）**：

- README 自述 research prototype、非官方产品，大量工程由 coding agents 完成；
- README 明示：开源版与内部版不完全一致，配套文章《Exploring Super-agent Design》中**部分设计属未来方向、未在本原型实现**——媒体所称「Router+Worker 双层架构」（原型内对应物=主 Session（orchestrator，14 应用工具，只路由不技术决策）+执行器 Session（worker，6 应用工具））与「CVM 认知执行环境」（仓库与拆解文均**未验证到该术语**）属文章层叙事，本档不以之为原型事实；
- 媒体转述性能数字（p95 15.7 min vs 单会话 29.5 min）非官方可复现基准（文章自述用耗时工具替代真实编码），**不引用不主张**。

## 2. 架构速览：五段流水线与五个工程支柱

流水线：**Source → Trigger/Thread → Eval（gate+assessment 两层）→ Ticket+Mandate → Assignment → 执行器 Session（每工单一目录，git worktree）→ Channel/Outbox**

领域模型 11 概念（docs/001）：Source / Trigger（不可变事件，幂等与审计单元）/ Thread / Correlation（只加上下文不路由）/ Channel（Thread 写侧）/ Policy / Eval / Mandate / Ticket / Assignment / Subscription；文档附 **12 条 Rejected concepts 表**（被否方案+理由，如「等待状态属于 Source 而非 Eval」「Policy 变更不撤销在途 Mandate 只记录并投递」「禁止从报告自由文本推断等待人工」）。

五个工程支柱（各含已验证实现位置）：

1. **单一写者+角色接口**：src/core 零 I/O 零外部依赖；每类状态恰一个写者（Engine 独写 Trigger/Thread/Decision/Ticket/Assignment，SQLite 单文件）；Engine 按调用方角色裁剪暴露面（EngineForSources / MainSession / People / Settings / Executor 五个 Pick 接口）；命令用祈使式命名（`stopTicket`）、事实回报用过去式（`assignmentStarted`），两类方法语义分离。
2. **事务性副作用队列（Effects）+ Outbox 回执**：命令在事务内 `enqueue`，**commit 后才执行**；六种 kind（assessment/launch/delivery/relay/notice/reply）各注册一个 handler；指数退避、同 kind/key 合并、按 kind 保序（确认先于结果送达同会话）、连败 3 次升健康问题；每条外发消息持回执（pending/sent/dropped+原因码），dropped 永不再排队。
3. **Policy 版本化 + Mandate 再验证**：规则+accept 模式+各 Source 策略段 hash 成 Policy 版本，每个 Mandate 记录依据版本与准入依据（via：addressed/followup/admission/bot）；变更→作废在途 Eval 并重排队、**定向通知**受影响工单，但不撤销在途 Mandate；已结束工单被消息唤醒（resume）前，先按当前 Policy 再验证授权仍成立，失败则保持结束、消息转普通评估。
4. **主动性三层安全阀**：admission 表（接收范围×发件人身份×是否点名 → 显式请求/推断候选/忽略 三格）→ accept 模式 off/**observe（默认）**/auto（observe=推断候选完整评估后只记录 "would accept" 不开工，积累决策记录后再开 auto）→ Mandate 固定范围与强度（更宽范围=新工单）。核心边界句：「收到上下文 ≠ 获得新授权」——订阅扇出可绕过评估器送上下文，但不能扩大 Mandate。
5. **重启安全的精细会计**：Source 各持读位置（重启不重放事件）；Trigger 幂等（重复 accept 只建一张工单）；输入 delivered/read 两态（被读进会话历史且产生后续动作才算已读）；工单结束时未读 follow-up 自动回流 Eval 重评；等待条件（如 CI 完成）建模为普通 Source，Eval 无状态可随时重启。

## 3. 与本项目逐层对照

| # | super-agent 机制 | 本项目对应现状 | 差距判断 |
| --- | --- | --- | --- |
| 1 | Source 插件协议（kind/instance/schema/facts/health/channel/scope 声明式） | 连接器六类（REQ-214）+kind=devops（REQ-554，`backend/internal/connector/devops.go`） | 形态同构；**缺「外部事件源」一类**——我们全部 run 由对话被动发起（docs/handbook/02_智能体/00_智能体模块.md 能力总表） |
| 2 | Eval=确定性 gate+模型 assessment 两层 | REQ-553 submit_review：verdict JSON→纯 Go 规则档评估→命中挂起，fail-closed（`backend/internal/tool/submitreview.go`） | **已同构**（我们先做且 fail-closed）；缺 observe 影子档——但 REQ-546 晋升提案 pending 暂存区（learning proposal 先裁决后入库）已是同一哲学的先例 |
| 3 | Mandate 不可变授权+恢复前再验证 | 审批三档 off/danger/all 管「工具调用时刻」（`backend/internal/tool/approval.go:182` StatefulInterrupt 挂起） | 互补不重叠：我们管每一步工具，它管「这活是谁、凭什么授权开工」；定时续跑（REQ-204）无授权凭据概念 |
| 4 | Ticket 阶段派生表（first-match-wins，纯从存储事实推导） | run phase 事件（run.started/finished）+TraceDrawer 过程视图 | 缺「**这一项在等谁、要什么**」的状态视图与互斥阶段语义 |
| 5 | Panel 三问（需要我吗/系统健康吗/来龙去脉）+健康态零占位 | 工作区感知批 REQ-576~590（事件人话化/候选徽标/⌂ 头） | **方向一致**；其「计数=列表长度、每行标注等谁要什么」可作感知类需求验收口径 |
| 6 | Effects 事务性队列+Outbox 回执 | REQ-585 事件双写（入库恒写+SSE 尽力推送，两态诚实） | 缺「**必达通道**」：审批请求/外发通知等副作用无重试/回执/保序语义 |
| 7 | 输入 delivered/read 会计+未读回流 | 委派六工具（REQ-208）；委派深度=1（112 号开放点） | 委派 instruction 是否被子 agent 真正消费**不可审计** |
| 8 | Policy/Prompt 版本化（numbered versions、会话绑定所用版本、override 记 base、默认变更报告） | 技能/提示=文件态无版本；REQ-555 回放导出**不含提示/技能版本** | 复现性缺口：回放无法严谨还原当时模型面 |
| 9 | model-surface 测试（模型所读一切 per role 尺寸预算防漂移） | doccheck 只机检文档结构 | **零成本可抄的测试思想** |
| 10 | 人工优先并发槽（maxWorkers+1 人工插槽，人工唤醒优先于新任务） | WF Runner 层内并行 ParallelLimit 4（`backend/internal/workflow/`，REQ-563）+委派队列 | **无公平性规则**：新任务可饿死人工插话恢复 |
| 11 | 后台作业治理（>5s 前台转后台、30min 硬上限杀掉+审计、handoff 时 subagent 停/shell job 留） | REQ-385 TurnLoop Push 抢占已交付；run_command 无转后台 | 长命令卡死 worker 的护栏缺失 |
| 12 | 每工单一个 git worktree 隔离 | 沙箱 work_dir；REQ-225 触发驱动未启 | REQ-225 若再启，worktree 是廉价形态候选 |
| 13 | 部件健康板（每部件一个 health entry、单写者认领、连败 3 次升 problem、成功复位） | 16 号部署档仅进程级 healthcheck | docs/handbook/02_智能体/53_Harness_设计实现与演进.md 开放问题「连接器周期探测」（原 51 号体检 W6）的现成落法 |
| 14 | 无状态 Eval（等待建模为 Source，重启安全） | WF-④ RunSnapshot/RerunFrom/ResumeInputs+迁移 087 已交付（REQ-562~569 批） | **我们恢复语义更细，此处我们领先**，无借鉴必要 |

## 4. 借鉴清单（全部暂不立项，触发驱动）

### A. 可直接落（微增强，随邻近批携带或小独立件，不需专门立项论证）

| # | 件 | 落点 | 成本 |
| --- | --- | --- | --- |
| A1 | **模型面预算机检**：内置工具描述+系统指引段+提示模板 per-role 总尺寸断言（照其 model-surface.spec 思路），防提示漂移膨胀 | backend 单测+docs/20 清单项 | ≈半天 |
| A2 | **人工优先并发槽**：WF Runner 层内并行与委派队列加一条规则——人工插话/恢复永远可越过 ParallelLimit 拿一个保留槽，新任务不得饿死人 | REQ-563 Runner v1.x 增强 | 极小 |
| A3 | **感知面板验收口径**：「健康态零占位/计数=列表长度/每行标注等谁要什么/问题必须可行动」 | docs/handbook/01_整体设计/17_产品_信息架构与界面设计.md 或 docs/20 口径行 | 极小 |

### B. 进池观察（采纳时另轮领号 docs/18+先过 docs/15 登记）

| # | 件 | 内容与落点 | 触发条件 |
| --- | --- | --- | --- |
| B1 | **外部事件源→准入门控→授权工单**（其核心骨架） | 连接器新增事件源类+admission 表+影子档；与 REQ-553 同构，可作其自然延伸；112 号开放点可加条目 | 出现真实「外部事件驱动自动化」诉求（如定时巡检/事件响应场景） |
| B2 | **Mandate 式授权对象+恢复再验证** | 记录谁、依据什么规则版本、范围与强度；恢复（resume/续跑）前按当前配置再验证 | 任何「自动接活/定时任务持续推进」类需求立项前作为前置地基（定时续跑 REQ-204 已有挂授权诉求的形态） |
| B3 | **必达副作用通道（outbox 表）** | 审批请求/外发通知/产物导出升级「必达」语义：事务内 enqueue+退避重试+回执+保序；REQ-585「尽力推送」两态诚实保留，之上叠强保证层 | 审批/通知外发丢消息的实证出现，或外发场景（如 webhooks）立项 |
| B4 | **委派输入已读会计** | delegate 工具补 delivered/read 两态+超时未读回流，委派链可审计 | 委派链审计诉求或委派深度>1 立项时 |
| B5 | **提示/技能版本绑定进回放** | REQ-555 envelope 增 prompt/skill 版本指纹字段（自声明，沿其 fidelity 先例） | 回放复现严谨化需求立项时 |
| B6 | **部件健康板** | 单写者认领+连败 N 次升 problem+成功复位——53 号开放问题「连接器周期探测」的落地形态 | 51 号体检 W6 遗留项领取时 |
| B7 | **长命令转后台+硬上限** | run_command/沙箱内 >5s 转 bg、后台 job 硬上限+审计 | 长命令卡死 run 的实证出现 |
| B8 | **worktree 隔离** | 并行多 run 触碰同一 git 项目时各一个 worktree，优于共享 work_dir | REQ-225 沙箱进阶再启时作为形态候选 |

### C. 工程文化借鉴（写档不改码）

| # | 件 | 落点 |
| --- | --- | --- |
| C1 | **「拒绝的备选」小节进方案档模板**——其 12 条 Rejected concepts 是最佳设计文档实践；我们 D-O/Q- 拍板散在迭代表，方案档固化此节（docs/15 同理可挂「使用状态」） | 方案档模板惯例（无强制） |
| C2 | **领域纯净纪律**（core 零 I/O、单一写者、祈使/过去式命名、角色裁剪接口） | docs/handbook/02_智能体/53~56 号开放问题或 docs/02 工程约定条目，不强推重构 |

## 5. 不借鉴边界

1. **无审批以 OS 用户直跑 shell**（执行器 Session 无宿主机级沙箱，其 README 自认）——我们已有审批三档+docker 沙箱三闸门（REQ-427），安全性领先，**不退**。
2. **单仓库 scope 与 GitHub/飞书硬编码进通用模块**——它是专用机器人；但若做 B1 事件源连接器，其 SourceKind 声明式协议（schema/facts/health/channel/scope）值得对照。
3. **p95 性能数字**——不可复现（见 §1 诚实边界），不进任何文档主张。
4. **CVM/Router-Worker 等文章层术语**——原型未验证，引用须标注为文章叙事。
5. **存储无限增长**（trigger/disposition/decision 永不清理）系其自认 open question，非可借鉴项。

## 6. 与邻档分工及素材源

**分工**：docs/research/02_智能体/08_调研预研/外部平台调研/41_DeepSeek-Harness_实现方案深度调研.md 拆 dsh **本体**（循环/会话/上下文/工具/沙箱=模型执行层）；docs/research/02_智能体/31_智能体对接DeepSeek-Harness_可行性及方案分析.md 与 docs/research/02_智能体/57_运行时借用dsh能力三路径决策分析_20261019.md 拆对接路径；docs/research/02_智能体/36_智能体开发架构深度对比_本地实现与DeepSeekHarness.md 拆十层架构——四档对象都是 **Harness 本体**；本档对象是 **Harness 之上的工单编排层原型**，不重叠。素材可作 docs/research/02_智能体/08_调研预研/外部平台调研/115_主流多智能体框架对比报告大纲_20261101.md 持久化与恢复/人机协同维度的补充（全文成文另轮）。

**素材源（2026-11-02 核实）**：

- 一手：github.com/deepseek-ai/dsh-superagent-prototype（README+docs/000~008+src/ 目录结构；浅克隆本地临时目录比对，未改未入库）；
- 二手（文章层叙事仅注记，不作原型事实）：掘金拆解文 juejin.cn/post/7703105289100296201、知乎 zhuanlan.zhihu.com/p/2099996800551855440、IT之家 ithome.com/1/020/615.htm。

**本项目侧事实源**：docs/handbook/02_智能体/00_智能体模块.md（能力总表）+53~56 号五层域档+docs/01 §9 需求池；对照锚点 file:line 见 §3 表。
