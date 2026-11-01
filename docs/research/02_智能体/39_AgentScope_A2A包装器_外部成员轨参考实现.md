---
title: AgentScope A2A 包装器——外部成员轨消费端参考实现
source: REQ-463/M263；wrappers/agentscope-a2a/；01 §3.12
---

# AgentScope A2A 包装器（REQ-463/M263，2026-10-28 交付）

> 定位：平台「外部智能体接入」轨（REQ-426~434 演进批 + A2A 深化批 REQ-441~446）的生态延伸——把 **AgentScope**（任意 Python agent 运行时的代表）经 A2A 协议接入平台协作面，实证「外部接入通道语言无关」的多后端主张。平台侧**零代码**（全链由既有 a2a client 消费）。

## 一、它是什么

`wrappers/agentscope-a2a/` 平台外 opt-in 独立服务：A2A server（标准库实现）+ 两档运行模式（stub 零凭证 / agentscope 真模型）。**不进 run-dev.sh 主链**（D-O15：平台主链零 Python 依赖零 venv，沿 REQ-325 sidecar「进程外可缺省」先例）——仅当显式运行时才需要 python3。

```bash
cd wrappers/agentscope-a2a && python3 -m agentscope_a2a --port 8099   # stub 零凭证
AGENTSCOPE_MODEL=qwen-max pip 装好 agentscope 后重跑                    # 真模型（凭证自备）
```

## 二、平台侧接入步骤（零代码）

1. 启动包装器（上方命令；`--port 0` 时随机端口经 stdout `PORT=xxxx` 宣告）。
2. 平台「设置 → 连接器」新增 **kind=a2a** 连接器：端点 `http://127.0.0.1:8099`——保存即真握手注册（拉 `/.well-known/agent-card.json`，缺 name 字段即注册失败如实报）。
3. 智能体（协调者）连接器勾选该实例 → 协作面外部成员（三色第三色）→ agent_as_tool 委派：message/send+轮询至终态；card 声明 `capabilities.streaming=true` 时走 message/stream SSE（REQ-432）。
4. 过程可见性=对端任务 history 转译 external.audit 事件（REQ-428）；能力画像（mode/成本档）经 card.properties 注入协调者描述（REQ-429）。

## 三、协议面（对齐平台 internal/a2a server，REQ-442 形态）

| 方法 | 语义 |
| --- | --- |
| `GET /.well-known/agent-card.json` | 卡片发现（name/skills/capabilities/properties.mode） |
| `message/send` | 异步任务化：立即回 working，后台转终态；同 contextId 单运行闸（REQ-441 同型） |
| `tasks/get` | 轮询快照（终态附 artifacts+history）；进程内任务表重启即失 TaskNotFound（同平台口径） |
| `tasks/cancel` | working→canceled，竞态防覆写（REQ-441 同型） |
| `message/stream` | SSE：状态迁移即帧，终态关流（平台 SendStream 消费） |

**input-required 终态**：stub 模式 `ask: <问题>` 前缀演示——问题文本经 status.message 外发（对齐平台 REQ-443 ask_human 语义）；平台 client 对 input-required 按「失败如实报」处理（无交互通道口径），消费方可取问题文本补答后重发。

## 四、边界与治理（通用口径，非本件特例）

- **审批**：外部成员委派无审批面（审批档仅覆盖平台本地工具轨；REQ-427 治理拉齐的边界延续）。
- **上下文隔离**：每次委派只携带末条用户消息（无平台历史回灌）；多轮靠 contextId 透传，会话沉淀在 wrapper 侧内存（重启即失）。
- **黑盒**：AgentScope 内部编排（ReAct 循环/工具选择）对平台不可见——只见输入输出与 history 转译。
- **FilePart 未支持**：纯文本单模态（REQ-447 触发驱动）。
- **凭据**：真模型模式凭证在 wrapper 侧环境变量自管，不进平台连接器（与 dsh 凭据自管同口径）。

## 五、验证与实证

- 跨语言回环单测：`backend/internal/a2a/agentscope_wrapper_test.go`——平台 client（FetchAgentCard/Send/AwaitTerminal/TasksGet/SendStream/CancelTask 原样零改动）spawn wrapper stub 进程全链六段（card 握手/回显终态/history 形态/input-required/SSE 流/cancel 回环）；python3 不在时 SKIP（平台零依赖纪律不受影响）。
- 独立冒烟：`wrappers/agentscope-a2a/examples/smoke.py`（SMOKE PASS）。
- 真模型端到端随凭证批；本档 stub 即协议面全链实证。
