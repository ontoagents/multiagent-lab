import { ArchDiagram } from '../components/diagrams'
import { REPO_URL } from '../content'
import { Tag } from 'antd'

// ---------------------------------------------------------------------------
// 整体架构页（REQ-238 扩展）：三平面 + 双代理总图（SVG 内联，源=platform-knowledge/
// 01_整体设计/00_整体设计.md §2/§3 + docs/02 §1/§2/§7 + 16_部署与运行）。
// ---------------------------------------------------------------------------

const PLANES = [
  {
    name: '主平台 backend',
    port: 'Go · Hertz · :8080',
    desc: '智能体装配与运行的中枢：Eino ADK 动态装配、SSE 事件流、知识库、技能/工具、模型代理、平台助手；经反向代理同源嵌入本体两平面。',
    points: ['配置驱动装配：改配置下次运行即生效', 'SSE 过程可观测（事件协议见 docs/02 §8）', '执行后端可插拔：inprocess / docker / k8s'],
  },
  {
    name: '本体构建平面 ontology-service',
    port: 'Go · :8091',
    desc: '本体全生命周期前半程：六条构建路径、LLM 生成必过质量门禁（qualitygate）、spec_json 多形态资产仓库、导入审查合并与工具链。',
    points: ['六路径收敛于资产段，差异只在生成段', '质量门禁 11 检查项 + 三维评分 + 自修复', '版本 diff / 源码视图 / 进化候选受控生长'],
  },
  {
    name: '本体运行平面 runtime-manager',
    port: 'Go · :8090',
    desc: '本体全生命周期后半程：运行方案编排（Oxigraph / Fuseki 引擎）、引擎一键安装、统一 MCP facade 供智能体消费。',
    points: ['MCP facade：onto_* 五工具 + sparql_query', '方案级等值对账硬闸（装载零丢失，REQ-293）', 'Fuseki 推理形态下限闸（REQ-329）'],
  },
]

const STACK: { layer: string; pick: string }[] = [
  { layer: '语言 / 框架', pick: 'Go 1.22+（go.work 多模块）· Vite + React 18 + AntD 6' },
  { layer: 'LLM 编排', pick: 'CloudWeGo Eino + Eino ADK（ChatModel / Agent / Runner / Interrupt-Resume / Callback）' },
  { layer: 'MCP', pick: 'mark3labs/mcp-go + eino-ext（facade 消费 + Agent 对外 /mcp 服务化）' },
  { layer: '存储', pick: 'SQLite（业务库，Key AES-256-GCM）· Qdrant 向量（主）+ SQLite 余弦（fallback）' },
  { layer: '图引擎', pick: 'Oxigraph（SPARQL，默认）· Fuseki（OWL 推理对照）' },
  { layer: '可视化', pick: 'React Flow（编辑态）· 3d-force-graph（三维浏览）· WebVOWL（语义记法对照）' },
  { layer: '协议', pick: 'REST + SSE（POST fetch 流式）· MCP（双向）' },
  { layer: '桌面壳', pick: 'Tauri 2（WebView2 / WKWebView 直载本机服务 · 系统托盘 · 单实例）' },
]

const DEPLOYS = [
  { title: '桌面安装包（推荐）', desc: 'Windows setup.exe / macOS universal DMG，安装即用，壳自动拉起本地服务。', link: '#/download', linkText: '前往下载 →' },
  { title: '本地开发', desc: 'git clone 后 ./run-dev.sh（Windows 用 run-dev.ps1）一条命令拉起全栈，浏览器打开 :8080。', link: undefined, linkText: '' },
  { title: 'Docker Compose', desc: 'deploy/ 下 docker compose up -d，服务器起步推荐；含远程访问凭证配置（REQ-209）。', link: undefined, linkText: '' },
  { title: 'Kubernetes（Helm）', desc: 'helm install 一键部署，Ingress / Qdrant 按需启用，适合团队共享部署。', link: undefined, linkText: '' },
]

export default function ArchitecturePage() {
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">首页</a> / 整体架构
        </div>
        <h1>整体架构</h1>
        <p className="module-tagline">
          三平面 + 双代理：主平台（:8080）经<b>反向代理</b>与<b>MCP Client</b>两种方式对接本体构建（:8091）/ 运行（:8090）两平面；
          一条 run-dev 命令拉起全栈，桌面安装包则由壳进程托管。
        </p>

        {/* 架构总图 */}
        <section className="sec">
          <div className="diag-card">
            <ArchDiagram />
          </div>
          <p className="diag-note">
            架构总览：客户端层（浏览器 / Tauri 桌面壳 / 外部 MCP 客户端）→ 主平台 → 本体两平面 → 引擎与存储；LLM 供应商经模型代理接入，Key 仅存本地加密。
          </p>
        </section>

        {/* 三平面说明 */}
        <section className="sec">
          <h2>三平面职责</h2>
          <div className="arch-grid">
            {PLANES.map((p) => (
              <div className="arch-card" key={p.name}>
                <div className="arch-name">{p.name}</div>
                <div className="arch-port">{p.port}</div>
                <p className="arch-desc">{p.desc}</p>
                <ul>
                  {p.points.map((pt) => (
                    <li key={pt}>{pt}</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </section>

        {/* 技术栈总表 */}
        <section className="sec">
          <h2>技术栈总表</h2>
          <div className="md-body arch-table-body">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 140 }}>层</th>
                  <th>选型</th>
                </tr>
              </thead>
              <tbody>
                {STACK.map((s) => (
                  <tr key={s.layer}>
                    <td><b>{s.layer}</b></td>
                    <td>{s.pick}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        {/* 协议面 */}
        <section className="sec">
          <h2>协议面</h2>
          <div className="feat-grid">
            <div className="feat-card">
              <span className="feat-icon">🔌</span>
              <div>
                <div className="feat-title">REST + SSE</div>
                <div className="feat-desc">POST fetch 流式（SSE），事件统一 {'{type, run_id, ts, data}'}；对话过程以来源着色事件卡呈现，可展开 JSON、调试原始事件、导出重放。</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🧩</span>
              <div>
                <div className="feat-title">MCP facade（消费侧）</div>
                <div className="feat-desc">runtime-manager 统一暴露 onto_* 五工具 + sparql_query，挂给任意 Agent——智能体无需理解图引擎差异即可消费本体。</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🌐</span>
              <div>
                <div className="feat-title">MCP 服务化（供给侧）</div>
                <div className="feat-desc">任意智能体可一键暴露为 MCP Server（/mcp · Streamable HTTP），Claude Desktop / Cursor 直连对话（REQ-131，实测见 docs/27）。</div>
              </div>
            </div>
          </div>
        </section>

        {/* 部署形态 */}
        <section className="sec">
          <h2>部署形态</h2>
          <div className="deploy-grid">
            {DEPLOYS.map((d) => (
              <div className="deploy-card" key={d.title}>
                <div className="deploy-title">{d.title}</div>
                <p className="deploy-desc">{d.desc}</p>
                {d.link ? <a className="deploy-link" href={d.link}>{d.linkText}</a> : null}
              </div>
            ))}
          </div>
          <p className="learn-hint">
            部署细节与环境变量速查见 <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`} target="_blank" rel="noreferrer">《部署与运行》</a>
            （单实例假设，刻意不做企业级多租户能力）。
          </p>
        </section>

        {/* 深入阅读 */}
        <section className="sec">
          <h2>深入阅读</h2>
          <div className="tech-strip">
            <Tag style={{ margin: 0, padding: '6px 12px' }}><a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/00_整体设计.md`} target="_blank" rel="noreferrer">整体设计（事实源）</a></Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}><a href={`${REPO_URL}/blob/main/docs/02_智能体_技术方案设计.md`} target="_blank" rel="noreferrer">技术方案设计</a></Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}><a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md`} target="_blank" rel="noreferrer">产品信息架构</a></Tag>
            <Tag style={{ margin: 0, padding: '6px 12px' }}><a href={`${REPO_URL}/blob/main/docs/18_REQ编号注册表.md`} target="_blank" rel="noreferrer">REQ 编号注册表</a></Tag>
          </div>
        </section>
      </div>
    </main>
  )
}
