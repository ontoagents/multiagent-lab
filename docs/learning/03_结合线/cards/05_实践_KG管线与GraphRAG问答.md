---
module: 知识体系
topic: 实践·平台实证——自研 KG 管线与 GraphRAG 问答
desc: internal/kg 两步抽取与社区摘要、KB graphrag 三步检索、伴生检索三臂——GraphRAG 模式的零依赖落地
synced: 2026-10-28
kind: practice
domain: bridge
prereq: ["01_概念_知识三形态对照.md"]
sources:
  - backend/internal/kg/kg.go
  - backend/internal/kg/pipeline.go
  - backend/internal/kg/community.go
  - backend/internal/kb/graphrag.go
  - REQ-287
  - REQ-368
check:
  - KG 抽取的 LLM 主路径与规则兜底各在什么条件下生效？
  - GraphRAG「局部问答」与「全局问答」分别走哪条管线？
---

**抽取管线**（internal/kg，D-O15/REQ-110 自研）：LLM 主路径分窗并发+两步抽取（pipeline.go），LLM 不可用/解析失败回退内置规则抽取（「A 是 B」「A 的 B」句式）——保证 graphrag 模式**零外部进程依赖**（验收 22 零 venv）。独立成包是防 chat→kb 依赖环。

**检索管线**（kb/graphrag.go 三步）：向量命中→多跳邻居扩展→claims 拼上下文；**全局问答**走社区摘要（community.go Louvain 社区检测+map-reduce 摘要，globalqa.go）——局部问题查实体邻域，全局问题（「这个库主要讲什么」）查社区聚合。混合检索（hybrid.go）并轨词法与向量。

**伴生检索三臂**（REQ-287，本体资产的检索底座）：FTS5 词法兜底+定义向量+RRF 融合，门控 τ 区间注入；REQ-358 伴生图增强（多跳关系上下文）、REQ-368 双时序（vt 有效性×tt 事务时间）让知识带版本地被检索。

**诚实边界与教训**：轻量实现如实标注（能力边界诚实是平台纪律）；知识卡问答类基准先冻结验证集再上线检索（58 号 §11.2——防自建基准自证循环）；为无消费方的数据预建索引是反模式（真实使用信号触发）。
