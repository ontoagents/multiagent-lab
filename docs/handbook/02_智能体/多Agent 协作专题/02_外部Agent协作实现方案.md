---
module: 智能体
topic: 外部 Agent 协作实现方案（多Agent 协作专题）
desc: 平台与外部 Agent/工具协作的实现梳理——MCP 双向（消费/提供/stdio 桥接/托管插件）、A2A 双向（自研 JSON-RPC server/client/外部成员三轨接入）、CLI 推理后端与连接器统一治理；附协议差距分析与研究方向补充区
synced: 2026-11-10
---

# 外部 Agent 协作实现方案

> [专题导读](README.md)第二档：梳理当前项目内**与外部 Agent/外部工具协作**的实现现状，对照 A2A/MCP 协议研究给出差距与研究方向。平台内部协作见 [01 档](01_平台内部多Agent协作实现方案.md)；协议事实源=[协议研究档](A2A_MCP协议研究.md)（本档只做对照引用，不复制协议内容）。

## 一、定位与边界

- **平台差异化定位（Q-22）**：自研 runtime 退守「协调者＋轻执行者」，重执行外接主流智能体——平台差异化=**协议枢纽（MCP/A2A 双向）＋统一治理＋资产供给＋跨协议过程可视化**。
- **单机自部署约束**：无公网回调地址——A2A pushNotifications=false 如实标注（`backend/internal/a2a/a2a.go:120`），实时性由 SSE 承载；Webhook 类机制不做（REQ-432 评估结论）。
- **依赖基线**：MCP=`eino-ext/components/tool/mcp v0.0.9` ＋ `mark3labs/mcp-go v0.43.0`；A2A=**自研** JSON-RPC 实现（无 a2a 框架依赖）；协议版本差距分析见 §八。
- **无 OpenAI 兼容 serving**：第三方调用平台智能体的正式面=REST/SSE＋MCP＋A2A 三面（openai_compat 仅作为消费协议连接外部模型供应商）。

## 二、对外协作面总览

| 协作面 | 端点/通道 | 方向 | 承载 |
| --- | --- | --- | --- |
| REST/SSE | `POST /api/conversations/{id}/runs`（SSE 流式）＋约 250 条 `/api/*` 管理路由 | 平台管理/对话 | `backend/internal/api/`（PLATFORM_TOKEN 鉴权） |
| MCP 提供 | `GET/POST /mcp`（Streamable HTTP）、`/mcp/kb` | 平台 → 外部 | Agent 服务化（agent_{id} 工具）＋KB 检索 |
| MCP 消费 | 连接器 kind=mcp（Streamable HTTP）/ kind=stdio（本地子进程） | 外部 → 平台 | 装配期拉工具目录入 Agent 工具表 |
| A2A 提供 | `GET /api/a2a/agents`、`GET /api/a2a/agents/{id}/.well-known/agent-card.json`、`POST /api/a2a/agents/{id}` | 平台 → 外部 | Agent Card＋Task 生命周期 |
| A2A 消费 | 连接器 kind=a2a | 外部 → 平台 | 外部 A2A 成员进协作面 |
| CLI 后端 | 连接器 kind=cli＋inference 适配器 | 外部 → 平台 | claude-code/opencode/aider/dsh 作成员或执行后端 |
| 统一抽象 | Connector 七类：mcp / kubernetes / ssh / a2a / cli / stdio / devops | 双向 | `backend/internal/store/connectors.go`＋托管插件 |

## 三、MCP 双向

### 3.1 消费侧（client——外部工具进平台）

- **Streamable HTTP**：`tool.FetchMCPToolsWithHeaders`（`backend/internal/tool/mcp.go:123`）拉工具目录——`{server}__{tool}` 前缀改名防撞（:21-51）、TTL 60s 进程内缓存（:53-81，REQ-356）、readOnlyHint 注解采集（:176）；拉回的工具转换进 Eino 工具表，与内置工具同管线受审批/审计/hook 覆盖。
- **stdio 桥接**（REQ-433）：`backend/internal/tool/stdio.go`——连接器 kind=stdio，命令白名单 npx/node，装配期真握手取清单、每调用独立 spawn（零孤儿进程）；npx 型本地 MCP server 生态接入。
- **自引用防护**：`isSelfMCPEndpoint`（`chat/assembler.go:1133`）拦截平台自身 `/mcp` 端点的环回挂载。

### 3.2 提供侧（server——平台 Agent 对外服务化，REQ-131/132）

- 端点 `GET/POST /mcp`（`api/server.go:220-225`，懒重建缓存）＋`/mcp/kb`（KB 检索对外提供方，REQ-554）；
- `mcpHandler`（`api/handlers_mcp.go:38-67`）用 mcp-go `NewStreamableHTTPServer` 把每个开启 `McpServe` 的 Agent 暴露为 `agent_{id}` 工具（`addAgentTool` :84，一次独立 run 承载）；
- Agent 级 Bearer Token 鉴权（`mcpAuth` :155），与 /mcp/kb 同口径。

### 3.3 平台托管 MCP 插件

`backend/internal/connector/plugin.go`（`PluginService`）：k8s/ssh/devops 三类由**进程内 Go MCP 插件**承载（loopback `PluginEndpoint`，main.go:211-212），经 `/connectors/{id}/mcp` 暴露——与 mcp 直通连接器同一装载管线，凭据不出进程。

## 四、A2A 双向（自研实现）

### 4.1 提供侧（server，REQ-369/387 端点＋REQ-441~446 深化批）

`backend/internal/a2a/a2a.go`——JSON-RPC 2.0 over HTTP：

- `HandleRPC`（:170）支持 `message/send`（:197）、`message/stream` SSE（:313）、`tasks/get`（:527）、`tasks/cancel`（:562）；
- `AgentCard`（:102）发布于 well-known 路径；任务落 `a2a_task` 表（migrations/062）——重启对账标 failed（可重新委派）＋30 天保留窗口清理（server.go:201-205，REQ-445）；
- 每个对外暴露的 Agent 由正常装配链执行（A2A 请求→会话 run），SSE 流式与对话流同源同管线。

### 4.2 消费侧（client，REQ-416/432）

`backend/internal/a2a/client.go`：`FetchAgentCard`（:68，发现）、`Send`（:276，委派）、`SendStream`（:206，SSE 订阅＋失败回落轮询）、`AwaitTerminal`（:320，轮询终态）、`CancelTask`（:303）。

### 4.3 外部成员进协作面（三轨）

- **接入**：连接器 kind=a2a → `assembleA2AMemberSpecs/Members`（`chat/assembler.go:563/612`）——外部 Agent 与本地成员并列进项目；
- **agent_as_tool 轨**：`a2aMemberInvoke`（`chat/a2a_member.go:89`）——input-required 状态结构化透传协调者（:208，向用户转述问题附答案再委派）、失败降级回喂（:118，沿 REQ-319 语义）、过程转译 `external.audit` 事件（:102，REQ-428）；
- **workflow/transfer 轨**：`newA2AMemberAgent`（a2a_member.go:137）＋`externalMemberAgent` 适配（`chat/external_member_agent.go:19-30`）——A2A 成员可作 DAG/编排节点与转移目标（REQ-446/M242 三轨混合编排全成立）；
- **CLI 成员同构**：`newCLIMemberTool`（`chat/cli_member.go:59`）——外部 CLI 智能体以同一协作面接入（见 §五）。

## 五、CLI 推理后端与 ACP

- **推理后端可插拔**（`backend/internal/inference/`）：`Registry/Backend` 接口（inference.go:97-121）＋`cliAdapter`（adapters.go:42）——claude-code（stream-json 解析 :168）/opencode/aider/dsh 按 PATH 探测桥接；dsh 另有 ACP 协议客户端（acp.go，JSON-RPC over stdio：session/new|prompt|update|resume|request_permission）。
- **两种用法**：①Agent 整体走外部后端（`chat/external.go runExternal`:35 执行，技能/MCP 降级为提示词注入，harness/context 旁路——53/54 号诚实边界）；②外部 CLI 智能体作**项目成员**（newCLIMemberTool，与 A2A 成员同轨）。
- **安装引导**：install hint registry＋探测失败保底 eino-adk（REQ-450）。

## 六、连接器统一治理

- **七类连接器**（REQ-214 起统一产品概念）：mcp/kubernetes/ssh/a2a/cli/stdio/devops——`testConnector` 真握手回写 tools/card 快照（handlers_connectors.go:465-488）；
- **凭据**：AES-256-GCM 服务端加密（`secrets.Box`），**不进 LLM 上下文**；CLI 场景 credentials.env 凭据态注入子进程/容器环境；
- **治理拉齐**（REQ-427）：外部成员委派调用纳入审批档（danger/all 均审＋豁免透传）＋审计事件＋CLI 子进程 docker 沙箱（三前置闸门）；
- **过程可视化**（REQ-428）：CLI stream-json/A2A 任务 history 统一转 `external.audit` run_event——跨协议（internal/cli/a2a）委派时间线/泳道分色/拓扑外部节点过程计数单面呈现；
- **能力画像**（REQ-429）：`ExternalMemberProfile`（`chat/member_profile.go:22`）合成 CLI 探测＋A2A card 技能画像，注入协调者描述支撑「描述即路由」；
- **上下文供给**（REQ-430）：委派外部成员时项目上下文三源注入（文件清单/KB 检索/本体摘要，`assembleScope.ExtContext`）；`context_injection` 开关**默认关**——内容随委派外发须显式确认；
- **混合编排模板**（REQ-431）：HybridTeamTemplate 一键创建本地＋CLI＋外部三色混合团队；
- **反向发现**（REQ-434）：`AgentsExposed` 扩 external_members 元数据透出——外部智能体反向发现平台资产。

## 七、代码地图

| 机制 | 落点 |
| --- | --- |
| A2A server/client | `backend/internal/a2a/a2a.go`、`backend/internal/a2a/client.go`（端点 api/server.go:457-461） |
| A2A 任务持久化 | `a2a_task` 表（migrations/062，store/a2a_task.go） |
| A2A/CLI 成员接入 | `backend/internal/chat/a2a_member.go`、`cli_member.go`、`external_member_agent.go`、assembler.go:563/612 |
| MCP 消费 | `backend/internal/tool/mcp.go:123`、`backend/internal/tool/stdio.go` |
| MCP 提供 | `backend/internal/api/handlers_mcp.go:38-67`、api/server.go:220-226 |
| 托管 MCP 插件 | `backend/internal/connector/plugin.go`（main.go:211-212） |
| 连接器 | `backend/internal/store/connectors.go`、`api/handlers_connectors.go` |
| CLI 推理后端/ACP | `backend/internal/inference/`（adapters.go/acp.go）、`chat/external.go` |
| 能力画像 | `backend/internal/chat/member_profile.go:22` |
| 协议可视化 | `api/handlers_protocol.go`（cards/tasks）＋external.audit 转译 |

## 八、协议差距与研究方向（后期在此补充）

> 对照[协议研究档](A2A_MCP协议研究.md)（信息核验至 2026-11-06）的初始分析；后续跟踪结论直接完善本节。

### 8.1 A2A 侧

1. **v1.0 对齐评估**：当前自研实现按 `message/send` 方法族＋JSON-RPC 2.0 绑定（v0.3 世代口径）；v1.0（2026-03-12）16 处 breaking change——方法族更名（SendMessage/GetTask 风格）、枚举常量化、`supportedInterfaces` 数组化、HTTP+JSON 去 `/v1` 前缀等。对齐策略（双版本卡片广告/一次性切换）待研究。
2. **新能力采纳评估**：签名 Agent Card（JWS，跨组织信任）、多租户（tenant 路由）、ListTasks（历史检索）、AP2 支付扩展——按单机自部署场景取舍。
3. **SDK 策略**：官方 Go SDK `a2aproject/a2a-go`（v2.5.0，生产级）已成熟——自研实现 vs 换 SDK 的成本/收益/回归面评估。
4. **发现机制**：当前平台内连接器手工登记＋well-known 卡片；registry/企业目录（官方 roadmap 重点）是否引入待研究。
5. **A2A 自环防护缺失**（2026-11-09 核实；**2026-11-10 已立项=REQ-624-fced 📋，01 §3.32**——测试握手与装配期按 环回主机＋本平台端口＋`/api/a2a/` 前缀识别自环拒绝或 connector.degraded 告警，人话错误注明处置；REQ-625 前置依赖；领 M 号随领取轮）：MCP 消费侧有 `isSelfMCPEndpoint` 环回拦截（assembler.go:1133/1285），A2A 连接器**无对等防护**——指向本平台 `/api/a2a/agents/{id}` 的回环连接器可创建并通过（代理 agent 场景下还会构成「平台 A2A 端点→连接器→本平台」二环）。

### 8.2 MCP 侧

1. **规范版本对齐**：mcp-go v0.43.0 承载的规范版本待核实；2026-07-28 版无状态化大改版（移除 initialize 握手/会话、`server/discover`、Header 路由、可缓存 tools/list、MRTR）——消费/提供两侧的升级路径与旧版兼容面评估。
2. **官方 SDK 评估**：`modelcontextprotocol/go-sdk`（Tier 1，已随 2026-07-28 更新）vs mark3labs/mcp-go——迁移窗口与双依赖收敛。
3. **扩展框架采纳**（SEP-2133）：MCP Apps（工具返回可渲染 UI）、Tasks 扩展（长任务）、EMA（企业统一授权）——单机场景优先级低，登记跟踪。

### 8.3 组合与治理

1. **跨协议 trace 关联**：一次请求穿 A2A→Agent→MCP→工具多层，OpenTelemetry 关联是研究档 §4.3 指出的共识方向——与现有 run_event/external.audit 体系的关系待设计。
2. **组合安全**：发现（A2A）与执行（MCP）叠加后的权限边界与审计归一——现有审批档/凭据加密是否充分，待安全视角复查。
3. **语义鸿沟**：A2A skill 描述粒度 ≠ MCP 工具 schema 精度——能力画像（REQ-429）已部分缓解，路由准确性的度量与改进待研究。

### 8.4 代理 agent（A2A 执行后端）——2026-11-09 分析结论（**2026-11-10 已立项=REQ-625-b747 📋，01 §3.32**——执行后端扩展形态落地；前置依赖 REQ-624 自环防护；优先级中低，M 号随领取轮）

**概念**：新建 Agent 时执行后端指向 a2a 连接器（外部端点）——该 Agent 每次运行＝输入经 A2A 转发外部 Agent（card 发现→Send/SendStream→input-required 透传→artifacts 回传），本地零 LLM 调用；外部 Agent 获得平台内完整身份（直聊会话树/项目成员资格/审批审计/画像注入/**A2A·MCP 再暴露**）。与现有「连接器授权」（Agent 保持本地 runtime、外部仅为其委派工具）是两种形态。

| 维度 | 代理 agent（整体外接） | 连接器授权（委派外接，现状） |
| --- | --- | --- |
| 本地模型成本 | 零本地 LLM 调用 | 协调者消耗本地模型 |
| 语义控制 | 外部全权（本地提示词不生效） | 本地协调者编排，可混合多成员 |
| 直聊外部 Agent | 支持（独立会话树） | 不支持（仅项目委派触达） |
| 失败/输入语义 | input-required 直接映射会话态 | 降级回喂（REQ-319） |
| 延迟 | 单跳 | 协调者推理＋委派两跳 |
| 风险 | 身份混淆（平台名片包装外部能力）＋再暴露回环（§8.1.5） | 路由质量依赖描述/画像 |

**结论**：真实缺口=「直聊外部 Agent」（现状只能项目内委派）；CLI 侧已有对称先例（inference_backend=cli 整体外接，`runExternal`）且 `externalMemberAgent` 适配器可复用——**建议形态不是新增 Agent 实体类型，而是执行后端扩展**（inference_backend 增 `a2a` 值，backend_conn_id 指向 a2a 连接器），零新概念、与 CLI 外接对称；优先级中低（连接器授权已覆盖协作主场景），前置依赖=自环防护（§8.1.5）。
