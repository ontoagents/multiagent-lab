---
module: 智能体
topic: Loop 域事实源（设计总纲·实现现状·演进跟踪）
desc: 循环层技术沉淀——ReAct 迭代上限/挂起恢复 checkpoint/定时续跑/进度产物/usage；代码地图、REQ/M39·M52 映射、C3 LongRun/C4 Evaluator 后置；姊妹档 53 号 Harness/54 号 Context/56 号 Graph
req: [REQ-204, REQ-49, REQ-210]
docs: ["02 §6 五层总纲", "38 号 C 阶段", "51 号体检"]
synced: 2026-10-29
---

# Loop：设计、实现与演进（2026-10-01 沉淀）

> **定位**：Loop 层（ReAct 迭代治理/挂起恢复/长任务推进/成本护栏）的域事实源，与 [53 号 Harness](53_Harness_设计实现与演进.md)、[54 号 Context](54_Context_设计实现与演进.md)、[56 号 Graph](56_Graph_设计实现与演进.md) 并列。技术总纲以 `docs/02 §6 五层总纲` 为准，需求以 docs/01 REQ-204 行为准。
> **层边界**：Loop 管「跑多久、断了怎么办、花了多少」——迭代上限、checkpoint 挂起恢复、定时续跑、进度产物、usage 统计。**不归**：审批挂起的 gating 策略（→53 号 Harness，挂起机制本身是 Loop 层 checkpoint 承载的）；编排（→56 号 Graph）。
> **文档地图**：38 号 C 阶段（Loop 最小闭环路线）、51 号体检（P-4 长任务可控断层残留）、docs/20 S2.16 上下文行相邻、docs/02 §12 M39/M52。

## 一、设计总纲与代码地图

| 组件 | 代码 | 职责 |
| --- | --- | --- |
| 迭代上限 | runner.go→adk ChatModelAgentConfig.MaxIterations | agent.max_iteration（normalizeMaxIter 默认 25，create 兜底）；侧板 Loop 页签配置 |
| checkpoint 持久化（C1） | internal/chat/checkpoint.go（NewStoreCheckPointStore，迁移 028） | 挂起中断快照落 SQLite，后端重启仍可恢复；多级审批沿用 cpAnchors 锚点（REQ-214 顺修：Resume 后再挂起沿用锚点 id 覆盖保存） |
| 挂起恢复 | runner.go handleInterrupted/Resume + conversation.interrupt_state | run.interrupted（kind=approval/ask_human）→resume 定向投递；新消息放弃挂起（abandon）；挂起超时自动 deny（REQ-231③，判定语义归 53 号审批面） |
| 定时续跑（C5） | api/scheduler.go + conversation_schedule 表（迁移 038） | interval≥1min/max_runs≤50 硬校验、cap 即摘、活跃会话跳过；**调度状态出进程入 DB（REQ-224/M52 收口——重启失效诚实边界闭合）** |
| 进度产物（C2） | tool/todo.go + handlers_export.go todo.md 导出 | todo_write 落 conversation.todo_json + GET /api/conversations/{id}/todo.md |
| usage 统计（C6） | runner.go run.finished + api/handlers_stats.go UsageStats | run 级 prompt/completion/total tokens；GET /api/stats/usage 按 model/agent/project/conversation 聚合（REQ-49 扩展出池兑现） |
| 侧板 Loop 页签 | web AgentSidePanel | max_iteration + 挂起恢复/续跑说明（REQ-219/M50） |

## 二、实现现状

| 能力 | REQ | 里程碑 | 状态 | 验证 |
| --- | --- | --- | --- | --- |
| C1 持久化 checkpoint/C2 todo.md/C5 定时续跑/C6 usage | REQ-204 | M39 | ✅（核心四项） | 真机 9/9 |
| C5 调度持久化（重启失效收口） | REQ-224 | M52 | ✅ | 迁移 038+DB 往返 |
| 挂起超时自动 deny | REQ-231③ | M58 | ✅ | 真机 danger 链（超时判定=恢复重入时） |
| Loop 页签（五层视角） | REQ-219 | M50 | ✅ | headless 16/16 |
| 迭代进度与生效值透出（run.started/resume max_iteration_effective+头部迭代 k/N chip+TraceDrawer 迭代≤N） | REQ-311 A4 | M115 | ✅ | API 生效上限=7 断言+UI chip smoke/req311/ |
| C3 LongRun 有界增量 | REQ-204→REQ-385 | 后置→📋 立项待领取 | 2026-10-24 评审立项批（M181）：TurnLoop 原语+delegate 底盘汇流 | — |
| C4 Evaluator 评估器 | REQ-204→REQ-388 | 后置→📋 触发驱动 | 2026-10-24 立项（M184）：触发=C3 落地后质量归因诉求；复用 REQ-223 Judge 契约 | — |

## 三、开放问题与诚实边界

1. **C3/C4 后置→已立项**——2026-10-24 智能体模块评审立项批（01 §3.10）：C3=REQ-385/M181 待领取、C4=REQ-388/M184 触发驱动（复用 REQ-223 agenteval Judge 契约不开新面，口径不变）。
2. **per-tool 成本无归因**——usage 只到 run 级（REQ-204 C6），工具级 token/成本归因列观察项（**已立项 REQ-397/M193：per-tool 成本归因——工具维度聚合+UI 透出，2026-10-24 漏项补立**；51 号 W6 同批评估维持）。
3. **挂起触达缺失**——审批/追问挂起无提醒通道（**已立项 REQ-395/M191：桌面客户端（REQ-321/324）推送打通，2026-10-24 漏项补立**）；超时自动 deny 已有，主动触达随 REQ-395 落地。
4. **abandon 语义单一**——仅新消息触发放弃挂起；显式「放弃本次挂起」按钮列观察项。
5. **REQ-385 TurnLoop 原语评估（2026-10-24，M181 评估件落痕）**——eino v0.9.19 `adk/turn_loop.go` 实测在位：`NewTurnLoop[T,M](TurnLoopConfig)` + **`Push(item, opts)` 运行中 push-based 输入原语**（preemptController 抢占式 turn 目标管理+Push 临界区）+checkpoint 集成（saveTurnLoopCheckpoint）+GenResume 恢复钩子。**评估结论**：①Push 原语可解「Eino 无 mid-run 输入通道」诚实边界（REQ-208 steer 轮界接续的深化路径=TurnLoop.Push 运行中介入）——**采用价值成立**；②采用是大改造面：Runner 装配链（chat.Assemble 产出 Runner→TurnLoop 形态）、事件翻译管线（runConsumer 消费 AgentEvent 不变）、checkpoint/checkPointIDOf 语义衔接、delegate 底盘汇流（REQ-208 delegate_background 已承载子调用异步化——汇流=TurnLoop 与 delegate 底盘的编排层归一）四线并行；③**拍板=评估通过、采用实施留专项重构轮**（与 REQ-390② assembleTools 函数化/③Service 分域拆分同窗口——运行时结构改造集中一轮，避免碎片化重构）；REQ-387 已交付的 A2A tasks 落库/凭证批（REQ-394）为长任务观测面提供了前置。
   **✅ 采用已落地（2026-10-29，REQ-385/M181 专项重构轮，01 v2.34）**：四线全落——①chat/turnloop.go 引擎接入 runOnce/Resume 主链（默认开，CHAT_TURNLOOP=0 回滚；工作流 Runner 不叠加）②轮界产物即刷即持久化（长任务有界增量）③主会话 steer=Service.Steer→Push(WithPreempt(AnySafePoint))+三事件可观测（steer.pushed/run.preempted/steer.applied）④delegate 汇流=agent delegate_async 开关（迁移 072）转 DelegateManager 底盘。本开放问题闭环。

> 体检来源：39 号 P-4「长任务可控断层」驱动 REQ-204/M39（39 号已于 2026-10-01 清理，结论归档于 00 号）。

## 四、维护约定

新增 Loop 层演进（LongRun/Evaluator/成本归因/挂起触达）默认落本档续行；里程碑权威 docs/02 §12。
