import type { ReactNode } from 'react'
import { Tooltip } from 'antd'
import {
  ApartmentOutlined,
  BookOutlined,
  DatabaseOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  ProjectOutlined,
  RobotOutlined,
  SettingOutlined,
  SunOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { readTheme, THEME_EVENT, toggleTheme } from '../lib/theme'
import { useUI, type PageKey } from '../store/ui'

// ---------------------------------------------------------------------------
// REQ-326①：全局导航自顶部横排（TopNav，已退役）迁左侧可收缩侧边栏——
// web/客户端同一 SPA（REQ-321/324 WebView 直载 127.0.0.1:8080），客户端窗口可缩，
// 顶部横排导航不可收缩、窄窗退化最重；左置后全站收敛「左全局导航（可收）+ 模块内容区」。
//   展开态：品牌区 + 六模块（图标 + 名称 + 一句话定位）；
//   收起态：48px 图标列（沿 CollapsedRail 基建形态，Tooltip 右置）；
//   底部工具区：主题切换 + 设置（VS Code 齿轮惯例，原顶栏右端入口迁此）；
//   窄窗自适应：<1080px 强制收起（transient 不写记忆，宽窗恢复用户偏好）。
//   REQ-111 五项导航组织语义不变（六入口 + 设置同一批），仅承载形态演进。
// ---------------------------------------------------------------------------

const PAGES: { key: PageKey; label: string; icon: ReactNode; desc: string }[] = [
  { key: 'agents', label: '智能体', icon: <RobotOutlined />, desc: '对话 · 多智能体协作 · 知识技能本体增强' },
  { key: 'projects', label: '项目', icon: <ProjectOutlined />, desc: '项目工作区 · 本地目录 · 成员协作' },
  { key: 'ontology', label: '本体', icon: <ApartmentOutlined />, desc: '构建 → 资产 → 运行 → 消费审计闭环' },
  { key: 'knowledge', label: '知识库', icon: <DatabaseOutlined />, desc: 'RAG · GraphRAG · LLM Wiki' },
  { key: 'skills', label: '技能', icon: <ThunderboltOutlined />, desc: '配置级能力包 · 工具白名单' },
  { key: 'reference', label: '平台知识', icon: <BookOutlined />, desc: '调研 · 开发 · 产品设计文档库' },
]

const COLLAPSE_KEY = 'eino.globalnav.collapsed'

/** 品牌标记（多智能体联结点，内联 SVG 非 emoji；TopNav 同源迁入） */
function BrandMark() {
  return (
    <span className="brand-mark" aria-hidden="true">
      <svg viewBox="0 0 24 24" width="18" height="18" fill="none" aria-hidden="true">
        <path d="M12 6.6 6.4 15.5M12 6.6l5.6 8.9M7.4 16.2h9.2" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
        <circle cx="12" cy="5.2" r="2.5" fill="currentColor" />
        <circle cx="5.6" cy="16.6" r="2.2" fill="currentColor" opacity="0.86" />
        <circle cx="18.4" cy="16.6" r="2.2" fill="currentColor" opacity="0.86" />
      </svg>
    </span>
  )
}

/** REQ-240⑥：明暗主题切换（左栏底部工具区）。 */
function ThemeToggle({ collapsed }: { collapsed: boolean }) {
  const [t, setT] = useState(readTheme)
  useEffect(() => {
    const sync = () => setT(readTheme())
    window.addEventListener(THEME_EVENT, sync)
    return () => window.removeEventListener(THEME_EVENT, sync)
  }, [])
  const dark = t === 'dark'
  const btn = (
    <button
      type="button"
      className="gn-item gn-foot-btn"
      aria-label={dark ? '切换为亮色' : '切换为暗色'}
      title={dark ? '切换为亮色' : '切换为暗色'}
      onClick={toggleTheme}
    >
      <span className="gn-item-icon">{dark ? <SunOutlined /> : <MoonOutlined />}</span>
      {!collapsed && <span className="gn-item-text">{
        dark ? '亮色模式' : '暗色模式'
      }</span>}
    </button>
  )
  return collapsed ? <Tooltip title={dark ? '切换为亮色' : '切换为暗色'} placement="right">{btn}</Tooltip> : btn
}

export default function GlobalNav() {
  const { page, setPage } = useUI()
  // 收起态记忆（用户偏好）；窄窗强制收起为 transient（不写 localStorage）
  const [prefCollapsed, setPrefCollapsed] = useState(() => localStorage.getItem(COLLAPSE_KEY) === '1')
  const [narrow, setNarrow] = useState(() => window.innerWidth < 1080)

  useEffect(() => {
    const mq = window.matchMedia('(max-width: 1080px)')
    const sync = () => setNarrow(mq.matches)
    sync()
    mq.addEventListener('change', sync)
    return () => mq.removeEventListener('change', sync)
  }, [])

  const collapsed = prefCollapsed || narrow
  const toggle = () => {
    setPrefCollapsed((v) => {
      localStorage.setItem(COLLAPSE_KEY, v ? '0' : '1')
      return !v
    })
  }

  return (
    <aside
      className={`global-nav${collapsed ? ' collapsed' : ''}`}
      aria-label="全局导航"
      data-testid="global-nav"
      data-collapsed={collapsed ? '1' : '0'}
    >
      <div className="gn-brand">
        <BrandMark />
        {!collapsed && (
          <span className="brand-text">
            <span className="brand-name">AgentLab</span>
            <span className="brand-sub">智能体构建平台</span>
          </span>
        )}
      </div>

      {!narrow && (
        <Tooltip title={collapsed ? '展开导航' : '收起导航'} placement="right" mouseEnterDelay={0.3}>
          <button
            type="button"
            className="gn-item gn-fold"
            aria-label={collapsed ? '展开导航' : '收起导航'}
            onClick={toggle}
          >
            <span className="gn-item-icon">{collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}</span>
            {!collapsed && <span className="gn-item-text">收起导航</span>}
          </button>
        </Tooltip>
      )}

      <nav className="gn-nav" aria-label="主导航">
        {PAGES.map((p) => {
          const active = page === p.key
          const btn = (
            <button
              type="button"
              className={`gn-item${active ? ' active' : ''}`}
              aria-current={active ? 'page' : undefined}
              data-page={p.key}
              onClick={() => setPage(p.key)}
            >
              <span className="gn-item-icon">{p.icon}</span>
              {!collapsed && (
                <span className="gn-item-text">
                  <span className="gn-item-label">{p.label}</span>
                  <span className="gn-item-desc">{p.desc}</span>
                </span>
              )}
            </button>
          )
          return (
            <Tooltip key={p.key} title={collapsed ? p.label : undefined} placement="right" mouseEnterDelay={0.3}>
              {btn}
            </Tooltip>
          )
        })}
      </nav>

      <div className="gn-foot">
        <ThemeToggle collapsed={collapsed} />
        {(() => {
          const active = page === 'settings'
          const btn = (
            <button
              type="button"
              className={`gn-item${active ? ' active' : ''}`}
              aria-label="设置"
              aria-current={active ? 'page' : undefined}
              data-page="settings"
              title={collapsed ? '设置（模型管理等）' : undefined}
              onClick={() => setPage('settings')}
            >
              <span className="gn-item-icon">{<SettingOutlined />}</span>
              {!collapsed && <span className="gn-item-text">设置</span>}
            </button>
          )
          return collapsed ? (
            <Tooltip title="设置（模型管理等）" placement="right">{btn}</Tooltip>
          ) : (
            btn
          )
        })()}
      </div>
    </aside>
  )
}
