import { useEffect, useState } from 'react'
import {
  ApartmentOutlined,
  AimOutlined,
  ApiOutlined,
  AppstoreOutlined,
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
  ImportOutlined,
  KeyOutlined,
  LayoutOutlined,
  LockOutlined,
  NodeIndexOutlined,
  ProfileOutlined,
  RightOutlined,
  RocketOutlined,
  RobotOutlined,
  SearchOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  SolutionOutlined,
  SyncOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import { MODULES, moduleText } from '../content'
import { useCopy, useLocale } from '../i18n'

// ---------------------------------------------------------------------------
// 功能模块页（REQ-238 立项；REQ-364 去文档化重写；单页纵排迭代；REQ-374 左侧竖向导航
// +内容丰富）：
// 导航形态：v1 每模块一页+页底 prev/next 翻页器（入口藏页底难发现）→ v2 单页纵排+顶部
// 横向吸顶条 → v3 左侧 sticky 竖列 → v4（同日回修）**左侧悬浮导航**：fixed 悬浮视口左缘、
// 不占内容宽度（内容恢复通栏），默认仅 7 枚圆点、鼠标悬停展开名称（CSS group-hover 展开
// max-width+opacity）；窄屏（≤900px）回落为顶部横向滚动条（名称常显，触屏无 hover）。
// 滚动高亮沿用 IntersectionObserver；页内跳转 scrollIntoView 不写 hash（与 hash 路由冲突，
// Implementation 页同款先例）；旧深链 #/module/<key> 挂载即定位对应区块。
// 内容丰富（REQ-374）：每模块新增「典型流程」步骤条（3~5 步动线，对齐平台真实动线，
// content.ts 数据单源）与「继续了解」站内延伸链接（在线演示/实现与原理/部署/架构/生态）。
// 内容形态沿 REQ-364：速览要点 + 图标功能卡网格，零文档链接；事实口径以
// platform-knowledge 导读档为单源（content.ts 头注）；英文侧双语（REQ-360）。
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
  ImportOutlined: <ImportOutlined />,
  KeyOutlined: <KeyOutlined />,
  LayoutOutlined: <LayoutOutlined />,
  LockOutlined: <LockOutlined />,
  NodeIndexOutlined: <NodeIndexOutlined />,
  ProfileOutlined: <ProfileOutlined />,
  AppstoreOutlined: <AppstoreOutlined />,
  RocketOutlined: <RocketOutlined />,
  RobotOutlined: <RobotOutlined />,
  SearchOutlined: <SearchOutlined />,
  SafetyCertificateOutlined: <SafetyCertificateOutlined />,
  SafetyOutlined: <SafetyOutlined />,
  SolutionOutlined: <SolutionOutlined />,
  SyncOutlined: <SyncOutlined />,
  TeamOutlined: <TeamOutlined />,
  ThunderboltOutlined: <ThunderboltOutlined />,
  ToolOutlined: <ToolOutlined />,
}

const COPY = {
  zh: {
    crumbsHome: '首页',
    title: '功能模块',
    tagline: '全部模块一页纵览——左侧导航直达对应区块，或直接向下滚动逐一了解。',
    navAria: '模块导航',
    briefAria: '模块速览',
    flowHead: '典型流程',
    flowAria: '典型使用流程',
    featuresHead: '核心能力',
    linksHead: '继续了解',
  },
  en: {
    crumbsHome: 'Home',
    title: 'Modules',
    tagline: 'Every module on one page — use the left rail to jump, or simply scroll.',
    navAria: 'Module navigation',
    briefAria: 'Module at a glance',
    flowHead: 'Typical flow',
    flowAria: 'Typical usage flow',
    featuresHead: 'Core capabilities',
    linksHead: 'Keep exploring',
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

  // 滚动高亮：视口带内取「最贴近带起点」的区块为当前——已被滚过、尾部仍留在带内的
  // 区块以很负的 top 在场，按几何最小 top 会错误胜出，故按到带起点（≈rootMargin top
  // 100px）的距离最小者胜
  useEffect(() => {
    const obs = new IntersectionObserver(
      (entries) => {
        let best: { id: string; dist: number } | null = null
        for (const e of entries) {
          if (!e.isIntersecting) continue
          const dist = Math.abs(e.boundingClientRect.top - 100)
          if (!best || dist < best.dist) best = { id: String(e.target.id).replace('module-', ''), dist }
        }
        if (best) setActive(best.id)
      },
      // 顶栏 56px + 余量；下沿收窄到视口上部 45%，保证「正在阅读的区块」胜出
      { rootMargin: '-100px 0px -55% 0px' },
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

      <div className="page-inner modules-layout">
        {/* 左侧悬浮导航（REQ-374 回修）：fixed 悬浮于视口左缘、不占内容宽度——默认仅圆点，
            悬停展开名称；窄屏回落顶部横向条 */}
        <aside className="modules-rail-wrap">
          <nav className="modules-rail" aria-label={c.navAria}>
            {MODULES.map((m) => (
              <button
                key={m.key}
                type="button"
                className={`module-nav-item${active === m.key ? ' active' : ''}`}
                onClick={() => jump(m.key)}
              >
                <span className="rail-dot" aria-hidden="true" />
                <span className="rail-label">{moduleText(m, locale).title}</span>
              </button>
            ))}
          </nav>
        </aside>

        <div className="modules-main">
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

                {/* 典型流程（REQ-374）：编号步骤条，一眼看清本模块怎么用 */}
                <section className="mod-flow" aria-label={c.flowAria}>
                  <h3 className="mod-sub-head">{c.flowHead}</h3>
                  <ol className="flow-strip">
                    {mt.flow.map((step, i) => (
                      <li className="flow-step" key={step}>
                        <span className="flow-num">{i + 1}</span>
                        <span className="flow-text">{step}</span>
                      </li>
                    ))}
                  </ol>
                </section>

                {/* 核心能力：图标功能卡网格（REQ-364，替代原文档链接列表） */}
                <section aria-label={c.featuresHead}>
                  <h3 className="mod-sub-head">{c.featuresHead}</h3>
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

                {/* 继续了解（REQ-374）：站内延伸入口 */}
                {m.links && m.links.length > 0 && (
                  <section className="mod-links" aria-label={c.linksHead}>
                    <span className="mod-links-head">{c.linksHead}</span>
                    {m.links.map((l) => (
                      <a className="mod-link" key={l.href + l.i18n[locale]} href={l.href}>
                        {l.i18n[locale]}
                        <RightOutlined className="mod-link-arrow" />
                      </a>
                    ))}
                  </section>
                )}
              </section>
            )
          })}
        </div>
      </div>
    </main>
  )
}
