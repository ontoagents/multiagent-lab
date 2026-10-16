---
module: 本体
topic: HelixDB / helix-foundry 深度调研（本体的构建、物化与使用）
desc: 数据驱动本体自动推断（三源证据 + union-find 合并 + 去蕴含边 + record/activity 分道）→ 提案验证 → generation 原子物化 → Explorer/Analyst/对象动作消费面；附 HelixDB 引擎本体（三模态一体 +「图库存本体但不用图库 schema」的关键反例）与 A/B/C 三层借鉴清单
synced: 2026-10-11
---

# HelixDB / helix-foundry 深度调研：本体的构建、物化与使用

> 状态：v1.2 ｜ 日期：2026-10-11（v1.2 交付回写 2026-10-12）｜ **v1.1 立项回写**：开发者指令「采纳建议，立项，当前agent不开发由其他agent开发」——本档 A/B 层建议已在 03 号需求档 §2.27 立项为 **REQ-292~295**（B1 扩注 REQ-278），决策 **D-O24**；**本轮只立项不开发，开发移交其他 agent**。去向索引见 §7.0，原「未立项」口径见 §8 第 7 条（已更新）｜ **v1.2 交付回写**：REQ-292~295 已由 Helix 线 agent 于 2026-10-12 **一次性交付收口**（M96=292 编辑三层与确定性身份／M98=293 装载原子对账／M99=295 证据分级·notes·诚实进度／M100=294 活动流判据；M97 已被并行伴生线 REQ-296 占用顺延）——实现落点与诚实边界见 03 号 §2.27 v1.15 交付注与 §8 v1.15 迭代行；单测 9 组+API 冒烟 33/33+UI headless 7/7（smoke/req292/）｜ 一手材料：GitHub API + raw 文件级精读 `HelixDB/helix-foundry` @ `main`（298 条目 / 264 文件）与 `HelixDB/helix-db` @ `main`（3,285 条目）
> 调研动因：开发者指令「深入调研一下本体相关的开源项目 HelixDB/helix-foundry，梳理其对本体构建、使用方面的方案和实现，分析当前项目可以借鉴的地方」
> **本档定位**：一手源码级技术调研（算法 + 数据结构 + 发布/消费路径），不是功能罗列。同为公司数据本体（Palantir Foundry 开源替代）路线的前辈档见 [82 号 OpenBKN 构建与运行深度分析](../../09_OpenBKN专题/82_OpenBKN构建与运行深度分析_20261016.md) 与 [22 号开源本体与语义层赛道全景](22_开源本体与语义层赛道全景.md)——三者是**同一赛道的三种截然不同解法**，建议对照读（见 §7.4）。
> 关联：REQ-102（qualitygate）/REQ-110（claim 溯源）/REQ-157（merge-preview 评审动线）/REQ-233（资产检索）/REQ-236（启动对账）/REQ-249（同名歧义）/REQ-278（变更影响分析）/REQ-279（蕴含保持切片）/REQ-290（消费观测装载态）；04 号《运行》、05 号《消费与审计》、02 号《构建》；62 号 K1~K3、65 号 B1~B3/B5

---

## 0. 一句话结论

**Helix Foundry 把「本体从哪儿来」的答案，从「让人建模」改成了「让数据自己投票」**——它不用 LLM 生成结构，而是用**一次 DuckDB SQL** 对所有候选键列做值重合自连接，配合数据库声明的外键、列名相似性三路打分，**确定性地**推断出跨数据源的实体合并与关系，并把每条推断都附上可读证据（哪些表按什么比例匹配 / 只有忽略大小写才匹配 / 完全没连上）。

**对我们最直接的三条价值**：①「结构确定性推断 + LLM 只写描述」的职责切分；②「重算不丢人工编辑」的 base + overlay + audit 三层；③「两条路径到同一目标时只保留一条」的去蕴含边判据。

同时它也是一个**清醒的反例**：它把本体塞进图数据库，却**完全不用图数据库的 schema 能力**——这在提醒我们「图存储 ≠ 本体运行时」。

---

## 1. 关键事实（2026-10-11 快照）

### 1.1 两个仓库

| 项 | Helix Foundry | HelixDB |
| --- | --- | --- |
| 仓库 | `HelixDB/helix-foundry`（上层应用，与 HelixDB 同团队） | `HelixDB/helix-db`（引擎） |
| Star / Fork / Issues | **1,103★ / 162 fork / 17 open issue** | **6,178★ / 383 fork / 22 open issue** |
| 语言 / 许可 | TypeScript / **Apache-2.0** | Rust / **Apache-2.0** |
| 创建 / 末次推送 | 2026-10-07 / **2026-10-11** | 2024-11-23 / 2026-10-09 |
| 仓库规模 | 298 条目 / 264 blob / `size` 1,231 KB | 3,285 条目 / `size` 54,120 KB |
| 自建文档 | README、**AGENTS.md（62KB agent 安装 runbook）**、CLAUDE.md、compose.yaml、**docs/ 8 份**（GUIDE / DESIGN / API / VALIDATION / OPERATIONS / HOSTING / onboarding） | README、AGENTS.md、**docs/languages/helixql.json**、docs/cypher.md、docs/codebase-map.md |
| 自述 | "Your company's data, connected into one ontology, on your own computer." | "OLTP graph database with native vector and full-text search built in Rust on Object Storage." |

> 星数口径提醒：第三方报道为「589★ / 719★ @ 3 天」（Reveneau、AlphaSignal、MindPattern 均 2026-10），本档 1,103★ 为 GitHub API 直接读数。**4 天破千属热度爆发，不代表稳定性**，引用须注明快照日。

### 1.2 它对我们最重要的身份标签

它是继 OpenBKN（82 号）之后，**第二个开源对标 Palantir Foundry Ontology 的项目**，且二者解法完全相反（见 §7.4）。

### 1.3 一句话产品

把 PostgreSQL / MySQL / Stripe / WorkOS / PostHog / REST / S3 / 文件上传的企业数据接进来，**自动提议一个跨来源的本体**，存进 HelixDB（图 + 向量 + BM25 三模态引擎），用不可变 Parquet 快照 + 隔离 DuckDB 进程做分析，最后由 Analyst 用自然语言提问、返回**执行过的 SQL** 与**快照引用**。

---

## 2. 架构：五个服务、三层数据

### 2.1 服务切分（GUIDE §Architecture 原文）

| 服务 | 职责 |
| --- | --- |
| **App** | React UI + 仅 loopback 的 API + token 校验 + 发布与对象动作 |
| **Worker** | 持久化 AI 运行、定时导入、管线执行、主动提案 |
| **Executor** | **一次性 DuckDB 子进程**——声明式输入、隔离 SELECT 执行 |
| **HelixDB** | 工作区元数据、API token、作业、提案、**本体记录与边** |
| **Ollama** | 本地结构化推理（默认 `qwen3:4b`，digest pin 在 `docs/models.json`） |

三个 Docker 卷：`helix-data`（HelixDB 元数据）、`foundry-data`（Parquet 快照 + 产物 + 分批上传）、`model-data`（Ollama 权重，不备份）。

### 2.2 它实际用到 HelixDB 的哪一档 API —— 关键事实

`apps/api/src/store.ts` 的 `HelixStore` 用的**不是** HelixQL 的 `.hx` schema 文件，而是 **TypeScript SDK 3.2 的低层 builder DSL**（走 `POST /v2/query`）：

```ts
import { Client, readBatch, writeBatch, g, SourcePredicate, Predicate,
         NodeRef, EdgeRef, BatchCondition, IndexSpec } from "@helix-db/helix-db";

export class HelixStore implements Store {
  client = Client.server(config.helix);
  // 唯一的图 label：所有元数据塞进 FoundryRecord 节点 + FoundryRelationship 边，
  // 用唯一索引模拟 KV
}
```

`init()` 建三个索引：`key`（**nodeUniqueEquality**）、`scope`、`scopeKind`（nodeEquality）。
每个资源是一条 `FoundryRecord` 节点，属性只有 `key = scope:id`、`scope`、`scopeKind = scope:kind`、`doc = JSON.stringify(resource)`——**整份本体定义、提案、审计记录是以 JSON blob 挂在节点上的**；只有对象之间才落成真的 `FoundryRelationship` 边（带 `scope`、`generation`、`relationId`、名称）。

> **必须点出的设计判断**：它的「本体」在图里是 **个体层（ABox）真图 + 模式层（TBox）JSON 文档**。图不承担类型约束、属性完整性、关系端点类型检查——这些全在应用层 `assert` 里做（见 §5.2）。这与 62 号 `kubernetes-ontology` 的 K5 同族第三方背书：**反对 "ontology-database-first" 漂移**。

### 2.3 HelixDB 引擎本身

三模态一体（README 三栏原文）：

| Graph | Vector | Full-text |
| --- | --- | --- |
| 有标签属性图，建模实体 / 关系 / 类型属性 | ANN 近邻搜索，**可被图遍历预过滤** | 节点与边属性上的 **BM25** 关键词检索 |

语句面三条并行：

1. **native DSL builder**（各语言 SDK，`g().n_with_label(...)` / Rust `#[query]` 宏），**没有 build / deploy 步骤**，所有 SDK 产出同一份 JSON request → `POST /v2/query`；
2. **Cypher profile**（`POST /v2/cypher` + `/explain` + gRPC `ExecuteCypher`）——明确声明是 **supported profile，不是完整 openCypher 兼容**；同一份存储无需迁移；**编译期判断读写**，云端读写分流，只读 key 对写语句返回 403；
3. **HelixQL 遗留**（`docs/languages/helixql.json` 仍定义 `N::` / `E::` / `V::` schema 语法、`QUERY name(...) => body`、`schema::N` 版本号），但**当前 `crates/` 下已无 helixql crate**（只有 `ast / planner / db / cli / cypher / server / graph-algorithms / metrics` 等），主干已是 builder + Cypher。

> **Foundry 一项向量能力都没用**：它的 Analyst 走 NL→SQL，不是混合检索。这是 §7.3 D2 的由来。

---

## 3. 本体构建（本项目的核心关切）—— `suggestOntology` 全流程逆向

**文件**：`apps/api/src/discovery.ts`，1,529 行。**入口**：`suggestOntology(store, scope, { datasets, sources, signature })`（L285–841，约 560 行）。

文件头注释就是它的全部哲学：

```ts
// Suggests a condensed ontology from the connected data: datasets that describe the
// same real-world thing in different systems are merged into one entity, and value
// overlap between key columns becomes links. Deterministic; one executor call.
```

**「Deterministic; one executor call」**——两次关键声明：整个推断**不调用模型**，只用**一次** executor（DuckDB）调用。AI 在哪儿用？GUIDE 明写：

> "When AI is ready, a background run rewrites **only its summary, descriptions and link names**."

即 **结构 = 确定性算法产出；措辞 = LLM 润色**。这是 §7.1 的 A1。

### 3.1 输入：不是原始表，而是 profile

只吃已激活且已 profile 的数据集（L292），每个 `Info` 承载：

```ts
type Info = { d: Resource; index: number; table: string; source: string;
              system: string;   // "connection:<id>" | "sample:<src>" | "dataset:<id>"
              columns: ColumnProfile[]; rows: number;
              key?: string; alt: Set<string> }
```

`ColumnProfile` 提供 `name / type / primaryKey / distinct / nulls`。**整个推断建立在「每列唯一值计数 + 非空计数 + 是否主键」这类极薄统计量上**，除 §3.3 的值集合外不看数据内容。对我们「由 CSV / 知识库构建本体」的启示：**不必把数据喂给推理，只需喂 profile + 键列值集**。

### 3.2 第一步：给每个表找主键（`ownKey`，L210–224）

四级降级，每级都有语言学理由：

1. 列名 slug 恰为 `id`，或 `*_id` 且词干是表名单数（`ticket_id` in `support_tickets`）；
2. 任一 `id` 结尾的唯一列；
3. 任一 `key/code/number/sku/slug/handle/username` 结尾且非 email；
4. 非整型且非 email 的唯一列 → 最后退到整型唯一列。

同时把「主键之外、唯一、键名且非 id 名」的其它列收进 `alt`（L312–323）——**备用身份键（email、外部 code）单独一路**：它们后续能做 1:1 合并，但不应被用来挑 anchor。

类型门槛 `keyType = /^(VARCHAR|UUID|U?(TINY|SMALL|BIG|HUGE)?INT(EGER)?)$/`——只有字符串 / 整数 / UUID 能当键。

### 3.3 第二步：一次 SQL 完成全键对值重合（`overlap`，L1015+）

算力核心：把**所有候选键列的值洗成一张长表**再自连接。

```sql
WITH vals AS (
  SELECT c, d, i, s, v FROM (            -- c=候选键列编号, d=所属表, i=是否整型, s=词干前3字母
    SELECT 0 AS c, 0 AS d, 0 AS i, 'cus' AS s, CAST("customer_id" AS VARCHAR) AS v
      FROM "ds_accounts" WHERE "customer_id" IS NOT NULL
    UNION ALL
    SELECT 1, 1, 0, 'ema', CAST("billing_email" AS VARCHAR) AS v
      FROM "ds_invoices" WHERE "billing_email" IS NOT NULL
    UNION ALL ...
  ) AS x WHERE trim(v) <> ''),
ex AS (SELECT c,d,i,s,v, count(*) AS n FROM vals GROUP BY c,d,i,s,v),                       -- 精确口径
ci AS (SELECT c,d,i,s, lower(trim(v)) AS v, count(*) AS n FROM vals GROUP BY c,d,i,s,lower(trim(v))),
cnt AS (...), cnt_ci AS (...),
pe AS (...matching('ex', 'count(*) AS m_exact, sum(a.n) AS a_rows_exact')),
pc AS (...matching('ci', 'm_ci, a_rows_ci'))
SELECT pc.a, pc.b, coalesce(pe.m_exact,0) AS m_exact, pc.m_ci, ...
WHERE pc.m_ci >= 0.5 * least(cca.d_ci, ccb.d_ci) OR pc.m_ci >= 20 ...
ORDER BY pc.m_ci DESC
```

`matching()` 的三路分支（L1031–1040）：

| 分支 | 条件 | 为什么这样限制 |
| --- | --- | --- |
| ① 非整型 × 非整型 | `a.i = 0 AND b.i = 0` | 字符串键自由配对 |
| ② 含整型 | `a.s = b.s AND (a.i = 1 OR b.i = 1)` | **整型键必须词干相同**——防 `id = 1..N` 序列号撞车产生笛卡尔积（注释原话：serial ids never multiply out across unrelated tables） |
| ③ 声明外键对 | 显式 `(a.c = X AND b.c = Y)` 白名单 | **已声明的外键无视名字差异配对** |

出参每行是一对待判键列，带：`m_exact / m_ci`（配对成功的**不同值个数**）、`a_rows_exact / a_rows_ci`（命中行数）、`a_distinct / a_distinct_ci`（A 侧去重值数）、`a_rows`（A 侧非空行数）。

### 3.4 第三步：把行变成 Edge，附加三档证据（L367–441）

依次过滤与打标（原文注释解释了每一条为什么排在这里）：

1. **目标必须是对端表的 own key / alt key**（L377–386）——专门用来**掐掉传递重合**，注释举例：`invoices.customer_id → subscriptions.customer_id` 不接受。
2. **整型词干校验**（L384）：`(A.integer || B.integer) && !sameStem(...)` 则拒。`sameStem` 允许前缀互通（`org_id` ↔ `organization`，最短一方 ≥ 3 字符）。
3. **大小写口径选择**（L387–390）：列名含 email，或 `a_rows_exact < a_rows_ci`，则**改用忽略大小写口径**。
4. **覆盖率闸门**（L391–394）：非声明边要求 `m/distinct ≥ 0.8` **或** `matched/rows ≥ 0.9`；声明边只要求 `m > 0`。
5. **是否「常规外键」**（`conventional`，L400–404）：两个上传文件之间，`subscriptions.customer_id` 这种常规外键算 link，而 `stripe_customer_id`、`external_id` 这种**带系统名前缀的**仍能意味同一条记录。
6. **`identity` 判定（可合并为同一实体）**（L406–414）：五个条件同时成立——A 侧列是该表唯一键、**B 侧列是对端 own key**、两侧**不在同一 system**、精确匹配达标（`m_exact/a_distinct ≥ 0.8 || a_rows_exact/a_rows ≥ 0.9`，注释：合并需要精确匹配，这样合并后的记录 join 时不需要 `lower()`）。
7. **`extension` 判定（1:1 扩展表）**（L416–423）：`user_profiles.user_id` 这类「自己的主键就是别人的主键」的表：同 system、非 identity、且名词对得上（B 表名 = A 列词干）。
8. **既非 identity 也非 extension 且 A 侧是键 → 丢弃**（L424）——剩下的才是 references（引用关系）。

最后打证据等级（L435）：

```ts
evidence: isDeclared ? "declared" : nameMatch ? "name" : "values"
// 排序权重：const rank = { declared: 2, name: 1, values: 0 };
```

> **借鉴点 A2 的来源**：关系的溯源不是「有 / 无」，而是 `declared > name > values` **三档强度**，且每档都能追到具体原料（DDL 外键 / 列名与词干 / 实际值重合率）。62 号 K1 只给了「边要有溯源声明」，这里补上了「溯源的强度分级」。

边拉通后按强度全局排序（L462–471）：`exact → evidence 档位 → 匹配率 → 名字字典序`（最后一项是为了**结果不依赖 locale**，注释：Code-point order）。

### 3.5 第四步：序列号陷阱的最后一道防线（L442–461）

```ts
// Serial integer ids overlap by chance, so two tables keyed by `id` are only the
// same records when a declared key or a non-integer match between them agrees.
const serial = e => e.identity && !e.declared && e.A.integer && e.B.integer
                    && e.A.ds.key === e.A.name && e.B.ds.key === e.B.name;
edges = edges.filter(e => !serial(e) || edges.some(o =>
  (o.identity || o.A.field) && !o.A.integer &&
  ((o.A.ds === e.A.ds && o.B.ds === e.B.ds) || (o.A.ds === e.B.ds && o.B.ds === e.A.ds))));
```

**两个自增主键表要合并，必须有第三方非整型证据佐证**（例如两边 email 列也重合）。这是从数据里区分「巧合」与「同一」的确定性做法。

### 3.6 第五步：union-find 合并实体（L473–491）

```ts
// Union-find over 1:1 cross-system links; an entity holds at most one dataset per
// system, which stops an email match from merging accounts with users.
```

一条极漂亮的约束：合并时若 `systems[a]` 与 `systems[b]` 有交集（同一 system 里已经有两个数据集），则**该边降级为非 identity**。效果是——PlanetScale 的 accounts 与 users 分别与别的表合并互不干扰，但不会因为 `account.email ≡ user.email` 而把两个表塌成一个实体。

落在 `tree`（被采纳的生成树）+ `corroborating`（同实体内的额外佐证边，后面给 note 提供「另有 N 处复核」证据）。

### 3.7 第六步：reference 去重与「谁指向谁」仲裁（L492–506）

```ts
// A key matching several tables refers to the one named for it: user_id points
// at users, not at user_profiles, which is keyed by user_id too.
const named = e => noun(e.B.ds.table) === stem(e.A);
const references = candidates.filter(e =>
  e.declared || named(e) || !candidates.some(o => o.A === e.A && o.B.ds !== e.B.ds && named(o)));
```

**优先保留「名词对得上」的那条边**，没有才用第一条。这解决属性图里典型的「外键到底指向哪个父表」歧义。

### 3.8 第七步：去蕴含边（`withoutImplied`，L943–1009）—— 本档最锋利的一条

```ts
// A direct link A -> C that a two-hop path A -> B -> C already gives (invoice ->
// subscription -> account) is left to traversal, once the data shows both routes
// reach the same record. Checked in one executor call.
```

枚举 `(direct: A→C, first: A→B, second: B→C)` 三元组，要求两条路由真的逐行可比（起点同表、第一跳落点 = 第二跳起点、落点同列），然后一条 SQL 判定：

```sql
SELECT i AS k, count(*) AS n,
  count(*) FILTER (WHERE b.x IS NOT NULL AND <end>(a.direct_col) = <end>(b.first_col)) AS agree
FROM invoices a LEFT JOIN subscriptions b ON <hop>(a.sub_id) = <hop>(b.sub_id)
WHERE a.direct_col IS NOT NULL
```

**判据：`agree ≥ 0.95 × n`（且 n > 0）→ 该直达边是蕴含的，删掉。** 并且有一条不能忽视的安全性约束（L994–1007）：

```ts
// Paths must stay intact: never drop a hop of a dropped link, or drop a link
// through a hop that was dropped.
```

即**删边不能删出断链**——用 `implied` / `hops` 两个集合互相锁定。

> **为什么重要**：从数据反推本体时，最常见的失败是「把每个可达关系都显式声明出来」，于是本体变成数据关系的转写而非抽象。它给了一个可执行、带阈值、无需人参与的判据与实现。这与我们的 REQ-278（变更影响分析）与 REQ-279（蕴含保持切片）同族：一个管「少建冗余关系」，一个管「保住蕴含」。

### 3.9 第八步：把 link 重锚到 anchor，修正匹配计数（`throughAnchor`，L877–937）

一条 link 的 `toDatasetId` 可能不是目标实体的 anchor dataset（实体是多个数据集拼出来的）。它构造嵌套子查询沿 member 的 `via` 链走到 anchor：

```sql
FROM ds_from a WHERE <ci>(a.from_col) IN (
  SELECT v FROM ( SELECT <str>(j0.col) AS v FROM ds_to j0
                  WHERE <str>(j0.via_col) IN (
                    SELECT v FROM ( SELECT <str>(j1.col) AS v FROM ds_anchor j1 ) ) ))
```

取回的行数按 `Math.min(l.matchedRows, n)` 处理——**只调低不调高**（保守），因为多跳走不到的行 join 后就是孤儿。

### 3.10 第九步：record vs activity 分道（L598–627）

```ts
// Large event streams feed metrics but are not stored as records. A large orders
// table is still a record, so the stream must also look like one.
const eventLike = x => x.source === "posthog" || x.rows > 50000 ||
  words(x.table).map(singular).some(w =>
    "event activity log track tracking click pageview visit session audit".split(" ").includes(w));
const streams = new Set(drafts.filter(d =>
  d.members.length === 1 && d.anchor.rows > 2000 && hasTime(d.anchor) && eventLike(d.anchor) &&
  refs.some(r => r.from === d) && !refs.some(r => r.to === d)));   // 只出不入
```

五个条件：单成员、行数 > 2000、有时间列、名字像事件、**只有出边没有入边**。命中则 `kind = "activity"`，**不物化成对象**，只参与指标计算。

> 这是「什么该成为实例、什么只该成为统计」的一条**数据驱动判据**。我们的 spec_json 有 Instance 层，但「灌装时是否生成实例」目前由用户决定；这条提供了自动推荐的口径。

### 3.11 第十步：命名、属性前缀、notes、hash（L637–840）

- **实体名**：`entityName(table)` → 去限定词（`marketing raw stg dim fact tbl app`）+ 末词单数化 + 首字母大写；冲突时加来源后缀或序号（L639–655）。内置不规则复数表里 `people→person`、`analyses→analysis` 等 16 条。
- **属性名冲突**：合并实体时非 anchor 表的列加来源前缀（`workos_name`），已占用则退到 `source_table` 或加序号（L1110–1135），**大小写不敏感去重**。
- **link 名**：由列名正则映射为人类可读短语（L1085–1100）：`customer|billing` → "billed to"、`owner` → "owned by"、`user|distinct_id|person|actor|member` → "performed by"、`campaign|utm` → "came from"、兜底 "references"。
- **notes（四类人类可读证据）**：
  - `merged`：`"Account combines PlanetScale accounts with WorkOS organizations (1,204 of 1,300 match, confirmed by email)."`（`confirmedBy` 会列出实体内其它佐证列，且**排除当前 join 的那一对**——注释：反向读 join 列不算第二条印证，L720–747）
  - `unmatched`：`"96 support tickets don't match any account."`
  - `case`：`"312 emails only match when ignoring case."`
  - `unlinked`：`"Campaign isn't linked to the other data."`；无唯一列的表进 `unplaced`，仍参与 dashboard，**setup 不停工**（注释：Data without a recognisable key still builds dashboards, so setup never stalls）
- **`hash`**：对（实体 id / kind / anchor / key / members + link 全部列）做 digest，`publishProposal` 用它做防陈旧（§4.4）。

返回值 `SuggestedOntology { signature, hash, summary, described, entities, links, notes, unplaced }`，每条 link 带 `totalRows / matchedRows / caseInsensitive / evidence`。

---

## 4. 确认与物化：从建议到真图上跑

### 4.1 `confirmOntology`（L1162–1214）：用户审阅

用户只能做两件事：**排除实体**（连带删 link、删对应 note）与**改名**。三条校验：名字非空、名字唯一、**保留至少一个 record 实体**。并且如上所述，无唯一键的数据也要让 setup 走完。改动后**重算 summary**（涉及数量与来源计数）。

### 4.2 `ontologyBundle`（L1225–1447）：建议 → 可执行 Bundle

每个实体被编译成：

1. **一条 pipeline SQL**：`SELECT <列> FROM anchor a LEFT JOIN member m1 ON <cast>(a.k) = <cast>(m1.via_col) ...`（`grew` 循环按 via 依赖逐层展开，L1319–1338），输出一个新的 Parquet 数据集。
   > cast 只在需要时加：`type(m.via.column) !== type(m.via.targetColumn)` 才包 `CAST(... AS VARCHAR)`（L1328–1332）——不让不必要的 cast 污染执行计划。
2. **归一化键列**：若某 link 是 case-insensitive 或跨类型匹配，就在 pipeline 的 SELECT 里额外输出 `lower(trim(CAST(col AS VARCHAR))) AS <as>_key`（L1301–1315），rel 改为 join 到它上面。**数据缺陷不修复，而是写到多处：一处 normalizer 列 + 一处 link 上的 `caseInsensitive: true` 标记 + 一处给人类看的 case note。**
3. **一个 quality check**（L1347–1352）：
   ```sql
   SELECT count(*) = count(DISTINCT "id") AND count("id") = count(*) AS ok FROM "pipeline_X"
   ```
   即「每行一个 Customer」。**checks 与产物同包**。

规模约束：`records.slice(0, 20)`，需要 pipeline 的实体最多 10 个。

### 4.3 `validateBundle`（assistant.ts L108–350）：生成物必须跑通才能入库

这条既服务 AI 路线也服务 schema 路线，任何 bundle 都要过它一次：

- **命名唯一性**：pipelines / ontology / dashboards / relationships 四类各自唯一（L154–165）。
- 每条 **pipeline**：真跑一次（limit 20）取回 profile；**单输入管线额外算出被丢弃的行**（`rejected:` 预览，L227–235）：`SELECT a.* FROM ds_in a WHERE NOT EXISTS (SELECT 1 FROM out b WHERE a.k = b.k)`；然后执行它的每条 check，全 true 才通过（`Object.values(rows[0]).every(v => v === true || v === 1 || v === "1")`）。
- 每个 **object type**：primaryKey 必须在 dataset 里存在且 `primaryKey` 为真（"Primary key must be unique and non-null"），属性必须存在。
- 每条 **relationship**：跑一次 LEFT JOIN 计数 SQL，产出 `matched / total / pairs` 与 **cardinality 字符串**（由两端列是否主键推出 `one-to-many` 等，L285–290）——**基数是量出来的，不是声明来的**。
- 记录 `inputVersions`（每个被引用数据集的当前版本）与 `baseGeneration`。
- **宽容修复，但不臆造**：`resolveDataset` 只在名字**唯一确定**一个授权数据集时才解析；小模型常把 type 配错 datasetId，若某 dataset 恰好同时满足「主键唯一 + 全部属性都在」且**只此一家**，才纠正过去（L130–150，注释：Small models sometimes pair a type with the wrong dataset ID）。

### 4.4 `publishProposal`（L1504+）：generation 原子发布

四重防陈旧（全部是 409 而非静默）：

```ts
assert(hash === proposal.data.hash, "Proposal changed; review its current version", 409);
for (const [datasetId, version] of Object.entries(report.inputVersions))
  assert((await store.get(scope, datasetId))?.data.activeVersion === version,
         "Input data changed. Revalidate this proposal.", 409);
assert((beforeGeneration || null) === (report.baseGeneration || null),
       "Published definitions changed. Revalidate this proposal.", 409);
```

物化过程：

| 步骤 | 实现 | 失败语义 |
| --- | --- | --- |
| 新 generation | `const generation = randomUUID()` | — |
| 每个 object type 一次 executor query，`resultBatches` 分批 500 读回 | 对象 id = `generation + "_" + logicalId`，`logicalId = digest(typeId + ":" + key).slice(0,32)`，`base: row` | **行数必须等于快照行数**，否则抛错且「workspace 未发布」（L1659–1662） |
| **人工对象带过来** | `manualObjects` 若在新数据里已不存在，仍以新 generation 重新 stage | 手建对象不被重算吞掉 |
| 每个 relationship 一次 query | 用 identities 映射 → 落成 `FoundryRelationship` 边 | — |
| **关系编辑保留** | `relationEdits` 按 `fromLogicalId:toLogicalId:name` 复用 | 重算不丢人工关系 |
| 切指针 | workspace 的 `activeGeneration` 一次性切换 | 旧的保持活跃直到切换完成（原子） |

进度上报是 **best-effort，绝不因它失败发布**（注释 L1597–1598：Saving it is best effort and never fails the publication）。

**回滚**：rollback = 重验证旧定义 + 对**当前快照**重新物化（不是简单倒回指针），过程中保留 manual actions 与其审计历史。

---

## 5. 本体的使用面

### 5.1 Explorer（图消费）

`GET /api/v1/workspaces/:w/graph`（routes/ontology.ts L113–188）：

- 先按 `activeGeneration` 过滤 objects / relations（**跨 generation 不串图**）。
- **无 focus**：全图按 **degree 降序**取 top-N（默认 1500，上限 3000）——注释原话：when it is too big to draw。
- **有 focus**：以 `${generation}_${focus}` 为起点做 **BFS，`depth ≤ 3`**，节点数不超过 limit。
- 节点返回 `type / name / label / degree`；`label` 由 `recordLabel`（L37–51）按优先级链给出：`first_name + last_name` > `name|full_name|display_name|title|subject|company_name` > **`^e-?mail$`（只认自己的邮箱，注释：requester_email names someone else）** > `"<type> <截断 key>"`。

### 5.2 记录详情、邻居与 base / overlay / audit 三层

`GET /records/:logicalId` 返回三件套：`record`（含 `effective`）+ `neighbors`（双向、limit 500）+ `history`（审计前 30 条，按五个维度模糊关联 objectId）。

真正的三层来自 `effectiveObject`（context.ts 全文 14 行）：

```ts
// Ontology-owned: an object with its workspace edits applied.
export async function effectiveObject(store, scope, r) {
  const overlay = await store.get(scope, "overlay_" + r.data.logicalId);
  return { ...r, data: { ...r.data,
    effective: { ...r.data.base, ...overlay?.data.values },
    edits: overlay?.data.values || {} } };
}
```

`base`（物化值）与 `overlay`（人工编辑）**分开存**，读取时合并。写侧护栏（`POST /actions/execute`，L249–333）：

- **必须先 preview**：preview 返回 `{ before, after, revision }`；execute 必须带 revision，服务端 `assert((overlay?.revision || 0) === b.revision, "Object was edited; refresh the preview", 409)`——**乐观锁**。
- **幂等键**：`idempotencyKey`（≥8 字符）→ `action_` + digest 作为资源 id；已存在则**校验收到的请求体 hash 一致**，不一致 409（"Idempotency key reused for different action"）。
- **可 revert**：`revert: true` 时把 overlay 值清空，而不是删记录。
- **字段白名单**：只能改 objectType 声明过的 properties，且**不能改 primaryKey**；类型必须与原值同 typeof。
- 每次都写一条 `kind: "audit"` 资源（actor / objectId / before / values / requestHash）。

> **这是我认为本项目最该抄的一条（B4）**：我们的资产编辑、CSV 灌装、由知识库重建，都存在覆盖人工编辑的风险；`base + overlay + audit + revision + idempotency` 这套组合拳成本不高而语义完整。

### 5.3 Analyst（只读消费）

- 三种 provider：本地 Ollama `qwen3:4b`（默认）／Claude `claude-sonnet-4-6`／OpenAI `gpt-5.4-mini`，**provider 之间无 fallback**。
- 预算三旋钮（`PUT /api/v1/workspaces/:w/provider`，owner 专属）：`maxCalls` 1–20 默认 20（每次 retry 给新预算）、`timeoutSeconds` 30–1800 默认 600、`dailyTokens` 1e3–1e7 默认 200,000。
- 上下文是**有界的** schema + profile + sample；走托管 provider 时对检出的敏感列做 masking（`allowRawData` 仅 API 可设，默认 false），且明说检测是启发式、要求用户自己 review source policy。
- 回答 = **执行过的 SQL + 快照引用**（citation 到具体 snapshot 版本）。
- 提案 vs 直答自动路由；路由决策在输入未变时复用。

### 5.4 执行隔离（Executor，`services/executor/src/engine.ts`）

- 一次性 DuckDB 实例，**内存 512MB / 2 线程 / 独立 temp 目录**，`allow_unsigned_extensions=false`、`autoinstall/autoload=false`（**禁扩展**）。
- 输入的 scope / version / table / file 全部过 `safePart = /^[a-zA-Z0-9_-]{1,100}$/` 才允许进路径（防目录穿越）。
- 生成 SQL 被**包成单条 SELECT**；多语句拒绝。
- 容器：`mem_limit: 2g`、`pids_limit: 128`、`cap_drop: [ALL]`、`no-new-privileges`、独立 `execution` 网络、**无凭证无外网**。
- 并发 2 槽；查询默认 120s、作业 900s；结果默认 2GB；spill 可配（`max_temp_directory_size`）。
- **优雅降级编排**：`oom_score_adj` 分级（请求 / executor 300 → worker 200 → HelixDB 最后被杀），注释说明为何不给 app 加 `mem_limit`（Node 堆会随之被压到 1.1GB，大工作区读图会超）。

---

## 6. HelixDB 引擎本体：作为候选評估

### 6.1 能力面

| 能力 | 说明 |
| --- | --- |
| 图 + 向量 + BM25 三模态 | 同一事务引擎；**pre-filter before ranking**（先用图遍历 / 结构条件过滤，再做向量与全文排序） |
| 原子多模态写 | 图写与向量写同提交 / 同回滚 |
| 声明式 schema 遗迹 | HelixQL `N::` / `E::` / `V::` + `QUERY ... =>`，带 `schema::N` 版本号 |
| 多语言 SDK | Rust 3.0 / TS 3.1 / Python 0.3.4 / Go 0.3.1，**统一 JSON request → `/v2/query`**，无 build / deploy 步骤 |
| Cypher profile | `POST /v2/cypher` + explain + gRPC；**supported profile 而非 openCypher 全兼容**；编译期判断读写并分流 |
| 部署形态 | embedded（进程内）／Docker／CLI `helix start dev`／Cloud；数据建于对象存储 |
| 索引与批处理 | `IndexSpec`（`nodeUniqueEquality` / `nodeEquality`…）、`BatchCondition` 条件批、`shouldAwaitDurability` |

### 6.2 结论：不建议引入为本项目本体运行时引擎

| 理由 | 说明 |
| --- | --- |
| **不是 RDF 引擎** | HelixDB 是 labeled property graph，没有 SPARQL、没有 RDF/OWL 语义。我们的主线是 spec_json → TTL → SPARQL（Oxigraph / Fuseki），引入它会**切断「TTL 装载 — SPARQL 查询」这条教学主链路** |
| 教学对照收益可由更低成本获得 | 我们已有 Oxigraph / Fuseki /（Cayley 占位）三套运行方案（04 号）；再加一个 property graph 引擎的边际收益低 |
| 部署面与 D-O15 精神 | 需多一个 Rust 服务 + 镜像 pin（`compose.yaml` 提示 **v0.0.10 起索引存储单向升级，旧镜像会拒绝**），与「单机可部署、零外部运行时依赖」方向相反 |
| 上游成熟度风险 | Foundry 创建仅 4 天，HelixDB 虽 6.1k★ 也是两年内项目且迭代快；作依赖需谨慎，作借鉴则无妨 |

### 6.3 但要借鉴它的检索形态（D1）

「**pre-filter → ranking**」（图结构先圈定候选，再由向量 / BM25 排序）是混合检索的标准答案形态。我们本体资产栏的搜索（REQ-233）、知识库 GraphRAG 的检索面都值得把它作为**目标形态的**参照——不是让我们引入它，而是让我们知道目标长什么样。

---

## 7. 对本项目可借鉴清单（三层 + 明确不做）

### 7.0 立项去向索引（2026-10-11 回写，D-O24；**2026-10-12 v1.2 四条全部交付 ✅**）

开发者已拍板采纳，本档建议按「同一交付批次可一起做」聚合为四条需求（避免碎片化占号），**只立项不开发，开发移交其他 agent**：

| 去向 | 覆盖条目 | 说明 |
| --- | --- | --- |
| **REQ-292**（P1，建议 M96） | **B4 + B5 + B6 + B9** | 本体编辑三层与确定性身份。**B5 是 B4 的技术前置**——overlay 按 `overlay_ + logicalId` 定位，身份键不稳则三层无从落地，故合并立项。**✅ 2026-10-12 交付（M96）**：迁移 043 overlay 表（logical_id=digest 前 32 hex）+saveSpec diff 提取+机器写入重放（重放前快照机器值作 base）+对象级三端点（revision 409/幂等键/身份键不可变）+「编辑保护」面板+审计落 onto_decision |
| **REQ-293**（P2） | **B3 + B7** | 物化与装载的原子对账——generation 原子发布/计数硬闸是「机制」，三重防陈旧是同一机制的「发布前置卫」，合并立项；依赖 REQ-292②。**✅ 2026-10-12 交付（M98）**：X-Onto-Triples 计数头→runtime-manager 引擎 COUNT 硬闸（不符中止且旧快照保持；计数未知 gate=skipped）+loaded_status v2（generation/行数/gate/loaded_at）+If-Match/base_version 防陈旧 409 三入口 |
| **REQ-294**（P3 触发驱动） | **B2 + B10** | 实例物化策略与去重约束——同属「物化哪些、如何防塌缩」的构建期确定性规则。**✅ 2026-10-12 随批交付（M100，触发驱动口径提前兑现）**：灌装活动流五条件 hint+skip_instances 不物化；B10 跨列值重合永不触发实体合并（单测锁定） |
| **REQ-295**（P2） | **A2 + A3 + A4 + A5** | 构建产出的证据分级、人话 notes 与诚实进度——同属「产出报告与进度反馈」面，零模型依赖。**✅ 2026-10-12 交付（M99）**：Relation.Evidence 三档（declared>name>values，Validate 枚举+merge 透传；KG/结构化=values、OWL 声明=declared；不入 TTL）+灌装/合并 notes 四类带数字+悬空目标降级注记（A5）+ai-draft job 实测轮次 progress（永不虚构百分比） |
| **REQ-278 扩注**（不占号） | **B1** | 去蕴含边作为其「结构冗余检测」子项，与影响分析同属结构图分析，同轮设计免二次返工 |
| **不占号（随文档轮/工程轮吸收）** | **A1 / A6 / C1~C6 / D1** | A1 已由 REQ-271~275（D-O23）承载，本档价值是把该口径推广到「由知识库构建」三策略与 CSV 灌装，作 REQ-292/295 的设计约束；A6 归 seeds 内容轮；C1~C6 为工程方法，占号无意义；D1 只取形态不引引擎 |

### 7.1 A 层：口径 / 产品原则（零代码，可即刻吸收）

| # | 借鉴点 | 落地建议 |
| --- | --- | --- |
| **A1** | **「结构确定性、措辞交给 LLM」的职责切分**——`suggestOntology` 零模型调用且标为 deterministic，AI 后台只重写 summary / description / link name | 我们的 REQ-271~275（自研 OntoChat 批次）与「由知识库构建」三策略：**候选类 / 候选关系 / 合并判定尽量由确定算法产出**，LLM 只负责命名与描述。收益：结果可重现、可 diff、可回放 |
| **A2** | **三档证据分级 `declared > name > values`**，每条关系携带 `evidence` 字段与排序权重 | 并入 05 号《消费与审计》的口径：**证据不是有无，而是强度**；把 62 号 K1（边级溯源声明）升级为「溯源 + 强度」 |
| **A3** | **notes 制度**：merged / unmatched / case / unlinked 四类，每条带具体数字与列名（"96 support tickets don't match any account"） | qualitygate 与「由知识库构建」的产出报告：从「扣分表」升级为「人类可核对的证据故事」 |
| **A4** | **永不虚构百分比与时间估计**——DESIGN.md 明文 "never invent a percentage or a time estimate"；进度只显示服务端实测计数（"12,345 records read"）；装饰性子步骤可在客户端节流渲染，**但结果永不早于服务端** | 我们的本体构建 / 导入 / LLM 生成进度条。这条纪律极便宜、信誉收益极高 |
| **A5** | **「退化但不停工」**：无唯一键的表进 `unplaced` 仍可做 dashboard（"setup never stalls"）；无连边的实体给 note 而非报错 | 我们质量门「告警通过 vs 拦截失败」的边界可参照：结构性缺失 → 降级 + 显式告知，而非让整条构建线卡死 |
| **A6** | **样本数据的「刻意不一致」叙事**——Loopwell 虚构公司 11 数据集，特意留坑：账单邮箱 ≠ 登录邮箱、无账号的工单、拼错的 campaign code | 我们的种子本体与学习内容：**真实的不一致才是好教学材料**；现有大型种子可借鉴此思路补 realistic gaps |

### 7.2 B 层：结构 / 算法（需设计，建议进载荷轮）

| # | 借鉴点 | 与主线的对接口 |
| --- | --- | --- |
| **B1** | **去蕴含边 `withoutImplied`**：判据 `agree ≥ 0.95 × n`，外加「删边不断链」的双向锁定 | 直接对口 **REQ-278（变更影响分析）** 与关系冗余治理：把「每个可达关系都显式声明」是反推本体的头号失败模式。建议作为一种结构冗余检测子项 |
| **B2** | **record vs activity 五条件自动判据**（单成员 + rows>2000 + 有时间列 + 事件名词 + 只出不入） | 回应 spec_json Instance 层「灌装时是否生成实例」的自动建议；与资产栏 CSV 灌装同族 |
| **B3** | **generation 原子发布 + 计数硬闸**（分批 500 读回、行数必须等于快照行数、否则中止且旧 generation 保持活跃） | 加固 **REQ-236 启动对账 / REQ-290 消费观测的 loaded_status**：把「装载成功」从布尔升级为「装载行数一致性 + 装载 generation 号」 |
| **B4** | **base + overlay + audit 三层**（`effective = { ...base, ...overlay }`，带 revision 乐观锁、idempotencyKey、revert、字段白名单、immutable primaryKey） | **最高优先级单条**：解决「由知识库重建 / CSV 灌装 / LLM 重生成覆盖人工编辑」的确定风险。建议作为独立 REQ 立项，落在 02 构建 + 03 资产 |
| **B5** | **确定性身份键 `logicalId = digest(typeId + ":" + key)`**（typeId 又是 `digest(typeName).slice(0,20)`） | 与 62 号 K3 canonicalId 命名脊同族，但给了三十行内的极简实现；直接对症 **REQ-249（同名歧义）** 与导出 TTL 时实例 IRI 的稳定性 |
| **B6** | **归一化键列 + 把数据缺陷写进模型元数据**（`lower(trim(CAST(x AS VARCHAR)))` 的 `_key` 列，link 标 `caseInsensitive`，并给 "312 emails only match when ignoring case" 的 note） | 我们灌装 / CSV 导入遇到大小写与类型不一致时目前是「归一化后静默通过」，应改为「记录 + 报告」 |
| **B7** | **三重防陈旧**（提案 hash + 输入数据版本 + 基础 generation），任一项不符 409 要求重验证 | 比 65 号 B3（plan_id 生命周期化）更完整——它还锁了**输入数据版本**。建议并入 **REQ-157 merge/preview 评审动线**的发布前校验 |
| **B8** | **checks 与产物同包、必须全 true**（"One row per Customer" 的 SQL 布尔表达式） | qualitygate（REQ-102）可吸收「check 随定义走、可重跑、可预览」的形态 |
| **B9** | **属性名冲突的确定性解法**（来源前缀 + `_2.._n` 后缀 + **大小写不敏感**去重 + 先到先得） | 我们「由知识库构建」合并时的属性名冲突目前缺少确定规则 |
| **B10** | **union-find 上的「每系统至多一个数据集」约束**（防 email 误合 users / accounts） | 我们虽无跨源合并动线，但「对话伴生生长」做概念去重时可借鉴此约束形态（同一来源不允许两套名词塌成一类） |

### 7.3 C 层：工程 / 产品方法（低成本，价值确定）

| # | 借鉴点 |
| --- | --- |
| **C1** | **AGENTS.md 作为交付形态**：README 直接给一段可粘贴的 prompt（"Paste this prompt into Claude Code / Codex / Cursor"），由 agent 读 AGENTS.md 执行 runbook；runbook 含 **"Never do"** 显节与「每步需批准、**删数据前先问**」的规则。我们的引导卡与领域 Agent Skill 可参考此形态 |
| **C2** | **诚实文档**：`docs/VALIDATION.md` 首段即写「这是可执行文件 MVP，结果来自本机，**不是生产 SLA**」，并把「只在 stub docker 上跑过、未经真机验证」的脚本单独列一节。我们 20 号冒烟清单与各调研档可吸收这条「未验证清单」惯例 |
| **C3** | **执行隔离清单**：`safePart` 正则防路径穿越 / 单 SELECT 包装 / 禁扩展 / `cap_drop: [ALL]` / 独立网络无凭证 / 内存·PID·时长·结果大小四限额 / **oom_score_adj 分级决定谁先被杀**。我们 SPARQL 工作台白名单（04 号）可直接对照，尤其中「包成单 SELECT 防多语句」这一招可平移为 SPARQL 单语句包装 |
| **C4** | **测试隔离**：`pnpm test` 用内存 store + 临时 loopback 端口，「无需任何运行中的服务」；browser test 用独立工作区 + `PW_SLOT` 端口偏移支持并行 |
| **C5** | **本地优先安全模型明写边界**：仅 loopback + **Host 头非本机返回 403（防 DNS rebinding）** + 浏览器非 GET 必须同源 + 备份**不含加密密钥**且要求用户另存。我们 16 号部署文档可吸收「明示不可置于反代后」的表达方式 |
| **C6** | **按 digest pin 的外部制品**：`docs/models.json` 记录 `qwen3:4b` 的 sha256 并在安装时校验；compose 里 HelixDB 亦 `image@sha256`，并明写「v0.0.10 起索引单向升级」 |
| **D1/D2** | HelixDB 的 **pre-filter → ranking** 检索形态（§6.3）；以及 Foundry「买了三模态引擎却只用图」的**自我边界**（向量能力没用上，值得警醒：别引入用不到的能力面） |

### 7.4 赛道对照：同一目标的三种解法（建议与 82 / 22 号对读）

| 维度 | **OpenBKN**（82 号） | **Helix Foundry**（本档） | **我们（multiagent-lab）** |
| --- | --- | --- | --- |
| 本体从哪来 | 人写 **BKN Lang**（Markdown + YAML DSL）→ 编译 | **数据说话**：一次性 SQL 推断 + 人审阅 | 人写 spec_json + 六条路径（含 LLM / 知识库 / ODP） |
| 知识组织 | 知识网络**包化**（tar / CHECKSUM / Diff） | Parquet 快照 + lineage + generation | spec_json 版本 + diff + 产物 |
| 一等公民 | **Risk**、本体 → SKILL.md 编译、AGENTS.md 治理 | **evidence notes**、quality checks、generation | CQ（REQ-248 / 255）、qualitygate、审计与溯源 |
| 运行时 | 自建栈 | HelixDB + DuckDB 双引擎 | Oxigraph / Fuseki / Cayley 多方案管理 |
| 交付形态 | 平台 | **一台笔记本上一个 docker compose** | 单机可部署平台（教学定位） |
| 我们对它的裁决 | 不集成（牌照 / 重叠 / fork 包袱）→ 借鉴思想 | **不引入引擎 → 借鉴算法与纪律（本档 A / B / C）** | — |

> **两家共识**：**都把「本体」放在应用层工程产物里，而不指望某个数据库替你承载语义。** OpenBKN 用文件系统 + 包，Helix Foundry 用图 + JSON doc。这与 62 号 kubernetes-ontology 的「反对 ontology-database-first 漂移」、我们 D-O1（构建与运行解耦）三方互相印证。

### 7.5 明确不做

| 项 | 理由 |
| --- | --- |
| 引入 HelixDB 作为本体运行时 / 存储 | 见 §6.2：非 RDF、切断 SPARQL 主线、部署面膨胀、违背 D-O15 精神 |
| 引入 Helix Foundry 的代码或 prompt | TypeScript / Docker Compose 栈与 Go 平台无关；其 prompt 面向「从关系库反推本体」，与我们 CQ 驱动构建（REQ-271~275）场景不同 |
| 「local-first 无账号」安全模型本身 | 我们平台有角色与连接器设计，拿掉鉴权不可行；**只借鉴其「明示边界」的表达方式（C5）** |
| 本地模型作为默认推理（qwen3:4b + Ollama） | 我们走平台模型代理 + 多后端；本地推理不作为默认路径（成本与体验） |
| 语音对话（GPT-Live orb） | 场景不符 |
| 连接器矩阵（Neon / Supabase / Stripe / WorkOS / PostHog） | 我们的连接器面向 MCP 与本体资产回流，与它的「SaaS 数据接入」不是同一目的 |

---

## 8. 诚实边界（必读）

1. **未运行**：沙箱未 clone、未执行 `docker compose up`、未实际调用 `suggestOntology`；**全部结论来自 GitHub API + raw 文件级精读**（README / AGENTS.md / 6 份 docs / compose.yaml + `discovery.ts` 1,529 行逐段读 + `store.ts` / `context.ts` / `routes/ontology.ts` / `services/executor/src/engine.ts` / `assistant.ts` 关键段落 + helix-db 的 README / cypher.md / helixql.json）。**没有一条经运行验证。**
2. **置信度分级**：`suggestOntology`（L285–841）、`overlap`（L1015+）、`withoutImplied`（L943–1009）、`throughAnchor`（L877–937）、`confirmOntology`、`ontologyBundle`、`validateBundle`（assistant.ts L108–350）、`publishProposal`（L1504–1835 头部）为**逐行精读**；`buildWorkspace`（L507–1504，约 1000 行）**只读了入口与片段**，其编排与进度细节未穷尽。
3. **星数与活跃度是快照**：1103★ / 4 天属爆发期读数，**不代表稳定性**；第三方报道（589★ / 719★）口径不同，引用请以 API 快照日为准。
4. **HelixQL 现状判断为推断**：我据「`crates/` 下已无 helixql crate + 主推 builder / Cypher」推断其**地位下降**，但 `docs/languages/helixql.json` 仍在、官网仍推 HelixQL 教学——**这是我的推断而非官方声明**，据此做技术判断请再核官网。
5. **「Foundry 没用 HelixDB 向量检索」** 的判断来自 `store.ts`（纯 builder 图 API）与 Analyst 走 NL→SQL 的路径，**未见 embed / 向量检索调用**——属「未见」而非「确认没有」，措辞保留。
6. **A / B / C 借鉴清单中的落地建议是我的设计判断**，不是 Foundry 原文建议；Foundry 原文只在自己的注释里解释「它为什么这样做」。
7. **立项状态（2026-10-11 更新）**：本档**已由「纯调研借鉴」转为「已立项」**——开发者指令「采纳建议，立项，当前agent不开发由其他agent开发」，按 18 号分配 **REQ-292~295** 并在 03 号需求档 §2.27 落档（B1 扩注 REQ-278），决策记 **D-O24**。去向索引见 §7.0。**本轮只立项不开发，开发移交其他 agent**（沿 REQ-253「此处不开发，由其他agent开发」与 REQ-255 批次「由其他agent后面统一开发」先例）。**但仍然成立的是**：REQ-292~295 的需求描述是**我的设计判断**（见第 6 条），Foundry 原文只解释「它为什么这样做」；领取开发的 agent 应回到本档原文与一手源码复核口径。
8. **仓库遗留问题已修复（2026-10-11 同日跟进）**：本档 v1.0 曾记录 `docs/15_开源项目及论文登记簿.md` 变更记录区（v2.29 与 v2.30 之间）**残留 git 冲突标记**（`<<<<<<< HEAD` / `=======` / `>>>>>>> 4b760fc`，git status 干净、无 MERGE_HEAD，属历史并行写入未正确合并所致），当时标注「本轮不处理」。**开发者随后指令「修复残留的git冲突标记」，已修复**：判定两侧内容**互补无语义冲突**（HEAD 侧 v2.29=66 号缺口批立项跟进 REQ-278~280；4b760fc 侧 v2.30=oo 双轨退役方案 B 落地），故**两行全部保留**（v2.29 在前、v2.30 在后），仅删除三行标记与残留的 1,731 字符 commit message 尾行；修复后全档三类标记计数归零，登记簿升 v2.32。

---

## 参考资料（一手）

**Helix Foundry**
- `README.md`（12.7KB）、`AGENTS.md`（62KB agent runbook，含 "Never do" 节）、`CLAUDE.md`、`compose.yaml`、`package.json`、`.env.example`、`Dockerfile`、`docs/models.json`
- `docs/`：GUIDE.md（26.8KB：功能 / 连接器 / AI 与审阅 / 架构 / 安全 / 开发 / 测试）、DESIGN.md（16.4KB：视觉标准 / 进度反馈 / 可访问性）、API.md、VALIDATION.md（12.4KB：已验证路径 / 本地 AI 工作流 / 基准）、OPERATIONS.md（20.2KB）、HOSTING.md、onboarding.md
- `apps/api/src/`：`discovery.ts`（1,529 行，**核心**）、`store.ts`、`context.ts`、`routes/ontology.ts`、`assistant.ts`（1,857 行）、`jobs.ts`、`source-sync.ts`、`live.ts`
- `packages/shared/src/`：`ontology.ts`（zod schema）、`pipeline.ts`、`workspace.ts`
- `services/executor/src/`：`engine.ts`（DuckDB 隔离）、`limits.ts`、`jobs.ts`、`server.ts`
- `tests/` 40+ 文件（含 `onboarding-ontology.test.ts` 25.6KB、`helix.test.ts`）、`scripts/`（setup / backup / restore / upgrade / doctor / migrate）

**HelixDB**
- `README.md`（10.3KB：三模态 / 四 SDK / Cypher / `helix chef`）、`docs/cypher.md`（32.2KB）、`docs/codebase-map.md`（28KB）、`docs/languages/helixql.json`（9.6KB：`N::` / `E::` / `V::` / `QUERY` / `schema::N` 语法定义）、`crates/` 12 个子 crate

**关联档**
- 本目录 **82 号**《OpenBKN 构建与运行深度分析》（同为 Palantir 开源替代，解法相反）、**22 号**《开源本体与语义层赛道全景》、**62 号**《kubernetes-ontology 与 Kartograph 借鉴分析》（K1~K3 同族）、**65 号**《open-ontologies 深度调研》（B1~B3 / B5 可对照）、**59 号**《Protégé 架构与借鉴分析》
- `docs/15_开源项目及论文登记簿.md`（本轮登记见 v2.31）
