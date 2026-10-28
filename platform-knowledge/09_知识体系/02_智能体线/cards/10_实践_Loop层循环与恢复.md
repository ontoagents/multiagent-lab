---
module: 知识体系
topic: 实践·Loop 层循环治理与挂起恢复
desc: 迭代上限、checkpoint 持久化、挂起恢复/放弃/超时、定时续跑与 usage 成本护栏
synced: 2026-10-28
kind: practice
domain: agent
prereq: ["08_实践_Harness装配链.md"]
sources:
  - platform-knowledge/02_智能体/55_Loop_设计实现与演进.md
  - backend/internal/chat/checkpoint.go
  - REQ-204
  - REQ-231
check:
  - 挂起（interrupt）的三种去向是什么？
  - 为什么挂起机制在 Loop 层而审批判定语义在 Harness 层？
---

**迭代上限**：agent.max_iteration（默认 25，normalizeMaxIter 兜底）——ReAct 循环的硬护栏，防模型无限自我对话。到限即终止并如实报错，不是静默吞掉。

**挂起与恢复**（checkpoint.go，迁移 028 SQLite 持久化）：工具要审批/ask_human 主动提问时，运行挂起——checkpoint 快照落库（**后端重启仍可恢复**），run.interrupted 事件标注 kind=approval/ask_human；恢复三去向：①resume 定向投递答案（审批通过/用户回答，多级审批沿用 cpAnchors 锚点）；②新消息到来放弃挂起（abandon，带 reason 留痕）；③挂起超时自动 deny（REQ-231③）。中断恢复链路的悬空 ToolCalls 由 patchtoolcalls 中间件在下次模型调用前补合成结果（REQ-454）。

**长任务推进**：会话级 schedule 定时续跑（REQ-224 状态机读模型可查 phase 派生：waiting_approval>waiting_human>running>scheduled>idle）；usage 统计（REQ-397 per-tool 成本归因挂观察）回答「花了多少」。

**层边界辨析**：审批「挂起」这个动作的载体（checkpoint）在 Loop 层，但「什么工具该审批」的判定语义在 Harness 层——两层的责任表见 55 号 §层边界。
