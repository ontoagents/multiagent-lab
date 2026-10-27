import {
  ApartmentOutlined,
  ApiOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  ExperimentOutlined,
  GithubOutlined,
  ReadOutlined,
  RocketOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { Button, Tag } from 'antd'
import { MODULES, REPO_URL, SEEDS, moduleText } from '../content'
import { ChatWorkbenchMock, OntologyConsoleMock } from '../components/mockups'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'
import type { ReactNode } from 'react'

// ---------------------------------------------------------------------------
// 首页（REQ-238）：项目概览。内容源=README / AGENTS 项目定位 / platform-knowledge/
// 01_整体设计/00_整体设计.md（摘编口径，深入阅读走模块页，避免双源）。
// 新人导览：Hero 后紧跟产品预览与"30 秒了解"五步导览，覆盖 长什么样→是什么→
// 能做什么→如何工作→上手。
// REQ-360：全页文案中英双份（zh 为事实源）。
// REQ-364：新增产品预览区（mockups 界面示意图，不跑平台即可直观了解界面形态）；
// "如何工作"导览随关键实现/实现原理合并改指 #/implementation。
// ---------------------------------------------------------------------------

const TOUR: { href: string; i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    href: '#/module/design',
    i18n: {
      zh: { title: '它是什么', desc: '单机可部署的智能体 + 本体双核平台，Go + Eino/ADK，数据全留本机' },
      en: { title: 'What it is', desc: 'A self-hostable dual-core platform for agents + ontologies, on Go + Eino/ADK — data stays local' },
    },
  },
  {
    href: '#/module/agents',
    i18n: {
      zh: { title: '能做什么', desc: '六大模块：智能体 / 本体 / 知识库 / 连接器 / 消费审计 / 学习中心' },
      en: { title: 'What it does', desc: 'Six modules: agents / ontology / knowledge base / connectors / consumption & audit / learning center' },
    },
  },
  {
    href: '#/architecture',
    i18n: {
      zh: { title: '系统长什么样', desc: '三平面 + 双代理拓扑，一张总图看清服务边界与端口' },
      en: { title: 'What it looks like', desc: 'Three planes + dual-proxy topology; one diagram shows service boundaries and ports' },
    },
  },
  {
    href: '#/implementation',
    i18n: {
      zh: { title: '如何工作', desc: '五层执行模型 + 质量门禁机制，附论文与开源依据' },
      en: { title: 'How it works', desc: 'Five-layer execution model + quality gates, with papers and open-source evidence' },
    },
  },
  {
    href: '#/deploy',
    i18n: {
      zh: { title: '立即上手', desc: '安装包 / 服务器 / 本地三种运行方式任选' },
      en: { title: 'Get started', desc: 'Run it from an installer, a server, or locally — pick your way' },
    },
  },
]

const SCENES: { icon: ReactNode; href: string; i18n: L10n<{ title: string; desc: string; link: string }> }[] = [
  {
    icon: <DesktopOutlined />,
    href: '#/deploy',
    i18n: {
      zh: {
        title: '个人 AI 工作台自托管',
        desc: '下载桌面安装包开箱即用：智能体、本体与知识库全套服务内置托管，SQLite 单文件存储，数据完全留在本机。',
        link: '获取安装包',
      },
      en: {
        title: 'Self-hosted personal AI workbench',
        desc: 'Download the desktop installer and go: agents, ontology and knowledge-base services all hosted locally, single-file SQLite storage — data never leaves your machine.',
        link: 'Get the installer',
      },
    },
  },
  {
    icon: <DatabaseOutlined />,
    href: '#/module/ontology',
    i18n: {
      zh: {
        title: '领域知识工程',
        desc: '六条构建路径把文档、对话与经验沉淀为可治理的本体资产：质量门禁、版本 diff、进化候选受控生长。',
        link: '看本体工程',
      },
      en: {
        title: 'Domain knowledge engineering',
        desc: 'Six build paths turn documents, conversations and experience into governable ontology assets: quality gates, version diff, controlled growth of evolution candidates.',
        link: 'Explore ontology engineering',
      },
    },
  },
  {
    icon: <ExperimentOutlined />,
    href: '#/implementation',
    i18n: {
      zh: {
        title: '智能体工程实验田',
        desc: '配置驱动装配、SSE 全事件流可观测、质量门禁与评估任务集——把 idea 推进为可复现的智能体方案。',
        link: '看实现与原理',
      },
      en: {
        title: 'Agent engineering playground',
        desc: 'Config-driven assembly, fully observable SSE event streams, quality gates and eval task sets — push an idea into a reproducible agent design.',
        link: 'See implementation & principles',
      },
    },
  },
  {
    icon: <ReadOutlined />,
    href: '#/demo',
    i18n: {
      zh: {
        title: '教学与团队演示',
        desc: '内置学习中心与 5 个示例本体（组织 / 缺陷 / 失效分析 / 基因 / 医学），在线 demo 免安装即可演示完整闭环。',
        link: '在线演示',
      },
      en: {
        title: 'Teaching & team demos',
        desc: 'Built-in learning center and 5 example ontologies (orgs / defects / failure analysis / gene / medical) — the online demo shows the full loop with zero install.',
        link: 'Online demo',
      },
    },
  },
]

const CORES: { i18n: L10n<{ title: string; desc: string; points: string[] }> }[] = [
  {
    i18n: {
      zh: {
        title: '智能体工程',
        desc: '对话工作台 × 多智能体协作 × 能力增强，过程完全可观测',
        points: [
          '流式对话与调用轨迹面板（工具 / 子智能体 / 检索 / 审批事件全留痕）',
          '多智能体协作：项目工作空间、成员编排、委派与切换事件可见',
          '协议枢纽双向开放：智能体暴露为 MCP / A2A 服务，CLI 与 A2A 外部智能体接入协作',
          '上下文工程 / Harness 沙箱与审批 / Loop 持久化与续跑（五层视角）',
          '评估驱动质量：agenteval 种子任务集 + LLM Judge 门控',
        ],
      },
      en: {
        title: 'Agent engineering',
        desc: 'Chat workbench × multi-agent collaboration × capability augmentation, fully observable',
        points: [
          'Streaming chat with a call-trace panel (tool / sub-agent / retrieval / approval events fully traced)',
          'Multi-agent collaboration: project workspace, member orchestration, visible delegation and transfer events',
          'Protocol hub open both ways: agents serve MCP / A2A outward; CLI and A2A external agents join collaboration inward',
          'Context engineering / harness sandbox & approvals / loop persistence & resume (five-layer view)',
          'Evaluation-driven quality: agenteval seed task sets + LLM judge gating',
        ],
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '本体工程',
        desc: '构建 → 资产 → 运行 → 消费审计 的完整闭环',
        points: [
          '六条构建路径：手工向导 / AI 对话 / 由知识库构建 / OO 双轨 / OntoExtend 等',
          '资产治理：质量门禁与质量卡、版本与 diff、进化候选受控生长、被引用聚合',
          '多引擎运行平面：Oxigraph / Fuseki，MCP facade（onto_* 工具）供智能体消费',
          '消费与审计观测台：KG 检索来源徽标、决策溯源、PROV-O 导出',
        ],
      },
      en: {
        title: 'Ontology engineering',
        desc: 'A complete loop: build → assets → run → consume & audit',
        points: [
          'Six build paths: manual wizard / AI chat / from knowledge base / OO dual-track / OntoExtend and more',
          'Asset governance: quality gate & quality card, versions and diff, controlled growth of evolution candidates, reference aggregation',
          'Multi-engine runtime plane: Oxigraph / Fuseki, with an MCP facade (onto_* tools) for agent consumption',
          'Consumption & audit observatory: KG retrieval source badges, decision provenance, PROV-O export',
        ],
      },
    },
  },
]

const FEATURES: { icon: ReactNode; i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    icon: <ApartmentOutlined />,
    i18n: {
      zh: { title: '知识库 RAG / GraphRAG', desc: '向量检索 + 自研 KG 抽取（LLM 主路径 + 规则回退），本体 TTL 直接装载为 KG 源，与本体双向打通' },
      en: { title: 'Knowledge base RAG / GraphRAG', desc: 'Vector retrieval + built-in KG extraction (LLM primary path with rule fallback); ontology TTLs load directly as KG sources' },
    },
  },
  {
    icon: <CloudServerOutlined />,
    i18n: {
      zh: { title: '外部连接器', desc: 'Kubernetes / SSH / 自定义 MCP / A2A / CLI / stdio 六类连接器，凭据加密托管，agent 级授权白名单' },
      en: { title: 'External connectors', desc: 'Kubernetes / SSH / custom MCP / A2A / CLI / stdio — six connector kinds with encrypted credential hosting and agent-level authorization whitelists' },
    },
  },
  {
    icon: <SafetyCertificateOutlined />,
    i18n: {
      zh: { title: '沙箱执行与审批', desc: 'docker / k8s 执行后端，danger 级工具三档审批、挂起超时与结构化审计事件' },
      en: { title: 'Sandbox execution & approvals', desc: 'docker / k8s execution backends; three-tier approval for dangerous tools, suspension timeouts and structured audit events' },
    },
  },
  {
    icon: <ThunderboltOutlined />,
    i18n: {
      zh: { title: '技能复用', desc: 'SKILL.md 标准技能包封装复用，.zip 批量导入（首批 177 件），集合一卡多子技能挂载' },
      en: { title: 'Skill reuse', desc: 'SKILL.md standard packs, packaged and reused; batch .zip import (177 first batch) and one-card collections' },
    },
  },
  {
    icon: <ApiOutlined />,
    i18n: {
      zh: { title: '协议开放（MCP + A2A）', desc: '智能体一键暴露为 MCP Server（/mcp）与 A2A 端点，被其他 Agent 调用；外部智能体（CLI 编码智能体 / A2A 远程智能体 / stdio MCP server）经连接器接入协作' },
      en: { title: 'Open protocols (MCP + A2A)', desc: 'Expose any agent as an MCP Server (/mcp) or A2A endpoint for other agents to call; external agents (CLI coding agents / A2A remote agents / stdio MCP servers) plug in via connectors' },
    },
  },
  {
    icon: <RobotOutlined />,
    i18n: {
      zh: { title: '平台助手', desc: '内置 AI 助手：平台答疑 / 知识检索 / 配置提案（人工确认后落库），随开随用' },
      en: { title: 'Platform assistant', desc: 'A built-in AI assistant for platform Q&A / knowledge retrieval / config proposals (applied only after human confirmation)' },
    },
  },
  {
    icon: <RocketOutlined />,
    i18n: {
      zh: { title: '单机可部署', desc: 'run-dev.sh 一条命令启动全栈（零 venv）；Docker Compose / Helm 交付；SQLite 单文件存储' },
      en: { title: 'Self-hostable', desc: 'One command (run-dev.sh) boots the full stack with zero venv; Docker Compose / Helm delivery; single-file SQLite storage' },
    },
  },
]

const TECH = ['Go', 'CloudWeGo Eino / ADK', 'Hertz', 'SQLite', 'SSE', 'React 18', 'AntD 6', 'Vite', 'three.js', '@xyflow/react']

const COPY = {
  zh: {
    heroTag: '开源 · Apache License 2.0 · 单机可部署',
    heroSub: '智能体构建平台',
    deploy: '部署运行 ↓',
    demo: '在线演示 →',
    ghRepo: 'GitHub 仓库',
    previewTitle: '产品预览',
    previewDesc: '两张界面示意图带你看清核心工作面：智能体对话工作台与本体工程控制台。以下为结构示意（按真实界面布局绘制），部署后即可得到同款界面。',
    tourTitle: '30 秒了解 AgentLab',
    scenesTitle: '使用场景',
    coresTitle: '双核心',
    modulesTitle: '功能模块',
    modulesMore: '查看详细介绍 →',
    featuresTitle: '平台特性',
    techTitle: '技术栈',
    quickstartTitle: '快速开始',
    qs1Title: '一条命令启动（开发/演示）',
    qs1Code: `git clone https://github.com/<owner>/multiagent-lab.git
cd multiagent-lab
./run-dev.sh
# 打开 http://localhost:8080（前端 :5173 由反代同源承载）`,
    qs1NotePre: '仓库地址见本页右上角「GitHub」链接；零 Python venv 依赖；首次启动自动建库（SQLite）并播种示例本体与学习中心内容包。',
    qs2Title: 'Docker Compose / Helm',
    qs2Code: `cd deploy
docker compose up -d
# 或 K8s：helm install agentlab deploy/helm`,
    qs2NotePre: '部署形态与远程访问凭证见仓库 ',
    qs2NoteLink: '《部署与运行》',
    qs2NotePost: '。',
    learnHintPre: '首次使用建议从内置',
    learnHintStrong: '学习中心',
    learnHintMid: '开始：七阶段学习路径 + 方法论卡 + 任务卡打卡，配套 ',
    learnHintPost: ' 个示例本体（',
    learnHintClose: '）。',
  },
  en: {
    heroTag: 'Open source · Apache License 2.0 · Self-hostable',
    heroSub: 'Agent Construction Platform',
    deploy: 'Deploy & run ↓',
    demo: 'Online demo →',
    ghRepo: 'GitHub repo',
    previewTitle: 'Product preview',
    previewDesc: 'Two interface mockups show the core workspaces: the agent chat workbench and the ontology engineering console. These are structural mockups drawn from the real UI — deploy the platform and you get the same thing.',
    tourTitle: 'AgentLab in 30 seconds',
    scenesTitle: 'Use cases',
    coresTitle: 'Dual cores',
    modulesTitle: 'Modules',
    modulesMore: 'Explore in detail →',
    featuresTitle: 'Platform features',
    techTitle: 'Tech stack',
    quickstartTitle: 'Quick start',
    qs1Title: 'One command to start (dev / demo)',
    qs1Code: `git clone https://github.com/<owner>/multiagent-lab.git
cd multiagent-lab
./run-dev.sh
# Open http://localhost:8080 (frontend :5173 served same-origin behind the proxy)`,
    qs1NotePre: 'Find the repo URL in the "GitHub" link at the top-right; zero Python venv dependency; first start creates the database (SQLite) and seeds the example ontologies and learning-center content pack automatically.',
    qs2Title: 'Docker Compose / Helm',
    qs2Code: `cd deploy
docker compose up -d
# Or K8s: helm install agentlab deploy/helm`,
    qs2NotePre: 'Deployment forms and remote-access credentials: ',
    qs2NoteLink: 'Deploy & Run',
    qs2NotePost: '.',
    learnHintPre: 'First time? Start with the built-in ',
    learnHintStrong: 'learning center',
    learnHintMid: ': a seven-stage learning path + methodology cards + task check-ins, plus ',
    learnHintPost: ' example ontologies (',
    learnHintClose: ').',
  },
}

export default function HomePage() {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  return (
    <main>
      {/* Hero */}
      <section className="hero">
        <div className="page-inner">
          <Tag color="geekblue" style={{ marginBottom: 14 }}>
            {c.heroTag}
          </Tag>
          <h1>
            AgentLab
            <span className="hero-sub">{c.heroSub}</span>
          </h1>
          <p className="hero-desc">
            {locale === 'zh' ? (
              <>
                基于 Go + CloudWeGo Eino/ADK 的智能体构建平台，<strong>单机可部署</strong>。
                <strong>智能体工程</strong>（对话 / 多智能体协作 / 知识·技能·本体增强）与
                <strong>本体工程</strong>（构建 → 资产 → 运行 → 消费审计闭环）双核心，
                内置学习中心与示例本体，能力边界诚实标注。
              </>
            ) : (
              <>
                An agent construction platform on Go + CloudWeGo Eino/ADK, <strong>self-hostable on a single machine</strong>.
                Dual cores: <strong>agent engineering</strong> (chat / multi-agent collaboration / knowledge·skill·ontology augmentation) and{' '}
                <strong>ontology engineering</strong> (build → assets → run → consume & audit), with a built-in learning center and example
                ontologies — capability boundaries honestly labeled.
              </>
            )}
          </p>
          <div className="hero-actions">
            <Button type="primary" size="large" href="#/deploy">
              {c.deploy}
            </Button>
            <Button size="large" href="#/demo">
              {c.demo}
            </Button>
            <Button size="large" icon={<GithubOutlined />} href={REPO_URL} target="_blank" rel="noreferrer">
              {c.ghRepo}
            </Button>
          </div>
        </div>
      </section>

      <div className="page-inner">
        {/* 产品预览（REQ-364：界面示意图，直观了解界面形态） */}
        <section className="sec">
          <h2>{c.previewTitle}</h2>
          <p className="preview-desc">{c.previewDesc}</p>
          <div className="preview-grid">
            <ChatWorkbenchMock />
            <OntologyConsoleMock />
          </div>
        </section>

        {/* 30 秒了解（新人导览） */}
        <section className="sec">
          <h2>{c.tourTitle}</h2>
          <div className="tour-strip">
            {TOUR.map((t, i) => (
              <a className="tour-card" key={t.href} href={t.href}>
                <span className="tour-num">{i + 1}</span>
                <span className="tour-title">{t.i18n[locale].title}</span>
                <span className="tour-desc">{t.i18n[locale].desc}</span>
              </a>
            ))}
          </div>
        </section>

        {/* 使用场景 */}
        <section className="sec">
          <h2>{c.scenesTitle}</h2>
          <div className="scene-grid">
            {SCENES.map((s) => (
              <div className="scene-card" key={s.href}>
                <span className="scene-icon">{s.icon}</span>
                <div className="scene-title">{s.i18n[locale].title}</div>
                <p className="scene-desc">{s.i18n[locale].desc}</p>
                <a className="scene-link" href={s.href}>
                  {s.i18n[locale].link} →
                </a>
              </div>
            ))}
          </div>
        </section>

        {/* 双核心 */}
        <section className="sec">
          <h2>{c.coresTitle}</h2>
          <div className="core-grid">
            {CORES.map((core, ci) => {
              const cc = core.i18n[locale]
              return (
                <div className="core-card" key={ci}>
                  <h3>{cc.title}</h3>
                  <p className="core-desc">{cc.desc}</p>
                  <ul>
                    {cc.points.map((p) => (
                      <li key={p}>{p}</li>
                    ))}
                  </ul>
                </div>
              )
            })}
          </div>
        </section>

        {/* 功能模块 */}
        <section className="sec">
          <h2>
            {c.modulesTitle} <a className="sec-more" href="#/module/agents">{c.modulesMore}</a>
          </h2>
          <div className="mod-grid">
            {MODULES.map((m) => {
              const mt = moduleText(m, locale)
              return (
                <a className="mod-card" key={m.key} href={`#/module/${m.key}`}>
                  <span className="mod-title">{mt.title}</span>
                  <span className="mod-tagline">{mt.tagline}</span>
                </a>
              )
            })}
          </div>
        </section>

        {/* 特性 */}
        <section className="sec">
          <h2>{c.featuresTitle}</h2>
          <div className="feat-grid">
            {FEATURES.map((f, fi) => (
              <div className="feat-card" key={fi}>
                <span className="feat-icon">{f.icon}</span>
                <div>
                  <div className="feat-title">{f.i18n[locale].title}</div>
                  <div className="feat-desc">{f.i18n[locale].desc}</div>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* 技术栈 */}
        <section className="sec">
          <h2>{c.techTitle}</h2>
          <div className="tech-strip">
            {TECH.map((t) => (
              <Tag key={t} style={{ margin: 0, padding: '4px 12px', fontSize: 13 }}>
                {t}
              </Tag>
            ))}
          </div>
        </section>

        {/* 快速开始 */}
        <section className="sec" id="quickstart">
          <h2>{c.quickstartTitle}</h2>
          <div className="qs-grid">
            <div className="qs-card">
              <div className="qs-title">{c.qs1Title}</div>
              <pre className="qs-code">{c.qs1Code}</pre>
              <p className="qs-note">{c.qs1NotePre}</p>
            </div>
            <div className="qs-card">
              <div className="qs-title">{c.qs2Title}</div>
              <pre className="qs-code">{c.qs2Code}</pre>
              <p className="qs-note">
                {c.qs2NotePre}
                <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`} target="_blank" rel="noreferrer">
                  {c.qs2NoteLink}
                </a>
                {c.qs2NotePost}
              </p>
            </div>
          </div>
          <p className="learn-hint">
            {c.learnHintPre}
            <strong>{c.learnHintStrong}</strong>
            {c.learnHintMid}
            {SEEDS.length}
            {c.learnHintPost}
            {SEEDS.map((s) => s.spec.name).join(' / ')}
            {c.learnHintClose}
          </p>
        </section>
      </div>
    </main>
  )
}
