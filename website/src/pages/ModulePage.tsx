import {
  ApartmentOutlined,
  AimOutlined,
  ApiOutlined,
  AuditOutlined,
  BranchesOutlined,
  ClusterOutlined,
  CheckCircleFilled,
  CloudServerOutlined,
  CommentOutlined,
  CompassOutlined,
  ControlOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  EyeOutlined,
  FileDoneOutlined,
  FileTextOutlined,
  FolderOutlined,
  KeyOutlined,
  LockOutlined,
  NodeIndexOutlined,
  SearchOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  SyncOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { MODULES, moduleText, type ModuleInfo } from '../content'
import { useCopy, useLocale } from '../i18n'

// ---------------------------------------------------------------------------
// 模块详细介绍页（REQ-238 立项；REQ-364 去文档化重写）：
// 旧版正文 = platform-knowledge 导读档全文内联渲染，互引一律解析为 GitHub 文档
// 链接，页面沦为"文档链接列表"。新版 = 纙站点层结构化数据直读：速览要点 +
// 图标功能卡网格（icon + 标题 + 一句话说明），零文档链接，30 秒看懂一个模块。
// 事实口径仍以 platform-knowledge 导读档为单源（content.ts 头注），深入阅读走
// 仓库 docs；英文侧功能卡中英双份（REQ-360）。
// ---------------------------------------------------------------------------

/** 功能卡图标映射：content.ts 存组件名（.ts 纯数据无 JSX），渲染侧还原组件 */
const FEATURE_ICONS: Record<string, React.ReactNode> = {
  ApartmentOutlined: <ApartmentOutlined />,
  AimOutlined: <AimOutlined />,
  ApiOutlined: <ApiOutlined />,
  AuditOutlined: <AuditOutlined />,
  BranchesOutlined: <BranchesOutlined />,
  ClusterOutlined: <ClusterOutlined />,
  CloudServerOutlined: <CloudServerOutlined />,
  CommentOutlined: <CommentOutlined />,
  CompassOutlined: <CompassOutlined />,
  ControlOutlined: <ControlOutlined />,
  DashboardOutlined: <DashboardOutlined />,
  DatabaseOutlined: <DatabaseOutlined />,
  DeploymentUnitOutlined: <DeploymentUnitOutlined />,
  EyeOutlined: <EyeOutlined />,
  FileDoneOutlined: <FileDoneOutlined />,
  FileTextOutlined: <FileTextOutlined />,
  FolderOutlined: <FolderOutlined />,
  KeyOutlined: <KeyOutlined />,
  LockOutlined: <LockOutlined />,
  NodeIndexOutlined: <NodeIndexOutlined />,
  SearchOutlined: <SearchOutlined />,
  SafetyCertificateOutlined: <SafetyCertificateOutlined />,
  SafetyOutlined: <SafetyOutlined />,
  SyncOutlined: <SyncOutlined />,
  TeamOutlined: <TeamOutlined />,
  ThunderboltOutlined: <ThunderboltOutlined />,
}

const COPY = {
  zh: {
    crumbsModules: '功能模块',
    briefAria: '模块速览',
    featuresHead: '核心能力',
  },
  en: {
    crumbsModules: 'Modules',
    briefAria: 'Module at a glance',
    featuresHead: 'Core capabilities',
  },
}

export default function ModulePage({ info }: { info: ModuleInfo }) {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  const mt = moduleText(info, locale)
  const idx = MODULES.findIndex((m) => m.key === info.key)
  const prev = MODULES[idx - 1]
  const next = MODULES[idx + 1]

  return (
    <main className="page">
      <div className="page-inner">
        <nav className="crumbs">
          <a href="#/module/agents">{c.crumbsModules}</a>
          {' / '}
          <span>{mt.title}</span>
        </nav>
        <h1>{mt.title}</h1>
        <p className="module-tagline">{mt.tagline}</p>

        {/* 速览：30 秒看懂本模块 */}
        <section className="module-brief" aria-label={c.briefAria}>
          <span className="eyebrow">Quick Glance</span>
          <ul>
            {mt.brief.map((b) => (
              <li key={b}>
                <CheckCircleFilled className="brief-check" />
                <span>{b}</span>
              </li>
            ))}
          </ul>
        </section>

        {/* 核心能力：图标功能卡网格（REQ-364，替代原文档链接列表） */}
        <section className="sec" aria-label={c.featuresHead}>
          <h2 className="module-feat-head">{c.featuresHead}</h2>
          <div className="mfeat-grid">
            {info.features.map((f) => (
              <article className="mfeat-card" key={f.icon}>
                <span className="mfeat-icon">{FEATURE_ICONS[f.icon]}</span>
                <h3 className="mfeat-title">{f.i18n[locale].title}</h3>
                <p className="mfeat-desc">{f.i18n[locale].desc}</p>
              </article>
            ))}
          </div>
        </section>

        <nav className="module-pager">
          {prev ? (
            <a href={`#/module/${prev.key}`}>← {moduleText(prev, locale).title}</a>
          ) : (
            <span />
          )}
          {next ? <a href={`#/module/${next.key}`}>{moduleText(next, locale).title} →</a> : <span />}
        </nav>
      </div>
    </main>
  )
}
