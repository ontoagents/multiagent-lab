---
module: 知识体系
topic: 实践·Harness 层装配链与防护
desc: assembler 装配管线、hooks 防护链、审批 gating、verify 背压与沙箱——一次运行的系统怎么搭
synced: 2026-10-28
kind: practice
domain: agent
prereq: ["03_概念_五层域总纲.md"]
sources:
  - platform-knowledge/02_智能体/53_Harness_设计实现与演进.md
  - backend/internal/chat/assembler.go
  - backend/internal/tool/hooks.go
check:
  - fetchGuard/cmdGuard 各拦什么？拒绝后语义是「报错」还是「回喂」？
  - run_command 生效的三个前置是什么？
---

**装配链**（assembler 8 步）：每次会话运行前把 agent 配置装配成可执行的 Runner——模型连接解析→上下文模式→技能注入→工具面装配（内置+MCP+本体 facade 门控）→hooks 编织→审批策略→运行后端分发→事件汇挂接。看懂 assembler 就看懂了「一份 agent 配置如何变成一次受控运行」。

**四件防护**：①**HookChain**（tool/hooks.go）——fetchGuard（HTTP 白名单域）与 cmdGuard（命令黑名单正则）在工具执行前拦截，拒绝不是报错而是**结构化回喂模型**（模型可自纠换路）；②**审批 gating**（tool/approval.go）——danger 档工具（写类/删除类）执行前挂起等人工审批，会话级覆盖 agent 级；③**verify 背压**——agent 配置 verify_command 时运行后校验，失败反馈进循环；④**沙箱**——run_command 类工具经运行后端（宿主/docker）隔离执行，三前置状态在 UI 如实标注。

**常见坑**：Harness 配置散在侧板多个页签（REQ-219 归位），改了「能力」页签的 verify_command 却在「基本」页签找沙箱——先认层再找配置。外部 CLI 后端不走装配链（见决策卡 06），给 claude-code 成员配 hooks 不生效是预期行为不是 bug。
