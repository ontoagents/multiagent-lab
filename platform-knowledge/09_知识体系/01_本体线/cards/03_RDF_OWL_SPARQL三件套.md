---
module: 知识体系
topic: RDF/OWL/SPARQL 三件套
desc: 三元组数据模型、语义层与查询语言的分工，以及平台内的落点（TTL 导出与只读查询白名单）
synced: 2026-10-22
kind: concept
domain: ontology
prereq: ["01_本体与知识工程谱系.md"]
sources:
  - platform-knowledge/04_本体/91_本体_专业概念手册.md
  - runtime-manager/internal/facade/sparql.go
  - REQ-151
  - REQ-366
check:
  - OWL 在 RDF 之上多给了什么？
  - 平台 SPARQL 面为什么只放行 SELECT/ASK？
  - ASK 查询适合回答哪类 CQ？
---

三件套是语义技术栈的最小可用组合，分工如下：

- **RDF**：数据模型——一切皆三元组（主语 谓语 宾语），用 IRI 全局命名。平台里本体的 TTL 导出就是 RDF 的一种序列化。
- **OWL**：RDF 之上的**语义层**——在三元组之外给词汇赋予形式化含义（类层级、属性定义域值域、disjointWith 等公理），使推理（子类传导、一致性检查）成为可能。平台的公理保留层（axioms，REQ-269）与数据属性声明（REQ-268）导出时即回写为 owl:disjointWith 等结构。
- **SPARQL**：查询语言——对 RDF 图做模式匹配。平台面是 `sparql_query` 工具与 SPARQL 工作台，词法级只读白名单只放行 SELECT 与 ASK（REQ-151 起，REQ-368/T1 补 ASK）：SELECT 出表格行，ASK 出布尔判定——「某个本体里是否存在满足某模式的事实」这类 CQ 用 ASK 最直接。

三件套在平台的关系要抓一个要点：**编辑态与查询态是两个东西**——智能体与用户编辑的是 spec 资产（归一化形态），查询的是运行引擎里的 RDF 图；TTL 导出与引擎装载是投影（见 [spec 归一化形态](07_决策_spec归一化形态.md) 与 [三平面架构](05_三平面架构.md)）。
