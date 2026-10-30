import {
  ApiOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  ExperimentOutlined,
  GithubOutlined,
  ReadOutlined,
  RocketOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
} from '@ant-design/icons'
import { Button, Tag } from 'antd'
import { MODULES, REPO_URL, SEEDS, moduleText } from '../content'
import { ChatWorkbenchMock, OntologyConsoleMock } from '../components/mockups'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'
import type { ReactNode } from 'react'

// ---------------------------------------------------------------------------
// 首页（REQ-238）：项目概览。内容源=README / AGENTS 项目定位 / platform-knowledge/
// 01_整体设计/17_产品_信息架构与界面设计（摘编口径，深入阅读走模块页，避免双源）。
// 新人导览：Hero 后紧跟产品预览与"30 秒了解"五步导览，覆盖 长什么样→是什么→
// 能做什么→如何工作→上手。
// REQ-360：全页文案中英双份（zh 为事实源）。
// REQ-364：新增产品预览区（mockups 界面示意图，不跑平台即可直观了解界面形态）。
// REQ-496 内容口径（2026-10-29）：首页偏向「使用场景」、不出现技术细节——
// 场景卡按 Q-23 用户画像（工程师/教师/合规/研究者/医生）重写，双核心与特性卡
// 改为能力语言（去实现机制与选型名），技术栈条撤除（归「整体架构」页）。
// ---------------------------------------------------------------------------

const TOUR: { href: string; i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    href: '#/module/design',
    i18n: {
      zh: { title: '它是什么', desc: '面向专业个人的工作学习助手——以本体为知识底座：知识可查询、可审计、可出题；自托管部署，数据/密钥/知识不出个人掌控' },
      en: { title: 'What it is', desc: 'A personal work & learning assistant for professionals, grounded on ontologies — knowledge you can query, audit and quiz. Self-hosted — data stays under your control' },
    },
  },
  {
    href: '#/module/agents',
    i18n: {
      zh: { title: '能做什么', desc: '七大功能模块：智能体 / 本体 / 知识库 / 项目 / 技能 / 平台知识 / 设置' },
      en: { title: 'What it does', desc: 'Seven modules: agents / ontology / knowledge base / projects / skills / platform knowledge / settings' },
    },
  },
  {
    href: '#/architecture',
    i18n: {
      zh: { title: '平台如何组成', desc: '整体架构一页看清：平台由哪些服务组成、如何分工协作' },
      en: { title: 'How it is composed', desc: 'One architecture page shows what the platform consists of and how the pieces work together' },
    },
  },
  {
    href: '#/implementation',
    i18n: {
      zh: { title: '如何工作', desc: '五层执行模型与核心机制图，附论文与开源依据' },
      en: { title: 'How it works', desc: 'Five-layer execution model + core mechanism diagrams, with papers and open-source evidence' },
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
        title: '个人 AI 工作台，数据自主可控',
        desc: '桌面安装包或一条命令启动：智能体、知识库、本体全套服务跑在自己的电脑上，专业工作数据不出个人掌控（本地客户端默认，服务端自部署并行）。',
        link: '获取安装包',
      },
      en: {
        title: 'A personal AI workbench that keeps data local',
        desc: 'Boot from a desktop installer or one command: agents, knowledge base and ontology services all run on your own machine — work data never leaves it.',
        link: 'Get the installer',
      },
    },
  },
  {
    icon: <DatabaseOutlined />,
    href: '#/module/ontology',
    i18n: {
      zh: {
        title: '领域知识资产化',
        desc: '把文档、对话与经验沉淀为结构化的领域本体：概念、关系有据可查，版本可追溯，知识不再散落一地。',
        link: '看本体工程',
      },
      en: {
        title: 'Turn domain knowledge into assets',
        desc: 'Distill documents, conversations and experience into structured domain ontologies — concepts and relations you can query, versions you can trace.',
        link: 'Explore ontology engineering',
      },
    },
  },
  {
    icon: <ReadOutlined />,
    href: '#/module/knowledge',
    i18n: {
      zh: {
        title: '资料问答，句句有出处',
        desc: '上传资料即建知识库：即问即答、回答标注原文出处；跨文档的复杂关联问题交给知识图谱多跳检索。',
        link: '看知识库',
      },
      en: {
        title: 'Document Q&A with receipts',
        desc: 'Upload materials and start asking: answers cite their sources, while cross-document links go through multi-hop graph retrieval.',
        link: 'Explore the knowledge base',
      },
    },
  },
  {
    icon: <TeamOutlined />,
    href: '#/module/projects',
    i18n: {
      zh: {
        title: '组一支智能体小队',
        desc: '把任务拆解给多个智能体协作完成：规划、执行、审查各司其职，还可纳入命令行编码智能体，谁在做什么全程可见。',
        link: '看项目协作',
      },
      en: {
        title: 'Assemble an agent team',
        desc: 'Decompose work across collaborating agents — planning, execution and review in their lanes, CLI coding agents welcome — with everything visible as it happens.',
        link: 'Explore project collaboration',
      },
    },
  },
  {
    icon: <ApiOutlined />,
    href: '#/module/agents',
    i18n: {
      zh: {
        title: '让外部工具用上你的智能体',
        desc: '智能体一键开放为 MCP / A2A 标准服务：Claude Desktop、Cursor 等客户端直接调用；也能把外部智能体请进你的协作。',
        link: '看智能体',
      },
      en: {
        title: 'Put your agents in external tools',
        desc: 'Expose agents as MCP / A2A services callable from Claude Desktop, Cursor and more — or bring external agents into your own collaboration.',
        link: 'Explore agents',
      },
    },
  },
  {
    icon: <ExperimentOutlined />,
    href: '#/demo',
    i18n: {
      zh: {
        title: '教学与团队演示',
        desc: '内置学习中心与 5 个示例本体（组织 / 缺陷 / 失效分析 / 基因 / 医学），在线演示免安装即可走完构建到消费全流程。',
        link: '在线演示',
      },
      en: {
        title: 'Teaching & team demos',
        desc: 'A built-in learning center plus 5 example ontologies (orgs / defects / failure analysis / gene / medical) — the online demo walks the full loop with zero install.',
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
          '流式对话与轨迹面板：工具、委派、检索、审批全程留痕，可回放',
          '多智能体协作：项目工作空间编排成员，外部智能体可请入协作',
          '能力增强：挂载知识库、本体与技能，回答有据可查',
          '安全可控：沙箱隔离执行，危险操作分级审批',
          '对外开放：智能体可被 Claude Desktop / Cursor 等外部工具调用（MCP / A2A）',
          '质量可回归：内置评估任务集，行为改动可量化验证',
        ],
      },
      en: {
        title: 'Agent engineering',
        desc: 'Chat workbench × multi-agent collaboration × capability augmentation, fully observable',
        points: [
          'Streaming chat with a trace panel: tools, delegation, retrieval and approvals traced and replayable',
          'Multi-agent collaboration: orchestrate members in project workspaces and welcome external agents',
          'Augmentation: mount knowledge bases, ontologies and skills so answers stay grounded',
          'Safety: isolated sandboxed execution with tiered approvals for dangerous operations',
          'Openness: agents callable from Claude Desktop, Cursor and beyond (MCP / A2A)',
          'Regression-ready quality: built-in evaluation task sets quantify behavior changes',
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
          '多条构建路径：AI 对话 / 向导 / 由知识库构建 / 外部导入 / 权威词表引入',
          '资产治理：质量门禁、版本 diff、演化候选受控生长',
          '专业编辑：公理与关系特性编辑、重命名级联改写，网页端对齐专业本体编辑器',
          '多本体协同：相互引用的本体整体装载与推理，答案标注来源本体',
          '运行与消费：运行方案多引擎可选，智能体经统一工具面消费，决策可溯源',
        ],
      },
      en: {
        title: 'Ontology engineering',
        desc: 'A complete loop: build → assets → run → consume & audit',
        points: [
          'Multiple build paths: AI chat / wizard / from knowledge base / external import / authoritative vocabularies',
          'Asset governance: quality gates, version diff, controlled growth of evolution candidates',
          'Professional editing: axioms and relation characteristics, rename with cascading rewrites — professional-grade, on the web',
          'Multi-ontology synergy: mutually referencing ontologies load and reason as a whole, answers labeled by source',
          'Run & consume: multi-engine run plans, a unified tool facade for agents, decisions traceable',
        ],
      },
    },
  },
]

const FEATURES: { icon: ReactNode; i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    icon: <SearchOutlined />,
    i18n: {
      zh: { title: '知识库检索与图谱', desc: '文档即问即答、句句溯源；知识图谱支撑跨文档多跳关联，语料与本体双向打通' },
      en: { title: 'Knowledge base & graph', desc: 'Ask your documents with sentence-level citations; a knowledge graph links evidence across documents, connected both ways with ontologies' },
    },
  },
  {
    icon: <CloudServerOutlined />,
    i18n: {
      zh: { title: '外部连接器', desc: 'MCP / Kubernetes / SSH / A2A / 命令行 / 本地服务六类连接器，凭据加密托管，按智能体授权' },
      en: { title: 'External connectors', desc: 'Six connector kinds — MCP / Kubernetes / SSH / A2A / CLI / local services — with encrypted credential hosting and per-agent authorization' },
    },
  },
  {
    icon: <SafetyCertificateOutlined />,
    i18n: {
      zh: { title: '沙箱与审批', desc: '智能体副作用隔离执行；危险操作分级审批、全程结构化留痕' },
      en: { title: 'Sandbox & approvals', desc: 'Agent side effects run isolated; dangerous operations take tiered approvals with structured audit trails' },
    },
  },
  {
    icon: <ThunderboltOutlined />,
    i18n: {
      zh: { title: '技能复用', desc: '业界标准技能包封装复用，.zip 批量导入（首批 177 件科学技能包），一卡多技能' },
      en: { title: 'Skill reuse', desc: 'Industry-standard skill packs reused across agents; batch .zip import (177 scientific packs first batch) and one-card collections' },
    },
  },
  {
    icon: <ApiOutlined />,
    i18n: {
      zh: { title: '协议开放（MCP + A2A）', desc: '智能体一键开放为标准服务，被 Claude Desktop / Cursor 等外部工具调用；外部智能体也能请入协作' },
      en: { title: 'Open protocols (MCP + A2A)', desc: 'Expose any agent as a standard service callable from Claude Desktop / Cursor and beyond; welcome external agents into collaboration, too' },
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
      zh: { title: '单机可部署', desc: '一条命令或桌面安装包即可启动，数据不出个人掌控；Docker Compose / Helm 交付可选' },
      en: { title: 'Self-hostable', desc: 'One command or a desktop installer to boot; data never leaves your machine, with optional Docker Compose / Helm delivery' },
    },
  },
]

const COPY = {
  zh: {
    heroTag: '开源 · Apache License 2.0 · 单机可部署',
    heroSub: '智能体构建平台',
    deploy: '部署运行 ↓',
    demo: '在线演示 →',
    ghRepo: 'GitHub 仓库',
    previewTitle: '产品预览',
    previewDesc: '两张界面示意图带你看清核心工作面：智能体对话工作台与本体工程控制台。以下为结构示意（按真实界面布局绘制），部署后即可得到同款界面。',
    tourTitle: '30 秒了解 OntoAgents',
    scenesTitle: '使用场景',
    coresTitle: '双核心',
    modulesTitle: '功能模块',
    modulesMore: '查看详细介绍 →',
    featuresTitle: '平台特性',
    quickstartTitle: '快速开始',
    qs1Title: '一条命令启动（开发/演示）',
    qs1Code: `git clone https://github.com/<owner>/multiagent-lab.git
cd multiagent-lab
./run-dev.sh
# 打开 http://localhost:8080（前端 :5173 由反代同源承载）`,
    qs1NotePre: '仓库地址见本页右上角「GitHub」链接；零 Python venv 依赖；首次启动自动建库并播种示例本体与学习中心内容包。',
    qs2Title: 'Docker Compose / Helm',
    qs2Code: `cd deploy
docker compose up -d
# 或 K8s：helm install ontoagents deploy/helm/ontoagents`,
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
    tourTitle: 'OntoAgents in 30 seconds',
    scenesTitle: 'Use cases',
    coresTitle: 'Dual cores',
    modulesTitle: 'Modules',
    modulesMore: 'Explore in detail →',
    featuresTitle: 'Platform features',
    quickstartTitle: 'Quick start',
    qs1Title: 'One command to start (dev / demo)',
    qs1Code: `git clone https://github.com/<owner>/multiagent-lab.git
cd multiagent-lab
./run-dev.sh
# Open http://localhost:8080 (frontend :5173 served same-origin behind the proxy)`,
    qs1NotePre: 'Find the repo URL in the "GitHub" link at the top-right; zero Python venv dependency; first start creates the database and seeds the example ontologies and learning-center content pack automatically.',
    qs2Title: 'Docker Compose / Helm',
    qs2Code: `cd deploy
docker compose up -d
# Or K8s: helm install ontoagents deploy/helm/ontoagents`,
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
            OntoAgents
            <span className="hero-sub">{c.heroSub}</span>
          </h1>
          <p className="hero-desc">
            {locale === 'zh' ? (
              <>
                面向专业个人的<strong>工作学习助手</strong>，以本体为知识底座——知识不是一堆文档，
                而是可查询、可审计、可出题的结构化资产。<strong>自托管部署</strong>（本地客户端默认、服务端并行），专业工作数据不出个人掌控；
                <strong>智能体工程</strong>（对话 / 多智能体协作 / 知识·技能·本体增强）与
                <strong>本体工程</strong>（构建 → 资产 → 运行 → 消费审计闭环）双核心，能力边界诚实标注。
              </>
            ) : (
              <>
                A <strong>personal work &amp; learning assistant for professionals</strong>, grounded on ontologies — knowledge as a
                structured asset you can query, audit and quiz, not a pile of documents. <strong>Self-hosted</strong> (local client by default, server deployment in parallel) —
                work data stays under your control. Dual cores: <strong>agent engineering</strong> (chat / multi-agent collaboration /
                knowledge·skill·ontology augmentation) and <strong>ontology engineering</strong> (build → assets → run → consume &amp;
                audit), with capability boundaries honestly labeled.
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
