# ontology-cq — 本体 CQ 基准集（REQ-345/M145，N2 扩容 2026-10-17）

种子本体的能力问题（CQ）基准集，文件形态（沿 backend kb questions.json 先例：id/type/q/gold 结构+note 说明）。

- 每份 `<种子名>.json`：`{note, ontology, cqs: [{id, cq, gold, type}]}`
- **规模：5 种子 × 10 条 = 50 条，三层均衡 4R+3E+3I**（86 号 N2 扩容——原 3 条/本体且全 R 层，单条回归失效=33% 波动、E/I 层稀薄，作回归基准统计强度不足；扩容后单条失效=10%）
- `gold` = 答案依赖的概念/关系名（与种子 spec 概念/关系一一对应，CQ 可答性的结构性前提；I 层 CQ 另有实例断言锚点——CQ 中的实例在种子 instances 实际承载）
- `type` 沿 81 号 CQ4OE 标注口径（R=关系层 / E=实体概念层 / I=实例断言层）；平展分类种子（defects/orgs）的 E 层为 schema 覆盖面问题，层级种子（gene_core/med_common/failure）为分类构成问题
- 与 `ontology-service/internal/seed/examples/<种子名>.json` 的 spec.CQ 由一致性测试锁定双端同步：
  `go test ./internal/evaldata/ -run TestCQDatasetConsistency`（无 build tag 始终运行）
- CQ 信号评测（LLM 翻译+白名单+gold 对齐）：`go test -tags eval ./internal/evaldata/ -run TestCQSignal -v`（扩容后 50 条，真机运行时长约按条数线性增长）

种子 CQ 标注为开发者自查非三人审核级（REQ-345 诚实边界）；管理面已随 REQ-346 v1 交付（cqbench 清单/「测我」/报告历史）。
