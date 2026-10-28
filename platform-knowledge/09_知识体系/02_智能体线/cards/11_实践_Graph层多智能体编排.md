---
module: 知识体系
topic: 实践·Graph 层多智能体编排
desc: agent_as_tool 主推/transfer 对照/workflow 五模式/后台委派——多个智能体怎么组队与流转
synced: 2026-10-28
kind: practice
domain: agent
prereq: ["03_概念_五层域总纲.md", "04_概念_外部智能体平台观察.md"]
sources:
  - platform-knowledge/02_智能体/56_Graph_设计实现与演进.md
  - platform-knowledge/02_智能体/08_调研预研/方案与设计分析/78_多智能体与子智能体执行支持与调度编排分析_20261013.md
  - backend/internal/chat/assembler.go
check:
  - 为什么 agent_as_tool 是主推而 transfer 只是对照？
  - 协调者委派成员时，成员能看到什么上下文？
---

**成员装配三轨**：①**agent_as_tool（主推）**——项目成员全量装配后包装为工具（function name=成员名），协调者按描述路由调用；ADK 只回传末条文本（44 号实测定案，子 Agent 中间过程仅事件流转发）；②**transfer（对照保留）**——ADK 内建转移，官方 NOT RECOMMENDED，UI 黄色风险标注（REQ-208 治理定案）；③**workflow（结构化）**——workflow_mode 五值：sequential/parallel/loop（adk 官方原语装配，loop MaxIterations 上限+告警）/free 自由协作/plan_execute 计划执行（REQ-455 三角色：Planner→Executor→Replanner），workflow.node 事件承载节点级进度与 HITL 定位（REQ-205）。

**后台委派**（REQ-208）：delegate_background 六工具让协调者把任务交给子会话异步执行，steer 插话（轮界接续语义——插话在当前轮结束后被消费）、collect 收摘要、interrupt/continue 治理。78 号六实证里最要紧的一条：**成员上下文=仅 tool arguments 里的单条 request**（无历史无共享列表）——任务描述质量决定成员产出上限，写委派 prompt 要像给新同事写任务单。

**三色成员**：内置模型成员（eino-adk 全量能力）+外部 CLI 成员（kind=cli）+A2A 远程成员（kind=a2a），能力画像路由（REQ-429）按描述自动选人。诚实边界：成员错误=终止整个 run（韧性缺口挂 REQ-385）。
