import { useEffect, useState } from 'react'
import { Tooltip } from 'antd'
import {
  ApartmentOutlined,
  AuditOutlined,
  BookOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
} from '@ant-design/icons'
import LearnPage from './LearnPage'
import BuildPage from './BuildPage'
import AssetsPage from './AssetsPage'
import RuntimePage from './RuntimePage'
import AuditPage from './AuditPage'

// ---------------------------------------------------------------------------
// 本体模块壳（D-O11 / REQ-104）：五栏位
//   学习中心（默认页）| 本体构建（六路径）| 本体资产（统一管理）| 本体运行（按引擎分组）
//   | 消费与审计（D-O15/REQ-110 第五栏）
// REQ-216⑨/D-O21 反转：伴生本体第六栏退役（回五栏）——六栏前提「伴生图非仓库资产」
// 被归属容器化推翻，栏职能被资产详情页「伴生候选」页签+可视化区伴生成长图吸收。
// 路由沿用 page 状态机：page='ontology' 内部以 sidebarKey 切换；
// 构建栏以 buildPath 驱动路径页，运行栏以 engineKey 驱动引擎分组页。
// REQ-326②：模块内导航自左侧边栏迁内容区顶部水平页签（全局导航左置①的必要配套，
// 避免双层左栏）；页签选中记忆（eino.onto.sidebar）与 onto-sidebar-change 事件动线
// 语义原样保留，仅承载形态演进；一句话定位收 Tooltip。
// ---------------------------------------------------------------------------

export type SidebarKey = 'learn' | 'build' | 'assets' | 'runtime' | 'audit'

const NAV: { key: SidebarKey; label: string; icon: React.ReactNode; desc: string }[] = [
  { key: 'learn', label: '学习中心', icon: <BookOutlined />, desc: '七阶段路径 · 方法论 · 任务卡' },
  { key: 'build', label: '本体构建', icon: <ApartmentOutlined />, desc: '六条构建路径' },
  { key: 'assets', label: '本体资产', icon: <DatabaseOutlined />, desc: '已构建本体统一管理 · 伴生候选' },
  { key: 'runtime', label: '本体运行', icon: <CloudServerOutlined />, desc: '按运行方式分组' },
  { key: 'audit', label: '消费与审计', icon: <AuditOutlined />, desc: '消费观测 · 决策审计 · PROV-O 溯源' },
]

/** 选中页签（模块内持久化，切走再切回不丢位置）；默认页 = 学习中心。
 *  REQ-216：旧 'companion' 值（第六栏）非法化回退默认页。REQ-326②：语义不变（五栏页签）。 */
export const ONTO_SIDEBAR_KEY = 'eino.onto.sidebar'

export function readSidebarKey(): SidebarKey {
  const v = localStorage.getItem(ONTO_SIDEBAR_KEY)
  return v === 'build' || v === 'assets' || v === 'runtime' || v === 'audit' ? v : 'learn'
}

export default function OntologyModule() {
  const [sidebarKey, setSidebarKey] = useState<SidebarKey>(readSidebarKey)

  useEffect(() => {
    const sync = () => setSidebarKey(readSidebarKey())
    window.addEventListener('onto-sidebar-change', sync)
    return () => window.removeEventListener('onto-sidebar-change', sync)
  }, [])

  const select = (key: SidebarKey) => {
    localStorage.setItem(ONTO_SIDEBAR_KEY, key)
    setSidebarKey(key)
  }

  const body =
    sidebarKey === 'learn' ? (
      <LearnPage />
    ) : sidebarKey === 'build' ? (
      <BuildPage />
    ) : sidebarKey === 'assets' ? (
      <AssetsPage />
    ) : sidebarKey === 'audit' ? (
      <AuditPage />
    ) : (
      <RuntimePage />
    )

  return (
    <div className="module-shell">
      <div className="module-tabs" role="tablist" aria-label="本体模块导航" data-testid="onto-module-tabs">
        {NAV.map((n) => (
          <Tooltip key={n.key} title={n.desc} placement="bottom" mouseEnterDelay={0.4}>
            <button
              type="button"
              role="tab"
              aria-selected={sidebarKey === n.key}
              aria-current={sidebarKey === n.key ? 'page' : undefined}
              className={`module-tab${sidebarKey === n.key ? ' active' : ''}`}
              onClick={() => select(n.key)}
            >
              <span className="module-tab-icon">{n.icon}</span>
              <span className="module-tab-label">{n.label}</span>
            </button>
          </Tooltip>
        ))}
      </div>
      <div className="module-body">{body}</div>
    </div>
  )
}
