import { AppleOutlined, WindowsOutlined } from '@ant-design/icons'
import { Button } from 'antd'
import { REPO_URL } from '../content'

// ---------------------------------------------------------------------------
// 客户端下载页（REQ-238 扩展）：桌面安装包（REQ-327 发布面）+ 源码构建 + 容器部署。
// 下载地址口径：发布面 gate 硬编码 ontoagents/multiagent-lab（public 仓，v* tag 触发
// client-release.yml 双平台构建并发 GitHub Release），故此处固定指向该仓 Releases，
// 不随构建仓（REPO_SLUG）注入变化。
// ---------------------------------------------------------------------------

const RELEASES_URL = 'https://github.com/ontoagents/multiagent-lab/releases'
const RELEASES_LATEST = `${RELEASES_URL}/latest`

const CLIENTS = [
  {
    icon: <WindowsOutlined />,
    platform: 'Windows',
    file: 'MultiAgentLab_<版本>_setup.exe',
    desc: 'Inno Setup 安装向导（per-user 安装，约 23MB）。安装后从开始菜单启动：壳自动拉起本地三服务并打开平台窗口，关窗收托盘、服务状态托盘实时可见。',
    note: '未签名：首次运行 SmartScreen 可能提示，点「更多信息 → 仍要运行」。',
  },
  {
    icon: <AppleOutlined />,
    platform: 'macOS',
    file: 'MultiAgentLab_<版本>_macos_universal.dmg',
    desc: 'Apple Silicon + Intel 通用二进制（一包双跑）。双击 MultiAgentLab.app 即启动：壳自动拉起本地服务并打开平台窗口。',
    note: 'ad-hoc 签名：首次打开若被 Gatekeeper 拦截，右键 App →「打开」。',
  },
]

export default function DownloadPage() {
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">首页</a> / 下载
        </div>
        <h1>下载与安装</h1>
        <p className="module-tagline">
          桌面客户端开箱即用：安装包内置平台全部服务与前端，安装即用、数据全本地（SQLite 单文件），
          无云端依赖。版本随 v* tag 自动构建发布（GitHub Releases）。
        </p>

        {/* 桌面客户端 */}
        <section className="sec">
          <h2>桌面客户端</h2>
          <div className="dl-grid">
            {CLIENTS.map((c) => (
              <div className="dl-card" key={c.platform}>
                <div className="dl-head">
                  <span className="dl-icon">{c.icon}</span>
                  <div>
                    <div className="dl-platform">{c.platform}</div>
                    <code className="dl-file">{c.file}</code>
                  </div>
                </div>
                <p className="dl-desc">{c.desc}</p>
                <p className="dl-note">{c.note}</p>
                <Button type="primary" href={RELEASES_LATEST} target="_blank" rel="noreferrer" block>
                  下载 {c.platform} 版本 →
                </Button>
              </div>
            ))}
          </div>
          <p className="dl-releases-hint">
            全部历史版本与更新日志见{' '}
            <a href={RELEASES_URL} target="_blank" rel="noreferrer">
              github.com/ontoagents/multiagent-lab/releases
            </a>
            ；推 v* tag 自动双平台构建发版（REQ-327），产物按版本号自动命名。
          </p>
        </section>

        {/* 安装后说明 */}
        <section className="sec">
          <h2>安装后</h2>
          <div className="feat-grid">
            <div className="feat-card">
              <span className="feat-icon">⚙️</span>
              <div>
                <div className="feat-title">首次启动</div>
                <div className="feat-desc">自动初始化 SQLite 数据库并预置「DeepSeek（预置）」模型连接；打开 设置 → 模型管理 填入 API Key 即可对话（Key 本地 AES-256-GCM 加密存储，不上传）。</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🖥️</span>
              <div>
                <div className="feat-title">服务托管</div>
                <div className="feat-desc">Tauri 壳自动拉起 backend（:8080）/ ontology-service（:8091）/ runtime-manager（:8090）三服务，托盘实时显示服务状态；数据根目录默认 %LOCALAPPDATA%/MultiAgentLab（Windows）。</div>
              </div>
            </div>
            <div className="feat-card">
              <span className="feat-icon">🧭</span>
              <div>
                <div className="feat-title">学习路径</div>
                <div className="feat-desc">内置学习中心提供七阶段学习路径与 5 个示例本体（组织 / 缺陷 / 故障 / 基因核心 / 医学常见病），首次使用建议从学习中心开始。</div>
              </div>
            </div>
          </div>
        </section>

        {/* 源码构建 */}
        <section className="sec" id="from-source">
          <h2>源码构建</h2>
          <div className="qs-grid">
            <div className="qs-card">
              <div className="qs-title">开发模式（一条命令启动全栈）</div>
              <pre className="qs-code">{`git clone https://github.com/ontoagents/multiagent-lab.git
cd multiagent-lab
./run-dev.sh          # Windows: .\\run-dev.ps1
# 打开 http://localhost:8080`}</pre>
              <p className="qs-note">依赖：Go 1.25+、Node 18+、Oxigraph（引擎方案启动需要，一键安装脚本见仓库《部署与运行》）；零 Python venv。</p>
            </div>
            <div className="qs-card">
              <div className="qs-title">桌面安装包（本地构建）</div>
              <pre className="qs-code">{`# Windows（Inno Setup / Go SFX 兜底双路径）
powershell -File deploy/windows/build.ps1
# macOS（含 DMG 组装）
bash deploy/macos/build.sh`}</pre>
              <p className="qs-note">构建脚本与 CI（client-release.yml）完全同源——CI 只是零改动复用这两个脚本。</p>
            </div>
            <div className="qs-card">
              <div className="qs-title">Docker Compose（服务器起步）</div>
              <pre className="qs-code">{`cd deploy
docker compose up -d
# 访问 http://<host>:8080`}</pre>
              <p className="qs-note">远程访问需配置访问凭证（REQ-209）；网络边界警告见《部署与运行》§3.0。</p>
            </div>
            <div className="qs-card">
              <div className="qs-title">Kubernetes（Helm）</div>
              <pre className="qs-code">{`helm install eino-lab deploy/helm/eino-lab \\
  --set ingress.enabled=true \\
  --set qdrant.enabled=true`}</pre>
              <p className="qs-note">适合团队共享部署；Ingress / Qdrant / 副本参数按需覆盖。</p>
            </div>
          </div>
          <p className="learn-hint">
            更多部署细节（依赖矩阵 / 环境变量速查 / 健康检查）见仓库{' '}
            <a href={`${REPO_URL}/blob/main/platform-knowledge/01_整体设计/16_部署与运行.md`} target="_blank" rel="noreferrer">《部署与运行》</a>。
          </p>
        </section>
      </div>
    </main>
  )
}
