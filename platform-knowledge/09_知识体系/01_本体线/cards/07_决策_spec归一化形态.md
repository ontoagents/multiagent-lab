---
module: 知识体系
topic: 决策·spec 归一化形态（为什么编辑态不是 OWL/TTL）
desc: 自研 spec_json 归一化形态的背景、备选、取舍与后果——校验管线、编辑保护三层与导出投影
synced: 2026-10-22
kind: decision
domain: ontology
prereq: ["04_TBox与ABox分层.md"]
sources:
  - pkg/ontology/spec/spec.go
  - docs/03_本体_需求文档.md §2.33
  - platform-knowledge/04_本体/08_调研预研/方法论与理论/31_本体建模规范深入研究报告.md
  - REQ-292
  - REQ-293
check:
  - 直接以 TTL 为编辑事实源会破坏哪些平台能力？
  - 编辑保护三层分别防什么？
---

**背景**：本体事实源用什么形态？OWL/TTL 表达力最强，但作为**编辑态**有硬伤：语法自由度大（同一语义多种写法）、LLM 生成易出非法结构、校验错误难定位到人话。

**备选与取舍**：拍板自研 `spec_json` 归一化形态（concepts/relations/data_properties/instances/axioms 五段、字段受限、校验器同源共用）作为唯一编辑事实源；OWL/TTL 降为**导出投影**（importer 保真导入、exporter 原生导出）。三条收益：①校验器可以输出 path 级人话定位（`concepts[2].parents[0]: 引用了未定义概念`）直接回喂 LLM 修正循环；②六条构建路径全部收敛到同一写入口与同一校验；③版本/发布/快照/影响分析都锚在 spec 版本上。

**后果与配套**：归一化形态必须补上「直接编辑 OWL 时代已被表达力掩盖」的工程问题——于是有了编辑保护三层：①对象级 overlay（人工编辑与机器重算分离，REQ-292）；②If-Match 文档级乐观锁（防陈旧覆盖，REQ-293②）；③strict 质量门禁（错误级命中阻断保存，REQ-156）。**严格语义不由 LLM 生成**（D-O22）是同一决策家族的边界条款：公理由真实资产导入保真承载。学习要点：归一化形态是「让 LLM 时代的知识工程可工程化」的地基——表达力换确定性，是本项目最重要的单项取舍之一。
