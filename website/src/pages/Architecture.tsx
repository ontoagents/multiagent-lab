import { ArchDiagram, ProtocolHubDiagram } from '../components/diagrams'
import { REPO_URL } from '../content'
import { Tag } from 'antd'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'

// ---------------------------------------------------------------------------
// 整体架构页（REQ-238 扩展）：三平面 + 双代理总图（SVG 内联，源=platform-knowledge/
// 01_整体设计/00_整体设计.md §2/§3 + docs/02 §1/§2/§7 + 16_部署与运行）。
// REQ-360：全页文案中英双份（zh 为事实源）；架构图内文字随语言切换（diagrams.tsx）。
// REQ-496 内容口径（2026-10-29）：本页只讲「整体架构设计」——三平面职责 / 技术栈 /
// 协议面 / 部署形态，不出现模块内部细节（检查项数、工具清单、机制名、REQ 编号、
// 端点路径等归各模块页与「实现与原理」页）。
// ---------------------------------------------------------------------------

const PLANES: { port: string; i18n: L10n<{ name: string; desc: string; points: string[] }> }[] = [
  {
    port: 'Go · Hertz · :8080',
    i18n: {
      zh: {
        name: '主平台 backend',
        desc: '智能体装配与运行的中枢：对话、多智能体协作、知识库、技能与工具、模型接入、平台助手；经反向代理同源嵌入本体两平面。',
        points: ['配置驱动：改配置，下次运行即生效', '对话过程事件流全程可观测、可回放', '执行方式可插拔：进程内 / 容器 / 集群'],
      },
      en: {
        name: 'Main platform (backend)',
        desc: 'The hub for agent assembly and running: chat, multi-agent collaboration, knowledge base, skills & tools, model access and the platform assistant; embeds the two ontology planes same-origin via reverse proxy.',
        points: ['Config-driven: change config, effective on the next run', 'Conversation event streams fully observable and replayable', 'Pluggable execution: in-process / containers / cluster'],
      },
    },
  },
  {
    port: 'Go · :8091',
    i18n: {
      zh: {
        name: '本体构建平面 ontology-service',
        desc: '本体全生命周期前半程：多条构建路径、LLM 生成必过质量门禁、版本化本体资产仓库与导入审查。',
        points: ['多条构建路径收敛于同一资产库', 'LLM 参与的生成必过统一质量门禁', '资产版本化：diff、溯源与受控演化'],
      },
      en: {
        name: 'Ontology build plane (ontology-service)',
        desc: 'First half of the ontology lifecycle: build paths, LLM generations that must pass the quality gate, a versioned ontology asset store and import review.',
        points: ['Build paths converge on one asset store', 'LLM-generated artifacts pass a unified quality gate', 'Versioned assets: diff, provenance, controlled evolution'],
      },
    },
  },
  {
    port: 'Go · :8090',
    i18n: {
      zh: {
        name: '本体运行平面 runtime-manager',
        desc: '本体全生命周期后半程：运行方案编排与图引擎管理、统一工具面（MCP facade）供智能体消费。',
        points: ['运行方案编排，图引擎一键安装与启停', '服务重启后自动对账恢复', '统一 MCP 工具面，智能体无需了解引擎差异'],
      },
      en: {
        name: 'Ontology runtime plane (runtime-manager)',
        desc: 'Second half of the ontology lifecycle: run-plan orchestration and graph-engine management, plus a unified tool facade (MCP) for agent consumption.',
        points: ['Run plans with one-click engine install and start/stop', 'Automatic reconciliation after restarts', 'A unified MCP tool facade — engine differences hidden from agents'],
      },
    },
  },
]

const STACK: { i18n: L10n<{ layer: string; pick: string }> }[] = [
  {
    i18n: {
      zh: { layer: '语言 / 框架', pick: 'Go 1.22+（go.work 多模块）· Vite + React 18 + AntD 6' },
      en: { layer: 'Language / framework', pick: 'Go 1.22+ (go.work multi-module) · Vite + React 18 + AntD 6' },
    },
  },
  {
    i18n: {
      zh: { layer: 'LLM 编排', pick: 'CloudWeGo Eino + Eino ADK（ChatModel / Agent / Runner / Interrupt-Resume / Callback）' },
      en: { layer: 'LLM orchestration', pick: 'CloudWeGo Eino + Eino ADK (ChatModel / Agent / Runner / Interrupt-Resume / Callback)' },
    },
  },
  {
    i18n: {
      zh: { layer: 'MCP', pick: 'mark3labs/mcp-go + eino-ext（facade 消费 + Agent 对外 /mcp 服务化）' },
      en: { layer: 'MCP', pick: 'mark3labs/mcp-go + eino-ext (facade consumption + serving agents via /mcp)' },
    },
  },
  {
    i18n: {
      zh: { layer: '存储', pick: 'SQLite（业务库，密钥加密存储）· 向量库 Qdrant（主）+ SQLite 余弦（fallback）' },
      en: { layer: 'Storage', pick: 'SQLite (business DB, encrypted keys) · Qdrant vectors (primary) + SQLite cosine (fallback)' },
    },
  },
  {
    i18n: {
      zh: { layer: '图引擎', pick: 'Oxigraph（SPARQL，默认）· Fuseki（OWL 推理对照）' },
      en: { layer: 'Graph engines', pick: 'Oxigraph (SPARQL, default) · Fuseki (OWL reasoning comparison)' },
    },
  },
  {
    i18n: {
      zh: { layer: '可视化', pick: 'React Flow（编辑态）· 3d-force-graph（三维浏览）· WebVOWL（语义记法对照）' },
      en: { layer: 'Visualization', pick: 'React Flow (editing) · 3d-force-graph (3D browsing) · WebVOWL (notation comparison)' },
    },
  },
  {
    i18n: {
      zh: { layer: '协议', pick: 'REST + SSE（POST fetch 流式）· MCP（双向）· A2A（双向：Agent Card 发现 + JSON-RPC 子集）' },
      en: { layer: 'Protocols', pick: 'REST + SSE (POST fetch streaming) · MCP (bidirectional) · A2A (bidirectional: agent-card discovery + JSON-RPC subset)' },
    },
  },
  {
    i18n: {
      zh: { layer: '桌面壳', pick: 'Tauri 2（WebView2 / WKWebView 直载本机服务 · 系统托盘 · 单实例）' },
      en: { layer: 'Desktop shell', pick: 'Tauri 2 (WebView2 / WKWebView loading local services · system tray · single instance)' },
    },
  },
]

const DEPLOYS: { link?: string; i18n: L10n<{ title: string; desc: string; linkText: string }> }[] = [
  {
    link: '#/deploy',
    i18n: {
      zh: { title: '桌面安装包（推荐）', desc: 'Windows setup.exe / macOS universal DMG，安装即用，壳自动拉起本地服务。', linkText: '前往部署 →' },
      en: { title: 'Desktop installer (recommended)', desc: 'Windows setup.exe / macOS universal DMG — install and go; the shell boots the local services.', linkText: 'Go to deploy →' },
    },
  },
  {
    i18n: {
      zh: { title: '本地开发', desc: 'git clone 后 ./run-dev.sh（Windows 用 run-dev.ps1）一条命令拉起全栈，浏览器打开 :8080。', linkText: '' },
      en: { title: 'Local development', desc: 'After git clone, ./run-dev.sh (run-dev.ps1 on Windows) boots the full stack with one command; open :8080 in the browser.', linkText: '' },
    },
  },
  {
    i18n: {
      zh: { title: 'Docker Compose', desc: 'deploy/ 下 docker compose up -d，服务器起步推荐；远程访问凭证可在部署时配置。', linkText: '' },
      en: { title: 'Docker Compose', desc: 'docker compose up -d under deploy/ — the recommended server starting point; remote-access credentials configurable at deploy time.', linkText: '' },
    },
  },
  {
    i18n: {
      zh: { title: 'Kubernetes（Helm）', desc: 'helm install 一键部署，Ingress / 向量库按需启用，适合团队共享部署。', linkText: '' },
      en: { title: 'Kubernetes (Helm)', desc: 'One-command helm install; Ingress / vector store optional — good for team-shared deployments.', linkText: '' },
    },
  },
]

const COPY = {
  zh: {
    crumbsHome: '首页',
    title: '整体架构',
    taglinePre: '三平面 + 双代理：主平台（:8080）经',
    taglineB1: '反向代理',
    taglineMid: '与',
    taglineB2: 'MCP Client',
    taglinePost: '两种方式对接本体构建（:8091）/ 运行（:8090）两平面；一条 run-dev 命令拉起全栈，桌面安装包则由壳进程托管。',
    diagNote:
      '架构总览：客户端层（浏览器 / 桌面客户端 / 外部 MCP·A2A 客户端）→ 主平台 → 本体两平面 → 引擎与存储；大模型供应商经模型代理接入，密钥仅存本地加密。',
    planesTitle: '三平面职责',
    stackTitle: '技术栈总表',
    stackHeadLayer: '层',
    stackHeadPick: '选型',
    protoTitle: '协议面',
    deployTitle: '部署形态',
    proto1Title: 'REST + SSE',
    proto1Desc: '对话过程以统一事件流实时推送：界面按事件来源着色呈现，可展开明细、可导出重放。',
    proto2Title: 'MCP 工具面（消费侧）',
    proto2Desc: '本体能力封装为一组标准工具（查询、可回答性预检、本体操作等）挂给任意智能体——智能体无需理解图引擎差异即可消费本体。',
    proto3Title: 'MCP 服务化（供给侧）',
    proto3Desc: '任意智能体一键开放为 MCP 服务，Claude Desktop / Cursor 等客户端直连对话。',
    proto4Title: 'A2A 双向（Agent ↔ Agent）',
    proto4Desc: '供给侧：平台智能体以标准名片（agent card）暴露为 A2A 端点，任务落库可查；消费侧：远程 A2A 智能体、命令行编码智能体与本地 MCP 服务经连接器接入，与本地成员同队协作。',
    hubTitle: '协议枢纽：双向开放全景',
    hubNote:
      '左侧外部客户端调用平台智能体（MCP / A2A）；右侧外部智能体经连接器接入为委派成员；治理（审批 / 审计 / 沙箱 / 凭据 / 上下文供给）内外一体。',
    hintPre: '部署细节与环境变量速查见 ',
    hintLink: '《部署与运行》',
    hintPost: '（单实例假设，刻意不做企业级多租户能力）。',
    readTitle: '深入阅读',
    readDesign: '整体设计（事实源）',
    readTech: '技术方案设计',
    readIA: '产品信息架构',
    readReq: 'REQ 编号注册表',
  },
  en: {
    crumbsHome: 'Home',
    title: 'Architecture',
    taglinePre: 'Three planes + dual proxy: the main platform (:8080) reaches the ontology build (:8091) and runtime (:8090) planes via ',
    taglineB1: 'reverse proxy',
    taglineMid: ' and ',
    taglineB2: 'MCP client',
    taglinePost: '; one run-dev command boots the full stack, while the desktop installer is hosted by its shell process.',
    diagNote:
      'Overview: client layer (browser / desktop client / external MCP & A2A clients) → main platform → two ontology planes → engines & storage; LLM vendors connect via the model proxy, keys stored locally with encryption only.',
    planesTitle: 'Plane responsibilities',
    stackTitle: 'Tech stack table',
    stackHeadLayer: 'Layer',
    stackHeadPick: 'Choice',
    protoTitle: 'Protocol surface',
    deployTitle: 'Deployment forms',
    proto1Title: 'REST + SSE',
    proto1Desc: 'Conversation progress streams as unified events: rendered with source colors in the UI, expandable to details, exportable and replayable.',
    proto2Title: 'MCP tool facade (consumer side)',
    proto2Desc: 'Ontology capabilities wrap into a standard tool set (queries, answerability pre-checks, ontology operations) attachable to any agent — engine differences stay invisible to agents.',
    proto3Title: 'MCP serving (provider side)',
    proto3Desc: 'Any agent can become an MCP service in one click; Claude Desktop / Cursor connect directly.',
    proto4Title: 'A2A both ways (agent ↔ agent)',
    proto4Desc: 'Provider: platform agents expose as A2A endpoints via a standard agent card, with persisted tasks; consumer: remote A2A agents, CLI coding agents and local MCP services join via connectors and work alongside local members.',
    hubTitle: 'Protocol hub: open both ways',
    hubNote:
      'Left: external clients call platform agents (MCP / A2A). Right: external agents join as delegable members via connectors. Governance (approvals / audit / sandbox / credentials / context supply) applies uniformly both ways.',
    hintPre: 'Deployment details and the env-var quick reference: ',
    hintLink: 'Deploy & Run',
    hintPost: ' (single-instance assumption; enterprise multi-tenancy intentionally not provided).',
    readTitle: 'Further reading',
    readDesign: 'Overall design (source of truth)',
    readTech: 'Technical design',
    readIA: 'Product IA',
    readReq: 'REQ registry',
  },
}

export default function ArchitecturePage() {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">{c.crumbsHome}</a> / {c.title}
        </div>
        <h1>{c.title}</h1>
        <p className="module-tagline">
          {c.taglinePre}
          <b>{c.taglineB1}</b>
          {c.taglineMid}
          <b>{c.taglineB2}</b>
          {c.taglinePost}
        </p>

        {/* 架构总图 */}
        <section className="sec">
          <div className="diag-card">
            <ArchDiagram />
          </div>
          <p className="diag-note">{c.diagNote}</p>
        </section>

        {/* 三平面说明 */}
        <section className="sec">
          <h2>{c.planesTitle}</h2>
          <div className="arch-grid">
            {PLANES.map((p, pi) => {
              const t = p.i18n[locale]
              return (
                <div className="arch-card" key={pi}>
                  <div className="arch-name">{t.name}</div>
                  <div className="arch-port">{p.port}</div>
                  <p className="arch-desc">{t.desc}</p>
                  <ul>
                    {t.points.map((pt) => (
                      <li key={pt}>{pt}</li>
                    ))}
                  </ul>
                </div>
              )
            })}
          </div>
        </section>

        {/* 技术栈总表 */}
        <section className="sec">
          <h2>{c.stackTitle}</h2>
          <div className="md-body arch-table-body">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 140 }}>{c.stackHeadLayer}</th>
                  <th>{c.stackHeadPick}</th>
                </tr>
              </thead>
              <tbody>
                {STACK.map((s, si) => (
                  <tr key={si}>
                    <td>
                      <b>{s.i18n[locale].layer}</b>
                    </td>
                    <td>{s.i18n[locale].pick}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        {/* 协议面 */}
        <section className="sec">
          <h2>{c.protoTitle}</h2>
          <div className="feat-grid">
            <div className="feat-card">
              <span className="feat-icon">🔌</span>
              <div>
                <div className="feat-title">{c.proto1Title}</div>
                <div className="feat-desc">{c.proto1Desc}</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🧩</span>
              <div>
                <div className="feat-title">{c.proto2Title}</div>
                <div className="feat-desc">{c.proto2Desc}</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🌐</span>
              <div>
                <div className="feat-title">{c.proto3Title}</div>
                <div className="feat-desc">{c.proto3Desc}</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🤝</span>
              <div>
                <div className="feat-title">{c.proto4Title}</div>
                <div className="feat-desc">{c.proto4Desc}</div>
              </div>
            </div>
          </div>
          <h3 className="ref-sub">{c.hubTitle}</h3>
          <div className="diag-card">
            <ProtocolHubDiagram />
          </div>
          <p className="diag-note">{c.hubNote}</p>
        </section>

        {/* 部署形态 */}
        <section className="sec">
          <h2>{c.deployTitle}</h2>
          <div className="deploy-grid">
            {DEPLOYS.map((d, di) => {
              const t = d.i18n[locale]
              return (
                <div className="deploy-card" key={di}>
                  <div className="deploy-title">{t.title}</div>
                  <p className="deploy-desc">{t.desc}</p>
                  {d.link ? (
                    <a className="deploy-link" href={d.link}>
                      {t.linkText}
                    </a>
                  ) : null}
                </div>
              )
            })}
          </div>
          <p className="learn-hint">
            {c.hintPre}
            <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`} target="_blank" rel="noreferrer">
              {c.hintLink}
            </a>
            {c.hintPost}
          </p>
        </section>

        {/* 深入阅读 */}
        <section className="sec">
          <h2>{c.readTitle}</h2>
          <div className="tech-strip">
            <Tag style={{ margin: 0, padding: '6px 12px' }}>
              <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/00_整体设计.md`} target="_blank" rel="noreferrer">
                {c.readDesign}
              </a>
            </Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}>
              <a href={`${REPO_URL}/blob/main/docs/02_智能体_技术方案设计.md`} target="_blank" rel="noreferrer">
                {c.readTech}
              </a>
            </Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}>
              <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md`} target="_blank" rel="noreferrer">
                {c.readIA}
              </a>
            </Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}>
              <a href={`${REPO_URL}/blob/main/docs/18_REQ编号注册表.md`} target="_blank" rel="noreferrer">
                {c.readReq}
              </a>
            </Tag>
          </div>
        </section>
      </div>
    </main>
  )
}
