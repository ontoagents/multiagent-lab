import { CompanionPipeline, QualityGateFlow, SseTimeline } from '../components/diagrams'
import { REPO_SLUG, REPO_URL } from '../content'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'

// ---------------------------------------------------------------------------
// 实现与原理页（REQ-238 扩展；REQ-364 由「关键实现 + 实现原理」两页合并而来，
// 旧链接 #/principles 在 App.tsx 重定向到本页）：
//   关键实现要点 → 三张核心机制图 → 五层执行视角 → 本体与知识机制 →
//   设计原则 → 理论与实践依据 → 源码入口 → 延伸阅读。
// 页内目录用 scrollIntoView（不能用 #hash 锚点，会与 hash 路由冲突）。
// 事实口径 = docs/02 §6/§7/§8 + docs/03/04 + platform-knowledge/ 整体设计、
// 02_智能体/53~56、04_本体/02~04、05_知识库/00 + docs/15 登记簿；本页只做可读版转述。
// REQ-360：全页文案中英双份（zh 为事实源）；机制图内文字随语言切换。
// ---------------------------------------------------------------------------

/** 关键实现要点：六组（原关键实现页） */
const POINTS: { i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    i18n: {
      zh: {
        title: '配置驱动装配',
        desc: '模型连接 → 智能体 → 项目三层配置全部存库（SQLite），每次运行按当前配置动态装配 Eino Agent——改配置下一次运行即生效，无需改代码重启。模型连接支持 openai_compat / anthropic 双协议（协议是连接属性，REQ-172/177）。',
      },
      en: {
        title: 'Config-driven assembly',
        desc: 'All three config layers — model connections → agents → projects — live in the database (SQLite); every run dynamically assembles the Eino agent from current config, so config changes take effect on the next run with no code change or restart. Model connections support dual protocols, openai_compat / anthropic (protocol is a connection attribute, REQ-172/177).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'SSE 过程完全可观测',
        desc: '对话执行中的工具调用、深度思考、子智能体委派、知识召回、本体查询均以来源着色的事件卡呈现；三档观测级别 + 模型级调用链 + 事件流重放（REQ-117），过程不再黑盒。',
      },
      en: {
        title: 'Fully observable SSE process',
        desc: 'Tool calls, deep thinking, sub-agent delegation, knowledge retrieval and ontology queries all render as source-colored event cards during execution; three observation levels + model-level call chains + event-stream replay (REQ-117) — no more black box.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'LLM 生成必过质量门禁',
        desc: '本体六条构建路径中所有 LLM 参与的生成产物，必须通过 qualitygate（11 检查项 + 三维评分）并支持自动修复；导入侧统一收口导入审查合并——收敛于资产段，杜绝低质量产物入库（REQ-171）。',
      },
      en: {
        title: 'LLM generations must pass the quality gate',
        desc: 'Across all six ontology build paths, anything LLM-generated must pass qualitygate (11 checks + three-dimension scoring) with auto-repair support; imports funnel through import review/merge — converging on the asset stage to keep low-quality artifacts out of the store (REQ-171).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '伴生本体（动态薄本体）',
        desc: '对话收尾旁路抽取知识候选，人工确认后入会话图并自动并入检索源；开关默认关、产物可摘除、失败静默降级——低侵入三原则的实例化（REQ-170）。',
      },
      en: {
        title: 'Companion ontology (dynamic thin ontology)',
        desc: 'A bypass extracts knowledge candidates as conversations wrap up; after human confirmation they enter the session graph and automatically join retrieval sources. Off by default, removable, silent degradation on failure — the three low-intrusion principles instantiated (REQ-170).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '可插拔抽象',
        desc: '执行后端（inprocess / docker / k8s）、推理后端（eino-adk / claude-code / opencode / aider / deepseek-harness，D-O13）、向量存储、图引擎均接口先行、实现可替换；开源优先，自研只保留装配层与路由薄层。',
      },
      en: {
        title: 'Pluggable abstractions',
        desc: 'Execution backends (inprocess / docker / k8s), reasoning backends (eino-adk / claude-code / opencode / aider / deepseek-harness, D-O13), vector stores and graph engines are all interface-first with replaceable implementations; open source first, with in-house code limited to thin assembly and routing layers.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '协议枢纽双向 + 诚实降级',
        desc: '消费侧：本体 facade 五工具挂给 Agent、外部 MCP / stdio server 亦可接入；供给侧：任意智能体一键 MCP 服务化（/mcp）或暴露为 A2A 端点（agent card 发现 + message/send·stream，任务落库）；CLI 编码智能体（Claude Code / OpenCode / Aider / DSH）与 A2A 远程智能体经连接器接入协作委派。方案未运行、向量库不可用等场景明确提示并降级，主链路不受影响。',
      },
      en: {
        title: 'Protocol hub, both ways + honest degradation',
        desc: 'Consumer side: the five ontology facade tools attach to any agent, and external MCP / stdio servers plug in too. Provider side: any agent can be exposed via MCP (/mcp) or as an A2A endpoint (agent-card discovery + message/send · stream, tasks persisted); CLI coding agents (Claude Code / OpenCode / Aider / DSH) and remote A2A agents join collaboration as delegated members via connectors. When a plan isn’t running or the vector store is unavailable, the platform says so clearly and degrades — the main path is unaffected.',
      },
    },
  },
]

/** 五层视角：智能体执行模型的五个正交切面（源自 platform-knowledge/02_智能体/53~56） */
const LAYERS: { key: string; i18n: L10n<{ title: string; scope: string; points: string[] }> }[] = [
  {
    key: 'Model',
    i18n: {
      zh: {
        title: 'Model · 模型层',
        scope: '连哪个模型、怎么采样',
        points: [
          '模型连接持久化，openai_compat / anthropic 双协议统一适配温度、MaxTokens 等采样参数',
          '推理后端可插拔（D-O13）：eino-adk / claude-code / opencode / aider / deepseek-harness 五种 Harness 后端，模型驱动的循环交给谁执行是可替换决策',
        ],
      },
      en: {
        title: 'Model',
        scope: 'Which model to call and how to sample',
        points: [
          'Model connections are persisted; dual protocols openai_compat / anthropic uniformly adapt sampling params like temperature and MaxTokens',
          'Pluggable reasoning backends (D-O13): eino-adk / claude-code / opencode / aider / deepseek-harness — who executes the model-driven loop is a replaceable decision',
        ],
      },
    },
  },
  {
    key: 'Context',
    i18n: {
      zh: {
        title: 'Context · 上下文层',
        scope: '模型每轮真正看到什么',
        points: [
          '预算三档（compact≈6k / standard≈24k / full），超预算先前缀摘要、再八段式压缩持久化跨轮复用',
          '工具结果剪枝（>4000 字符留头 2400 + 尾 800）；tool 轮次保全——悬空调用补"结果未知"，不破坏消息结构',
          '召回链：知识库命中与伴生本体三元组按序注入，retrieval 事件全程可观测',
        ],
      },
      en: {
        title: 'Context',
        scope: 'What the model actually sees each turn',
        points: [
          'Three budget tiers (compact≈6k / standard≈24k / full); over budget, prefix summary first, then eight-segment compression persisted for cross-turn reuse',
          'Tool-result pruning (>4000 chars keeps head 2400 + tail 800); tool-turn integrity — dangling calls get a "result unknown" filler without breaking message structure',
          'Recall chain: knowledge-base hits and companion-ontology triples injected in order, with retrieval events observable throughout',
        ],
      },
    },
  },
  {
    key: 'Harness',
    i18n: {
      zh: {
        title: 'Harness · 执行层',
        scope: '怎么把事做安全、做扎实',
        points: [
          '8 步工具装配链：内置 → 技能 → 连接器 → 本体 facade → 项目文件 → 执行原语 → Hook 包装 → 审批包装',
          'HookChain 防护：SSRF 预检、危险命令模式拦截；审批三档（off / danger / all）+ 豁免清单，挂起请求随 checkpoint 持久化',
          '验证背压 verify_on_stop 与沙箱后端（inprocess / docker / k8s）构成"生成 → 校验 → 阻断"闭环',
        ],
      },
      en: {
        title: 'Harness',
        scope: 'Getting things done safely and solidly',
        points: [
          'An 8-step tool assembly chain: built-ins → skills → connectors → ontology facade → project files → execution primitives → hook wrapping → approval wrapping',
          'HookChain protection: SSRF pre-checks, dangerous-command pattern interception; three approval tiers (off / danger / all) + exemption lists, pending requests persisted with checkpoints',
          'Verification backpressure (verify_on_stop) and sandbox backends (inprocess / docker / k8s) form a "generate → verify → block" loop',
        ],
      },
    },
  },
  {
    key: 'Loop',
    i18n: {
      zh: {
        title: 'Loop · 循环层',
        scope: '跑多久、断了怎么办、花了多少',
        points: [
          'ReAct 迭代上限默认 25，超限即停可恢复；checkpoint 快照落库，进程重启后挂起运行可续',
          '定时续跑：调度状态入库（间隔 ≥1min、次数封顶），长期任务不依赖进程常驻',
          'todo.md 进度产物 + run 级 usage 统计，token 消耗与轮次可回放',
        ],
      },
      en: {
        title: 'Loop',
        scope: 'How long it runs, what happens on interruption, what it costs',
        points: [
          'ReAct iteration cap defaults to 25 — stops on exceed, resumable; checkpoint snapshots persist so suspended runs survive process restarts',
          'Scheduled resume: schedule state in the database (interval ≥1min, capped count) so long tasks don’t require a resident process',
          'todo.md progress artifact + per-run usage stats; token spend and turns are replayable',
        ],
      },
    },
  },
  {
    key: 'Graph',
    i18n: {
      zh: {
        title: 'Graph · 协作层',
        scope: '多个智能体怎么组队',
        points: [
          'agent_as_tool 为推荐协作路径：成员智能体包装为工具，协调者按"能力描述"路由',
          'subagent.enter / exit 事件记录嵌套轨迹；子智能体上下文隔离，起到"防火墙"语义',
          '本地 / CLI / A2A 三色成员同队：外部成员画像注入协调者智能路由，审批 / 审计 / 沙箱治理拉齐，跨协议过程统一泳道可视化',
          'transfer 控制权移交仅保留兼容位；协作与工作流编排配置全部在项目侧收敛',
        ],
      },
      en: {
        title: 'Graph',
        scope: 'How multiple agents team up',
        points: [
          'agent_as_tool is the recommended collaboration path: member agents wrapped as tools, the coordinator routes by capability descriptions',
          'subagent.enter / exit events record nesting traces; sub-agents run with isolated contexts acting as a "firewall"',
          'Local / CLI / A2A members in one team: external-member profiles feed smart routing, approvals / audit / sandbox governance applies uniformly, cross-protocol processes render in unified swimlanes',
          'transfer control handoff remains only as a compatibility slot; collaboration and workflow orchestration config converges on the project side',
        ],
      },
    },
  },
]

/** 本体工程机制（源自 platform-knowledge/04_本体/02~04 与整体设计） */
const ONTOLOGY: { i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    i18n: {
      zh: {
        title: 'spec_json 单一资产',
        desc: '仓库即唯一事实源（D-O1）：本体以 spec_json 形态落库，TTL / VOWL JSON / CSV 等都是它的导出视图；版本快照、diff 与 fork 溯源全部围绕这一份资产展开，运行方案只是它的"部署视图"。',
      },
      en: {
        title: 'spec_json as the single asset',
        desc: 'The repository is the only source of truth (D-O1): ontologies persist as spec_json, with TTL / VOWL JSON / CSV as export views; version snapshots, diff and fork provenance all revolve around this one asset — runtime plans are just its "deployment view".',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '六路径殊途同归',
        desc: '手工向导、AI 对话（OntoChat 流程）、由知识库构建、导入外部本体、OntoExtend 扩展、权威本体库引入——六条构建路径最终走同一套校验循环、收敛到同一段资产能力，入口不同、门禁一致。',
      },
      en: {
        title: 'Six paths, one destination',
        desc: 'Manual wizard, AI chat (OntoChat flow), build from knowledge base, external ontology import, OntoExtend evolution, and authoritative ontology library import — all six build paths run the same validation loop and converge on the same asset capabilities: different entries, identical gates.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '质量门禁与修复环',
        desc: 'Spec.Validate 单一结构校验 → qualitygate 11 项三维评分 → LLM 生成走"双校验源"修复环，≤3 轮铁律；进化候选以 vN-cK 配对门控评估，质量回退超过阈值不予合入。',
      },
      en: {
        title: 'Quality gate and repair loop',
        desc: 'Spec.Validate single structural validation → qualitygate’s 11 checks with three-dimension scoring → LLM generations run a dual-source repair loop, hard-capped at 3 rounds; evolution candidates are evaluated with vN-cK paired gating and merges are rejected when quality regresses past threshold.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '多引擎运行平面',
        desc: 'Oxigraph（SPARQL 1.1，单二进制）与 Fuseki（OWL 推理）按方案编排启停；启动对账支持重启领养，逐本体图隔离装载（REQ-320），推理对账下限闸（REQ-329）保证"引擎三元组 ≥ 源"才放行。',
      },
      en: {
        title: 'Multi-engine runtime plane',
        desc: 'Oxigraph (SPARQL 1.1, single binary) and Fuseki (OWL reasoning) start/stop per plan; startup reconciliation supports restart adoption, per-ontology graph isolation (REQ-320), and the reasoning reconciliation lower-bound gate (REQ-329) only passes when "engine triples ≥ source".',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'MCP facade 消费',
        desc: '本体能力以 6 个只读 onto_* 工具（概念查询 / 实例枚举 / 邻居扩展 / SPARQL）经 MCP facade 暴露给智能体，只读白名单与图隔离保证消费侧不污染数据源。',
      },
      en: {
        title: 'Consuming via MCP facade',
        desc: 'Ontology capabilities are exposed to agents through six read-only onto_* tools (concept query / instance enumeration / neighbor expansion / SPARQL) via the MCP facade; read-only whitelists and graph isolation keep the consumer side from polluting data sources.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '伴生本体旁路',
        desc: '对话中增量抽取候选三元组，人工确认后进入会话图，可并入检索源——旁路落地、默认关闭、随时可摘除，不侵入主链路。',
      },
      en: {
        title: 'Companion-ontology bypass',
        desc: 'Candidate triples are extracted incrementally during conversation and, after human confirmation, enter the session graph and can join retrieval sources — a bypass that’s off by default, removable anytime, never intruding on the main path.',
      },
    },
  },
]

/** 知识增强机制（源自 platform-knowledge/05_知识库/00） */
const KNOWLEDGE: { i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    i18n: {
      zh: {
        title: 'RAG 管道',
        desc: '固定长度 + 重叠切分 → 向量化 → TopK 检索注入，retrieval 事件打点来源；向量后端可插拔（Qdrant 主力，SQLite 余弦兜底），单机部署零外部依赖。',
      },
      en: {
        title: 'RAG pipeline',
        desc: 'Fixed-size chunking with overlap → embedding → TopK retrieval injection, with retrieval events marking sources; pluggable vector backends (Qdrant primary, SQLite cosine fallback) keep standalone deployments dependency-free.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'GraphRAG',
        desc: '索引时由 LLM 抽取知识图谱（失败回退规则抽取，degraded 不阻断导入）；实体 / 关系 / 主张三表带 chunk_id 溯源；检索时向量命中 → 图一跳扩展 → 关系与主张汇出，多跳问题优于纯向量召回。',
      },
      en: {
        title: 'GraphRAG',
        desc: 'At index time an LLM extracts the knowledge graph (rule-based fallback on failure; degraded mode never blocks import); entity / relation / claim tables carry chunk_id provenance; retrieval does vector hit → one-hop graph expansion → relations & claims merge, beating pure vector recall on multi-hop questions.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'LLM Wiki',
        desc: '写时合成为互链 Markdown 页面层，检索"读页不读片段"，零 embedding 依赖；与 RAG、GraphRAG 三类知识库并存互补，按语料形态选择。',
      },
      en: {
        title: 'LLM Wiki',
        desc: 'Synthesizes interlinked Markdown pages at write time; retrieval "reads pages, not fragments" with zero embedding dependency. It coexists with RAG and GraphRAG — choose per corpus shape.',
      },
    },
  },
]

/** 设计原则四条（源自 platform-knowledge/01_整体设计/00 §7） */
const PRINCIPLES: { i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    i18n: {
      zh: {
        title: '开源优先',
        desc: '凡有成熟开源实现，不重复造轮子：Eino / Oxigraph / Fuseki / rdflib / Qdrant 等核心组件均为社区验证过的项目，自研代码聚焦胶水与编排。',
      },
      en: {
        title: 'Open source first',
        desc: 'Never reinvent what mature open source already provides: core components like Eino / Oxigraph / Fuseki / rdflib / Qdrant are community-proven; in-house code focuses on glue and orchestration.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '可插拔抽象',
        desc: '模型、推理后端、向量库、SPARQL 引擎、沙箱后端全部面向接口编程，换实现不动业务代码。',
      },
      en: {
        title: 'Pluggable abstractions',
        desc: 'Models, reasoning backends, vector stores, SPARQL engines and sandbox backends are all programmed against interfaces — swap implementations without touching business code.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '诚实降级',
        desc: '依赖缺失时明确声明 degraded 并降级运行（如 Qdrant 不可用退 SQLite 余弦、KG 抽取失败回退规则抽取），绝不静默假装成功。',
      },
      en: {
        title: 'Honest degradation',
        desc: 'When a dependency is missing, declare degraded explicitly and keep running (e.g. fall back to SQLite cosine when Qdrant is down, or rule-based extraction when KG extraction fails) — never silently pretend success.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '低侵入扩展',
        desc: '伴生本体旁路、知识库导入、外部工具接入均遵循"可并行、可关闭、可摘除"，扩展不绑架主链路。',
      },
      en: {
        title: 'Low-intrusion extension',
        desc: 'The companion-ontology bypass, knowledge-base import and external tool integration all follow "run in parallel, can be turned off, can be removed" — extensions never hijack the main path.',
      },
    },
  },
]

/** 标准与协议（第二册 T3 类） */
const STANDARDS: { name: string; by: string; url: string; use: L10n<string> }[] = [
  { name: 'Model Context Protocol (MCP)', by: 'Anthropic · 2024', url: 'https://modelcontextprotocol.io', use: { zh: '智能体 ↔ 工具 / 本体 facade 的统一协议底座', en: 'The unified protocol base for agents ↔ tools and the ontology facade' } },
  { name: 'Agent2Agent Protocol (A2A)', by: 'Google 发起 · Linux Foundation · 2025', url: 'https://a2a-protocol.dev', use: { zh: 'Agent ↔ Agent 互操作协议：平台 A2A 端点与委派客户端的协议底座（与 MCP 互补——MCP=工具层、A2A=Agent 层）', en: 'The agent-to-agent interop protocol: the base for the platform’s A2A endpoints and delegation client (complementary to MCP — MCP for tools, A2A for agents)' } },
  { name: 'RDF 1.1', by: 'W3C', url: 'https://www.w3.org/TR/rdf11-concepts/', use: { zh: '本体数据模型与序列化基础（Turtle）', en: 'Data model and serialization base for ontologies (Turtle)' } },
  { name: 'OWL 2', by: 'W3C', url: 'https://www.w3.org/TR/owl2-overview/', use: { zh: '本体表达力来源（类 / 属性 / 公理），Fuseki 推理目标方言', en: 'Source of ontology expressivity (classes / properties / axioms); the reasoning target dialect for Fuseki' } },
  { name: 'SHACL', by: 'W3C', url: 'https://www.w3.org/TR/shacl/', use: { zh: '约束校验与 qualitygate 结构类规则的依据', en: 'Basis for constraint validation and the structural rules in qualitygate' } },
  { name: 'SPARQL 1.1', by: 'W3C', url: 'https://www.w3.org/TR/sparql11-query/', use: { zh: '图查询语言，工作台与 MCP facade 的查询接口', en: 'The graph query language behind the workbench and MCP facade query interface' } },
  { name: 'PROV-O', by: 'W3C', url: 'https://www.w3.org/TR/prov-o/', use: { zh: '本体构建过程的来源追溯建模参考', en: 'Modeling reference for provenance tracking of the construction process' } },
]

/** 论文与文献（第二册 T1 / T3 代表条目，完整台账见 docs/15） */
const PAPERS: { name: string; meta: string; url: string; use: L10n<string> }[] = [
  { name: 'ReAct: Synergizing Reasoning and Acting', meta: 'Yao et al. · arXiv 2210.03629 · 2022', url: 'https://arxiv.org/abs/2210.03629', use: { zh: '推理-行动交织范式，智能体运行循环（Loop）的理论原型', en: 'The reasoning-acting interleaving paradigm — theoretical prototype of the agent loop' } },
  { name: 'GraphRAG (From Local to Global)', meta: 'Microsoft Research · arXiv 2404.16130 · 2024', url: 'https://arxiv.org/abs/2404.16130', use: { zh: '知识图谱增强 RAG 的方法论源头，GraphRAG 知识库实现依据', en: 'Methodological origin of KG-augmented RAG; basis of the GraphRAG knowledge base' } },
  { name: 'OntoChat', meta: 'ESWC 2024 · arXiv 2403.05921', url: 'https://arxiv.org/abs/2403.05921', use: { zh: '对话式本体工程四步工作流（用户故事 → 能力问题 → 聚类 → 测试），"AI 对话构建"路径蓝本', en: 'Four-step conversational ontology engineering workflow (user story → CQ → clustering → test); blueprint of the AI-chat build path' } },
  { name: 'Participatory Prompting for Ontology Construction', meta: 'arXiv 2408.15256 · 2024', url: 'https://arxiv.org/abs/2408.15256', use: { zh: '参与式提示设计，人机协作建本的交互策略', en: 'Participatory prompt design — interaction strategies for human-AI co-construction' } },
  { name: 'OntoExtend', meta: 'arXiv 2607.17963 · 2026', url: 'https://arxiv.org/abs/2607.17963', use: { zh: '需求驱动的三段式本体扩展（Retriever → Extender → Integrator），增量进化路径参照', en: 'Requirement-driven three-stage ontology extension (Retriever → Extender → Integrator); reference for incremental evolution' } },
  { name: 'Online Clustering-based Ontology Extension', meta: 'Frontiers in Big Data · 2024', url: 'https://www.frontiersin.org/articles/10.3389/fdata.2024.1463543/full', use: { zh: '对话中自动聚簇生长本体的路线，伴生本体机制的理论支撑', en: 'Growing ontologies via in-conversation clustering — theoretical support for the companion mechanism' } },
  { name: 'NeOn Methodology', meta: 'Suárez-Figueroa et al. · 2012', url: 'https://link.springer.com/chapter/10.1007/978-3-642-24794-1_5', use: { zh: '协作式本体工程七阶段模型，多路径构建流程的方法论源头', en: 'The seven-stage collaborative ontology engineering model; methodological origin of multi-path construction' } },
  { name: 'TOVE: Towards Enterprise Ontologies', meta: 'Gruninger & Fox · 1995', url: 'https://doi.org/10.1007/BFb0026565', use: { zh: '能力问题（Competency Questions）评估法原始出处，CQ 测试的传统依据', en: 'Original source of the Competency Questions evaluation method; traditional basis for CQ testing' } },
  { name: 'Ontology Development 101', meta: 'Noy & McGuinness · 2001', url: 'https://protege.stanford.edu/publications/ontology_development/ontology101.pdf', use: { zh: '本体建模入门基线，新手向导与文档措辞的参照', en: 'The beginner baseline for ontology modeling; reference for the new-user wizard and documentation wording' } },
  { name: 'LLM Wiki (write-time accumulation)', meta: 'A. Karpathy · gist · 2026', url: 'https://gist.github.com/karpathy/2364b8bcb5b1b5cf4d58', use: { zh: '"写时合成、读页不读片段"知识层范式，LLM Wiki 知识库类型依据', en: 'The "synthesize at write time, read pages not fragments" paradigm; basis of the LLM Wiki knowledge-base type' } },
]

/** 已集成开源（第一册 ✅ 已集成 类，完整台账见 docs/15） */
const INTEGRATED: { name: string; license: string; use: L10n<string> }[] = [
  { name: 'CloudWeGo Eino + Eino ADK', license: 'Apache-2.0', use: { zh: 'Go 智能体编排主骨架：组件抽象、图编排、ReAct / 多智能体运行时', en: 'The Go agent orchestration backbone: component abstractions, graph orchestration, ReAct / multi-agent runtime' } },
  { name: 'CloudWeGo Hertz', license: 'Apache-2.0', use: { zh: 'backend HTTP 服务与 API 网关', en: 'Backend HTTP service and API gateway' } },
  { name: 'mcp-go', license: 'MIT', use: { zh: 'MCP 协议 Go 实现，facade 与客户端连接的基座', en: 'MCP protocol in Go — the base for the facade and client connections' } },
  { name: 'eino-ext', license: 'Apache-2.0', use: { zh: 'MCP Client 组件与 claude 模型适配器（REQ-172）', en: 'MCP Client component and the claude model adapter (REQ-172)' } },
  { name: 'rdflib (Python)', license: 'BSD-3', use: { zh: 'sidecar 进程：TTL 解析 / 图校验 / 导出三职责（D-O5 轻量路线）', en: 'Sidecar process: TTL parsing / graph validation / export (D-O5 lightweight route)' } },
  { name: 'pySHACL', license: 'Apache-2.0', use: { zh: 'SHACL 约束校验执行器', en: 'SHACL constraint validation executor' } },
  { name: 'Oxigraph', license: 'Apache-2.0 / MIT', use: { zh: 'P1 唯一 SPARQL 引擎（pin v0.5.11），单二进制免部署', en: 'The only P1 SPARQL engine (pinned v0.5.11), single binary, zero deployment' } },
  { name: 'Apache Jena Fuseki', license: 'Apache-2.0', use: { zh: 'P2 OWL 推理引擎，方案化编排启停', en: 'P2 OWL reasoning engine, orchestrated per plan' } },
  { name: 'Qdrant', license: 'Apache-2.0', use: { zh: '向量检索主后端（KB_VECTOR_BACKEND 可插拔）', en: 'Primary vector retrieval backend (pluggable via KB_VECTOR_BACKEND)' } },
  { name: 'ReactFlow (@xyflow/react)', license: 'MIT', use: { zh: '工作流编辑器画布（D-O9 择优定案）', en: 'Workflow editor canvas (chosen after comparison in D-O9)' } },
  { name: 'CodeMirror 6', license: 'MIT', use: { zh: '代码 / SPARQL 编辑器', en: 'Code / SPARQL editors' } },
  { name: 'react-diff-viewer-continued', license: 'MIT', use: { zh: '本体版本 diff 可视化', en: 'Ontology version diff visualization' } },
  { name: '3d-force-graph / react-force-graph-3d', license: 'MIT', use: { zh: '三维力导向本体可视化（REQ-154 / 179）', en: '3D force-directed ontology visualization (REQ-154 / 179)' } },
  { name: 'WebVOWL', license: 'MIT', use: { zh: 'VOWL 视觉规范参考，owl2vowl 导出形态对齐', en: 'VOWL visual notation reference; aligned with owl2vowl export form' } },
  { name: 'santhosh-tekuri/jsonschema', license: 'MIT', use: { zh: 'spec_json 结构校验（Spec.Validate 底层）', en: 'spec_json structural validation (under Spec.Validate)' } },
]

/** 借鉴思想的开源项目（第一册 🔭 类：未引入代码，采纳设计思想） */
const INSPIRED: { name: string; takeaway: L10n<string> }[] = [
  { name: 'semantica (D-O15)', takeaway: { zh: '项目退役后按 D-O15 反转"去-semantica 化"，仍保留其三层架构与 GraphRAG / PROV-O 溯源思路作对照参考', en: 'After the project retired, D-O15 reversed the dependency ("de-semantica-ization"); its three-layer architecture and GraphRAG / PROV-O provenance ideas remain as reference' } },
  { name: 'OntoChat', takeaway: { zh: '对话式构建流程与开源 prompt 库可复用，REQ-82/103 的蓝本', en: 'Conversational construction flow and reusable open prompts; blueprint of REQ-82/103' } },
  { name: 'OntoExtend', takeaway: { zh: '三段式扩展管线（检索 → 扩展 → 整合）映射到六路径之"OntoExtend"', en: 'Three-stage extension pipeline (retrieve → extend → integrate), mapped to the OntoExtend build path' } },
  { name: 'Open Ontologies (docs 65/66)', takeaway: { zh: '判词纪律与 plan-apply 工作流借鉴，2026-10-08 拍板摘除集成（D-O 编号登记）', en: 'Verdict discipline and plan-apply workflow borrowed; integration removed by decision on 2026-10-08 (registered as a D-O decision)' } },
  { name: 'Helix Foundry', takeaway: { zh: 'base+overlay+audit 三层编辑保护与结构确定性推断（REQ-292~295 批次）', en: 'base+overlay+audit three-layer edit protection and deterministic structural inference (REQ-292~295 batch)' } },
  { name: 'kubernetes-ontology', takeaway: { zh: '边级溯源（K1）、AI 消费契约（K2）、canonicalId（K3）设计', en: 'Edge-level provenance (K1), AI consumption contract (K2), canonicalId (K3) designs' } },
  { name: 'Microsoft Ontology-Playground', takeaway: { zh: '定位同构的平台对照，验证"本体 + 智能体"产品方向的合理性', en: 'A structurally identical platform — validation of the "ontology + agents" product direction' } },
  { name: 'ontology-on-snowflake', takeaway: { zh: '本体质量基准分档（50/60/70/78.2%）为 qualitygate 评分提供量标依据（REQ-313）', en: 'Tiered ontology quality benchmarks (50/60/70/78.2%) ground the qualitygate scoring scale (REQ-313)' } },
  { name: 'Graphify', takeaway: { zh: '确定性解析优先 + LLM 兜底、边级可信度、增量更新策略', en: 'Deterministic parsing first with LLM fallback, edge-level confidence, incremental update strategy' } },
  { name: 'HelixDB', takeaway: { zh: 'pre-filter → ranking 检索管线借鉴', en: 'pre-filter → ranking retrieval pipeline borrowed' } },
  { name: 'Kartograph', takeaway: { zh: '架构边界可执行化（G1）与统一写路径（G2）', en: 'Executable architecture boundaries (G1) and a unified write path (G2)' } },
  { name: 'LLM Wiki ecosystem', takeaway: { zh: '三问裁决不引入代码，采纳"写时积累"范式另立知识库类型', en: 'A three-question review decided against importing code; the "write-time accumulation" paradigm became a new knowledge-base type' } },
]

const GUIDES: { hash: string; i18n: L10n<{ title: string; desc: string }> }[] = [
  { hash: '#/architecture', i18n: { zh: { title: '整体架构', desc: '三平面 + 双代理拓扑总图' }, en: { title: 'Architecture', desc: 'Three planes + dual-proxy topology diagram' } } },
  { hash: '#/ecosystem', i18n: { zh: { title: '开源生态', desc: '全量项目关联图谱与七类关键特征对比' }, en: { title: 'Ecosystem', desc: 'Full project relation graph and seven-category comparison' } } },
  { hash: '#/module/design', i18n: { zh: { title: '平台知识体系', desc: '模块导读与领域事实源' }, en: { title: 'Platform knowledge', desc: 'Module guides and domain sources of truth' } } },
]

const COPY = {
  zh: {
    crumbsHome: '首页',
    title: '实现与原理',
    tagline: '这一页回答三个问题：机制如何工作（实现要点 + 三张机制图 + 五层视角）、为什么这样设计（设计原则与取舍）、凭什么可信（标准、论文与开源依据）。',
    toc: [
      { id: 'impl-points', label: '实现要点' },
      { id: 'impl-diagrams', label: '机制图' },
      { id: 'impl-layers', label: '五层视角' },
      { id: 'impl-onto', label: '本体与知识' },
      { id: 'impl-principles', label: '设计原则' },
      { id: 'impl-evidence', label: '依据' },
      { id: 'impl-src', label: '源码' },
    ],
    s1Title: '关键实现要点',
    sseTitle: 'SSE 事件流：对话过程完全可观测',
    sseNotePre: '事件顺序：',
    sseNoteMid: '。前端按事件来源着色渲染为可展开的事件卡，支持调试原始事件与导出重放。协议细节见 ',
    sseNoteLink: 'docs/02 §8',
    sseNotePost: '。',
    qgTitle: '六条构建路径收敛于质量门禁与资产段',
    qgNotePre: '设计要义：',
    qgNoteB: '差异只在生成段，收敛在资产段',
    qgNoteMid: '——无论手工向导、AI 对话、OO 双轨导入、由知识库构建还是 CSV 灌装，产物一律经 qualitygate（LLM 生成必过 + 自修复）进入统一的多形态资产仓库；版本、diff、审查合并、进化候选均作用于资产层。',
    compTitle: '伴生本体：对话驱动的动态薄本体',
    compNote:
      '与「先建本体再对话」的传统路径互补：在对话收尾时由旁路抽取候选，人工确认后进入会话图并自动并入 KG 检索源——本体随对话生长，而主链路零感知。',
    s2Eyebrow: 'Agent Execution',
    s2Title: '智能体执行模型：五层视角',
    s2Desc: '把「AgentLab 如何运行一个智能体」沿五个正交切面拆开看——每层有独立配置、独立事件契约与代码落点，单层可替换、可观测。',
    s2Note: '五层的运行时序统一由 SSE 事件流暴露，事件协议与三张机制图见上方 ',
    s2NoteLink: '机制图',
    s2NotePost: '。',
    s3Eyebrow: 'Ontology & Knowledge',
    s3Title: '本体工程与知识增强机制',
    s3Desc: '本体：六条构建路径殊途同归——入口可以是对话、导入或向导，出口必然是同一份 spec_json 资产与同一套质量门禁。知识：三类知识库按语料形态并存互补。',
    s4Eyebrow: 'Design Principles',
    s4Title: '设计原则',
    s4Desc: '贯穿全部模块取舍的四条铁律。',
    s5Eyebrow: 'Evidence & References',
    s5Title: '理论与实践依据',
    s5Desc: '平台机制不是凭空发明：标准与协议给数据形态，论文给方法论，开源项目给工程实践。以下为代表性条目，完整台账（含七类分级、全部 25 篇论文）见 docs/15 登记簿。',
    refStandards: '标准与协议',
    refPapers: '论文与方法论',
    refIntegrated: '已集成开源项目',
    badgeIntegrated: '✅ 生产在用',
    refInspired: '借鉴思想的项目',
    badgeInspired: '🔭 思想采纳',
    integratedHeadLicense: '许可证',
    note15Pre: '台账维护约定（',
    note15Link: 'docs/15',
    note15Mid: '）：新项目 / 新论文一经调研即登记，七类分级 + 六状态流转；本页仅展示进入生产或影响设计的条目。',
    srcTitle: '源码入口',
    srcItems: [
      { href: 'backend/internal/chat', desc: ' —— Eino ADK 装配（inprocess + docker 分发）与 SSE 运行器' },
      { href: 'ontology-service', desc: ' —— 六路径 / qualitygate / spec_json 资产仓库' },
      { href: 'runtime-manager', desc: ' —— Oxigraph / Fuseki 方案编排与 MCP facade' },
      { href: 'backend/internal/companion', desc: ' —— 伴生本体旁路管线' },
      { href: 'web/src/pages/ontology', desc: ' —— 本体五栏前端（构建 / 资产 / 运行 / 消费审计）' },
    ],
    nextTitle: '延伸阅读',
  },
  en: {
    crumbsHome: 'Home',
    title: 'Implementation & Principles',
    tagline: 'This page answers three questions: how the mechanisms work (implementation highlights + three diagrams + the five-layer view), why they are designed this way (principles and trade-offs), and why they are trustworthy (standards, papers and open-source evidence).',
    toc: [
      { id: 'impl-points', label: 'Highlights' },
      { id: 'impl-diagrams', label: 'Diagrams' },
      { id: 'impl-layers', label: 'Five layers' },
      { id: 'impl-onto', label: 'Ontology & knowledge' },
      { id: 'impl-principles', label: 'Principles' },
      { id: 'impl-evidence', label: 'Evidence' },
      { id: 'impl-src', label: 'Source' },
    ],
    s1Title: 'Implementation highlights',
    sseTitle: 'SSE event stream: fully observable conversations',
    sseNotePre: 'Event order: ',
    sseNoteMid: '. The frontend renders them as expandable, source-colored event cards with raw-event debugging and export & replay. Protocol details: ',
    sseNoteLink: 'docs/02 §8',
    sseNotePost: '.',
    qgTitle: 'Six build paths converge on the quality gate and asset stage',
    qgNotePre: 'Design essence: ',
    qgNoteB: 'differences live only in generation; everything converges on the asset stage',
    qgNoteMid: ' — manual wizard, AI chat, OO dual-track import, from-knowledge-base or CSV fill: all artifacts enter the unified multi-form asset store through qualitygate (mandatory for LLM generations, with self-repair); versions, diff, review merge and evolution candidates all act on the asset layer.',
    compTitle: 'Companion ontology: a conversation-driven dynamic thin ontology',
    compNote:
      'Complementary to the traditional "build the ontology first, then chat" path: a bypass extracts candidates as the conversation wraps up; after human confirmation they enter the session graph and automatically join KG retrieval sources — the ontology grows with the conversation while the main path stays oblivious.',
    s2Eyebrow: 'Agent Execution',
    s2Title: 'The agent execution model: a five-layer view',
    s2Desc: '“How does AgentLab run an agent?” split along five orthogonal facets — each layer has its own config, event contract and code location; any single layer is replaceable and observable.',
    s2Note: 'The runtime sequence of all five layers is exposed via the SSE event stream; the event protocol and three diagrams are above under ',
    s2NoteLink: 'Diagrams',
    s2NotePost: '.',
    s3Eyebrow: 'Ontology & Knowledge',
    s3Title: 'Ontology engineering and knowledge enhancement',
    s3Desc: 'Ontology: six build paths, one destination — the entry may be chat, import or a wizard, but the exit is always the same spec_json asset and the same quality gate. Knowledge: three knowledge-base types coexist by corpus shape.',
    s4Eyebrow: 'Design Principles',
    s4Title: 'Design principles',
    s4Desc: 'Four iron rules that run through every module trade-off.',
    s5Eyebrow: 'Evidence & References',
    s5Title: 'Evidence & references',
    s5Desc: 'These mechanisms weren’t invented from nothing: standards define data shapes, papers provide methodology, open-source projects contribute engineering practice. Representative entries below; the full registry (seven categories, all 25 papers) is in docs/15.',
    refStandards: 'Standards & protocols',
    refPapers: 'Papers & methodology',
    refIntegrated: 'Integrated open source',
    badgeIntegrated: '✅ In production',
    refInspired: 'Projects whose ideas we borrowed',
    badgeInspired: '🔭 Idea adopted',
    integratedHeadLicense: 'License',
    note15Pre: 'Registry convention (',
    note15Link: 'docs/15',
    note15Mid: '): every new project / paper is registered as soon as it’s researched, with seven-category grading and a six-status lifecycle; this page shows only what entered production or influenced design.',
    srcTitle: 'Source code entry points',
    srcItems: [
      { href: 'backend/internal/chat', desc: ' — Eino ADK assembly (inprocess + docker dispatch) and the SSE runner' },
      { href: 'ontology-service', desc: ' — six paths / qualitygate / the spec_json asset store' },
      { href: 'runtime-manager', desc: ' — Oxigraph / Fuseki plan orchestration and the MCP facade' },
      { href: 'backend/internal/companion', desc: ' — the companion-ontology bypass pipeline' },
      { href: 'web/src/pages/ontology', desc: ' — the ontology five-tab frontend (build / assets / run / consumption & audit)' },
    ],
    nextTitle: 'Keep reading',
  },
}

function SectionTitle({ id, eyebrow, title, desc }: { id?: string; eyebrow?: string; title: string; desc?: string }) {
  return (
    <div className="sec-head" id={id}>
      {eyebrow && <span className="eyebrow">{eyebrow}</span>}
      <h2>{title}</h2>
      {desc && <p>{desc}</p>}
    </div>
  )
}

/** 页内目录跳转：hash 路由站点内不能再用 #hash 锚点，统一 scrollIntoView */
function scrollToId(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

export default function ImplementationPage() {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">{c.crumbsHome}</a> / {c.title}
        </div>
        <h1>{c.title}</h1>
        <p className="module-tagline">{c.tagline}</p>

        {/* 页内目录（REQ-364：合并后内容较长，先给地图） */}
        <nav className="page-toc" aria-label="Page sections">
          {c.toc.map((t) => (
            <button key={t.id} type="button" className="toc-item" onClick={() => scrollToId(t.id)}>
              {t.label}
            </button>
          ))}
        </nav>

        {/* ---------- 一、关键实现要点 ---------- */}
        <section className="sec" id="impl-points">
          <SectionTitle title={c.s1Title} />
          <div className="impl-grid">
            {POINTS.map((p, i) => (
              <div className="impl-card" key={i}>
                <div className="impl-index">{String(i + 1).padStart(2, '0')}</div>
                <div>
                  <div className="impl-title">{p.i18n[locale].title}</div>
                  <p className="impl-desc">{p.i18n[locale].desc}</p>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* ---------- 二、三张核心机制图 ---------- */}
        <section className="sec" id="impl-diagrams">
          <h2>{c.sseTitle}</h2>
          <div className="diag-card">
            <SseTimeline />
          </div>
          <p className="diag-note">
            {c.sseNotePre}
            <code>run.started → (skill.loaded) → agent.started → (retrieval) → message.delta* → (tool.call / tool.result | subagent.enter/exit | ontology.query)* → run.finished | run.error</code>
            {c.sseNoteMid}
            <a href={`${REPO_URL}/blob/main/docs/02_智能体_技术方案设计.md`} target="_blank" rel="noreferrer">
              {c.sseNoteLink}
            </a>
            {c.sseNotePost}
          </p>

          <h2>{c.qgTitle}</h2>
          <div className="diag-card">
            <QualityGateFlow />
          </div>
          <p className="diag-note">
            {c.qgNotePre}
            <b>{c.qgNoteB}</b>
            {c.qgNoteMid}
          </p>

          <h2>{c.compTitle}</h2>
          <div className="diag-card">
            <CompanionPipeline />
          </div>
          <p className="diag-note">{c.compNote}</p>
        </section>

        {/* ---------- 三、五层视角 ---------- */}
        <section className="sec" id="impl-layers">
          <SectionTitle eyebrow={c.s2Eyebrow} title={c.s2Title} desc={c.s2Desc} />
          <div className="layer-grid">
            {LAYERS.map((l) => (
              <article key={l.key} className="layer-card">
                <header>
                  <span className="layer-key">{l.key}</span>
                  <h3>{l.i18n[locale].title}</h3>
                  <span className="layer-scope">{l.i18n[locale].scope}</span>
                </header>
                <ul>
                  {l.i18n[locale].points.map((p, i) => (
                    <li key={i}>{p}</li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
          <p className="sec-note">
            {c.s2Note}
            <button type="button" className="linklike" onClick={() => scrollToId('impl-diagrams')}>
              {c.s2NoteLink}
            </button>
            {c.s2NotePost}
          </p>
        </section>

        {/* ---------- 四、本体工程与知识增强 ---------- */}
        <section className="sec" id="impl-onto">
          <SectionTitle eyebrow={c.s3Eyebrow} title={c.s3Title} desc={c.s3Desc} />
          <div className="onto-grid">
            {ONTOLOGY.map((o, oi) => (
              <article key={oi} className="onto-card">
                <h3>{o.i18n[locale].title}</h3>
                <p>{o.i18n[locale].desc}</p>
              </article>
            ))}
          </div>
          <div className="kb-grid">
            {KNOWLEDGE.map((k, ki) => (
              <article key={ki} className="kb-card">
                <h3>{k.i18n[locale].title}</h3>
                <p>{k.i18n[locale].desc}</p>
              </article>
            ))}
          </div>
        </section>

        {/* ---------- 五、设计原则 ---------- */}
        <section className="sec" id="impl-principles">
          <SectionTitle eyebrow={c.s4Eyebrow} title={c.s4Title} desc={c.s4Desc} />
          <div className="principle-grid">
            {PRINCIPLES.map((p, i) => (
              <article key={i} className="principle-card">
                <span className="principle-num">{String(i + 1).padStart(2, '0')}</span>
                <h3>{p.i18n[locale].title}</h3>
                <p>{p.i18n[locale].desc}</p>
              </article>
            ))}
          </div>
        </section>

        {/* ---------- 六、理论与实践依据 ---------- */}
        <section className="sec" id="impl-evidence">
          <SectionTitle eyebrow={c.s5Eyebrow} title={c.s5Title} desc={c.s5Desc} />

          <h3 className="ref-sub">{c.refStandards}</h3>
          <div className="ref-table">
            {STANDARDS.map((s) => (
              <a key={s.name} className="ref-row" href={s.url} target="_blank" rel="noreferrer">
                <span className="ref-name">{s.name}</span>
                <span className="ref-meta">{s.by}</span>
                <span className="ref-use">{s.use[locale]}</span>
              </a>
            ))}
          </div>

          <h3 className="ref-sub">{c.refPapers}</h3>
          <div className="ref-table">
            {PAPERS.map((p) => (
              <a key={p.name} className="ref-row" href={p.url} target="_blank" rel="noreferrer">
                <span className="ref-name">{p.name}</span>
                <span className="ref-meta">{p.meta}</span>
                <span className="ref-use">{p.use[locale]}</span>
              </a>
            ))}
          </div>

          <h3 className="ref-sub">
            {c.refIntegrated} <span className="badge badge-integrated">{c.badgeIntegrated}</span>
          </h3>
          <div className="ref-table">
            {INTEGRATED.map((o) => (
              <div key={o.name} className="ref-row">
                <span className="ref-name">{o.name}</span>
                <span className="ref-meta">{o.license}</span>
                <span className="ref-use">{o.use[locale]}</span>
              </div>
            ))}
          </div>

          <h3 className="ref-sub">
            {c.refInspired} <span className="badge badge-inspired">{c.badgeInspired}</span>
          </h3>
          <div className="inspired-grid">
            {INSPIRED.map((o) => (
              <article key={o.name} className="inspired-card">
                <h4>{o.name}</h4>
                <p>{o.takeaway[locale]}</p>
              </article>
            ))}
          </div>

          <p className="sec-note">
            {c.note15Pre}
            <a href={`https://github.com/${REPO_SLUG}/blob/main/docs/15_开源项目及论文登记簿.md`} target="_blank" rel="noreferrer">
              {c.note15Link}
            </a>
            {c.note15Mid}
          </p>
        </section>

        {/* ---------- 七、源码入口 ---------- */}
        <section className="sec" id="impl-src">
          <h2>{c.srcTitle}</h2>
          <div className="md-body">
            <ul>
              {c.srcItems.map((s) => (
                <li key={s.href}>
                  <a href={`${REPO_URL}/tree/main/${s.href}`} target="_blank" rel="noreferrer">
                    {s.href}
                  </a>
                  {s.desc}
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* ---------- 延伸阅读 ---------- */}
        <section className="sec">
          <SectionTitle eyebrow="Next" title={c.nextTitle} />
          <div className="guide-grid">
            {GUIDES.map((g) => (
              <a key={g.hash} className="guide-card" href={g.hash}>
                <h3>{g.i18n[locale].title}</h3>
                <p>{g.i18n[locale].desc}</p>
                <span className="guide-arrow">→</span>
              </a>
            ))}
          </div>
        </section>
      </div>
    </main>
  )
}
