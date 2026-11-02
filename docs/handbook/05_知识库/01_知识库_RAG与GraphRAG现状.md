---
module: 知识库
topic: 知识库 RAG 与 GraphRAG 现状（架构与设计）
desc: 知识库三类型实现现状——索引/检索双管道、可插拔 VectorStore（Qdrant+SQLite fallback）、SQLite 数据模型与自研 KG 三表、API/SSE 契约、KG 增强治理面、LLM Wiki 类型、KB→本体构建数据源、现行风险与开放问题
synced: 2026-11-02
---

# 知识库 RAG 与 GraphRAG 现状

> **定位**：知识库模块的**架构与设计现状档**（子模块实现状态档）。模块导读与能力总表见 [00_知识库模块](00_知识库模块.md)；需求事实源《[11_知识库_需求文档](../../11_知识库_需求文档.md)》；设计沿革与开源选型史见《[12_知识库_方案设计](../../12_知识库_方案设计.md)》（2026-11-01 四区收敛后为账本壳，终态叙述已迁本档）；技术栈归属与平台架构锚点在《02_智能体_技术方案设计.md》§6.9 对接契约。

## 1. 模块定位与三类型

知识库是主平台内的全局资源：文档入库 → 切分 → Embedding → chunk 池 → 检索 → 对话注入的 RAG 闭环。`kb.mode` 为**库级能力开关**（KB-11 起解除互斥，存量库视为仅启用原 mode 对应能力，检索内部按库能力路由），承载三类型并存可对照（SC-K9）：

| 类型 | kb.mode 值 | 检索路径 | 一句话定位 |
| --- | --- | --- | --- |
| 向量检索 RAG | `rag`（默认） | chunk 向量化 → VectorStore TopK | 精确片段检索基线 |
| GraphRAG 检索 | `graphrag` | 向量命中 → 自研 KG 一跳扩展 + claims | 语义图谱检索（KG 自研内置，D-O15 反转后现行口径） |
| LLM Wiki | `wiki` | 互链 Markdown 页面 2-gram 评分读页 | 写时合成层，零 embedding 依赖（REQ-241） |

三类型共享 chunk 池（一份文档入库同时供各检索路径）；`conversation.kb_id` 对话关联留平台库，知识库侧不感知会话。

## 2. 索引管道（异步状态机）

```
文档入库（粘贴文本 / txt·md 上传 / content_b64 二进制通道）
  → 切分器（块/重叠参数化）
  → Embedding（OpenAI 兼容模型连接）
  → chunk 表（正文+索引状态：待处理/处理中/就绪/失败）
```

- **摄入通道**：REQ-439 起 `content_b64` 二进制通道——PDF/DOCX 经 base64 无损传输（`file.text()` 读二进制必失真 U+FFFD），解码后按 filename 分发文本层抽取（`kb/richtext.go`：DOCX 完整 / PDF 朴素+明确报错边界），原始字节计 10MB 护栏；OCR/表格抽取维持 KB-2 需求池触发驱动。
- **异步语义**（REQ-440）：导入与重建索引**落库即返回**（201/200 时 doc status=indexing），embedding 与 KG 抽取由后台串行队列完成（ctx 脱离请求生命周期 10min 上限；单 worker 与同步语义等价，防 SQLite 并发写），前端轮询至稳定；graphrag_json 落库（GraphragInfo 持久可见——KG 徽标/失败原因刷新不丢）。诚实边界：后台队列进程内，重启丢队列任务=文档停留 indexing，重试=重建索引按钮如实呈现；整库重建三端点维持同步+显式手动+成本警告先例。
- **维度回显**：doc 表 `dim` 列（迁移 066）——wiki 库 chunksOnly 零 embedding，dim=0 如实零维。

## 3. 检索管道与对话注入

统一检索出口 `route.go SearchUnified`：按库能力开关路由（向量〔含混合检索〕/ 图谱 / wiki 读页）。对话提问（选了知识库）→ retrieve(query, topK, threshold) → 片段+得分 → 拼接上下文 → SSE `retrieval` 事件 → Agent 生成（REQ-17）。

- 事件字段：`kb_id / doc_name / chunk_preview / score / mode / degraded`；GraphRAG 命中含 `entities / relations / claims`（附 chunk_id 出处摘录+句级偏移 span，前端高亮）；wiki 命中 `source=wiki` 且 hits 附 `sources`（chunk_id 解析为 doc 标题+seq 的可读定位）。
- 上下文注入格式：`[片段 doc:seq score]`（RAG）/ `[KG 实体/关系 doc:seq score]`（GraphRAG，含 entity_type/relation_type 标注）。
- 降级语义：GraphRAG 无 KG 命中回退纯向量标 `degraded`；wiki 无命中 `degraded=true` 引导重建；Qdrant 不可达（构造时启动探测）自动落 SQLite fallback，人话告警 + healthz `knowledge_backend=sqlite-ok` 如实可查（REQ-593；降级不阻断建库）。

## 4. 可插拔 VectorStore 抽象

```go
type VectorStore interface { // 两种实现
    Upsert(ctx, kbID string, chunks []Chunk) error
    Search(ctx, kbID string, vec []float32, topK int, minScore float64) ([]Hit, error)
    DeleteByDoc(ctx, kbID, docID string) error
}
// QdrantStore（首选，kb/qdrant.go）：自建 Qdrant REST client（KB-O1 定稿；
//   collection=kb_{id}，cosine，point id=chunk id）
// SQLiteStore（fallback，kb/vector.go）：余弦 TopK，无外部依赖对照（不删）
```

- 选择逻辑：配置 `KB_VECTOR_BACKEND=qdrant|sqlite`（默认 qdrant，`QDRANT_URL` 缺省 http://127.0.0.1:6333）；qdrant 分支构造时启动探测一次——`QDRANT_URL` 未配置或 ping 不通 → 人话告警 + 自动落 SQLite fallback（KB-O1「缺失/不可达可退」口径，REQ-593 补实现——此前仅文档记载、探测未接线，无 Qdrant 机器上索引/删除/检索全链 dial 硬错）；显式 `KB_VECTOR_BACKEND=sqlite` 零探测直取；未知后端值报错不静默。运行期 Qdrant 中途掉线不做热切换（healthz `knowledge_backend=qdrant-unreachable` 如实，重启即回退）；fallback 期间新索引 chunk 向量 BLOB 直存 SQLite，切回 qdrant 后该期间文档需重建索引恢复向量臂命中（诚实边界，启动日志已提示）。
- 设计基调（D-KB1）：**平台内组件级**，开源优先、eino 生态同栈；不引入平台级成品知识库（WeKnora/RAGFlow/Dify 对照评估结论=维持组件级方案，成品列 P2 独立集成候选，调研档在 docs/research/05_知识库/）。

## 5. 数据模型（SQLite）

| 表 | 关键列 | 说明 |
| --- | --- | --- |
| `kb` | id, name, brief, top_k, threshold, mode, created_at | 库级检索参数+三类型能力开关（迁移 008 `kb.mode`） |
| `doc` | id, kb_id, name, status, chunk_count, dim, wiki_hash, updated_at | 文档单位；status 索引状态机；dim 维度回显（迁移 066）；wiki_hash 摄取指纹（迁移 040） |
| `chunk` | id, doc_id, kb_id, content, status, store_backend, vector_ref | 正文恒存 SQLite；Qdrant 路径 vector_ref=point id，fallback 路径 BLOB 直存；wiki 库 chunksOnly StoreBackend=none |
| `kg_entity` | + alias（分号分隔）、description | 自研 KG 实体（迁移 057 系）；别名消歧合并后保留归并 |
| `kg_relationship` / `kg_claim` | + review_status（approved 默认 / rejected） | claim.chunk_id 溯源；rejected 不参与检索与展示 |
| `kg_community` | community_id, kb_id, level, member_entity_ids(JSON), summary, summary_method | 社区摘要 LLM 生成缓存，重建失效 |
| `wiki_page` | id, kb_id, page_type, title, content_md, sources_json | 六类型页面（summary/entity/concept/topic/synthesis/index）；sources_json=溯源 chunk_id 数组；重建全量替换 |

KG 抽取配置（模型/提示词覆写/本体挂载/语料预算）为**库级**设置，存 kb 元信息，不改全局默认连接语义。

## 6. API 与事件契约

| 契约 | 端点/事件 |
| --- | --- |
| KB CRUD | `/api/kbs`（含 mode 与能力开关） |
| 文档 | `POST /api/kbs/{id}/docs`（粘贴/txt·md/content_b64；异步返回 status=indexing）、`DELETE`（级联） |
| 试运行 | `POST /api/kb/{id}/search-preview`（直接返回命中，wiki 库自动路由） |
| GraphRAG 检索 | `POST /api/kbs/{id}/graphrag-search`（`{query, top_k?, min_score?}` → hits 含 entities/relations/claims/score） |
| KG 治理面 | `/api/kg/{id}/entities?search=`、`/neighbors/{entityId}?hops=&relation_types=`、`/stats`、`/merge`、`/review`、`/quality`、`/merge-suggestions`、`/entity-alias`、`/communities/rebuild`、`/global-search?q=` |
| LLM Wiki | `GET /api/kb/{id}/wiki/pages`（清单含全文）、`POST /api/kb/{id}/wiki/rebuild`（手动全量重建，返回 pages/llm_calls/skipped_docs/duration 成本可见） |
| KB→本体 | `GET /api/kbs/selectable-for-ontology-build`（§8） |
| SSE | `retrieval` 事件（§3 字段），回答前发出（REQ-17） |

## 7. 自研 KG 与增强治理面

- **抽取**：REQ-98——`method=llm` LLM 结构化抽取主路径（规则回退 `lightweight`），graphrag 能力开启的库导入即自动建 KG（失败不阻断导入），落 SQLite 三表，**KG 直接在本平台重存**（历史 semantica worker 转发口径已随 D-O15 退役；worker 归档休眠勿重启）。
- **检索**：三步——向量命中 → KGSubjectsByChunks → KGNeighbors 一跳+claims；多跳遍历 SQLite 递归 CTE（学习尺度 2 跳内）；graphrag-search 增参 entity/hops/relation_types。
- **社区与全局问答**：Louvain 社区检测（gonum 为主路径）；社区摘要**按需生成+缓存**（首次 global-search 触发，不重建时全量预生成）；社区检索含 embedding 臂演进项。
- **消歧治理**：merge 仅人工触发（不做自动合并）；merge-suggestions 规则+向量双臂（名称包含+名/描述 embedding 相似，类型不同附「慎并」警示）；entity-alias 别名人工标注。
- **本体词表注入**（KB-6③）：抽取 prompt 可挂本体词表白名单——**运行期单向 本体→KB**，词表经构建平面 spec 只读投影（label 优先名兜底，120/60 截断注入）。
- **模块边界**（知识库×本体交集治理五原则，见 11 §3.1）：单一资产归属/展示位≠管理入口/运行期单向/可解释标记/命名单一真源；本模块 KG 为唯一治理入口，本体「消费与审计」栏为只读观测台。

## 8. LLM Wiki 类型（REQ-241）

- **定位**：第三类型写时合成层，不替代 RAG/GraphRAG——文档入库在 chunk 池之外由 LLM 摄取生成互链 Markdown 页面层，查询读页非检索片段；模式沿 Karpathy LLM Wiki。
- **摄取管线**（`kb/wiki.go`，LLM 经 WikiLLMFunc 注入防 import 成环，`WIKI_LLM_CONN_ID` 可选指定连接）：①summary 页每文档 1 次 LLM（全文 6000 rune 截断→摘要+实体/概念清单）；②聚合页批量 1 次（entity/concept/topic/synthesis 四类，JSON pages 数组，未知类型与空页丢弃、重名去重，docs 引用确定性后处理转 chunk_id 并集溯源）；③index 页确定性生成零 LLM；④互链 `[[页面标题]]`，页面标题即导航锚。
- **成本模型**：N 文档 → N+1 次调用；`wiki_hash`（chunk 拼接 sha256）未变更文档跳过=变更数+1，零变更 0 调用不替换；前端成本预估警告如实呈现。
- **检索臂**：route 前置分支——查询词 2-gram（中文 bigram）对 page.title（×3）+content 覆盖评分、整题命中加成 → TopK 页面为 hits；index 页不参与命中；无命中 degraded 引导重建。
- **UI**：知识库左侧边栏第三类型页——wiki 库两页签（Wiki 页面〔按类型分组+Drawer Markdown 预览+溯源数+重建成本警告+生成连接选择〕/ 文档与检索）；检索参数仅 TopK/min_score（能力固定读页）。
- **边界（诚实标注）**：v1 手动全量重建（沿 REQ-130 先例），增量后台维护与 lint 体检为后续触发项；页面质量依赖抽取模型；学习尺度 ~百文档内；文档删除即时移除其 summary 页、聚合页留待重建收敛。

## 9. KB 作为本体构建数据源

本体构建栏「由知识库构建本体」（D-O14）的数据源全部来自 KB，设计实现分两端：

- **KB 侧选择器**：`GET /api/kbs/selectable-for-ontology-build`——RAG 库含 chunk_count，GraphRAG 库含 chunk_count+entity_count+relation_count；前端按 mode 徽标分组。
- **本体侧编排**（`backend/internal/ontobuild`，独立包避免 kb→chat 回环）：策略 A `POST /api/ontologies/build-from-kb`（chunk-llm：chunk 池→LLM 抽 spec_json，复用 `/api/ontology-llm/generate`+CQ 引导）；策略 B/C `POST /api/ontologies/kg-to-spec-json`（kg-direct KG 直转薄映射层 entity→Concept / relation→Relation / claim→Attribute；kg-llm-hybrid KG 初稿+LLM 校验补全）；KG 源为自研内置 backend（策略 B/C 均读自存表）。
- **门禁**：三生成端点草稿统一经构建平面 Spec.Validate+quality/check 快评（校验管线只有一条，degraded 降级标注）。
- **学习价值闭环**：「文本→三种语义资产形态」完整矩阵——RAG 片段（检索用）/ KG（GraphRAG 检索端点数据）/ 本体（本路径抽取），与本体侧构建/运行/消费/审计四环节形成映射。

## 10. 学习价值设计

| 环节 | 开源组件/机制 | 学习点 |
| --- | --- | --- |
| 切分 | 自研薄层（块/重叠参数化） | chunking 对召回质量的影响（search-preview 试运行可对照） |
| Embedding | 模型连接（OpenAI 兼容） | embedding 模型差异（BGE/OpenAI 系列）对召回的影响 |
| 向量检索 | Qdrant（cosine + HNSW）/ SQLite 余弦全扫 | 向量库索引 vs 暴力扫描的性能差异；两 Store 实现同一接口 |
| 上下文注入 | Eino Retriever 抽象 + Prompt 拼接 | RAG 在编排框架中的位置 |
| fallback 思想 | SQLiteStore | 降级设计与无依赖部署权衡 |

## 11. 代码落点

- `backend/internal/kb/`：service.go（导入/CRUD）、async.go（后台索引队列）、route.go（SearchUnified 统一检索路由）、vector.go/qdrant.go（双 Store）、graphrag.go（KG 检索三步）、hybrid.go/ppr.go（混合检索）、automerge.go/disambig.go/entityvec.go（消歧与向量建议）、dimcheck.go（维度漂移）、richtext.go（富格式文本层）、wiki.go（LLM Wiki 摄取与检索臂）、evaldata（KB eval）。
- `backend/internal/kg/`：kg.go/pipeline.go（抽取管线）、community.go/community_vec.go（社区与 embedding 臂）、globalqa.go（全局问答）。
- `backend/internal/ontobuild/`：KB→本体三策略编排。
- 迁移：008_kb_mode / 040_kb_wiki（wiki_page+wiki_hash）/ 057_knowledge_graph（KG 三表）/ 066_kb_doc_dim。

## 12. 风险登记（现行）

| 风险 | 触发 | 缓解/降级 |
| --- | --- | --- |
| embedding 连接误配（如 DeepSeek 无 embedding） | 建库向量化失败 | 引导提示+索引状态失败可重试；不提供仅文本不向量路径 |
| Qdrant 容器不可达 | 无 Qdrant 机器上索引/删除/检索全链 dial 硬错（REQ-593 缺陷场景） | 构造时启动探测不可达 → 自动落 SQLiteStore（人话告警+healthz 如实）；运行期中途掉线不热切换，重启即回退 |
| 库级换 embedding 模型维度不一致 | 用户换模型 | doc.dim 记录+变更提示重建；维度漂移检测告警立项在途（KB-20） |
| KG 抽取与 embedding 模型一致性 | 换模型致 KG 与 chunk 错位 | doc 维度记录+切换时提示重建 KG |
| 索引后台队列进程内 | 后端重启 | 队列任务丢失=文档停留 indexing，重试入口如实呈现（REQ-440 诚实边界） |

退役行：semantica worker 钉版本/worker 不可达两风险随 D-O15（worker 归档休眠）失义，不再登记。

## 13. 开放问题（现行）

| 优先级 | 问题 | 归属 |
| --- | --- | --- |
| KB-O2（P2） | WeKnora/RAGFlow 独立服务集成（11 KB-3）是否启动——评估结论=与平台内组件级知识库并行不替代 | 开发者 |
| KB-O3（P2） | embedding 模型变更时库级重建策略（锁定维度 or 提示重建）；KB-20 维度漂移检测告警立项承接 | 开发者 |
| KB-O6（P2，部分关闭） | 同库双召回融合已成立（KB-11 库级能力开关）；跨库联合检索留 B4 触发式（37 号 §2） | 开发者 |

已关闭：KB-O1（Qdrant 对接方式已定稿=自建 REST client）/KB-O4、KB-O5（semantica 侧接口扩展与钉版本校验随 D-O15 失义关闭）。
