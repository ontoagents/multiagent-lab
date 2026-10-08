---
module: 本体
topic: open-ontologies 深度调研（功能全景与技术原理）
desc: 122 个 MCP 工具的完整功能面 + 证书体系/RAG 切片/双时序/生命周期机制逆向 + 与 34 号的增量
synced: 2026-10-08
---

# open-ontologies 深度调研：功能全景与技术原理

> 状态：v1.0 ｜ 日期：2026-10-08 ｜ 一手材料：GitHub API + raw 文件级精读 `fabio-rovai/open-ontologies` @ `main`（3198 文件 / ~226 MB，沙箱无法 clone）
> 调研动因：开发者指令「github.com/fabio-rovai/open-ontologies 项目到底实现了哪些功能？补一份 open-ontologies 开源项目的深度调研」
> **与 34 号的关系（重要，先读）**：34 号《open-ontologies 借鉴与引入分析》是**产品集成视角**（v0.2，2026-09-20，当时 39 工具）——回答「我们能引入什么、怎么双轨集成」。本档是**技术能力视角**——回答「它到底实现了什么、原理与实现方式是什么」。两者互补不替代；34 号的集成拍板（独立双轨 / REQ-100 / M8.5）不变，本档只做能力与原理的**增量补齐**。
> 关联：REQ-100（OpenOntologies 双轨集成，✅ M8.5 已交付）、REQ-78（双轨 TTL 互通，冻结）、REQ-39 系（本体运行时/MCP）、04 号《运行》、05 号《消费与审计》

---

## 0. 一句话结论

**它已不是「一个本体 MCP Server」，而是「一个自带形式化验证内核的本体工程与治理引擎」**——单二进制 Rust（无 JVM），把「推理 → 出证书 → 独立内核复核」这条链做成产品主叙事，并在此之上长出变更治理（Terraform 式 plan/apply/drift/rollback）、RAG 切片蕴含保持、双时序、对齐稳定匹配等一整套能力。**对我们最直接的价值：它把 05 号《消费与审计》里"缺的"整条可信链——溯源、证书、可复核——做成了可借鉴的完整先例。**

## 1. 关键事实（2026-10-08 快照）

| 项 | 值 |
| --- | --- |
| 仓库 | `fabio-rovai/open-ontologies`（受 [Tesseract Semantics](https://tesseractsemantics.com) 商业支持；引擎 MIT 永久） |
| 版本 | **Cargo.toml `version = "2.1.0"`**（我们 M8.5 集成的是 v2.0.1） |
| Star / Fork | **930★ / 123 fork / 19 open issues / 12 watchers** |
| 语言 / 许可 | **Rust（edition 2024）/ MIT** |
| 创建 / 末次推送 | 2026-03-09 / **2026-10-01**（活跃） |
| 仓库体积 | 3198 文件；`size` 字段 232,020 KB（≈226 MB）；CHANGELOG 自述 `benchmark/` 检出后 23 GB、`studio/`+`case-studies/` 另 5.5 GB |
| 定位（自述） | "Plan a change to a production ontology. See every consequence before you apply it. Then give the reviewer a proof that they can check without trust in you." |
| 论文 | **arXiv:2605.09184**《Open Ontologies: Tool-Augmented Ontology Engineering with Stable Matching Alignment》；**arXiv:2605.09168**《CIVeX: Causal Intervention Verification for Language Agents》（均 Fabio Rovai, 2026） |
| 工具面 | **122 个 MCP 工具**（`docs/tool-reference.md` 自述；由 `tests/tool_reference_test.rs` 强制"添加工具必须补文档行"）；SKILL.md 写 110（未同步），我们 15 号旧记录 119（v2.0.1 时） |
| 工具档位 | **10 个 tool profile**：`full` / `authoring` / `validation` / `reasoning` / `alignment` / `governance` / `data` / `planning` / `retrieval` / `evaluation` |
| 形式化内核 | **四套**：`lean/`（主，Lean 4 v4.33.1，核心 Lean 无 Mathlib）、`isabelle/`（独立第二形式化）、`rocq/`（第三，Rocq 9.2）、`dafny/`（RuleTable 形式化） |
| 案例 | `case-studies/` **22 个**（health/AV/钢铁/水务/自然治理/保险/基金/跨境技能/数据中心/文化遗产…） |
| 工程纪律 | 18 份 `docs/decisions/` 决策档（编号有洞、`decision_numbers_test.rs` 强制）；README 受 **ASD-STE100 简化技术英语**约束并由测试强制（`readme_simplified_english_test.rs`） |

**规模感**：`src/` 下 Rust 单文件即 200KB 量级——`server.rs` 233KB、`tableaux.rs` 231KB、`tptp.rs` 207KB、`reason.rs` 204KB、`main.rs` 176KB、`shacl.rs` 168KB、`temporal.rs` 147KB。这是一个体量远超"教学工具"的工程。

---

## 2. 它到底实现了哪些功能（核心回答）

按 10 个 tool profile 归口，122 个工具构成**九个能力面**。逐面给出「做什么 + 代表工具」。

### 2.1 核心与读写（floor 工具，每个 profile 都带）

`onto_status`（健康 + 载入三元组数）/ `onto_validate`（RDF/OWL 语法）/ `onto_load`（文件或内联 Turtle）/ `onto_query`（SPARQL）/ `onto_stats`（三元组/类/属性/实例计数）/ `onto_clear`。
外围：`onto_convert`（turtle/ntriples/rdfxml/nquads/trig 互转）、`onto_diff`、`onto_save`、`onto_pull`（远程 URL 或 SPARQL 端点取本体）、`onto_push`（推到 SPARQL 端点）、`onto_import`（递归解析 `owl:imports` 链）。
**缓存与仓库层**：`onto_cache_status/list/remove`、`onto_recompile`、`onto_unload`（编译缓存 = 源文件 → N-Triples 的落盘产物，带 TTL/auto_refresh 配置）、`onto_repo_list/load`（按 `[general] ontology_dirs` 配置的仓库目录）。
**版本层**：`onto_version`（命名快照）/ `onto_history` / `onto_rollback`。

### 2.2 数据管线（data profile）——结构化数据 → RDF

| 工具 | 功能 |
| --- | --- |
| `onto_map` | 检视数据文件 schema + 已载本体 → 生成映射配置 |
| `onto_ingest` | **CSV / JSON / NDJSON / XML / YAML / XLSX / Parquet** → RDF 三元组并载入 |
| `onto_import_schema` | 关系库 schema → OWL 类/属性/基数（需 `postgres`/`duckdb` feature） |
| `onto_sql_ingest` | 对 PostgreSQL/DuckDB 跑 SQL `SELECT` → RDF（DuckDB 借扩展可联邦 CSV/Parquet/JSON/HTTPFS） |
| `onto_sql_sync_state/reset/states_list` | **CDC 水位线**记录（增量同步的 sync_key 游标） |
| `onto_shacl` / `onto_shacl_check` | SHACL 形状校验 / 提案形状的干跑结构检查 |
| `onto_vocab_check` | **闭世界词表检查**：数据用的每个谓词与 `rdf:type` 是否真在本体里声明 |
| `onto_extend` | 一条龙：ingest → SHACL → reason |
| `onto_induce` | **「一表进，一本出」**：从一张数据表归纳出 OWL 类 + 类型化属性 + SHACL 形状 + 装载映射，**每行附证据** |
| `onto_shape_induce` / `onto_shape_combinatorics` | Kastor 式数据驱动 SHACL 形状归纳 / 枚举类的属性组合格（K-CAP 2025 #36） |
| `onto_owl_shacl_coevolve_check` / `_incremental` | OWL+SHACL 联合演化校验及增量版（K-CAP 2025 #33） |

> `onto_induce` 的「一表进一本出 + 每行附证据」是**把"从零建本体"做成了确定性管线**，与我们 58 号「从零建本体深化优化」正面同题。

### 2.3 推理与形式化（reasoning profile）——**全项目最重的一块**

**前向链推理**：`onto_reason`（`rdfs` / `owl-rl` / `owl-rl-ext` profile，物化推断）、`onto_reason_incremental`（只推新增三元组的后果，不重算全闭包）。
**描述逻辑**：`onto_dl_check`（DL tableaux 判 `subClass ⊑ superClass`）、`onto_dl_explain`（类不可满足的 clash 轨迹）、`onto_classify_el`（OWL-EL 片段分类，#30）、`onto_dl_refute`（`src/dl_refute.rs`）。
**规则即数据**：`onto_rules_import`（**SWRL** 从其语法、**RIF Core** 从其 XML 读入 → Horn 规则表）。
**证书**：`onto_reason --certificate DIR` 写推导证书；`onto_trace_label`（对推理轨迹逐步贴标签：`entailed` / `not_entailed` / `outside_the_fragment`，且**「局部可靠但前提不在库里」单独一词**，永不合并）。
**解释**：`onto_justify`（公理定位，见 §3.5）、`onto_provenance`（溯源半环）。
**一阶导出与外部证明器**：`onto_fol_export`（TPTP / Common Logic / SMT-LIB / LADR）、`onto_fol_prove`（跑一阶证明器并**回读推导重检**）、`onto_fol_model`（找有限模型并**检查它**）。
**边界自省**：`onto_dlp_boundary`（**在信任一个推理结果前，先问规则引擎到底"看得见"你的哪些公理**）。

### 2.4 本体自检与质量（validation profile）

`onto_defects`（**判数据之前先判本体自身**）、`onto_lint`（缺标签/注释/domain/range）、`onto_support_check/report/verdict`（**图里的断言是否被它引用的来源支撑**——unsourced-claim 率 + 支撑率）、`onto_provenance`（同上）。

### 2.5 生命周期治理（governance profile）——Terraform 式

`onto_plan`（diff + **blast radius** + **risk score** + 锁 IRI）→ `onto_enforce`（设计模式：`generic`/`boro`/`value_partition`/自定义 SPARQL）→ `onto_apply`（`safe` 写差量 / `migrate` 加 `owl:equivalentClass` 桥）→ `onto_monitor`（SPARQL watcher + 阈值告警，动作 `notify`/`block_next_apply`/`auto_rollback`/`log`）→ `onto_drift`（版本对比 + **重命名检测** + drift 速度 + 自校准置信）→ `onto_lineage`（只追加审计轨迹）。
配套：`onto_lock`、`onto_conservative_check`（**保守性检查**，见 §3.8）、`onto_policy_register/list/check`（ARGOS 式策略，#40 ISWC 2025 WOP）、`onto_pack` / `onto_unpack`（**包自带证明**，见 §3.8）。

### 2.6 对齐与本体互操作（alignment profile）

`onto_align`（**6 个加权信号**：标签相似 / 属性重叠 / 父类重叠 / 实例重叠 / 限制模式 / 图邻域 → 输出 `owl:equivalentClass`/`skos:exactMatch`/`rdfs:subClassOf` 候选）、`onto_align_feedback`（接受/拒绝以**自校准权重**）、`onto_align_fuzzy` + `onto_align_flora`（FLORA 模糊逻辑裁决，#38 **ISWC 2025 最佳论文**）、`onto_eval_alignment`（OAEI 式 P/R/F1，#31）、`onto_ossie_import`（Apache Ossie 本体文档 → OWL 2 DL + SHACL）。
临床向：`onto_crosswalk`（ICD-10/SNOMED/MeSH 映射）、`onto_enrich`（加 `skos:exactMatch`）、`onto_validate_clinical`。

### 2.7 检索与 RAG（retrieval profile）——**与我们 05 号消费审计直接相关**

| 工具 | 功能 |
| --- | --- |
| `onto_segment_retrieve` | 取种子 IRI 的 **TBox 切片邻域**以支撑 LLM 推理（#34 SEMANTiCS 2025 GrOWL-RAG） |
| `graph_projection_entailment_check` | **切片是否仍支撑答案依赖的断言**（逐 claim，带 Lean 证书，见 §3.6） |
| `graph_projection_lossy_check` / `onto_closure_diff` | 投影切片的有损审计 / **不提供目标时**报告投影保住了哪些结论 |
| `onto_module_extract` | **签名上的模块**（不是切片）：语法局部性可证的最小公理子集，**全本体在这些 IRI 上的每条蕴含仍是子集的蕴含**（Cuenca Grau et al. JAIR 2008 定理） |
| `onto_communities` | 实体图社区检测 → 每社区给**骨架**（规模/度数最高成员/内部关系/桥接） |
| `onto_extract_scaffold` / `_validate` | 模式引导的**结构化抽取脚手架**（OntoGPT SPIRES 的 MCP 原生化，#28）+ 校验 LLM 抽取结果 |
| `onto_embed` / `onto_search` / `onto_similarity` / `onto_hnsw_build` | 文本 + **庞加莱（Poincaré）结构**双嵌入；自然语言 → 最相似类；cosine + 庞加莱距离 + 乘积分；HNSW 索引（`ef_construction`/`ef_search` 显式） |
| `onto_cq_run` / `onto_verify_cq` / `onto_cq_verdicts_list` | **能力问题（CQ）批量运行** + 人工/LLM 判定落库（#29、#39 ISWC 2025 Lippolis） |
| `eval_rag` / `eval_rag_mmrag` | mmRAG 基准打分（#41 ISWC 2025） |
| `borderline_partition` / `borderline_record_verdict` | 边界对划分 + 编排者判定（#37 NORA NeurIPS 2025） |

> `onto_cq_run` + `onto_verify_cq` 与我们 REQ-248（CQ 一等公民）、REQ-255（CQ→SPARQL 验收）同题——它把 CQ 做成了**有判定的可回归基准**。

### 2.8 双时序（跨 data/retrieval）

`onto_temporal_query`（只在时间范围内命名图上跑 SPARQL 图模式：**某有效时刻、某记录时刻图上说了什么**）、`onto_temporal_snapshot`（某时刻哪些命名图在范围内、其余为何排除）、`onto_temporal_conflicts`（**声明了重叠有效期的**不相交冲突，与非重叠分列）。见 §3.7。

### 2.9 动力学与规划（planning profile）

`onto_action_register/list/applicable/apply/apply_concurrent`（注册动作 schema，**BC+ 并发动作原子 tick**，#43）、`onto_invariant_register/list/check/remove`（BC+ 静态因果律 / SPARQL ASK 不变量）、`onto_default_register/apply`（BC+ 默认值律）、`onto_plan_compile_pddl` + `onto_plan_classical`（**编译 PDDL 域调用 Fast Downward 子进程**，#50、#45）、`onto_plan_validate`（**不碰真库**地校验候选计划）、`onto_certify_action`（CIVeX 式因果证书）、`oo-matcert`（矩阵乘积证书，`MatCert.mul_of_check`）。

### 2.10 可扩展面（生态）

`.claude-plugin/` + `SKILL.md`（Claude 技能）、`.mcp/server.json`（MCP 注册）、`community/registry.json`（社区本体包）、`skills/community/`（教 agent 串 `onto_*` 的工作流配方）、**伴侣 MCP server**（OpenCheir：对本体工程会话施加"保存后必须校验/推送前必须版本化"治理）、**WASM 插件**（`wasmi` 沙箱内进程内跑社区工具）、Studio（Tauri 2 + React 19 + Tailwind 4 桌面壳：虚拟化本体树 / AI 聊天 / Protégé 式属性检查器 / lineage 面板）、Obsidian 插件。

---

## 3. 技术原理与实现方式

### 3.1 技术栈与存储

| 层 | 实现 |
| --- | --- |
| RDF/SPARQL | **Oxigraph 0.5**（`[patch.crates-io]` pin 到 `fabio-rovai/oxigraph` 某 revision——为拿到 `Store::snapshot`：一个不加锁、固定单一状态的读视图，见 decision 0010） |
| 状态/lineage/反馈/向量 | SQLite（`rusqlite` bundled） |
| MCP | `rmcp` 1.4（`transport-io` + `transport-streamable-http-server`） |
| HTTP | `axum` 0.8 + `tower-http`（CORS） |
| 嵌入 | `tract-onnx` + `tokenizers` + `instant-distance`（HNSW），可选 `turbovec`（2–4 bit 量化，可增量增删） |
| 插件 | `wasmi`（纯 Rust WASM 解释器 + fuel 计量） |
| 数据 | `csv`/`quick-xml`/`serde_yaml`/`calamine`(XLSX)/`arrow`+`parquet`；`sqlx`(postgres)/`duckdb` 可选 |
| 测试 | `proptest`（**专测证书边界**——TSV 载任意 RDF 项，正是"手写例子过、对抗例子伪造推导"的形状） |

**默认 in-memory**：持久化需显式 `OPEN_ONTOLOGIES_STORAGE_MODE=persistent` + `--data-dir`（README 自己点名的两个坑：忘设则 `load`→`reason` 从空库开始、什么都没证；`--data-dir` 是 flag 不是环境变量，漏了就写进 `~/.open-ontologies`）。

### 3.2 推理体系（规则表 + tableaux 双路）

- **前向链**：基于 Horn 规则表，**是 OWL 2 RL profile 78 条规则中的 29 条**。29/78 这个数字被**反复、显式**地声明（`onto_dlp_boundary`、explanation、justify 均强调「这是本引擎在这张规则表下的主张」）。
- **17 条能推出 `false` 的 OWL 2 RL 规则中，检测 10 条**；其中只有 `cax-dw` 一条在 `lean/` 里有语义条件、也只有它会被写成 refutation。
- **tableaux**：`src/tableaux.rs`（231KB）实现 **SHIQ** 描述逻辑推演，`onto_dl_check`/`onto_dl_explain` 用它；TBox 无个体不可满足的情形只有 tableaux 看得见（规则路看不见），**且 tableaux 的答案不带证书**。
- **OWL-EL** 单独分类器（`src/classify_el.rs`）。

### 3.3 证书体系——本项目的"心脏"（这是最值得抄的架构）

**证书 = 两个 TSV 文件**（示例演示中合计 1.3 KB）：

```
asserted.tsv     你的主张：   <ex:myCup> <rdf:type> <ex:Espresso>
derivations.tsv  每条一行：   规则  结论  该结论的前提
                 rdfs9  <ex:myCup> <rdf:type> <ex:Coffee>  <ex:myCup> <rdf:type> <ex:Espresso>  <ex:Espresso> <rdfs:subClassOf> <ex:Coffee>
```

**校验方式**：检查器逐行**用该行声明的规则、从该行的前提重新导出结论**，再确认每个前提是"已断言"或"更早某行的结论"。
**关键定理**（Lean 4，核心 Lean 无 Mathlib）：

| 你问 | 得到的 | 依据定理 |
| --- | --- | --- |
| OWL 推理 | 推导证书 | `OOCert.certificate_sound` |
| 用**你自己写的规则**推理 | 证书 + **换一个判词** | `OOCert.horn_certificate_sound` |
| 是否可满足 | 有限模型 | `Dl.satisfiable_of_checkModel` |
| 求解器的模型是否真实 | 回放后的模型 | `Fol.satisfiable_of_check` |
| 是否不一致 | 反驳（refutation） | `OOCert.refutation_sound` |
| 数据是否合形状 | 校验报告 | `Shacl.validate_spec` |
| 检索切片是否仍支撑结论 | 逐 claim 的保持性 | `OOCert.certificate_sound` |

**四内核 + 外部复核（工程上极罕见）**：

1. `lean/`——主形式化，Lean 4 v4.33.1，`lake build` 105 jobs，**无 Mathlib**（15,288 行，可一个下午读完）。
2. `isabelle/`——**独立第二形式化**，从 W3C 原始规范写、**刻意不读 `lean/`**；它抓到过一个真缺陷（证书格式没说"重复的绑定键"是什么意思，Lean 静默全化、Isabelle 拒绝，1,718 份证书里 47 份因此分裂）→ decision 0008。它买到的是**"规范解读的独立性"，不是内核独立性**——这一点项目自己反复澄清。
3. `rocq/`——**第三形式化，Rocq 9.2**，从规范独立写、不读 lean/isabelle；在 1,593 行上与 Lean 一致 1,269、不一致 324，**单一原因**：Lean 跳过空行、Rocq 拒绝空行（decision 0015，**OPEN**）。同一轮还发现 Rocq 校验器遇到规则号 `99999999999999999999` 会栈溢出、OCaml 以 exit 2 退出，而 exit 2 是该工具的"解析错误"码——**崩溃在冒充判词**。
4. `dafny/RuleTable.dfy`——规则表形式化。
5. **外部复核（`docs/independent-rechecking.md`，RAN，2026-09-15）**：用 `lean4export` 导出 515 MB / 9,850,420 行，再用 **`nanoda`（Rust 写的 Lean 4 类型检查器，与 Lean 共享零代码、不跑 Lean 运行时）** 复核，**25 分钟通过 exit 0**。副产品比主结果更值：允许清单外的 `sorryAx`/`Lean.ofReduceBool` 明明在导出里却没被用——**在 Lean 之外独立确认了"这批证明里没有 `sorry`、没有 `native_decide`"**，且不依赖任何 `#guard_msgs` 钉。
   **成本诚实**：25 分钟 + 515 MB 中间文件，只换来"同一类型论的独立实现复核"（防 C++ 内核 bug，**不防类型论本身或公理形状之误**）。
   **决策**：暂不集成（唯一例外 `leanchecker` 一行 CI，免费且闭掉"elaborator 篡改"缺口），触发条件写明三条（外部方要求不信 Lean 而信判词 / pinned 版本爆出内核不健全 bug / Lean 层涨到一个人读不完）。

**为什么值得抄**：它给出了「**引擎不可信 → 证书 → 独立内核复核**」的完整工程范式，以及配套的**判词纪律**：`entailed`（内置规则，已被证明）/ `entailed_under_supplied_rules`（你的规则=假设，不是事实）/ `clash_found_by_this_engine`（**本引擎发现冲突**，绝不冒充 `unsatisfiable`）/ `ungrounded_in_source`/ `measured`（测量值不冒充定理）。**措辞即契约，且有测试钉住判词不许退化。**

### 3.4 规则即数据（decision 0003）

`rules-import --from swrl` / `--from rif` 把标准规则语法读进 Horn 表。**只有一小片语法是三元组模式上的 Horn**：SWRL 内置原子、same/different-individual 原子、data range 一律**拒绝**；RIF 的 quality/`External`/`Expr`/`rif:local` 常量、列表项、呈现语法一律拒绝。**每种拒绝都被命名并计数，且默认"一个拒绝就整批失败"**——理由写得极准：*一个悄悄丢了一半规则的规则集仍会到达不动点，证书仍然是绿的，于是你得到一份关于"没人写过的规则集"的可靠证明。*
**你提供的规则是假设**，所以只赚 `entailed_under_supplied_rules`（真于"满足你的规则的每个模型"），**永不赚 `entailed`**。判词变化本身有测试钉住。

### 3.5 解释与溯源（decision 0016）

- `onto_justify`：**公理定位（MinA）**——`T` 的一个 justification 是断言图的子集 `S`，`T ∈ closure(S)` 且对任何真子集都不成立。**「最小性」才是产品**（一个不最小的支撑集会冤枉无辜公理，用户删掉一条发现结论还在，就会去怀疑工具）。三个主张三个词：充分性（写证书则可证）、最小性（**只有执行、无定理**）、表完整性（Reiter 命中集树，对单调 oracle 完备、有界、会被标 `truncated`）。支持 `candidate` 模式**只检不搜**（判词：`minimal_justification` / `not_a_justification_not_minimal` / `not_a_justification_target_not_reached`）。不一致时目标换成"本引擎发现 clash"，判词仍用 `clash_found_by_this_engine`。
- `onto_provenance`：**Green-Karvounarakis-Tannen 溯源半环**，套在规则表上跑第二遍不动点，每个导出三元组带一个在断言三元组上的代数表达式。**6 种半环**：`boolean`(∨,∧ 可导性) / `why`(并+吸收；极小元=justifications) / `lineage`(∪,∪，**不是** justification) / `counting`(+，× 证明树条数) / `tropical`(min,+ 最便宜证明树) / `trust`(**max-min**，最佳推导置信=最弱前提；与 tropical 的 min-plus 明确区分)。
  **递归是溯源静默出错处**：Datalog 递归 + 环 → 无穷多证明树，counting 发散、how-provenance 是无穷级数。项目逐条处理：吸收型（why/tropical/trust）收敛（`a ⊕ (a⊗b) = a`）；boolean/lineage 因值格有限且迭代单调而收敛（**不是**吸收性，两者理由**永不合并**）；counting **不收敛** → 带 `depth_bound`（默认 32）、`value_is_exact=false`、`value_means = LOWER BOUND`、`cycle_in_support`，溢出报 `saturated` 而**不报计数**。权重越界直接拒绝而非硬掰（tropical 拒负权、trust 拒 >1）。

> 二者关系：feature 1 是 feature 2 在正确半环下的特例（why 的极小单项式 = MinA，已测）。但**保证不共享**：provenance 在 DAG 上做代数、可截断；justify 重跑引擎。**不一致时以重跑为准。**

### 3.6 RAG 切片：蕴含保持（decision 0007、0011）——我们 05 号最该抄的一节

**痛点**：99% 覆盖率仍可能丢掉答案依赖的那一条三元组；且**覆盖率朝错的方向动**——切片越大覆盖率越高，于是"按覆盖率调优的检索器学会的是多取，不是取对"。**要的性质是"蕴含保持"，不是覆盖率。**

**四象限（这是全项目最锋利的一段设计）**：设 `G`=全库、`P`=检索切片、`q`=答案依赖的 ground 正三元组：

| `G⊢q` | `P⊢q` | 含义 |
| --- | --- | --- |
| 是 | 是 | **preserved**——唯一能带证书的格子 |
| 是 | 否 | **切片丢的**（检索问题） |
| 否 | 是 | 投影不是源子集 / 单调性被破坏 → **引擎不健全，永不是检索问题** |
| 否 | 否 | **`ungrounded_in_source`——断言在源里根本没支撑，修的是生成器不是检索器** |

**第四格是价值所在**：生成器编造与检索器漏取**修复方向相反**，把它们都报成"未保持"会把每次排查都送到错误的团队。且它**明说**`ungrounded_in_source` ≠ 断言为假（RDF 蕴含是开放世界、本引擎只覆盖 29/78 条规则）。
**实现细节**：`Reasoner::run_full` 跑两遍（`materialize=false` + `certificate_dir`），成员判定**从磁盘上的 `asserted.tsv`/`derivations.tsv` 读回**而非查内存闭包——**因为 Lean 检查器只见这两个文件**。
**模块 vs 切片（decision 0011）**：**模块带定理，切片带测量**——`onto_module_extract` 是签名上的语法局部性模块（Cuenca Grau/Horrocks/Kazakov/Sattler, JAIR 31 (2008) 定理），**项目明确写"工具引用该定理、本机没有机器检查它、`lean/` 里没有一个文件关于局部性"**，并退而给**测量**（在仓库的 pizza 本体上：模块 238/1345 公理，2,583 个差异中**丢 0 条结论**）。`onto_conservative_check` 用同一套机制回答生命周期问题：**加了这些公理，是否改变了本体已经说过的话？**

### 3.7 双时序（`src/temporal.rs`，147KB）

**两个刻意独立的时钟**：**VALID time**（陈述在世界上何时成立）与 **RECORDED time**（库何时开始持有它）。合并二者会丢掉人们真正问的两个问题——「当时什么是真的」与「当时我们信什么」（审计要后者、分析要前者、矛盾检测要两者）。
**盘上形状**：因 RDF-star 解析器不接受，改用**命名图承载断言 + 默认图上描述有效期**（普通 TriG，任何 store 可读）：
```turtle
:g1 { :HEK293 a :AdherentCellLine . }
:g2 { :HEK293 a :SuspensionCellLine . }
{ :g1 t:validFrom "2024-01-01"^^xsd:date ; t:validTo "2026-05-01"^^xsd:date ;
      t:recordedAt "2024-01-05T09:00:00Z"^^xsd:dateTime ; t:recordedUntil "2026-05-02T09:00:00Z"^^xsd:dateTime .
  :g2 t:validFrom "2026-05-01"^^xsd:date ; t:recordedAt "2026-05-02T09:00:00Z"^^xsd:dateTime . }
```
**语义**：缺 `validFrom`=「一直如此」、缺 `validTo`=「仍为真」、缺 `recordedUntil`=「仍被相信」、**完全无时序描述的图=timeless**（对每个快照都在范围内 → 给既有库加这套词汇**在使用前什么都不改**）。区间两轴都半开 `[from, to)`——**边界相接不算重叠**，这才使"5 月前贴壁、5 月起悬浮"成为**修正**而非矛盾。
**关闭 recorded 区间**是 `as_of` 能回答"当时信什么"的前提（只前向收窄的话，被修正的断言会在其后每个快照里与修正一起反复出现）。
**血缘是断言的、永不推断**：同期两图可能是修正（同一权威替换自己的断言）/ 分歧（两个来源）/ 撤回（撤下且无替代），**周期本身分不出三者**；两个谓词写在新图上——`temporal:supersedes`（命名被替换的图）与 `temporal:retracts`（命名被撤回且无替代的图）。

> **对 62 号的重要回应**：62 号我们指出 Kartograph 的「Bi-Temporal」是 README 标语（文件树 1690 路径命中 0）。**open-ontologies 有真正的实现**（147KB 源码 + 三个工具 + 半开区间语义 + supersedes/retracts）。若将来要采纳双时序，**这是唯一有代码可对照的先例**。

### 3.8 生命周期机制细节

- **plan 是"完整期望态"**（如 Terraform）：**Turtle 里没有的就被删**。所以计划同时报 TBox **和实例数据**（apply 两者都删）——**任何删除至少 medium 风险，被引用则 high**。每个计划持久化在状态库、返回 `plan_id`（保留最近 100 条），使 `plan` 与 `apply` 可跨进程/跨 MCP 调用；「最近」按"谁算的"限定（一个 HTTP 模式共享一个状态库，不限定会让一个会话 apply 掉另一个会话还在评审的变更）。给不存在的 `plan_id` 是**报错而非静默回退到最新**。
- **保守性（唯一"关于语义"的部分）**：`check_conservativity: true` 才跑（要双向推理到不动点），**关着时块仍在且写 `ran: false` + 原因**——因为"缺失的块与干净的块对仪表盘长得一样"。**永不致命**：跑不动就置 `conservativity.skipped` 并照返回计划。判词是**词不是布尔**，且是"相对于 Horn 规则表"而非"在描述逻辑里"的保守性。
- **apply 两模式**：`safe`（写差量）/ `migrate`（同样 + 给消费者加 `owl:equivalentClass`/Property 桥）。`strategy` 报 `delta`，或**当任一侧存在空节点时改报 `reload`**（bnode 标签是 store 局部的，对其做集合差无意义，`DELETE/INSERT DATA` 也载不动它）。
- **重命名桥**：`migrate` 用 `DriftDetector`（与 `onto_drift` 同一个自校准检测器）把每个被删项一对一配到替代项（**一对一**：一个新增不能同时是两个删除的替代）；名/标签相似度 < **0.6** 的配对被拒。因 `owl:equivalentClass` 是**硬逻辑断言，错的桥比缺的桥更糟**——每个桥附相似度分（`bridges`），每个被拒的删除列入 `unbridged_removals`，**两者都要看才敢信一次迁移**。
- **drift**：Jaro-Winkler 相似度检测重命名 + drift 速度 + SQLite 反馈回路自校准。
- **pack/unpack（decision 0017）**：`onto_pack` 写出可移植版本化包（排序 N-Triples + manifest：name/version/counts/timestamp/sha256/打包时的 lint-enforce 结果 + **覆盖图与证书联合的 `content_sha256`**）；`onto_unpack` **任一摘要不匹配即拒**，并**在本机重跑 Lean 检查器**，「**证书被拒则阻断装载；检查器缺席则报告"什么都没检"，而那不是通过**」。
- **反馈自校准**：lint 与 enforce 从用户判定学习（同一告警被 dismiss 三次即抑制、accept 一次即钉住）；`align`/`drift` 用同一模式。

### 3.9 MCP 工具面的工程纪律（我们直接可用）

- **10 个 profile** 是"一个客户来干一件事"的工具子集；`--tools-allow`/`--tools-deny` 是"调用者可被信任做什么"——**两轴只做收窄**（命名一个 profile 没有的工具也拿不回来），**未知 profile 直接拒绝启动**（"因为某人打错了 flag 就静默把整份目录发布给只要一件事的部署，是唯一值得为它拒绝启动的失败"）。
- **每个 profile 带同一 floor**（status/validate/load/query/stats/clear）——"任何 profile 里没有别的东西能干成事，而一个既不能检查自己装了什么、又不能清掉一次坏装载的客户，会被困在自己的 profile 里"。
- **规模数字一个都不许手写**：`tests/toolfilter_profiles_test.rs` 从 router 测，**任何注册工具不属于任何 profile 就让下一个加工具的人带着失败测试**；tool-reference 的 122 行由 `#[tool(...)]` 属性生成并测试强制。

### 3.10 为什么这套工程纪律值得单独看

`docs/decisions/` 18 份决策档**每份只写一条规则 + 它防的失败**，且都写明**为什么这条规则值它的成本**。READme 受 **ASD-STE100**（一句一意、主动语态、描述句 ≤25 词、段落 ≤6 句）约束并由测试强制。**"这纪律抓到过什么"一节列出：一周内 5 个 DL 假清洁（不一致本体被满信心地报为一致）、一条能推出"任何序列化器都写不出的三元组"的规则（使库非确定：同一输入三次跑分别得 40/9/24 条推断）、两个独立内核在同一批证书上分歧**。这段自陈的价值高于它的功能列表：**它把"我们怎么知道自己错了"写成了可复现的工程事实。**

---

## 4. 对本项目的借鉴增量（相对 34 号）

34 号（v0.2，39 工具）已定的结论不变；以下是**本轮新增/更新**的部分：

### 4.1 事实更新

| 项 | 34 号（2026-09-20） | 本档（2026-10-08） |
| --- | --- | --- |
| 工具数 | 39 | **122**（+ 10 profile 档位） |
| 版本 | 未记 | **2.1.0**（我们集成的是 2.0.1） |
| Star | 未记 | **930★** |
| 变更生命周期 | 提到 plan/apply/drift/rollback | 补全 `enforce`/`monitor`/`policy`/`pack`/`conservativity`/`lineage` + 反馈自校准 |
| 证书体系 | 「不采用（学习项目不需要）」 | 补全四内核 + 外部复核 + 判词纪律 + `leanchecker` 例外——**"不引入代码"结论不变，但"思想借鉴"的具体条目大幅增加** |
| RAG 切片 | 「借鉴思想，记需求池」 | 补全**四象限**语义 + `module_extract`（定理）vs 切片（测量）之别 |
| 双时序 | 未提 | **新发现**：147KB 真实现（不是标语） |
| 对齐 | 「stable matching 记 P2 远期」 | 补全 6 信号 + FLORA（ISWC 2025 最佳论文）+ SSSOM 输出 + OAEI 评分 |
| CQ | 未提 | **新发现**：`onto_cq_run` + `onto_verify_cq`（与 REQ-248/255 同题） |

### 4.2 建议借鉴条目（按"是否要写代码"分层）

**A. 纯口径/词汇层（不写代码，建议现在就吸收进 05 号《消费与审计》）**
- **A1 判词纪律**：`entailed` vs `entailed_under_supplied_rules` vs `clash_found_by_this_engine` vs `measured` vs `ungrounded_in_source`——**一份报告必须说清"这是被证明的、这是引擎说的、这是测出来的、这是你的假设"**。这与 62 号 K1/K2（k8s-ontology 的溯源声明层 + AI_CONTRACT）同族，且此处更完整。
- **A2 「缺失 ≠ 不存在」**：`ungrounded_in_source` 明说"不等于断言为假"——与我们 05 号 lossy 报告口径一致，可统一措辞。
- **A3 溯源半环词表**：why / lineage / counting / tropical / trust 六种语义的**名字与混用禁令**，可直接作为我们溯源设计的对照表（尤其 **`lineage` ≠ justification** 这条）。
- **A4 覆盖率陷阱的表述**：*"按覆盖率调优的检索器学会的是多取，不是取对"* ——我们 05 号讲切片/检索质量时可直接引用这一句作论据。

**B. 结构层（需设计，建议进 05 号/REQ 载荷轮）**
- **B1 切片蕴含保持的四象限**（`preserved` / `lost` / 引擎不健全 / `ungrounded_in_source`）——**四个格子对应四个不同的修复团队**，这是"有损报告"从计数升级为诊断的关键一步，且与我们 62 号 K2（消费契约）可一轮同批设计。
- **B2 「模块带定理、切片带测量」**：把"我们有定理保证"与"我们量过"分开写——**可直接作为我们质量门禁报告字段设计**（`guarantee: theorem | measurement | none`）。
- **B3 计划的生命周期化**：`plan_id` 持久化 + 「最近」按"谁算的"限定 + 无匹配 id 报错而非回退——我们 REQ-157 merge/preview 的评审-应用动线可直接对齐（**跨会话误 apply 是真风险**）。
- **B4 `strateg` 降级为 `reload` 的触发条件**（空节点 → 集合差无意义）——我们的差量/全量切换判断可照抄。
- **B5 保守性检查的三态**（`ran` / `skipped`+原因 / 干净）——"**缺失的块与干净的块对仪表盘长得一样**"是我们所有报告型接口的通病，建议作为 05 号报告字段通用规范。
- **B6 双时序（valid/recorded + 半开区间 + supersedes/retracts）**——62 号埋的"若采纳须自设双轴"，**此处有完整先例可对照**；且给出关键设计理由（**关闭 recorded 区间才让 `as_of` 可回答**）。

**C. 工程方法层（低成本、价值确定）**
- **C1 规模数字不许手写**（从 router 测、加工具就带失败测试）——我们 qualitygate/工具清单可直接照抄这一条。
- **C2 未知配置名拒绝启动**（而非静默回退默认）——一条极便宜、防"打错 flag 静默发布"的纪律。
- **C3 决策档格式**：一文件一规则 + 写明它防的失败 + 写明为什么值它的成本 + 索引表 + 编号测试——我们 15 号/18 号的治理可吸收。
- **C4 「这纪律抓到过什么」清单**：把"我们怎么发现自己错了"写成可复现事实——建议我们每个大模块也留一节。

### 4.3 不建议（结论与 34 号一致，予以确认）

| 项 | 理由 |
| --- | --- |
| Rust 引擎/二进制引入 | 我们从 2.0.1 已集成双轨（M8.5）；换栈无收益，且 REQ-78 互通仍冻结 |
| Lean/Isabelle/Rocq/Dafny 四内核 | 学习项目不需要可信计算基；`lake build` + 515 MB 导出代价远超收益（**思想借 A1，代码不引**） |
| PDDL/Fast Downward、CIVeX、临床 crosswalk、22 案例集 | 场景不符 |
| Studio（Tauri 桌面）、Obsidian 插件 | 形态不符 |
| WASM 插件市场 / 社区 registry | 生态规模不符 |

---

## 5. 与 34 号的协同（不替代）

- **34 号继续是"集成档案"**：双轨边界、oo-worker、工作台形态、REQ-100/78 口径——本次不动。
- **本档是"能力档案"**：当我们要回答"这套能力我们自己要做到什么程度"时看本档；当回答"怎么把它接进来/摘出去"时看 34 号。
- **建议在 34 号头部加一行反向索引**指到本档（已在回写中执行）。

## 6. 诚实边界（必读）

1. **未运行**：沙箱无法 clone，也未安装二进制作实测；**全部结论来自 README + `docs/*` + `src/*` 源码头部与结构 + Cargo.toml 的一手阅读**，无一条经运行验证。文中凡涉"实测/数字"均系**项目自述**（项目自己区分 RAN/READ，这是它的优点，我沿用）。
2. **工具数三个口径**：`docs/tool-reference.md` 122 / `SKILL.md` 110 / 我们 15 号旧记录 119（v2.0.1）。**122 是 docs 自述且受测试强制**，但 SKILL 未同步 → 说明该仓库文档也存在滞后，引用时以 `main` 为准并注明快照日期。
3. **29/78 的覆盖面**：它是 OWL 2 RL 的一部分（29 条）+ SHIQ tableaux（**不带证书**）。**"无 clash"不等于一致，"本引擎没推出来"不等于不蕴含**——项目自己反复声明，我们引用时必须一并带上。
4. **交付物是"证据"而非"正确性"**：全部证书只证"在该规则表下从该断言集可导出"，**不证本体正确**、不证 OWL 2 RL 语义。
5. **`temporal.rs` 未逐行读**：双时序结论来自模块头注释（约 3KB）+ 三个工具的描述，**未通读 147KB 实现**，故"有真实现"这一判断成立，但细节完备性未穷尽。
6. **单一作者 + 商业实体**：作者 Fabio Rovai + Tesseract Semantics，star 增速快（3→930 约 7 个月），**API 稳定性未经历长期检验**（34 号已记此风险，依然成立）。
7. **`benchmark/` 23GB**：仓库含巨大基准数据（LUBM/OAEI anatomy/EPC 等），**我们的 fork/镜像策略需注意体积**。
8. **未立项**：本档为**纯调研借鉴**（沿 59/62/63/64 号先例），无新需求行；若采纳 A/B 簇条目，按 18 号注册表查号 → 03 号需求档立项。

---

## 参考资料（一手）

- README（30.7KB）、README.zh-CN.md、SKILL.md、ECOSYSTEM.md、CLAUDE.md（41.7KB）、CHANGELOG.md（245KB）、Cargo.toml
- `docs/`：architecture.md、tool-reference.md（122 工具全表）、lifecycle.md、explanation.md、independent-rechecking.md、lean-certificates.md（64KB）、trusted-computing-base.md（71KB）、modules-and-conservativity.md、reasoning-systems-inventory.md、ci-gates.md、first-order-export.md
- `docs/decisions/` 18 份（0001 inference≠assertion / 0002 carry a certificate / 0003 rule is data / 0005 prover is an oracle / 0006 model is a certificate / 0007 slice preserves / 0008 binding admits one reading / 0009 translation carries satisfaction / 0010 input is a value / 0011 module vs slice / 0012 concurrency below certificate / 0013 second oracle / 0014 verifier of rewrite / 0015 blank line (OPEN) / 0016 conclusion names axioms / 0017 pack carries proof / 0018 conformance ≠ truth）
- `src/`：temporal.rs（双时序头注）、projection_entailment.rs（四象限头注）、reason.rs、tableaux.rs、server.rs、shacl.rs、module_extract.rs、justify.rs、provenance.rs、conservativity.rs、plan.rs、matcert.rs、toolfilter.rs
- 证明器目录：`lean/`、`isabelle/`（OO_*.thy 14 份）、`rocq/`、`dafny/`、`aeneas/`
- 论文：arXiv:2605.09184、arXiv:2605.09168；集成方法来源 #28–#50（NORA NeurIPS 2025 / ISWC 2025 ×4 / K-CAP 2025 ×2 / SEMANTiCS 2025 / JAIR 31 (2008)）
- 关联档：本目录 **34 号**《open-ontologies 借鉴与引入分析》（集成视角，v0.2）、**39 号**《本体运行时与 MCP 服务开源方案调研》、**62 号**《kubernetes-ontology 与 Kartograph 借鉴分析》、15 号《开源项目及论文登记簿》第五类
