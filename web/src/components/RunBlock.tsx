/**
 * REQ-362 对话时间线按序交错与执行详情完成收起（ZCode 式对话流）：
 * - groupRunBlocks：把 run.started…run.finished/run.error 的整轮条目归为一个运行块。
 *   运行中/异常终态（stopped/interrupted/run.error/verify_failed）块内条目原样平铺
 *   （时间交错由 ChatWindow.applyRunEvent 的正文分段保证）；成功完成后块自动收起为
 *   一行任务小结（工具数/委派聚合/思考字数/耗时/tokens），最终回答外显其下；
 *   点击摘要重展完整时间线回看（REQ-317/323「执行完成自动收缩」从工具组级升到整轮级）。
 * - 最终回答=块内最后一段非空助手正文（done 时自块体提升到块外常显；重展时中间正文段
 *   按序留在块体内回看）。无过程细节的纯问答轮（无工具/思考/过程事件）不包块原样平铺。
 * - 回放同构：历史事件按时间排序后同样归组（message.delta 默认不入库沿 REQ-224⑤——
 *   回放块体内无分步正文，仅过程事件时间线+最终回答，诚实边界）。
 */
import { useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import './run-block.css'

export interface RunBlockSummary {
  tools: number
  failedTools: number
  delegates: string // 「成员A×2·3.4s」聚合（source=agent:* 的委派调用）
  reasoningChars: number
  elapsedMs?: number
  usage?: { prompt?: number; completion?: number; total?: number }
  finish?: string
  stopped?: boolean
  paused?: boolean // run.finished(reason=interrupted)
  err?: boolean // run.error
  warn?: boolean // verify_failed
}

/** 归组后的整轮运行块（挂到条目流中；live/ended 平铺渲染，done 收起渲染） */
export interface RunBlockItem {
  kind: '__run_block'
  status: 'live' | 'done' | 'ended'
  body: any[] // 块体（过程事件+工具组+中间正文段+终态卡；done 时不含已提升的回答）
  answer: any | null // 最终回答段（仅 done 提升外显）
  summary: RunBlockSummary
  subDepth: number
}

export const isRunBlock = (it: any): it is RunBlockItem => it?.kind === '__run_block'

const isTerminalEv = (t?: string) => t === 'run.finished' || t === 'run.error'

function buildSummary(body: any[], terminal: any): RunBlockSummary {
  let tools = 0
  let failedTools = 0
  let reasoningChars = 0
  const byName = new Map<string, { n: number; ms: number }>()
  const countCall = (name: string, source: string) => {
    tools++
    if (source.startsWith('agent:')) {
      const e = byName.get(name) ?? { n: 0, ms: 0 }
      e.n++
      byName.set(name, e)
    }
  }
  for (const b of body) {
    // 工具调用优先从归组块统计（groupToolPhases 先于本函数执行，裸 tool.call 已被 __tool_phase 组替换）
    if (b.kind === '__tool_phase') {
      for (const c of b.calls ?? []) {
        const src = String(c.source ?? '')
        countCall(String(c.name ?? '') || '成员', src)
        if (c.err) failedTools++
        if (src.startsWith('agent:') && typeof c.durationMs === 'number') {
          const e = byName.get(String(c.name ?? '')) ?? { n: 0, ms: 0 }
          e.ms += c.durationMs
          byName.set(String(c.name ?? ''), e)
        }
      }
      continue
    }
    if (b.kind !== 'event') continue
    if (b.evType === 'tool.call') {
      countCall(String(b.evData?.tool_name ?? '') || '成员', String(b.evData?.source ?? ''))
    }
    if (b.evType === 'tool.result') {
      const c = typeof b.evData?.content === 'string' ? b.evData.content : ''
      if (/^Error|^错误|执行失败|QUERY_REJECTED|invalid/i.test(c)) failedTools++
      const src = String(b.evData?.source ?? '')
      if (src.startsWith('agent:') && typeof b.evData?.duration_ms === 'number') {
        const name = String(b.evData?.tool_name ?? '') || '成员'
        const e = byName.get(name)
        if (e) e.ms += b.evData.duration_ms
      }
    }
    if (b.evType === 'reasoning') reasoningChars += b.reasoning?.length ?? 0
  }
  const delegates = [...byName.entries()].map(([name, v]) => `${name}×${v.n}${v.ms > 0 ? `·${fmtMs(v.ms)}` : ''}`).join('、')
  const s: RunBlockSummary = { tools, failedTools, delegates, reasoningChars }
  if (terminal?.evType === 'run.error') {
    s.err = true
    s.elapsedMs = typeof terminal.evData?.elapsed_ms === 'number' ? terminal.evData.elapsed_ms : undefined
    return s
  }
  const d = terminal?.evData ?? {}
  if (typeof d.elapsed_ms === 'number') s.elapsedMs = d.elapsed_ms
  const u = d.usage
  if (u && typeof u.total_tokens === 'number' && u.total_tokens > 0) {
    s.usage = { prompt: u.prompt_tokens, completion: u.completion_tokens, total: u.total_tokens }
  }
  if (d.finish_reason) s.finish = String(d.finish_reason)
  if (d.reason === 'stopped') s.stopped = true
  if (d.reason === 'interrupted') s.paused = true
  if (d.reason === 'verify_failed') s.warn = true
  return s
}

/**
 * 归组：run.started 起收集到 run.finished/run.error（含）为止；未闭合（防御：遇下一条
 * 用户消息/下一个 run.started）按原样平铺截断。工具归组（groupToolPhases）先于本函数执行，
 * __tool_phase 组作为不透明条目进块体。
 */
export function groupRunBlocks(items: any[], running = false): any[] {
  const out: any[] = []
  let i = 0
  while (i < items.length) {
    const it = items[i]
    if (!(it.kind === 'event' && it.evType === 'run.started')) {
      out.push(it)
      i++
      continue
    }
    const body: any[] = [it]
    let j = i + 1
    let terminal: any = null
    for (; j < items.length; j++) {
      const x = items[j]
      if ((x.kind === 'msg' && x.role === 'user') || (x.kind === 'event' && x.evType === 'run.started')) break // 未闭合防御
      body.push(x)
      if (x.kind === 'event' && isTerminalEv(x.evType)) {
        terminal = x
        j++
        break
      }
    }
    i = j
    const hasProcess = body.some((b) => b.kind === 'event' && b.evType !== 'run.started' && !isTerminalEv(b.evType))
    if (!hasProcess) {
      // 纯问答轮（无过程细节）：不包块
      out.push(...body)
      continue
    }
    let status: RunBlockItem['status']
    if (!terminal) status = running ? 'live' : 'ended'
    else if (terminal.evType === 'run.error') status = 'ended'
    else {
      const r = terminal.evData?.reason
      status = r === 'stopped' || r === 'interrupted' || r === 'verify_failed' ? 'ended' : 'done'
    }
    let answer: any = null
    let rest = body
    if (status === 'done') {
      let ai = -1
      for (let k = body.length - 1; k >= 0; k--) {
        const b = body[k]
        if (b.kind === 'msg' && b.role === 'assistant' && String(b.content ?? '').trim()) {
          ai = k
          break
        }
      }
      if (ai >= 0) {
        answer = body[ai]
        rest = body.filter((_, k) => k !== ai)
      }
    }
    out.push({
      kind: '__run_block',
      status,
      body: rest,
      answer,
      summary: buildSummary(body, terminal),
      subDepth: body.find((b) => b.subDepth != null)?.subDepth ?? 0,
    })
  }
  return out
}

function fmtMs(ms?: number): string {
  return ms == null ? '' : ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

function compactNum(n: number): string {
  return n >= 10000 ? `${(n / 1000).toFixed(0)}k` : n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}

function summaryText(s: RunBlockSummary): string {
  const parts: string[] = []
  if (s.tools > 0) parts.push(`${s.tools} 次工具调用${s.failedTools > 0 ? `（${s.failedTools} 失败）` : ''}`)
  if (s.delegates) parts.push(`委派 ${s.delegates}`)
  if (s.reasoningChars > 0) parts.push(`思考 ${compactNum(s.reasoningChars)} 字`)
  if (typeof s.elapsedMs === 'number') parts.push(fmtMs(s.elapsedMs))
  if (s.usage?.total) parts.push(`tokens ${s.usage.prompt ?? 0}+${s.usage.completion ?? 0}=${s.usage.total}`)
  if (s.finish && s.finish !== 'stop') parts.push(`finish=${s.finish}`)
  return parts.join(' · ')
}

function StatusMark({ s }: { s: RunBlockSummary }) {
  const cls = s.err ? 'bad' : s.warn || s.stopped ? 'warn' : 'ok'
  const glyph = s.err ? '✕' : s.warn ? '⚠' : s.stopped ? '⏹' : s.paused ? '⏸' : '✓'
  return (
    <span className={`run-block-mark ${cls}`} aria-hidden="true">
      {glyph}
    </span>
  )
}

/** 运行块渲染（仅 done 态被 listItems 选中：收起=一行小结，展开=块体按序回看） */
export function RunBlockView({ block, renderItem }: { block: RunBlockItem; renderItem: (it: any, key: string) => ReactNode }) {
  const [open, setOpen] = useState(false)
  const s = block.summary
  const statusTxt = s.err ? '运行失败' : s.warn ? '完成但验证未通过' : '任务完成'
  return (
    <div className="run-block" style={{ '--sub-depth': block.subDepth } as CSSProperties} data-testid="run-block">
      <div
        className={`run-block-head${open ? ' open' : ''}`}
        onClick={() => setOpen((o) => !o)}
        title={open ? '收起执行详情' : '展开执行详情（思考 / 工具调用 / 中间回答 · 按时间顺序）'}
        data-testid="run-block-head"
      >
        <StatusMark s={s} />
        <span className="run-block-title">
          {statusTxt}
          {summaryText(s) ? ` · ${summaryText(s)}` : ''}
        </span>
        <span className={`tool-caret${open ? ' open' : ''}`} />
      </div>
      {open && (
        <div className="run-block-body" data-testid="run-block-body">
          {block.body.map((it, i) => renderItem(it, String(i)))}
        </div>
      )}
    </div>
  )
}
