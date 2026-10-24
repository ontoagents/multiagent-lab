// REQ-401/M197：todo 产物卡（自 ChatWindow.tsx 拆出；行为零变更）。
import { useState } from 'react'
import { CaretDownOutlined, CheckSquareOutlined } from '@ant-design/icons'

export type TodoItem = { title: string; status: string }

function parseTodos(json?: string | null): TodoItem[] {
  if (!json) return []
  try {
    const arr = JSON.parse(json)
    if (!Array.isArray(arr)) return []
    return arr.filter((it) => it && typeof it.title === 'string')
  } catch {
    return []
  }
}

const TODO_STATUS_META: Record<string, { icon: string; color: string; label: string }> = {
  pending: { icon: '○', color: 'var(--c-ink-3)', label: '待办' },
  in_progress: { icon: '◐', color: 'var(--c-brand, #4f7cff)', label: '进行中' },
  done: { icon: '●', color: '#52c41a', label: '完成' },
}

export function TodoCard({ todoJson }: { todoJson?: string | null }) {
  const todos = parseTodos(todoJson)
  // 报障迭代（2026-10-20）：超长清单在固定高 .msg-list flex 列里 min-height:auto 不收缩，
  // 卡片撑爆消息区且溢出部分压到发送区底下——列表限高滚动 + 头部点击折叠（全局记忆）双兜底
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('eino.todo.cardCollapsed') === '1')
  const toggle = () => {
    setCollapsed((c) => {
      localStorage.setItem('eino.todo.cardCollapsed', c ? '0' : '1')
      return !c
    })
  }
  if (todos.length === 0) return null
  const done = todos.filter((t) => t.status === 'done').length
  const active = todos.filter((t) => t.status === 'in_progress').length
  return (
    <div className="todo-card" data-testid="todo-card">
      <button type="button" className="todo-card-head" data-testid="todo-card-toggle" onClick={toggle} aria-expanded={!collapsed}>
        <CheckSquareOutlined style={{ color: 'var(--c-ink-2)' }} />
        <span>任务清单</span>
        <span className="todo-card-count">
          {done}/{todos.length} 完成{active > 0 ? ` · ${active} 进行中` : ''}
        </span>
        <CaretDownOutlined className={`todo-card-chevron${collapsed ? ' collapsed' : ''}`} aria-hidden="true" />
      </button>
      {!collapsed && (
        <ul className="todo-card-list">
        {todos.map((t, i) => {
          const meta = TODO_STATUS_META[t.status] ?? TODO_STATUS_META.pending
          return (
            <li key={i} className={`todo-card-item st-${TODO_STATUS_META[t.status] ? t.status : 'pending'}`}>
              <span className="todo-card-icon" style={{ color: meta.color }}>{meta.icon}</span>
              <span className="todo-card-title" style={t.status === 'done' ? { textDecoration: 'line-through', color: 'var(--c-ink-3)' } : undefined}>
                {t.title}
              </span>
              <span className="todo-card-status" style={{ color: meta.color }}>{meta.label}</span>
            </li>
          )
        })}
        </ul>
      )}
    </div>
  )
}