import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Drawer, Input, Segmented, Select, Space, Switch, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined, RollbackOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { RunEventDTO } from '../api/types'
import { describeEvent, levelGated } from './ChatWindow'
import EventReplayDrawer from './EventReplayDrawer'
import { DRAWER_SIZES, drawerSizeProps } from '../lib/layout'

/**
 * 调用轨迹面板（REQ-217①②④/M48；REQ-284/M89 时序优化）：
 * - 行类型：工具调用/模型步骤（仅 debug_persist 落库时可见，缺失诚实标注）/run.warning/
 *   审批挂起（run.interrupted）/subagent（缩进）/retrieval/artifact.saved/debug.cli/运行始末/
 *   Harness 审计（REQ-224）/伴生沉淀（REQ-281）；
 * - REQ-284① 流式输出归并：reasoning.delta/message.delta 逐 chunk 落库（M52⑤），逐条渲染
 *   即「一个 token 一行」——连续 chunk 归并为「思考段/正文段」单行（字数+时间跨度，思考段
 *   可展开全文），段边界=被其他事件打断处（天然保留时序结构）；
 * - REQ-284② 时序条：每运行顶部四泳道 waterfall（思考/正文/工具/事件标记），段按时长比例
 *   定位——一次 run 的「思考→工具→再思考→正文」时序一眼可读；
 * - 过滤：类型多选 / 运行选择 / 关键字 / 全部展开收起；显示设置与运行治理收编（④定案）；
 * - 「重放此运行」（②定案）：EventReplayDrawer 并入为面板内动作。
 */

const TYPE_GROUPS = [
  { value: 'tool', label: '工具调用', types: ['tool.call', 'tool.result'] },
  { value: 'delta', label: '流式输出', types: ['reasoning.delta', 'message.delta'] },
  { value: 'model', label: '模型步骤', types: ['model.step'] },
  { value: 'warning', label: '警告', types: ['run.warning'] },
  { value: 'approval', label: '审批挂起', types: ['run.interrupted'] },
  // REQ-224/M52（51 号 W1）：harness 事件域（结构化审计）
  { value: 'harness', label: 'Harness 审计', types: ['approval.granted', 'approval.denied', 'hook.denied', 'verify.completed', 'verify.failed', 'connector.degraded'] },
  { value: 'subagent', label: '子智能体', types: ['subagent.enter', 'subagent.exit'] },
  { value: 'retrieval', label: '知识召回', types: ['retrieval'] },
  // REQ-281：伴生沉淀过程事件域（抽取/候选/入图判定/拒绝镜像）
  { value: 'companion', label: '伴生沉淀', types: ['companion.extract', 'companion.candidates', 'companion.decision', 'companion.ingest', 'companion.reject'] },
  { value: 'artifact', label: '产物', types: ['artifact.saved'] },
  { value: 'cli', label: 'CLI 输出', types: ['debug.cli'] },
  { value: 'run', label: '运行始末', types: ['run.started', 'run.finished', 'run.error'] },
]
const typesOfGroups = (gs: string[]) => {
  const set = new Set<string>()
  for (const g of gs) for (const t of TYPE_GROUPS.find((x) => x.value === g)?.types ?? []) set.add(t)
  return set
}

const TYPE_COLOR: Record<string, string> = {
  'tool.call': 'blue', 'tool.result': 'geekblue', 'model.step': 'purple', 'run.warning': 'orange',
  'run.interrupted': 'gold', 'subagent.enter': 'cyan', 'subagent.exit': 'cyan', 'retrieval': 'green',
  'companion.extract': 'cyan', 'companion.candidates': 'cyan', 'companion.decision': 'gold',
  'companion.ingest': 'green', 'companion.reject': 'red',
  'artifact.saved': 'magenta', 'debug.cli': 'default', 'run.started': 'default', 'run.finished': 'green',
  'run.error': 'red', 'skill.loaded': 'default',
}

export default function TraceDrawer({
  conversationId,
  title,
  open,
  onClose,
  granularity,
  showReasoning,
  showRaw,
  debugLevel,
  debugPersist,
  toolApproval,
  running,
  patch,
  onShowRaw,
  onChangeDebugLevel,
  onPatchConv,
}: {
  conversationId: string
  title: string
  open: boolean
  onClose: () => void
  granularity: 'all' | 'key' | 'off'
  showReasoning: boolean
  showRaw: boolean
  debugLevel: number
  debugPersist: boolean
  toolApproval: string
  running: boolean
  patch: (p: { granularity?: 'all' | 'key' | 'off'; showReasoning?: boolean; debugPersist?: boolean }) => void
  onShowRaw: (v: boolean) => void
  onChangeDebugLevel: (n: number) => void
  onPatchConv: (p: { tool_approval: string }) => void
}) {
  const [events, setEvents] = useState<RunEventDTO[]>([])
  const [loading, setLoading] = useState(false)
  const [typeGroups, setTypeGroups] = useState<string[]>([])
  const [runFilter, setRunFilter] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [expandAll, setExpandAll] = useState(false)
  const [replayRun, setReplayRun] = useState<string | null>(null)
  // REQ-284②：时序条开关（默认开）
  const [showTimeline, setShowTimeline] = useState(true)

  const load = () => {
    if (!conversationId) return
    setLoading(true)
    api
      .listEvents(conversationId)
      .then((r) => setEvents(r ?? []))
      .catch(() => setEvents([]))
      .finally(() => setLoading(false))
  }
  useEffect(() => {
    if (open) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, conversationId])

  // 按运行分组（created_at 升序；run_id 空的散事件归「未分组」段）
  const runs = useMemo(() => {
    const map = new Map<string, RunEventDTO[]>()
    for (const e of events) {
      const k = e.run_id || '__none__'
      if (!map.has(k)) map.set(k, [])
      map.get(k)!.push(e)
    }
    return [...map.entries()].map(([runId, evs]) => ({
      runId,
      events: evs,
      allEvents: evs,
      startedAt: evs[0]?.created_at ?? '',
      finished: evs.find((e) => e.type === 'run.finished' || e.type === 'run.error'),
    }))
  }, [events])

  const hasDebugRecorded = events.some((e) => e.type === 'model.step')

  // 过滤链：运行 → 类型组 → 关键字 → 展示级别门控（与回放同口径）。
  // 注意：类型过滤会滤掉 delta（组外），列表随之收窄；时序条始终基于全量事件（概览不随过滤缩放）。
  const filteredRuns = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    const allow = typeGroups.length > 0 ? typesOfGroups(typeGroups) : null
    return runs
      .filter((r) => !runFilter || r.runId === runFilter)
      .map((r) => ({
        ...r,
        allEvents: r.events,
        events: r.events.filter((e) => {
          if (allow && !allow.has(e.type)) return false
          if (levelGated(e.type, debugLevel)) return false
          if (!kw) return true
          const desc = describeEvent(e.type, safeParse(e.data))
          return desc.text.toLowerCase().includes(kw) || (e.data ?? '').toLowerCase().includes(kw)
        }),
      }))
  }, [runs, runFilter, typeGroups, keyword, debugLevel])

  return (
    <Drawer
      title={`调用轨迹 · ${title}`}
      placement="right"
      {...drawerSizeProps('trace', DRAWER_SIZES.medium)}
      open={open}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space size={6}>
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={load}>刷新</Button>
        </Space>
      }
    >
      {/* 显示设置（REQ-217④ 收编自头部） */}
      <Typography.Text strong style={{ fontSize: 12 }}>显示设置（同时作用于会话流过程卡）</Typography.Text>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, margin: '6px 0 12px' }}>
        <div>
          <div style={{ marginBottom: 4 }}><Typography.Text style={{ fontSize: 12 }}>过程事件粒度</Typography.Text></div>
          <Segmented
            size="small"
            value={granularity}
            onChange={(v) => patch({ granularity: v as 'all' | 'key' | 'off' })}
            options={[{ value: 'all', label: '全部' }, { value: 'key', label: '关键' }, { value: 'off', label: '精简' }]}
          />
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block' }}>
            关键=工具调用与运行状态；精简=仅消息与结论
          </Typography.Text>
        </div>
        <Space size={24} wrap>
          <span>
            <Typography.Text style={{ fontSize: 12 }}>深度思考过程 </Typography.Text>
            <Switch size="small" checked={showReasoning} onChange={(v) => patch({ showReasoning: v })} />
          </span>
          <span>
            <Typography.Text style={{ fontSize: 12 }}>原始事件 JSON </Typography.Text>
            <Switch size="small" checked={showRaw} onChange={onShowRaw} />
          </span>
          <span>
            <Typography.Text style={{ fontSize: 12 }}>时序条 </Typography.Text>
            <Switch size="small" checked={showTimeline} onChange={setShowTimeline} />
          </span>
        </Space>
      </div>

      {/* 运行治理（行为配置归会话配置面） */}
      <Typography.Text strong style={{ fontSize: 12 }}>运行治理</Typography.Text>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, margin: '6px 0 12px' }}>
        <div>
          <Typography.Text style={{ fontSize: 12 }}>观测级别（仅影响之后的运行）</Typography.Text>
          <Select
            size="small"
            style={{ width: '100%', marginTop: 4 }}
            value={String(debugLevel)}
            disabled={running}
            onChange={(v) => onChangeDebugLevel(Number(v))}
            options={[
              { value: '0', label: '简洁（默认）' },
              { value: '1', label: '详细 · 装配快照/分步用量/工具耗时' },
              { value: '2', label: '调试 · 另附模型输入全文/工具 schema' },
            ]}
          />
        </div>
        <span>
          <Typography.Text style={{ fontSize: 12 }}>调试事件入库 </Typography.Text>
          <Switch size="small" checked={debugPersist} onChange={(v) => patch({ debugPersist: v })} />
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block' }}>
            开启后调试档细节随运行落库，供轨迹面板与回放查看（级别≥详细时产生）
          </Typography.Text>
          {debugLevel >= 1 && !debugPersist && (
            <Alert type="warning" showIcon style={{ marginTop: 6 }} title="当前级别≥详细，但入库关闭：调试细节不会留存到历史（REQ-149）" />
          )}
        </span>
        <div>
          <Typography.Text style={{ fontSize: 12 }}>工具调用审批（会话级覆盖）</Typography.Text>
          <Select
            size="small"
            style={{ width: '100%', marginTop: 4 }}
            value={toolApproval}
            onChange={(v) => onPatchConv({ tool_approval: v })}
            options={[
              { value: '', label: '跟随智能体配置（侧板 Harness 页签）' },
              { value: 'on', label: '本对话强制开启审批' },
              { value: 'off', label: '本对话关闭审批' },
            ]}
          />
        </div>
        {!hasDebugRecorded && (
          <Alert type="info" showIcon title="历史运行未见模型步骤事件（未开调试事件入库）——该类行不可用为诚实标注，非缺陷" />
        )}
      </div>

      {/* 过滤 */}
      <Typography.Text strong style={{ fontSize: 12 }}>过滤</Typography.Text>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, margin: '6px 0 10px' }}>
        <Select
          size="small"
          mode="multiple"
          allowClear
          placeholder="类型多选（空=全部）"
          value={typeGroups}
          onChange={setTypeGroups}
          options={TYPE_GROUPS.map((g) => ({ value: g.value, label: g.label }))}
        />
        {/* REQ-224/M52（51 号 W1）：harness 视角预置过滤器——治理面一键聚焦 */}
        <Space size={4} wrap style={{ marginBottom: 4 }}>
          <a
            style={{ fontSize: 11 }}
            onClick={() => setTypeGroups(['harness', 'approval', 'warning'])}
          >
            harness 视角
          </a>
          <a style={{ fontSize: 11 }} onClick={() => setTypeGroups([])}>全部</a>
        </Space>
        <Space size={6} wrap>
          <Select
            size="small"
            showSearch
            allowClear
            placeholder="运行选择（全部）"
            style={{ width: 220 }}
            value={runFilter || undefined}
            onChange={(v) => setRunFilter(v || '')}
            options={runs.filter((r) => r.runId !== '__none__').map((r, i) => ({
              value: r.runId,
              label: `运行 ${i + 1} · ${fmtTime(r.startedAt)}（${r.events.length} 事件）`,
            }))}
          />
          <Input size="small" placeholder="关键字过滤" style={{ width: 150 }} value={keyword} onChange={(e) => setKeyword(e.target.value)} allowClear />
          <Button size="small" onClick={() => setExpandAll(true)}>全部展开</Button>
          <Button size="small" onClick={() => setExpandAll(false)}>收起</Button>
        </Space>
      </div>

      {/* 时间线：按运行分组 */}
      {filteredRuns.length === 0 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>暂无事件（运行一轮后刷新，或调整过滤条件）</Typography.Text>}
      {filteredRuns.map((r, ri) => (
        <RunBlock key={r.runId} run={r} index={ri} showRaw={showRaw} expandAll={expandAll} showTimeline={showTimeline}
          onReplay={() => setReplayRun(r.runId)} />
      ))}

      {replayRun !== null && (
        <EventReplayDrawer
          conversationId={conversationId}
          title={title}
          level={debugLevel}
          runId={replayRun === '__none__' ? undefined : replayRun}
          open
          onClose={() => setReplayRun(null)}
        />
      )}
    </Drawer>
  )
}

function fmtTime(ts: string) {
  if (!ts) return '—'
  const d = new Date(ts)
  return isNaN(+d) ? ts : d.toLocaleString()
}

function safeParse(s?: string): any {
  if (!s) return undefined
  try { return JSON.parse(s) } catch { return undefined }
}

// ---------------------------------------------------------------------------
// REQ-284①②：流式输出归并 + 时序条
// ---------------------------------------------------------------------------

/** 展示段：event=普通事件原样；reasoning/message=连续 delta 归并段（段边界=被其他事件打断处） */
type Seg =
  | { kind: 'event'; e: RunEventDTO }
  | { kind: 'reasoning'; items: RunEventDTO[]; text: string; chars: number; start: RunEventDTO; end: RunEventDTO }
  | { kind: 'message'; items: RunEventDTO[]; chars: number; start: RunEventDTO; end: RunEventDTO }

/** 连续 delta 归并（治「一个 token 一行」）：相邻同类 delta 合为一段，其余事件切分段边界 */
function buildSegments(events: RunEventDTO[]): Seg[] {
  const segs: Seg[] = []
  let rBuf: RunEventDTO[] = []
  let mBuf: RunEventDTO[] = []
  const flush = () => {
    if (rBuf.length) {
      const text = rBuf.map((e) => String(safeParse(e.data)?.delta ?? '')).join('')
      segs.push({ kind: 'reasoning', items: rBuf, text, chars: text.length, start: rBuf[0], end: rBuf[rBuf.length - 1] })
      rBuf = []
    }
    if (mBuf.length) {
      const chars = mBuf.reduce((n, e) => n + String(safeParse(e.data)?.delta ?? '').length, 0)
      segs.push({ kind: 'message', items: mBuf, chars, start: mBuf[0], end: mBuf[mBuf.length - 1] })
      mBuf = []
    }
  }
  for (const e of events) {
    if (e.type === 'reasoning.delta') { rBuf.push(e); continue }
    if (e.type === 'message.delta') { mBuf.push(e); continue }
    flush()
    segs.push({ kind: 'event', e })
  }
  flush()
  return segs
}

const tsOf = (e?: RunEventDTO) => (e ? +new Date(e.created_at) : NaN)

/** 时序段（waterfall 用）：泳道 0=思考 1=正文 2=工具 3=事件标记 */
type TSegment = { lane: 0 | 1 | 2 | 3; s: number; e: number; color: string; tip: string }
type TMark = { lane: 3; at: number; color: string; tip: string }

/** 从事件序列提取时序段：思考/正文块、工具调用对（tool_call_id 配对）、挂起段、关键事件标记 */
function buildTimeline(events: RunEventDTO[]): { segs: TSegment[]; marks: TMark[]; t0: number; t1: number } {
  const t0 = tsOf(events[0])
  let t1 = tsOf(events[events.length - 1])
  if (!isFinite(t0) || !isFinite(t1)) return { segs: [], marks: [], t0: NaN, t1: NaN }
  if (t1 < t0) t1 = t0

  const segs: TSegment[] = []
  const marks: TMark[] = []
  let suspendStart: number | null = null
  // delta 块
  for (const seg of buildSegments(events)) {
    if (seg.kind === 'event') continue
    const s = tsOf(seg.start), e = tsOf(seg.end)
    if (!isFinite(s) || !isFinite(e)) continue
    segs.push(seg.kind === 'reasoning'
      ? { lane: 0, s, e, color: '#9254de', tip: `思考 · ${seg.chars} 字（${fmtDur(e - s)}，${seg.items.length} 段流式）` }
      : { lane: 1, s, e, color: '#1677ff', tip: `正文输出 · ${seg.chars} 字（${fmtDur(e - s)}）` })
  }
  // 工具对：tool_call_id 配对（call→result；result 无配对则零长点）
  const callAt = new Map<string, { t: number; name: string }>()
  let pending: { t: number; name: string } | null = null
  for (const e of events) {
    const d = safeParse(e.data)
    const t = tsOf(e)
    if (!isFinite(t)) continue
    if (e.type === 'tool.call') {
      const item = { t, name: String(d?.tool_name ?? '?') }
      if (d?.tool_call_id) callAt.set(String(d.tool_call_id), item)
      else pending = item
    } else if (e.type === 'tool.result') {
      const origin = (d?.tool_call_id && callAt.get(String(d.tool_call_id))) || pending
      const s = origin?.t ?? t
      const name = origin?.name ?? String(d?.tool_name ?? '?')
      if (d?.tool_call_id) callAt.delete(String(d.tool_call_id))
      pending = null
      segs.push({ lane: 2, s, e: t, color: '#13c2c2', tip: `工具 ${name} · ${fmtDur(t - s)}` })
      marks.push({ lane: 3, at: t, color: '#13c2c2', tip: `工具完成 ${name}` })
    } else if (e.type === 'run.interrupted') {
      // 挂起段：到下一个 run.started（恢复）或收尾；先记起点，画到暂定 t1，后续 resumed 覆盖
      marks.push({ lane: 3, at: t, color: '#faad14', tip: `审批/追问挂起${d?.kind ? `（${d.kind}）` : ''}` })
      suspendStart = t
    } else if (e.type === 'run.started' && d?.resumed && suspendStart != null) {
      segs.push({ lane: 3, s: suspendStart, e: t, color: '#faad14', tip: `人工处理间隔 · ${fmtDur(t - suspendStart)}` })
      suspendStart = null
    } else if (['approval.granted', 'approval.denied'].includes(e.type)) {
      marks.push({ lane: 3, at: t, color: e.type === 'approval.granted' ? '#52c41a' : '#ff4d4f', tip: `${e.type === 'approval.granted' ? '批准' : '拒绝'}（${d?.decision_source ?? 'manual'}）` })
    } else if (e.type === 'run.warning') {
      marks.push({ lane: 3, at: t, color: '#fa8c16', tip: `警告：${String(d?.message ?? '').slice(0, 60)}` })
    } else if (e.type === 'verify.failed' || e.type === 'verify.completed') {
      marks.push({ lane: 3, at: t, color: e.type === 'verify.failed' ? '#ff4d4f' : '#52c41a', tip: `验证背压 ${e.type === 'verify.failed' ? '失败' : '通过'}` })
    } else if (e.type === 'connector.degraded' || e.type === 'hook.denied') {
      marks.push({ lane: 3, at: t, color: '#ff4d4f', tip: describeEvent(e.type, d).text })
    }
  }
  if (suspendStart != null) segs.push({ lane: 3, s: suspendStart, e: t1, color: '#faad14', tip: '挂起中（尚未恢复）' })
  return { segs, marks, t0, t1 }
}

/** 时长人话（毫秒） */
function fmtDur(ms: number) {
  if (!isFinite(ms) || ms < 0) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m${Math.round((ms % 60000) / 1000)}s`
}

const LANE_LABEL = ['思考', '正文', '工具', '事件']

/** 四泳道时序条（REQ-284②）：段按时长比例定位，最小可见宽 0.5%；hover 显示时段与内容 */
function TimelineStrip({ events }: { events: RunEventDTO[] }) {
  const { segs, marks, t0, t1 } = useMemo(() => buildTimeline(events), [events])
  const T = t1 - t0
  if (!isFinite(t0) || T <= 0) return null
  const pct = (t: number) => Math.max(0, Math.min(100, ((t - t0) / T) * 100))
  return (
    <div style={{ margin: '4px 10px 8px', padding: '6px 8px 4px', border: '1px solid var(--c-border, #e3e6f0)', borderRadius: 6, background: 'var(--c-panel, #fff)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2, marginLeft: 22 }}>
        <Typography.Text type="secondary" style={{ fontSize: 10 }}>时序 · 总 {fmtDur(T)}（思考/正文/工具/事件）</Typography.Text>
        <span style={{ flex: 1 }} />
        <Typography.Text type="secondary" style={{ fontSize: 10 }}>{fmtTime(events[0].created_at)} → {fmtTime(events[events.length - 1].created_at)}</Typography.Text>
      </div>
      <div style={{ position: 'relative', height: 56, marginLeft: 22 }}>
        {[0, 1, 2, 3].map((lane) => (
          <div key={lane} style={{ position: 'absolute', left: 0, right: 0, top: lane * 13, height: 11 }}>
            <span style={{ position: 'absolute', left: -22, width: 20, top: -1, fontSize: 9, color: 'var(--c-ink-3, #9aa1bd)' }}>{LANE_LABEL[lane]}</span>
            <div style={{ position: 'absolute', left: 0, right: 0, top: 3, height: 5, background: 'var(--c-bg-soft, #f6f7fb)', borderRadius: 3 }} />
          </div>
        ))}
        {segs.map((sg, i) => {
          const left = pct(sg.s)
          const w = Math.max(pct(sg.e) - left, 0.5)
          return (
            <Tooltip key={i} title={sg.tip}>
              <div style={{
                position: 'absolute', left: `${left}%`, width: `${w}%`,
                top: sg.lane * 13 + 3, height: 5, background: sg.color, borderRadius: 3, opacity: 0.85, cursor: 'default',
              }} />
            </Tooltip>
          )
        })}
        {marks.map((mk, i) => (
          <Tooltip key={i} title={mk.tip}>
            <div style={{
              position: 'absolute', left: `${pct(mk.at)}%`,
              top: 3 * 13, width: 2, height: 11 * 3 + 2, background: mk.color, cursor: 'default',
            }} />
          </Tooltip>
        ))}
      </div>
    </div>
  )
}

function RunBlock({
  run, index, showRaw, expandAll, showTimeline, onReplay,
}: {
  run: { runId: string; events: RunEventDTO[]; allEvents: RunEventDTO[]; startedAt: string; finished?: RunEventDTO }
  index: number
  showRaw: boolean
  expandAll: boolean
  showTimeline: boolean
  onReplay: () => void
}) {
  const [openId, setOpenId] = useState<Record<string, boolean>>({})
  const [openSeg, setOpenSeg] = useState<Record<string, boolean>>({})
  useEffect(() => {
    if (expandAll) {
      const m: Record<string, boolean> = {}
      run.events.forEach((e) => (m[e.id] = true))
      setOpenId(m)
    } else setOpenId({})
  }, [expandAll, run.events])
  const fin = run.finished
  const finState = fin ? safeParse(fin.data) : undefined
  // REQ-231④：生效审批策略（run.started 透出 tool_approval {mode, source}——治 P-H2 生效不可见）
  const approvalChip = (() => {
    const started = run.events.find((e) => e.type === 'run.started')
    const ap = started ? safeParse(started.data)?.tool_approval : undefined
    if (!ap || !ap.mode) return null
    const label = ap.mode === 'danger' ? '危险工具审批' : ap.mode === 'all' ? '全部审批' : String(ap.mode)
    return (
      <Tooltip title={`生效来源：${ap.source === 'conversation' ? '会话级覆盖' : '智能体级策略'}`}>
        <Tag color={ap.mode === 'all' ? 'purple' : 'gold'} style={{ margin: 0, fontSize: 10 }}>
          审批·{label}{ap.source === 'conversation' ? '（会话覆盖）' : ''}
        </Tag>
      </Tooltip>
    )
  })()
  // REQ-291：运行配置审计——当轮模型与推理参数（run.started 恒落；缺省键=跟随后端协议默认，不臆造）
  const modelCfgChip = (() => {
    const started = run.events.find((e) => e.type === 'run.started')
    const d = started ? safeParse(started.data) : undefined
    if (!d) return null
    const temp = d.temperature ?? null
    const toks = d.max_tokens ?? null
    if (!d.model && temp == null && toks == null) return null
    return (
      <Tooltip title={`当轮模型 ${d.model ?? '—'}；temperature ${temp ?? '后端默认'} / max_tokens ${toks ?? '后端默认'}（未设置=跟随后端协议默认）`}>
        <Tag style={{ margin: 0, fontSize: 10 }}>
          {d.model ? `${d.model}` : '模型 —'}{temp != null ? ` · t=${temp}` : ''}{toks != null ? ` · ${toks}tok` : ''}
        </Tag>
      </Tooltip>
    )
  })()
  // REQ-284①：流式输出归并段（列表主视图——delta 不再逐条渲染）
  const segs = useMemo(() => buildSegments(run.events), [run.events])
  return (
    <div style={{ marginBottom: 14, border: '1px solid var(--c-border, #e3e6f0)', borderRadius: 8, overflow: 'hidden' }}>
      <div style={{ background: 'var(--c-bg-soft, #f6f7fb)', padding: '6px 10px', display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <Typography.Text strong style={{ fontSize: 12 }}>运行 {index + 1}</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>{fmtTime(run.startedAt)}</Typography.Text>
        {modelCfgChip}
        {approvalChip}
        {fin && (
          <Tag color={fin.type === 'run.error' ? 'red' : (finState?.reason === 'verify_failed' ? 'orange' : 'green')} style={{ margin: 0, fontSize: 10 }}>
            {fin.type === 'run.error' ? '出错' : finState?.reason ? String(finState.reason) : '完成'}
          </Tag>
        )}
        {finState?.duration_ms != null && <Typography.Text type="secondary" style={{ fontSize: 11 }}>{finState.duration_ms}ms</Typography.Text>}
        {finState?.usage?.total_tokens != null && (
          <Tooltip title={`输入 ${finState.usage.input_tokens ?? '—'} / 输出 ${finState.usage.output_tokens ?? '—'}`}>
            <Tag style={{ margin: 0, fontSize: 10 }}>{finState.usage.total_tokens} tok</Tag>
          </Tooltip>
        )}
        <span style={{ flex: 1 }} />
        <Button size="small" type="link" icon={<RollbackOutlined />} onClick={onReplay}>重放此运行</Button>
      </div>
      {showTimeline && <TimelineStrip events={run.allEvents} />}
      <div style={{ padding: '4px 0' }}>
        {segs.map((seg, si) => {
          if (seg.kind === 'reasoning') {
            const key = `seg-r-${si}`
            const span = fmtDur(tsOf(seg.end) - tsOf(seg.start))
            return (
              <div key={key} style={{ padding: '3px 10px 3px' }}>
                <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, cursor: 'pointer' }}
                  onClick={() => setOpenSeg((m) => ({ ...m, [key]: !m[key] }))}>
                  <Tag color="purple" style={{ margin: 0, fontSize: 10 }}>思考</Tag>
                  <span style={{ fontSize: 12, flex: 1 }}>
                    深度思考 {seg.chars} 字 · {span} · {seg.items.length} 段流式
                    {seg.text ? ` · ${seg.text.replace(/\s+/g, ' ').slice(0, 60)}…` : ''}
                  </span>
                </div>
                {(openSeg[key] || expandAll) && seg.text && (
                  <pre style={{ margin: '2px 0 2px 34px', maxHeight: 300, overflow: 'auto', whiteSpace: 'pre-wrap', fontSize: 11, color: 'var(--c-ink-2)', background: 'var(--c-bg-soft, #f6f7fb)', padding: 6, borderRadius: 6 }}>{seg.text}</pre>
                )}
              </div>
            )
          }
          if (seg.kind === 'message') {
            const span = fmtDur(tsOf(seg.end) - tsOf(seg.start))
            return (
              <div key={`seg-m-${si}`} style={{ padding: '3px 10px 3px' }}>
                <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
                  <Tag color="blue" style={{ margin: 0, fontSize: 10 }}>正文</Tag>
                  <span style={{ fontSize: 12, flex: 1, color: 'var(--c-ink-2)' }}>
                    流式输出 {seg.chars} 字 · {span}（正文全文以消息表为准，此处为输出时段）
                  </span>
                </div>
              </div>
            )
          }
          const e = seg.e
          const d = safeParse(e.data)
          const desc = describeEvent(e.type, d)
          const depth = e.type === 'subagent.enter' || e.type === 'subagent.exit' ? 1 : 0
          const dur = e.type === 'tool.result' && d?.duration_ms != null ? `${d.duration_ms}ms` : ''
          return (
            <div key={e.id} style={{ padding: '3px 10px 3px' }}>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, paddingLeft: depth * 16, cursor: 'pointer' }}
                onClick={() => setOpenId((m) => ({ ...m, [e.id]: !m[e.id] }))}>
                <Tag color={TYPE_COLOR[e.type] ?? 'default'} style={{ margin: 0, fontSize: 10 }}>{e.type}</Tag>
                <span style={{ fontSize: 12, flex: 1 }} className={desc.warn ? 'warn-text' : ''}>{desc.text}</span>
                {d?.source && <Tag style={{ margin: 0, fontSize: 10 }}>{String(d.source)}</Tag>}
                {dur && <span style={{ fontSize: 11, color: 'var(--c-ink-2)' }}>{dur}</span>}
              </div>
              {(showRaw || openId[e.id]) && e.data && (
                <pre className="raw-json" style={{ margin: '2px 0 2px 34px', maxHeight: 240, overflow: 'auto' }}>{JSON.stringify(d ?? e.data, null, 2)}</pre>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
