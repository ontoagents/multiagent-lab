import { useEffect, useState } from 'react'
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
import { MODULES, moduleText } from '../content'
import { useCopy, useLocale } from '../i18n'

// ---------------------------------------------------------------------------
// 功能模块页（REQ-238 立项；REQ-364 去文档化重写；迭代批单页纵排改造）：
// 旧版 = 每模块一页（#/module/<key>），页间仅靠底部 prev/next 翻页器切换——入口藏
// 在页底且一次只见相邻两个，开发者反馈「切换方式隐藏地太深了，很难发现」。
// 新版 = 七模块单页纵排 + 吸顶锚点条：全部模块名常驻可见（topnav 之下 sticky），
// 点击平滑跳转对应区块、滚动时当前区块高亮（IntersectionObserver）——上下切换，
// 一眼可见。旧深链 #/module/<key>（首页模块卡/页脚/实现页互引）兼容：挂载即定位
// 到对应模块区块。页内跳转用 scrollIntoView（不能用 #hash 锚点，与 hash 路由冲突，
// Implementation 页同款先例）。
// 内容形态沿 REQ-364：速览要点 + 图标功能卡网格（icon + 标题 + 一句话说明），
// 零文档链接；事实口径仍以 platform-knowledge 导读档为单源（content.ts 头注）；
// 英文侧功能卡中英双份（REQ-360）。
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
    crumbsHome: '首页',
    title: '功能模块',
    tagline: '七个模块一页纵览——点击模块名直达对应区块，或直接向下滚动逐一了解。',
    navAria: '模块导航',
    briefAria: '模块速览',
    featuresHead: '核心能力',
  },
  en: {
    crumbsHome: 'Home',
    title: 'Modules',
    tagline: 'All seven modules on one page — click a module name to jump to it, or simply scroll.',
    navAria: 'Module navigation',
    briefAria: 'Module at a glance',
    featuresHead: 'Core capabilities',
  },
}

export default function ModulePage({ initial }: { initial?: string }) {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  const [active, setActive] = useState(initial ?? MODULES[0].key)

  // 旧深链 #/module/<key>：定位到对应模块区块（instant 直达语义；跨 key 跳转同样生效）
  useEffect(() => {
    if (!initial) return
    document.getElementById(`module-${initial}`)?.scrollIntoView({ block: 'start' })
    setActive(initial)
  }, [initial])

  // 滚动高亮：区块进入「吸顶条下方视口带」即点亮对应 chip
  useEffect(() => {
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive(String(e.target.id).replace('module-', ''))
        }
      },
      // 顶栏 56px + 吸顶条 ~52px + 余量；下沿收窄到视口上部 45%，保证「正在阅读的区块」胜出
      { rootMargin: '-130px 0px -55% 0px' },
    )
    for (const m of MODULES) {
      const el = document.getElementById(`module-${m.key}`)
      if (el) obs.observe(el)
    }
    return () => obs.disconnect()
  }, [])

  const jump = (key: string) => {
    setActive(key)
    document.getElementById(`module-${key}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <main className="page page-modules">
      <div className="page-inner">
        <nav className="crumbs">
          <a href="#/">{c.crumbsHome}</a>
          {' / '}
          <span>{c.title}</span>
        </nav>
        <h1>{c.title}</h1>
        <p className="module-tagline">{c.tagline}</p>
      </div>

      {/* 吸顶模块锚点条：直接挂在 main 下（父容器纵贯整页才吸得住；7 个模块名常驻可见） */}
      <nav className="module-nav" aria-label={c.navAria}>
        <div className="module-nav-inner">
          {MODULES.map((m) => (
            <button
              key={m.key}
              type="button"
              className={`module-nav-item${active === m.key ? ' active' : ''}`}
              onClick={() => jump(m.key)}
            >
              {moduleText(m, locale).title}
            </button>
          ))}
        </div>
      </nav>

      <div className="page-inner">
        {MODULES.map((m) => {
          const mt = moduleText(m, locale)
          return (
            <section className="mod-section" id={`module-${m.key}`} key={m.key} aria-label={mt.title}>
              <h2 className="mod-title">{mt.title}</h2>
              <p className="mod-tagline">{mt.tagline}</p>

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
              <section aria-label={c.featuresHead}>
                <h3 className="module-feat-head">{c.featuresHead}</h3>
                <div className="mfeat-grid">
                  {m.features.map((f) => (
                    <article className="mfeat-card" key={f.icon}>
                      <span className="mfeat-icon">{FEATURE_ICONS[f.icon]}</span>
                      <h4 className="mfeat-title">{f.i18n[locale].title}</h4>
                      <p className="mfeat-desc">{f.i18n[locale].desc}</p>
                    </article>
                  ))}
                </div>
              </section>
            </section>
          )
        })}
      </div>
    </main>
  )
}
