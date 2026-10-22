import { lazy, Suspense, useEffect, useState } from 'react'
import { GithubOutlined } from '@ant-design/icons'
import { Spin } from 'antd'
import HomePage from './pages/Home'
import ModulePage from './pages/ModulePage'
import ArchitecturePage from './pages/Architecture'
import ImplementationPage from './pages/Implementation'
import DeployPage from './pages/Deploy'
import EcosystemPage from './pages/Ecosystem'
import { MODULES, REPO_SLUG, REPO_URL } from './content'
import { useLocale } from './i18n'
import type { L10n } from './locale'

// 演示页懒加载：three.js/react-force-graph 体量大（主包 ~3MB→首屏 ~1MB），首访演示页才拉取
const DemoPage = lazy(() => import('./pages/DemoPage'))

// ---------------------------------------------------------------------------
// 官网壳（REQ-238）：hash 路由（GitHub Pages 子路径静态托管免 404 配置）
//   #/                首页（产品预览 + 使用场景 + 全景导览）
//   #/module/<key>    功能模块（七模块单页纵排+吸顶锚点条；<key> 定位到对应区块，迭代批）
//   #/architecture    整体架构（三平面 + 双代理 SVG 总图）
//   #/implementation  实现与原理（REQ-364 由关键实现+实现原理合并；旧 #/principles 重定向至此）
//   #/ecosystem       开源生态（关联图谱 / 七类对比表 / 状态口径）
//   #/deploy          部署运行（桌面客户端 / 服务端部署 web 访问 / 本地运行）
//   #/demo            内置演示数据（种子本体可视化）
// 右上角语言切换 + GitHub 仓库链接常驻（REQ-360）。
// ---------------------------------------------------------------------------

function useHashRoute(): string {
  const [hash, setHash] = useState(() => window.location.hash || '#/')
  useEffect(() => {
    const on = () => setHash(window.location.hash || '#/')
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  return hash
}

/** 品牌标记：与平台 TopNav 同源的三节点网络 SVG */
function BrandMark({ size = 20 }: { size?: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      <path d="M12 6.6 6.4 15.5M12 6.6l5.6 8.9M7.4 16.2h9.2" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
      <circle cx="12" cy="5.2" r="2.5" fill="currentColor" />
      <circle cx="5.6" cy="16.6" r="2.2" fill="currentColor" opacity="0.86" />
      <circle cx="18.4" cy="16.6" r="2.2" fill="currentColor" opacity="0.86" />
    </svg>
  )
}

const NAVS: { hash: string; label: L10n<string>; match: (h: string) => boolean }[] = [
  { hash: '#/', label: { zh: '首页', en: 'Home' }, match: (h) => h === '#/' || h === '' },
  { hash: '#/module/agents', label: { zh: '功能模块', en: 'Modules' }, match: (h) => h.startsWith('#/module/') },
  { hash: '#/architecture', label: { zh: '整体架构', en: 'Architecture' }, match: (h) => h.startsWith('#/architecture') },
  { hash: '#/implementation', label: { zh: '实现与原理', en: 'How it works' }, match: (h) => h.startsWith('#/implementation') || h.startsWith('#/principles') },
  { hash: '#/ecosystem', label: { zh: '开源生态', en: 'Ecosystem' }, match: (h) => h.startsWith('#/ecosystem') },
  { hash: '#/deploy', label: { zh: '部署运行', en: 'Deploy' }, match: (h) => h.startsWith('#/deploy') },
  { hash: '#/demo', label: { zh: '在线演示', en: 'Demo' }, match: (h) => h.startsWith('#/demo') },
]

const COPY = {
  zh: {
    brandSub: '智能体构建平台',
    navAria: '主导航',
    ghAria: 'GitHub 仓库',
    langAria: '切换语言',
    notFoundTitle: '页面不存在',
    backHome: '返回首页',
    footerRepo: '代码仓库',
    footerLinks: { design: '整体设计', impl: '实现与原理', ecosystem: '开源生态', deploy: '部署运行', demo: '在线演示' },
  },
  en: {
    brandSub: 'Agent Construction Platform',
    navAria: 'Main navigation',
    ghAria: 'GitHub repository',
    langAria: 'Switch language',
    notFoundTitle: 'Page not found',
    backHome: 'Back to home',
    footerRepo: 'Source',
    footerLinks: { design: 'Overall Design', impl: 'How it works', ecosystem: 'Ecosystem', deploy: 'Deploy', demo: 'Demo' },
  },
}

function Body({ route }: { route: string }) {
  const moduleMatch = route.match(/^#\/module\/([\w-]+)/)
  if (moduleMatch) {
    // 迭代批：模块页单页纵排——#/module/<key> 定位到对应模块区块（旧深链/模块卡/页脚互引兼容）
    const mod = MODULES.find((m) => m.key === moduleMatch[1])
    return mod ? <ModulePage initial={mod.key} /> : <NotFound />
  }
  if (route.startsWith('#/architecture')) return <ArchitecturePage />
  // REQ-364：关键实现 + 实现原理合并为一页；旧 #/principles 链接重定向
  if (route.startsWith('#/implementation') || route.startsWith('#/principles')) return <ImplementationPage />
  if (route.startsWith('#/ecosystem')) return <EcosystemPage />
  if (route.startsWith('#/deploy')) return <DeployPage />
  if (route.startsWith('#/demo'))
    return (
      <Suspense
        fallback={
          <main className="page">
            <div className="page-inner" style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
              <Spin size="large" />
            </div>
          </main>
        }
      >
        <DemoPage />
      </Suspense>
    )
  return <HomePage />
}

function NotFound() {
  const { locale } = useLocale()
  const c = COPY[locale]
  return (
    <main className="page">
      <div className="page-inner">
        <h1>{c.notFoundTitle}</h1>
        <p>
          <a href="#/">{c.backHome}</a>
        </p>
      </div>
    </main>
  )
}

export default function App() {
  const route = useHashRoute()
  const { locale, setLocale } = useLocale()
  const c = COPY[locale]
  return (
    <div className="site">
      <header className="topnav">
        <a className="brand" href="#/">
          <span className="brand-mark">
            <BrandMark />
          </span>
          <span className="brand-text">
            <span className="brand-name">AgentLab</span>
            <span className="brand-sub">{c.brandSub}</span>
          </span>
        </a>
        <nav className="topnav-nav" aria-label={c.navAria}>
          {NAVS.map((n) => (
            <a key={n.hash} className={`nav-item${n.match(route) ? ' active' : ''}`} href={n.hash}>
              {n.label[locale]}
            </a>
          ))}
        </nav>
        <span className="topnav-spacer" />
        <button
          type="button"
          className="lang-toggle"
          onClick={() => setLocale(locale === 'zh' ? 'en' : 'zh')}
          aria-label={c.langAria}
          title="切换语言 / Switch language"
        >
          {locale === 'zh' ? 'EN' : '中文'}
        </button>
        <a className="gh-link" href={REPO_URL} target="_blank" rel="noreferrer" aria-label={c.ghAria} title={`github.com/${REPO_SLUG}`}>
          <GithubOutlined />
          <span className="gh-text">GitHub</span>
        </a>
      </header>

      <Body route={route} />

      <footer className="footer">
        <div className="footer-inner">
          <span>
            <BrandMark size={14} /> AgentLab · {c.brandSub} — Apache License 2.0
          </span>
          <span className="footer-links">
            <a href={REPO_URL} target="_blank" rel="noreferrer">
              {c.footerRepo}
            </a>
            {' · '}
            <a href="#/module/design">{c.footerLinks.design}</a>
            {' · '}
            <a href="#/implementation">{c.footerLinks.impl}</a>
            {' · '}
            <a href="#/ecosystem">{c.footerLinks.ecosystem}</a>
            {' · '}
            <a href="#/deploy">{c.footerLinks.deploy}</a>
            {' · '}
            <a href="#/demo">{c.footerLinks.demo}</a>
          </span>
        </div>
      </footer>
    </div>
  )
}
