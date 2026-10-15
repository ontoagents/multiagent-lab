import { CloudServerOutlined, DesktopOutlined, ExperimentOutlined, RocketOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { Button, Tag } from 'antd'

// ---------------------------------------------------------------------------
// 部署运行页（REQ-238）：平台三种部署运行方式——桌面客户端 / 数据中心服务端（web 访问）/
// 本地运行（浏览器访问），外加版本发布与运维速览。事实源 = platform-knowledge/
// 01_整体设计/16_部署与运行.md（v0.16），本页为面向访客的摘编；安装包发布面 = public 仓
// ontoagents/multiagent-lab（REQ-327，与开发主仓解耦）。
// ---------------------------------------------------------------------------

const RELEASES_URL = 'https://github.com/ontoagents/multiagent-lab/releases'
const DEPLOY_DOC_URL = `https://github.com/ontoagents/multiagent-lab/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`

const WAYS: {
  icon: React.ReactNode
  title: string
  badge: string
  fit: string
  cmd?: string
  access: string
  points: string[]
  link?: { href: string; text: string }
}[] = [
  {
    icon: <DesktopOutlined />,
    title: '桌面客户端',
    badge: '个人自用 · 推荐',
    fit: '安装即用，数据全留在本机，离线可用。安装包内置平台全部服务与前端，壳进程自动编排、托盘可见。',
    access: '双击图标即启动，壳自动拉起三个本地服务并打开平台窗口',
    points: [
      'Windows：setup.exe 图形向导（约 37MB，可改安装盘，数据随安装目录）',
      'macOS：universal DMG（约 64MB，Apple Silicon + Intel 一包双跑）',
      '数据本地 SQLite 单文件；模型 Key AES-256-GCM 加密，不上传',
      '依赖引擎（oxigraph / python sidecar / Fuseki）首启一键安装，不捆绑安装包',
    ],
    link: { href: RELEASES_URL, text: '前往 Releases 下载安装包 →' },
  },
  {
    icon: <CloudServerOutlined />,
    title: '数据中心服务端部署',
    badge: '团队共享 · web 访问',
    fit: '部署在服务器 / 数据中心，团队成员通过浏览器远程访问同一实例；Docker Compose 起步，Kubernetes 进阶。',
    cmd: `cd deploy\ndocker compose up -d --build      # 需向量检索：--profile qdrant\n# 访问 http://<host>:8080`,
    access: '部署完成后浏览器访问 http://<host>:8080（backend 内置前端托管）',
    points: [
      '远程访问凭证：backend 设 PLATFORM_TOKEN，非本机访问 /api/* 须携带（Bearer 头 / ?access_token），本机回环豁免',
      '安全边界：对外只暴露 :8080；:8090 / :8091 与引擎端口仅内部网络可达，绝不暴露公网',
      'Kubernetes：helm install 一键部署（Ingress 可选）；SQLite 单文件存储，单副本运行',
      'HTTPS：明文 HTTP 传输加密请经反代（Caddy / Nginx）终结',
    ],
  },
  {
    icon: <ExperimentOutlined />,
    title: '本地运行',
    badge: '开发 / 演示 · 浏览器访问',
    fit: '源码一条命令拉起全栈，本机浏览器访问；适合开发调试、功能演示与源码阅读。',
    cmd: `git clone https://github.com/ontoagents/multiagent-lab.git\ncd multiagent-lab\n./run-dev.sh          # Windows: .\\run-dev.ps1\n# 打开 http://localhost:8080`,
    access: '浏览器打开 http://localhost:8080（前端 :5173 热更新可并行）',
    points: [
      '依赖：Go 1.25+ · Node 18+ · python3 + rdflib · oxigraph 0.5+（一键安装脚本见仓库）',
      '缺依赖不阻塞启动：对应能力自动降级，run-dev.sh 打印警告',
      '零 Python venv；前端热更新 cd web && npm run dev',
    ],
  },
]

const OPS: { icon: React.ReactNode; title: string; desc: string }[] = [
  {
    icon: <RocketOutlined />,
    title: '版本与发布',
    desc: '推 v* tag 自动触发 GitHub Actions 双平台构建并发版（REQ-327），产物按版本命名：MultiAgentLab_<版本>_setup.exe / _macos_universal.dmg。历史版本与更新日志见 Releases。',
  },
  {
    icon: <ThunderboltOutlined />,
    title: '健康检查与数据',
    desc: '健康检查 GET :8080 / :8091 / :8090 的 /healthz；备份直接复制 data/ 目录（.secret 密钥文件与 DB 一并带走，否则加密数据无法解密）；停服后复制最稳。',
  },
]

export default function DeployPage() {
  return (
    <main className="page">
      <div className="page-inner">
        <div className="page-hero">
          <span className="eyebrow">Deploy &amp; Run</span>
          <h1>部署运行</h1>
          <p className="lead">
            一套代码，三种运行方式：<strong>桌面客户端</strong>（个人自用，开箱即用）、
            <strong>数据中心服务端部署</strong>（团队共享，浏览器远程访问）、
            <strong>本地运行</strong>（开发演示，本机浏览器访问）。
            部署细节、环境变量速查与排障手册见仓库{' '}
            <a href={DEPLOY_DOC_URL} target="_blank" rel="noreferrer">《部署与运行》</a>。
          </p>
        </div>

        <section className="sec">
          <div className="way-grid">
            {WAYS.map((w) => (
              <article key={w.title} className="way-card">
                <header className="dl-head">
                  <span className="dl-icon">{w.icon}</span>
                  <div>
                    <div className="dl-platform">{w.title}</div>
                    <Tag color="geekblue" style={{ marginTop: 4, fontSize: 11 }}>
                      {w.badge}
                    </Tag>
                  </div>
                </header>
                <p className="way-fit">{w.fit}</p>
                {w.cmd && <pre className="qs-code way-cmd">{w.cmd}</pre>}
                <div className="way-access">
                  <span className="way-access-label">访问方式</span>
                  {w.access}
                </div>
                <ul className="way-points">
                  {w.points.map((p, i) => (
                    <li key={i}>{p}</li>
                  ))}
                </ul>
                {w.link && (
                  <p className="way-link">
                    <Button type="primary" href={w.link.href} target="_blank" rel="noreferrer">
                      {w.link.text}
                    </Button>
                  </p>
                )}
              </article>
            ))}
          </div>
        </section>

        <section className="sec">
          <h2>版本与运维</h2>
          <div className="feat-grid">
            {OPS.map((o) => (
              <div className="feat-card" key={o.title}>
                <span className="feat-icon">{o.icon}</span>
                <div>
                  <div className="feat-title">{o.title}</div>
                  <div className="feat-desc">{o.desc}</div>
                </div>
              </div>
            ))}
          </div>
          <p className="dl-releases-hint">
            全部安装包与更新日志：<a href={RELEASES_URL} target="_blank" rel="noreferrer">github.com/ontoagents/multiagent-lab/releases</a>
            　·　未签名说明：Windows 首次运行 SmartScreen 提示点「更多信息 → 仍要运行」；macOS 被 Gatekeeper 拦截时右键 App →「打开」。
          </p>
        </section>
      </div>
    </main>
  )
}
