// ---------------------------------------------------------------------------
// 官网内容单源（REQ-238）：演示数据 = ontology-service/internal/seed/examples/
// 种子 spec（后端启动播种同一份）。
// REQ-364 模块页去文档化：模块介绍为站点层结构化数据（tagline/brief/flow/
// features），全部中英双语（REQ-360：zh 为事实源）。
// 内容分工约定（2026-10-24）：每模块内容只聚焦本模块；与其他模块有交集处
// 在行文内顺带注明对方模块（「项目」「设置」等），两侧各讲各的切面、相互
// 呼应但不重复完整描述；跨模块能力唯一归属——MCP/A2A 协议能力=智能体段、
// 连接器管理面=设置段、外部成员协作=项目段，技能段只讲技能本体。
// 功能卡事实口径 = platform-knowledge/ 各模块 00 导读页「已交付能力总表 /
// 已交付能力表」与 docs/02/03/04/11/12（2026-10-24 起以能力总表为准），
// 仅做直观化摘编；已排期未开发条目以「（规划中）」标注状态（en: planned）。
// REQ-496 内容口径（2026-10-29）：模块页只讲「功能介绍」不讲技术细节——
// 内部实现术语（agent_as_tool / FTS5 / RRF / kind=cli / skill.loaded /
// spec_json / 存储与协议选型等）改写为用户能力语言，技术细节归
// 「实现与原理」页；同步最新交付能力（跨本体协同 / 图检索强化 /
// 专业编辑面 / 计划执行工作流 / OntoChat 工作台），新增「平台知识」模块卡。
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
  /** 速览要点（首屏直观化；3 条以内短句） */
  brief: string[]
  /** 典型流程（REQ-374：3~5 步动线，短句祈使式；内容对齐平台真实动线） */
  flow: string[]
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
        brief: ['定义智能体：提示词 / 模型 / 工具 / 知识库 / 本体 / 技能一处装配', '对话全程留痕：工具调用、委派、检索、审批可观测、可回放', '开放双向：智能体可被外部工具调用，也能把外部智能体请入协作'],
        flow: ['在设置页配好模型连接', '创建智能体，勾选工具 / 知识库 / 技能', '对话试用，轨迹面板看执行细节', '窗格对比不同模型与提示词', '在项目中编排成团队协作'],
      },
      en: {
        title: 'Agents',
        tagline: 'The execution core: define agents, chat directly, consume every platform capability',
        brief: ['Define agents: prompt / model / tools / knowledge / ontology / skills assembled in one place', 'Fully traced conversations: tool calls, delegations, retrieval and approvals — observable and replayable', 'Open both ways: agents get called by external tools and welcome external agents into collaboration'],
        flow: ['Configure a model connection in Settings', 'Create an agent, pick tools / knowledge / skills', 'Chat and inspect execution in the trace panel', 'Compare models and prompts side-by-side', 'Orchestrate the agent into a team in Projects'],
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
          zh: { title: '多智能体协作', desc: '把任务委派给合适的成员：协调者按各成员的能力描述自动路由，本地 / 命令行 / 远程智能体同队协作；子智能体上下文隔离、嵌套轨迹可见，成员失败或超时自动改道自愈，委派过程有单据可查；组队与工作流编排在「项目」模块' },
          en: { title: 'Multi-agent collaboration', desc: 'Delegate to the right member: the coordinator routes automatically by each member\'s capability profile — local, CLI and remote agents on one team; sub-agent contexts stay isolated with visible nested traces, failed or timed-out members re-route and self-heal, and every delegation leaves a queryable slip; team assembly and workflows live in the Projects module' },
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
          zh: { title: '上下文工程', desc: '三档上下文预算（compact / standard / full），超限自动摘要压缩；每轮喂给模型什么内容可控、可见' },
          en: { title: 'Context engineering', desc: 'Three context budget tiers (compact / standard / full) with automatic summary compression when over budget — what the model sees each turn stays under your control and in view' },
        },
      },
      {
        icon: 'SafetyOutlined',
        i18n: {
          zh: { title: '沙箱与审批', desc: '副作用在沙箱中隔离执行；危险操作分级审批 + 豁免清单，审批请求直接出现在对话里，过程全程留痕' },
          en: { title: 'Sandbox & approvals', desc: 'Side effects run isolated in a sandbox; dangerous operations take tiered approvals with an exemption list — approval requests appear right in the conversation, fully traced' },
        },
      },
      {
        icon: 'SyncOutlined',
        i18n: {
          zh: { title: '断点续跑与定时任务', desc: '长时间任务中断后可从断点恢复；支持按周期自动运行（间隔与次数可控），任务统一列表管理；挂起等待审批时主动提醒（规划中）' },
          en: { title: 'Resume & scheduling', desc: 'Long tasks resume from where they stopped; run automatically on a schedule with controllable intervals and run caps, managed from one task list; proactive reminders while waiting on approvals (planned)' },
        },
      },
      {
        icon: 'RocketOutlined',
        i18n: {
          zh: { title: '对外服务化（MCP + A2A）', desc: '任意智能体一键开放为标准服务：MCP 服务可被 Claude Desktop / Cursor 直连；A2A 端点可被其他智能体平台发现并调用，任务落库可查；危险操作照常过审批闸门，外部追问会转回给你确认；接入指引在「设置」页协议面' },
          en: { title: 'Serving outward (MCP + A2A)', desc: 'Turn any agent into a standard service in one click: as an MCP server it connects straight from Claude Desktop / Cursor; as an A2A endpoint it is discoverable and callable by other agent platforms, with tasks persisted and queryable; dangerous operations still pass approvals, and external follow-up questions come back to you for input; how-to guides live in the Settings protocol section' },
        },
      },
      {
        icon: 'ApiOutlined',
        i18n: {
          zh: { title: '外部智能体接入', desc: '把外部智能体请进协作：命令行编码智能体（Claude Code / OpenCode / Aider 等）、远程 A2A 智能体、本地 MCP 服务皆可入队为委派成员；能力画像自动登记、协调者据此智能路由，审批 / 审计 / 沙箱 / 凭据治理一视同仁，委派可携带项目文件与知识库上下文；三种成员一键成团；外部智能体亦可反向发现平台暴露的资产' },
          en: { title: 'External agents in', desc: 'Bring external agents into collaboration: CLI coding agents (Claude Code / OpenCode / Aider and more), remote A2A agents and local MCP services all join as delegable members; capability profiles register automatically for smart routing, approvals / audit / sandbox / credentials apply uniformly, delegations can carry project-file and knowledge context, a one-click template assembles a mixed team, and external agents can discover platform-exposed assets in reverse' },
        },
      },
      {
        icon: 'RobotOutlined',
        i18n: {
          zh: { title: '平台助手', desc: '内置系统级智能体：平台使用问题随问随答（官方文档同源）、配置辅助；复杂输入框旁「AI 优化」一键改写，先预览再确认回填' },
          en: { title: 'Platform assistant', desc: 'A built-in system agent: instant answers about the platform itself (from the same official docs) and configuration help; an "AI optimize" button rewrites complex inputs with preview-before-apply' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '评估驱动质量', desc: '内置评估任务集与智能评分：智能体行为改动可回归验证；每次运行的模型开销可查，成本看得见' },
          en: { title: 'Eval-driven quality', desc: 'Built-in evaluation task sets with automated scoring make behavior changes regression-verifiable; per-run model cost is queryable — spend stays visible' },
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
        brief: ['多条构建路径殊途同归，LLM 生成的产物必过质量门禁', '版本化本体资产：diff 对比、质量档案、演化候选受控生长', '多引擎运行平面，智能体经统一工具面消费与审计'],
        flow: ['选构建路径（对话 / 向导 / 导入 / 知识库）', '过质量门禁生成版本化资产', '图形 / 层级浏览治理与演化', '编排运行方案，一键启停', '智能体经统一工具面消费与审计'],
      },
      en: {
        title: 'Ontology',
        tagline: 'The structured-knowledge core: build → assets → run → consume & audit',
        brief: ['Build paths converge on one asset store; every LLM generation passes the quality gate', 'Versioned ontology assets: diff, quality history, controlled growth', 'Multi-engine runtime; agents consume and audit via a unified tool facade'],
        flow: ['Pick a build path (chat / wizard / import / KB)', 'Pass the quality gate into a versioned asset', 'Govern and evolve via graph & hierarchy views', 'Stage a run plan, start and stop in one click', 'Agents consume via the unified tool facade, fully audited'],
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
          zh: { title: '多条构建路径', desc: '手工向导 / AI 对话 / 由知识库构建 / 导入外部本体 / 增量扩展 / 权威词表一键引入（PROV-O、SKOS 等）——入口不同，出口同一套校验与资产能力' },
          en: { title: 'Multiple build paths', desc: 'Manual wizard / AI chat / build from knowledge base / external import / incremental extension / one-click authoritative vocabularies (PROV-O, SKOS and more) — different entries, identical validation and asset capabilities' },
        },
      },
      {
        icon: 'CommentOutlined',
        i18n: {
          zh: { title: '对话式构建', desc: 'OntoChat 对话工作台：澄清缺口、生成候选、逐轮确认，边聊边建；「访谈 → 问题清单 → 分析 → 覆盖测试」四步工作流内置；也可在任意智能体侧打开「对话建本体」能力' },
          en: { title: 'Conversational building', desc: 'The OntoChat workspace: clarify gaps, generate candidates and confirm round by round while you chat; a built-in four-step flow (interview → question list → analysis → coverage testing); or flip on "build via chat" in any agent side panel' },
        },
      },
      {
        icon: 'SafetyCertificateOutlined',
        i18n: {
          zh: { title: '质量门禁与档案', desc: '11 项检查 + 三维评分：LLM 生成的产物必过门禁，支持自动修复；资产级质量档案随版本追加——草稿 → 入库 → 当前对照与趋势线' },
          en: { title: 'Quality gate & history', desc: '11 checks with three-dimension scoring: every LLM generation must pass, with auto-repair; a per-asset quality history tracks draft → ingested → current, with a trend line' },
        },
      },
      {
        icon: 'DatabaseOutlined',
        i18n: {
          zh: { title: '资产治理', desc: '单一版本化资产：版本快照与 diff、fork 溯源、演化候选受控生长；人工编辑留痕可审计；TTL / CSV 等多种标准格式导出' },
          en: { title: 'Asset governance', desc: 'One versioned asset: snapshots & diff, fork provenance, controlled growth of evolution candidates; manual edits are logged and auditable; export to TTL, CSV and other standard formats' },
        },
      },
      {
        icon: 'ToolOutlined',
        i18n: {
          zh: { title: '专业编辑面', desc: '网页端完成专业本体编辑器级操作：公理与限制公理表单化编辑、关系特性声明（传递 / 对称 / 函数性 / 逆属性）、实体重命名自动改写全部引用、实例按类型化表单编辑' },
          en: { title: 'Professional editing', desc: 'Professional-editor-grade editing on the web: form-based axiom & restriction editing, relation characteristics (transitive / symmetric / functional / inverse), renaming that rewrites every reference automatically, and typed forms for instance editing' },
        },
      },
      {
        icon: 'NodeIndexOutlined',
        i18n: {
          zh: { title: '多本体协同', desc: '相互引用的本体作为整体使用：装载时自动补全引用集（缺失明确报错、循环引用合并装载），预计算跨本体推理，查询跨方案内全部本体并标注答案来源；导出自带跨本体引用与对齐信息' },
          en: { title: 'Multi-ontology synergy', desc: 'Mutually referencing ontologies work as a whole: loading auto-completes the reference set (missing refs fail loudly, cycles load as a union), cross-ontology inferences are precomputed, queries span every ontology in the plan and answers are labeled by source; exports carry cross-ontology references and alignments' },
        },
      },
      {
        icon: 'CloudServerOutlined',
        i18n: {
          zh: { title: '多引擎运行', desc: 'Oxigraph（查询）与 Fuseki（推理）双引擎按方案编排：一键安装、启停与重载；服务重启后自动对账恢复，伴生图随启停快照回放' },
          en: { title: 'Multi-engine runtime', desc: 'Oxigraph (query) and Fuseki (reasoning) engines staged per plan: one-click install, start/stop and reload; automatic reconciliation after restarts, companion graphs snapshot and replay' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '可视化', desc: '3D 力导向图与 VOWL 视图浏览本体结构，层级树三面联动定位；五栏工作台覆盖学习 → 构建 → 资产 → 运行 → 消费审计' },
          en: { title: 'Visualization', desc: 'Browse structure as 3D force graphs and VOWL views, with a three-way-linked hierarchy tree; a five-tab workbench covers learning → build → assets → run → consume & audit' },
        },
      },
      {
        icon: 'AuditOutlined',
        i18n: {
          zh: { title: '消费与审计', desc: '统一工具面供智能体消费：可回答性预检（提问前先验概念覆盖）、只读查询、本体操作与时序查询；挂载运行方案即注入；决策审计时间线与 PROV-O 溯源导出' },
          en: { title: 'Consume & audit', desc: 'A unified tool facade for agents: answerability pre-checks (verify concept coverage before asking), read-only queries, ontology operations and history queries — injected by mounting a run plan; decision audit timeline and PROV-O provenance export' },
        },
      },
      {
        icon: 'AimOutlined',
        i18n: {
          zh: { title: '业务本体路线（规划中）', desc: '已排期待开发：概念同义词一等公民、业务域起步模板、建表语句反推本体骨架、实例质量面板、概念词典导出' },
          en: { title: 'Business-ontology roadmap (planned)', desc: 'Scheduled, not yet built: first-class concept synonyms, business-domain starter templates, schema-to-ontology reverse engineering, instance quality panel and concept-dictionary export' },
        },
      },
    ],
  },
  {
    key: 'knowledge',
    i18n: {
      zh: {
        title: '知识库',
        tagline: '文档知识核：检索问答 / 知识图谱 / LLM Wiki 三形态',
        brief: ['混合检索：关键词与语义双路融合，能力按库开关', '知识图谱：导入即建图，多跳关联、社区摘要、溯源到句', 'LLM Wiki：整页合成互链百科，跨文档综合问答'],
        flow: ['上传文档 / 网页 / 结构化数据（含 DOCX / PDF）', '自动切分入库', 'GraphRAG 导入即建知识图谱', '检索调优（混合 / 多跳 / 社区摘要）', '挂载到智能体增强回答'],
      },
      en: {
        title: 'Knowledge Base',
        tagline: 'The document-knowledge core: retrieval Q&A / knowledge graph / LLM Wiki',
        brief: ['Hybrid retrieval: keyword + semantic fused, capability switches per base', 'Knowledge graph: built on import — multi-hop links, community summaries, sentence-level citation', 'LLM Wiki: synthesized interlinked pages for cross-document Q&A'],
        flow: ['Upload docs / pages / structured data (DOCX & PDF too)', 'Auto chunked and indexed', 'GraphRAG builds a KG on import', 'Tune retrieval (hybrid / multi-hop / community summaries)', 'Mount on agents to ground answers'],
      },
    },
    links: [
      { href: '#/implementation', i18n: { zh: '知识增强机制', en: 'Knowledge augmentation' } },
    ],
    features: [
      {
        icon: 'SearchOutlined',
        i18n: {
          zh: { title: '混合检索', desc: '关键词与语义两路检索融合排序、各取所长；命中带来源与策略标注，短问题自动换路；每个库可独立开关 RAG / GraphRAG / Wiki 能力' },
          en: { title: 'Hybrid retrieval', desc: 'Keyword and semantic retrieval fused into one ranking — the best of both; hits carry source and strategy tags, short queries switch lanes automatically; RAG / GraphRAG / Wiki toggle per base' },
        },
      },
      {
        icon: 'NodeIndexOutlined',
        i18n: {
          zh: { title: 'GraphRAG', desc: '导入即由大模型抽取知识图谱（实体 / 关系 / 论断，论断可溯源到原文段落）；检索在图上多跳扩展找到关联证据；社区摘要回答「这个库整体在讲什么」；抽取可挂本体做约束对齐，语料也能反哺本体构建' },
          en: { title: 'GraphRAG', desc: 'A knowledge graph is extracted on import (entities / relations / claims, claims traceable to source passages); retrieval expands multi-hop over the graph for linked evidence; community summaries answer "what is this corpus about overall"; extraction can align to an ontology, and the corpus feeds ontology building too' },
        },
      },
      {
        icon: 'DeploymentUnitOutlined',
        i18n: {
          zh: { title: '图检索强化', desc: '图谱检索同时理解两层信号：语义相关性 + 实体在关系网络中的重要性——多跳问题回答质量经内置基准实测提升；图谱重建后实体身份保持稳定、引用不断链' },
          en: { title: 'Graph-aware ranking', desc: 'Graph retrieval reads two signals at once — semantic relevance and how pivotal an entity is in the relation network — measurably improving multi-hop answers on the built-in benchmark; entity identities survive graph rebuilds, so references never break' },
        },
      },
      {
        icon: 'FileTextOutlined',
        i18n: {
          zh: { title: 'LLM Wiki', desc: '把整库写时合成为互链的百科页面，问答「读页不读片段」、跨文档综合；无需向量化即可使用，与检索 / 图谱按语料形态并存互补' },
          en: { title: 'LLM Wiki', desc: 'Synthesizes the whole corpus into interlinked wiki pages — Q&A reads pages, not fragments, across documents with zero embedding dependency; coexists with retrieval and the graph by corpus shape' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '检索可观测与句级溯源', desc: '检索过程全程可见：命中片段、三元组、来源文档一目了然；回答论断句级高亮回链原文，实体消歧支持自动建议 + 人工确认' },
          en: { title: 'Observable retrieval & citations', desc: 'Retrieval is observable end to end: chunks, triples and source documents at a glance; answer claims highlight at sentence level back to the source, with entity disambiguation via auto-suggest plus human confirm' },
        },
      },
      {
        icon: 'ProfileOutlined',
        i18n: {
          zh: { title: '富格式摄入', desc: 'DOCX / PDF 文本直接抽取入库，导入后台异步索引不打断使用；OCR 与表格抽取（规划中）' },
          en: { title: 'Rich-format ingestion', desc: 'DOCX / PDF text extracts on import, with background indexing that never blocks your work; OCR and table extraction (planned)' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '评测基准', desc: '内置种子语料与三类基准问题集（多跳 / 全局 / 关键词）加跑分器：检索质量可量化，改动先跑分再上线；增量重建与可插拔图存储（规划中）' },
          en: { title: 'Retrieval benchmarks', desc: 'A seed corpus and three benchmark question sets (multi-hop / global / keyword) with a scorer: retrieval quality is measurable — score before you ship; incremental rebuild and pluggable graph storage (planned)' },
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
        brief: ['成员编排与工作流模式（顺序 / 并行 / 循环 / 计划执行）', '协作研发模板：规划者 / 编码者 / 审查者一键成团', '本地目录绑定，文件与 Git 资产纳管可审计'],
        flow: ['创建项目（或一键套用协作研发模板）', '编排「智能体」库中的成员与协作配置', '会话内委派任务，过程轨迹可查', '产物文件 / Git 纳管沉淀'],
      },
      en: {
        title: 'Projects',
        tagline: 'The orchestration layer: multi-agent workspaces (workflows / templates / files / Git)',
        brief: ['Member orchestration and workflow modes (sequential / parallel / loop / plan-execute)', 'Dev-team template: planner / coder / reviewer in one click', 'Local directory binding; file and Git assets managed and auditable'],
        flow: ['Create a project (or one-click a dev-team template)', 'Arrange members from the Agents library and collaboration config', 'Delegate tasks in sessions with traceable progress', 'Artifacts land as managed files / Git'],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'TeamOutlined',
        i18n: {
          zh: { title: '协作工作空间', desc: '项目内编排成员智能体——成员与协调者来自「智能体」库，外部智能体（远程 / 命令行）经「设置」连接器注册后亦可入队；谁参与、怎么委派、如何交接，配置收敛于项目侧（委派韧性与过程留痕见「智能体」模块）' },
          en: { title: 'Collaboration workspace', desc: 'Orchestrate member agents per project — members and the coordinator come from the Agents library, and external agents (remote / CLI) registered via Settings connectors can join too; who joins, how work is delegated and handed over is configured here (delegation resilience and tracing live in the Agents module)' },
        },
      },
      {
        icon: 'ApartmentOutlined',
        i18n: {
          zh: { title: '工作流编排', desc: '顺序 / 并行 / 循环 / 计划执行（先规划再执行）四种编排模式，成员即节点自动调度；也可交给模型自主编排' },
          en: { title: 'Workflow orchestration', desc: 'Four orchestration modes — sequential / parallel / loop / plan-execute (plan first, run after) — with members as auto-scheduled nodes; or hand orchestration to the model' },
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
          zh: { title: '协作拓扑与泳道', desc: '成员组网与委派关系一图看清（含外部成员节点）；执行过程按智能体分泳道回放，本地与外部成员统一时间线，委派链路可追溯' },
          en: { title: 'Topology & swimlanes', desc: 'Member networking and delegation in one diagram (external members included); replay runs as per-agent swimlanes with a unified timeline across local and external members and traceable delegation chains' },
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
        brief: ['标准技能包：领域指令 + 工具白名单，一次封装多智能体复用', '.zip 批量导入（首批 177 件科学技能包，MIT）', '技能集合一卡多子技能；挂载与会话两级开关'],
        flow: ['新建技能或导入 .zip 标准包', '按主题组织技能集合', '到「智能体」配置勾选挂载', '会话内开关控制，启用了什么对话里可见'],
      },
      en: {
        title: 'Skills',
        tagline: 'The extension mechanism: "how to do a class of work" as reusable skill packs',
        brief: ['Standard packs: domain instruction + tool allowlist, packaged once reused everywhere', 'Batch .zip import (177 scientific packs first batch, MIT)', 'Collections: one card, many sub-skills; mount-level and session-level switches'],
        flow: ['Create a skill or import a standard .zip', 'Organize skills into collections', 'Mount them in the Agents config by checkbox', 'Toggle per session — what is enabled stays visible in chat'],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'ThunderboltOutlined',
        i18n: {
          zh: { title: '技能封装', desc: '技能 = 领域指令 + 可选工具白名单 + 附加资源：挂载到智能体后，指令在对话装配时注入、工具并入可用面；内置「结构化输出约束」「K8s 排障指引」示例开箱可看' },
          en: { title: 'Skill packaging', desc: 'A skill = domain instruction + optional tool allowlist + resources: once mounted, the instruction is injected at assembly and its tools join the agent\'s toolset; built-in "structured output" and "K8s troubleshooting" samples ship out of the box' },
        },
      },
      {
        icon: 'ImportOutlined',
        i18n: {
          zh: { title: '标准包导入', desc: '批量导入业界标准技能包（SKILL.md 格式）：正文与随包资源一并入库，正文用到时才加载；同名跳过不覆盖、数量超限如实标注；首批已导入 177 件科学技能包（MIT）' },
          en: { title: 'Standard-pack import', desc: 'Batch-import industry-standard skill packs (SKILL.md format): body and bundled resources ingest together, loaded only when used; same-name packs skip without overwriting, quota overruns honestly reported; first batch — 177 scientific packs (MIT)' },
        },
      },
      {
        icon: 'AppstoreOutlined',
        i18n: {
          zh: { title: '技能集合', desc: '一卡多子技能：挂载集合即启用其下全部成员，各子技能保留独立触发粒度、工具白名单取并集；导入可按包自动归集成卡' },
          en: { title: 'Skill collections', desc: 'One card, many sub-skills: mounting a collection enables all members, each keeping its own trigger granularity with tool allowlists unioned; imports can auto-group into collection cards' },
        },
      },
      {
        icon: 'EyeOutlined',
        i18n: {
          zh: { title: '两级开关与过程可见', desc: '智能体配置勾选挂载 + 会话内「技能」开关两级控制；本次对话启用了哪些技能，对话里看得到' },
          en: { title: 'Two-level switches & visibility', desc: 'Two-level control — mount by checkbox in the agent config, toggle per session; which skills this conversation enabled is visible in-chat' },
        },
      },
    ],
  },
  {
    key: 'platformkb',
    i18n: {
      zh: {
        title: '平台知识',
        tagline: '平台的自我说明：需求 / 方案 / 调研全量文档，站内可读、可检索、可追问',
        brief: ['与知识目录 1:1：助手维护的文档，人在页面即可见', '目录定位 + 语义检索组合，直达目标段落', 'AI 助手同源答疑：回答给出处，可跳转原文'],
        flow: ['打开「平台知识」按目录浏览', '搜索或让助手定位主题', '精读正文与关联来源', '版本更新自动同步可见'],
      },
      en: {
        title: 'Platform Knowledge',
        tagline: 'The platform explains itself: requirements / design / research docs, readable, searchable and askable in-app',
        brief: ['1:1 with the knowledge tree: docs maintained by agents are visible to humans', 'Directory lookup + semantic search combine to land on the right passage', 'The AI assistant answers from the same docs, with sources you can jump to'],
        flow: ['Browse the tree in Platform Knowledge', 'Search, or ask the assistant to locate a topic', 'Read the passage and its linked sources', 'Updates sync automatically and stay visible'],
      },
    },
    features: [
      {
        icon: 'AppstoreOutlined',
        i18n: {
          zh: { title: '文档树与导读', desc: '七大子目录与平台导航一一对应；每模块导读页先讲全貌，正文与源文件指针齐全，越读越深' },
          en: { title: 'Doc tree & overviews', desc: 'Seven sub-directories mirror the platform navigation; every module opens with an overview page, and passages carry pointers to their sources — the deeper you go, the more specific it gets' },
        },
      },
      {
        icon: 'SearchOutlined',
        i18n: {
          zh: { title: '检索与定位', desc: '目录发现、关键词定位、语义搜索、分段精读组合使用，几步直达目标段落' },
          en: { title: 'Search & locate', desc: 'Directory discovery, keyword location, semantic search and passage-level reading combine to land on the right spot in a few steps' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '知识沉淀卡', desc: '决策 / 概念 / 实践三类知识卡沉淀项目方法论，学习与引用两相宜' },
          en: { title: 'Knowledge cards', desc: 'Decision / concept / practice knowledge cards distill the project\'s methodology — good to learn from and to cite' },
        },
      },
      {
        icon: 'RobotOutlined',
        i18n: {
          zh: { title: '助手同源答疑', desc: '平台助手基于同一份文档回答使用问题，答案附出处；文档即助手的知识源，人与 AI 读到同一份事实' },
          en: { title: 'Assistant answers from the same docs', desc: 'The platform assistant answers usage questions from the very same documents, sources attached — humans and AI read the same facts' },
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
        brief: ['多厂商模型连接一处配置，云端预设 + 本地模型', '六类连接器统一注册，凭据加密托管', '协议面总览与运维自助：诊断 / 备份 / 用量统计'],
        flow: ['配置供应商与模型连接（云端 / 本地预设）', '连接器凭据与智能体级授权', '协议面总览（MCP 接入指引 / A2A 任务）', '使用统计与诊断 / 备份'],
      },
      en: {
        title: 'Settings',
        tagline: 'The foundation: model connections · connectors · protocols · ops self-service',
        brief: ['Multi-vendor model connections in one place; cloud presets + local models', 'Six connector kinds registered uniformly with encrypted credential hosting', 'Protocol overview and ops self-service: diagnostics / backup / usage stats'],
        flow: ['Configure vendors & model connections (cloud / local presets)', 'Connector credentials and per-agent authorization', 'Protocol overview (MCP how-to / A2A tasks)', 'Usage stats, diagnostics & backup'],
      },
    },
    links: [
      { href: '#/deploy', i18n: { zh: '部署运行', en: 'Deploy' } },
    ],
    features: [
      {
        icon: 'KeyOutlined',
        i18n: {
          zh: { title: '模型连接', desc: '全平台唯一的模型配置入口：主流协议接入多厂商模型（OpenAI 兼容 / Anthropic 兼容），厂商预设一键填充、自动发现模型列表；密钥本地加密，「智能体」「知识库」「本体」统一引用此处连接，密钥不落消费方；本地 Ollama 等预设让零 Key 也能跑通' },
          en: { title: 'Model connections', desc: 'The single model-config entry for the whole platform: multi-vendor models over the mainstream protocols (OpenAI-compatible / Anthropic-compatible) with one-click vendor presets and auto-discovery; keys stay encrypted locally — Agents, Knowledge Base and Ontology all reference connections from here, and keys never reach a consumer; local presets like Ollama run with zero keys' },
        },
      },
      {
        icon: 'ClusterOutlined',
        i18n: {
          zh: { title: '连接器管理', desc: 'MCP / Kubernetes / SSH / A2A / 命令行 / 本地服务六类连接器统一注册：凭据加密托管、注册时自动校验连通；外部智能体与工具的使用权在「智能体」侧按需勾选' },
          en: { title: 'Connector management', desc: 'Six connector kinds — MCP / Kubernetes / SSH / A2A / CLI / local services — register uniformly with encrypted credentials and automatic connectivity checks at registration; what each agent may use is granted per agent in the Agents side panel' },
        },
      },
      {
        icon: 'ApiOutlined',
        i18n: {
          zh: { title: '协议面总览', desc: '「协议」分区一页看全：MCP 服务接入指引（客户端配置一键复制）、A2A 发现预览与任务列表（状态 / 产物 / 错误）——协议能力本体在「智能体」模块，这里是观测与管理面' },
          en: { title: 'Protocol overview', desc: 'One "Protocols" section: MCP serving instructions with copy-ready client configs, A2A discovery previews and a task list (status / artifacts / errors) — the capability itself lives in the Agents module; this is the observation & management plane' },
        },
      },
      {
        icon: 'ToolOutlined',
        i18n: {
          zh: { title: '运维自助', desc: '一键诊断包（健康快照 + 最近错误 + 脱敏配置导出）与一键备份恢复（数据快照 + 伴生图 + 智能体文件打包）' },
          en: { title: 'Ops self-service', desc: 'One-click diagnostics bundle (health snapshot, recent errors, redacted config) and one-click backup/restore (data snapshots + companion graphs + agent files)' },
        },
      },
      {
        icon: 'DashboardOutlined',
        i18n: {
          zh: { title: '运行环境与统计', desc: '沙箱与推理后端按需选择、可用性自动探测；每次运行的用量统计可回放' },
          en: { title: 'Runtime & usage stats', desc: 'Sandbox and reasoning backends selectable per need with automatic availability probing; per-run usage stats are replayable' },
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
        brief: ['产品定位与三平面 + 双代理架构', '技术栈总表与 REST+SSE / MCP / A2A 协议面', '设计原则四条：开源优先 / 可插拔 / 诚实降级 / 低侵入'],
        flow: ['读整体架构（三平面+双代理）', '五层执行模型与机制图', '设计原则与理论依据', '进入部署或在线演示'],
      },
      en: {
        title: 'Overall Design',
        tagline: 'The deep-dive entry: positioning / architecture / tech stack / protocols / deployment',
        brief: ['Product positioning and the three-plane + dual-agent architecture', 'Tech-stack table and REST+SSE / MCP / A2A protocol surfaces', 'Four design principles: open-source first / pluggable / honest degradation / low intrusion'],
        flow: ['Read the architecture (three planes + dual agents)', 'Five-layer execution model with diagrams', 'Design principles and theoretical grounding', 'Head to deploy or the online demo'],
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
          zh: { title: '产品定位', desc: '面向专业个人的工作学习助手，以本体为知识底座；自托管部署，数据不出个人掌控，能力边界诚实标注' },
          en: { title: 'Positioning', desc: 'A personal work & learning assistant for professionals, grounded on ontologies; self-hosted with data under your control, capability boundaries honestly labeled' },
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
          zh: { title: '协议面', desc: 'REST + SSE 全事件流对外；MCP 双向（消费本体工具面、供给智能体服务化）+ A2A 双向（暴露平台智能体、接入外部智能体）' },
          en: { title: 'Protocol surface', desc: 'REST + SSE event streams outward; MCP both ways (consuming the ontology tool facade, serving agents) and A2A both ways (exposing platform agents, pulling in external ones)' },
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
