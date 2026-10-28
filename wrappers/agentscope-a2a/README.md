# agentscope-a2a — AgentScope A2A 包装器（REQ-463/M263）

平台外 **opt-in** 独立服务：把 [AgentScope](https://github.com/modelscope/agentscope)（或无凭证 stub）智能体经 A2A 协议暴露给平台，「任意 Python agent 运行时经 A2A 即插即用」多后端主张的参考实现。

**D-O15 合规**：本目录不进 `run-dev.sh` 主链——平台主链零 Python 依赖零 venv（沿 REQ-325 sidecar「进程外可缺省」先例）。仅当你显式运行本包装器时才需要 python3。

## 快速开始（stub 模式，零凭证）

```bash
cd wrappers/agentscope-a2a
python3 -m agentscope_a2a --port 8099
# → agentscope-a2a listening on http://127.0.0.1:8099 mode=stub
```

然后在平台「智能体 → 连接器」新增 `kind=a2a` 连接器，端点填 `http://127.0.0.1:8099`——注册真握手（拉 Agent Card）、委派走 message/send+轮询、流式走 message/stream（card 声明 streaming=true）。平台侧**零代码改动**（全链由既有 client 消费；`backend/internal/a2a/agentscope_wrapper_test.go` 即跨语言回环实证）。

stub 行为：文本回显+计划式回复；`ask: <问题>` 前缀演示 **input-required** 终态（问题文本外发，对齐平台 REQ-443 ask_human 语义）。

## 真模型模式（凭证自备）

```bash
pip install agentscope
export AGENTSCOPE_MODEL=qwen-max          # DashScope 模型名
export DASHSCOPE_API_KEY=sk-...           # AgentScope 凭证
python3 -m agentscope_a2a --port 8099
```

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `AGENTSCOPE_MODEL` | （空=stub） | 设置后走 agentscope ReActAgent；未安装 agentscope 时启动期人话报错 |
| `AGENTSCOPE_AGENT_NAME` | AgentScope Wrapper Agent | 卡片 name |
| `AGENTSCOPE_SYS_PROMPT` | 你是一个经由 A2A 接入平台的智能体。 | 真模型模式系统提示词 |
| `AGENTSCOPE_MAX_ITERS` | 8 | ReActAgent 迭代上限 |

## 协议面（对齐平台 internal/a2a server，REQ-442 形态）

| 方法 | 语义 |
| --- | --- |
| `GET /.well-known/agent-card.json` | 卡片发现（name/skills/capabilities.streaming/properties.mode） |
| `message/send` | 异步任务化：立即回 working task，后台执行转终态；同 contextId 单运行闸 |
| `tasks/get` | 轮询快照（终态附 artifacts+history）；重启丢映射 TaskNotFound（同平台口径） |
| `tasks/cancel` | working→canceled（竞态防覆写，对齐 REQ-441） |
| `message/stream` | SSE：状态迁移即帧（JSON-RPC result=任务快照），终态关流 |

## 诚实边界（与平台通用口径一致）

- 外部成员无审批面（审批档仅覆盖平台本地工具轨）。
- AgentScope 内部编排对平台黑盒：平台只见输入输出与 history 转译过程（REQ-428）。
- 上下文隔离：每次委派只携带末条用户消息（无平台历史回灌）；多轮靠 contextId 透传。
- FilePart 未支持（REQ-447 触发驱动）；纯文本单模态。
- 真模型模式端到端随凭证批；stub 模式为协议面全链验证。

正式接入指引（产品视角）见 `platform-knowledge/02_智能体/` 相应档。
