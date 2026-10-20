// ---------------------------------------------------------------------------
// 官网内容单源（REQ-238）：演示数据 = ontology-service/internal/seed/examples/
// 种子 spec（后端启动播种同一份）。
// REQ-364 模块页去文档化：模块介绍不再内联 platform-knowledge 导读档全文
// （原正文大量 md 互引渲染为 GitHub 文档链接，阅读不直观），改为站点层
// 结构化数据——tagline 一句话定位 + brief 速览要点 + features 图标功能卡
// （icon + title + desc），全部中英双语（REQ-360：zh 为事实源）。
// 功能卡事实口径 = platform-knowledge/ 各模块 00 导读页与 docs/02/03/04/11/12，
// 仅做直观化摘编；深入阅读仍可走仓库 docs（官网不再承载文档链接）。
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
}

/** 图标功能卡（REQ-364）：icon 为 @ant-design/icons 组件名，映射表见 ModulePage */
export interface ModuleFeature {
  icon: string
  i18n: L10n<{ title: string; desc: string }>
}

export interface ModuleInfo {
  key: string
  i18n: L10n<ModuleText>
  features: ModuleFeature[]
}

/** 功能模块（目录序=导航序；整体设计放最后，作为深入了解入口） */
export const MODULES: ModuleInfo[] = [
  {
    key: 'agents',
    i18n: {
      zh: {
        title: '智能体',
        tagline: '对话 · 多智能体协作 · 知识/技能/本体增强',
        brief: ['流式对话，工具/子智能体/检索/审批事件全留痕', '多智能体协作：项目空间编排、委派路由、隔离可观测', '五层执行模型：模型/上下文/沙箱/循环/协作全可配'],
      },
      en: {
        title: 'Agents',
        tagline: 'Chat · Multi-agent collaboration · Knowledge/skill/ontology augmentation',
        brief: ['Streaming chat with full traces of tool / sub-agent / retrieval / approval events', 'Multi-agent collaboration: project-space orchestration, delegation routing, isolated observability', 'Five-layer execution model: model / context / harness / loop / graph all configurable'],
      },
    },
    features: [
      {
        icon: 'CommentOutlined',
        i18n: {
          zh: { title: '对话工作台', desc: '流式对话 + 事件轨迹面板：工具调用、深度思考、子智能体委派、知识召回、审批请求全程留痕，可展开、可回放' },
          en: { title: 'Chat workbench', desc: 'Streaming chat with a trace panel: tool calls, deep thinking, sub-agent delegation, retrieval and approvals fully traced — expandable and replayable' },
        },
      },
      {
        icon: 'TeamOutlined',
        i18n: {
          zh: { title: '多智能体协作', desc: '项目空间内编排成员智能体，agent_as_tool 按能力描述路由委派；嵌套调用轨迹可见，子智能体上下文隔离' },
          en: { title: 'Multi-agent collaboration', desc: 'Orchestrate member agents in a project space; agent_as_tool routes delegation by capability; nested traces visible, sub-agent contexts isolated' },
        },
      },
      {
        icon: 'ControlOutlined',
        i18n: {
          zh: { title: '上下文工程', desc: '三档上下文预算（compact / standard / full），超限先前缀摘要再压缩持久化；工具结果自动剪枝，每轮喂给模型什么完全可控' },
          en: { title: 'Context engineering', desc: 'Three context budget tiers (compact / standard / full); over budget, prefix-summary then persisted compression; tool results auto-pruned — full control over what the model sees' },
        },
      },
      {
        icon: 'SafetyOutlined',
        i18n: {
          zh: { title: '沙箱与审批', desc: 'docker / k8s 执行后端隔离副作用；危险操作三档审批 + 豁免清单，HookChain 拦截 SSRF 与危险命令' },
          en: { title: 'Sandbox & approvals', desc: 'docker / k8s execution backends isolate side effects; three-tier approvals + exemption lists for dangerous operations; HookChain blocks SSRF and risky commands' },
        },
      },
      {
        icon: 'SyncOutlined',
        i18n: {
          zh: { title: '断点续跑', desc: 'checkpoint 快照落库，进程重启后挂起运行可恢复；定时续跑让长期任务不依赖进程常驻' },
          en: { title: 'Resume & persistence', desc: 'Checkpoint snapshots persist to the database; suspended runs survive restarts, and scheduled resume keeps long tasks alive without a resident process' },
        },
      },
      {
        icon: 'FileDoneOutlined',
        i18n: {
          zh: { title: '评估驱动质量', desc: 'agenteval 种子任务集 + LLM Judge 门控：智能体行为改动可回归验证' },
          en: { title: 'Eval-driven quality', desc: 'agenteval seed task sets + LLM judge gating: behavior changes are regression-verifiable' },
        },
      },
    ],
  },
  {
    key: 'ontology',
    i18n: {
      zh: {
        title: '本体',
        tagline: '构建 → 资产 → 运行 → 消费审计 全流程闭环',
        brief: ['六条构建路径殊途同归，LLM 生成必过质量门禁', 'spec_json 单一资产：版本 diff、进化候选受控生长', 'Oxigraph / Fuseki 运行平面，MCP facade 供智能体消费'],
      },
      en: {
        title: 'Ontology',
        tagline: 'Build → Assets → Run → Consume & audit, a closed loop',
        brief: ['Six build paths converge on one asset store; every LLM generation passes the quality gate', 'spec_json as the single asset: version diff, controlled growth of evolution candidates', 'Oxigraph / Fuseki runtime plane with an MCP facade for agent consumption'],
      },
    },
    features: [
      {
        icon: 'ApartmentOutlined',
        i18n: {
          zh: { title: '六条构建路径', desc: '手工向导 / AI 对话 / 由知识库构建 / 导入外部本体 / OntoExtend 扩展 / 权威库引入——入口不同，出口同一套校验与资产能力' },
          en: { title: 'Six build paths', desc: 'Manual wizard / AI chat / from knowledge base / external import / OntoExtend evolution / authoritative library — different entries, identical validation and asset capabilities' },
        },
      },
      {
        icon: 'SafetyCertificateOutlined',
        i18n: {
          zh: { title: '质量门禁', desc: '11 检查项 + 三维评分，LLM 生成必过门禁并支持自动修复（≤3 轮）；低质量产物进不了资产库' },
          en: { title: 'Quality gate', desc: '11 checks + three-dimension scoring; every LLM generation must pass, with auto-repair (≤3 rounds) — low-quality artifacts never enter the store' },
        },
      },
      {
        icon: 'DatabaseOutlined',
        i18n: {
          zh: { title: '资产治理', desc: 'spec_json 单一资产：版本快照与 diff、fork 溯源、进化候选受控生长；TTL / VOWL / CSV 都是导出视图' },
          en: { title: 'Asset governance', desc: 'spec_json as the single asset: version snapshots & diff, fork provenance, controlled growth of evolution candidates; TTL / VOWL / CSV are export views' },
        },
      },
      {
        icon: 'CloudServerOutlined',
        i18n: {
          zh: { title: '多引擎运行', desc: 'Oxigraph（SPARQL 1.1）与 Fuseki（OWL 推理）按方案编排启停，逐本体图隔离装载，启动对账防陈旧' },
          en: { title: 'Multi-engine runtime', desc: 'Oxigraph (SPARQL 1.1) and Fuseki (OWL reasoning) start/stop per plan; per-ontology graph isolation and startup reconciliation keep loads fresh' },
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
          zh: { title: '消费与审计', desc: 'MCP facade 六个只读工具供智能体消费；KG 检索来源徽标、决策审计、PROV-O 溯源' },
          en: { title: 'Consume & audit', desc: 'Six read-only MCP facade tools for agent consumption; KG retrieval source badges, decision audit, PROV-O provenance' },
        },
      },
    ],
  },
  {
    key: 'knowledge',
    i18n: {
      zh: {
        title: '知识库',
        tagline: 'RAG / GraphRAG 双子模块（KG 自研内置）',
        brief: ['RAG：向量检索，Qdrant 主力 / SQLite 兜底可插拔', 'GraphRAG：导入即建 KG，向量命中 → 图一跳扩展', 'LLM Wiki：写时合成互链页面，零 embedding 依赖'],
      },
      en: {
        title: 'Knowledge Base',
        tagline: 'RAG / GraphRAG twin sub-modules (built-in KG)',
        brief: ['RAG: vector retrieval with pluggable Qdrant primary / SQLite fallback', 'GraphRAG: KG built on import; vector hit → one-hop graph expansion', 'LLM Wiki: write-time synthesis of interlinked pages, zero embedding dependency'],
      },
    },
    features: [
      {
        icon: 'SearchOutlined',
        i18n: {
          zh: { title: 'RAG 检索', desc: '切分 → 向量化 → TopK 注入；Qdrant 主力、SQLite 余弦兜底，向量后端可插拔，单机部署零外部依赖' },
          en: { title: 'RAG retrieval', desc: 'Chunk → embed → TopK injection; Qdrant primary with SQLite cosine fallback, pluggable vector backends — zero external dependency' },
        },
      },
      {
        icon: 'NodeIndexOutlined',
        i18n: {
          zh: { title: 'GraphRAG', desc: '导入即由 LLM 抽取知识图谱（失败回退规则抽取，不阻断导入）；检索时向量命中 → 图一跳扩展，多跳问题显著优于纯向量' },
          en: { title: 'GraphRAG', desc: 'LLM extracts a KG on import (rule fallback, never blocks); retrieval does vector hit → one-hop graph expansion — clearly better on multi-hop questions' },
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
          zh: { title: '检索可观测', desc: 'retrieval 事件全程打点：命中了哪些片段、哪些三元组、来自哪份文档，对话中一目了然' },
          en: { title: 'Observable retrieval', desc: 'retrieval events mark sources end to end: which chunks, which triples, from which document — visible right in the conversation' },
        },
      },
    ],
  },
  {
    key: 'projects',
    i18n: {
      zh: {
        title: '项目',
        tagline: '多智能体协作工作空间（目录绑定 / 文件 / Git）',
        brief: ['多智能体协作工作空间，成员编排与委派', '本地目录绑定，文件与 Git 资产纳管', '协作与工作流配置收敛于项目侧'],
      },
      en: {
        title: 'Projects',
        tagline: 'Multi-agent collaboration workspace (directory binding / files / Git)',
        brief: ['A collaboration workspace for multiple agents with member orchestration and delegation', 'Local directory binding; files and Git assets under management', 'Collaboration and workflow configuration converge on the project side'],
      },
    },
    features: [
      {
        icon: 'TeamOutlined',
        i18n: {
          zh: { title: '协作工作空间', desc: '项目内编排成员智能体：谁参与、怎么委派、如何交接，配置收敛于项目侧' },
          en: { title: 'Collaboration workspace', desc: 'Orchestrate member agents per project: who joins, how work is delegated and handed over — all configured on the project side' },
        },
      },
      {
        icon: 'FolderOutlined',
        i18n: {
          zh: { title: '目录绑定与文件纳管', desc: '绑定本地目录后，文件资产在项目内统一浏览、读写与审计' },
          en: { title: 'Directory binding & files', desc: 'Bind a local directory and browse, read/write and audit file assets within the project' },
        },
      },
      {
        icon: 'BranchOutlined',
        i18n: {
          zh: { title: 'Git 资产管理', desc: '项目文件变更纳入 Git 管理，改动可追溯、可回滚' },
          en: { title: 'Git asset management', desc: 'Project file changes are managed under Git — traceable and revertible' },
        },
      },
      {
        icon: 'DeploymentUnitOutlined',
        i18n: {
          zh: { title: '协作拓扑可视化', desc: '成员智能体的组网与委派关系一图看清' },
          en: { title: 'Collaboration topology', desc: 'See member-agent networking and delegation relationships in one diagram' },
        },
      },
    ],
  },
  {
    key: 'skills',
    i18n: {
      zh: {
        title: '技能',
        tagline: '智能体能力扩展（提示词 / 工具包装）',
        brief: ['提示词 + 工具包装为可复用技能', '智能体一键暴露为 MCP Server（/mcp）', 'K8s / SSH / 自定义 MCP 三类外部连接器'],
      },
      en: {
        title: 'Skills',
        tagline: 'Agent capability extensions (prompts / tool wrapping)',
        brief: ['Prompts + tool wrapping packaged as reusable skills', 'Expose any agent as an MCP Server (/mcp) in one click', 'Three kinds of external connectors: K8s / SSH / custom MCP'],
      },
    },
    features: [
      {
        icon: 'ThunderboltOutlined',
        i18n: {
          zh: { title: '技能封装', desc: '提示词 + 工具包装为可复用技能，跨智能体共享装配' },
          en: { title: 'Skill packaging', desc: 'Package prompts + tools into reusable skills, shared across agents' },
        },
      },
      {
        icon: 'ApiOutlined',
        i18n: {
          zh: { title: 'MCP 服务化', desc: '任意智能体一键暴露为 MCP Server（/mcp，Streamable HTTP），外部客户端直连调用' },
          en: { title: 'MCP serving', desc: 'Expose any agent as an MCP Server (/mcp, Streamable HTTP) in one click for direct external-client calls' },
        },
      },
      {
        icon: 'ClusterOutlined',
        i18n: {
          zh: { title: '外部连接器', desc: 'Kubernetes / SSH / 自定义 MCP 三类连接器接入外部系统，凭据加密托管' },
          en: { title: 'External connectors', desc: 'Kubernetes / SSH / custom MCP connectors reach external systems, with encrypted credential hosting' },
        },
      },
    ],
  },
  {
    key: 'settings',
    i18n: {
      zh: {
        title: '设置',
        tagline: '模型连接 · 连接器 · 运行环境 · 使用统计',
        brief: ['模型连接双协议多厂商，Key 本地加密', '连接器凭据托管，agent 级授权白名单', '沙箱后端选择与 run 级用量统计'],
      },
      en: {
        title: 'Settings',
        tagline: 'Model connections · Connectors · Runtime · Usage stats',
        brief: ['Dual-protocol multi-vendor model connections with locally encrypted keys', 'Connector credential hosting with agent-level authorization whitelists', 'Sandbox backend selection and per-run usage statistics'],
      },
    },
    features: [
      {
        icon: 'KeyOutlined',
        i18n: {
          zh: { title: '模型连接', desc: 'openai_compat / anthropic 双协议接入多厂商模型，API Key 本地加密存储；协议是连接属性' },
          en: { title: 'Model connections', desc: 'Connect multi-vendor models via openai_compat / anthropic dual protocols; API keys encrypted locally — the protocol is a connection attribute' },
        },
      },
      {
        icon: 'LockOutlined',
        i18n: {
          zh: { title: '凭据托管', desc: '连接器凭据加密托管，授权精确到 agent 级白名单' },
          en: { title: 'Credential hosting', desc: 'Connector credentials encrypted at rest; authorization precise to agent-level whitelists' },
        },
      },
      {
        icon: 'DashboardOutlined',
        i18n: {
          zh: { title: '运行环境与统计', desc: '沙箱后端按需选择；run 级 token 用量与轮次统计可回放' },
          en: { title: 'Runtime & usage stats', desc: 'Pick the sandbox backend per need; per-run token usage and turn counts are replayable' },
        },
      },
    ],
  },
  {
    key: 'design',
    i18n: {
      zh: {
        title: '整体设计',
        tagline: '产品定位 / 架构分层 / 技术栈 / 协议面 / 部署形态',
        brief: ['产品定位与三平面 + 双代理架构', '技术栈总表与 REST+SSE / MCP 协议面', '设计原则四条：开源优先 / 可插拔 / 诚实降级 / 低侵入'],
      },
      en: {
        title: 'Overall Design',
        tagline: 'Product positioning / architecture layers / tech stack / protocols / deployment',
        brief: ['Product positioning; three planes + dual-proxy architecture', 'Tech stack table and the REST+SSE / MCP protocol surface', 'Four design principles: open source first / pluggable / honest degradation / low intrusion'],
      },
    },
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
          zh: { title: '协议面', desc: 'REST + SSE 全事件流对外，MCP 双向：消费本体 facade、供给智能体服务化' },
          en: { title: 'Protocol surface', desc: 'REST + SSE event streams outward; MCP both ways — consuming the ontology facade and serving agents' },
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
