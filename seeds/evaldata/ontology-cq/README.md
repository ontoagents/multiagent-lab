# ontology-cq — 本体 CQ 基准集（REQ-345/M145）

种子本体的能力问题（CQ）基准集，文件形态（沿 backend kb questions.json 先例：id/type/q/gold 结构+note 说明）。

- 每份 `<种子名>.json`：`{note, ontology, cqs: [{id, cq, gold, type}]}`
- `gold` = 答案依赖的概念/关系名（与种子 spec 概念/关系一一对应，CQ 可答性的结构性前提）
- `type` 沿 81 号 CQ4OE 标注口径（R=关系层 / E=实体概念层 / I=实例断言层）
- 与 `ontology-service/internal/seed/examples/<种子名>.json` 的 spec.CQ 由一致性测试锁定双端同步：
  `go test ./internal/evaldata/ -run TestCQDatasetConsistency`（无 build tag 始终运行）
- CQ 信号评测（LLM 翻译+白名单+gold 对齐）：`go test -tags eval ./internal/evaldata/ -run TestCQSignal -v`

种子 CQ 标注为开发者自查非三人审核级（REQ-345 诚实边界）；管理面（清单/版本/来源）留 REQ-346 触发。
