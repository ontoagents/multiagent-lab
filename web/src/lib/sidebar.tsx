import { useCallback, useState } from 'react'
import { Button, Tooltip } from 'antd'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'

/**
 * REQ-240 前端优化①：全站侧栏统一可收缩（本体模块两栏先行，其余模块左栏统一接入）。
 * hook 管理 collapsed 状态与 localStorage 记忆；Rail 渲染收起态图标列。
 */

export function useSidebarCollapse(storageKey: string, initial = false) {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(storageKey) === '1' || initial)
  const toggle = useCallback(() => {
    setCollapsed((v) => {
      localStorage.setItem(storageKey, v ? '0' : '1')
      return !v
    })
  }, [storageKey])
  return { collapsed, toggle }
}

/** 收起态图标列（展开钮 + 可选图标项；宽 48px 与本体模块一致）。 */
export function CollapsedRail({
  onExpand,
  items = [],
  ariaLabel = '侧栏（已收起）',
}: {
  onExpand: () => void
  items?: { key: string; icon: React.ReactNode; label: string; active?: boolean; onClick?: () => void }[]
  ariaLabel?: string
}) {
  return (
    <aside
      className="sidebar"
      style={{ width: 48, flex: '0 0 48px', maxWidth: 48, alignItems: 'center', padding: '8px 0', gap: 2 }}
      aria-label={ariaLabel}
    >
      <Button type="text" size="small" icon={<MenuUnfoldOutlined />} aria-label="展开侧栏" onClick={onExpand} style={{ marginBottom: 6 }} />
      {items.map((it) => (
        <Tooltip key={it.key} title={it.label} placement="right" mouseEnterDelay={0.3}>
          <button
            type="button"
            className={`onto-nav-item${it.active ? ' active' : ''}`}
            aria-label={it.label}
            aria-current={it.active ? 'page' : undefined}
            onClick={it.onClick}
            style={{ width: 40, justifyContent: 'center', padding: '8px 0' }}
          >
            <span className="onto-nav-icon">{it.icon}</span>
          </button>
        </Tooltip>
      ))}
    </aside>
  )
}

/** side-head 里的收起按钮（展开态用）。 */
export function SidebarCollapseButton({ onClick, label = '收起侧栏' }: { onClick: () => void; label?: string }) {
  return (
    <Button type="text" size="small" icon={<MenuFoldOutlined />} aria-label={label} onClick={onClick} />
  )
}
