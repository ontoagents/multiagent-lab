import { ArchDiagram, ProtocolHubDiagram } from '../components/diagrams'
import { REPO_URL } from '../content'
import { Tag } from 'antd'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'

// ---------------------------------------------------------------------------
// 整体架构页（REQ-238 扩展）：三平面 + 双代理总图（SVG 内联，源=platform-knowledge/
// 01_整体设计/00_整体设计.md §2/§3 + docs/02 §1/§2/§7 + 16_部署与运行）。
// REQ-360：全页文案中英双份（zh 为事实源）；架构图内文字随语言切换（diagrams.tsx）。
// ---------------------------------------------------------------------------

const PLANES: { port: string; i18n: L10n<{ name: string; desc: string; points: string[] }> }[] = [
  {
    port: 'Go · Hertz · :8080',
    i18n: {
      zh: {
        name: '主平台 backend',
        desc: '智能体装配与运行的中枢：Eino ADK 动态装配、SSE 事件流、知识库、技能/工具、模型代理、平台助手；经反向代理同源嵌入本体两平面。',
        points: ['配置驱动装配：改配置下次运行即生效', 'SSE 过程可观测（事件协议见 docs/02 §8）', '执行后端可插拔：inprocess / docker / k8s'],
      },
      en: {
        name: 'Main platform (backend)',
        desc: 'The hub for agent assembly and running: dynamic Eino ADK assembly, SSE event streams, knowledge base, skills/tools, model proxy, platform assistant; embeds the two ontology planes same-origin via reverse proxy.',
        points: ['Config-driven assembly: change config, effective on the next run', 'Observable SSE process (event protocol in docs/02 §8)', 'Pluggable execution backends: inprocess / docker / k8s'],
      },
    },
  },
  {
    port: 'Go · :8091',
    i18n: {
      zh: {
        name: '本体构建平面 ontology-service',
        desc: '本体全生命周期前半程：六条构建路径、LLM 生成必过质量门禁（qualitygate）、spec_json 多形态资产仓库、导入审查合并与工具链。',
        points: ['六路径收敛于资产段，差异只在生成段', '质量门禁 11 检查项 + 三维评分 + 自修复', '版本 diff / 源码视图 / 进化候选受控生长'],
      },
      en: {
        name: 'Ontology build plane (ontology-service)',
        desc: 'First half of the ontology lifecycle: six build paths, LLM generations that must pass the quality gate, a multi-form spec_json asset store, import review/merge and tooling.',
        points: ['Six paths converge on the asset stage; only generation differs', 'Quality gate: 11 checks + three-dimension score + self-repair', 'Version diff / source view / controlled growth of evolution candidates'],
      },
    },
  },
  {
    port: 'Go · :8090',
    i18n: {
      zh: {
        name: '本体运行平面 runtime-manager',
        desc: '本体全生命周期后半程：运行方案编排（Oxigraph / Fuseki 引擎）、引擎一键安装、统一 MCP facade 供智能体消费。',
        points: ['MCP facade：onto_* 五工具 + sparql_query', '方案级等值对账硬闸（装载零丢失，REQ-293）', 'Fuseki 推理形态下限闸（REQ-329）'],
      },
      en: {
        name: 'Ontology runtime plane (runtime-manager)',
        desc: 'Second half of the ontology lifecycle: run-plan orchestration (Oxigraph / Fuseki engines), one-click engine install, and a unified MCP facade for agent consumption.',
        points: ['MCP facade: five onto_* tools + sparql_query', 'Plan-level equivalence reconciliation hard gate (zero-loss loading, REQ-293)', 'Fuseki reasoning-mode lower-bound gate (REQ-329)'],
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
      zh: { layer: '存储', pick: 'SQLite（业务库，Key AES-256-GCM）· Qdrant 向量（主）+ SQLite 余弦（fallback）' },
      en: { layer: 'Storage', pick: 'SQLite (business DB, AES-256-GCM keys) · Qdrant vectors (primary) + SQLite cosine (fallback)' },
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
      zh: { title: 'Docker Compose', desc: 'deploy/ 下 docker compose up -d，服务器起步推荐；含远程访问凭证配置（REQ-209）。', linkText: '' },
      en: { title: 'Docker Compose', desc: 'docker compose up -d under deploy/ — the recommended server starting point; includes remote-access credential config (REQ-209).', linkText: '' },
    },
  },
  {
    i18n: {
      zh: { title: 'Kubernetes（Helm）', desc: 'helm install 一键部署，Ingress / Qdrant 按需启用，适合团队共享部署。', linkText: '' },
      en: { title: 'Kubernetes (Helm)', desc: 'One-command helm install; Ingress / Qdrant optional — good for team-shared deployments.', linkText: '' },
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
      '架构总览：客户端层（浏览器 / Tauri 桌面壳 / 外部 MCP 客户端）→ 主平台 → 本体两平面 → 引擎与存储；LLM 供应商经模型代理接入，Key 仅存本地加密。',
    planesTitle: '三平面职责',
    stackTitle: '技术栈总表',
    stackHeadLayer: '层',
    stackHeadPick: '选型',
    protoTitle: '协议面',
    deployTitle: '部署形态',
    proto1Title: 'REST + SSE',
    proto1Desc: 'POST fetch 流式（SSE），事件统一 {type, run_id, ts, data}；对话过程以来源着色事件卡呈现，可展开 JSON、调试原始事件、导出重放。',
    proto2Title: 'MCP facade（消费侧）',
    proto2Desc: 'runtime-manager 统一暴露 onto_* 五工具 + sparql_query，挂给任意 Agent——智能体无需理解图引擎差异即可消费本体。',
    proto3Title: 'MCP 服务化（供给侧）',
    proto3Desc: '任意智能体可一键暴露为 MCP Server（/mcp · Streamable HTTP），Claude Desktop / Cursor 直连对话（REQ-131，实测见 docs/27）。',
    proto4Title: 'A2A 双向（Agent ↔ Agent）',
    proto4Desc: '供给侧：平台智能体暴露为 A2A 端点——agent card 发现 + message/send · message/stream，任务落库可查（REQ-369/442）；消费侧：kind=a2a 连接器把任意 A2A 远程智能体接入协作委派，CLI 编码智能体（kind=cli，Claude Code / OpenCode / Aider / DSH）与本地 stdio MCP server（kind=stdio）同队（REQ-416/426/433）。',
    hubTitle: '协议枢纽：双向开放全景',
    hubNote:
      '左侧外部客户端调用平台智能体（MCP /mcp、A2A agent card 发现）；右侧外部智能体经三类连接器（CLI / A2A / stdio）接入为委派成员；治理（审批 / 审计 / 沙箱 / 凭据 / 上下文供给）内外一体。',
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
      'Overview: client layer (browser / Tauri desktop shell / external MCP clients) → main platform → two ontology planes → engines & storage; LLM vendors connect via the model proxy, keys stored locally with encryption only.',
    planesTitle: 'Plane responsibilities',
    stackTitle: 'Tech stack table',
    stackHeadLayer: 'Layer',
    stackHeadPick: 'Choice',
    protoTitle: 'Protocol surface',
    deployTitle: 'Deployment forms',
    proto1Title: 'REST + SSE',
    proto1Desc: 'Streaming via POST fetch (SSE) with a unified {type, run_id, ts, data} event envelope; the conversation renders as source-colored event cards — expandable JSON, raw-event debugging, export & replay.',
    proto2Title: 'MCP facade (consumer side)',
    proto2Desc: 'runtime-manager uniformly exposes five onto_* tools + sparql_query; attach them to any agent so it can consume ontologies without knowing graph-engine differences.',
    proto3Title: 'MCP serving (provider side)',
    proto3Desc: 'Any agent can be exposed as an MCP Server (/mcp · Streamable HTTP) in one click; Claude Desktop / Cursor connect directly (REQ-131, tested in docs/27).',
    proto4Title: 'A2A both ways (agent ↔ agent)',
    proto4Desc: 'Provider: platform agents exposed as A2A endpoints — agent-card discovery + message/send · message/stream with persisted tasks (REQ-369/442). Consumer: kind=a2a connectors pull any remote A2A agent into delegation, alongside CLI coding agents (kind=cli: Claude Code / OpenCode / Aider / DSH) and local stdio MCP servers (kind=stdio) in one team (REQ-416/426/433).',
    hubTitle: 'Protocol hub: open both ways',
    hubNote:
      'Left: external clients call platform agents (MCP /mcp; A2A agent-card discovery). Right: external agents join as delegable members via three connector kinds (CLI / A2A / stdio). Governance (approvals / audit / sandbox / credentials / context supply) applies uniformly both ways.',
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
