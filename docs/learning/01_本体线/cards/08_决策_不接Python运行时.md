---
module: 知识体系
topic: 决策·不接 Python 运行时（D-O15）
desc: semantica worker 的引入与退役——零 venv 一键启动、Go 原生 TTL 导出对齐，以及边界里保留了什么
synced: 2026-10-22
kind: decision
domain: ontology
prereq: ["05_三平面架构.md"]
sources:
  - docs/03_本体_需求文档.md §5
  - platform-knowledge/04_本体/08_调研预研/方法论与理论/19_本体_semantica集成方案.md
  - REQ-110
check:
  - D-O15 反转了什么、保留了什么？
  - 为什么「导入导出用 Python、运行时不用」是可以接受的边界？
---

**背景**：本体抽取与 RDF 生态的成熟库多在 Python（rdflib 等）。早期方案曾引入 semantica worker（Python 侧车进程）承载 KG 抽取，`run-dev.sh` 需要维护 venv。

**反转**：D-O15 拍板**不接 Python 运行时**——①部署纪律升级为「一条命令启动零 venv」（验收 22 条明文）；②KG 抽取/消费/审计自研内置 Go 实现（D-O15 反转批，REQ-110）；③semantica worker 代码归档休眠**勿重新启用**（tools/semantica-worker/）。配套工程：TTL 导出用 Go 原生导出器字节级对齐（REQ-235⑥ 起脱离 python），替代原 python sidecar 导出。

**保留的边界**：`python3+rdflib` 并没有被赶尽杀绝——**本体导入（OWL/TTL 解析）与导出校验仍走 python sidecar**（可选依赖，缺了只影响导入导出不阻断主链路）。这条边界的教育意义：D-O15 反对的是**运行时对 Python 进程的依赖**（生命周期/资源/跨平台矩阵都会被它拖复杂），而**批处理边界的工具借用**（解析一次文件）在成本收益上是另一笔账——决策反对的不是语言，是依赖形态。

**后果**：平台主链路（对话/构建/运行/消费）在无 Python 的机器上完整可用；代价是部分 RDF 生态高级能力（如 OWL 推理机）需要 Go 侧自研或引入（owlrl 检查档走的是「声明+导出后外部校验」的轻路线）。
