---
module: 知识体系
topic: TBox 与 ABox 分层
desc: 术语层与断言层的分工——平台 spec 的双层映射、装载顺序依赖与治理面差异
synced: 2026-10-22
kind: concept
domain: ontology
prereq: ["03_RDF_OWL_SPARQL三件套.md"]
sources:
  - platform-knowledge/04_本体/91_本体_专业概念手册.md
  - ontology-service/internal/seed/examples/med_common.json
  - docs/03_本体_需求文档.md §2.33
check:
  - 为什么实例断言依赖 TBox 先在？平台哪条校验体现这个依赖？
  - TBox 与 ABox 的治理（审计/质量）关注点有什么不同？
---

**TBox**（Terminology Box，术语层）定义「世界上有什么类、类之间什么关系」：概念、概念层级、对象属性（关系）及其定义域值域、数据属性、公理。**ABox**（Assertion Box，断言层）陈述「具体事实」：某个实例属于某概念、某实例与某实例有某关系、某实例的某属性值。

平台的 spec 结构是严格的 TBox/ABox 分层：`concepts`/`relations`/`data_properties` 是 TBox，`instances`（含 attributes 与关系断言）是 ABox。这个分层带来两条硬约束：

- **装载顺序**：ABox 断言依赖 TBox 术语先在——实例的类型与关系断言引用的概念/关系必须已定义；spec 校验（Validate）对悬空引用直接报错（如「实例类型引用了未定义概念」），种子文件的组织也总是 TBox 段在前。
- **治理差异**：TBox 变更是模型级变更（影响所有实例的语义，走版本/发布状态机与影响分析 REQ-278）；ABox 变更是数据级变更（伴随生图沉淀与对账，见 [三平面架构](05_三平面架构.md) 的运行平面）。看 `med_common.json` 种子即可印证：前段是百余概念分类树与九条关系，后段是一百七十个实例断言——两层各自独立演化。
