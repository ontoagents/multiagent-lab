---
module: 智能体
topic: Graph 域事实源（设计总纲·实现现状·演进跟踪）
desc: 协作层技术沉淀——成员装配（agent_as_tool/transfer）/subagent 事件/工作流编排（REQ-205 ✅）/后台委派（REQ-208 ✅）；演进=REQ-385 LongRun 子调用异步化/REQ-389 D2 图状态远期实验；姊妹档 53 号 Harness/54 号 Context/55 号 Loop
req: [REQ-33, REQ-205, REQ-208, REQ-385, REQ-389]
docs: ["02 §6 五层总纲", "02 §6.4 多 Agent 装配", "38 号 D 阶段", "39 号 E2", "44 号 Eino 分析"]
synced: 2026-10-24
---

# Graph：设计、实现与演进（2026-10-01 沉淀）

> **定位**：Graph 层（多智能体协作编排）的域事实源，与 [53 号 Harness](53_Harness_设计实现与演进.md)、[54 号 Context](54_Context_设计实现与演进.md)、[55 号 Loop](55_Loop_设计实现与演进.md) 并列。技术总纲以 `docs/02 §6 五层总纲`+§6.4 为准，需求以 docs/01 REQ-205/REQ-33 行为准。
> **层边界**：Graph 管「多个智能体怎么组队与流转」——成员装配语义、委派机制、工作流编排。**不归**：单 agent 内的审批/hooks（→53 号）、迭代与恢复（→55 号）。
> **文档地图**：38 号 D 阶段（Graph 兑现路线，未启动）、39 号 E2（编排收口，并入 REQ-208 扩注）、44 号 Eino 深度分析（agent_as_tool/transfer 机制对照，44 号定案 transfer=NOT RECOMMENDED）、docs/02 §6.4。

## 一、设计总纲与代码地图

| 组件 | 代码 | 职责 |
| --- | --- | --- |
| agent_as_tool（推荐路径） | assembler.go（adk.NewAgentTool 包装，function name=成员名） | 项目成员全量装配后包装为工具，协调者按描述路由调用；ADK 只回传末条文本（REQ-203 代码实测定案，子 Agent 结果压缩无需开发） |
| transfer_to_agent（对照路径） | assembler.go（adk.SetSubAgents） | ADK 内建转移机制；44 号 NOT RECOMMENDED 路线，**治理定案并入 REQ-208 扩注**（迁移 agent_as_tool 或保留对照+UI 风险标注） |
| subagent 事件 | runner.go trackAgent（EmitInternalEvents 开启） | subagent.enter/exit 按 AgentName 变化推导；轨迹面板嵌套缩进呈现（REQ-217/M48） |
| 项目编排配置 | 项目侧板 CollabView「智能体协作」（collab_mode/workflow_mode/成员+协调者） | 协作模式与工作流模式的产品配置面（agent 侧板 Graph 入口已随 REQ-251/M73 退役——原占位指引，委派机制说明并入 CollabView；REQ-205 落地再入） |
| workflow 引擎 | assembler.go assembleWorkflow（adk.NewSequential/Parallel/LoopAgent） | **REQ-205/M40 ✅ 已交付（2026-10-23）**：三值落地〔loop 上限 3〕+workflow.node 节点事件+HITL 节点标注+只读回放条带；官方 NOT RECOMMENDED 口径如实注记（agent_as_tool 仍为推荐默认） |

## 二、实现现状

| 能力 | REQ | 里程碑 | 状态 | 验证 |
| --- | --- | --- | --- | --- |
| 多成员装配（agent_as_tool+transfer 双轨） | REQ-33/124 | M5 起 | ✅（装配能力） | 多智能体项目会话 |
| subagent 嵌套事件与轨迹呈现 | REQ-117/217 | M17/M48 | ✅ | headless |
| Graph 页签占位指引（agent 级诚实空态） | REQ-219 | M50 | ✅（入口已随 REQ-251/M73 退役，编排说明并入项目协作视图） | headless 16/16 |
| 委派感知最小面（时序条第五智能体泳道栈式切段+ToolLine 委派徽标+执行过程块按成员聚合统计；P2 拓扑图挂需求池 B1） | REQ-311 A5 | M115 | ✅ | UI 五泳道断言 smoke/req311/（委派聚合需项目会话真实委派场景） |
| 成员委派韧性（错误降级回喂〔中断类穿透〕/per-member 委派超时迁移 049/subagent.delegate 委派单事件——78 号 G1/G2/G7 收口；P2 上下文注入/并行修正/深度>1 挂需求池） | REQ-319 | M121 | ✅ | 真机 E2E 协调者改道自愈 8/8+UI 6/6 smoke/req319/ |
| 工作流编排兑现（三值/图状态/HITL/回放） | REQ-205=REQ-33 兑现 | M40 | ✅（2026-10-23 交付：ADK 工作流原语装配+节点级事件+进度条带；01 v1.79） | 装配单测 2 组+headless 10/10 smoke/req205/ |
| transfer 治理 + continuation 子智能体 | REQ-208 | 39 号 E2 并入 | ✅（2026-10-23 交付：transfer 保留对照+侧板黄色风险标注〔定案〕+后台委派六工具+DelegateManager+steer 轮界接续+冷恢复；01 v1.78） | chat 单测 5 组+真机边界待凭证 |

## 三、开放问题与诚实边界

1. ~~workflow_mode 空壳~~——**REQ-205/M40 已交付（2026-10-23）**：三模式装配+节点级事件+进度条带落位（本条原为滞后残留，2026-10-24 顺修）。
2. **transfer 双轨并存→已定案（2026-10-23 REQ-208 批）**：保留对照实现+项目侧板黄色风险标注（官方 NOT RECOMMENDED），不迁移不删除；新场景引导走 agent_as_tool（17 号 LG-6）。
3. **agent_as_tool 同步阻塞**——子智能体调用占协调者一轮（39 号 P-3）；**已立项 REQ-385/M181（C3 LongRun：TurnLoop+delegate 底盘汇流）待领取**（01 §3.10）。
4. **agent 级 Graph 配置为空**——成员/协作/工作流均在项目侧配置；agent 侧板 Graph 入口曾为占位指引（REQ-219 定案），已随 REQ-251/M73 退役（零配置项纯说明不入 activity bar），M40 落地时再评估是否立配置入口。
5. **子智能体上下文隔离**——成员独立装配（各自 harness/context），协调者不可见子内部过程（仅 enter/exit 事件）——防火墙语义是特性非缺陷，调试依赖成员侧轨迹。
6. **D2 图状态（远期实验）**——REQ-389/M185 已立项（2026-10-24）：todo.md 进度产物/项目协作状态以伴生图承载、SPARQL 查运行状态（38 号差异化战略）；前置 REQ-377 伴生图持久化已交付就绪；可选实验分支双轨起步（01 §3.10）。

> 体检来源：39 号 P-3/A-3「编排语义弱、三岔无路标」即本档开放问题（39 号已于 2026-10-01 清理，结论归档于 00 号）。

## 四、维护约定

Graph 层为五层中最薄一层：REQ-205/M40 启动即本档正式开篇（工作流引擎选型/图状态/HITL 设计输入在此沉淀）；里程碑权威 docs/02 §12。
