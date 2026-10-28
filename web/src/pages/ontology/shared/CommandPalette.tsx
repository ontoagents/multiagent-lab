import { useEffect, useMemo, useRef, useState } from 'react'
import { Input, Modal, Tag } from 'antd'
import { useOntologies } from './useOntoList'
import { deepLinkOnto } from './ontoRoute'
import { PATHS } from '../build/paths'

// ---------------------------------------------------------------------------
// REQ-469/M269 键盘效率面：Mod+K 命令面板——五栏/构建路径/引擎/本体清单模糊跳转。
// 跳转动作统一走 deepLinkOnto（hash 通道 REQ-470 + localStorage 记忆 + 事件动线，
// 已在栏内时经各页 onto-sidebar-change/popstate/hashchange 监听即时应用）。
// ---------------------------------------------------------------------------
type Cmd = { key: string; label: string; group: string; run: () => void }

export default function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { ontos } = useOntologies()
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const inputRef = useRef<any>(null)

  useEffect(() => {
    if (open) {
      setQ('')
      setIdx(0)
      setTimeout(() => inputRef.current?.focus(), 60)
    }
  }, [open])

  const cmds = useMemo<Cmd[]>(() => {
    const cols: Cmd[] = [
      { key: 'c-build', label: '前往：本体构建', group: '栏位', run: () => deepLinkOnto({ col: 'build', path: 'home' }) },
      { key: 'c-assets', label: '前往：本体资产', group: '栏位', run: () => deepLinkOnto({ col: 'assets' }) },
      { key: 'c-runtime', label: '前往：本体运行', group: '栏位', run: () => deepLinkOnto({ col: 'runtime' }) },
      { key: 'c-audit', label: '前往：消费与审计', group: '栏位', run: () => deepLinkOnto({ col: 'audit' }) },
      { key: 'c-learn', label: '前往：学习中心', group: '栏位', run: () => deepLinkOnto({ col: 'learn' }) },
    ]
    const paths: Cmd[] = PATHS.map((p) => ({
      key: `p-${p.key}`,
      label: `构建路径：${p.label}`,
      group: '构建',
      run: () => deepLinkOnto({ col: 'build', path: p.key }),
    }))
    const engines: Cmd[] = (
      [
        ['oxigraph', 'Oxigraph'],
        ['fuseki', 'Fuseki'],
        ['cayley', 'Cayley'],
      ] as const
    ).map(([k, n]) => ({
      key: `e-${k}`,
      label: `运行引擎：${n}`,
      group: '运行',
      run: () => deepLinkOnto({ col: 'runtime', engine: k }, [['eino.onto.engineKey', k]]),
    }))
    const assets: Cmd[] = ontos.slice(0, 100).map((o) => ({
      key: `a-${o.id}`,
      label: `打开本体：${o.name}`,
      group: '资产',
      run: () => deepLinkOnto({ col: 'assets', assetId: o.id }, [['eino.onto.asset.focus', o.id]]),
    }))
    return [...cols, ...paths, ...engines, ...assets]
  }, [ontos])

  const ql = q.trim().toLowerCase()
  const hits = (ql ? cmds.filter((c) => c.label.toLowerCase().includes(ql)) : cmds).slice(0, 30)

  const exec = (c?: Cmd) => {
    if (!c) return
    onClose()
    // 延后一帧执行——先关面板避免焦点/浮层与新视图竞态
    setTimeout(() => c.run(), 30)
  }

  return (
    <Modal open={open} onCancel={onClose} footer={null} closable={false} width={540} title="命令面板（跳转）" data-testid="onto-command-palette">
      <Input
        ref={inputRef}
        allowClear
        placeholder="搜索栏位 / 构建路径 / 引擎 / 本体…"
        value={q}
        onChange={(e) => {
          setQ(e.target.value)
          setIdx(0)
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown') {
            e.preventDefault()
            setIdx((i) => Math.min(i + 1, hits.length - 1))
          } else if (e.key === 'ArrowUp') {
            e.preventDefault()
            setIdx((i) => Math.max(i - 1, 0))
          } else if (e.key === 'Enter') {
            e.preventDefault()
            exec(hits[idx])
          } else if (e.key === 'Escape') {
            e.preventDefault()
            onClose()
          }
        }}
        data-testid="onto-palette-input"
      />
      <div style={{ marginTop: 8, maxHeight: '46vh', overflowY: 'auto' }}>
        {hits.map((c, i) => (
          <div
            key={c.key}
            onClick={() => exec(c)}
            onMouseEnter={() => setIdx(i)}
            data-testid="onto-palette-item"
            style={{
              padding: '6px 10px',
              borderRadius: 6,
              cursor: 'pointer',
              background: i === idx ? 'var(--c-bg-soft, #eef1f8)' : undefined,
              display: 'flex',
              gap: 8,
              alignItems: 'center',
            }}
          >
            <Tag style={{ margin: 0 }}>{c.group}</Tag>
            <span style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.label}</span>
          </div>
        ))}
        {hits.length === 0 && <div style={{ padding: 12, color: 'var(--c-ink-3)', fontSize: 12 }}>无匹配命令</div>}
      </div>
      <div style={{ marginTop: 8, fontSize: 11, color: 'var(--c-ink-3)' }}>↑↓ 选择 · Enter 跳转 · Esc 关闭（REQ-469）</div>
    </Modal>
  )
}
