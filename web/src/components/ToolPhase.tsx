/**
 * REQ-174 对话执行过程展示优化（ZCode 式单行/收起）→ REQ-317 树形化升级 → REQ-323 叶级收放：
 * - buildGroup：把连续的 tool.call / tool.result 事件流按 tool_call_id 栈式配对成**调用树**——
 *   挂起调用期间到达的新调用（典型=agent_as_tool 委派成员后成员内部的工具调用）成为其子节点；
 *   无主结果独立成根节点（中断残留语义不变），无 id 时回退同栈同名配对（存量事件兼容）；
 * - REQ-323 叶级收放：运行中**活跃叶子节点**（栈顶挂起调用）自动展开执行步骤信息（入参/结果），
 *   叶子完成（结果到达、栈顶回退到上级分支）即自动收起，焦点随执行推进；用户手动开合优先于自动态；
 * - ToolPhaseBlock：live（流式中）树展开、运行节点高亮；结束后**自动收起**为单行摘要
 *   （只剩最上层枝干），点击可重新展开整树回看（REQ-317 语义保持）；
 * - ToolLine：入参摘要**同一行内横向滚动**展示（不换行；摘要上限放宽，不再截到不可读），
 *   委派徽标（REQ-311 A5）保留。
 */
import { useState } from 'react'
import type { CSSProperties } from 'react'
import './tool-phase.css'

/** 一次工具调用（call/result 合并后） */
export interface ToolCallRow {
  name: string
  source?: string
  callId?: string
  args?: string
  argsSummary: string
  result?: string
  durationMs?: number
  running: boolean // 无 result：流式中=运行中；历史中=被中断未回结果
  err: boolean
}

/** 调用树节点：children=本调用挂起期间发生的嵌套调用（委派成员内部工具等） */
export interface ToolCallNode {
  row: ToolCallRow
  children: ToolCallNode[]
}

/** 归组后的执行过程块（挂到 items 流中替代原 N 张事件卡） */
export interface ToolPhaseGroup {
  kind: '__tool_phase'
  tree: ToolCallNode[]
  calls: ToolCallRow[] // 全部节点平铺（计数/委派聚合用）
  totalMs: number
  live: boolean // 流式进行中（展开）；结束后 false（收起）
  subDepth: number
  activeLeafCallId?: string // REQ-323：运行中活跃叶子（栈顶挂起调用）——其执行细节自动展开
}

/** 来源分类（与 ChatWindow.eventSource 同口径的最小副本——避免环导入） */
function srcClassOf(source?: string, name = ''): string {
  const s = source ?? ''
  if (s.startsWith('skill:')) return 'skill'
  if (s.startsWith('mcp:')) return 'mcp'
  if (s.startsWith('ontology:')) return 'onto'
  if (s.startsWith('agent:')) return 'subagent'
  if (s === 'builtin') return 'builtin'
  if (name.startsWith('onto_')) return 'onto'
  if (name.startsWith('mcp_') || name.includes('__')) return 'mcp'
  return 'builtin'
}

/** 参数摘要：对象取前几组 k=v，标量直接截断。REQ-317：上限放宽——超出部分由同行横向滚动承载，不再截到不可读 */
function summarizeArgs(args: unknown): string {
  if (args == null || args === '') return ''
  let obj: unknown = args
  if (typeof args === 'string') {
    try {
      obj = JSON.parse(args)
    } catch {
      return truncate(String(args), 200)
    }
  }
  if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
    const parts: string[] = []
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      parts.push(`${k}=${typeof v === 'string' ? truncate(v, 80) : truncate(JSON.stringify(v), 80)}`)
      if (parts.length >= 4) break
    }
    return truncate(parts.join('  '), 200)
  }
  return truncate(JSON.stringify(obj), 200)
}

function truncate(s: string, n: number): string {
  const t = String(s).replace(/\s+/g, ' ').trim()
  return t.length > n ? t.slice(0, n) + '…' : t
}

const isToolEv = (t?: string) => t === 'tool.call' || t === 'tool.result'

/** 归组：一轮内的 tool.call/result 合并为**一棵调用树**（REQ-323 改制——此前任何非工具事件都切组，
 *  委派场景 subagent.enter/exit 恰在 tool.call 与 tool.result 之间到达，调用被劈进两组、
 *  委派节点永远挂起配不上结果）。现在仅用户/助手消息切分组（新提问=新一轮）；
 *  subagent/思考/model.step 等事件照常按原位透传渲染，同时进入配对上下文（buildGroup 跳过）。
 *  runActive=运行中（ChatWindow running 透传）：最新组在工具执行期间保持展开，叶级细节可见。 */
export function groupToolPhases(items: any[], runActive = false): any[] {
  const out: any[] = []
  let buf: any[] = []
  const flush = () => {
    if (buf.length) { out.push(buildGroup(buf, items, runActive)); buf = [] }
  }
  for (const it of items) {
    if (it.kind === 'event' && isToolEv(it.evType)) {
      buf.push(it)
      continue
    }
    if (it.kind === 'msg') flush()
    if (buf.length) buf.push(it) // 已开组的轮次内：非工具事件进配对上下文（不切组）+ 原位渲染
    out.push(it)
  }
  flush()
  return out
}

function makeNode(d: any): ToolCallNode {
  return {
    row: {
      name: String(d.tool_name ?? ''),
      source: d.source,
      callId: d.tool_call_id != null ? String(d.tool_call_id) : undefined,
      args: typeof d.arguments === 'string' ? d.arguments : d.arguments != null ? JSON.stringify(d.arguments) : undefined,
      argsSummary: summarizeArgs(d.arguments),
      running: true,
      err: false,
    },
    children: [],
  }
}

function closeNode(node: ToolCallNode, d: any): void {
  const content = typeof d.content === 'string' ? d.content : d.content != null ? JSON.stringify(d.content) : ''
  node.row.result = content
  node.row.durationMs = typeof d.duration_ms === 'number' ? d.duration_ms : undefined
  node.row.running = false
  node.row.err = /^Error|^错误|执行失败|QUERY_REJECTED|invalid/i.test(content)
}

function buildGroup(buf: any[], all: any[], runActive = false): ToolPhaseGroup {
  const roots: ToolCallNode[] = []
  const stack: ToolCallNode[] = [] // 挂起中的调用（栈顶=当前最内层）
  const openById = new Map<string, ToolCallNode>()

  for (const it of buf) {
    if (!isToolEv(it.evType)) continue // REQ-323：轮内吸收的非工具事件（subagent/思考等，组外原位渲染）不参与配对
    const d = it.evData ?? {}
    if (it.evType === 'tool.call') {
      const node = makeNode(d)
      const parent = stack[stack.length - 1]
      if (parent) parent.children.push(node)
      else roots.push(node)
      stack.push(node)
      if (node.row.callId) openById.set(node.row.callId, node)
    } else {
      // tool.result：优先 tool_call_id 定位；无 id 回退栈内同名挂起调用（存量事件兼容）；无主结果独立成行
      const id = d.tool_call_id != null ? String(d.tool_call_id) : undefined
      let node = (id && openById.get(id)) || undefined
      if (node) {
        openById.delete(id!)
        const idx = stack.lastIndexOf(node)
        if (idx >= 0) stack.splice(idx, 1)
      } else {
        node = [...stack].reverse().find((n) => n.row.name === String(d.tool_name ?? '') && n.row.running)
        if (node) stack.splice(stack.lastIndexOf(node), 1)
      }
      if (node) closeNode(node, d)
      else
        roots.push({
          row: {
            name: String(d.tool_name ?? ''),
            source: d.source,
            argsSummary: '',
            result: typeof d.content === 'string' ? d.content : d.content != null ? JSON.stringify(d.content) : '',
            durationMs: typeof d.duration_ms === 'number' ? d.duration_ms : undefined,
            running: false,
            err: /^Error|^错误|执行失败|QUERY_REJECTED|invalid/i.test(String(d.content ?? '')),
          },
          children: [],
        })
    }
  }
  // live：REQ-323 放宽——runActive（运行中）即最新组展开（工具执行数秒内叶级细节可见），
  // 兼走旧流式判定（末项=流式助手消息且本组后无更多工具事件）；
  // REQ-317 修正仍保留：组间插入的 model.step/思考卡不再是打断点（laterTool 才是）。
  // runActive 为 false（历史/空闲）时严格按旧口径，回放态不虚展开。
  const lastBuf = [...buf].reverse().find((b) => isToolEv(b.evType))
  const groupEndIdx = lastBuf ? all.lastIndexOf(lastBuf) : -1
  const runLive = all.length > 0 && all[all.length - 1]?.kind === 'msg' && all[all.length - 1]?.streaming === true
  const laterTool = all.slice(groupEndIdx + 1).some((m) => m.kind === 'event' && (isToolEv(m.evType) || m.evType === 'run.started'))
  const live = (runActive || runLive) && !laterTool

  const calls: ToolCallRow[] = []
  const walk = (n: ToolCallNode) => {
    calls.push(n.row)
    n.children.forEach(walk)
  }
  roots.forEach(walk)

  return {
    kind: '__tool_phase',
    tree: roots,
    calls,
    totalMs: calls.reduce((s, c) => s + (c.durationMs ?? 0), 0),
    live,
    subDepth: (buf.find((b) => isToolEv(b.evType))?.subDepth) ?? 0,
    // REQ-323：栈顶=当前最内层挂起调用（活跃叶子）——其执行细节自动展开，完成（出栈）即收起
    activeLeafCallId: live ? stack[stack.length - 1]?.row.callId : undefined,
  }
}

const fmtMs = (ms?: number) => (ms == null ? '' : ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`)

/** 单次调用行：入参摘要同行横向滚动；点击展开原始入参/结果。
 *  REQ-323：autoOpen=本行为运行中活跃叶子时自动展开执行步骤信息（入参/结果），完成即自动收起；
 *  用户手动开合后以手动状态优先（不再被自动态翻转）。 */
function ToolLine({ call, pending, autoOpen }: { call: ToolCallRow; pending?: boolean; autoOpen?: boolean }) {
  const [userOpen, setUserOpen] = useState<boolean | null>(null)
  const open = userOpen ?? !!autoOpen
  const cls = srcClassOf(call.source, call.name)
  const isLiveRunning = call.running && !pending
  const dotCls = isLiveRunning ? 'run' : pending ? 'pend' : call.err ? 'bad' : 'ok'
  const dotTxt = isLiveRunning ? '' : pending ? '–' : call.err ? '✕' : '✓'
  return (
    <div className={`tool-line src-${cls}${call.err ? ' err' : ''}${isLiveRunning ? ' running' : ''}`}>
      <div className="tool-line-head" onClick={() => setUserOpen(!open)} title={open ? '收起详情' : '展开入参与结果'}>
        <span className={`tool-dot ${dotCls}`}>{dotTxt}</span>
        {/* REQ-311 A5：委派徽标——source=agent:{id} 的调用即协调者对成员智能体的委派（function name=成员名） */}
        {cls === 'subagent' && <span className="tool-delegate">委派</span>}
        <span className="tool-name">{call.name}</span>
        {call.argsSummary && <span className="tool-args">{call.argsSummary}</span>}
        {call.durationMs != null && <span className="tool-dur">{fmtMs(call.durationMs)}</span>}
        {isLiveRunning && <span className="tool-running-hint">执行中…</span>}
        <span className={`tool-caret${open ? ' open' : ''}`} />
      </div>
      {open && (
        <pre className="tool-raw">
          {call.args ? `入参：${prettyJson(call.args)}\n` : ''}
          {call.result ? `结果：${prettyJson(call.result)}` : isLiveRunning ? '执行中…' : pending ? '（无结果：被中断）' : '（无结果）'}
        </pre>
      )}
    </div>
  )
}

/** 调用树节点渲染：子调用挂在父调用下的引导线容器内（树形层级一眼可读）；
 *  REQ-323：callId 作 key（流式重建时不丢行内手动开合状态），活跃叶子逐层透传 autoOpen */
function TreeNode({ node, pendingTree, activeLeafCallId }: { node: ToolCallNode; pendingTree?: boolean; activeLeafCallId?: string }) {
  return (
    <div className="tool-node">
      <ToolLine
        call={node.row}
        pending={pendingTree && node.row.running}
        autoOpen={node.row.callId != null && node.row.callId === activeLeafCallId}
      />
      {node.children.length > 0 && (
        <div className="tool-node-children">
          {node.children.map((ch, i) => (
            <TreeNode key={ch.row.callId ?? i} node={ch} pendingTree={pendingTree} activeLeafCallId={activeLeafCallId} />
          ))}
        </div>
      )}
    </div>
  )
}

function prettyJson(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}

/** REQ-311 A5：委派聚合统计——按成员归并 agent 来源调用的次数与耗时（「委派 成员A×2·3.4s」） */
function delegateSummary(group: ToolPhaseGroup): string {
  const byName = new Map<string, { n: number; ms: number }>()
  for (const c of group.calls) {
    if (srcClassOf(c.source, c.name) !== 'subagent') continue
    const e = byName.get(c.name) ?? { n: 0, ms: 0 }
    e.n++
    e.ms += c.durationMs ?? 0
    byName.set(c.name, e)
  }
  return [...byName.entries()].map(([name, v]) => `${name}×${v.n}${v.ms > 0 ? `·${fmtMs(v.ms)}` : ''}`).join('、')
}

/** 执行过程块：live 整树展开（运行节点高亮）；结束自动收起为单行摘要（只剩最上层枝干，点击展开整树回看） */
export function ToolPhaseBlock({ group }: { group: ToolPhaseGroup }) {
  const [open, setOpen] = useState(false)
  const okCount = group.calls.filter((c) => !c.err).length
  const running = group.calls.some((c) => c.running)
  // REQ-311 A5：委派聚合统计（摘要行尾「· 委派 成员A×2·3.4s、成员B×1·12.0s」）
  const delegates = delegateSummary(group)
  const delegateTxt = delegates ? ` · 委派 ${delegates}` : ''
  const summary = `执行过程 · ${group.calls.length} 次工具调用${group.totalMs > 0 ? ` · ${fmtMs(group.totalMs)}` : ''}${
    okCount < group.calls.length ? ` · ${group.calls.length - okCount} 失败` : ''
  }${delegateTxt}`
  if (group.live) {
    return (
      <div className={`tool-phase${running ? ' live' : ''}`} style={{ '--sub-depth': group.subDepth } as CSSProperties}>
        <div className="tool-phase-head live-head">
          <span className="tool-caret open" />
          <span className="tool-phase-title">{summary}{running ? ' · 进行中' : ''}</span>
        </div>
        <div className="tool-tree">
          {group.tree.map((n, i) => (
            <TreeNode key={n.row.callId ?? i} node={n} activeLeafCallId={group.activeLeafCallId} />
          ))}
        </div>
        {delegates && <div className="tool-phase-title tool-delegate-summary">委派统计：{delegates}</div>}
      </div>
    )
  }
  return (
    <div className="tool-phase" style={{ '--sub-depth': group.subDepth } as CSSProperties}>
      {!open && (
        <div className="tool-phase-head" onClick={() => setOpen(true)} title="展开调用树（含嵌套委派细节）">
          <span className="tool-caret" />
          <span className="tool-phase-title">{summary}</span>
        </div>
      )}
      {open && (
        <>
          <div className="tool-phase-head open" onClick={() => setOpen(false)} title="收起">
            <span className="tool-caret open" />
            <span className="tool-phase-title">{summary}</span>
          </div>
          <div className="tool-tree">
            {group.tree.map((n, i) => (
              <TreeNode key={i} node={n} pendingTree />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
