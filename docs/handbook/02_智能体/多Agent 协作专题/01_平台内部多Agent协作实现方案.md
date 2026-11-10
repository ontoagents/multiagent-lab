---
module: 智能体
topic: 平台内部多 Agent 协作实现方案（多Agent 协作专题）
desc: 项目容器内协调者-成员协作的实现梳理——双轨协作（agent_as_tool/transfer）、委派韧性与异步委派、workflow 编排与 DAG 引擎、上下文隔离与事件流；附代码地图与开放问题补充区
synced: 2026-11-10
---

# 平台内部多 Agent 协作实现方案

> [专题导读](README.md)第一档：梳理当前项目内**平台内部**多 Agent 协作的实现现状，并留后期补充区。外部 Agent 协作见 [02 档](02_外部Agent协作实现方案.md)；协议背景（A2A/MCP）见 [协议研究档](A2A_MCP协议研究.md)。

## 一、定位与边界

- **范围**：单机进程内、同一平台下的多 Agent 协作——项目（project）容器中的协调者-成员协作、编排与委派。
- **技术基线**：Go + Eino ADK（`github.com/cloudwego/eino v0.9.22` 内含 `eino/adk` 包，无独立 adk 模块）+ SQLite（modernc 纯 Go）。
- **框架原语优先**：内部协作用 Eino ADK 框架原语，不引入 A2A——对照协议研究 §5.5 口诀「一个 Agent 内部的子 Agent 编排 → 用框架原语，不必强上 A2A」。A2A/MCP 属跨进程/跨组织协作，归 [02 档](02_外部Agent协作实现方案.md)。
- **场景层**：协作场景形态与谱系研究在 research 区 112 号，本档只落实现。

## 二、协作模型总览

两轴配置决定协作拓扑（`backend/internal/store/models.go`）：

| 配置 | 取值 | 语义 |
| --- | --- | --- |
| `Project.CollabMode` | single / agent_as_tool / transfer | 协作轨道：单成员 / 成员即工具（推荐）/ 控制转移 |
| `Project.WorkflowMode` | free / sequential / parallel / loop / plan_execute / dag | 编排形态 |

- **Agent 实体**（`store/models.go:6-76`，表 `agent`=migrations/001）：名称/描述/系统提示词、模型连接与采样参数、`MaxIteration`、工具/技能/MCP 勾选、审批三件套（`ToolApproval/ApprovalExempt/ApprovalTimeoutHours`）、沙箱、hooks、委派超时/异步（`DelegateTimeoutSeconds/DelegateAsync`）、上下文模式（`ContextMode/CompactPolicy`）、备援模型（`ModelFailoverConnIDs`）等。单 Agent 可直聊，也可被项目引用为成员。
- **项目容器**：表 `project_agent`（migrations/001，role=coordinator|member）；协调者缺省取第一个成员（`chat/assembler.go:326-329`）。
- **装配**：每次运行现装配，不常驻——`chat.Assembler.Assemble`（`backend/internal/chat/assembler.go:241`）按会话 scope 分发；项目会话走 `assembleProject`（:316），每个成员经 `buildOne`（:1084）全量装配为 `adk.NewChatModelAgent`（模型经 `ModelConnection` 解析，工具按十步管线合并，见 §六）。
- **路由语义**：「描述即路由」——成员的 description 是协调者判断"何时委派给谁"的依据；外部成员另有能力画像注入（[02 档 §六](02_外部Agent协作实现方案.md)）。

## 三、协作轨道

### 3.1 agent_as_tool（推荐路径）

`assembleAgentAsTool`（`assembler.go:448`）：每个成员全量装配后经 `adk.NewAgentTool`（:483、:486）包装为协调者的一个工具，**function name=成员名**。协调者以普通工具调用发起委派，成员在独立子上下文中执行，结果以工具结果文本回传。

- 优点：协调者模型无需专门训练即可路由（工具语义即成员语义）；审批/审计/hook 包装与普通工具同管线覆盖。
- 委派韧性包装见 §四。

### 3.2 transfer（对照路径）

`assembleTransfer`（`assembler.go:1021`）：用 `adk.SetSubAgents`（:1067）挂子智能体，控制权在 Agent 间转移。与 agent_as_tool 并存供对照实验。

### 3.3 workflow 编排（ADK 原语）

`assembleWorkflow`（`assembler.go:868`）：WorkflowMode 取 sequential / parallel / loop 时用 `adk.NewSequentialAgent / NewParallelAgent / NewLoopAgent`（:917-921，loop 上限 3 轮）组合成员——**成员即节点、无 LLM 路由**的确定性编排。

### 3.4 plan_execute

`assemblePlanExecute`（`assembler.go:947`）：Eino ADK prebuilt `planexecute.NewPlanner / NewReplanner / NewExecutor / New`（:956-985）——先规划再执行、可重规划。

### 3.5 DAG 引擎（自研，库层交付、装配链待接）

`backend/internal/workflow/`（动态工作流编排批 REQ-562~569 八件＋REQ-552 计划层）：

- **Spec/校验**：类型化四类节点（llm / tool / gate / delegate，`spec.go:14-19`）＋条件边（`var:name==value`，:77）＋join（all|any，:28）＋ `Validate` 四项逐类人话校验（环 DFS/悬空边/join 缺条件/变量类型不匹配）＋VarTable 双限变量表；
- **Runner**（`runner.go:85`）：深度分层顺序＋层内并行（`DefaultParallelLimit=4`）＋条件边求值（:268）＋失败重试 1 次＋`skip_downstream` 级联跳过（:367）＋一等节点事件 `wf.node.*` 四族；
- **计划合成**：`Synthesize`（`compose.go:64`，LLM 结构化输出＋指纹缓存）→ `PlanAndExecute`（`plan.go:14`，REQ-552/M377）三步链（规划者→校验→执行），发 `dag.plan.*` 事件；
- **门禁**：`EvaluateGate`（`gate.go:19`）四断言类型＋回喂循环＋人审卡点（RequireApproval）；
- **快照重跑**：`RunSnapshot.RerunFrom`（`snapshot.go:88`，目标节点＋下游 BFS 作废/上游保留）＋冷恢复。
- **诚实边界**：`internal/workflow` 目前**无外部 importer**；项目 `workflow_mode=dag` 在 `assembleProject` 诚实降级为 free 并告警（`assembler.go:364-367`）——chat 装配执行链属后续集成轮（56 号档 §三.7 同口径）。

## 四、委派韧性与异步委派

**同步委派韧性包装** `delegateTool`（`chat/delegatetool.go:33-103`）：

1. 成员失败**降级回喂**：错误文本回传协调者上下文，不中断整轮（中断类信号 `adk.InterruptSignal` / `compose.ExtractInterruptInfo` 穿透，:91-103）；
2. **per-member 超时**：`DelegateTimeoutSeconds` 按成员控制；
3. **`subagent.delegate` 事件**：委派过程入事件流。

**异步/后台委派**（`chat/delegate_bg.go` + `chat/delegate_tools.go`）：

- `asyncDelegateTool`（delegatetool.go:119）＋`DelegateManager`（delegate_bg.go:36-97）——委派任务在**子会话**（scope=agent）后台运行，不阻塞协调者；
- 六件套工具 `delegate_background / status / steer / collect / interrupt / continue`（delegate_tools.go:5-10、55-246）：提交后可查进度、中途插话、收割结果、中断、续跑；
- 持久化：`conversation_delegate` 表（migrations/056，`store.Delegate`）——重启后可对账续管。

## 五、上下文、通信与可观测

- **上下文隔离模型**：成员各自独立 harness/context，**无共享黑板/消息队列**——成员间通信=委派工具的 `{"request": "..."}` 入参字符串与工具结果文本；ADK AgentTool 只回传末条文本。协调者仅见 `subagent.enter/exit/delegate` 等事件。
- **会话与轮次**：`Conversation`（scope=agent|project，models.go:218）承载运行态（InterruptState/PauseState/ContextState 等）；轮次循环=自研 `runTurnLoop`（`chat/turnloop.go:109`，包 `adk.NewTurnLoop`）支持轮界 steer/pause/续跑；checkpoint 双实现（内存/SQLite，表 migrations/028）支持冷恢复。
- **上下文预算**：`chat/context.go` 三档 `EffectiveContextMode`（:48）＋`mergeToolTurns` 按时间归并重建历史（:110）＋压缩落 `conversation.context_state`＋工具结果剪枝。
- **事件流**：`chat/event_names.go` 注册约 40 类事件（run.* / message.delta / tool.call·result / subagent.* / wf.node.* 等），SSE 即时下发（`api/sse/sse.go`）＋`run_event` 表落库（SchemaVersion=2）——回放/轨迹/泳道的过程单源。
- **协作过程账本**（REQ-557）：run.finished `usage_by_agent`＋事件级 agent 字段＋`runmetrics` 聚合（按成员分组）。
- **分歧裁决**（REQ-553）：`submit_review` 工具＋StatefulInterrupt 挂起＋人工 resolve 两态回注——结构化协作的人机卡点。
- **跨运行上下文重建**：`recallChain`（`chat/runner.go:1999`）——KB 检索/记忆召回/本体命中三源准入。

## 六、工具体系（成员的"手脚"）

- **注册表**：`tool.Registry`（`backend/internal/tool/registry.go:37-114`），来源四类 `SourceKind`：builtin / skill / mcp / ontology。
- **装配管线**（`assembleTools`，assembler.go:1192-1212 十步）：builtin → onchat → 技能白名单 → MCP → 本体 facade → 文件工具 → 分歧识别 → platform_cli → hook 包装 → 审批包装（`approval.go IsDangerousTool`）。
- **内置/常备工具**：current_time、ask_human、ontochat_* 四件（builtin）；http_fetch（常备，SSRF 黑名单＋截断）、todo_write、save_file/list_files/read_file（项目会话）、grep/glob/write_file（work_dir 安全根）、run_command（沙箱/目录/审批三前置）、load_skill、submit_review、platform_cli（常备/条件装配）；平台助手 L0 组另有 doc_read/doc_grep 等管理面工具。
- **本体工具**：`ontology.FetchTools` 以 MCP 客户端拉 runtime-manager facade（6 查询件＋onto_ops＋runtime 件）——本体知识经 MCP 形态供给内部成员。

## 七、代码地图

| 机制 | 落点 |
| --- | --- |
| Agent 实体/CRUD | `backend/internal/store/models.go:6-76`、`store/agents.go`、migrations/001 |
| 项目容器/成员归属 | `project_agent` 表（migrations/001:30-35）、`store.ProjectMember` |
| 装配入口/项目装配 | `backend/internal/chat/assembler.go:241`（Assemble）、:316（assembleProject） |
| agent_as_tool / transfer | assembler.go:448 / :1021 |
| 委派韧性/异步委派 | `backend/internal/chat/delegatetool.go`、`delegate_bg.go`、`delegate_tools.go` |
| 轮次循环/checkpoint | `backend/internal/chat/turnloop.go:109`、`checkpoint.go` |
| 上下文预算/压缩 | `backend/internal/chat/context.go:48/:110` |
| ADK workflow 原语 | assembler.go:868-921（sequential/parallel/loop）、:947（plan_execute） |
| DAG 引擎 | `backend/internal/workflow/`（spec/runner/compose/plan/gate/snapshot） |
| 事件流 | `backend/internal/chat/event_names.go`、`backend/internal/api/sse/sse.go`、`run_event` 表 |
| 工具注册/装配管线 | `backend/internal/tool/registry.go:37-114`、assembler.go:1192-1212 |

## 八、开放问题与研究方向（后期在此补充）

> 初始梳理（2026-11-06）登记如下，后续研究结论直接完善本节。

1. **DAG 装配链接入**：`internal/workflow` 库层已交付但 chat 装配链未接（dag→free 降级）——`Runner` 事件驱动装配是既定下一步（113 号路线 A）。
2. **成员间上下文共享**：当前严格隔离、只回传末条文本；多轮协作的中间产物共享（黑板/结构化 Artifact 传递）是否需要、以何种形态引入，待场景驱动研究（关联 112 号开放点）。
3. **委派深度与扇出**：成员再委派成员（嵌套委派）的语义与护栏现状如何，待核实补记。
4. **编排模式选型指引**：CollabMode×WorkflowMode 六轨道组合的适用场景与默认值策略，可提炼为创建向导内的选型指南。
5. **与外部协作的统一视角**：外部 A2A/CLI 成员与本节内部成员共用 agent_as_tool 轨道（见 [02 档 §四.3](02_外部Agent协作实现方案.md)）——「内部框架原语 vs 外部协议协作」的边界随演进复核。
6. **项目会话 @ 成员定向路由＋分散模式**（2026-11-09 研究输入；**2026-11-10 已立项交付=REQ-623-4371/M433-4371 ✅，实现与本节研究结论的差异**：多 @ 按开发者拍板「全部处理」而非 MVP 单 @——按提及序 ADK SequentialAgent 顺序执行〔后者可见前者回答，群聊语义〕；外部成员点名范围=协调者 a2a/cli 连接器实例名〔与 workflow 轨外部节点同口径〕；对比模式互斥双 400；解析在 API 层 mention.go＋ctx 注入〔REQ-527 detached runCtx 由 handler 挂载〕，TurnLoop steer/resume 同链自动携带，redo 复用 runConversation 自动覆盖；详录 01 §3.32/迭代表 v3.31、smoke/req622/、02 档 §八.6 立项注）：原研究要点存档——**①@ 定向路由（机制层）**：@成员 → 消息直接触发该成员运行（直投执行，跳过协调者路由；路由提示注入协调者为备选语义）；**②分散模式（模式层）**：`CollabMode` 新增 `mention` 值——项目不设协调者执行循环，@ 为唯一触发（无 @ 消息后端拒绝并提示「请 @ 指定成员」），解决「成员中无一适合做协调者」的场景。关键差异：mention 模式下成员**可见会话历史**（群聊语义，受 ContextMode 预算），区别于 agent_as_tool 子代理上下文隔离。实现要点：路由决策必须在**运行层**（runs 入口按消息解析 @ → 单成员 `buildOne` 装配直跑）而非装配层（Assemble 不见单条消息）；`workflow_mode` 强制 free（编排模式语义依赖协调者）；协调者字段在该模式下隐藏；前端 @ 菜单产结构化 mention_agent_id 优先、文本 @ 前缀解析兜底；成员身份归属复用 REQ-557 事件 agent 字段；本地/外部（A2A/CLI）成员同轨——@ 外部成员即 `a2aMemberInvoke`/`newCLIMemberTool`。落点：store/models.go:146 白名单+1、assembleProject 增 mention 分支或 runs 层前置路由、ProjectModal 协作模式下拉+1、Sender 成员菜单。
7. **内部成员间 A2A 互调**（2026-11-09 核实）：设计口径不走 A2A（框架原语承担，见 §一）；但平台每个 Agent 天生有 A2A server 端点（[02 档 §四.1](02_外部Agent协作实现方案.md)），且 **A2A 连接器无自环防护**（MCP 有 `isSelfMCPEndpoint`，assembler.go:1133/1285；A2A 侧未发现对等拦截）——回环连接器指向本平台 `/api/a2a/agents/{id}` 技术上可通，属「可行但不应为常规路径」（多一跳 HTTP＋task 落库开销，过程呈现在 external.audit 而非 subagent 事件）；自环防护缺失作为安全对齐项登记（见 02 档 §八）。
8. **会话容器语义澄清**（2026-11-09）：项目模块「新对话」已创建 `scope=project` 团队会话（ProjectsPage.tsx:138 → `assembleProject` 装配协调者＋全体成员，缺成员直接报错 assembler.go:322-324）——「一个会话多 agent 协作」在项目容器**已成立**；单 agent 会话（scope=agent）是智能体/工作区模块动线（AgentsPage.tsx:113/157、WorkspacePage.tsx:164）。跨容器「群聊」（智能体模块多 agent 直聊）与项目容器定位重叠，不建议重复建设；改进方向=项目会话 UI 的团队构成可见性（协调者/成员标识）。
