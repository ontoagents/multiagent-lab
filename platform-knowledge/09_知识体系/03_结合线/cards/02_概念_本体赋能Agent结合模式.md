---
module: 知识体系
topic: 概念·本体赋能 Agent 的结合模式
desc: 四模式框架——GraphRAG/Ontology-as-Tools/混合推理/AgentO，各自回答什么问题
synced: 2026-10-28
kind: concept
domain: bridge
prereq: ["01_概念_知识三形态对照.md", "../../01_本体线/cards/05_三平面架构.md"]
sources:
  - platform-knowledge/04_本体/08_调研预研/方法论与理论/61_本体Agent开发上手指南.html
  - platform-knowledge/08_调研预研/58_核心知识体系提取可行方案.md §4.2
check:
  - GraphRAG 与 Ontology-as-Tools 的本质区别是什么（知识供给 vs 能力供给）？
  - 「本体如何运行起来」对应哪几种模式？
---

**四模式框架**（61 号《本体 Agent 开发上手指南》）：本体与 Agent 的结合不是一种技术而是一族模式——

| 模式 | 结合方式 | 回答的问题 |
| --- | --- | --- |
| GraphRAG | 本体/KG 作为**检索底座**喂给模型上下文 | 模型怎么「知道」领域知识 |
| Ontology-as-Tools | 本体操作封装为**工具**（查询/校验/演化） | 模型怎么「动手」用本体 |
| 混合推理 | 本体公理推理+LLM 判断**互补校验** | 答案怎么「可信」 |
| AgentO | 本体直接**驱动** Agent 的构建与行为规约 | 智能体本身怎么被本体化描述 |

**判别轴**：前三者是「本体赋能 Agent」（知识供给/能力供给/可信供给），AgentO 反过来「Agent 被本体赋能」（规约供给）。同一平台常多模式并存——模式选择由「答案需要多可信、知识更新多频繁」决定，不是越重越好。

**本平台的落点全景**：GraphRAG→KB 模块 graphrag backend+伴生检索（实践卡 05）；Ontology-as-Tools→facade 工具面（实践卡 04）；混合推理→owlrl 装载期检查+qualitygate（本体线卡 10 的延长线）；AgentO→运行时动态本体/对话伴生本体生长（01_运行时动态本体专题）的远期方向。
