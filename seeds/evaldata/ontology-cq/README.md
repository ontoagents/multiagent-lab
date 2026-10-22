# ontology-cq — 本体 CQ 基准集（REQ-345/M145 建，N2 扩容 50 条，REQ-367/M165 评测纵深 60 条分层 r2）

种子本体的能力问题（CQ）基准集，文件形态（沿 backend kb questions.json 先例：id/type/q/gold 结构+note 说明）。

- 每份 `<种子名>.json`：`{note, ontology, revision, cqs: [{id, cq, gold, type, kind, difficulty}]}`
- **规模（r2，REQ-367/M165 2026-10-22）：5 种子 × 12 条 = 60 条**；r1=REQ-345/M145 + N2 扩容（5×10=50，三层均衡 4R+3E+3I）
- `gold` = 答案依赖的概念/关系名（与种子 spec 概念/关系一一对应，CQ 可答性的结构性前提；I 层 CQ 另有实例断言锚点——CQ 中的实例在种子 instances 实际承载）
- `type` 沿 81 号 CQ4OE 标注口径（R=关系层 / E=实体概念层 / I=实例断言层）；平展分类种子（defects/orgs）的 E 层为 schema 覆盖面问题，层级种子（gene_core/med_common/failure）为分类构成问题

## 分层标注口径（r2 双维，REQ-367/M165）

- **`kind` 题型（答案获得路径）**：
  - `fact` 事实型——答案为 schema 元素本身（领域概念枚举/分类构成枚举）；
  - `relation` 关系型——答案经一条显式关系获得（单跳，含实例锚定的单跳查询）；
  - `inference` 推理型——答案需 ≥2 个模式的联接、跨节点链路或类层层级归约（如「过热故障这类故障需要采取哪些维护措施」= 类型锚定 → 引起 → 对应 三模式联接）。
- **`difficulty` 难度（查询构造难度）**：
  - `easy`——单跳直查或纯枚举；
  - `medium`——反向查询（宾语反查主语）、实例锚定、跨概念归纳；
  - `hard`——多跳链路、类层归约下的多模式联接。
- **r2 标注分布（诚实记录）**：fact 15 / relation 31 / inference 14；easy 29 / medium 19 / hard 12。既有 50 条（r1）无推理型——r2 增量 10 条全部为推理型/难档（**增量扩容不回溯改数**约束下填补分层空格的诚实选择，既有 CQ 原文与 gold 零变更仅补标注）。`easy × inference` 为空格：推理型题天然不易为 easy——难度与题型两维相关性本身就是分层的洞察产出，不做人为凑格。
- **四场景映射（检索/构建/校验/推理）**：检索=fact+relation 的既有元素覆盖面（schema 覆盖与单跳可达）；构建=gold 缺口信号（跨域/低对齐即构建输入，REQ-343 CQ 驱动扩展消费同源）；校验=relation 层关系语义与 I 层实例断言锚点一致性；推理=inference 档（多跳链与层级归约）。场景由维度组合承载，不单设第三标注维。

## 评测口径声明（REQ-352 收编；报告 metric_definition 字段同源随行）

- **翻译率**：LLM 翻译产出非空 SELECT 查询即计通过（瞬时故障重试一次）；
- **白名单率**：词法级只读校验（首关键词 SELECT + 变更关键字黑名单；字符串字面量/IRI/注释不误伤）；
- **gold 对齐率**：查询文本引用 gold 概念/关系 URN（任一命中即对齐）——**非答案内容比对**；跨域本体该率低为预期负信号非缺陷（基准集按源种子域编写）；全枚举型 CQ 不引用具体概念 IRI，与该口径结构性不适用（REQ-348 信号边界，信号只报告不做硬门禁沿 M43 先例）；
- **分层跑分**：报告 `by_difficulty`/`by_kind` 各档独立三率（REQ-367）；`metric_definition.dataset_rev` 记录本报告所用数据集版本，`generator_ver` 记录评测器口径版本（r2-req367）——版本对比消费这两个字段防口径漂移误读；
- **不回溯纪律**：r2 不改动 r1 既有 50 条的题面/gold/type/标注判定结果（31 号挂账既有口径不变）；后续 revision 演进只增不改。

## 运行与验证

- 与 `ontology-service/internal/seed/examples/<种子名>.json` 的 spec.CQ 由一致性测试锁定双端同步（含双维标注齐备/枚举合法/每数据集分档覆盖/总量 ≥50）：
  `go test ./internal/evaldata/ -run TestCQDatasetConsistency`（无 build tag 始终运行）
- CQ 信号评测（LLM 翻译+白名单+gold 对齐）：`go test -tags eval ./internal/evaldata/ -run TestCQSignal -v -timeout 30m`（60 条真机 ≈15 分钟——超出 go test 默认 10 分钟，**必须带 -timeout**）
- 「测我」服务化（REQ-346 起）：`POST /api/ontology/cq-benchmark/run {ontology_id, dataset}` → 202 job → 报告落 artifact `cq-benchmark-report`（runs 追加式历史）；MCP 工具面 `cq_benchmark_datasets`/`cq_benchmark_run`（REQ-366 T5）透传同源数据
- **真机基线（2026-10-17 r1，GLM，50 条）**：翻译 100% · 白名单 100% · gold 对齐 94%（47/50）；3 条失败均为全枚举型 E 层 CQ（口径结构性不适用，见上）
- **真机基线（2026-10-22 r2，GLM，gene_core 12 条）**：翻译 12/12 · 白名单 12/12 · gold 对齐 12/12 全绿（含 2 条新增推理型/难档；分层报告 by_difficulty easy7/medium3/hard2、by_kind fact3/relation5/inference4 全档对齐）——「测我」可复跑实锤

种子 CQ 标注为开发者自查非三人审核级（REQ-345 诚实边界）；管理面已随 REQ-346 v1 交付（cqbench 清单/「测我」/报告历史）。
