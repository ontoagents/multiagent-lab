// REQ-401/M197：伴生沉淀呈现辅助（自 ChatWindow.tsx 拆出——回放摘要行+悬浮胶囊；行为零变更）。
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { LoadingOutlined } from '@ant-design/icons'

/** REQ-330④：回放态伴生管线摘要行（REQ-323 的可折叠归组块退役——live 过程由悬浮胶囊承载，
 *  回放只留一行轻量摘要；全量过程卡归「调用轨迹」伴生分类与 AgentCompanionManage） */
export interface CompanionSummaryItem {
  kind: '__companion_summary'
  n: number
  cand: number
  ingested: number
  failed: boolean
  subDepth: number
}

export const isCompanionPipelineEv = (it: any) =>
  it.kind === 'event' && typeof it.evType === 'string' && it.evType.startsWith('companion.') && it.evType !== 'companion.hit'

function makeCompanionSummary(events: any[]): CompanionSummaryItem {
  const terminal = events.find((e) => e.evType === 'companion.extract' && (e.evData?.phase === 'done' || e.evData?.phase === 'error'))
  const cand = events.reduce((s, e) => s + (e.evType === 'companion.candidates' ? Number(e.evData?.count ?? 0) : 0), 0)
  return {
    kind: '__companion_summary',
    n: events.length,
    cand,
    ingested: Number(terminal?.evData?.auto_ingested ?? 0),
    failed: terminal?.evData?.phase === 'error',
    subDepth: events[0]?.subDepth ?? 0,
  }
}

/** REQ-330④：回放流中连续伴生管线事件折为一行摘要（占首事件位；companion.hit 不在此列） */
export function summarizeCompanionPhases(items: any[]): any[] {
  const out: any[] = []
  let buf: any[] = []
  for (const it of items) {
    if (isCompanionPipelineEv(it)) {
      buf.push(it)
      continue
    }
    if (buf.length) {
      out.push(makeCompanionSummary(buf))
      buf = []
    }
    out.push(it)
  }
  if (buf.length) out.push(makeCompanionSummary(buf))
  return out
}

/** REQ-330②：伴生沉淀悬浮胶囊（聊天流右上角，与 chat-stop 同列对齐；旁路过程与本职工作事件解耦）。
 *  处理中=展开态胶囊增量摘要（候选 N · 自动入图 M 随事件到达递增）；终态定格 6s 后淡出为小徽标；
 *  点击随时展开/收起明细面板（管线过程卡逐张回看，候选卡自带的明细展开一并可用）。 */
export function CompanionCapsule({ events, renderEvent }: { events: any[]; renderEvent: (it: any, i: number) => ReactNode }) {
  const terminal = events.find((e) => e.evType === 'companion.extract' && (e.evData?.phase === 'done' || e.evData?.phase === 'error'))
  const failed = terminal?.evData?.phase === 'error'
  const processing = !terminal
  const [expanded, setExpanded] = useState(false)
  const [minimized, setMinimized] = useState(false)
  useEffect(() => {
    if (!terminal) {
      setMinimized(false)
      return
    }
    const t = window.setTimeout(() => setMinimized(true), 6000) // 终态定格 6s 后淡出为小徽标
    return () => window.clearTimeout(t)
  }, [terminal])
  const cand = events.reduce((s, e) => s + (e.evType === 'companion.candidates' ? Number(e.evData?.count ?? 0) : 0), 0)
  const ingested = Number(terminal?.evData?.auto_ingested ?? 0)
  const statTxt = `${cand > 0 ? `候选 ${cand}` : ''}${cand > 0 && ingested > 0 ? ' · ' : ''}${ingested > 0 ? `自动入图 ${ingested}` : ''}`
  const label = processing
    ? `伴生沉淀 · 处理中${statTxt ? ` · ${statTxt}` : ''}`
    : failed
      ? `伴生沉淀失败${statTxt ? ` · ${statTxt}` : ''}`
      : `伴生沉淀完成${statTxt ? ` · ${statTxt}` : ''}`
  return (
    <div className={`companion-capsule${minimized && !expanded ? ' mini' : ''}${failed ? ' failed' : ''}`} data-testid="companion-capsule">
      <button
        type="button"
        className="companion-capsule-pill"
        data-testid="companion-capsule-pill"
        onClick={() => setExpanded((o) => !o)}
        title={expanded ? '收起伴生沉淀明细' : '展开伴生沉淀明细'}
      >
        {processing ? (
          <LoadingOutlined spin style={{ color: 'var(--ant-color-primary, #4f46e5)' }} />
        ) : (
          <span className={`companion-capsule-mark${failed ? ' bad' : ''}`}>{failed ? '✕' : '✓'}</span>
        )}
        {!minimized && <span className="companion-capsule-label">{label}</span>}
      </button>
      {expanded && (
        <div className="companion-capsule-panel" data-testid="companion-capsule-panel">
          <div className="companion-capsule-head">
            <span className="companion-capsule-title">{label}</span>
            <button type="button" className="companion-capsule-close" onClick={() => { setExpanded(false); setMinimized(true) }} aria-label="收起">✕</button>
          </div>
          <div className="companion-capsule-body">
            {events.map((e, i) => renderEvent(e, i))}
          </div>
        </div>
      )}
    </div>
  )
}
