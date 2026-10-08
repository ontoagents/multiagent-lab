// GraphPaneShell 图形画板外壳（REQ-276）：包住 onto-flow-split 画板，其下沿提供拖拽
// 手柄调整画板高度（320px~85vh，localStorage `eino.onto.flow.height` 记忆，双击复位默认 clamp）。
// children 保持原样（Splitter.onto-flow-split 自带默认高度，外壳仅在内联 height 时覆盖）。
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'

const HEIGHT_KEY = 'eino.onto.flow.height'
const MIN_HEIGHT = 320

function clampHeight(px: number): number {
  return Math.min(Math.round(window.innerHeight * 0.85), Math.max(MIN_HEIGHT, Math.round(px)))
}

export default function GraphPaneShell({ children }: { children: ReactNode }) {
  const [height, setHeight] = useState<number | null>(null)
  const shellRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef(false)

  useEffect(() => {
    const saved = Number(localStorage.getItem(HEIGHT_KEY))
    if (saved >= MIN_HEIGHT && saved <= Math.round(window.innerHeight * 0.85)) setHeight(saved)
  }, [])

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault()
    dragRef.current = true
    e.currentTarget.setPointerCapture(e.pointerId)
    document.body.style.cursor = 'row-resize'
    document.body.style.userSelect = 'none'
  }, [])
  const onPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current || !shellRef.current) return
    const top = shellRef.current.getBoundingClientRect().top
    setHeight(clampHeight(e.clientY - top))
  }, [])
  const endDrag = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current) return
    dragRef.current = false
    e.currentTarget.releasePointerCapture(e.pointerId)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    setHeight((h) => {
      if (h != null) localStorage.setItem(HEIGHT_KEY, String(h))
      return h
    })
  }, [])
  const reset = useCallback(() => {
    localStorage.removeItem(HEIGHT_KEY)
    setHeight(null)
  }, [])

  return (
    <div>
      <div ref={shellRef} className={height ? 'graph-pane-override' : undefined} style={height ? { height } : undefined}>
        {children}
      </div>
      <div
        className="onto-flow-vresize"
        role="separator"
        aria-label="拖动调整画板高度（双击复位）"
        title="拖动调整画板高度（双击复位默认）"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onDoubleClick={reset}
      />
    </div>
  )
}
