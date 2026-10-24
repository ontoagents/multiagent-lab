// ---------------------------------------------------------------------------
// 官网内容单源（REQ-238）：演示数据 = ontology-service/internal/seed/examples/
// 种子 spec（后端启动播种同一份）。
// REQ-364 模块页去文档化：模块介绍为站点层结构化数据（tagline/brief/flow/
// features），全部中英双语（REQ-360：zh 为事实源）。
// 分工配合轮（2026-10-24）：每模块新增 role（在平台中的分工定位——管什么/
// 不管什么，同时用于页顶「平台分工总览」条）与 coop（与其他模块的配合点，
// 点名对方模块）——七模块各有侧重、互为指路，不再各说各话；跨模块能力唯一
// 归属：MCP/A2A 协议能力=智能体段、连接器管理面=设置段、外部成员协作=项目段，
// 技能段回归纯技能（扩展机制）。
// 功能卡事实口径 = platform-knowledge/ 各模块 00 导读页「已交付能力总表 /
// 已交付能力表」与 docs/02/03/04/11/12（2026-10-24 起以能力总表为准），
// 仅做直观化摘编；已排期未开发条目以「（规划中）」标注状态（en: planned）。
// ---------------------------------------------------------------------------

import type { Locale, L10n } from './locale'

// 演示种子（与后端启动播种同源；?raw + JSON.parse，避免 TS 深层 JSON 类型推断）
import orgsRaw from '../../ontology-service/internal/seed/examples/orgs.json?raw'
import defectsRaw from '../../ontology-service/internal/seed/examples/defects.json?raw'
import failureRaw from '../../ontology-service/internal/seed/examples/failure.json?raw'
import geneCoreRaw from '../../ontology-service/internal/seed/examples/gene_core.json?raw'
import medCommonRaw from '../../ontology-service/internal/seed/examples/med_common.json?raw'
import type { Spec } from '../../web/src/api/types'

/** 仓库地址：构建期注入（CI=触发仓库 github.repository，随迁移/fork 自动跟随；缺省=组织仓库），见 vite.config.ts define */
export const REPO_SLUG: string = __REPO_SLUG__
export const REPO_URL = `https://github.com/${REPO_SLUG}`

export interface ModuleText {
  title: string
  tagline: string
  /** 分工定位（1~2 句：本模块在平台中管什么、不管什么；页顶总览条复用） */
  role: string
  /** 速览要点（首屏直观化；3 条以内短句） */
  brief: string[]
  /** 典型流程（REQ-374：3~5 步动线，短句祈使式；内容对齐平台真实动线） */
  flow: string[]
  /** 与其他模块的配合（2~3 条；点名对方模块+一句怎么接） */
  coop: string[]
}

/** 图标功能卡（REQ-364）：icon 为 @ant-design/icons 组件名，映射表见 ModulePage */
export interface ModuleFeature {
  icon: string
  i18n: L10n<{ title: string; desc: string }>
}

/** 延伸入口（REQ-374）：站内跨页深链（href 站内共用，label 双语） */
export interface ModuleLink {
  href: string
  i18n: L10n<string>
}

export interface ModuleInfo {
  key: string
  i18n: L10n<ModuleText>
  features: ModuleFeature[]
  links?: ModuleLink[]
}

/** 功能模块（目录序=导航序；整体设计放最后，作为深入了解入口） */
export const MODULES: ModuleInfo[] = [
  {
    key: 'agents',
    i18n: {
      zh: {
        title: '智能体',
        tagline: '执行核心：定义智能体、直接对话、消费全平台能力',
        role: '平台的执行核心：单个智能体在此定义并直接对话——模型、提示词、工具在装配期组合成 ReAct 循环；知识库、本体、技能、连接器的能力最终都汇入智能体的工具循环被模型调用。',
        brief: ['定义智能体：提示词/模型/工具/知识库/本体/技能/沙箱一处装配', '流式对话全程留痕：工具/委派/检索/审批事件可观测可回放', '对外双向：智能体可暴露为 MCP/A2A 服务，也可委派外部智能体'],
        flow: ['在设置页配好模型连接', '创建智能体，勾选工具/知识库/技能', '对话试用，轨迹面板看执行细节', '窗格对比不同模型与提示词', '在项目中编排成团队协作'],
        coop: [
          '知识库 / 本体：会话挂载即注入检索与 onto_* 工具，命中来源以事件透明呈现',
          '技能：勾选挂载即扩展打法；项目：智能体作为成员被编排与委派',
          '设置：对话与向量化所用连接均引用设置页配置，密钥不落智能体',
        ],
      },
      en: {
        title: 'Agents',
        tagline: 'The execution core: define agents, chat directly, consume every platform capability',
        role: 'The execution core: agents are defined and chatted with here — model, prompt and tools assemble into a ReAct loop; capabilities from the knowledge base, ontology, skills and connectors all flow into the agent tool loop.',
        brief: ['Define agents: prompt / model / tools / KB / ontology / skills / sandbox assembled in one place', 'Streaming chat, fully traced: tool / delegation / retrieval / approval events, observable and replayable', 'Open both ways: agents serve MCP/A2A outward and delegate to external agents'],
        flow: ['Configure a model connection in Settings', 'Create an agent, pick tools / knowledge / skills', 'Chat and inspect execution in the trace panel', 'Compare models and prompts side-by-side', 'Orchestrate the agent into a team in Projects'],
        coop: [
          'Knowledge Base / Ontology: mounting them injects retrieval and onto_* tools into the conversation, sources visible as events',
          'Skills: checkbox mounting extends know-how; Projects: agents are orchestrated and delegated as members',
          'Settings: chat and embedding connections all reference Settings — keys never live on the agent',
        ],
      },
    },
    links: [
      { href: '#/implementation', i18n: { zh: '五层执行视角', en: 'Five-layer execution' } },
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'CommentOutlined',
        i18n: {
          zh: { title: '对话工作台', desc: '流式对话 + 事件轨迹面板：工具调用、深度思考、子智能体委派、知识召回、审批请求全程留痕，可展开、可回放；会话跨库搜索与消息级操作（复制 / 重新生成 / 编辑重发）' },
          en: { title: 'Chat workbench', desc: 'Streaming chat with a trace panel: tool calls, deep thinking, sub-agent delegation, retrieval and approvals fully traced — expandable and replayable; cross-session search and per-message actions (copy / regenerate / edit-resend)' },
        },
      },
      {
        icon: 'TeamOutlined',
        i18n: {
          zh: { title: '多智能体协作', desc: 'agent_as_tool 按成员的能力描述路由委派，子智能体上下文隔离、嵌套轨迹可见；成员失败或超时自动降级回喂协调者改道自愈，委派过程有结构化委派单' },
          en: { title: 'Multi-agent collaboration', desc: 'agent_as_tool routes delegation by member capability descriptions; sub-agent contexts stay isolated with visible nested traces; failed or timed-out members degrade gracefully back to the coordinator with structured delegation slips' },
        },
      },
      {
        icon: 'LayoutOutlined',
        i18n: {
          zh: { title: '对话对比', desc: '同一问题 2~4 窗格并排对照：每窗格可独立切换智能体、模型、温度、提示词与技能开关，覆盖/继承状态一目了然；剖面对话级保存、可采纳转正' },
          en: { title: 'Compare panes', desc: 'Ask once, compare 2–4 panes side by side: each pane overrides agent, model, temperature, prompt and skills independently with clear inherit/override badges; named profiles save per conversation and can be promoted' },
        },
      },
      {
        icon: 'ControlOutlined',
        i18n: {
          zh: { title: '上下文工程', desc: '三档上下文预算（compact / standard / full），超限先前缀摘要再压缩持久化；工具结果自动剪枝，压缩策略可选，每轮喂给模型什么完全可控' },
          en: { title: 'Context engineering', desc: 'Three context budget tiers (compact / standard / full); over budget, prefix-summary then persisted compression; tool results auto-pruned and compression strategy selectable — full control over what the model sees' },
        },
      },
      {
        icon: 'SafetyOutlined',
        i18n: {
          zh: { title: '沙箱与审批', desc: 'docker / k8s 执行后端隔离副作用；危险操作三档审批 + 豁免清单，MCP 工具按 annotations 收窄审批面，HookChain 拦截 SSRF 与危险命令' },
          en: { title: 'Sandbox & approvals', desc: 'docker / k8s execution backends isolate side effects; three-tier approvals + exemption lists, MCP tool approvals narrowed by annotations; HookChain blocks SSRF and risky commands' },
        },
      },
      {
        icon: 'SyncOutlined',
        i18n: {
          zh: { title: '断点续跑与定时任务', desc: 'checkpoint 快照落库，进程重启后挂起运行可恢复；定时续跑支持周期间隔与次数上限、任务列表面统一管理；挂起主动触达提醒（规划中）' },
          en: { title: 'Resume & scheduling', desc: 'Checkpoint snapshots persist to the database; suspended runs survive restarts; scheduled resume supports intervals, run caps and a task list view; proactive alerts on pending approvals (planned)' },
        },
      },
      {
        icon: 'RocketOutlined',
        i18n: {
          zh: { title: '双向协议开放', desc: '对外：智能体一键暴露为 MCP Server（/mcp）与 A2A 端点（agent card 发现 + 任务落库）；对内：A2A 委派客户端把外部智能体接入对话委派（轮询回传 + 失败降级）——平台级协议能力的唯一归属地' },
          en: { title: 'Open protocols, both ways', desc: 'Outward: expose any agent as an MCP Server (/mcp) or an A2A endpoint (agent-card discovery + persisted tasks); inward: the A2A delegation client brings external agents into your chats (polling + graceful degradation) — the single home of platform protocol capabilities' },
        },
      },
      {
        icon: 'RobotOutlined',
        i18n: {
          zh: { title: '平台助手', desc: '内置系统级智能体：平台知识答疑、配置辅助，复杂输入框旁「AI 优化」一键改写（diff 预览确认回填）；配置与普通智能体归一单源' },
          en: { title: 'Platform assistant', desc: 'A built-in system agent answering platform questions and helping with config; an "AI optimize" button rewrites complex inputs with diff-preview confirmation; configuration unified with regular agents' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '评估驱动质量', desc: 'agenteval 种子任务集 + LLM Judge 门控：智能体行为改动可回归验证；per-tool 成本归因让每次工具调用的 token 开销可查' },
          en: { title: 'Eval-driven quality', desc: 'agenteval seed task sets + LLM judge gating make behavior changes regression-verifiable; per-tool cost attribution shows the token cost of every tool call' },
        },
      },
    ],
  },
  {
    key: 'ontology',
    i18n: {
      zh: {
        title: '本体',
        tagline: '结构化知识核：构建 → 资产 → 运行 → 消费审计',
        role: '平台的结构化知识核：把领域知识建模为可版本化、可运行推理的本体资产——知识库回答「文档里说了什么」，本体回答「世界如何构成」，两者互补不互替。',
        brief: ['六条构建路径殊途同归，LLM 生成必过质量门禁', 'spec_json 单一资产：版本 diff、质量档案、进化候选受控生长', 'Oxigraph / Fuseki 运行平面，MCP facade 21 工具供智能体消费'],
        flow: ['选构建路径（对话/向导/导入/知识库）', '过质量门禁生成版本化资产', '图形/层级浏览治理与演化', '编排运行方案（Oxigraph/Fuseki）', '智能体经 MCP facade 消费与审计'],
        coop: [
          '智能体：运行方案注入 onto_* 工具；「对话建本体」能力与伴生本体让知识边聊边沉淀',
          '知识库：KB 语料可反哺本体构建（路径三）；GraphRAG 抽取又可挂本体约束——双向互哺',
          '设置：AI 创建与 embedding 所用连接统一由设置页供给',
        ],
      },
      en: {
        title: 'Ontology',
        tagline: 'The structured-knowledge core: build → assets → run → consume & audit',
        role: 'The structured-knowledge core: domain knowledge modeled as versioned, runnable, reasoner-backed ontology assets — KBs answer "what do the docs say", ontologies answer "how is the world structured"; complementary, not interchangeable.',
        brief: ['Six build paths converge on one asset store; every LLM generation passes the quality gate', 'spec_json as the single asset: version diff, quality history, controlled growth of evolution candidates', 'Oxigraph / Fuseki runtime plane with a 21-tool MCP facade for agent consumption'],
        flow: ['Pick a build path (chat / wizard / import / KB)', 'Pass the quality gate into a versioned asset', 'Govern and evolve via graph & hierarchy views', 'Stage a runtime plan (Oxigraph / Fuseki)', 'Agents consume via the MCP facade, fully audited'],
        coop: [
          'Agents: running plans inject onto_* tools; the "build via chat" capability and companion ontologies accumulate knowledge as you talk',
          'Knowledge Base: corpus can feed ontology building (path three); GraphRAG extraction can be constrained by an ontology — a two-way exchange',
          'Settings: LLM and embedding connections for AI creation come from Settings',
        ],
      },
    },
    links: [
      { href: '#/demo', i18n: { zh: '在线演示（层级浏览）', en: 'Online demo (hierarchy)' } },
      { href: '#/implementation', i18n: { zh: '本体机制', en: 'Ontology mechanics' } },
    ],
    features: [
      {
        icon: 'ApartmentOutlined',
        i18n: {
          zh: { title: '六条构建路径', desc: '手工向导 / AI 对话 / 由知识库构建 / 导入外部本体 / OntoExtend 扩展 / 权威本体库一键引入（PROV-O、SKOS 等 10 条真机验证）——入口不同，出口同一套校验与资产能力' },
          en: { title: 'Six build paths', desc: 'Manual wizard / AI chat / from knowledge base / external import / OntoExtend evolution / one-click authoritative imports (PROV-O, SKOS and 8 more, machine-verified) — different entries, identical validation and asset capabilities' },
        },
      },
      {
        icon: 'CommentOutlined',
        i18n: {
          zh: { title: '对话式构建', desc: 'OntoChat 深化：澄清轮缺口清单、CQ 候选生成确认、质量报告卡新旧对照、草稿快照链回看；也可在智能体侧板开「对话建本体」能力，边聊边建' },
          en: { title: 'Conversational building', desc: 'Deepened OntoChat: clarification checklists, CQ candidate confirmation, before/after quality report cards and a draft snapshot chain; or flip on the "build via chat" capability in any agent side panel' },
        },
      },
      {
        icon: 'SafetyCertificateOutlined',
        i18n: {
          zh: { title: '质量门禁与档案', desc: '11 检查项 + 三维评分，LLM 生成必过门禁并支持自动修复（≤3 轮）；资产级质量档案随版本追加——草稿→入库→当前三点对照与趋势线' },
          en: { title: 'Quality gate & history', desc: '11 checks + three-dimension scoring; every LLM generation must pass, with auto-repair (≤3 rounds); a per-asset quality history tracks draft → ingested → current scores with a trend line' },
        },
      },
      {
        icon: 'DatabaseOutlined',
        i18n: {
          zh: { title: '资产治理', desc: 'spec_json 单一资产：版本快照与 diff、fork 溯源、进化候选受控生长；人工编辑三层保护（base+overlay+audit），TTL / VOWL / CSV 都是导出视图' },
          en: { title: 'Asset governance', desc: 'spec_json as the single asset: version snapshots & diff, fork provenance, controlled growth of evolution candidates; three-layer edit protection (base+overlay+audit); TTL / VOWL / CSV are export views' },
        },
      },
      {
        icon: 'CloudServerOutlined',
        i18n: {
          zh: { title: '多引擎运行', desc: 'Oxigraph（SPARQL 1.1）与 Fuseki（OWL 推理）按方案编排启停、一键安装；逐本体图隔离装载，启动对账防陈旧；伴生图随启停自动快照回放' },
          en: { title: 'Multi-engine runtime', desc: 'Oxigraph (SPARQL 1.1) and Fuseki (OWL reasoning) start/stop per plan with one-click install; per-ontology graph isolation and startup reconciliation; companion graphs snapshot and replay across restarts' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '可视化', desc: '3D 力导向图与 VOWL 视图浏览本体结构，五栏工作台覆盖构建 → 资产 → 运行 → 消费审计 → 学习中心' },
          en: { title: 'Visualization', desc: 'Browse ontology structure as 3D force graphs and VOWL views; a five-tab workbench covers build → assets → run → consume & audit → learning' },
        },
      },
      {
        icon: 'AuditOutlined',
        i18n: {
          zh: { title: '消费与审计', desc: 'MCP facade 21 工具：只读查询 7 件（含 check_answerable 可回答性预检——提问前先验概念覆盖）+ 13 件本体操作 + 时序查询；决策审计表、PROV-O 溯源导出' },
          en: { title: 'Consume & audit', desc: 'A 21-tool MCP facade: 7 read-only queries (incl. check_answerable — verify concept coverage before asking) + 13 ontology operations + history queries; decision audit table and PROV-O provenance export' },
        },
      },
      {
        icon: 'AimOutlined',
        i18n: {
          zh: { title: '业务本体路线（规划中）', desc: '已排期待开发：概念同义词 TBox 一等公民（SKOS 别名全量捕获）、业务域起步模板、DDL 反推 TBox（粘贴建表语句生成骨架）、ABox 质量面板、概念词典导出' },
          en: { title: 'Business-ontology roadmap (planned)', desc: 'Scheduled, not yet built: first-class concept synonyms (full SKOS altLabel capture), business-domain starter templates, DDL-to-TBox reverse engineering (paste CREATE TABLE), ABox quality panel and concept-dictionary export' },
        },
      },
    ],
  },
  {
    key: 'knowledge',
    i18n: {
      zh: {
        title: '知识库',
        tagline: '文档知识核：RAG / GraphRAG / LLM Wiki 三形态',
        role: '平台的文档知识核：非结构化语料入库、检索、注入对话——管文档事实与来源溯源，不管概念体系（那是本体的领地）。',
        brief: ['混合检索：BM25 + 向量 RRF 融合，能力开关按库路由', 'GraphRAG：多跳 1~3、社区摘要全局问答、句级溯源', 'LLM Wiki：写时合成互链页面，零 embedding 依赖'],
        flow: ['上传文档 / 网页 / 结构化数据（含 DOCX/PDF）', '切分向量化入库（Qdrant/SQLite）', 'GraphRAG 导入即建知识图谱', '检索调优（混合/多跳/社区摘要）', '挂载智能体增强回答'],
        coop: [
          '智能体：会话挂载 KB 即获检索增强，retrieval 事件让命中与得分全程可溯',
          '本体：GraphRAG 抽取可按本体概念/关系约束对齐；语料亦可作为本体构建来源',
        ],
      },
      en: {
        title: 'Knowledge Base',
        tagline: 'The document-knowledge core: RAG / GraphRAG / LLM Wiki',
        role: 'The document-knowledge core: unstructured corpora ingested, retrieved and injected into chat — it owns document facts and provenance, not conceptual schemas (that is ontology territory).',
        brief: ['Hybrid retrieval: BM25 + vector fused with RRF, capability switches routed per base', 'GraphRAG: 1–3 hop traversal, community-summary global Q&A, sentence-level citation', 'LLM Wiki: write-time synthesis of interlinked pages, zero embedding dependency'],
        flow: ['Upload docs / pages / structured data (DOCX & PDF too)', 'Chunk and embed into the store (Qdrant/SQLite)', 'GraphRAG builds a KG on import', 'Tune retrieval (hybrid / multi-hop / community summaries)', 'Mount on agents to ground answers'],
        coop: [
          'Agents: mounting a KB grounds answers in retrieval, with retrieval events making hits and scores traceable',
          'Ontology: GraphRAG extraction can align to ontology concepts/relations; corpus can also feed ontology building',
        ],
      },
    },
    links: [
      { href: '#/implementation', i18n: { zh: '知识增强机制', en: 'Knowledge augmentation' } },
    ],
    features: [
      {
        icon: 'SearchOutlined',
        i18n: {
          zh: { title: '混合检索', desc: 'FTS5（CJK 友好分词）BM25 与向量检索 RRF 融合，命中带策略标注、短查询自动降级纯向量；kb.mode 解除互斥改库级能力开关，检索按库能力路由' },
          en: { title: 'Hybrid retrieval', desc: 'FTS5 (CJK-friendly) BM25 fused with vector search via RRF; hits carry strategy tags and short queries fall back to pure vector; kb.mode became a per-base capability switch, routed automatically' },
        },
      },
      {
        icon: 'NodeIndexOutlined',
        i18n: {
          zh: { title: 'GraphRAG', desc: '导入即由 LLM 抽取知识图谱（失败回退规则抽取，不阻断导入）；检索向量命中 → 图扩展，多跳 1~3 可配、实体聚焦遍历；Louvain 层次社区摘要回答全局问题' },
          en: { title: 'GraphRAG', desc: 'LLM extracts a KG on import (rule fallback, never blocks); retrieval expands vector hits over the graph with configurable 1–3 hops and entity-focused traversal; Louvain community summaries answer global questions' },
        },
      },
      {
        icon: 'FileTextOutlined',
        i18n: {
          zh: { title: 'LLM Wiki', desc: '写时合成为互链 Markdown 页面层，检索"读页不读片段"，零 embedding 依赖；与 RAG / GraphRAG 按语料形态并存互补' },
          en: { title: 'LLM Wiki', desc: 'Synthesizes interlinked Markdown pages at write time; retrieval "reads pages, not fragments" with zero embedding dependency — coexists with RAG / GraphRAG by corpus shape' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '检索可观测与句级溯源', desc: 'retrieval 事件全程打点：命中片段、三元组、来源文档一目了然；回答论断句级高亮回链原文，抽取治理支持消歧双臂（自动建议 + 人工确认）' },
          en: { title: 'Observable retrieval & citations', desc: 'retrieval events mark sources end to end: chunks, triples and documents at a glance; answer claims highlight at sentence level back to the source, with dual-arm entity disambiguation (auto-suggest + human confirm)' },
        },
      },
      {
        icon: 'ProfileOutlined',
        i18n: {
          zh: { title: '富格式摄入', desc: 'DOCX / PDF 文本层解析入库（纯 Go 零新依赖，PDF 为朴素文本层+明确报错边界）；OCR 与表格抽取（规划中）' },
          en: { title: 'Rich-format ingestion', desc: 'DOCX / PDF text-layer parsing on import (pure Go, zero new deps; PDF is a plain text layer with explicit error boundaries); OCR and table extraction (planned)' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '评测基准', desc: '种子语料 + 多跳/全局/关键词三类基准问题集 + Recall@k 跑分器：检索类改动先跑分再合入；增量重建与可插拔 KG 存储（规划中）' },
          en: { title: 'Retrieval benchmarks', desc: 'Seed corpus + three benchmark question sets (multi-hop / global / keyword) + a Recall@k scorer: retrieval changes are scored before merging; incremental rebuild and pluggable KG storage (planned)' },
        },
      },
    ],
  },
  {
    key: 'projects',
    i18n: {
      zh: {
        title: '项目',
        tagline: '编排层：多智能体协作工作空间（工作流 / 模板 / 文件 / Git）',
        role: '平台的编排层：把多个智能体（含外部 A2A 成员）组成团队，配上工作流、本地目录与 Git——单智能体的能力在此组合成协作生产力；智能体怎么执行归智能体模块管，团队怎么组队归这里管。',
        brief: ['成员编排与工作流模式（顺序/并行/循环）', '协作研发模板：规划者/编码者/审查者一键成团', '本地目录绑定，文件与 Git 资产纳管可审计'],
        flow: ['创建项目（或一键套用协作研发模板）', '编排成员智能体与协作配置', '会话内委派任务，过程轨迹可查', '产物文件/Git 纳管沉淀'],
        coop: [
          '智能体：成员与协调者均来自智能体库，委派沿能力描述路由',
          '设置：A2A 连接器注册的外部智能体可入协作面成为成员',
          '知识库 / 本体：项目会话同样可挂载，团队共享知识上下文',
        ],
      },
      en: {
        title: 'Projects',
        tagline: 'The orchestration layer: multi-agent workspaces (workflows / templates / files / Git)',
        role: 'The orchestration layer: teams of agents (external A2A members included) with workflows, local directories and Git — individual capabilities combine into collaborative output; how an agent executes belongs to Agents, how the team is assembled belongs here.',
        brief: ['Member orchestration and workflow modes (sequential / parallel / loop)', 'Dev-team template: planner / coder / reviewer in one click', 'Local directory binding; file and Git assets managed and auditable'],
        flow: ['Create a project (or one-click a dev-team template)', 'Orchestrate member agents and collaboration config', 'Delegate tasks in sessions with traceable progress', 'Artifacts land as managed files / Git'],
        coop: [
          'Agents: members and the coordinator all come from the agent library; delegation routes along capability descriptions',
          'Settings: external agents registered via A2A connectors join the collaboration surface as members',
          'Knowledge Base / Ontology: project sessions mount them too, so the team shares one knowledge context',
        ],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'TeamOutlined',
        i18n: {
          zh: { title: '协作工作空间', desc: '项目内编排成员智能体：谁参与、怎么委派、如何交接，配置收敛于项目侧；成员失败或超时降级回喂协调者，委派过程有结构化委派单' },
          en: { title: 'Collaboration workspace', desc: 'Orchestrate member agents per project: who joins, how work is delegated and handed over — all configured on the project side; failed or timed-out members feed back to the coordinator with structured delegation slips' },
        },
      },
      {
        icon: 'ApartmentOutlined',
        i18n: {
          zh: { title: '工作流编排', desc: 'workflow_mode 三模式落地：sequential 顺序 / parallel 并行 / loop 循环（次数上限），成员即节点由引擎调度；free 模式保持模型自主编排' },
          en: { title: 'Workflow orchestration', desc: 'Three workflow modes, delivered: sequential / parallel / loop (with caps) — members become engine-scheduled nodes; free mode keeps model-driven orchestration' },
        },
      },
      {
        icon: 'SolutionOutlined',
        i18n: {
          zh: { title: '协作研发模板', desc: '一键创建规划者（拆解委派汇总）/ 编码者（沙箱执行 + 产物落盘）/ 审查者（读产物复核出报告）三角色团队与示例任务种子，开箱演练多智能体研发动线' },
          en: { title: 'Dev-team template', desc: 'One click creates a planner (decompose & summarize) / coder (sandboxed execution & artifacts) / reviewer (verify outputs & report) trio plus seeded sample tasks — a ready-to-run multi-agent dev walkthrough' },
        },
      },
      {
        icon: 'FolderOutlined',
        i18n: {
          zh: { title: '目录绑定与文件纳管', desc: '绑定本地目录后，文件资产在项目内统一浏览、读写与审计；路径越界被强制拦截，读写全部进过程时间线' },
          en: { title: 'Directory binding & files', desc: 'Bind a local directory and browse, read/write and audit file assets within the project; path escapes are hard-blocked and every access lands in the trace' },
        },
      },
      {
        icon: 'BranchesOutlined',
        i18n: {
          zh: { title: 'Git 资产管理', desc: '项目文件变更纳入 Git 管理：分支切换、提交历史、逐提交变更明细与 diff 预览、工作区未提交变更一目了然' },
          en: { title: 'Git asset management', desc: 'Project files under Git: branch switching, commit history, per-commit change details and diff previews, plus uncommitted working-tree changes at a glance' },
        },
      },
      {
        icon: 'DeploymentUnitOutlined',
        i18n: {
          zh: { title: '协作拓扑与泳道', desc: '成员智能体组网与委派关系一图看清（含 A2A 外部成员节点）；执行过程按智能体分泳道回放，委派链路可追溯' },
          en: { title: 'Topology & swimlanes', desc: 'Member-agent networking and delegation in one diagram (A2A external members included); replay runs as per-agent swimlanes with traceable delegation chains' },
        },
      },
    ],
  },
  {
    key: 'skills',
    i18n: {
      zh: {
        title: '技能',
        tagline: '扩展机制：把「怎么做某类事」封装为可复用技能包',
        role: '智能体的能力扩展机制：技能包=领域指令+工具白名单，挂载即让智能体「会」一类事；配置级能力包，不引入新的运行时组件——协议与连接器不在此（归智能体/设置）。',
        brief: ['SKILL.md 标准技能包：领域指令 + 工具白名单', '.zip 批量导入（首批 177 件科学技能包，MIT）', '技能集合一卡多子技能；两级开关精细控制'],
        flow: ['新建技能或导入 .zip 标准包', '按主题组织技能集合', '在智能体上勾选挂载', '会话 chip 控制注入，skill.loaded 可见'],
        coop: [
          '智能体：唯一挂载点——agent 级勾选 + 会话级「技能」chip 两级开关',
          '工具面：技能的 tools 白名单与内置工具、连接器工具先到先得合并，组合打法一次封装处处复用',
        ],
      },
      en: {
        title: 'Skills',
        tagline: 'The extension mechanism: "how to do a class of work" as reusable skill packs',
        role: 'The capability extension for agents: a skill pack = domain instruction + tool allowlist; mounting makes an agent "know" a class of work. Configuration-level packs, no new runtime components — protocols and connectors live in Agents/Settings.',
        brief: ['SKILL.md standard packs: domain instruction + tool allowlist', 'Batch .zip import (177 scientific packs first batch, MIT)', 'Collections: one card, many sub-skills; two-level switches'],
        flow: ['Create a skill or import a standard .zip', 'Organize skills into collections', 'Mount skills on agents by checkbox', 'Session chip controls injection; skill.loaded stays visible'],
        coop: [
          'Agents: the single mounting point — agent-level checkboxes plus a session-level "skills" chip',
          'Tool surface: a skill\'s tools merge first-come-first-served with built-in and connector tools — package a playbook once, reuse it everywhere',
        ],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'ThunderboltOutlined',
        i18n: {
          zh: { title: '技能封装', desc: '结构化技能包 = 领域 instruction + 工具白名单 + 附加资源：instruction 装配期注入系统提示词，tools 并入工具集；内置「结构化输出约束」「K8s 排障指引」示例开箱可看' },
          en: { title: 'Skill packaging', desc: 'A structured pack = domain instruction + tool allowlist + resources: the instruction is injected into the system prompt at assembly, tools join the toolset; built-in "structured output" and "K8s troubleshooting" samples ship out of the box' },
        },
      },
      {
        icon: 'ImportOutlined',
        i18n: {
          zh: { title: '标准包导入', desc: '「导入技能（.zip）」批量解析业界 SKILL.md 标准包：frontmatter→字段、正文经 load_skill 懒加载、同目录资源随包入库；同名跳过不覆盖、限额防线超限如实标注；首批导入 177 件科学技能包（MIT）' },
          en: { title: 'Standard-pack import', desc: '"Import skills (.zip)" batch-parses industry-standard SKILL.md packs: frontmatter mapped to fields, body lazy-loaded via load_skill, sibling resources ingested; same-name skipped, quota overruns honestly reported; first batch — 177 scientific packs (MIT)' },
        },
      },
      {
        icon: 'AppstoreOutlined',
        i18n: {
          zh: { title: '技能集合', desc: '一卡多子技能：智能体挂载集合即启用其下全部成员，各子技能保留独立触发粒度、工具白名单取并集；技能包导入可按包自动归集成卡（不嵌套，成员须普通技能）' },
          en: { title: 'Skill collections', desc: 'One card, many sub-skills: mounting a collection enables all members, each keeping its own trigger granularity, tool allowlists unioned; zip imports can auto-group by pack (no nesting — members must be plain skills)' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '两级开关与过程可见', desc: 'agent 级勾选挂载 + 会话级「技能」chip 两级开关；运行事件 skill.loaded 标注注入——本智能体本次启用了哪些技能，对话里看得到' },
          en: { title: 'Two-level switches & visibility', desc: 'Agent-level checkbox mounting plus a session-level "skills" chip; skill.loaded events mark every injection — which skills this agent enabled in this conversation is visible in-chat' },
        },
      },
    ],
  },
  {
    key: 'settings',
    i18n: {
      zh: {
        title: '设置',
        tagline: '底座：模型连接 · 连接器 · 协议面 · 运维自助',
        role: '平台的连接与运维底座：模型连接、外部连接器、协议面与运维自助统一在此；自身不含智能能力——所有模块从这里取连接与凭据，密钥不落到任何消费方。',
        brief: ['双协议多厂商模型连接，云端预设 + 本地 Ollama', '四类连接器（MCP/K8s/SSH/A2A）统一注册与凭据托管', '协议面总览与运维自助：诊断包 / 备份恢复 / 用量统计'],
        flow: ['配置供应商与模型连接（云端/本地预设）', '连接器凭据与 agent 级授权', '协议面总览（MCP 接入指引/A2A 任务）', '使用统计与诊断/备份'],
        coop: [
          '智能体 / 项目：chat 连接与连接器授权均以 agent 为消费单元引用此处配置',
          '知识库 / 本体：embedding 与 AI 创建复用同一套连接，Key 加密不外泄',
          '全部模块：诊断自助包与一键备份覆盖全平台数据面',
        ],
      },
      en: {
        title: 'Settings',
        tagline: 'The foundation: model connections · connectors · protocols · ops self-service',
        role: 'The connection & ops foundation: model connections, external connectors, protocol overview and self-service ops live here; no intelligence of its own — every module draws connections and credentials from here, and keys never reach a consumer.',
        brief: ['Dual-protocol multi-vendor connections; cloud presets + local Ollama', 'Four connector kinds (MCP/K8s/SSH/A2A), unified registry and credential hosting', 'Protocol overview and ops: diagnostics / backup-restore / usage stats'],
        flow: ['Configure vendors & model connections (cloud / local presets)', 'Connector credentials and per-agent authorization', 'Protocol overview (MCP how-to / A2A tasks)', 'Usage stats, diagnostics & backup'],
        coop: [
          'Agents / Projects: chat connections and connector grants are consumed per agent from here',
          'Knowledge Base / Ontology: embedding and AI creation reuse the same connections — keys stay encrypted',
          'All modules: the diagnostics bundle and one-click backup cover the whole platform data plane',
        ],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'KeyOutlined',
        i18n: {
          zh: { title: '模型连接', desc: 'openai_compat / anthropic 双协议接入多厂商模型（厂商预设一键填充 + 自动发现拉模型列表），API Key 本地加密；本地 Ollama 等预设让零 Key 也能跑通' },
          en: { title: 'Model connections', desc: 'Connect multi-vendor models via openai_compat / anthropic dual protocols (one-click vendor presets + auto-discovery of model lists), keys encrypted locally; local presets like Ollama run with zero API keys' },
        },
      },
      {
        icon: 'ClusterOutlined',
        i18n: {
          zh: { title: '连接器管理', desc: '设置页连接器分区：MCP / K8s / SSH / A2A 四类实例 CRUD、凭据 AES-256-GCM 加密托管、连接测试（A2A 注册即握手校验 agent card）；授权精确到 agent 级白名单' },
          en: { title: 'Connector management', desc: 'A connectors section in settings: CRUD for MCP / K8s / SSH / A2A instances, AES-256-GCM encrypted credentials and connection tests (A2A registration handshakes the agent card up front); authorization precise to agent-level whitelists' },
        },
      },
      {
        icon: 'ApiOutlined',
        i18n: {
          zh: { title: '协议面总览', desc: '「协议」分区一页看全：MCP 服务面接入指引（客户端配置一键复制）、A2A 发现与 agent card 预览、A2A 任务列表（状态/产物/错误）——能力本体在智能体模块，这里是观测与管理面' },
          en: { title: 'Protocol overview', desc: 'One "Protocols" section: MCP serving instructions with copy-ready client configs, A2A discovery with agent-card previews, and an A2A task list (status / artifacts / errors) — the capability lives in Agents; this is the observation & management plane' },
        },
      },
      {
        icon: 'ToolOutlined',
        i18n: {
          zh: { title: '运维自助', desc: '一键诊断包（健康快照 + 最近错误 + 脱敏配置导出）与一键备份恢复（SQLite 快照 + 伴生图 + 智能体文件打包）' },
          en: { title: 'Ops self-service', desc: 'One-click diagnostics bundle (health snapshot, recent errors, redacted config) and one-click backup/restore (SQLite snapshots + companion graphs + agent files)' },
        },
      },
      {
        icon: 'DashboardOutlined',
        i18n: {
          zh: { title: '运行环境与统计', desc: '沙箱后端按需选择、推理后端探测；run 级 token 用量与轮次统计可回放' },
          en: { title: 'Runtime & usage stats', desc: 'Pick the sandbox backend per need with backend probing; per-run token usage and turn counts are replayable' },
        },
      },
    ],
  },
  {
    key: 'design',
    i18n: {
      zh: {
        title: '整体设计',
        tagline: '深入了解入口：产品定位 / 架构分层 / 技术栈 / 协议面 / 部署形态',
        role: '深入了解的入口：产品定位、三平面架构、技术栈与四条设计原则的统一叙述——上面六个模块的每个取舍依据都在这里。',
        brief: ['产品定位与三平面 + 双代理架构', '技术栈总表与 REST+SSE / MCP / A2A 协议面', '设计原则四条：开源优先 / 可插拔 / 诚实降级 / 低侵入'],
        flow: ['读整体架构（三平面+双代理）', '五层执行模型与机制图', '设计原则与理论依据', '进入部署或在线演示'],
        coop: [
          '全部模块：开源优先 / 可插拔 / 诚实降级 / 低侵入贯穿每个模块的取舍',
          '部署：从这里进入本地 / Docker / K8s 三条部署路径',
        ],
      },
      en: {
        title: 'Overall Design',
        tagline: 'The deep-dive entry: positioning / architecture / tech stack / protocols / deployment',
        role: 'The deep-dive entry: positioning, the three-plane architecture, tech stack and four design principles in one narrative — the rationale behind every trade-off in the six modules above.',
        brief: ['Product positioning and the three-plane + dual-agent architecture', 'Tech-stack table and REST+SSE / MCP / A2A protocol surfaces', 'Four design principles: open-source first / pluggable / honest degradation / low intrusion'],
        flow: ['Read the architecture (three planes + dual agents)', 'Five-layer execution model with diagrams', 'Design principles and theoretical grounding', 'Head to deploy or the online demo'],
        coop: [
          'All modules: open-source first / pluggable / honest degradation / low intrusion run through every trade-off',
          'Deploy: local / Docker / K8s paths start here',
        ],
      },
    },
    links: [
      { href: '#/architecture', i18n: { zh: '整体架构图', en: 'Architecture' } },
      { href: '#/ecosystem', i18n: { zh: '开源生态', en: 'Ecosystem' } },
    ],
    features: [
      {
        icon: 'AimOutlined',
        i18n: {
          zh: { title: '产品定位', desc: '单机可部署的智能体 + 本体双核平台，数据全留本机，能力边界诚实标注' },
          en: { title: 'Positioning', desc: 'A self-hostable dual-core platform for agents + ontologies; data stays local, capability boundaries honestly labeled' },
        },
      },
      {
        icon: 'ClusterOutlined',
        i18n: {
          zh: { title: '三平面 + 双代理', desc: '接入 / 服务 / 资源三平面架构，Nginx + Hertz 双代理，一张总图看清服务边界与端口' },
          en: { title: 'Three planes + dual proxy', desc: 'Access / service / resource planes behind an Nginx + Hertz dual proxy; one diagram shows boundaries and ports' },
        },
      },
      {
        icon: 'ApiOutlined',
        i18n: {
          zh: { title: '协议面', desc: 'REST + SSE 全事件流对外；MCP 双向（消费本体 facade、供给智能体服务化）+ A2A 双向（暴露平台智能体、接入外部智能体）' },
          en: { title: 'Protocol surface', desc: 'REST + SSE event streams outward; MCP both ways (consuming the ontology facade, serving agents) and A2A both ways (exposing platform agents, pulling in external ones)' },
        },
      },
      {
        icon: 'CompassOutlined',
        i18n: {
          zh: { title: '设计原则', desc: '开源优先 / 可插拔抽象 / 诚实降级 / 低侵入扩展，四条铁律贯穿全部模块取舍' },
          en: { title: 'Design principles', desc: 'Open source first / pluggable abstractions / honest degradation / low-intrusion extension — four iron rules behind every trade-off' },
        },
      },
    ],
  },
]

/** 模块文案（语言维度读取） */
export function moduleText(m: ModuleInfo, locale: Locale): ModuleText {
  return m.i18n[locale]
}

/** 演示种子（后端 internal/seed 启动播种的同一批示例本体） */
export interface SeedSpec {
  key: string
  file: string
  spec: Spec
}

function parseSeed(key: string, file: string, raw: string): SeedSpec {
  return { key, file, spec: JSON.parse(raw) as Spec }
}

export const SEEDS: SeedSpec[] = [
  parseSeed('orgs', 'orgs.json', orgsRaw),
  parseSeed('defects', 'defects.json', defectsRaw),
  parseSeed('failure', 'failure.json', failureRaw),
  parseSeed('gene_core', 'gene_core.json', geneCoreRaw),
  parseSeed('med_common', 'med_common.json', medCommonRaw),
]

export const seedCounts = (s: Spec) => ({
  concepts: s.concepts?.length ?? 0,
  relations: s.relations?.length ?? 0,
  instances: s.instances?.length ?? 0,
})
