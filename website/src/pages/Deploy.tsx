import { CloudServerOutlined, DesktopOutlined, ExperimentOutlined, RocketOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { Button, Tag } from 'antd'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'
import type { ReactNode } from 'react'

// ---------------------------------------------------------------------------
// 部署运行页（REQ-238）：平台三种部署运行方式——桌面客户端 / 数据中心服务端（web 访问）/
// 本地运行（浏览器访问），外加版本发布与运维速览。事实源 = platform-knowledge/
// 01_整体设计/16_部署与运行.md（v0.16），本页为面向访客的摘编；安装包发布面 = public 仓
// ontoagents/multiagent-lab（REQ-327，与开发主仓解耦）。
// REQ-360：全页文案中英双份（zh 为事实源）；命令块保持通用，注释行随语言。
// ---------------------------------------------------------------------------

const RELEASES_URL = 'https://github.com/ontoagents/multiagent-lab/releases'
const DEPLOY_DOC_URL = `https://github.com/ontoagents/multiagent-lab/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`

const WAYS: {
  icon: ReactNode
  cmd?: string
  link?: { href: string; text: L10n<string> }
  i18n: L10n<{ title: string; badge: string; fit: string; access: string; points: string[] }>
}[] = [
  {
    icon: <DesktopOutlined />,
    link: { href: RELEASES_URL, text: { zh: '前往 Releases 下载安装包 →', en: 'Get the installer from Releases →' } },
    i18n: {
      zh: {
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
      },
      en: {
        title: 'Desktop client',
        badge: 'Personal use · recommended',
        fit: 'Install and go — data stays on your machine, works offline. The installer bundles every platform service and the frontend; the shell process orchestrates them, visible in the system tray.',
        access: 'Double-click the icon to start; the shell boots the three local services and opens the platform window',
        points: [
          'Windows: setup.exe graphical wizard (~37MB, choose the install drive; data lives in the install directory)',
          'macOS: universal DMG (~64MB, one package for Apple Silicon + Intel)',
          'Local single-file SQLite storage; model keys encrypted with AES-256-GCM, never uploaded',
          'Dependency engines (oxigraph / python sidecar / Fuseki) install in one click on first start — not bundled',
        ],
      },
    },
  },
  {
    icon: <CloudServerOutlined />,
    cmd: `cd deploy\ndocker compose up -d --build      # 需向量检索：--profile qdrant\n# 访问 http://<host>:8080`,
    i18n: {
      zh: {
        title: '数据中心服务端部署',
        badge: '团队共享 · web 访问',
        fit: '部署在服务器 / 数据中心，团队成员通过浏览器远程访问同一实例；Docker Compose 起步，Kubernetes 进阶。',
        access: '部署完成后浏览器访问 http://<host>:8080（backend 内置前端托管）',
        points: [
          '远程访问凭证：backend 设 PLATFORM_TOKEN，非本机访问 /api/* 须携带（Bearer 头 / ?access_token），本机回环豁免',
          '安全边界：对外只暴露 :8080；:8090 / :8091 与引擎端口仅内部网络可达，绝不暴露公网',
          'Kubernetes：helm install 一键部署（Ingress 可选）；SQLite 单文件存储，单副本运行',
          'HTTPS：明文 HTTP 传输加密请经反代（Caddy / Nginx）终结',
        ],
      },
      en: {
        title: 'Data-center server deployment',
        badge: 'Team shared · web access',
        fit: 'Deploy on a server / data center; team members reach the same instance from a browser. Start with Docker Compose, move up to Kubernetes.',
        access: 'After deployment, open http://<host>:8080 in a browser (the backend serves the frontend)',
        points: [
          'Remote-access credentials: set PLATFORM_TOKEN on the backend; non-local /api/* calls must carry it (Bearer header / ?access_token); loopback is exempt',
          'Security boundary: only :8080 is exposed; :8090 / :8091 and engine ports stay on the internal network — never expose them publicly',
          'Kubernetes: one-command helm install (Ingress optional); single-file SQLite storage, single replica',
          'HTTPS: terminate TLS on a reverse proxy (Caddy / Nginx) in front of plain HTTP',
        ],
      },
    },
  },
  {
    icon: <ExperimentOutlined />,
    cmd: `git clone https://github.com/ontoagents/multiagent-lab.git\ncd multiagent-lab\n./run-dev.sh          # Windows: .\\run-dev.ps1\n# 打开 http://localhost:8080`,
    i18n: {
      zh: {
        title: '本地运行',
        badge: '开发 / 演示 · 浏览器访问',
        fit: '源码一条命令拉起全栈，本机浏览器访问；适合开发调试、功能演示与源码阅读。',
        access: '浏览器打开 http://localhost:8080（前端 :5173 热更新可并行）',
        points: [
          '依赖：Go 1.25+ · Node 18+ · python3 + rdflib · oxigraph 0.5+（一键安装脚本见仓库）',
          '缺依赖不阻塞启动：对应能力自动降级，run-dev.sh 打印警告',
          '零 Python venv；前端热更新 cd web && npm run dev',
        ],
      },
      en: {
        title: 'Run locally',
        badge: 'Dev / demo · browser access',
        fit: 'One command from source boots the full stack, accessed in a local browser; good for development, demos and code reading.',
        access: 'Open http://localhost:8080 in a browser (frontend hot reload on :5173 can run in parallel)',
        points: [
          'Dependencies: Go 1.25+ · Node 18+ · python3 + rdflib · oxigraph 0.5+ (one-click install scripts in the repo)',
          'Missing dependencies never block startup: related capabilities degrade automatically; run-dev.sh prints warnings',
          'Zero Python venv; frontend hot reload via cd web && npm run dev',
        ],
      },
    },
  },
]

const OPS: { icon: ReactNode; i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    icon: <RocketOutlined />,
    i18n: {
      zh: {
        title: '版本与发布',
        desc: '推 v* tag 自动触发 GitHub Actions 双平台构建并发版（REQ-327），产物按版本命名：MultiAgentLab_<版本>_setup.exe / _macos_universal.dmg。历史版本与更新日志见 Releases。',
      },
      en: {
        title: 'Versions & releases',
        desc: 'Pushing a v* tag triggers GitHub Actions builds for both platforms and a release (REQ-327); artifacts are named per version: MultiAgentLab_<version>_setup.exe / _macos_universal.dmg. History and changelogs in Releases.',
      },
    },
  },
  {
    icon: <ThunderboltOutlined />,
    i18n: {
      zh: {
        title: '健康检查与数据',
        desc: '健康检查 GET :8080 / :8091 / :8090 的 /healthz；备份直接复制 data/ 目录（.secret 密钥文件与 DB 一并带走，否则加密数据无法解密）；停服后复制最稳。',
      },
      en: {
        title: 'Health checks & data',
        desc: 'Health checks at GET :8080 / :8091 / :8090 /healthz; back up by copying the data/ directory (take the .secret key file together with the DB, or encrypted data cannot be decrypted); copy while stopped for consistency.',
      },
    },
  },
]

/** 命令块注释行随语言（命令本体通用） */
const CMD_NOTES = {
  zh: {
    deploy: `cd deploy\ndocker compose up -d --build      # 需向量检索：--profile qdrant\n# 访问 http://<host>:8080`,
    local: `git clone https://github.com/ontoagents/multiagent-lab.git\ncd multiagent-lab\n./run-dev.sh          # Windows: .\\run-dev.ps1\n# 打开 http://localhost:8080`,
  },
  en: {
    deploy: `cd deploy\ndocker compose up -d --build      # add --profile qdrant for vector retrieval\n# open http://<host>:8080`,
    local: `git clone https://github.com/ontoagents/multiagent-lab.git\ncd multiagent-lab\n./run-dev.sh          # Windows: .\\run-dev.ps1\n# open http://localhost:8080`,
  },
}

const COPY = {
  zh: {
    eyebrow: 'Deploy & Run',
    title: '部署运行',
    leadPre: '一套代码，三种运行方式：',
    leadB1: '桌面客户端',
    leadMid1: '（个人自用，开箱即用）、',
    leadB2: '数据中心服务端部署',
    leadMid2: '（团队共享，浏览器远程访问）、',
    leadB3: '本地运行',
    leadMid3: '（开发演示，本机浏览器访问）。部署细节、环境变量速查与排障手册见仓库 ',
    leadLink: '《部署与运行》',
    leadPost: '。',
    accessLabel: '访问方式',
    opsTitle: '版本与运维',
    releasesHintPre: '全部安装包与更新日志：',
    releasesHintMid: '　·　未签名说明：Windows 首次运行 SmartScreen 提示点「更多信息 → 仍要运行」；macOS 被 Gatekeeper 拦截时右键 App →「打开」。',
  },
  en: {
    eyebrow: 'Deploy & Run',
    title: 'Deploy',
    leadPre: 'One codebase, three ways to run: the ',
    leadB1: 'desktop client',
    leadMid1: ' (personal, out of the box), the ',
    leadB2: 'data-center server',
    leadMid2: ' (team shared, remote browser access), or ',
    leadB3: 'locally from source',
    leadMid3: ' (dev / demo, local browser). Deployment details, the env-var quick reference and troubleshooting: ',
    leadLink: 'Deploy & Run',
    leadPost: '.',
    accessLabel: 'Access',
    opsTitle: 'Versions & operations',
    releasesHintPre: 'All installers and changelogs: ',
    releasesHintMid: '　·　Unsigned note: on Windows, SmartScreen → "More info → Run anyway"; on macOS, if Gatekeeper blocks the app, right-click → "Open".',
  },
}

export default function DeployPage() {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  const cmds = CMD_NOTES[locale]
  return (
    <main className="page">
      <div className="page-inner">
        <div className="page-hero">
          <span className="eyebrow">{c.eyebrow}</span>
          <h1>{c.title}</h1>
          <p className="lead">
            {c.leadPre}
            <strong>{c.leadB1}</strong>
            {c.leadMid1}
            <strong>{c.leadB2}</strong>
            {c.leadMid2}
            <strong>{c.leadB3}</strong>
            {c.leadMid3}
            <a href={DEPLOY_DOC_URL} target="_blank" rel="noreferrer">
              {c.leadLink}
            </a>
            {c.leadPost}
          </p>
        </div>

        <section className="sec">
          <div className="way-grid">
            {WAYS.map((w, wi) => {
              const t = w.i18n[locale]
              return (
                <article key={wi} className="way-card">
                  <header className="dl-head">
                    <span className="dl-icon">{w.icon}</span>
                    <div>
                      <div className="dl-platform">{t.title}</div>
                      <Tag color="geekblue" style={{ marginTop: 4, fontSize: 11 }}>
                        {t.badge}
                      </Tag>
                    </div>
                  </header>
                  <p className="way-fit">{t.fit}</p>
                  {w.cmd && <pre className="qs-code way-cmd">{locale === 'zh' ? w.cmd : wi === 1 ? cmds.deploy : cmds.local}</pre>}
                  <div className="way-access">
                    <span className="way-access-label">{c.accessLabel}</span>
                    {t.access}
                  </div>
                  <ul className="way-points">
                    {t.points.map((p, i) => (
                      <li key={i}>{p}</li>
                    ))}
                  </ul>
                  {w.link && (
                    <p className="way-link">
                      <Button type="primary" href={w.link.href} target="_blank" rel="noreferrer">
                        {w.link.text[locale]}
                      </Button>
                    </p>
                  )}
                </article>
              )
            })}
          </div>
        </section>

        <section className="sec">
          <h2>{c.opsTitle}</h2>
          <div className="feat-grid">
            {OPS.map((o, oi) => (
              <div className="feat-card" key={oi}>
                <span className="feat-icon">{o.icon}</span>
                <div>
                  <div className="feat-title">{o.i18n[locale].title}</div>
                  <div className="feat-desc">{o.i18n[locale].desc}</div>
                </div>
              </div>
            ))}
          </div>
          <p className="dl-releases-hint">
            {c.releasesHintPre}
            <a href={RELEASES_URL} target="_blank" rel="noreferrer">
              github.com/ontoagents/multiagent-lab/releases
            </a>
            {c.releasesHintMid}
          </p>
        </section>
      </div>
    </main>
  )
}
