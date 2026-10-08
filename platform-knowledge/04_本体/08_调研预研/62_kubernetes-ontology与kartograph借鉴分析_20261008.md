---
module: 本体
topic: kubernetes-ontology 与 Kartograph 借鉴分析（图谱/本体平台类开源项目两例）
desc: 两项目一手文件精读对照本体模块现状——kubernetes-ontology（Go/关系规格单一事实源/边级溯源与断言态/AI 消费契约/canonicalId 身份脊）出 K1~K9；Kartograph（Python/DDD 边界可执行化/本体自演进/变更日志 JSONL/MCP 防御契约/溯源原文工具）出 G1~G11；含两条路线对照与优先级落点建议
synced: 2026-10-08
---

# kubernetes-ontology 与 Kartograph 借鉴分析（2026-10-08）

> **触发**：开发者指令「结合当前项目背景，分别调研 github.com/Colvin-Y/kubernetes-ontology 和 github.com/openshift-hyperfleet/kartograph 项目，分析当前项目可以借鉴哪些」。
> **数据获取方式**：沙箱网络无法 clone，采用 **GitHub API（元数据/recursive tree）+ raw 文件直读**（README / AI_CONTRACT / RelationSpec 源码 / OWL 导出 / specs 规格目录 / 引擎与安全源码）。结论优先基于一手文件，**未做全量代码统计**，涉及 star/活跃度处已单独标注。
> **方法**：以「本体模块五栏现状（[00_本体模块](../00_本体模块.md) 各子模块实现状态文档 + [04_方案设计](../../../docs/04_本体_方案设计.md)）」为基准，做**已覆盖 / 可借鉴 / 不建议**三档判定，借鉴点编号 K（kubernetes-ontology）与 G（Kartograph）。

---

## 0. 两项目定位与「两条路线」对照

| 维度 | kubernetes-ontology | Kartograph |
| --- | --- | --- |
| 作者/组织 | Colvin-Y（个人） | openshift-hyperfleet（Red Hat 系，Konflux 构建/Quay 发布） |
| 语言/许可 | Go ／ Apache-2.0 | Python(FastAPI) ／ Apache-2.0 |
| 规模/活跃度 | 266 文件；**3★**/0 fork；创建 2026-04-23、末次推送 **2026-06-02** | 1690 文件；**10★**/2 fork、10 open issue；创建 2025-12-05、末次推送 **2026-08-03** |
| 自我定位 | 只读 Kubernetes **拓扑本体服务**（诊断 + AI-agent 消费），in-memory 图 | 企业级 **双时序属性图平台**（多源接入 → LLM 抽取 → 图存储 → MCP 消费）；README 自述 experimental |
| 路线 | **事实内核优先**：不追求「先建本体数据库」，graph-first + agent-first，明确写下「不要 ontology-database-first 漂移」 | **图谱存储优先 + 平台化**：Apache AGE(Postgres) 为存储、DDD 六上下文分工、多租户/SpacyDB 鉴权 |
| 对本项目的价值面 | **模型层与消费契约的写法**（关系规格、溯源、AI 契约） | **工程组织与写路径的写法**（架构边界测试、变更日志、schema 自演进、MCP 防御细节） |

**一句话结论**：两项目**恰好代表两条相反的架构哲学**，而本项目既定立场（D-O1 构建与运行解耦、spec_json 为主形态、本体=模型层、D-O15 零 Python 运行时依赖）位于**两者之间偏 kubernetes-ontology 一侧**——它的演进文档（`docs/design/kubernetes-semantic-kernel-evolution.md`）几乎逐句印证我们的判断；Kartograph 则在**工程纪律与写路径契约**上给了可直接抄的作业。**两者均不引入代码**（语言形态/企业级 IAM/AGE 存储三项均与项目口径冲突）。

---

## 一、kubernetes-ontology（Colvin-Y）

### 1.1 项目全貌

只读 K8s 拓扑服务：从 K8s 对象构建内存图，informer/轮询保鲜，暴露稳定 CLI + HTTP 查询（entity/relations/neighbors/expand/**diagnostic subgraph**）。开源 MVP 刻意轻量：无 CRD/控制器、不写被观测资源、无持久化库、无外部图后端。三入口=CLI（agent 用）/ HTTP API / 拓扑观测器（cytoscape，Go embed 静态资源单二进制）。附**项目自带 Codex 型 Skill**（`skills/kubernetes-ontology-access`，可 `npx skills add` 安装）与 AI_CONTRACT.md。

### 1.2 设计要点精读（一手）

1. **关系规格单一事实源（`internal/model/relation_spec.go`）**——35 条关系的 `RelationSpec{Kind, Comment, Domain, Range, InverseOf, DefaultSourceType, DefaultState, ResolverHints}` 集中声明；**OWL 导出（`make owl`）与 JSON Schema 校验、边构造函数、文档全部从这一份声明生成**：`NewEdgeWithResolver` 直接取 `spec.DefaultSourceType/DefaultState/ResolverHints[0]` 作为边的默认溯源。OWL 侧产出 **36 类（8 抽象 + 28 具体）+ 35 对象属性**（含 domain/range/inverse）。
2. **边级溯源与断言态是一等公民**——每条边带 `EdgeProvenance{SourceType, State, Resolver, LastSeenAt}`；`SourceType ∈ {explicit_ref, selector_match, owner_reference, binding_resolution, inference_rule, observed, label_evidence}`，`State ∈ {asserted, inferred, observed}`，`Resolver` 是有版本号的命名解析器（如 `owner-chain/v1`、`csi-component-rule/<driver>/pv-agent/v1`）。
3. **AI_CONTRACT.md：给 agent 的「有界、带溯源证据图」消费契约**——响应含 `schemaVersion` / `recipe`+`lanes`（场景包）/ `partial` / `warnings` / `degradedSources` / `budgets{maxDepth,storageMaxDepth,maxNodes,maxEdges,truncated,truncationReasons}` / `rankedEvidence` / `conflicts` / `freshness`；并显式规定**两级稳定性**（A 保证结构 vs B 尽力而为证据）、**冲突保留不静默裁决**、**「缺失≠不存在」**（`IncludeEvents=false` 时不得断言「无事件」）、`canonicalId` 为唯一身份键且**后端存储 ID 禁止进入下游逻辑**。
4. **Resolver 分族的关系恢复**（`internal/resolve/{explicit,infer,owner,selector}`）——显式引用/推断规则/owner 链/selector 匹配四类恢复器各自独立、按规则命中，未命中即诚实留空。
5. **语义内核演进路线（`kubernetes-semantic-kernel-evolution.md`）**——北极星=持续维护的语义内核；五阶段：①稳定契约 ②快照→持续内存内核 ③查询服务面 ④**语义映射层长在内核之上、不得扭曲事实模型** ⑤持久化后端与推理钩子。并列出四条风险，其中**「ontology-database-first 漂移」与「诊断工具专用漂移」**最贴合我们本体运行主线的取舍场景。
6. **教学不依赖真机**——`samples/failure-modes/*/diagnostic-graph.json` 七类离线故障图 + `internal/fixtures/*.golden.json`；观测器可脱离集群直接打开样例；观测面板**先暴露证据可信度（预算截断/警告/冲突/排序证据）再给叙事解释**。

### 1.3 可借鉴清单（K 系列，按价值排序）

| # | 借鉴点 | 一手证据 | 我们的缺口（现状对照） | 建议落点与优先级 |
| --- | --- | --- | --- | --- |
| **K1** | **关系/断言的「溯源 + 断言态」声明层** | `EdgeProvenance{SourceType,State,Resolver}`；7 种来源类型 × 3 种状态枚举；inferred 不等于真相 | spec_json 的 `Relation{name,label,definition,from,to}` 与 `InstanceRel{rel,target}` **只有正确性、没有来源与可信度**；OWL 有损导入的「丢什么/推什么」目前仅在导入报告里作 lossy 计数，未落到元素级 | spec_json 增**可选**溯源声明（来源类型/推导规则 id/状态），存量资产零迁移；口径上「导入=asserted、LLM 推理=inferred、规则推导=rule:X/v1」；消费侧按状态排序证据。**P2**（与 REQ-235 lossy 动线同轮） |
| **K2** | **AI 消费契约（契约文档 + 响应级 additive 元数据）** | AI_CONTRACT.md 全文：schemaVersion / A 级保证 vs B 级尽力 / partial+warnings+degradedSources+budgets+rankedEvidence+conflicts+freshness / 契约演进规则 | 我们有 **guide 紧凑口径**（运行栏 facade）与 onto_* 6 只读工具，但**没有一份「本体消费契约」文档**，响应无「截断/降级/冲突/新鲜度」结构化字段；「本体命中→回答贡献」度量亦缺（05 号演进项） | 提炼《本体消费契约》（版本号 + 稳定/尽力两级 + additive 元数据清单）落 `docs/04` 附录或 04_本体 模块根；facade 响应加 additive 字段（不破坏 6 工具既有形态）。**P2** |
| **K3** | **canonicalId 身份脊（身份与存储 ID 解耦）** | `canonicalId` 为唯一身份键、**后端 ID 禁止进入下游逻辑**；身份生成确定性 | 我们已有确定性 URN（`URIPrefix/Sanitize`，导出与查询翻译共用），但**身份键与人类名同名**，导致概念/实例同名歧义只能靠跨域查重拦截（REQ-249） | 命名空间化身份：`concept:名` / `instance:名` / `attr:名` 作为对外身份键（URI 已是此形态，**spec 层身份键与 URI 对齐**）；根治 REQ-249 并让 onto_* 精确匹配无歧义。**P2**（REQ-249 同轮） |
| **K4** | **RelationSpec 单一事实源 → 多出口生成** | 一份 `relationSpecs` 派生：边的默认溯源 + OWL 对象属性（domain/range/inverse）+ JSON Schema + 文档 | 我们的 relations **内联在 spec_json**（定义与使用一体，57 号 V2 已记）；domain/range 已在（from/to），但 **inverse、默认来源、解析器提示无处声明**，导出与校验各写一遍 | 关系规格「声明集中、导出与校验共用」——落地= spec 关系加 `inverse`/可选溯源字段，TTL 导出与 qualitygate **读同一份声明**（对齐 REQ-268 数据属性的「声明与使用统一」做法）。**P2**（与 K1/G3 同轮） |
| **K5** | **语义内核五阶段演进路线**（作为外部验证与里程碑参照） | 五阶段 + 四条风险（ontology-database-first 漂移 / 工具专用漂移 / 层间纠缠 / 强事实与弱证据无边界） | 我们的 ontology runtime 主线正在「加载并运行预构建 ontology」阶段，**尚无公开的演进阶梯**；04 号 §3 有现状但缺「下一阶段是什么」的表述 | **不立项**：作为本体运行主线的**外部参照档**（其 Phase 4「语义映射层长在内核之上、不得扭曲事实模型」= D-O1 的第三方背书；风险 1/4 对应我们「本体=模型层」边界与 K1/K2）。档内引用即可 |
| **K6** | **命名 + 版本化的推导规则 id（Resolver 型式）** | `owner-chain/v1`、`service-selector/v1`、`csi-component-rule/<driver>/pv-agent/v1` | KB 抽取规则、导入策略、LLM 构建路径目前**以策略名（A/B/C）标识**，无「规则 id + 版本」的细粒度可追溯标识 | 抽取/推导规则给稳定 id+版本，供决策审计（onto_decision）与溯源链引用。**P3** |
| **K7** | **离线场景样例包 + 可离线打开的观测器 + golden fixtures** | `samples/failure-modes/` 7 类 + `internal/fixtures/*.golden.json`；观测器脱离集群可打开样例 | 我们已有 7 份种子本体 + 学习中心内容包；**缺「样例即场景」的离线演示包**（如「本体装载失败/推理差异/导入有损」样例） | 为种子本体配「场景样例包」（JSON/TTL + 期望快照），观测/可视化可离线打开——教学不依赖引擎在跑。**P3** |
| **K8** | **能力默认只读 + 显式 opt-in 升级 + 安装足迹文档化** | 只读 RBAC（get/list/watch）、`rbac.readSecrets=false` 默认、三种部署模式各自足迹明示 | 我们 facade/工作台已是**双只读白名单**（大半覆盖）；**缺 opt-in 升级开关与「安装足迹」文档形态** | 对齐现有安全口径，补「默认最窄 + 显式开权 + 足迹说明」的表述（16 号部署档 §3.0 已部分承载）。**P3**（增量小） |
| **K9** | **能力自带 Agent Skill（能力分发=可安装 Skill）** | `skills/kubernetes-ontology-access/SKILL.md` + `npx skills add`，把上手流程做成 Skill 而非文档 | 平台已有技能模块 + 学习中心引导；**「每个能力面配一个 guide 型 Skill」尚未成制度** | 运行/消费能力各配 guide 型 Skill（技能模块已具备承载，零新基建）。**P3** |

### 1.4 不建议借鉴

| 项 | 理由 |
| --- | --- |
| K8s informer/轮询保鲜 + scoped reconcile（增量重建） | 我们是模型/知识平台，不是集群代理；无「持续观测外部世界」需求；其 reconcile 机制体量远超收益 |
| 纯内存图（无持久化） | 我们已持久化（SQLite 三表 + 版本快照），回退无收益；其 in-memory 是 MVP 边界而非目标态 |
| HTTP 无鉴权/TLS | 其自述「仅本地/受控环境」；我们的部署边界已在 16 号 §3.0 明示，无需照搬 |
| 诊断 recipe/lanes 作为机制 | 其 recipe 是**诊断场景**标签；我们的对应物是 **CQ（能力问题）+ 任务卡**——语义位不同，不移植 |

> **反哺点（正向）**：其 `docs/ontology/kubernetes-ontology.owl`（36 类/35 关系，含 domain/range/inverse）可作我们 **K8s 运维种子**的**对照建模范本**（命名规范 + 域值域 + 逆关系齐全），并演示「同一模型双出口：OWL + JSON Schema」——低成本的种子质量提升素材。

---

## 二、Kartograph（openshift-hyperfleet）

### 2.1 项目全貌

企业级属性图平台（README 自述 experimental、官网标注各组件 Not Started/In-Progress/MVP/Complete）。**DDD 六有界上下文**：Identity(IAM+SpiceDB) ／ Management(控制面：KG、数据源、加密凭据、同步计划) ／ Ingestion(适配器：dlt **仅 Extract 阶段**，产出 JobPackage) ／ Extraction(**LLM Agent** 产出 MutationLog JSONL + 确定性处理器处理重命名/删除) ／ Graph(存储引擎：**Apache AGE**(Postgres 扩展)，事务写 + 作用域只读 API) ／ Querying(**MCP Server**，等）。

### 2.2 设计要点精读

1. **架构边界可执行化**：`specs/nfr/architecture.spec.md` 开宗明义「**结构约束由自动化测试强制**（pytest-archon），非领域行为」——域层禁依赖 infra/app/框架；ports 层禁依赖 infra/app；app 只依赖 domain+ports；infra 禁依赖 app；**有界上下文互不 import**；共享内核不依赖任何上下文；**仅一个 composition 模块（mcp_dependencies）允许跨上下文 import**。
2. **规格即事实源**：`specs/` 下 40+ 份 GIVEN/WHEN/THEN 行为规格（extraction/graph/iam/ingestion/management/nfr/query/shared-kernel/ui），配 `.claude/skills/{spec,develop}` + `AGENTS.md`（agent 人格=DDD/TDD/tracer bullet，要求「100% TDD」「先写测试」「web 搜索最佳实践后再写码」「atomic conventional commit」）+ `.agent-memory/spec-alignment-reviewer.md`。
3. **本体（图谱 schema）是自演进的一等对象**（`specs/graph/schema.spec.md`）：类型定义含 `label/description/required_properties/optional_properties`；**schema learning**（CREATE/UPDATE 出现未声明属性 → 自动进 optional，**required 不动**）；**无 DEFINE 的类型 CREATE 即拒**；`node`/`edge` 同名可共存（标签按实体类型作用域）。
4. **写路径契约（`specs/graph/mutations.spec.md`）**：MutationLog JSONL（DEFINE/CREATE/UPDATE/DELETE）**幂等复放** + **确定性实体 ID `{type}:{16_hex}`** + **操作定序**（DEFINE → DELETE（边先于点）→ CREATE（点先于边）→ UPDATE）+ **系统属性服务端盖章**（`knowledge_graph_id` 调用方传入一律拒绝/忽略，防伪造）+ 引用完整性/孤边检测 + 批量装载（COPY + 暂存表、重复 ID 与孤边识别、advisory lock 且**锁按确定性顺序获取防死锁**）。
5. **本体 authoring 不变量**（`management/domain/ontology_prepopulation.py`）：**跨要素组合约束**——「被标记为 prepopulated 的关系，其两端实体类型也必须 prepopulated」，抛结构化 `PrepopulationValidationError`；并提供**稳定关系就绪键** `source|label|target` 供设计产物引用。`EdgeTypeDefinition` 另含 `bidirectional / inverse_label / inverse_of / auto_generated / bidirectional_pair_key`。
6. **MCP 消费面的防御性契约**（`specs/query/mcp-server.spec.md`）：`query_graph` 只读 Cypher 工具——**关键字拒绝集含 CREATE/DELETE/SET/REMOVE/MERGE/EXPLAIN/LOAD**；**无 LIMIT 自动补**（默认 1000/上限 10000）；**取 limit+1 行判定 `truncated`**；超时默认 30s/上限 60s；**内部属性剥离**（如 `all_content_lower`）；AGE 单列返回规范化（node→`{node:{}}`、edge→`{edge:{}}`、map 保留、标量→`{value:}`）；另两工具/资源：`fetch_documentation_source`（按 GitHub/GitLab blob URL 取**出处原文**并剥 AsciiDoc 元数据，支持 PAT 与自托管）、资源 `knowledge_graphs://accessible`、**`instructions://agent`（启动期文件缺失即 fail-fast）**；鉴权=API Key 或 Bearer JWT，无凭据 401、鉴权后端不可达 503。
7. **Secure Enclave（按实体鉴权的拓扑保留式脱敏）**：未授权节点→**仅 ID**（保留 label）、未授权边→**仅 ID+start_id+end_id**，**拓扑（谁连着谁）始终保留**；**任何异常→拒绝（fail-safe）**；按 kg_id 在单请求内缓存鉴权结果。
8. **抽取运行元数据与双轨**：MutationLog run 记录 **session/KG/actor/时间戳 + token 用量与成本 + 按操作类的计数**；`instance_change_record.py` 提供**实例变更 before/after 属性快照 + 逐属性差异**；`instance_generator_templates/` 是**确定性扫描器**（实体/关系扫描 → JSONL，含 examples 四种），与 LLM 抽取并存——「**确定性结构化事实 + LLM 语义关系**」双轨。
9. **接入侧工程形态**：适配器端口化（`IDatasourceAdapter`）+ **凭据经共享内核端口**（`ICredentialReader`，后端可从 Fernet 换 Vault 而上层零改动）+ **checkpoint 增量同步**（commit SHA，只取变更文件）+ JobPackage 打包 + **按 worktree 隔离端口的多实例开发**（`dev-instance.sh`，假 OIDC 签发真 RS256 JWT）+ DB 快照备份/恢复 + 图修复脚本 + kustomize base/overlays + Tekton/release-please/renovate。
10. **Domain-Oriented Observability**：AGENTS.md 明确「领域探针 100% 优于 logger/print」（引 Fowler 文）。

### 2.3 可借鉴清单（G 系列，按价值排序）

| # | 借鉴点 | 一手证据 | 我们的缺口（现状对照） | 建议落点与优先级 |
| --- | --- | --- | --- | --- |
| **G1** | **架构边界可执行化（结构约束即测试）** | `specs/nfr/architecture.spec.md` + pytest-archon；域/端口/应用/基础设施四层 + 上下文隔离 + 单一 composition 例外 | 我们是 go.work 三服务 + 包内分层的**模块化单体**，边界靠约定与文档（AGENTS.md），**无自动化边界测试**；`backend/ontology-service/runtime-manager` 与各服务内部分层易被越界 import 侵蚀 | 增架构约束测试（Go 侧可用 golangci-lint `depguard` 或自写 `go/parser` 断言测试）：服务间不互相 import（只走 HTTP/反代）、spec 包不依赖 repo/rest、engine 不依赖 facade…**P2** |
| **G2** | **统一写路径契约：变更日志 + 幂等 + 确定性 ID + 操作定序 + 服务端盖章** | `specs/graph/mutations.spec.md` DEFINE/CREATE/UPDATE/DELETE；`{type}:{16_hex}`；DEFINE→DELETE→CREATE→UPDATE；`knowledge_graph_id` 调用方值一律拒绝；孤边/重复检测 | 我们有**四条独立写入动线**（CSV 实例灌装、导入（spec/OWL/GraphML）、伴生候选、对话生长 ontoextend），**各自一套语义**；无统一变更日志、无幂等约定、无「系统属性服务端盖章」防伪造 | 提炼**本体/KG 变更操作契约**（操作集 + 定序 + 幂等 + 确定性实例 ID + 系统字段服务端盖章），统一四条动线的落库语义，并作为决策审计（onto_decision）与资产治理四事件（REQ-224）的载荷底座。**P2** |
| **G3** | **本体自演进 + authoring 组合不变量 + inverse/双向声明** | schema learning（未声明属性自动进 optional、required 不动）；无 DEFINE 即拒；`ObjectDefinition{bidirectional,inverse_label,inverse_of,bidirectional_pair_key}`；prepopulation 校验 + 稳定就绪键 `source\|label\|target` | 我们 qualitygate 已有悬空引用/重名/跨域同名等**单要素**检查（含 M77 `dangling_dataprop_domain`/`undeclared_attribute_key`）；**缺跨要素组合不变量**；关系**无 inverse 声明**；与 K4 同向 | ①qualitygate 增**组合不变量**类检查（如：关系 inverse 须成对互指、inverse 两端 domain/range 互换）；②关系补 `inverse`/双向声明，导出与校验共用（= K4）。**P2** |
| **G4** | **MCP 消费面的防御性契约细节** | 关键字拒绝集（含 **EXPLAIN/LOAD**）；自动 LIMIT 默认/上限；**limit+1 探测 truncated**；超时上下限；**内部属性剥离**；结果形态规范化（node/edge/map/scalar）；`instructions://agent` **启动期 fail-fast**；401/503 语义 | 我们 facade=**6 只读 onto_* 工具** + 服务端 **SELECT 白名单**（与 G4 同精神）；**缺**：结果截断语义、内部属性剥离、超时/上限作为显式契约、guide 作为 MCP resource、鉴权不可达语义 | ①工作台/facade 响应补 `truncated`+自动 LIMIT（limit+1 探测）②attr/内部键剥离白名单 ③超时上限显式化 ④guide 升格为 MCP resource（紧凑口径已有）⑤错误码语义（401/503/超时分类）。**P2** |
| **G5** | **「出处原文」获取工具（fetch_documentation_source）** | 按 blob URL 取源文件正文并剥元数据，支持私有仓 token 与自托管实例 | 我们有**来源指针**（platform-knowledge frontmatter `sources`、导入原始存档、PROV-O 导出、平台知识页），但**没有「给实体 → 返回其出处原文」的消费工具**——溯源止于「指向」，未到「可取原文」 | 消费与审计/伴生侧增「出处原文」能力：给定实体/实例 → 返回其来源文档片段（含私有源 token 处理）。**P2**（与 REQ-224/溯源链同轮） |
| **G6** | **抽取运行元数据 + 确定性生成器与 LLM 双轨** | MutationLog run 记录 actor/session/KG/token/成本/按类计数；`instance_change_record` before/after 逐属性差异；`instance_generator_templates/` 确定性扫描器 → JSONL | 我们 onto_decision 决策审计有 subject_kind 过滤与溯源链，**缺 token/成本/操作计数**（05 号演进项「本体命中→回答贡献度量缺失」同族）；KB 抽取为轻量实现，「确定性结构化事实 + LLM 语义」**双轨尚未制度化** | ①审计表补运行元数据列（成本/token/计数/actor）②KB/本体 LLM 链路明确「确定性生成器优先 + LLM 补语义」双轨（对齐 KB 升级自研增强主轴）。**P2~P3** |
| **G7** | **Secure Enclave：拓扑保留式按实体脱敏** | 未授权节点仅 ID、未授权边仅 ID+端点，**拓扑保留**；异常一律拒绝；请求内 kg_id 缓存 | 我们本体为单机教学定位，暂无话题/项目级共享；一旦引入**可见性/共享/多项目**，会立刻撞上「过滤掉就看不见图、保留就泄内容」的两难 | **设计储备**（不立项）：把「保留拓扑 + 内容脱敏 + fail-safe 拒绝 + 请求内缓存」写入未来共享/权限设计的候选方案。**P3** |
| **G8** | **DDD + GIVEN/WHEN/THEN specs 目录 + spec/develop Skill + spec 对齐审查者** | `specs/<上下文>/*.spec.md`；`.claude/skills/{spec,develop}`；`.agent-memory/spec-alignment-reviewer.md`；AGENTS.md 开发循环 8 步 | 我们是 Go+wine 需求档（01/03/11）+ 方案档（02/04/12）+ 平台知识导读；**「行为规格」与「需求」未分层**；agent 协作纪律有（AGENTS.md 硬性纪律 7 条）但**无「规格对齐审查」的独立角色/记忆** | **参照档**：把 GIVEN/WHEN/THEN 行为规格补在需求之外（可选、逐能力推进），并评估「spec 对齐审查」作为 agent 协作角色（多 agent 并行已在发生）。**P3** |
| **G9** | **按 worktree 隔离端口的多实例开发 + 快照/恢复 + 定点修复脚本** | `dev-instance.sh`（每实例确定端口偏移 + 独立网络/卷 + 假 OIDC）；`dev-backup/dev-restore`、`dev-repair-age-graphs` | 我们 `run-dev.sh` 一条命令起全栈（验收 22 的卖点），但**无端口隔离的多实例**（多 agent 并行开发会撞端口/撞 SQLite 文件）；无库快照/恢复工具 | ①run-dev 增「实例名 → 端口偏移」参数（多 agent 并行零台冲突）②数据目录快照/恢复脚本（含定点修复而不全量恢复）。**P3** |
| **G10** | **DataSource 一等公民 + 凭据端口化 + checkpoint 增量同步** | `specs/management/data-sources.spec.md` + `specs/ingestion/{adapters,sync-lifecycle}.spec.md`：`IDatasourceAdapter` 端口、`ICredentialReader` 共享内核端口（Fernet→Vault 零改动）、commit SHA checkpoint、只取变更文件、JobPackage 打包 | 知识库侧资料同步目前是**任务式脚本**（如微信知识库 wechat_track.py/build_panorama.py），**无数据源对象、无 checkpoint 增量语义、无凭据抽象** | 知识库资料同步的**设计参考**：数据源（源+凭据+计划）一等公民、checkpoint 增量、变更集与打包。**P3**（触发驱动，不急于工程化） |
| **G11** | **Domain-Oriented Observability（领域探针优先）** | AGENTS.md 明文；`*/application/observability/*_probe.py` 一族 | 我们有 runtime-manager facade trace（翻译透视落库）+ 审计事件结构化（REQ-224 契约）；**无「探针」这一层抽象** | 观测面/审计事件设计时按「领域探针」建模（业务语义命名、与日志解耦）。**P3** |

### 2.4 不建议借鉴 / 待核验

| 项 | 判定 |
| --- | --- |
| 企业级 IAM 全套（Keycloak + SpiceDB + 多租户 + API Key 生命周期 + 工作区/团队继承） | **不借鉴**：远超「单机可部署 + 教学」定位（D-O11）；其平台级鉴权与我们的部署边界（16 号 §3.0）、服务间零鉴权口径冲突 |
| Apache AGE 作为图存储 | **不借鉴**：15 号台账 v2.18 已评估（AGE release 恒 `-rc`）；维持 SQLite 三表 + KB-9 预留 `KGStore` 抽象 |
| dlt / FastAPI / FastMCP / pytest-archon 等 Python 栈直接引入 | **不借鉴**：D-O15 不接入 Python 运行时依赖（零 venv）；**取其契约与分层思想，Go 侧重实现** |
| Tekton/Konflux 流水线、release-please/renovate、kustomize overlays 全套 | **部分**：CI/发布形态已有基线，不整体引入 |
| **「Bi-Temporal（双时序）」** | ⚠️ **待核验**：README 描述为「Enterprise-Ready Bi-Temporal Knowledge Graph Platform」，但**仓库文件树 1690 条路径中 `bitemporal`/`bi-temporal`/`valid_time`/`valid_from`/`transaction_time` 命中 0**；实现层可见的是 `instance_change_record`（实例变更 **before/after 属性快照** + 逐属性差异）与 mutation run 时间戳——属**变更日志式历史**，**未见双轴（有效时间 / 事务时间）建模**。结论=标语为愿景，如需采纳须自行设计双轴 |

---

## 三、两项目的收敛与分歧（对本项目的信号）

**三处强收敛（可信度更高的借鉴信号）**——两个独立项目不约而同：

| 收敛点 | kubernetes-ontology | Kartograph | 我们的对应缺口 |
| --- | --- | --- | --- |
| **关系/边是带 domain/range/inverse 的一等声明** | `RelationSpec{Kind,Domain,Range,InverseOf}` 单一事实源 | `EdgeTypeDefinition{source_labels,target_labels,inverse_of,bidirectional}` | spec_json 关系缺 **inverse/双向声明**（→ K4/G3） |
| **事实要带「来源/推导/可信度」** | `EdgeProvenance{SourceType,State,Resolver}` | run 元数据 + before/after 变更记录 + 数据源/会话溯源 | spec 关系与实例断言**无来源与可信度**（→ K1/G2/G6） |
| **消费面必须只读且显式防御** | 只读 RBAC + `readSecrets` 默认关 | 关键字拒绝集 + 自动 LIMIT + 超时 + 内部属性剥离 | 我们已有 SELECT 白名单，**缺结果截断/超时/剥离等契约细节**（→ K8/G4） |

**一处根本分歧（战略级对照）**：kubernetes-ontology **明确反对「本体数据库优先」**，主张先让事实内核可信、语义映射层长在内核之上；Kartograph 则**直接以图数据库为地基**做平台化。这与我们既有拍板高度一致——**D-O1（构建与运行解耦）+ spec_json 为主形态 + 本体=模型层（57 号「不建议 Action/Function」）**，即「事实/模型先行，存储与推理是可换后端（KB-9 `KGStore` 抽象预留）」。**该分歧不产生新决策，但为既有立场提供了第三方论据**（K5）。

---

## 四、结论

1. **两项目均不引入代码**，但**合计 20 条借鉴点（K1~K9 / G1~G11）**，其中 **P2 九条**集中于两处：
   - **模型层表达力与可信度**：K1 边/断言溯源 + K3 身份键命名空间化 + K4/G3 关系规格单源多出口与 inverse 声明 + G3 组合不变量质量门——**四处可一轮同批设计**（与 REQ-235 lossy 动线、REQ-249 同名歧义、REQ-268 数据属性同族）。
   - **消费与写路径契约**：K2 本体消费契约 + G2 变更操作契约 + G4 MCP 防御细节 + G5 出处原文工具 + G6 运行元数据——**共同指向 05 号「消费与审计」的下一轮**（与 REQ-224 载荷轮天然衔接）。
2. **G1（架构边界可执行化）是唯一「与本体功能无关但价值确定」的一条**：三服务 + 多 agent 并行开发下，边界测试是低成本防侵蚀手段。
3. **K5/G8 作参照档、G7 作设计储备**，均**不立项**；本轮为**纯调研借鉴，无新需求行**（沿 59 号 Protégé 先例）；若开发者拍板采纳，按「撞号查 [18 号注册表](../../../docs/18_REQ编号注册表.md) → 03 需求档立项 → 回写方案档」流程执行。
4. **诚实边界**：①Kartograph 自述 experimental，多组件未达 MVP，其「Bi-Temporal」为标语（见 2.4）；②kubernetes-ontology 为个人项目、3★、末次推送 2026-06-02（**近四个月未更新**）——**仅供设计借鉴、不可作依赖**；③本文未做全量代码统计，结论以所读文件为限。

---

## 五、关联档

- **同域对照**：[57 Topology 式本体系统借鉴分析](57_Topology式本体系统借鉴分析_20261001.md)（V1 版本发布态）/ [59 Protégé 架构与借鉴分析](59_Protégé架构与借鉴分析_20261002.md)（P1~P8）/ [23 本体_开源实现方案借鉴研究](23_本体_开源实现方案借鉴研究.md) / [08 开源复用与自研边界分析](08_本体_开源复用与自研边界分析.md)（D-O5）
- **运行时与消费面**：[39 本体运行时与 MCP 服务开源方案调研](39_本体运行时与MCP服务开源方案调研.md)（我们已有 6 只读 onto_* 工具的来路）/ [34 open-ontologies 借鉴与引入分析](34_open-ontologies借鉴与引入分析.md)（119 MCP 工具对照）/ [90 本体_底层实现原理](90_本体_底层实现原理.md)
- **缺口来源**：[52 本体模块产品与架构演进分析](52_本体模块产品与架构演进分析_20261001.md) / [60 多渠道高质量本体平台缺口分析](60_多渠道高质量本体平台缺口分析_20261002.md)（H1~H12）
- **实现状态**：[00_本体模块](../00_本体模块.md) 及其 [01 学习中心](../01_学习中心.md) / [02 构建](../02_构建.md) / [03 资产](../03_资产.md) / [04 运行](../04_运行.md) / [05 消费与审计](../05_消费与审计.md)
- **架构权威**：[04_本体_方案设计](../../../docs/04_本体_方案设计.md)（§3 运行平面 / §4 消费审计 / §6.3 已知边界）、[03_本体_需求文档](../../../docs/03_本体_需求文档.md)（§5 决策 D-O1/D-O5/D-O15/D-O19）
- **登记**：[15 开源项目及论文登记簿](../../../docs/15_开源项目及论文登记簿.md) 第七类（两项均 🔭 对照参考）
