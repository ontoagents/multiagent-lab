---
AIGC:
    Label: "1"
    ContentProducer: 001191110102MACQD9K64018705
    ProduceID: 3559570444136323_0-drive/220702462725908132/A2A_MCP协议研究_20261106/A2A_MCP协议研究.md
    ReservedCode1: ""
    ContentPropagator: 001191110102MACQD9K64028705
    PropagateID: 3559570444136323#1793944318920
    ReservedCode2: ""
---
# A2A 与 MCP 协议研究

> 整理日期：2026-11-06 · 信息核验至当日，版本数据均来自官方规范/官方博客/Linux Foundation 公告
>
> 一句话定位：**MCP 解决"Agent ↔ 工具/数据"的纵向连接，A2A 解决"Agent ↔ Agent"的横向协作，二者互补而非竞争。**

---

## 0. 速览表

| 维度 | MCP (Model Context Protocol) | A2A (Agent2Agent Protocol) |
|---|---|---|
| 解决的问题 | Agent 如何标准化地访问工具、数据、API | Agent 如何发现彼此、委派任务、协作 |
| 集成方向 | 纵向（agent-to-environment） | 横向（agent-to-agent，可跨组织） |
| 发起方 | Anthropic（2024-11） | Google（2025-04，50+ 伙伴同发） |
| 当前治理 | Agentic AI Foundation (AAIF) | Agentic AI Foundation (AAIF)，Growth Stage |
| 当前规范版本 | **2026-07-28**（上一版 2025-11-25） | **v1.0.1**（2026-05-28；v1.0.0 为 2026-03-12） |
| 线路协议基础 | JSON-RPC 2.0 | JSON-RPC 2.0 / gRPC / HTTP+JSON 三种绑定 |
| 传输 | stdio（本地）、Streamable HTTP（远程） | HTTP(S) + SSE 流 + Webhook 推送 |
| 核心抽象 | Host / Client / Server；Tools、Resources、Prompts | Client Agent / Remote Agent；Agent Card、Task、Message、Part、Artifact |
| 对端可见性 | Server 能力显式声明，Host 控制上下文 | 远程 Agent 是**不透明黑盒**（不暴露记忆/工具/逻辑） |
| 状态模型 | 无状态（2026-07-28 起），跨调用状态用显式句柄 | 显式 Task 生命周期（8 种状态） |
| SDK | TypeScript / Python / Go / C#（Tier 1） | Python / JS / Java / Go / .NET（生产级）+ Rust |
| 生态规模 | Tier 1 SDK 月下载近 5 亿（2026-07 官方博客） | 150+ 组织支持，22k+ GitHub stars（2026-04 LF 公告） |

---

## 1. 背景：为什么需要 Agent 协议

LLM 应用进入 Agent 时代后出现两个集成瓶颈：

1. **工具接入的 M×N 问题**：M 个 Agent 应用要接 N 个工具/数据源，两两定制集成成本爆炸。MCP 把它变成 M+N：工具方实现一次 MCP Server，所有 Agent 宿主都能用。
2. **Agent 协作的孤岛问题**：不同团队、不同框架（LangGraph、CrewAI、Semantic Kernel、自研）构建的 Agent 无法互操作，跨厂商协作只能靠脆弱的私有 glue code。A2A 给出统一的应用层协议：发现、协商、任务委派、结果回传。

两者共同构成多 Agent 系统的互操作基座。2026 年 8 月起，两个协议同属 Linux Foundation 旗下的 **Agentic AI Foundation（AAIF）**，治理上已完全中立化。

---

## 2. MCP 深入

### 2.1 定位与设计哲学

MCP 是一个开放协议，标准化 **LLM 应用（Agent）与外部工具、数据源之间的连接方式**。官方架构文档明确的四条设计原则：

1. **Server 应极易构建**——Host 承担复杂编排，Server 只做聚焦能力；
2. **Server 高度可组合**——多个 Server 无缝拼装；
3. **Server 不能读整个对话，也看不到其他 Server**——上下文聚合权在 Host，Server 间相互隔离（安全边界的核心）；
4. **能力可渐进添加**——核心协议最小化，扩展能力协商解锁。

### 2.2 架构：Host / Client / Server

```
┌─────────────────────────────────────────────┐
│  应用宿主进程 (Host)                          │
│  e.g. Claude Desktop / IDE / 你自己的 Agent   │
│                                             │
│  ┌────────┐   ┌────────┐   ┌────────┐       │
│  │Client 1│   │Client 2│   │Client 3│       │
│  └───┬────┘   └───┬────┘   └───┬────┘       │
└──────┼────────────┼────────────┼────────────┘
       │ 1:1        │ 1:1        │ 1:1
┌──────▼────┐ ┌─────▼─────┐ ┌────▼──────┐
│ Server 1  │ │ Server 2  │ │ Server 3  │
│ 文件&Git  │ │  数据库    │ │ 外部API   │
└───────────┘ └───────────┘ └───────────┘
```

- **Host**：容器与协调者。创建/管理 Client 实例、控制连接权限与生命周期、执行安全策略与用户同意、聚合各 Client 的上下文、对接 LLM。
- **Client**：由 Host 创建，与**恰好一个** Server 1:1 通信。每个请求附带协议版本与能力声明；维护订阅与通知；在 Server 之间维持安全边界。
- **Server**：通过 MCP 原语暴露 resources/tools/prompts；可以是本地进程（stdio）或远程服务（HTTP）。需要用户输入时通过回复中的 `InputRequiredResult` 发起。

### 2.3 核心原语

| 原语 | 控制方 | 说明 |
|---|---|---|
| **Tools** | 模型控制 | LLM 决定何时调用；`tools/list` 发现、`tools/call` 执行；2026-07-28 起 input/output schema 支持完整 JSON Schema 2020-12（oneOf/anyOf/$ref） |
| **Resources** | 应用控制 | 供应用/context 使用的只读数据（文件、记录等），支持 `subscriptions/listen` 订阅变更 |
| **Prompts** | 用户控制 | 服务端定义的提示词模板，由用户显式选择触发 |

配套原语的演进（重要）：

- **Sampling / Roots / Logging**：已在 2026-07-28 版**正式弃用**（12 个月最低弃用窗口，功能仍可用，新实现不建议接入）。官方建议：Sampling → 直接对接 LLM Provider API；Roots → 用工具参数/资源 URI/服务端配置传目录；Logging → stdio 传输用 stderr、结构化可观测用 OpenTelemetry。
- **Elicitation**（2025-06-18 引入的服务端向用户收集输入的机制）：在无状态架构下由 **MRTR** 模式承载（见 2.5）。

### 2.4 传输层

| 传输 | 场景 | 状态 |
|---|---|---|
| **stdio** | 本地子进程 Server | 活跃 |
| **Streamable HTTP** | 远程 Server（单个 `/mcp` 端点，支持可选 SSE 升级） | 活跃（远程首选） |
| HTTP+SSE（旧双端点方案） | — | 已弃用（2025-03-26 起标记，2026-07-28 归入正式弃用清单） |

### 2.5 2026-07-28 规范：无状态化大改版

这是 MCP 发布以来最大的一次修订，核心变化直接影响部署架构：

1. **移除协议级会话**：废弃 `initialize`/`initialized` 握手与 `Mcp-Session-Id` 头。每个请求自描述——协议版本、客户端身份、客户端能力都放在请求 `_meta` 中（`io.modelcontextprotocol/protocolVersion`、`clientCapabilities`、`clientInfo`）。任何请求可落到轮询负载均衡后的任意实例，不再需要粘性会话和共享会话存储。
2. **可选发现**：新增 `server/discover` 调用，客户端可提前获取 Server 支持的版本与能力（非必需）。
3. **Header 路由**：方法与工具名进入 `Mcp-Method`、`Mcp-Name` HTTP 头，网关可直接按头路由与鉴权。
4. **可缓存的列表结果**：`tools/list` 等响应带缓存提示（`ttlMs`）与确定性排序，客户端可缓存工具目录，且保持上游 prompt cache 稳定。
5. **MRTR（Multi Round-Trip Requests）**：替代原先需要长开双向流的 `elicitation/create`、`sampling/createMessage`、`roots/list`。模式：工具执行中途需要输入时，返回 `resultType: "input_required"` 及所需请求，客户端收集答案后以 `inputResponses` 重试原调用。
6. **正式扩展框架**（SEP-2133）：扩展用反向 DNS ID 标识、经 capabilities 中的 `extensions` map 协商、独立仓库与版本。首批官方扩展：
   - **MCP Apps**：工具返回可在对话中直接渲染的交互 UI（与 OpenAI Apps SDK、MCP-UI 合作，已生产可用）；
   - **Tasks**（SEP-2663）：长时间运行任务的官方扩展；
   - **EMA（Enterprise-Managed Authorization）**：企业统一授权，一次登录访问全部 MCP Server，已稳定。
7. **授权加固**：响应必须按 RFC 9207 校验 `iss` 参数（堵授权服务器混淆攻击）；客户端注册从动态客户端注册（DCR, RFC 7591）转向 **Client ID Metadata Documents（CIMD）**；凭据绑定到签发授权服务器。
8. **流程治理**：正式的功能生命周期与弃用政策（Active → Deprecated → Removed，弃用窗口至少 12 个月）、SEP 工作流、贡献者阶梯（Contributor Ladder）。

### 2.6 版本演进史

| 版本 | 时间 | 关键内容 |
|---|---|---|
| 2024-11-05 | 2024-11 | 初版；stdio + HTTP+SSE；Sampling/Roots |
| 2025-03-26 | 2025-03 | Streamable HTTP 授权改进、音频内容、MCP 传输重构起点 |
| 2025-06-18 | 2025-06 | 结构化工具输出（structuredContent）、Elicitation、OAuth 资源服务器细化 |
| 2025-11-25 | 2025-11 | 上一代稳定版；进入 AAIF 治理；官方 Registry 推进 |
| **2026-07-28** | **2026-07** | **无状态核心、MRTR、扩展框架、授权加固、弃用政策（当前最新）** |

### 2.7 安全模型要点

- 信任锚在 **Host**：用户同意、能力协商、上下文聚合、Server 隔离都由 Host 强制。
- 远程 Server 走 OAuth 2.1 系（2025-06-18 起细化），2026-07-28 进一步对齐 OIDC 部署实践。
- **Tool Annotations**（readOnlyHint、destructiveHint、idempotentHint、openWorldHint 等）是工具行为的风险词汇表——注意官方立场：它们是**提示而非保证**，不能替代宿主侧的沙箱与审批。
- 已知攻击面：confused deputy、工具描述投毒（tool poisoning）、跨 Server 数据外泄——设计 Host 时需把"每个请求落在哪个 Server、能带什么上下文"当作安全决策点。

---

## 3. A2A 深入

### 3.1 定位与设计哲学

A2A 是**不透明 Agent 应用之间的通信与互操作协议**：不同框架、不同厂商构建的 Agent 通过它发现彼此的能力、协商交互方式、围绕任务协作——**无需暴露内部记忆、工具或专有逻辑**。

官方明确的"不是什么"（边界感是理解 A2A 的关键）：

- 不是 LangGraph/CrewAI/ADK 那样的 Agent 开发框架；
- 不是 sub-agent 或工具调用协议（Agent 调用自己的子 Agent、调用工具，用框架原语或 MCP）；
- 不是 MCP 的替代品；
- 不是聊天应用协议——它是机器对机器的自治系统协议。

### 3.2 角色与核心对象

三个角色：**User**（发起目标）→ **A2A Client（Client Agent）**（代表用户发起 A2A 通信）→ **A2A Server（Remote Agent）**（暴露 HTTP 端点执行任务；对客户端是黑盒）。

| 对象 | 职责 | 关键字段 |
|---|---|---|
| **Agent Card** | Agent 的"数字名片"：身份、端点、能力、技能列表、认证要求；客户端据此决定是否、如何、安全地交互 | `supportedInterfaces`（有序数组，每项含自己的 `protocolVersion`，可同时广告 v0.3 与 v1.0）、skills、securitySchemes、签名（JWS） |
| **Task** | 有状态的工作单元，唯一 ID + 明确生命周期，支撑长时任务与多轮交互 | taskId、status、artifacts、`contextId`（服务端生成，把相关 Task/交互分组） |
| **Message** | 一次交互轮次 | role（`user`/`agent`）、messageId、Parts |
| **Part** | 内容最小容器，模态无关 | 四选一：`text` / `raw`（内联字节）/ `url`（外部引用）/ `data`（结构化 JSON）；附加 mediaType、filename、metadata |
| **Artifact** | 任务的"实物产出"（文档、图片、结构化数据） | artifactId、name、Parts；可增量流式回传 |

Agent 响应两种形态：能立即回答 → 直接回 **Message**；需要执行长任务 → 创建 **Task**。

### 3.3 Task 生命周期

Task 有 8 种状态：`submitted → working →（input-required / auth-required）→ completed | canceled | failed | rejected`。

- **input-required / auth-required**：执行中需要用户补充输入或完成认证，任务暂停等待客户端回应后继续——这是 A2A 支持多轮人机协作的核心机制；
- **rejected**：远端 Agent 明确拒绝接受任务；
- 状态更新可通过 SSE 流实时推送，或 Webhook 回调。

### 3.4 交互模式与传输绑定

三种交互模式（按持久性/实时性选择）：

1. **轮询（Polling）**：发起请求后周期性查询任务状态，最简单；
2. **SSE 流式**：客户端打开 HTTP 流，实时接收增量结果与状态更新；
3. **推送通知（Push Notifications）**：超长任务或客户端可能离线时，Server 在关键节点回调客户端预注册的 Webhook。

传输绑定（同一套数据模型，三种线路格式，Agent Card 中按优先级声明）：

| 绑定 | 说明 |
|---|---|
| JSON-RPC 2.0 | 最初的经典绑定 |
| gRPC | 高性能、强类型场景 |
| HTTP+JSON/REST | Web 友好；v1.0 推荐媒体类型 `application/a2a+json` |

### 3.5 发现与信任

- **Well-known URI**：Agent Card 发布在约定地址（`/.well-known/agent-card.json` 类路径），客户端无需注册中心即可发现；
- **Registry**：公共/企业注册中心批量发现（官方 roadmap 中 registry 是重点方向）；
- **签名 Agent Card（v1.0 新增）**：卡片可携带 JWS 签名（RFC 7515，对 RFC 8785 规范化 JSON 计算），跨组织场景下客户端可先验证发布者身份再信任卡片；
- 认证凭据（OAuth token、API key 等）走标准 HTTP 头，与协议消息本身解耦；认证方案在卡片中声明（v1.0 支持 6 种 scheme，含 mTLS）。

### 3.6 v1.0 的关键变化（相对 v0.3）

v1.0.0（2026-03-12）是首个稳定版，规范标注 16 处 breaking change，与 v0.3 **不在线级兼容**（但一张卡片可同时广告两代接口，供迁移）：

| 变化 | v0.3 | v1.0 |
|---|---|---|
| Part 判别字段 | 显式 `kind` 字段 | 移除（内容字段本身即判别） |
| 枚举表示 | 字符串枚举 | 常量（如 `TASK_STATE_COMPLETED`） |
| 操作命名 | `message/send` 系 | 更名 **SendMessage / GetTask** 风格 |
| HTTP+JSON 路径 | 带 `/v1` 前缀 | 去除前缀 |
| 接口声明 | `url` + `preferredTransport` + `additionalInterfaces` | 单一有序 `supportedInterfaces` 数组 |

v1.0 新能力：**ListTasks**（带过滤与游标分页，只返回调用者有权看到的任务）、**多租户**（每个请求可带 `tenant` 字段，单端点承载多 Agent；卡片接口可声明默认租户）、**签名卡片**。

### 3.7 版本演进与治理

```
2025-04  Google 发布 A2A（50+ 企业同发）
2025-06-23  Google 捐赠给 Linux Foundation（OSS NA 宣布）
2025-07-31  v0.3.0（签名卡片雏形、mTLS、扩展卡片）
2025-08     IBM 的 ACP（Agent Communication Protocol）并入 A2A
2026-03-12  v1.0.0（首个稳定版）
2026-04     LF 一周年报告：150+ 组织、22k+ stars、云平台全面集成
2026-05-28  v1.0.1（当前最新：媒体类型/错误码对齐/示例勘误）
2026-08-17  加入 Agentic AI Foundation（AAIF）Growth Stage，与 MCP、goose、AGENTS.md 同门
2026-10-01  官方 A2A CLI 发布（拉卡片/发消息/流式订阅/stdin-server 测试）
```

治理：TSC 8 席——AWS、Cisco、Google、IBM Research、Microsoft、Salesforce、SAP、ServiceNow；Apache 2.0 许可。

Roadmap（v1.1 方向）：任务历史、事件过滤、**双向流**、多轮 **elicitation**、互操作规范、registry 与测试工具整合。

### 3.8 周边协议生态

- **AP2（Agent Payments Protocol）**：基于 A2A 的支付扩展，让 Agent 发起的交易具备加密授权凭证（user consent mandate），60+ 支付/金融组织参与；UCP 经 AP2 mandates 扩展与之兼容——Agent 商务（Agentic Commerce）的信任底座。
- **AGNTCY / agentgateway**：Cisco 系 agent 生态资产，与 A2A 同属 LF/AAIF 体系。

---

## 4. A2A vs MCP：对比与组合

### 4.1 维度对比

| 维度 | MCP | A2A |
|---|---|---|
| 对端是什么 | 工具/数据/API（原语：无状态、结构化输入输出） | 其他 Agent（自治：会推理规划、多轮对话、维护状态） |
| 架构 | Host 内嵌多 Client，1:1 连 Server | Client Agent ↔ Remote Agent，对等协作 |
| 会话/状态 | 无状态（请求自描述） | 显式 Task 生命周期 + contextId |
| 长任务 | Tasks 扩展（2026-07-28 起官方扩展） | 一等公民（生命周期、SSE、Webhook 原生） |
| 中途要输入 | MRTR（resultType: input_required） | input-required / auth-required 任务状态 |
| 上下文共享 | Host 聚合，Server 相互隔离 | 不共享内部记忆；通过 Message/Artifact 显式交换 |
| 多模态 | 文本/文件为主，结构化 JSON | Part 原生支持 text/raw/url/data，音视频文件均可 |
| 身份/信任 | OAuth 2.1 + CIMD + EMA；Server 侧匿名能力导向 | 签名 Agent Card、多租户、企业认证 scheme |
| 典型问题域 | "帮我查数据库/调 API/读文件" | "这个案子你专业，帮我处理一下，做完给我结果" |

### 4.2 组合架构（官方推荐姿势）

```
            ┌──────────────┐   A2A    ┌──────────────┐
  User ────▶│ Client Agent │◀───────▶│ Remote Agent │
            │  ┌────────┐  │          │  ┌────────┐  │
            │  │MCP     │  │          │  │MCP     │  │
            │  └───┬────┘  │          │  └───┬────┘  │
            └──────┼───────┘          └──────┼───────┘
                   │MCP                      │MCP
             ┌─────▼─────┐             ┌─────▼─────┐
             │ 工具/数据   │             │ 专属工具    │
             └───────────┘             └───────────┘
```

**MCP 给每个 Agent 深度（装备工具），A2A 给系统广度（跨边界协作）。** 官方汽修厂案例：客户 ↔ 店长 Agent 走 A2A 对话；店长委派 mechanic Agent（A2A）；mechanic 用 MCP 调诊断仪/维修手册/举升机；mechanic 向零件供应商 Agent 下单（A2A）。

### 4.3 集成时的已知挑战（来自学术分析与社区实践）

- **语义鸿沟**：A2A 的 skill 描述粒度 ≠ MCP 工具 schema 的精确度；把高层任务翻译成正确的工具调用序列仍是 Agent 的推理责任，协议不兜底；
- **组合安全**：发现（A2A）与执行（MCP）叠加后，权限边界、审计、治理需要额外的统一治理层（协议本身不管这个）；
- **跨协议调试**：一次用户请求穿 A2A→Agent→MCP→工具多层，需要跨协议的 trace 关联（OpenTelemetry 是当前共识方向）。

---

## 5. Agent 如何使用这些协议（实操视角）

### 5.1 作为 MCP 宿主：给 Agent 装备工具

标准流程：

1. 配置 Server 连接（本地命令或远程 URL + 认证）；
2. Host 为每个连接创建 Client；
3. 拉取工具目录 `tools/list`（2026-07-28 起响应可按 TTL 缓存）；
4. 把工具定义转换成 LLM 的 function/tool 定义；
5. LLM 决定调用 → Host 执行 `tools/call` → 把 `structuredContent` 塞回模型上下文 → 循环。

`tools/call` 请求在 2026-07-28 规范下长这样（无状态，一个请求自足）：

```http
POST /mcp HTTP/1.1
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/call
Mcp-Name: search

{"jsonrpc":"2.0","id":1,"method":"tools/call",
 "params":{"name":"search","arguments":{"q":"otters"},
  "_meta":{"io.modelcontextprotocol/clientInfo":{"name":"my-app","version":"1.0"}}}}
```

设计要点：

- **权限与同意在 Host 侧**：哪些 Server 可连、每次调用是否需用户确认、上下文带多少，都是 Host 的策略决策；
- Server 隔离：不要把 A Server 的数据透给 B Server；
- 工具结果有 `resultType`（complete / input_required）；遇到 `input_required`（MRTR）时收集用户输入再重试。

### 5.2 构建 MCP Server：暴露自己的工具

- 专注单一能力域，Server 极简；
- 工具 schema 用完整 JSON Schema 2020-12（组合、条件、$ref 都可用）；
- 输出尽量用 `structuredContent`（结构化数据，供宿主程序消费）+ 文本摘要（供模型阅读）双通道；
- 远程部署：Streamable HTTP + OAuth 2.1；企业场景支持 EMA 扩展可大幅降低接入摩擦；
- 跨调用状态不要藏在会话里——用服务端铸造的显式句柄作为工具参数传递（模型可见、可在工具间穿线）。

### 5.3 作为 A2A 客户端：发现并委派任务

1. **发现**：从 well-known URI / registry 获取 Agent Card；校验签名（跨组织必做）；按 skills + 认证要求 + transport 偏好选择 Agent；
2. **协商**：读 `supportedInterfaces`，按各自 `protocolVersion` 选定交互接口与绑定（JSON-RPC/gRPC/REST）；
3. **委派**：`SendMessage` 发起（同步、SSE 流式两种）；对方要么立即回 Message，要么创建 Task 返回 taskId；
4. **跟进**：轮询 `GetTask` / 订阅 SSE / 注册 Webhook；状态进入 `input-required` 时按提示补充内容再推进；`auth-required` 时完成认证流；
5. **收货**：从 Task 的 `artifacts` 取结果（可增量流式）；用 `contextId` 延续同一业务上下文的多轮协作；长会话用 `ListTasks` 检索历史任务。

### 5.4 构建 A2A Server：把自己变成可被协作的 Agent

- 发布 Agent Card：身份、skills（写清楚每个技能做什么——这是别人"选中你"的依据）、认证 scheme、传输接口清单；
- 技能实现为 Task 生命周期状态机：接收 → working →（必要时 input-required）→ completed + artifacts；
- 保持"不透明"：只通过 Message/Artifact 交换信息，不暴露内部 prompt、工具清单、记忆——这是协议设计哲学，也是你的 IP 边界；
- 企业化：多租户（tenant 路由）、签名卡片、企业认证。

### 5.5 组合模式：什么时候用哪个

决策口诀：

- 对端是**无状态的原子能力**（查库、算数、调 API）→ **MCP 工具**；
- 对端是**会推理、有多轮、有状态的自治系统**，且可能属于别的团队/组织 → **A2A Agent**；
- 一个 Agent **内部**的子 Agent 编排 → 用框架原语（Eino 的 graph/branch、LangGraph 等），不必强上 A2A；
- 同一个能力既要给自己用又要给别人用 → 可以"双暴露"：技能边界清晰、可无状态调用的部分包成 MCP 资源/工具；有状态协作流程走 A2A。

### 5.6 Go 技术栈落地（贴合 eino-multiagent-lab）

| 组件 | 现状（2026-11） |
|---|---|
| MCP Go SDK | **Tier 1**：官方 `modelcontextprotocol/go-sdk`，已随 2026-07-28 规范更新（无状态核心、MRTR 支持） |
| A2A Go SDK | `a2aproject/a2a-go`，v2.5.0（2026-08-18），生产级 |
| A2A CLI | 官方 `a2a` 命令行（2026-10-01），可拉卡片/发消息/流式订阅，也可把任意 stdin/stdout 程序变成测试用 A2A Server——本地实验很方便 |
| Eino 集成思路 | Eino 的 Tools 组件抽象（`InvokableTool` 等）可以包一层适配器对接 MCP Go SDK 的 client，把远端 MCP 工具注册进 Eino agent 的 tool set；A2A 侧则把远端 Agent 封装成一个 Eino Tool（对外仍是一次 function call，内部走 a2a-go） |

建议的上手路径（本仓库实验）：

1. 用 MCP Go SDK 起一个最小 stdio Server（1 个工具），在 eino-lab 里以 Host 身份接入；
2. 用 `a2a` CLI 跑官方示例 Server，体验 Agent Card → SendMessage → Task 流转；
3. 用 a2a-go 把 lab 里的一个子 Agent 暴露成 A2A Server，再从主 Agent 以 A2A 客户端身份委派任务——正好对应"框架内编排 vs 跨进程协议协作"的对比实验。

### 5.7 版本兼容备忘

- MCP：写代码认准 `2026-07-28`；连接旧版 Server 时协议版本随请求声明，注意旧握手（initialize）仅存在于 2025-11-25 及更早实现；
- A2A：新项目直接 v1.0.1；需要对接存量 v0.3 Agent 时，利用卡片可同时广告双版本接口的机制平滑迁移（官方建议 2026 年底前完成 0.x 迁移）。

---

## 6. 生态与趋势

- **规模**：MCP Tier 1 SDK 月下载近 5 亿（TS/Python 累计各破 10 亿）；A2A 150+ 组织、22k stars，供应链/金融/保险/IT 运维已有生产部署。
- **云平台**：A2A 已 GA 集成 Azure AI Foundry、Copilot Studio、Amazon Bedrock AgentCore Runtime、Google Vertex AI Agent Builder——跨云 Agent 互操作成为云厂商默认能力。
- **治理合流**：2026-08 起 MCP 与 A2A 同属 AAIF，加上 goose、AGENTS.md、agentgateway，Agent 基础设施的开放标准版图基本成型。
- **共同演进方向**：长任务原语（MCP Tasks 扩展 ↔ A2A Task 生命周期）、多轮输入（MRTR ↔ input-required）、服务端发现（Server Card WG ↔ Agent Card/Registry）、Agent 身份与支付（EMA/AP2）——两个协议在向对方学习，但边界保持清晰。
- **关注但不急**：AGP 等研究型协议在补"生命周期/版本演进管理"的空白；对工程实践而言，MCP+A2A 仍是当前默认答案。

---

## 7. 参考资料

**MCP**
- 官方规范（2026-07-28）：https://modelcontextprotocol.io/specification/2026-07-28
- 规范变更记录：https://modelcontextprotocol.io/specification/2026-07-28/changelog
- 官方博客（2026-07-28 发布说明）：https://blog.modelcontextprotocol.io/posts/2026-07-28/
- 架构文档：https://modelcontextprotocol.io/specification/2026-07-28/architecture

**A2A**
- 官方站点与规范（v1.0.1）：https://a2a-protocol.org/
- 核心概念：https://a2a-protocol.org/latest/topics/key-concepts/
- A2A 与 MCP 官方对比：https://a2a-protocol.org/latest/topics/a2a-and-mcp/
- LF 一周年公告：https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year
- Go SDK：https://github.com/a2aproject/a2a-go · MCP Go SDK：https://github.com/modelcontextprotocol/go-sdk

**延伸**
- A2A+MCP 集成挑战的学术分析：https://arxiv.org/pdf/2505.03864.pdf
- 中文社区技术站（非官方）：https://agent2agent.info/zh-cn/about/

---

> 本内容由 Coze AI 生成，请遵循相关法律法规及《人工智能生成合成内容标识办法》使用与传播。
