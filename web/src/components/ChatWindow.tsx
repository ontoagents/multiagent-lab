import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { Alert, Button, Collapse, Dropdown, Input, Modal, Popover, Segmented, Space, Splitter, Switch, Tag, Tooltip, Typography } from 'antd'
import { AlignLeftOutlined, BookOutlined, BranchesOutlined, ClusterOutlined, CopyOutlined, EditOutlined, EllipsisOutlined, LoadingOutlined, RedoOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { Bubble, Welcome } from '@ant-design/x'
import type { BubbleListProps } from '@ant-design/x'
import XMarkdown from '@ant-design/x-markdown'
import { api, connDisplayName, extApproval, resumeConversation, runConversation } from '../api/client'
import type { ComparePaneConfig } from '../api/client'
import ConvSettingsPanel from './chat/ConvSettingsPanel'
import ObservePanel from './chat/ObservePanel'
import ComparePane from './chat/ComparePane'
import Composer from './chat/Composer'
import TraceDrawer from './TraceDrawer'
import HighlightSpans from './HighlightSpans'
import { AgentLogo } from './AgentLogo'
import { groupToolPhases, ToolPhaseBlock, type ToolPhaseGroup } from './ToolPhase'
import { groupRunBlocks, RunBlockView, RunMetaLine, isRunBlock } from './RunBlock'
import type {
  Agent,
  Conversation,
  KBHit,
  KnowledgeBase,
  ModelConnection,
  Project,
  RuntimeProfile,
} from '../api/types'
import { useUI } from '../store/ui'
// REQ-401/M197 拆分：事件注册表单源与五域辅助件（行为零变更；四份事件清单收敛 lib/eventRegistry）
import { describeEvent, eventSource, fmtNum, injectSourceLabel, isKeyGranularity, levelGated } from '../lib/eventRegistry'
import { applyRunEvent, toInterruptState, type ChatItem } from './chat/runApply'
import { CompanionCapsule, summarizeCompanionPhases, type CompanionSummaryItem } from './chat/companion'
import { TodoCard } from './chat/TodoCard'
import { BLANK_PANE, BUBBLE_ROLES, ChatChip, SEG_COLORS, SEG_LABELS, type PaneSel } from './chat/bits'
import DelegatePanel from './chat/DelegatePanel'

/**
 * 中间对话窗口（原型 06 §3.1 / §3.2，Ant Design X Bubble/Sender）：
 * - 垂直三段：配置条（单行徽标）/ 消息流（Bubble.List 内建滚动 + autoScroll 置底）/ 输入区；
 * - 阅读尺度：气泡自身 max-width 880 居中（滚动条因此贴对话区最右缘），输入区同宽对齐；
 * - 助手正文：XMarkdown 渲染（流式期间尾部游标），用户消息保持纯文本；
 * - 运行中：消息流右下角悬浮「停止生成」入口，复用 stop()（abort + POST /stop）；
 * - agent 直聊：头部显示智能体名，「配置」打开智能体弹窗；
 * - project 会话：头部注明「项目：xxx · 会话使用的智能体：xxx」；
 * - 执行细节：历史事件回放、token 用量与耗时、深度思考紧凑行、工具调用 JSON、原始事件调试开关
 * - M4-M8：子智能体 / 知识召回 / 本体事件卡（实时与回放共用 describeEvent）；
 *   会话级配置为输入卡内底部的三个开关 chip（知识库 / 本体 / 技能，X Sender footer），
 *   随改随存（merge-safe patchConv）；具体参数在「知识库」页与智能体属性中维护
 */

// REQ-477②③：时长人话格式——思考行「持续 Xs」（fmtDur）与运行计时行「工作中 m 分 ss 秒」（fmtWall）分用
const fmtDur = (ms: number): string => {
  if (ms < 1000) return '<1s'
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const s = Math.floor(ms / 1000)
  return `${Math.floor(s / 60)} 分 ${String(s % 60).padStart(2, '0')} 秒`
}
const fmtWall = (ms: number): string => {
  const s = Math.max(0, Math.floor(ms / 1000))
  const m = Math.floor(s / 60)
  return m > 0 ? `${m} 分 ${String(s % 60).padStart(2, '0')} 秒` : `${s} 秒`
}

/** REQ-478①：思考行（详略分档）——live 默认收起，思考内容行内**单行滚动**（超宽来回 marquee，
 *  随流式增量周期重测；工具行/委派/正文分段等重点过程照常展开呈现），点击展开全文；
 *  完成后收起+「持续 Xs · N 字」。开合记忆由 ChatWindow.reasoningOpen 承载（evKey 稳定键）。 */
function ThinkRow({ it, open, onToggle, style }: { it: ChatItem; open: boolean; onToggle: () => void; style?: CSSProperties }) {
  const len = it.reasoning?.length ?? 0
  const streaming = !!it.streamKey
  const boxRef = useRef<HTMLSpanElement | null>(null)
  const txtRef = useRef<HTMLSpanElement | null>(null)
  useEffect(() => {
    if (!streaming || open) return
    const box = boxRef.current
    const txt = txtRef.current
    if (!box || !txt) return
    const measure = () => {
      const over = txt.scrollWidth - box.clientWidth
      if (over > 8) {
        txt.style.setProperty('--roll-x', `${-over}px`)
        txt.style.setProperty('--roll-dur', `${Math.max(4, Math.min(14, over / 30))}s`)
        txt.classList.add('rolling')
      } else {
        txt.classList.remove('rolling')
      }
    }
    measure()
    const timer = window.setInterval(measure, 1000)
    return () => {
      window.clearInterval(timer)
      txt.classList.remove('rolling')
    }
  }, [streaming, open, len])
  const meta = [
    typeof it.reasonMs === 'number' ? `持续 ${fmtDur(it.reasonMs)}` : '',
    len ? `${len} 字` : '',
  ].filter(Boolean).join(' · ')
  return (
    <div className="think-row" style={style} data-testid="think-row">
      <div className="think-head" onClick={onToggle} title={open ? '收起思考过程' : '展开思考过程'}>
        <span aria-hidden>🧠</span>
        <span>思考</span>
        {streaming && <LoadingOutlined spin style={{ color: 'var(--ant-color-primary, #4f46e5)' }} />}
        {meta && <span className="think-meta">· {meta}</span>}
        {!open && len > 0 && (
          <span className="think-peek" ref={boxRef}>
            <span className="think-peek-text" ref={txtRef}>{it.reasoning}</span>
          </span>
        )}
        <span className={`think-caret${open ? ' open' : ''}`} aria-hidden />
      </div>
      {open && len > 0 && <pre className="thinking think-body">{it.reasoning}</pre>}
    </div>
  )
}

export default function ChatWindow({
  conversation,
  agents,
  projects,
  onConversationUpdated,
  onToggleSidePanel,
  initialDraft,
}: {
  conversation: Conversation
  agents: Agent[]
  projects: Project[]
  onConversationUpdated: () => void
  /** REQ-336：侧板开合（⋮会话设置「智能体配置」入口复用；由页面持有状态与面板） */
  onToggleSidePanel?: () => void
  /** REQ-360/M161：跨模块跳转预填启动消息（预填不代发——最后一脚留给用户按发送）。
   *  经 useEffect 写入输入框（会话切换复用实例时也能到达），非挂载期一次性。 */
  initialDraft?: string
}) {
  const { bumpData, showToast, setCurrentConv } = useUI()
  const isProjectScope = conversation.scope === 'project'

  const project = isProjectScope
    ? projects.find((p) => p.id === conversation.project_id) ?? null
    : null
  // agent 直聊 = 绑定的智能体；项目会话 = 主智能体，缺省取第一个成员
  const agent =
    conversation.scope === 'agent'
      ? agents.find((a) => a.id === conversation.agent_id) ?? null
      : project
        ? agents.find((a) => a.id === (project.coordinator || project.agent_ids[0])) ?? null
        : null
  // REQ-302/M106：会话级伴生本体切换——本体候选（agent 有伴生绑定时拉一次）
  const [companionOntos, setCompanionOntos] = useState<{ id: string; name: string }[]>([])
  useEffect(() => {
    if (!agent?.companion_ontology_id) return
    let alive = true
    api.listOntologies().then((lst) => {
      if (alive) setCompanionOntos((Array.isArray(lst) ? lst : []).map((o) => ({ id: o.id, name: o.name })))
    }).catch(() => { /* 目录失败时仅默认项可选 */ })
    return () => { alive = false }
  }, [agent?.companion_ontology_id])


  const [items, setItems] = useState<ChatItem[]>([])
  const [input, setInput] = useState('')
  // REQ-360/M161：跨模块预填启动消息（构建平面「在智能体对话中构建/转到对话继续」跳转；
  // 预填不代发，用户可改后自行发送）
  useEffect(() => {
    if (initialDraft) setInput(initialDraft)
  }, [initialDraft])
  const [running, setRunning] = useState(false)
  // REQ-483③：运行计时行改为消息流末尾一行灰字（参考图形态）——流内承载，随 run 收尾消失
  const [runStart, setRunStart] = useState<number | null>(null)
  const [nowTick, setNowTick] = useState(0)
  useEffect(() => {
    if (!running) return
    const t = window.setInterval(() => setNowTick((n) => n + 1), 1000)
    return () => window.clearInterval(t)
  }, [running])
  // REQ-403/M199：原始事件 JSON 开关单源化——头部过程面板唯一配置点（localStorage 按会话记忆），
  // TraceDrawer 经 props 消费同一 state（其工具栏本地双份退役）
  const [showRaw, setShowRaw] = useState(() => localStorage.getItem(`eino.showraw.${conversation?.id ?? ''}`) === '1')
  useEffect(() => {
    if (conversation?.id) localStorage.setItem(`eino.showraw.${conversation.id}`, showRaw ? '1' : '0')
  }, [conversation?.id, showRaw])
  // REQ-135②③：对话级过程展示配置——粒度 all|key|off + 深度思考显隐（localStorage 按会话记忆）
  const [granularity, setGranularity] = useState<'all' | 'key' | 'off'>('all')
  const [showReasoning, setShowReasoning] = useState(true)
  // REQ-117/M17 三档观测级别：0 简洁 / 1 详细 / 2 调试（每会话记忆；仅影响之后的运行）
  const [debugLevel, setDebugLevel] = useState(() => Number(localStorage.getItem(`eino.debug.${conversation?.id}`)) || 0)
  useEffect(() => {
    setDebugLevel(Number(localStorage.getItem(`eino.debug.${conversation?.id}`)) || 0)
  }, [conversation?.id])
  const changeDebugLevel = (lv: number) => {
    setDebugLevel(lv)
    if (conversation?.id) localStorage.setItem(`eino.debug.${conversation.id}`, String(lv))
    // REQ-149②：数据前提联动——级别≥1 而入库关闭时，明确提示历史生效范围
    if (lv >= 1 && !debugPersist) {
      showToast('详细/调试档仅影响之后的运行；未开启「调试事件入库」，调试细节不会留存到历史')
    }
  }
  // REQ-135②：对话级过程展示配置（粒度/深度思考），localStorage 按会话记忆，切会话回填
  const convCfgKey = `eino.convcfg.${conversation?.id}`
  useEffect(() => {
    try {
      const cfg = JSON.parse(localStorage.getItem(convCfgKey) || '{}')
      setGranularity(cfg.granularity ?? 'all')
      setShowReasoning(cfg.showReasoning ?? true)
      setDebugPersist(cfg.debugPersist ?? false)
    } catch {
      setGranularity('all')
      setShowReasoning(true)
      setDebugPersist(false)
    }
  }, [convCfgKey])
  // REQ-149②：调试事件入库开关（对话级记忆；级别≥1 时产生 model.step/装配快照落库）
  const [debugPersist, setDebugPersist] = useState(false)
  const patchConvCfg = (patch: { granularity?: 'all' | 'key' | 'off'; showReasoning?: boolean; debugPersist?: boolean }) => {
    if (!conversation?.id) return
    let cfg: any = {}
    try {
      cfg = JSON.parse(localStorage.getItem(convCfgKey) || '{}')
    } catch { /* 忽略坏数据 */ }
    localStorage.setItem(convCfgKey, JSON.stringify({ ...cfg, ...patch }))
    if (patch.granularity) setGranularity(patch.granularity)
    if (patch.showReasoning !== undefined) setShowReasoning(patch.showReasoning)
    if (patch.debugPersist !== undefined) setDebugPersist(patch.debugPersist)
  }
  // 深度思考卡的展开状态（按稳定 evKey 记录，独立于 items，历史重载不丢失）：
  // 无记录时默认「流式中展开、结束后收起」，用户手动开合后以用户选择为准
  const [reasoningOpen, setReasoningOpen] = useState<Record<string, boolean>>({})
  const runRef = useRef<{ abort: () => void; done: Promise<void> } | null>(null)
  const runKeyRef = useRef('')
  // REQ-281：伴生沉淀过程事件实时补齐——事件在 run 收尾后旁路产生（SSE 已关），
  // 收尾后短轮询 companion.* 事件追加进流；已见事件 id 集合防重复（历史回放与轮询共用）。
  const seenEventsRef = useRef<Set<string>>(new Set())
  // REQ-282 B2：固定 2s×60（120s 硬顶）改指数退避 2s→5s→10s + 10 分钟硬顶——
  // 思考模型抽取动辄数分钟，预算调大后终态卡不再被轮询上限饿死（历史回放仍兜底自愈）。
  const companionPollRef = useRef<{ timer: number; cancel: () => void } | null>(null)
  // REQ-330②：伴生管线过程事件（extract/candidates/decision/ingest/reject）进右上角悬浮胶囊，
  // 不再入聊天流（与本职工作事件解耦；companion.hit 保留流内=对当前回答的归因增强）
  const [capsuleEvents, setCapsuleEvents] = useState<ChatItem[]>([])
  const stopCompanionPoll = () => {
    if (companionPollRef.current) {
      companionPollRef.current.cancel()
      window.clearTimeout(companionPollRef.current.timer)
      companionPollRef.current = null
    }
  }
  const startCompanionPoll = (convId: string) => {
    stopCompanionPoll()
    setCapsuleEvents([]) // 新一轮沉淀活动=新胶囊（终态徽标随会话切换/新活动重置）
    let waited = 0
    let emptyAfterDone = 0
    let sawDone = false
    let cancelled = false
    // REQ-330①：done 终结闸基线——仅计「本次收尾之后产生」的 done/error，防上一轮抽取的在途终态提前自灭轮询
    let baselineTs = ''
    try {
      void api.listEvents(convId, { type_prefix: 'companion.', limit: 1, order: 'desc' }).then((newest) => {
        baselineTs = newest[0]?.created_at ?? ''
      }).catch(() => { /* 取不到基线=不设闸 */ })
    } catch { /* 同上 */ }
    const tick = async () => {
      if (cancelled) return
      let freshCount = 0
      try {
        // REQ-330①：增量取数改 desc 取最新窗（升序+limit 恒取最老 50 条，伴生存量>50 的会话新事件恒在窗外=轮询盲区）；
        // reverse 回时序后沿用 seenEvents 去重
        const evs = await api.listEvents(convId, { type_prefix: 'companion.', limit: 50, order: 'desc' })
        const fresh = evs.filter((e) => !seenEventsRef.current.has(e.id)).reverse()
        freshCount = fresh.length
        for (const e of fresh) {
          seenEventsRef.current.add(e.id)
          let d: any = {}
          try { d = e.data ? JSON.parse(e.data) : {} } catch { /* 忽略坏数据 */ }
          if (e.type === 'companion.extract' && (d?.phase === 'done' || d?.phase === 'error') && (!baselineTs || String(e.created_at ?? '') > baselineTs)) sawDone = true
          const desc = describeEvent(e.type, d)
          const item: ChatItem = { kind: 'event', evType: e.type, eventText: desc.text, eventErr: desc.err, eventWarn: desc.warn, evData: d, evKey: `ce-${e.id}` }
          if (e.type === 'companion.hit') setItems((prev) => [...prev, item]) // ③：命中归因卡保留聊天流
          else setCapsuleEvents((prev) => [...prev, item]) // ②：管线过程事件进悬浮胶囊
        }
        emptyAfterDone = sawDone && freshCount === 0 ? emptyAfterDone + 1 : 0
      } catch { /* 网络抖动下一轮重试 */ }
      if (waited >= 600000 || (sawDone && emptyAfterDone >= 2)) {
        companionPollRef.current = null
        return
      }
      const delay = waited < 20000 ? 2000 : waited < 120000 ? 5000 : 10000
      waited += delay
      const timer = window.setTimeout(tick, delay)
      companionPollRef.current = { timer, cancel: () => { cancelled = true } }
    }
    void tick()
  }
  // REQ-150②：用户主动停止标记——abort 断流后 run.finished 不会到达，收尾据此把流式消息置终态
  const stopFlagRef = useRef(false)

  // 历史还原：消息表（对话正文）+ 事件表（执行时间线）按时间合并
  useEffect(() => {
    // B3（platform-knowledge/智能体/ChatWindow渲染与SSE链路审查）：切换会话先中止旧流，防止旧会话事件写入新会话列表
    runRef.current?.abort()
    runRef.current = null
    stopCompanionPoll() // REQ-281：切会话同时停旧会话的伴生事件轮询
    setRunning(false)
    let alive = true
    setItems([])
    setCapsuleEvents([]) // REQ-330②：胶囊随会话切换重置（历史沉淀靠回放摘要行+TraceDrawer 承接）
    if (!conversation) return
    Promise.all([api.listMessages(conversation.id), api.listEvents(conversation.id).catch(() => [] as never[])])
      .then(([msgs, evs]) => {
        if (!alive) return
        seenEventsRef.current = new Set(evs.map((e) => e.id)) // REQ-281：已回放事件标记，轮询只补增量
        const timeline: { ts: string; item: ChatItem }[] = []
        // REQ-406①/M202：message.segment 分段重建——先按 run 聚合段文本，助手消息与聚合
        // 文本精确匹配时以段代显（交错正文回放）；不匹配/无段（存量轮）走原消息气泡
        const segTextByRun: Record<string, string> = {}
        for (const e of evs) {
          if (e.type !== 'message.segment') continue
          try {
            const d = e.data ? JSON.parse(e.data) : {}
            segTextByRun[e.run_id ?? '_'] = (segTextByRun[e.run_id ?? '_'] ?? '') + (d.text ?? '')
          } catch {
            /* 忽略非 JSON 数据 */
          }
        }
        const segTextsConsumed = new Set<string>()
        msgs.forEach((m) => {
          if (m.role === 'assistant' && m.content) {
            const hit = Object.entries(segTextByRun).find(([, txt]) => txt === m.content && !segTextsConsumed.has(txt))
            if (hit) {
              segTextsConsumed.add(hit[1])
              return // 以段代显：segment 事件按 ts 交错进入 timeline
            }
          }
          timeline.push({ ts: m.created_at, item: { kind: 'msg', role: m.role, content: m.content } })
        })
        const reasoningCards: Record<string, ChatItem> = {}
        for (const e of evs) {
          let d: any = {}
          try {
            d = e.data ? JSON.parse(e.data) : {}
          } catch {
            /* 忽略非 JSON 数据 */
          }
          if (e.type === 'message.delta') continue // 正文已由消息表还原，避免重复
          if (e.type === 'message.segment') {
            // REQ-477①：已以段代显的 run，段以 msg 形态入线——原 event 形态令 REQ-362 完成块
            // answer 提升（只认 kind=msg）恒 miss，切换会话后正文全在折叠体内不可见（报障实锤）；
            // 连续段合并单条（末段即最终回答可被提升外显），块体内按 run-seg 淡一档回看；
            // 未消费（如消息已删）也照常呈现，不丢内容
            const text = d.text ?? ''
            const lastIt = timeline[timeline.length - 1]?.item
            if (text && lastIt && lastIt.kind === 'msg' && lastIt.role === 'assistant' && lastIt.seg) {
              lastIt.content = (lastIt.content ?? '') + text
            } else if (text) {
              timeline.push({ ts: e.created_at, item: { kind: 'msg', role: 'assistant', content: text, seg: true } })
            }
            continue
          }
          if (e.type === 'reasoning.delta') {
            const k = e.run_id || '_'
            if (!reasoningCards[k]) {
              reasoningCards[k] = {
                kind: 'event',
                evType: 'reasoning',
                reasoning: '',
                evKey: `hist-${k}`,
                evData: { run_id: e.run_id, type: 'reasoning' },
              }
              timeline.push({ ts: e.created_at, item: reasoningCards[k] })
            }
            reasoningCards[k].reasoning += d?.delta ?? ''
            continue
          }
          const desc = describeEvent(e.type, d)
          timeline.push({
            ts: e.created_at,
            item: { kind: 'event', evType: e.type, eventText: desc.text, eventErr: desc.err, eventWarn: desc.warn, evData: d, ts: e.created_at },
          })
        }
        timeline.sort((a, b) => (a.ts < b.ts ? -1 : a.ts > b.ts ? 1 : 0))
        setItems(timeline.map((t) => t.item))
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [conversation.id])

  // ---- 会话配置 chips 的选项数据（知识库 / 本体运行方案）----
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [profilesErr, setProfilesErr] = useState(false)
  const [kbsErr, setKbsErr] = useState(false)
  const [picker, setPicker] = useState<'kb' | 'onto' | null>(null)
  // REQ-336：会话设置（头部⋮）——重命名/检索参数弹层；输入区「/」快速配置与文件附加
  const [convSetOpen, setConvSetOpen] = useState(false)
  const [renameVal, setRenameVal] = useState('')
  const [slashOpen, setSlashOpen] = useState(false)
  const [attachFiles, setAttachFiles] = useState<{ name: string; size: number }[]>([])
  const [attaching, setAttaching] = useState(false)

  // 选项列表：挂载时拉取；失败降级为空 + chip 置灰说明
  const loadCfgOptions = () => {
    api.listRuntimeProfiles().then((ps) => { setProfiles(ps); setProfilesErr(false) }).catch(() => setProfilesErr(true))
    api.listKBs().then((ks) => { setKbs(ks); setKbsErr(false) }).catch(() => setKbsErr(true))
    // REQ-19f 窗格模型覆盖候选：已启用的对话模型连接
    api.listConnections().then((cs) => setConns(cs.filter((c) => c.enabled && c.conn_type === 'chat'))).catch(() => {})
  }
  useEffect(loadCfgOptions, [])
  const [conns, setConns] = useState<ModelConnection[]>([])
  const convAgent = agents.find((a) => a.id === conversation?.agent_id) ?? null
  // REQ-336：模型 chip 退役——模型配置归智能体（侧板「模型」页签；会话级 model_conn_id 覆盖能力
  // 保留在会话行，无发送框入口）。REQ-291 一次性迁移保留（老偏好仍落会话行，后端装配照常生效）。
  // REQ-291 迁移：REQ-174 时代的 localStorage 模型偏好（eino.chat.model.{id}）一次性写入会话行后清除
  const migratedModelRef = useRef<string | null>(null)
  useEffect(() => {
    if (!conversation?.id || conversation.model_conn_id || migratedModelRef.current === conversation.id) return
    let legacy: string | null = null
    try { legacy = localStorage.getItem(`eino.chat.model.${conversation.id}`) } catch { return }
    migratedModelRef.current = conversation.id
    if (!legacy) return
    localStorage.removeItem(`eino.chat.model.${conversation.id}`)
    api.updateConversation(conversation.id, { ...conversation, model_conn_id: legacy })
      .then(() => onConversationUpdated())
      .catch(() => { /* 迁移失败静默——历史偏好不丢，仅无 UI 入口 */ })
  }, [conversation?.id, conversation?.model_conn_id])

  // ---- REQ-19e/19f 对话对比模式：开关 + 2~4 窗格 + 每窗格单项覆盖（''= 继承对话当前配置）----
  const cmpKey = `eino.compare.${conversation?.id}`
  const [cmp, setCmp] = useState<{ on: boolean; panes: PaneSel[] }>({ on: false, panes: [{ ...BLANK_PANE }, { ...BLANK_PANE }] })
  useEffect(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(cmpKey) || 'null')
      if (saved && typeof saved.on === 'boolean' && Array.isArray(saved.panes) && saved.panes.length >= 2 && saved.panes.length <= 4) {
        setCmp({ on: saved.on, panes: saved.panes.map((p: any) => ({ agent: p?.agent ?? '', model: p?.model ?? '', kb: p?.kb ?? '', profile: p?.profile ?? '', noHistory: !!p?.noHistory, temperature: typeof p?.temperature === 'number' ? p.temperature : null, instruction: p?.instruction ?? '', skills: p?.skills ?? '' })) })
        return
      }
    } catch { /* 忽略坏数据 */ }
    setCmp({ on: false, panes: [{ ...BLANK_PANE }, { ...BLANK_PANE }] })
  }, [cmpKey])
  const patchCmp = (patch: Partial<{ on: boolean; panes: PaneSel[] }>) => {
    setCmp((prev) => {
      const next = { ...prev, ...patch }
      try { localStorage.setItem(cmpKey, JSON.stringify(next)) } catch { /* 忽略配额 */ }
      return next
    })
  }
  const setPaneSel = (i: number, field: keyof PaneSel, value: string | number | boolean | null) => {
    setCmp((prev) => {
      if (!prev.on) return prev
      const panes = prev.panes.map((p, j) => (j === i ? { ...p, [field]: value } : p))
      const next = { ...prev, panes }
      try { localStorage.setItem(cmpKey, JSON.stringify(next)) } catch { /* 忽略配额 */ }
      return next
    })
  }
  const resizePanes = (panes: PaneSel[], n: number): PaneSel[] =>
    Array.from({ length: n }, (_, i) => panes[i] ?? { ...BLANK_PANE })

  // REQ-144：配置剖面对话级命名保存（localStorage，不做全局剖面库）
  const profilesKey = `eino.compare.profiles.${conversation?.id}`
  const [cmpProfiles, setCmpProfiles] = useState<{ name: string; cfg: Partial<PaneSel> }[]>([])
  const [profileSaveFor, setProfileSaveFor] = useState<number | null>(null)
  const [profileName, setProfileName] = useState('')
  useEffect(() => {
    try {
      const list = JSON.parse(localStorage.getItem(profilesKey) || '[]')
      if (Array.isArray(list)) setCmpProfiles(list)
    } catch { /* 忽略坏数据 */ }
  }, [profilesKey])
  const persistProfiles = (list: { name: string; cfg: Partial<PaneSel> }[]) => {
    setCmpProfiles(list)
    try { localStorage.setItem(profilesKey, JSON.stringify(list)) } catch { /* 忽略配额 */ }
  }
  // 剖面仅捕获「配置型」字段（智能体/模型/库/方案/温度/提示词/技能/历史口径）
  const capturePane = (p: PaneSel): Partial<PaneSel> => ({
    agent: p.agent, model: p.model, kb: p.kb, profile: p.profile,
    temperature: p.temperature, instruction: p.instruction, skills: p.skills, noHistory: p.noHistory,
  })
  const saveProfile = (i: number) => setProfileSaveFor(i)
  const doSaveProfile = () => {
    if (profileSaveFor == null) return
    const name = profileName.trim()
    if (!name) { showToast('请输入剖面名称', 'err'); return }
    const cfg = capturePane(cmp.panes[profileSaveFor])
    const next = [...cmpProfiles.filter((x) => x.name !== name), { name, cfg }]
    persistProfiles(next)
    showToast(`剖面「${name}」已保存（对话级，可在任意窗格应用）`)
    setProfileSaveFor(null)
    setProfileName('')
  }
  const applyProfile = (i: number, cfg: Partial<PaneSel>) => {
    setCmp((prev) => {
      if (!prev.on) return prev
      const panes = prev.panes.map((p, j) => (j === i ? { ...p, ...cfg } : p))
      const next = { ...prev, panes }
      try { localStorage.setItem(cmpKey, JSON.stringify(next)) } catch { /* 忽略配额 */ }
      return next
    })
    showToast(`已应用剖面到窗格 ${i + 1}（覆盖合并序：剖面 > 窗格单项 > 智能体 > 全局）`)
  }
  // REQ-144 可选增强：采纳窗格配置转正（agent 直聊：写回 agent_id/模型/知识库/方案/技能开关继续对话；
  // 项目会话不提供——主智能体/协调者变更属项目配置职责）
  const adoptPane = (sel: PaneSel) => {
    if (isProjectScope) return
    const kb = sel.kb || conversation.kb_id
    const rp = sel.profile || conversation.runtime_profile_id
    const patch: Partial<Conversation> = {
      agent_id: sel.agent || conversation.agent_id,
      enable_kb: sel.kb ? true : conversation.enable_kb,
      ontology_enabled: sel.profile ? true : conversation.ontology_enabled,
    }
    if (sel.model) patch.model_conn_id = sel.model // REQ-291：窗格模型选择随采纳写回会话行
    if (kb) patch.kb_id = kb
    if (rp) patch.runtime_profile_id = rp
    if (sel.skills) patch.enable_skills = sel.skills === 'on'
    patchConv(patch)
    showToast(`已采纳窗格配置（智能体 ${agents.find((a) => a.id === patch.agent_id)?.name ?? '—'}），关闭对比后单流沿用`)
  }

  // REQ-144：复制上一窗格配置（纯前端操作，便于单变量 A/B）
  const copyPrevPane = (i: number) => {
    setCmp((prev) => {
      if (!prev.on || i <= 0) return prev
      const panes = prev.panes.map((p, j) => (j === i ? { ...prev.panes[i - 1] } : p))
      const next = { ...prev, panes }
      try { localStorage.setItem(cmpKey, JSON.stringify(next)) } catch { /* 忽略配额 */ }
      return next
    })
    showToast(`已复制窗格 ${i} 的配置到窗格 ${i + 1}`)
  }

  // 窗格消息流（会话内累计；切会话清空；窗格数变化对齐长度）
  const [paneItems, setPaneItems] = useState<ChatItem[][]>([[], []])
  const paneRunMapRef = useRef<Record<string, number>>({})
  useEffect(() => {
    setPaneItems((prev) => prev.map(() => []))
    paneRunMapRef.current = {}
  }, [conversation?.id])
  useEffect(() => {
    setPaneItems((prev) => (prev.length === cmp.panes.length ? prev : Array.from({ length: cmp.panes.length }, (_, i) => prev[i] ?? [])))
  }, [cmp.panes.length])
  const updatePane = (i: number, updater: (prev: ChatItem[]) => ChatItem[]) => {
    setPaneItems((prev) => prev.map((p, j) => (j === i ? updater(p) : p)))
  }

  // REQ-143：窗格 Agent（所选优先，缺省继承对话所属/项目主智能体）
  const paneAgentOf = (sel: PaneSel) => (sel.agent ? agents.find((a) => a.id === sel.agent) ?? null : agent)
  // Agent 候选：项目会话 = 项目成员（标记）+ 全局其余；Agent 会话 = 全部
  const agentOptions = useMemo(() => {
    const memberIds = new Set(isProjectScope ? project?.agent_ids ?? [] : [])
    return agents.map((a) => ({
      value: a.id,
      label: isProjectScope ? `${a.name}${memberIds.has(a.id) ? '（项目成员）' : '（全局）'}` : a.name,
    }))
  }, [agents, isProjectScope, project])

  // 对话级配置落库（M8）：后端 PUT 为 full-replace，必须合并当前会话字段，避免重置 title/kb_id/top_k 等
  const patchConv = async (patch: Partial<Conversation>, opts?: { silent?: boolean }) => {
    try {
      // REQ-400/M196：If-Match 乐观锁（全量载荷+updated_at 基线——并发窗口后写覆盖前写 409 提示刷新）
      await api.updateConversation(conversation.id, { ...conversation, ...patch }, conversation.updated_at)
      if (!opts?.silent) showToast('对话配置已更新')
      onConversationUpdated()
    } catch (e: any) {
      if (e?.status === 409) {
        showToast('对话配置已被其他窗口修改——已刷新为最新配置，请重新编辑', 'err')
        onConversationUpdated()
        return
      }
      showToast(e.message, 'err')
    }
  }

  // ---- 会话配置 chips（Sender footer 内）：纯开关，ON = 品牌填充 / OFF = 浅色描边 ----
  // 规则：ON 时若已绑定则直接启用；未绑定则弹轻量单选列表（选后记住绑定，写入会话字段）
  const agentSkills = agent?.skills ?? []
  const runningProfiles = profiles.filter((p) => p.status === 'running')
  const kbBound = conversation.kb_id ? kbs.find((k) => k.id === conversation.kb_id) ?? null : null
  const ontoBound = conversation.runtime_profile_id ? profiles.find((p) => p.id === conversation.runtime_profile_id) ?? null : null
  const skillsOn = conversation.enable_skills ?? true // 后端列待跟进：默认开，保持既有行为

  const kbDisabled = kbs.length === 0
  const ontoDisabled = runningProfiles.length === 0
  const skillsDisabled = agentSkills.length === 0

  const kbHint = kbDisabled
    ? (kbsErr ? '知识库列表暂不可用' : '暂无可用知识库，请先在「知识库」页创建')
    : (conversation.enable_kb ? `知识检索已开启${kbBound ? `（${kbBound.name}）` : ''}` : '开启知识检索')
  const ontoHint = ontoDisabled
    ? (profilesErr ? '本体运行方案列表暂不可用' : '暂无 running 状态的本体运行方案，请先在本体页启动')
    : (conversation.ontology_enabled
      ? `本体增强已开启${ontoBound ? `（${ontoBound.name}）` : ''}；点击可换方案或关闭`
      : '开启本体增强')
  const skillsHint = skillsDisabled
    ? '该智能体未挂载技能，请在智能体属性中配置'
    : (skillsOn ? `技能已启用（${agentSkills.length} 个）` : '技能已停用')

  const toggleKb = () => {
    if (picker === 'kb') { setPicker(null); return }
    if (kbDisabled) return
    if (conversation.enable_kb) { patchConv({ enable_kb: false }); return } // 关闭但保留 kb_id 绑定
    if (conversation.kb_id) { patchConv({ enable_kb: true }); return }
    setPicker('kb')
  }
  const toggleOnto = () => {
    if (picker === 'onto') { setPicker(null); return }
    if (ontoDisabled) return
    // 未开启：已绑定方案 → 直接开启；未绑定 → 打开方案选择器
    if (!conversation.ontology_enabled) {
      if (conversation.runtime_profile_id) { patchConv({ ontology_enabled: true }); return }
      setPicker('onto')
      return
    }
    // 已开启：打开方案选择器（换方案保持开启；关闭走选择器底部动作——REQ-215）
    setPicker('onto')
  }
  const toggleSkills = () => {
    if (skillsDisabled) return
    patchConv({ enable_skills: !skillsOn })
  }

  // 轻量单选列表（Popover 内容）：Esc / 点击外部关闭，非 Modal
  const pickerList = (
    title: string,
    items: { id: string; name: string; meta?: string }[],
    current: string | null,
    onPick: (id: string) => void,
    footer?: { icon: ReactNode; label: string; onClick: () => void },
    extra?: ReactNode, // REQ-311 B3：附加区（会话检索参数），渲染在 footer 之前
  ) => (
    <div className="chip-picker" onKeyDown={(e) => { if (e.key === 'Escape') setPicker(null) }}>
      <div className="chip-picker-title">{title}</div>
      <div className="chip-picker-list">
        {items.map((it, i) => (
          <button
            key={it.id}
            type="button"
            autoFocus={i === 0}
            className={`chip-picker-item${it.id === current ? ' current' : ''}`}
            onClick={() => { setPicker(null); onPick(it.id) }}
          >
            <span className="chip-picker-name">{it.name}</span>
            {it.meta ? <span className="chip-picker-meta">{it.meta}</span> : null}
          </button>
        ))}
      </div>
      {extra}
      {footer && (
        <>
          <div className="chip-picker-divider" />
          <button
            type="button"
            className="chip-picker-item chip-picker-manage"
            onClick={() => { setPicker(null); footer.onClick() }}
          >
            {footer.icon}
            <span className="chip-picker-name">{footer.label}</span>
          </button>
        </>
      )}
    </div>
  )

  const kbChip = <ChatChip on={conversation.enable_kb} disabled={kbDisabled} icon={<BookOutlined />} label="知识库" title={kbHint} onClick={toggleKb} />
  const ontoChip = <ChatChip on={conversation.ontology_enabled} disabled={ontoDisabled} icon={<ClusterOutlined />} label="本体" title={ontoHint} onClick={toggleOnto} />
  const skillsChip = <ChatChip on={skillsOn} disabled={skillsDisabled} icon={<ThunderboltOutlined />} label="技能" title={skillsHint} onClick={toggleSkills} />
  // REQ-336：模型 chip 退役——模型配置归智能体（侧板「模型」页签），发送框不再提供会话级切换；
  // 伴生 chip 退役——会话伴生本体归位头部 ⋮ 会话设置。「/」快速配置与文件附加见下方 Sender。

  // 事件卡渲染（思考紧凑行 / 工具详情 / 终态摘要）：
  // 紧凑、左侧色条区分来源、与助手文本列对齐（缩进统一由样式 .event-card/.event-side/.tool-phase
  // 的 margin-left 计算，内联只透传子智能体嵌套深度变量——根级=助手正文列，不再错位到头像列）
  // REQ-117：按 enter/exit 序列计算子智能体嵌套深度（回放与实时共用，渲染时缩进）
  const depthStyle = (d?: number): CSSProperties => ({ '--sub-depth': d ?? 0 }) as CSSProperties
  const withSubDepth = (list: ChatItem[]): ChatItem[] => {
    let depth = 0
    return list.map((it) => {
      if (it.kind !== 'event') return it
      if (it.evType === 'subagent.enter') {
        const d = depth
        depth += 1
        return { ...it, subDepth: d }
      }
      if (it.evType === 'subagent.exit') {
        depth = Math.max(0, depth - 1)
        return { ...it, subDepth: depth }
      }
      return { ...it, subDepth: depth }
    })
  }

  // REQ-330④：回放态伴生管线摘要行节点（listItems 平铺映射与 RunBlock 块体渲染共用）
  const companionSummaryNode = (s: CompanionSummaryItem, key: string): ReactNode => {
    // REQ-406②/M202：回放摘要行可点直达轨迹「伴生沉淀」组——与 live 胶囊「点击展开明细」
    // 同一信息架构（两形态一套呈现语义）
    const txt = `🧩 伴生沉淀 · ${s.n} 条过程${s.cand > 0 ? ` · 候选 ${s.cand} 条` : ''}${s.ingested > 0 ? ` · 自动入图 ${s.ingested} 条` : ''}${s.failed ? ' · 失败' : ''} · 点击查看过程`
    return (
      <div
        key={key}
        className="event-card src-onto companion-summary-line"
        data-testid="companion-summary-line"
        style={{ '--sub-depth': s.subDepth, cursor: 'pointer' } as CSSProperties}
        onClick={() => {
          setTraceFocusGroup('companion')
          setTraceOpen(true)
        }}
      >
        <span>{txt}</span>
      </div>
    )
  }

  const renderEventCard = (it: ChatItem, i: number) => {
    // REQ-483②：run.started「运行开始」卡不再渲染（计时行承接；带装配快照的调试态保留）
    if (it.evType === 'run.started' && !it.evData?.assembly) return null
    // REQ-478①：思考行详略分档——live 亦默认收起（内容单行滚动），点击展开全文；记忆沿 evKey
    if (it.evType === 'reasoning') {
      const cardKey = it.evKey ?? `reasoning-${i}`
      const open = reasoningOpen[cardKey] ?? false
      return (
        <ThinkRow
          key={i}
          it={it}
          open={open}
          onToggle={() => setReasoningOpen((prev) => ({ ...prev, [cardKey]: !open }))}
          style={depthStyle(it.subDepth)}
        />
      )
    }
    if (it.evType === 'model.step') {
      const msgs: any[] = Array.isArray(it.evData?.input) ? it.evData.input : []
      const tools: any[] = Array.isArray(it.evData?.tools) ? it.evData.tools : []
      const roleLabel: Record<string, string> = { user: '用户', assistant: '助手', system: '系统', tool: '工具' }
      return (
        <div key={i} className="event-card src-builtin" style={depthStyle(it.subDepth)}>
          <span>{it.eventText}</span>
          {(msgs.length > 0 || tools.length > 0) && (
            <Collapse
              ghost
              size="small"
              items={[{
                key: 'detail',
                label: <span className="event-link">调用链路详情</span>,
                children: (
                  <div>
                    {msgs.length > 0 && (
                      <>
                        <div style={{ fontSize: 11, color: 'var(--ant-color-text-secondary, #888)', marginBottom: 4 }}>
                          本次模型输入（{msgs.length} 条，含历史与系统提示词）
                        </div>
                        <pre className="raw-json">{msgs.map((m, mi) => `[${mi + 1}] ${roleLabel[m.role] ?? m.role}（${m.chars} 字）${m.content ? `\n${m.content}` : m.preview ? `\n${m.preview}` : ''}`).join('\n\n')}</pre>
                      </>
                    )}
                    {tools.length > 0 && (
                      <>
                        <div style={{ fontSize: 11, color: 'var(--ant-color-text-secondary, #888)', margin: '6px 0 4px' }}>
                          本次绑定工具（{tools.length}）
                        </div>
                        <pre className="raw-json">{tools.map((tt) => `• ${tt.name}${tt.desc ? `：${tt.desc}` : ''}`).join('\n')}</pre>
                      </>
                    )}
                  </div>
                ),
              }]}
            />
          )}
        </div>
      )
    }
    // REQ-281：伴生沉淀过程卡（本体青色族；候选批与入图结果可展开明细）
    if (it.evType?.startsWith('companion.')) {
      const cands: Array<{ id?: string; kind?: string; name?: string; confidence?: number; batch_rank?: number; aligned?: boolean }> =
        Array.isArray(it.evData?.items) ? it.evData.items : []
      const kindLabel: Record<string, string> = { concept: '概念', relation: '关系', event: '事件' }
      // REQ-282 B4：started 卡在其终态（done/error）未到达前渲染「抽取中」进行态
      const extracting = it.evType === 'companion.extract' && it.evData?.phase === 'started' &&
        !items.some((x, xi) => xi > i && x.evType === 'companion.extract' && (x.evData?.phase === 'done' || x.evData?.phase === 'error'))
      return (
        <div key={i} className="event-card src-onto" style={depthStyle(it.subDepth)}>
          <span>{extracting && <LoadingOutlined spin style={{ marginRight: 6, color: 'var(--ant-color-primary, #4f46e5)' }} />}{it.eventText}</span>
          {it.evType === 'companion.candidates' && cands.length > 0 && (
            <Collapse
              ghost
              size="small"
              items={[{
                key: 'items',
                label: <span className="event-link">展开候选明细（{cands.length}）</span>,
                children: (
                  <ul className="retrieval-hits">
                    {cands.map((c, ci) => (
                      <li key={ci} className="retrieval-hit">
                        <div className="retrieval-meta">
                          <span className="retrieval-doc" title={c.name}>{kindLabel[c.kind ?? ''] ?? c.kind} · {c.name}</span>
                          <span className="retrieval-score">置信 {fmtNum(c.confidence)} · 分位 {fmtNum(c.batch_rank)}{c.aligned ? ' · 已对齐' : ''}</span>
                        </div>
                      </li>
                    ))}
                  </ul>
                ),
              }]}
            />
          )}
          {it.evType === 'companion.ingest' && (it.evData?.mode === 'auto' || it.evData?.result === 'aggregate') && (
            <div className="retrieval-excerpt">
              {it.evData?.mode === 'auto' && <span style={{ marginRight: 10 }}>置信 {fmtNum(it.evData?.confidence)} ≥ 阈值 {fmtNum(it.evData?.threshold)} · 分位 {fmtNum(it.evData?.batch_rank)}</span>}
              {it.evData?.result === 'aggregate' && <span>同事实印证 · confirmCount={it.evData?.confirm_count ?? '?'}</span>}
            </div>
          )}
          {showRaw && it.evData && <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>}
        </div>
      )
    }
    const isTool = it.evType === 'tool.call' || it.evType === 'tool.result'

    // M6：知识召回引用块（可展开命中片段，绿族强调）
    if (it.evType === 'retrieval') {
      // REQ-194②：伴生图召回卡——实体明细带 match（vector/lexical）与 2 跳标注（诚实呈现召回来源）
      if (it.evData?.source === 'companion') {
        const ents: Array<{ label?: string; definition?: string; match?: string; relations?: Array<{ rel?: string; other?: string; dir?: string; hop?: number }> }> =
          Array.isArray(it.evData?.entities) ? it.evData.entities : []
        const matchText: Record<string, string> = { vector: '向量', lexical: '词法', 'vector+lexical': '向量+词法' }
        return (
          <div key={i} className="event-card src-retrieval retrieval-card">
            <span>{it.eventText}</span>
            {ents.length > 0 && (
              <Collapse
                ghost
                size="small"
                items={[
                  {
                    key: 'entities',
                    label: <span className="event-link">展开命中实体（{ents.length}）</span>,
                    children: (
                      <ul className="retrieval-hits">
                        {ents.map((e, ei) => (
                          <li key={ei} className="retrieval-hit">
                            <div className="retrieval-meta">
                              <span className="retrieval-doc" title={e.definition}>{e.label}</span>
                              {e.match && <span className="retrieval-seq">{matchText[e.match] ?? e.match}召回</span>}
                            </div>
                            {(e.relations ?? []).length > 0 && (
                              <div className="retrieval-excerpt">
                                {(e.relations ?? []).map((r, ri) => (
                                  <span key={ri} style={{ marginRight: 10 }}>
                                    {r.dir === 'out' ? (
                                      <>
                                        「{r.rel}」→ {r.other}
                                      </>
                                    ) : (
                                      <>
                                        {r.other} →「{r.rel}」
                                      </>
                                    )}
                                    {r.hop === 2 ? '（2跳）' : ''}
                                  </span>
                                ))}
                              </div>
                            )}
                          </li>
                        ))}
                      </ul>
                    ),
                  },
                ]}
              />
            )}
            {showRaw && it.evData && <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>}
          </div>
        )
      }
      const hits: KBHit[] = Array.isArray(it.evData?.hits) ? it.evData.hits : []
      return (
        <div key={i} className="event-card src-retrieval retrieval-card">
          <span>{it.eventText}</span>
          {hits.length > 0 && (
            <Collapse
              ghost
              size="small"
              items={[{
                key: 'hits',
                label: <span className="event-link">展开命中片段</span>,
                children: (
                  <ul className="retrieval-hits">
                    {hits.map((h, hi) => (
                      <li key={hi} className="retrieval-hit">
                        <div className="retrieval-meta">
                          <span className="retrieval-doc" title={h.doc}>{h.doc}</span>
                          <span className="retrieval-seq">#{h.seq}</span>
                          <span className="retrieval-score">{typeof h.score === 'number' ? h.score.toFixed(3) : '—'}</span>
                        </div>
                        {/* B1 引用溯源：excerpt 命中区间句级高亮（spans 缺失如实不标） */}
                        <div className="retrieval-excerpt">
                          <HighlightSpans text={h.excerpt} spans={h.spans} />
                        </div>
                        {/* REQ-311 A3：wiki 页溯源（REQ-241 后端已返回，此前未渲染） */}
                        {(h.sources ?? []).length > 0 && (
                          <div className="retrieval-meta" style={{ marginTop: 2 }}>
                            <span className="retrieval-seq">来源页：{(h.sources ?? []).map((s) => `${s.doc || '页面'}#${s.seq}`).join('、')}</span>
                          </div>
                        )}
                      </li>
                    ))}
                  </ul>
                ),
              }]}
            />
          )}
          {showRaw && it.evData && <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>}
        </div>
      )
    }

    // REQ-311 A6：上下文账单卡（预算/历史/裁剪压缩/注入明细——模型本轮上下文构成可解释）
    if (it.evType === 'context.stats') {
      const inj: Array<{ source?: string; messages?: number; chars?: number }> = Array.isArray(it.evData?.injections) ? it.evData.injections : []
      const budget = it.evData?.budget_tokens ? `${it.evData.budget_tokens} tok` : '不限量'
      const pruned = (it.evData?.pruned_count ?? 0) > 0
        ? `${it.evData.pruned_count} 条${it.evData?.summarized ? '（LLM 压缩摘要持久化）' : '（直接裁剪，原文仍在对话记录）'}`
        : '未发生'
      return (
        <div key={i} className="event-card src-builtin" style={depthStyle(it.subDepth)}>
          <span>{it.eventText}</span>
          <Collapse
            ghost
            size="small"
            items={[{
              key: 'bill',
              label: <span className="event-link">上下文账单明细</span>,
              children: (
                <div style={{ fontSize: 11, color: 'var(--ant-color-text-secondary, #888)', display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <span>预算档：{it.evData?.budget_mode ?? '—'}（{budget}）</span>
                  <span>历史上下文：{it.evData?.history_messages ?? '—'} 条 · {it.evData?.history_chars ?? 0} 字 · 约 {it.evData?.history_tokens ?? '—'} tok</span>
                  <span>裁剪/压缩：{pruned}</span>
                  <span>
                    注入：{inj.length > 0
                      ? inj.map((x) => `${injectSourceLabel(x.source)} ${x.messages} 条消息/${x.chars} 字`).join('；')
                      : '本次无外部注入（未挂知识库/伴生或零命中）'}
                  </span>
                </div>
              ),
            }]}
          />
          {showRaw && it.evData && <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>}
        </div>
      )
    }

    const assembly = it.evType === 'run.started' ? it.evData?.assembly : undefined
    return (
      <div key={i} className={`event-card src-${eventSource(it.evType, it.evData)}${it.eventErr ? ' err' : ''}${it.eventWarn ? ' warn' : ''}`} style={depthStyle(it.subDepth)}>
        <span>{it.eventText}</span>
        {assembly && (
          <Collapse
            ghost
            size="small"
            items={[{
              key: 'assembly',
              label: <span className="event-link">装配快照（{assembly.mode} · {(assembly.agents ?? []).length} 个智能体）</span>,
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {(assembly.agents ?? []).map((a: any, ai: number) => (
                    <div key={ai}>
                      <div style={{ fontSize: 11, color: 'var(--ant-color-text-secondary, #888)', marginBottom: 2 }}>
                        [{ai + 1}] {a.name}（{a.role}）· {a.model} ｜ 工具 {(a.tools ?? []).map((x: any) => x.name).join(', ') || '无'} ｜ 技能 {(a.skills ?? []).join(', ') || '无'} ｜ MCP {(a.mcp ?? []).join(', ') || '无'}
                      </div>
                      {/* REQ-311 A2：提示词装配透镜——最终系统指令按来源分段（基座/技能目录/项目目录/本体指引） */}
                      {(Array.isArray(a.instruction_segments) && a.instruction_segments.length > 0) ? (
                        <Collapse
                          ghost
                          size="small"
                          items={a.instruction_segments.map((s: any, si: number) => ({
                            key: `${ai}-${si}`,
                            label: (
                              <span style={{ fontSize: 11 }}>
                                <Tag color={SEG_COLORS[s?.source] ?? 'default'} style={{ margin: '0 6px 0 0', fontSize: 10 }}>{SEG_LABELS[s?.source] ?? s?.source}</Tag>
                                {s?.chars ?? 0} 字
                              </span>
                            ),
                            children: <pre className="raw-json" style={{ maxHeight: 220, overflow: 'auto' }}>{s?.text}</pre>,
                          }))}
                        />
                      ) : a.instruction ? (
                        <pre className="raw-json" style={{ maxHeight: 200, overflow: 'auto' }}>{`指令: ${a.instruction}`}</pre>
                      ) : null}
                    </div>
                  ))}
                </div>
              ),
            }]}
          />
        )}
        {it.evType === 'ontology.unavailable' && (it.evData?.reason != null || it.evData?.detail != null) && (
          <div className="event-detail">{String(it.evData.reason ?? it.evData.detail)}</div>
        )}
        {isTool && it.evData && (
          <Collapse
            ghost
            size="small"
            items={[{
              key: 'detail',
              label: <span className="event-link">详情</span>,
              children: <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>,
            }]}
          />
        )}
        {showRaw && it.evData && !isTool && (
          <pre className="raw-json">{JSON.stringify(it.evData, null, 2)}</pre>
        )}
      </div>
    )
  }

  // Bubble.List 数据（消息走 user/ai 角色，事件卡为无边框自定义内容）
  // REQ-174：连续 tool.call/result 归组为「执行过程」块（单行/收起）后再映射
  // REQ-217④ 顺修：granularity/showReasoning 此前已接线（状态与记忆在）但 displayItems 只应用
  // levelGated——两开关从未生效；现补过程门控（key=工具与运行状态，off=仅消息结论）。
  // REQ-323：连续 companion.* 沉淀过程卡再归组——处理中展开、完成自动收起（可手动展开），治多窗沉淀卡刷屏；
  // running 透传 groupToolPhases——运行期间最新工具组恒展开（叶级执行细节可见）
  // REQ-330④：companion 管线事件已不入流（live 归胶囊）——回放态折为轻量摘要行
  // REQ-362：groupRunBlocks 整轮归组——run.started…终态成功完成自动收起为一行任务小结，
  // 最终回答外显其下（时间交错由 applyRunEvent 正文分段承载；异常态平铺不收起）
  const displayItems = useMemo(
    () =>
      groupRunBlocks(
        groupToolPhases(
          summarizeCompanionPhases(
            withSubDepth(
              items.filter((it) => {
                if (it.evType && levelGated(it.evType, debugLevel)) return false
                if (!showReasoning && it.evType === 'reasoning') return false
                if (it.kind === 'msg' || !it.evType) return true
                if (granularity === 'off') return false
                if (granularity === 'key') {
                  return isKeyGranularity(it.evType) // REQ-401：白名单收敛事件注册表单源
                }
                return true
              }),
            ),
          ),
          running,
        ),
        running,
      ),
    [items, debugLevel, granularity, showReasoning, running],
  )

  // REQ-362：块体内回看条目→React 节点（中间正文段淡一档呈现/工具组/事件卡），与平铺映射共用
  const renderStreamItem = (it: any, key: string): ReactNode => {
    if (it.kind === 'msg') {
      return (
        <div key={key} className="run-seg" style={depthStyle(it.subDepth)}>
          <XMarkdown className="chat-md" content={String(it.content ?? '')} openLinksInNewTab />
        </div>
      )
    }
    if ((it as unknown as { kind: string }).kind === '__tool_phase') {
      return <ToolPhaseBlock key={key} group={it as unknown as ToolPhaseGroup} />
    }
    if ((it as unknown as { kind: string }).kind === '__companion_summary') {
      return companionSummaryNode(it as unknown as CompanionSummaryItem, key)
    }
    return renderEventCard(it, Number(key) || 0)
  }

  // REQ-362：归组条目→Bubble.List 数据（done 运行块=收起摘要+外显最终回答；live/ended 块体平铺）
  const toBubbleList = (grouped: any[], prefix: string): any[] => {
    const out: any[] = []
    let n = 0
    const walk = (list: any[]) => {
      for (const it of list) {
        if (isRunBlock(it)) {
          if (it.status === 'done') {
            const bi = n++
            out.push({ key: `${prefix}rb${bi}`, role: 'event', content: <RunBlockView block={it} renderItem={renderStreamItem} /> })
            if (it.answer) {
              out.push({
                key: `${prefix}rba${bi}`,
                role: 'ai',
                content: it.answer.content ?? '',
                loading: false,
                extraInfo: { streaming: false },
              })
            }
            // REQ-478②：轮次元信息条（文件 N/耗时/tokens/收尾时刻）——回答之下、下一问之上
            out.push({ key: `${prefix}rbm${bi}`, role: 'event', content: <RunMetaLine summary={it.summary} /> })
          } else {
            walk(it.body) // live/ended：按时间顺序平铺（时间交错呈现）
          }
          continue
        }
        // REQ-483②：run.started「运行开始」卡不再渲染（计时行承接；带装配快照的调试态保留）
        if (it.kind === 'event' && it.evType === 'run.started' && !it.evData?.assembly) continue
        const ki = n++
        if (it.kind === 'msg') {
          out.push({
            key: `${prefix}m${ki}`,
            role: it.role === 'user' ? 'user' : 'ai',
            content: it.content ?? '',
            loading: !!it.streaming && !it.content,
            extraInfo: { streaming: !!it.streaming }, // 供 contentRender 判断流式状态（尾部游标）
          })
          continue
        }
        if ((it as unknown as { kind: string }).kind === '__tool_phase') {
          out.push({ key: `${prefix}t${ki}`, role: 'event', content: <ToolPhaseBlock group={it as unknown as ToolPhaseGroup} /> })
          continue
        }
        if ((it as unknown as { kind: string }).kind === '__companion_summary') {
          out.push({ key: `${prefix}cs${ki}`, role: 'event', content: companionSummaryNode(it as unknown as CompanionSummaryItem, `${prefix}cs${ki}`) })
          continue
        }
        out.push({ key: `${prefix}e${ki}`, role: 'event', content: renderEventCard(it, ki) })
      }
    }
    walk(grouped)
    // REQ-483③：运行计时行入流（流尾一行灰字+分隔线，随流滚动——原左下悬浮胶囊退役）
    if (running && runStart != null) {
      out.push({
        key: `${prefix}working`,
        role: 'event',
        content: (
          <div className="chat-workingline" data-testid="chat-workingline">
            <span>工作中 {fmtWall(Date.now() - runStart)}</span>
          </div>
        ),
      })
    }
    return out
  }
  const listItems = useMemo(
    () => toBubbleList(displayItems, ''),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [displayItems, showRaw, reasoningOpen, granularity, showReasoning, debugLevel, running, runStart, nowTick],
  )

  // REQ-150② 窗格状态对齐：从窗格消息流派生本轮状态（流式中/出错/已停止/完成/待提问），窗格头统一呈现
  const paneStatus = (i: number): 'idle' | 'streaming' | 'error' | 'stopped' | 'done' => {
    const list = paneItems[i] ?? []
    if (!list.length) return 'idle'
    const last = list[list.length - 1]
    if (last.kind === 'msg' && last.streaming) return 'streaming'
    let err = false
    let stopped = false
    for (let k = list.length - 1; k >= 0; k--) {
      const it = list[k]
      if (it.kind === 'msg' && it.role === 'user') break // 只看本轮（最后一条提问之后）
      if (it.kind === 'event') {
        if (it.evType === 'run.error') err = true
        if (it.evType === 'run.finished' && it.evData?.reason === 'stopped') stopped = true
      }
    }
    if (err) return 'error'
    // 主动停止：正常路径 run.finished(reason=stopped) 事件卡命中；断流兜底路径命中收尾写入的停止文案
    if (stopped) return 'stopped'
    const lastMsg = list[list.length - 1]
    if (lastMsg.kind === 'msg' && lastMsg.role === 'assistant' && lastMsg.content === '（已停止生成）') return 'stopped'
    return 'done'
  }

  // REQ-19e 窗格消息流条目（与单路 listItems 同构映射：消息走角色、事件卡无边框；
  // REQ-174 同构归组 + REQ-362 整轮运行块同构——完成收起/最终回答外显与主区一致）
  // REQ-407/M203：对比窗格列 memoize（此前每帧 render 全量重算归组管线）
  const paneListItems = useCallback(
    (i: number) =>
      toBubbleList(
        groupRunBlocks(groupToolPhases(withSubDepth(paneItems[i] ?? []), running), running),
        `p${i}`,
      ),
    [paneItems, running],
  )

  // 中断恢复（M11 收尾 + REQ-14 审批）：会话挂起的 ask_human 提问 / 工具审批（随会话数据同步）
  const [interrupt, setInterrupt] = useState<{
    kind: 'ask_human' | 'approval'
    question: string
    choices: string[]
    toolName: string
    arguments: string
  } | null>(null)
  const [answer, setAnswer] = useState('')
  // REQ-206/M41 S2：外部推理后端权限转审批的挂起卡（approval.request → 决议 → 清卡）
  const [extApprovalState, setExtApproval] = useState<{ toolName: string; kind: string } | null>(null)
    // REQ-217①/M48：调用轨迹面板（重放并入面板——头部「重放」按钮退役，EventReplayDrawer 作面板内动作）
  const [traceOpen, setTraceOpen] = useState(false)
  // REQ-403②：后台委派面板（委派列表/插话/打断/收结果）
  const [delegatesOpen, setDelegatesOpen] = useState(false)
  // REQ-406②/M202：轨迹抽屉打开时预选的过滤组（伴生摘要行点击直达）
  const [traceFocusGroup, setTraceFocusGroup] = useState<string | null>(null)
  useEffect(() => {
    // REQ-406③/M202：挂起状态收敛单源——此处内联映射退役，与 SSE（run.interrupted）/
    // 对比窗格同走 toInterruptState（三路写入→单一翻译函数）
    try {
      const st = conversation.interrupt_state ? JSON.parse(conversation.interrupt_state) : null
      setInterrupt(st && (st.question || st.tool_name) ? toInterruptState(st) : null)
    } catch {
      setInterrupt(null)
    }
  }, [conversation.id, conversation.interrupt_state])

  // 运行/恢复共用的事件翻译（send 与 resume 的 SSE 处理一致；归约见模块级 applyRunEvent）
  // REQ-311 A4：迭代进度——k=模型调用步累计（model.step），N=协调者生效迭代上限（run.started 透出）
  const [iterStat, setIterStat] = useState<{ k: number; n: number | null } | null>(null)
  const handleRunEvent = ({ event, data }: { event: string; data: any }) => {
    const payload = data?.data ?? {}
    if (event === 'run.interrupted') setInterrupt(toInterruptState(payload))
    // REQ-206/M41 S2：外部推理后端（deepseek-harness-acp）权限转审批——approval.request 挂起
    // 等待决议卡；run 终态（finished/error）时挂起随 run 进程结束自动失效，清卡。
    if (event === 'approval.request') setExtApproval({ toolName: payload.tool_name ?? '', kind: payload.kind ?? '' })
    if (event === 'run.finished' || event === 'run.error') setExtApproval(null)
    if (event === 'run.started') setIterStat({ k: 0, n: typeof payload.max_iteration_effective === 'number' ? payload.max_iteration_effective : null })
    if (event === 'model.step') setIterStat((p) => ({ k: (p?.k ?? 0) + 1, n: p?.n ?? null }))
    if (event === 'run.finished' || event === 'run.error') setIterStat(null)
    setItems((prev) => applyRunEvent(prev, event, payload, runKeyRef.current))
  }

  // 外部权限决议（POST ext-approval；后端把决议映射为 ACP optionId/cancelled 应答，turn 继续）
  const resolveExt = async (decision: 'approve' | 'deny') => {
    try {
      await extApproval(conversation.id, decision)
      setExtApproval(null)
    } catch (e: any) {
      showToast(e?.message ?? '决议失败', 'err')
    }
  }

  // streamStart 运行/恢复公共段：追加流式助手消息 → 消费 SSE → 收尾刷新
  const streamStart = async (
    start: (handler: (ev: { event: string; data: any }) => void) => { abort: () => void; done: Promise<void> },
  ) => {
    stopFlagRef.current = false
    setRunStart(Date.now()) // REQ-477③：运行计时行起点
    setItems((prev) => [...prev, { kind: 'msg', role: 'assistant', content: '', streaming: true }])
    setRunning(true)
    const aborter = start(handleRunEvent)
    runRef.current = aborter
    try {
      await aborter.done
    } catch { /* 用户中断 */ }
    // 运行结束：reasoning 卡收起（保留内容，可手动展开）；主动停止断流时终态事件收不到，兜底收尾流式消息
    setIterStat(null) // REQ-311 A4：迭代进度随 run 终态清除
    setItems((prev) => prev.map((it) => {
      if (it.streamKey) return { ...it, streamKey: undefined }
      if (it.kind === 'msg' && it.streaming) {
        return stopFlagRef.current ? { ...it, streaming: false, content: it.content || '（已停止生成）' } : { ...it, streaming: false }
      }
      return it
    }))
    setRunning(false)
    setRunStart(null)
    runRef.current = null
    bumpData()
    onConversationUpdated()
    // REQ-281：伴生抽取在收尾后旁路异步执行（LLM 一轮 60~90s），SSE 已关——
    // 收尾后短轮询补齐沉淀过程事件；REQ-283 C：门控扩 project 作用域（REQ-281 边界闭合——
    // 伴生事件按会话归属，项目会话中绑定伴生的成员智能体产出的沉淀同样实时可见）；未绑定零轮询。
    if (isProjectScope || convAgent?.companion_ontology) startCompanionPoll(conversation.id)
  }

  // REQ-409/M205：消息级操作——复制 / 重新生成（最后一条 user 消息原样重发）/ 编辑重发
  //（沿 Q-6 不追溯语义：不删历史，自当前消息开新轮）
  // REQ-478③：引用（blockquote 预填输入框）/ 派生会话（fork 端点拷贝配置与全量消息，创建后直跳）
  const copyMessage = async (content: string) => {
    try {
      await navigator.clipboard.writeText(content)
      showToast('已复制')
    } catch {
      showToast('复制失败（浏览器剪贴板不可用）', 'err')
    }
  }
  const quoteMessage = (content: string) => {
    const t = content.length > 220 ? `${content.slice(0, 220)}…` : content
    const q = t.split('\n').map((l) => `> ${l}`).join('\n')
    setInput((v) => (v ? `${v}\n${q}\n\n` : `${q}\n\n`))
    showToast('已引用到输入框')
  }
  const [forking, setForking] = useState(false)
  const forkSession = async () => {
    if (forking || running) return
    setForking(true)
    try {
      const c = await api.forkConversation(conversation.id)
      bumpData()
      setCurrentConv(c.id)
      showToast(`已派生新会话「${c.title}」——历史已带上，过程事件留原会话`)
    } catch (e: any) {
      showToast(e?.message ?? '派生会话失败', 'err')
    } finally {
      setForking(false)
    }
  }
  const lastUserText = (): string | null => {
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i]
      if (it.kind === 'msg' && it.role === 'user' && it.content) return it.content
    }
    // 回放态 items 为空时从消息历史兜底（conversation.messages 不在 props——走 listMessages）
    return null
  }
  const regenerate = async () => {
    if (running) return
    const text = lastUserText()
    if (!text) {
      showToast('未找到可重新生成的提问', 'err')
      return
    }
    if (cmp.on && cmp.panes.length >= 2) {
      await sendCompare(text)
      return
    }
    runKeyRef.current = `run-${Date.now()}`
    setItems((prev) => [...prev, { kind: 'msg', role: 'user', content: text }])
    await streamStart((handler) => runConversation(conversation.id, text, debugLevel, handler, undefined, debugPersist))
  }
  const [editingMsg, setEditingMsg] = useState<string | null>(null)
  const [editText, setEditText] = useState('')
  // REQ-409/M205：消息级操作菜单挂 user 气泡（X Bubble messageRender 尾缀操作行）
  const bubbleRoles = useMemo<BubbleListProps['role']>(() => {
    const roles: BubbleListProps['role'] = { ...BUBBLE_ROLES }
    roles.user = {
      ...roles.user,
      contentRender: (content: any) => {
        const text = typeof content === 'string' ? content : String(content ?? '')
        // REQ-483④：操作钮图标化——hover 显现灰色小图标（原常驻蓝色链接行）
        return (
          <div data-testid="user-msg" className="msg-actions-host">
            <div style={{ whiteSpace: 'pre-wrap' }}>{text}</div>
            <Space size={0} className="msg-actions user-actions">
              <Button type="text" size="small" icon={<CopyOutlined />} title="复制" aria-label="复制" data-testid="user-copy" onClick={() => copyMessage(text)} />
              <Button type="text" size="small" icon={<EditOutlined />} title="编辑重发" aria-label="编辑重发" data-testid="user-edit" onClick={() => startEditMsg(text)} />
            </Space>
          </div>
        )
      },
    }
    roles.ai = {
      ...roles.ai,
      contentRender: (content: any, info: any) => {
        const text = typeof content === 'string' ? content : String(content ?? '')
        // REQ-477④ 顺修：REQ-409 挂消息操作时覆盖丢了 XMarkdown——助手回答自近期版本起纯文本
        // 直出（markdown 裸符号可见）；恢复 Markdown 渲染（流式游标口径同 BUBBLE_ROLES.ai），操作行保留
        return (
          <div data-testid="ai-msg" className="msg-actions-host">
            <XMarkdown
              className="chat-md"
              content={text}
              openLinksInNewTab
              streaming={{ hasNextChunk: !!info?.extraInfo?.streaming, tail: true }}
            />
            {/* REQ-483④：操作行图标化——灰色小图标+Tooltip（原常驻蓝色链接×4） */}
            <Space size={0} className="msg-actions ai-actions">
              <Button type="text" size="small" icon={<CopyOutlined />} title="复制" aria-label="复制" data-testid="ai-copy" onClick={() => copyMessage(text)} />
              <Button type="text" size="small" icon={<AlignLeftOutlined />} title="引用到输入框" aria-label="引用" data-testid="quote-btn" onClick={() => quoteMessage(text)} />
              {!isProjectScope && (
                <Button type="text" size="small" icon={<BranchesOutlined />} title="派生会话（拷贝当前历史继续对话）" aria-label="派生会话" data-testid="fork-btn" disabled={forking} onClick={forkSession} />
              )}
              <Button type="text" size="small" icon={<RedoOutlined />} title="重新生成" aria-label="重新生成" data-testid="regen-btn" disabled={running} onClick={regenerate} />
            </Space>
          </div>
        )
      },
    }
    return roles
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, running, cmp.on, cmp.panes.length, forking, conversation.id])
  const startEditMsg = (content: string) => {
    setEditingMsg(content)
    setEditText(content)
  }
  const resendEdited = async () => {
    const text = editText.trim()
    if (!text || running) return
    setEditingMsg(null)
    if (cmp.on && cmp.panes.length >= 2) {
      await sendCompare(text)
      return
    }
    runKeyRef.current = `run-${Date.now()}`
    setItems((prev) => [...prev, { kind: 'msg', role: 'user', content: text }])
    await streamStart((handler) => runConversation(conversation.id, text, debugLevel, handler, undefined, debugPersist))
  }

  const send = async () => {
    let text = input.trim()
    if (!text || running) return
    if (!isProjectScope && !agent) return
    if (isProjectScope && !agent) {
      showToast('项目还没有成员智能体，请先在项目配置中添加成员', 'err')
      return
    }
    // REQ-336：附加文件随消息发送——文本附文件清单（文件本体已在发送前上传入智能体工作目录）
    if (attachFiles.length > 0) {
      text += `\n（附加文件：${attachFiles.map((f) => f.name).join('、')} —— 已存入工作目录，可直接读取）`
    }
    setInput('')
    setAttachFiles([])
    setSlashOpen(false)
    // REQ-19e：对比模式走多窗格并行（一次提问 N 路，停止整组）
    if (cmp.on && cmp.panes.length >= 2) {
      await sendCompare(text)
      return
    }
    runKeyRef.current = `run-${Date.now()}`
    setItems((prev) => [...prev, { kind: 'msg', role: 'user', content: text }])
    // REQ-291：模型偏好已落会话行（后端装配读 conversation.model_conn_id），不再逐请求下发
    await streamStart((handler) => runConversation(conversation.id, text, debugLevel, handler, undefined, debugPersist))
  }

  // REQ-336：附加文件上传——入智能体文件根（未设置 work_dir 落默认 agent-files 目录），发送时附清单引用
  const uploadAttachments = async (files: FileList | null) => {
    const agentId = convAgent?.id
    if (!agentId || !files || files.length === 0) return
    setAttaching(true)
    try {
      for (const f of Array.from(files)) {
        const fd = new FormData()
        fd.append('file', f)
        const r = await fetch(`/api/agents/${agentId}/files`, { method: 'POST', body: fd })
        if (!r.ok) {
          const b = await r.json().catch(() => ({}))
          showToast(`附件 ${f.name} 上传失败：${b?.error ?? r.status}`, 'err')
          continue
        }
        const b = await r.json()
        setAttachFiles((prev) => [...prev, { name: b.path ?? f.name, size: Number(b.size ?? f.size) }])
      }
    } finally {
      setAttaching(false)
    }
  }

  // REQ-19e/19f 对比发送：共用输入框一次提问 → N 路并行；meta 建窗格 run_id 映射，事件按 run_id 路由
  const sendCompare = async (text: string) => {
    stopFlagRef.current = false
    const panesPayload: ComparePaneConfig[] = cmp.panes.map((p) => ({
      ...(p.agent ? { agent_id: p.agent } : {}),
      ...(p.model ? { model_conn_id: p.model } : {}),
      ...(p.kb ? { kb_id: p.kb } : {}),
      ...(p.profile ? { runtime_profile_id: p.profile } : {}),
      ...(p.temperature != null ? { temperature: p.temperature } : {}),
      ...(p.instruction ? { instruction: p.instruction } : {}),
      ...(p.skills ? { enable_skills: p.skills === 'on' } : {}),
      ...(p.noHistory ? { no_history: true } : {}),
    }))
    paneRunMapRef.current = {}
    setRunStart(Date.now()) // REQ-477③：对比轮同挂运行计时行
    setPaneItems((prev) =>
      prev.map(() => [
        { kind: 'msg' as const, role: 'user' as const, content: text },
        { kind: 'msg' as const, role: 'assistant' as const, content: '', streaming: true },
      ]),
    )
    setRunning(true)
    const aborter = runConversation(conversation.id, text, debugLevel, handleCompareEvent, panesPayload, debugPersist)
    runRef.current = aborter
    try {
      await aborter.done
    } catch { /* 用户中断 */ }
    // 主动停止断流时各窗格收不到 run.finished，兜底把残留流式消息置终态（REQ-150② 状态对齐）
    setPaneItems((prev) => prev.map((p) => p.map((it) => {
      if (it.streamKey) return { ...it, streamKey: undefined }
      if (it.kind === 'msg' && it.streaming) {
        return stopFlagRef.current ? { ...it, streaming: false, content: it.content || '（已停止生成）' } : { ...it, streaming: false }
      }
      return it
    })))
    setRunning(false)
    setRunStart(null)
    runRef.current = null
    bumpData()
    onConversationUpdated()
  }

  // 对比 SSE 路由：meta 建映射 → 事件按 data.run_id 落到对应窗格；无映射的组级事件（守卫错误）广播全部窗格
  const handleCompareEvent = ({ event, data }: { event: string; data: any }) => {
    if (event === 'meta') {
      if (Array.isArray(data?.panes)) {
        const m: Record<string, number> = {}
        for (const p of data.panes) {
          if (p?.run_id != null && typeof p.pane === 'number') m[String(p.run_id)] = p.pane
        }
        paneRunMapRef.current = m
      }
      return
    }
    const payload = data?.data ?? {}
    const idx = paneRunMapRef.current[String(data?.run_id ?? '')]
    if (event === 'run.interrupted' && idx !== undefined) setInterrupt(toInterruptState(payload))
    if (idx === undefined) {
      setPaneItems((prev) => prev.map((p, i) => applyRunEvent(p, event, payload, `pane-${i}`)))
      return
    }
    updatePane(idx, (prev) => applyRunEvent(prev, event, payload, `pane-${idx}`))
  }

  // resume 答复挂起中断（ask_human 自由答复 / 审批 批准|拒绝），事件流与运行同构
  const resume = async (decision?: string) => {
    const isApproval = interrupt?.kind === 'approval'
    const text = isApproval ? (decision ?? '') : answer.trim()
    if (!text || running || !interrupt) return
    setAnswer('')
    setInterrupt(null)
    runKeyRef.current = `resume-${Date.now()}`
    setItems((prev) => [
      ...prev,
      {
        kind: 'msg' as const,
        role: 'user' as const,
        content: isApproval
          ? `[审批] ${decision === 'approve' ? '批准' : '拒绝'} · ${interrupt.toolName}`
          : text,
      },
    ])
    await streamStart((handler) => resumeConversation(conversation.id, text, debugLevel, handler, debugPersist))
  }

  // M17 阶段二：事件流导出（JSON 全量，供归档/外部重放）
  const exportEventsJSON = async () => {
    try {
      const evs = await api.listEvents(conversation.id)
      const blob = new Blob([JSON.stringify(evs, null, 2)], { type: 'application/json;charset=utf-8' })
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `${(conversation.title || '对话').replace(/[\/:*?"<>|]/g, '_')}-events.json`
      a.click()
      URL.revokeObjectURL(a.href)
    } catch (e: any) {
      showToast(e?.message ?? '导出失败', 'err')
    }
  }

  const stop = () => {
    stopFlagRef.current = true // REQ-150②：断流后终态事件收不到，收尾按此置「已停止」
    runRef.current?.abort()
    api.stopConversation(conversation.id).catch(() => {})
  }

  // REQ-113①：导出对话为 Markdown 下载（events=含过程事件附录）
  const exportMarkdown = async (withEvents: boolean) => {
    try {
      const md = await api.exportConversation(conversation.id, withEvents)
      const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' })
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `${(conversation.title || '对话').replace(/[\\/:*?"<>|]/g, '_')}.md`
      a.click()
      URL.revokeObjectURL(a.href)
    } catch (e: any) {
      showToast(e?.message ?? '导出失败', 'err')
    }
  }

  const subjectName = isProjectScope ? project?.name : agent?.name
  // REQ-403/M199：会话级模型覆盖选项源 + 生效审批档人话标签
  const [modelConns, setModelConns] = useState<{ id: string; name: string }[]>([])
  useEffect(() => {
    api
      .listConnections()
      .then((cs) => setModelConns((cs as any[]).filter((c: any) => c.conn_type === 'chat' && c.enabled).map((c: any) => ({ id: c.id as string, name: c.name as string }))))
      .catch(() => setModelConns([]))
  }, [])
  const agentApprovalLabel = agent?.tool_approval === 'danger' ? '危险工具审批' : agent?.tool_approval === 'all' ? '全部审批' : '关闭（直接执行）'
  const canSend = isProjectScope ? !!project : !!agent
  const placeholder = canSend
    ? `给「${subjectName}」发消息…`
    : isProjectScope
      ? '项目尚未配置成员智能体'
      : '请先创建智能体'

  return (
    <div className="chat">
      <div className="chat-header">
        {isProjectScope && project ? (
          <>
            <Typography.Text strong className="subject">项目：{project.name}</Typography.Text>
            <Tag>会话使用的智能体：{agent ? agent.name : '未配置成员'}</Tag>
          </>
        ) : (
          <Typography.Text strong className="subject">{agent ? agent.name : '对话'}</Typography.Text>
        )}
        {isProjectScope && project && <Tag>{project.collab_mode}</Tag>}
        {!isProjectScope && agent && (
          <>
            <Tag>{agent.runtime_backend}</Tag>
            {/* REQ-336：会话伴生/检索参数/审批覆盖归位会话设置（头部⋮）；「最大迭代」静态标签已移除 */}
          </>
        )}
        {/* REQ-336：会话名称右侧「⋮」=会话设置面板（重命名/伴生/审批/检索参数/智能体配置一屏全览；
            会话级配置归位单一入口——替代原散布的 Dropdown 子菜单+嵌套弹窗方案） */}
        <Popover
          trigger="click"
          open={convSetOpen}
          onOpenChange={(o) => {
            setConvSetOpen(o)
            if (o) setRenameVal(conversation.title ?? '')
          }}
          placement="bottomLeft"
          arrow={false}
          content={
            <ConvSettingsPanel
              conversation={conversation}
              patchConv={patchConv}
              renameVal={renameVal}
              setRenameVal={setRenameVal}
              modelConns={modelConns}
              agentApprovalLabel={agentApprovalLabel}
              agent={agent}
              companionOntos={companionOntos}
              isProjectScope={isProjectScope}
              project={project}
              subjectName={subjectName}
              onToggleSidePanel={onToggleSidePanel}
              onClose={() => setConvSetOpen(false)}
              onCompanionChange={(id) => {
                void patchConv({ companion_ontology_id: id }, { silent: true }).then(() => {
                  showToast(id ? '本会话伴生已切换（抽取/召回走该本体；其他会话不受影响）' : '已清除覆盖——本会话跟随智能体默认伴生本体')
                })
              }}
            />
          }
        >
          <Button size="small" type="text" icon={<EllipsisOutlined />} aria-label="会话设置" data-testid="conv-more" title="会话设置（重命名 / 审批 / 检索参数 / 伴生 / 智能体配置）" />
        </Popover>
        {/* REQ-311 A4：运行中迭代进度（k=模型调用步累计，N=生效迭代上限；项目会话同样可见） */}
        {running && iterStat && (iterStat.k > 0 || iterStat.n != null) && (
          <Tooltip title="k=本次运行模型调用步累计（model.step）；N=协调者/智能体生效迭代上限">
            <Tag color="processing" style={{ margin: 0 }}>迭代 {iterStat.k}{iterStat.n ? `/${iterStat.n}` : ''}</Tag>
          </Tooltip>
        )}
        <span className="spacer" />
        <Space size={4} className="chat-ops">
          {/* REQ-113①：对话导出 Markdown（仅消息 / 含过程事件附录） */}
          <Dropdown
            menu={{
              items: [
                { key: 'msg', label: '导出 Markdown（仅消息）' },
                { key: 'full', label: '导出 Markdown（含过程事件）' },
                { key: 'events-json', label: '导出事件流（JSON）' },
              ],
              onClick: ({ key }) => {
                if (key === 'events-json') exportEventsJSON()
                else exportMarkdown(key === 'full')
              },
            }}
            disabled={!conversation.id}
          >
            <Button size="small">导出</Button>
          </Dropdown>
          {/* REQ-323：「过程显示」（聊天流展示多少）与「观测/采集档」（后端记录多少）并为一个「过程」入口——
              两档同面板+关系说明自解释（采集决定记多少、显示决定看多少，采集仅影响之后的运行）。
              替代 REQ-322 头部并列的过程显示 Segmented + 观测 Dropdown（开发者问「是否重复了」）。
              TraceDrawer「显示与运行治理」原入口保留（同源 convCfg/会话记忆） */}
          <Popover
            trigger="click"
            placement="bottomRight"
            arrow={false}
            content={
              <ObservePanel
                granularity={granularity}
                patchConvCfg={patchConvCfg}
                showReasoning={showReasoning}
                showRaw={showRaw}
                setShowRaw={setShowRaw}
                debugLevel={debugLevel}
                changeDebugLevel={changeDebugLevel}
                debugPersist={debugPersist}
              />
            }
          >
            <Button size="small" data-testid="proc-btn" title="过程显示（看多少）与采集档（记多少）——面板内有关系说明">
              过程 · {granularity === 'all' ? '全部' : granularity === 'key' ? '关键' : '精简'}{debugLevel > 0 ? ` · ${['简洁', '详细', '调试'][debugLevel] ?? debugLevel}` : ''}{debugPersist ? '+入库' : ''}
            </Button>
          </Popover>
          {/* REQ-217①②④/M48：调用轨迹入口（重放/过程展示/调试三档/原始 JSON 收编面板，头部精简） */}
          <Button size="small" onClick={() => setTraceOpen(true)}>调用轨迹</Button>
          {/* REQ-403②/M199 余项：后台委派入口层级提升（原仅模型经 delegate_* 工具可见，用户无管理面） */}
          <Button size="small" data-testid="delegate-entry" onClick={() => setDelegatesOpen(true)}>后台委派</Button>
          {/* REQ-19e 对比模式：开关 + 2~4 列（运行中锁定；开关与窗格配置按会话记忆） */}
          <Tooltip title="对比模式：一次提问多窗格并行，窗格可分别覆盖模型/知识库/运行方案——学习配置差异对同一问题的影响（SC-10）">
            <span className="cmp-switch" aria-label="对话对比模式开关">
              <Switch size="small" checked={cmp.on} disabled={running || !conversation.id} onChange={(v) => patchCmp({ on: v })} />
              <span className="cmp-switch-label">对比</span>
            </span>
          </Tooltip>
          {cmp.on && (
            <Segmented
              size="small"
              value={String(cmp.panes.length)}
              disabled={running}
              onChange={(v) => patchCmp({ panes: resizePanes(cmp.panes, Number(v)) })}
              options={[
                { value: '2', label: '2 列' },
                { value: '3', label: '3 列' },
                { value: '4', label: '4 列' },
              ]}
            />
          )}
          {/* REQ-336：方块状「智能体侧边栏」开关退役——侧板入口归会话设置 ⋮「智能体配置」（原 REQ-135① 收进入口的再归位） */}
        </Space>
      </div>

      <div className={`msg-list${running ? ' running' : ''}`}>
        {/* REQ-330②：伴生沉淀悬浮胶囊（右上角；管线过程事件与本职工作事件解耦，旁路呈现独立于显示档） */}
        {capsuleEvents.length > 0 && <CompanionCapsule events={capsuleEvents} renderEvent={renderEventCard} />}
        {cmp.on ? (
          <>
            {/* 对比开启前/外的对话历史（共享，折叠收纳；对比轮次的各窗格回答也会落库进此历史） */}
            {items.length > 0 && (
              <Collapse
                ghost
                size="small"
                className="cmp-history"
                items={[{
                  key: 'h',
                  label: <Typography.Text type="secondary" style={{ fontSize: 12 }}>对比外的对话历史（{items.length} 条，收起以聚焦本轮对照）</Typography.Text>,
                  children: <Bubble.List items={listItems} role={bubbleRoles} />,
                }]}
              />
            )}
            <Splitter className="cmp-splitter">
              {cmp.panes.map((sel, i) => {
                const inh = paneAgentOf(sel)
                // REQ-150：继承/覆盖显性化——徽标 Tooltip 显示继承的实际来源值
                // REQ-291：模型继承面纳入会话级覆盖（合并序 窗格 > 会话 > 智能体 > 全局）
                const inhModelConn = conversation.model_conn_id || inh?.model_conn_id
                const paneInherit = {
                  model: inhModelConn
                    ? (conns.find((c) => c.id === inhModelConn) ? connDisplayName(conns.find((c) => c.id === inhModelConn)!) : '会话级覆盖连接')
                    : '全局默认连接',
                  kb: conversation.kb_id
                    ? (kbs.find((k) => k.id === conversation.kb_id)?.name ?? '已绑定')
                    : '未绑定（不注入）',
                  profile: conversation.runtime_profile_id
                    ? (profiles.find((pp) => pp.id === conversation.runtime_profile_id)?.name ?? '已绑定')
                    : '未绑定',
                  skills: (conversation.enable_skills ?? true) ? '开启' : '关闭',
                }
                return (
                <Splitter.Panel key={i} min="15%">
                <ComparePane
                  index={i}
                  sel={sel}
                  paneAgent={inh}
                  paneInherit={paneInherit}
                  status={paneStatus(i)}
                  hasItems={(paneItems[i] ?? []).length > 0}
                  conns={conns}
                  kbs={kbs}
                  profiles={profiles}
                  agentOptions={agentOptions}
                  cmpProfiles={cmpProfiles}
                  running={running}
                  canAdopt={!isProjectScope && !!conversation.agent_id && !running}
                  bubbleRoles={bubbleRoles}
                  paneListItems={paneListItems}
                  onAdopt={adoptPane}
                  onPaneSel={setPaneSel}
                  onApplyProfile={applyProfile}
                  onSaveProfile={saveProfile}
                  onCopyPrev={copyPrevPane}
                />
                </Splitter.Panel>
                );
              })}
            </Splitter>
          </>
        ) : items.length === 0 ? (
          <div className="msg-empty">
            <Welcome
              variant="borderless"
              icon={<span className="agent-tile"><AgentLogo agent={agent} size={30} context="runtime" /></span>}
              title={`开始与「${subjectName ?? '智能体'}」对话`}
              description={
                isProjectScope
                  ? '项目会话由成员智能体协作处理；「配置」中可维护项目成员与协作模式（M4 起生效）。'
                  : '消息将流式返回；「配置」中可调整系统提示词、模型与采样参数（下次运行生效）。'
              }
            />
          </div>
        ) : (
          <>
            {/* REQ-311 B4/M115：todo 产物卡——todo_write 机器侧清单（conversation.todo_json），run 收尾刷新可见 */}
            <TodoCard todoJson={conversation.todo_json} />
            {/* REQ-478③：主路径切 action-enabled roles（复制/引用/派生会话/重新生成+用户消息操作行）——原主路径无操作钮 */}
            <Bubble.List items={listItems} role={bubbleRoles} />
          </>
        )}
        {/* REQ-483③：悬浮「停止生成」与「工作中」胶囊退役——计时行入流（listItems 尾）、停止由 Sender 加载态内建钮承接 */}
      </div>

      {/* REQ-206/M41 S2：外部推理后端（deepseek-harness-acp）权限转审批卡——
          与中断审批卡不同：run 仍在进行（SSE 保持打开），决议即时送回 dsh 继续本轮 */}
      {extApprovalState && (
        <div className="composer chat-interrupt">
          <Alert
            type="warning"
            showIcon
            title={`外部权限请求（${agent?.inference_backend === 'deepseek-harness-acp' ? 'DeepSeek Harness · ACP' : '外部 Harness'}）：${extApprovalState.toolName}`}
            description={
              <div className="chat-interrupt-body">
                <Space size={8}>
                  <Button type="primary" onClick={() => resolveExt('approve')}>批准本次</Button>
                  <Button danger onClick={() => resolveExt('deny')}>拒绝</Button>
                </Space>
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                  由外部推理后端发起的工具权限请求，按当前审批档转由你决议；批准后工具在外部侧执行。停止运行等同拒绝。
                </Typography.Text>
              </div>
            }
          />
        </div>
      )}

      {/* 中断恢复（M11 收尾 + REQ-14 审批）：ask_human 答复卡 / 工具审批卡 */}
      {interrupt && !running && (
        <div className="composer chat-interrupt">
          {interrupt.kind === 'approval' ? (
            <Alert
              type="warning"
              showIcon
              title={`工具调用等待审批：${interrupt.toolName}`}
              description={
                <div className="chat-interrupt-body">
                  <pre className="chat-interrupt-args">{interrupt.arguments || '（无参数）'}</pre>
                  <Space size={8}>
                    <Button type="primary" onClick={() => resume('approve')}>批准并执行</Button>
                    <Button danger onClick={() => resume('deny')}>拒绝</Button>
                  </Space>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    批准后工具将真实执行；拒绝会把拒答结果返回给智能体。也可直接发送新消息（放弃本次审批）。
                  </Typography.Text>
                </div>
              }
            />
          ) : (
            <Alert
              type="warning"
              showIcon
              title={`智能体需要你的输入：${interrupt.question}`}
              description={
                <div className="chat-interrupt-body">
                  {interrupt.choices.length > 0 && (
                    <Space size={6} wrap>
                      {interrupt.choices.map((c) => (
                        <Button key={c} size="small" onClick={() => setAnswer(c)}>{c}</Button>
                      ))}
                    </Space>
                  )}
                  <Space.Compact style={{ width: '100%' }}>
                    <Input
                      value={answer}
                      onChange={(e) => setAnswer(e.target.value)}
                      placeholder="输入你的答复…"
                      onPressEnter={() => resume()}
                      autoFocus
                    />
                    <Button type="primary" onClick={() => resume()} disabled={!answer.trim()}>答复并继续</Button>
                  </Space.Compact>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    也可直接发送新消息（将放弃本次提问，按新问题运行）。
                  </Typography.Text>
                </div>
              }
            />
          )}
        </div>
      )}

      <Composer
        input={input}
        setInput={setInput}
        slashOpen={slashOpen}
        setSlashOpen={setSlashOpen}
        attachFiles={attachFiles}
        setAttachFiles={setAttachFiles}
        attaching={attaching}
        convAgentId={convAgent?.id}
        running={running}
        canSend={canSend}
        placeholder={placeholder}
        runningProfiles={runningProfiles}
        kbs={kbs}
        agentSkills={agentSkills}
        ontoDisabled={ontoDisabled}
        kbDisabled={kbDisabled}
        skillsDisabled={skillsDisabled}
        picker={picker}
        setPicker={setPicker}
        pickerList={pickerList}
        kbChip={kbChip}
        ontoChip={ontoChip}
        skillsChip={skillsChip}
        kbHint={kbHint}
        ontoHint={ontoHint}
        skillsHint={skillsHint}
        cmpOn={cmp.on}
        cmpPaneCount={cmp.panes.length}
        conversationKbId={conversation.kb_id}
        conversationProfileId={conversation.runtime_profile_id}
        conversationOntologyEnabled={conversation.ontology_enabled}
        patchConv={patchConv as never}
        onSend={send}
        onStop={stop}
        onUploadAttachments={(f) => void uploadAttachments(f)}
      />

      {/* REQ-336：会话设置——重命名（头部⋮入口） */}
      {/* REQ-336：会话设置——会话检索参数（自知识库 chip Popover 迁入；随会话保存，0=跟随知识库默认） */}
      {/* REQ-144：剖面命名保存（对话级） */}
      <Modal
        open={profileSaveFor != null}
        centered
        title={`保存窗格 ${(profileSaveFor ?? 0) + 1} 配置为剖面`}
        width={420}
        onCancel={() => { setProfileSaveFor(null); setProfileName('') }}
        onOk={doSaveProfile}
        okText="保存"
        cancelText="取消"
      >
        <Input
          value={profileName}
          onChange={(e) => setProfileName(e.target.value)}
          placeholder="剖面名称（对话级，如：低温度-严格事实）"
          onPressEnter={doSaveProfile}
          autoFocus
        />
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
          剖面捕获：智能体/模型/知识库/方案/温度/提示词改写/技能/历史口径。应用时按「剖面 &gt; 窗格单项 &gt; 智能体 &gt; 全局」合并。
        </Typography.Text>
      </Modal>

      {/* REQ-409/M205：编辑重发 Modal（Q-6 不追溯语义——自当前消息开新轮） */}
      <Modal
        open={editingMsg !== null}
        title="编辑重发"
        okText="重发"
        onCancel={() => setEditingMsg(null)}
        onOk={resendEdited}
      >
        <Input.TextArea
          rows={4}
          value={editText}
          onChange={(e) => setEditText(e.target.value)}
          data-testid="edit-msg-area"
        />
      </Modal>

      {/* REQ-217①②/M48：调用轨迹面板（显示设置/运行治理收编；重放为面板内动作） */}
      {traceOpen && conversation.id && (
        <TraceDrawer
          conversationId={conversation.id}
          title={conversation.title || subjectName || '对话'}
          open={traceOpen}
          onClose={() => setTraceOpen(false)}
          debugLevel={debugLevel}
          showRaw={showRaw}
          onShowRawChange={setShowRaw}
          focusGroup={traceFocusGroup ?? undefined}
          onFocusGroupConsumed={() => setTraceFocusGroup(null)}
        />
      )}
      <DelegatePanel conversationId={conversation.id} open={delegatesOpen} onClose={() => setDelegatesOpen(false)} />
    </div>
  )
}
