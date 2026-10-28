---
module: 知识体系
topic: 实践·Context 层供给（预算/压缩/剪枝/召回）
desc: 模型每轮真正看到什么——context_mode 三档预算、工具轮次保全、压缩持久化与召回链
synced: 2026-10-28
kind: practice
domain: agent
prereq: ["08_实践_Harness装配链.md"]
sources:
  - platform-knowledge/02_智能体/54_Context_设计实现与演进.md
  - backend/internal/chat/context.go
  - REQ-201
  - REQ-210
check:
  - compact/standard/full 三档各是什么预算策略？默认哪档？
  - 为什么工具调用轮次必须保全？悬空 ToolCalls 怎么处理？
---

**预算三档**（context_mode）：compact≈6k tokens（长会话低成本）、standard≈24k（默认）、full 不限量（调试用）。预算不是简单截断——是「历史还原+治理」的组合拳。

**四步治理链**（context.go）：①**工具轮次保全**（mergeToolTurns）——消息与 run_event 按时间归并，assistant 的 ToolCalls 必须与 tool 结果配对（REQ-210 不变量），悬空调用补「结果未知」合成文本——否则下一次模型调用直接 400（API 要求 ToolCalls 必须有对应结果）；②**压缩**——超预算先应用持久化前缀摘要（八段式，按该 agent 的模型生成并落库复用），仍超则保留尾半预算+首条 user；③**剪枝**——旧工具结果可替换为占位（轮次结构不破坏）；④**召回注入**——知识库检索/伴生上下文在装配期拼入。

**常见坑**：会话「失忆」多半是压缩触发而非 bug——查 context_state 持久化段与展示级别的调试事件；改历史（REQ-409 消息操作）后必须保持轮次配对完整，否则恢复运行即悬空 ToolCalls 报错（REQ-454 已在恢复链路自动修补合成）。
