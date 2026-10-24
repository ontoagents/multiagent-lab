import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { Avatar, Alert, Button, Checkbox, Collapse, Dropdown, Input, InputNumber, Modal, Popover, Segmented, Select, Space, Splitter, Switch, Tag, Tooltip, Typography } from 'antd'
import { BookOutlined, BulbOutlined, CaretDownOutlined, CheckSquareOutlined, ClusterOutlined, EllipsisOutlined, LoadingOutlined, PaperClipOutlined, StopOutlined, ThunderboltOutlined, UserOutlined } from '@ant-design/icons'
import { Bubble, Sender, ThoughtChain, Welcome } from '@ant-design/x'
import type { BubbleListProps } from '@ant-design/x'
import XMarkdown from '@ant-design/x-markdown'
import { api, connDisplayName, extApproval, resumeConversation, runConversation } from '../api/client'
import type { ComparePaneConfig } from '../api/client'
import TraceDrawer from './TraceDrawer'
import HighlightSpans from './HighlightSpans'
import { AgentLogo } from './AgentLogo'
import { groupToolPhases, ToolPhaseBlock, type ToolPhaseGroup } from './ToolPhase'
import { groupRunBlocks, RunBlockView, isRunBlock } from './RunBlock'
import type {
  Agent,
  Conversation,
  KBHit,
  KnowledgeBase,
  Message,
  ModelConnection,
  Project,
  RuntimeProfile,
} from '../api/types'
import { useUI } from '../store/ui'

interface ChatItem {
  kind: 'msg' | 'event'
  role?: Message['role']
  content?: string
  metaAgent?: string
  eventText?: string
  eventErr?: boolean
  eventWarn?: boolean
  streaming?: boolean
  // 执行细节增强（06 §4 执行可观测）
  evType?: string // reasoning | run.started | run.finished | run.warning | run.error | tool.call | tool.result | skill.loaded | subagent.enter | subagent.exit | retrieval | ontology.query | ontology.unavailable
  evData?: any // 事件 data（解析后，供详情展开与调试面板）
  reasoning?: string // 深度思考累积内容
  evKey?: string // 稳定卡片键（深度思考展开状态按它记录，历史/实时各自生成）
  streamKey?: string // 运行中的 reasoning 卡合并键；运行结束置空收起
  subDepth?: number // REQ-117：子智能体嵌套深度（缩进渲染）
}

// REQ-19f/143 对比窗格单项覆盖（UI 本地形态：''= 继承对话当前配置；提交时映射 ComparePaneConfig）
interface PaneSel {
  agent: string // ''= 继承对话所属/项目主智能体；否则窗格级独立装配（REQ-143）
  model: string
  kb: string
  profile: string
  noHistory: boolean // REQ-143③：不携带对话历史（干净对照）
  temperature: number | null // REQ-144：推理参数覆盖（null=继承）
  instruction: string // REQ-144：系统提示词临时改写（''=继承）
  skills: '' | 'on' | 'off' // REQ-144：技能开关（''=继承，REQ-19g 口径）
}
const BLANK_PANE: PaneSel = { agent: '', model: '', kb: '', profile: '', noHistory: false, temperature: null, instruction: '', skills: '' }

// 子 Agent 名（§6.5 subagent.enter/exit payload = 子 Agent 名；字段名做兼容取值）
function subagentName(d: any): string {
  return String(d?.name ?? d?.agent_name ?? d?.sub_agent ?? d?.agent ?? '').trim()
}

// 事件卡文案（实时流与历史回放共用：新增事件必须在此登记，两条路径才一致）。
// 导出供事件重放视图（M17 阶段二 EventReplayDrawer）复用同一文案源。
export function describeEvent(type: string, d: any): { text: string; err?: boolean; warn?: boolean } {
  switch (type) {
    case 'run.started':
      return { text: `▶ 运行开始 · ${d?.agent_name ?? ''} · ${d?.model ?? ''}`.replace(/ ·\s*$/, '') }
    case 'run.finished':
      if (d?.reason === 'verify_failed') {
        return { text: `⚠ 运行结束但验证未通过（verify_failed）${typeof d?.elapsed_ms === 'number' ? ` · ${(d.elapsed_ms / 1000).toFixed(1)}s` : ''}——未标记完成，可检查 verify 命令后重试`, warn: true }
      }
      return d?.reason === 'stopped' ? { text: '⏹ 已停止' } : d?.reason === 'interrupted' ? { text: '⏸ 已挂起 · 等待答复' } : { text: finishSummary(d) }
    // M11 收尾 + REQ-14 审批：中断恢复（ask_human 答复 / 工具审批）
    case 'run.interrupted':
      return d?.kind === 'approval' || (!d?.question && d?.tool_name)
        ? { text: `⏸ 工具审批 · ${d?.tool_name ?? ''}` }
        : { text: `⏸ 等待答复 · ${d?.question ?? ''}` }
    case 'run.warning':
      return { text: `⚠ 运行警告 · ${d?.message ?? ''}`.replace(/ ·\s*$/, ''), warn: true }
    case 'run.error':
      return { text: `⚠ ${d?.message ?? '运行失败'}`, err: true }
    case 'model.step': {
      // REQ-117/M17：模型调用链路（调试观测）
      const parts = [`🧠 模型调用 #${d?.seq ?? '?'}`]
      if (d?.agent) parts.push(String(d.agent))
      if (typeof d?.duration_ms === 'number') parts.push(d.duration_ms >= 1000 ? `${(d.duration_ms / 1000).toFixed(1)}s` : `${d.duration_ms}ms`)
      const u = d?.usage
      if (u && typeof u.total_tokens === 'number') parts.push(`tokens ${u.prompt_tokens ?? 0}+${u.completion_tokens ?? 0}=${u.total_tokens}`)
      if (d?.input_count) parts.push(`输入 ${d.input_count} 条/${d.input_chars ?? 0} 字`)
      return { text: parts.join(' · ') }
    }
    case 'tool.call':
      return { text: `⚙ 调用工具 ${d?.tool_name ?? ''}` }
    case 'tool.result':
      return { text: `⚙ 工具结果 ${d?.tool_name ?? ''}` }
    case 'skill.loaded': {
      // §6.5 v0.6 payload 为 skills[{id,name}]；兼容旧的 skill_name/name 单值写法
      const many = Array.isArray(d?.skills) ? d.skills.map((s: any) => s?.name).filter(Boolean).join('、') : ''
      const one = d?.skill_name ?? d?.name ?? ''
      return { text: `📚 技能 ${many || one}`.replace(/ $/, '') }
    }
    // M4：多智能体协作（AgentAsTool / Transfer）
    case 'subagent.enter': {
      const n = subagentName(d)
      return { text: n ? `↳ 进入子智能体 ${n}` : '↳ 进入子智能体' }
    }
    case 'subagent.exit': {
      const n = subagentName(d)
      return { text: n ? `↳ 子智能体 ${n} 完成` : '↳ 子智能体已完成' }
    }
    // REQ-208/M42 余项：后台委派生命周期（started/completed/failed/stopped/steered）
    case 'subagent.delegate': {
      const who = d?.member ? ` ${d.member}` : ''
      const head = d?.result_head ? ` · ${String(d.result_head).slice(0, 60)}` : ''
      switch (d?.phase) {
        case 'started':
          return { text: `🚀 后台委派${who} · ${String(d?.task ?? '').slice(0, 60)}（不阻塞，可插话/收取）` }
        case 'completed':
          return { text: `✅ 后台委派${who} 完成${head}` }
        case 'failed':
          return { text: `❌ 后台委派${who} 失败 · ${String(d?.error ?? '').slice(0, 80)}`, warn: true }
        case 'stopped':
          return { text: `⏸ 后台委派${who} 已停止（子会话历史完整可恢复）`, warn: true }
        case 'steered':
          return { text: `💬 后台委派${who} 插话 · ${String(d?.note ?? '').slice(0, 60)}` }
        default: {
          // REQ-416/M212：同步委派单按 channel 区分内部/外部 A2A（外部附端点）
          if (d?.channel === 'a2a') {
            const ep = d?.endpoint ? ` · ${String(d.endpoint)}` : ''
            return { text: `🌐 委派外部成员${who}（A2A）· ${String(d?.task ?? '').slice(0, 60)}${ep}` }
          }
          return { text: `🚀 委派成员${who} · ${String(d?.task ?? '').slice(0, 60)}` }
        }
      }
    }
    // REQ-205/M40：工作流节点开始/完成（进度高亮数据源）
    case 'workflow.node': {
      const idx = typeof d?.index === 'number' && d.index > 0 ? `（${d.index}/${d.total}）` : ''
      if (d?.phase === 'completed') return { text: `☑ 工作流节点完成 · ${d?.node ?? ''}${idx}` }
      return { text: `▶ 工作流节点开始 · ${d?.node ?? ''}${idx}［${d?.mode ?? ''}］` }
    }
    // REQ-224 H10：本体消费命中 / ①状态机变更
    case 'ontology.hit':
      return { text: `⚡ 本体命中 · ${d?.tool ?? ''}${d?.target ? ` · ${d.target}` : ''}（${d?.ontology_id ?? ''}）` }
    case 'state.changed':
      return { text: `⏱ 状态变更 · ${d?.field ?? ''} · ${d?.reason ?? ''}${d?.kind ? `（${d.kind}）` : ''}` }
    // M6：知识召回（引用块展开由 renderEventCard 处理）；REQ-194②：伴生图来源单列
    case 'retrieval': {
      if (d?.source === 'companion') {
        const n = Array.isArray(d?.entities) ? d.entities.length : 0
        // 17 号⑦术语分层：用户可见文案以「伴生本体」为主语
        return { text: `🧩 伴生本体召回 · ${n} 实体` }
      }
      const n = Array.isArray(d?.hits) ? d.hits.length : 0
      return { text: `📚 知识召回 · ${n} 条` }
    }
    // REQ-281：伴生沉淀过程事件族（抽取/候选/入图判定/拒绝——收尾后旁路产生，经轮询或回放进流）
    case 'companion.extract': {
      if (d?.phase === 'started') return { text: `🧩 伴生沉淀 · 收尾触发（${d?.fresh ?? 0} 条新消息 · ${d?.windows ?? 0} 窗${d?.pending_windows ? ` · 积压 ${d.pending_windows} 窗待续` : ''}${d?.window_budget ? ` · 每窗预算 ${d.window_budget}s` : ''}）` }
      if (d?.phase === 'done') return { text: `🧩 伴生沉淀完成 · 候选 ${d?.candidates ?? 0} 条${d?.auto_ingested ? ` · 自动入图 ${d.auto_ingested} 条` : ''}` }
      if (d?.phase === 'error') return { text: `🧩 伴生沉淀失败${d?.window ? `（第 ${d.window}/${d?.total ?? '?'} 窗）` : ''} · ${d?.message ?? '未知错误'}`, err: true }
      return { text: '🧩 伴生沉淀 · 抽取中…' }
    }
    case 'companion.candidates': {
      if (!d?.count) return { text: `🧩 伴生沉淀 · 第 ${d?.window ?? '?'}/${d?.total ?? '?'} 窗无可抽内容` }
      return { text: `🧩 伴生候选 · 第 ${d?.window ?? '?'}/${d?.total ?? '?'} 窗新增 ${d.count} 条` }
    }
    case 'companion.decision':
      // REQ-282 A4：自动入图失败变体（decision 已发而候选留 pending 的状态可见可解释）
      if (d?.action === 'auto_ingest_failed') return { text: `🧩 自动入图失败 · ${d?.name ?? ''} · ${d?.error ?? '未知原因'}（候选留待人工）`, warn: true }
      return { text: `🧩 自动入图判定 · ${d?.name ?? ''} · 置信 ${fmtNum(d?.confidence)} ≥ 阈值 ${fmtNum(d?.threshold)} 且分位 ${fmtNum(d?.batch_rank)} ≥ 0.5` }
    case 'companion.ingest': {
      const mode = d?.mode === 'auto' ? '自动入图' : '确认入图'
      const result = d?.result === 'aggregate' ? ` · 印证聚合（第 ${d?.confirm_count ?? '?'} 次确认）` : d?.result === 'conflict_replace' ? ' · 矛盾旧边已失效化' : ''
      return { text: `🧩 伴生${mode} · ${d?.name ?? ''}${result}` }
    }
    case 'companion.reject':
      return { text: `🧩 候选拒绝 · ${d?.name ?? ''}` }
    // M8：本体（经 facade，via=mcp）
    case 'ontology.query':
      return { text: `🔗 本体查询${d?.profile_id ? ` · ${d.profile_id}` : ''}` }
    case 'ontology.unavailable':
      return { text: '⚠ 本体方案不可用 · 已降级', err: true }
    // REQ-224/M52：结构化审计事件（schema_version=2 契约）
    case 'approval.granted':
      return { text: `✅ 审批通过 · ${d?.tool_name ?? ''}${d?.decision_source === 'timeout' ? '（超时自动）' : ''}` }
    case 'approval.denied':
      return { text: `⛔ 审批拒绝 · ${d?.tool_name ?? ''}${d?.decision_source === 'timeout' ? ' · 挂起超时自动拒绝' : ''}`, warn: true }
    case 'hook.denied':
      return { text: `🛡 守卫拦截 · ${d?.guard ?? ''} · ${d?.tool_name ?? ''} · ${d?.reason ?? ''}`, warn: true }
    case 'verify.completed':
      return { text: `🧪 验证通过 · ${d?.command ?? ''}` }
    case 'verify.failed':
      return { text: `🧪 验证未通过 · ${d?.error ?? ''}`, warn: true }
    case 'connector.degraded':
      return { text: `🔌 连接器降级 · ${d?.connector ?? ''} · ${d?.reason ?? ''}`, warn: true }
    // REQ-206/M41 S2：外部推理后端（deepseek-harness-acp）权限转审批
    case 'approval.request':
      return { text: `🔐 外部权限请求 · ${d?.tool_name ?? ''}${d?.kind ? ` · ${d.kind}` : ''}（等待你在下方卡片决议）`, warn: true }
    // REQ-311 A1：Harness 审计族/产物/CLI 补齐 live 呈现（文案与 TraceDrawer 同源）
    case 'artifact.saved':
      return { text: `💾 产物已保存 · ${d?.name ?? d?.path ?? ''}` }
    case 'debug.cli': {
      if (d?.stopReason) return { text: `🖥 CLI · stop=${d.stopReason}` }
      const line = String(d?.line ?? d?.raw ?? '').replace(/\s+/g, ' ').trim()
      return { text: `🖥 CLI${d?.stream ? `(${d.stream})` : ''} · ${line.slice(0, 80) || '输出'}` }
    }
    // REQ-311 A6：上下文账单（历史构成 + 注入明细一次透出，展开见 renderEventCard）
    case 'context.stats': {
      const inj = Array.isArray(d?.injections) ? d.injections : []
      const injTxt = inj.map((x: any) => injectSourceLabel(x?.source)).join('、')
      return { text: `🧾 上下文账单 · 历史 ${d?.history_messages ?? '—'} 条/约 ${d?.history_tokens ?? '—'} tok${inj.length ? ` · 注入 ${injTxt}` : ' · 无外部注入'}` }
    }
    default:
      return { text: `· ${type}` }
  }
}

// REQ-281：数值紧凑展示（0.95→0.95；undefined→'—'），供伴生判定文案
function fmtNum(v: any): string {
  return typeof v === 'number' ? String(Math.round(v * 100) / 100) : '—'
}

// REQ-311 A6：context.stats 注入源人话标签（账单卡与摘要文案共用）
function injectSourceLabel(src?: string): string {
  switch (src) {
    case 'kb': return '知识库召回'
    case 'companion': return '伴生本体召回'
    case 'ontology_guide': return '本体方案指引'
    case 'project_dir': return '项目本地目录'
    default: return src || '其他来源'
  }
}

// run.finished 摘要：耗时 / token 用量 / finish_reason
function finishSummary(d: any): string {  const parts = ['✓ 运行完成']
  if (typeof d?.elapsed_ms === 'number') {
    parts.push(d.elapsed_ms >= 1000 ? `${(d.elapsed_ms / 1000).toFixed(1)}s` : `${d.elapsed_ms}ms`)
  }
  const u = d?.usage
  if (u && typeof u.total_tokens === 'number' && u.total_tokens > 0) {
    parts.push(`tokens ${u.prompt_tokens ?? 0}+${u.completion_tokens ?? 0}=${u.total_tokens}`)
  }
  if (d?.finish_reason) parts.push(`finish=${d.finish_reason}`)
  return parts.join(' · ')
}

type EventSource = 'builtin' | 'skill' | 'mcp' | 'onto' | 'subagent' | 'retrieval'

/**
 * 事件来源分类（原型 06 §10 色彩语义：内置 灰 / 技能 绿 / MCP 紫 / 本体 青 / 异常 红；
 * 子智能体取紫族、知识召回取绿族）。
 * §6.5：tool.call/result 已带 source（builtin | skill:{id} | mcp:{server} | ontology:facade | agent:{id}），按前缀采信；
 * 缺失时回退到 tool_name 命名约定：onto_* → 本体；mcp_ 前缀或 server__tool 双下划线 → MCP。
 */
function eventSource(evType: string | undefined, evData: any): EventSource {
  if (evType === 'skill.loaded') return 'skill'
  if (evType === 'subagent.enter' || evType === 'subagent.exit' || evType === 'subagent.delegate' || evType === 'workflow.node') return 'subagent'
  if (evType === 'retrieval') return 'retrieval'
  if (evType?.startsWith('companion.')) return 'onto' // REQ-281：伴生沉淀归本体青色族（伴生图=本体域资产）
  if (evType === 'ontology.query' || evType === 'ontology.unavailable') return 'onto'
  const src = evData?.source
  if (typeof src === 'string' && src) {
    if (src === 'builtin') return 'builtin'
    if (src.startsWith('skill:')) return 'skill'
    if (src.startsWith('mcp:')) return 'mcp'
    if (src.startsWith('ontology:')) return 'onto'
    if (src.startsWith('agent:')) return 'subagent'
    return 'onto' // 其余 source 视为本体运行方案 profile_id（§6.5）
  }
  const name = String(evData?.tool_name ?? '')
  if (name.startsWith('onto_')) return 'onto'
  if (name.startsWith('mcp_') || name.includes('__')) return 'mcp'
  return 'builtin'
}

// REQ-311 A2：提示词装配透镜——分段来源标签与配色（与后端 instructionSegments source 键对应）
const SEG_LABELS: Record<string, string> = { base: '基座指令', skills: '技能目录', project_dir: '项目本地目录', ontology_guide: '本体方案指引' }
const SEG_COLORS: Record<string, string> = { base: 'blue', skills: 'green', project_dir: 'gold', ontology_guide: 'purple' }

/** REQ-149① 展示级别门控：简洁档（level 0）隐藏调试细节事件（model.step 等），详细/调试档展开 */
export function levelGated(type: string, level: number): boolean {
  return level < 1 && (type === 'model.step' || type === 'debug.cli')
}

/** REQ-150 对比窗格覆盖徽标：覆盖态品牌填充显示值名，继承态浅色；点击经 Popover 选择（继承值显性化） */
function paneBadgeSelect(opts: {
  label: string
  value: string
  valueLabel: string
  inheritText: string
  options: { value: string; label: string }[]
  onChange: (v: string) => void
}) {
  const { label, value, valueLabel, inheritText, options, onChange } = opts
  return (
    <Popover
      trigger="click"
      placement="bottomLeft"
      arrow={false}
      content={
        <div style={{ width: 230 }}>
          <Select
            size="small"
            style={{ width: '100%' }}
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder={`选择${label}（清空 = 继承）`}
            value={value || undefined}
            onChange={(v) => onChange(v ?? '')}
            options={options}
          />
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
            继承：{inheritText}
          </Typography.Text>
        </div>
      }
    >
      <button type="button" className={`cmp-badge clickable${value ? ' set' : ''}`} title={`${label}：${value ? valueLabel : `继承（${inheritText}）`}`}>
        {label}{value ? `·${valueLabel}` : '·继承'}
      </button>
    </Popover>
  )
}

/** 会话挂起中断卡数据（run.interrupted payload → 组件状态；实时与对比窗格共用） */
function toInterruptState(payload: any) {
  return {
    kind: (payload.kind === 'approval' || (!payload.question && payload.tool_name) ? 'approval' : 'ask_human') as 'approval' | 'ask_human',
    question: payload.question ?? '',
    choices: Array.isArray(payload.choices) ? payload.choices : [],
    toolName: payload.tool_name ?? '',
    arguments: payload.arguments ?? '',
  }
}

/**
 * 单路事件流归约（REQ-19e/19f：单路消息区与对比窗格共用同一事件→条目翻译）。
 * runKey 为本次流的自定义合并键（reasoning 卡按它合并增量）。
 * REQ-362 时间交错：运行中正文按模型步分段——过程事件到达时关闭当前正文段（空段弹出，
 * 防幽灵打字气泡），后续正文增量另起新段；呈现「思考卡→工具行→中间回答段→…」真实时序。
 */
function applyRunEvent(prev: ChatItem[], event: string, payload: any, runKey: string): ChatItem[] {
  switch (event) {
    case 'run.started':
    case 'run.finished': {
      const desc = describeEvent(event, payload)
      const next = [...prev]
      if (event === 'run.finished') {
        const ti = next.length - 1
        const last = next[ti]
        if (last && last.kind === 'msg' && last.streaming) {
          if (!last.content) {
            // 空段：停止/挂起保留占位文案，其余弹出（无正文即无气泡）
            if (payload.reason === 'stopped' || payload.reason === 'interrupted') {
              next[ti] = { ...last, streaming: false, content: payload.reason === 'stopped' ? '（已停止生成）' : '（已暂停，等待你的答复）' }
            } else {
              next.pop()
            }
          } else {
            next[ti] = { ...last, streaming: false }
          }
        }
      }
      const ev: ChatItem = { kind: 'event', evType: event, eventText: desc.text, evData: payload }
      if (event === 'run.finished') {
        // REQ-330⑤：完成卡排在流式助手消息之后（回放按时间戳本就合并到答案下方——live 对齐回放位，
        // 治「完成卡插答案上方、自动滚动钉尾时在视口外，切会话才可见」）
        next.push(ev)
      } else {
        next.splice(next.length - 1, 0, ev) // run.started 卡在答案之前（回放同位），保留尾部打字指示气泡
      }
      return next
    }
    case 'run.interrupted': {
      const desc = describeEvent(event, payload)
      const next = [...prev]
      closeTailSegment(next)
      next.push({ kind: 'event', evType: event, eventText: desc.text, evData: payload })
      return next
    }
    case 'reasoning.delta': {
      const next = [...prev]
      closeTailSegment(next)
      const last = next[next.length - 1]
      // REQ-362：思考卡按步拆分——仅当末项就是本次运行的思考卡才合并，否则新开一张
      // （多步运行的步间工具事件天然隔开各步思考卡；evKey 带步序号保证展开状态稳定）
      if (last && last.kind === 'event' && last.evType === 'reasoning' && last.streamKey === runKey) {
        next[next.length - 1] = { ...last, reasoning: (last.reasoning ?? '') + (payload.delta ?? '') }
        return next
      }
      const step = next.filter((x) => x.kind === 'event' && x.evType === 'reasoning' && x.streamKey === runKey).length
      next.push({ kind: 'event', evType: 'reasoning', reasoning: payload.delta ?? '', streamKey: runKey, evKey: `${runKey}-${step}` })
      return next
    }
    case 'message.delta': {
      const next = [...prev]
      const last = next[next.length - 1]
      if (last && last.kind === 'msg' && last.streaming) {
        next[next.length - 1] = { ...last, content: last.content + (payload.delta ?? '') }
        return next
      }
      // 当前无开放正文段：另起新段（REQ-362 时间交错——新段总在最新过程事件之后）
      next.push({ kind: 'msg', role: 'assistant', content: payload.delta ?? '', streaming: true })
      return next
    }
    case 'run.error': {
      const msg = payload.message ?? '运行失败'
      const next = [...prev]
      const ti = next.length - 1
      const last = next[ti]
      if (last && last.kind === 'msg' && last.streaming) {
        if (!last.content) next.pop()
        else next[ti] = { ...last, streaming: false }
      }
      const ev: ChatItem = { kind: 'event', evType: 'run.error', eventErr: true, eventText: `⚠ ${msg}`, evData: payload }
      return [...next, ev]
    }
    default:
      // 过程类事件（工具/技能/子智能体/知识召回/本体）：REQ-362——关闭/弹出尾部正文段后
      // 追加到流尾（正文段自此按事件边界分段，时间顺序即呈现顺序）
      // REQ-311 A1：Harness 审计族（审批决议/守卫拦截/验证背压/连接器降级）与产物/CLI/上下文账单
      // 进 live 流——事件本已随 SSE 到达，此前被白名单静默丢弃、运行中黑盒只能事后翻轨迹
      if (
        event === 'tool.call' || event === 'tool.result' || event === 'skill.loaded' ||
        event === 'subagent.enter' || event === 'subagent.exit' ||
        event === 'retrieval' || event === 'ontology.query' || event === 'ontology.unavailable' ||
        event === 'run.warning' || event === 'model.step' || event === 'approval.request' ||
        event === 'approval.granted' || event === 'approval.denied' || event === 'hook.denied' ||
        event === 'verify.completed' || event === 'verify.failed' || event === 'connector.degraded' ||
        event === 'artifact.saved' || event === 'debug.cli' || event === 'context.stats'
      ) {
        const desc = describeEvent(event, payload)
        const next = [...prev]
        closeTailSegment(next)
        next.push({ kind: 'event', evType: event, eventText: desc.text, eventErr: desc.err, eventWarn: desc.warn, evData: payload })
        return next
      }
      return prev
  }
}

/** REQ-362：关闭尾部开放正文段——有内容置为完结段（时间交错分段点），空段弹出（防幽灵打字气泡） */
function closeTailSegment(list: ChatItem[]): void {
  const ti = list.length - 1
  const last = list[ti]
  if (!last || last.kind !== 'msg' || !last.streaming) return
  if (!last.content) list.pop()
  else list[ti] = { ...last, streaming: false }
}

/** 会话配置 chip（渲染在 Sender footer 内）：ON = 品牌填充，OFF = 浅色描边；禁用置灰 + Tooltip 说明 */
function ChatChip({ on, disabled, icon, label, title, onClick }: {
  on: boolean
  disabled: boolean
  icon: ReactNode
  label: string
  title: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className={`chat-chip${on ? ' on' : ''}${disabled ? ' disabled' : ''}`}
      aria-pressed={on}
      aria-disabled={disabled || undefined}
      title={disabled ? undefined : title}
      onClick={onClick}
    >
      <span className="chat-chip-icon">{icon}</span>
      <span>{label}</span>
    </button>
  )
}

/** REQ-330④：回放态伴生管线摘要行（REQ-323 的可折叠归组块退役——live 过程由悬浮胶囊承载，
 *  回放只留一行轻量摘要；全量过程卡归「调用轨迹」伴生分类与 AgentCompanionManage） */
interface CompanionSummaryItem {
  kind: '__companion_summary'
  n: number
  cand: number
  ingested: number
  failed: boolean
  subDepth: number
}

const isCompanionPipelineEv = (it: any) =>
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
function summarizeCompanionPhases(items: any[]): any[] {
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
function CompanionCapsule({ events, renderEvent }: { events: any[]; renderEvent: (it: any, i: number) => ReactNode }) {
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

// Bubble 角色映射（X 2.x：role 单数；条目 role 必须命中此处定义的 key）
const BUBBLE_ROLES: BubbleListProps['role'] = {
  user: {
    placement: 'end',
    avatar: <Avatar icon={<UserOutlined />} style={{ background: 'var(--c-brand)', color: '#fff' }} />,
    styles: { content: { background: 'var(--c-brand)', color: '#fff', borderRadius: 12, borderBottomRightRadius: 4 } },
  },
  ai: {
    placement: 'start',
    // 智能体标识：与顶栏品牌同源的三节点网络标记（侧栏节点 / 空态保持一致）
    avatar: (
      <Avatar shape="square" className="agent-avatar">
        <span className="agent-glyph" />
      </Avatar>
    ),
    // REQ-256 bugfix：气泡随主题（原硬编码 #fff——暗色时白底叠 darkAlgorithm 白字不可读）
    styles: { content: { background: 'var(--c-panel)', border: '1px solid var(--c-line)', borderRadius: 12, borderBottomLeftRadius: 4 } },
    // 助手正文走 Markdown（XMarkdown）：流式期间尾部游标，hasNextChunk=false 时收尾刷新
    contentRender: (content, info) => (
      <XMarkdown
        className="chat-md"
        content={String(content ?? '')}
        openLinksInNewTab
        streaming={{ hasNextChunk: !!info.extraInfo?.streaming, tail: true }}
      />
    ),
  },
  event: {
    variant: 'borderless',
    styles: {
      root: { paddingBlock: 4 },
      content: { padding: 0, background: 'transparent', width: '100%', maxWidth: '100%' },
    },
  },
}

/**
 * 中间对话窗口（原型 06 §3.1 / §3.2，Ant Design X Bubble/Sender/ThoughtChain）：
 * - 垂直三段：配置条（单行徽标）/ 消息流（Bubble.List 内建滚动 + autoScroll 置底）/ 输入区；
 * - 阅读尺度：气泡自身 max-width 880 居中（滚动条因此贴对话区最右缘），输入区同宽对齐；
 * - 助手正文：XMarkdown 渲染（流式期间尾部游标），用户消息保持纯文本；
 * - 运行中：消息流右下角悬浮「停止生成」入口，复用 stop()（abort + POST /stop）；
 * - agent 直聊：头部显示智能体名，「配置」打开智能体弹窗；
 * - project 会话：头部注明「项目：xxx · 会话使用的智能体：xxx」；
 * - 执行细节：历史事件回放、token 用量与耗时、深度思考 ThoughtChain、工具调用 JSON、原始事件调试开关
 * - M4-M8：子智能体 / 知识召回 / 本体事件卡（实时与回放共用 describeEvent）；
 *   会话级配置为输入卡内底部的三个开关 chip（知识库 / 本体 / 技能，X Sender footer），
 *   随改随存（merge-safe patchConv）；具体参数在「知识库」页与智能体属性中维护
 */
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
  const { bumpData, showToast } = useUI()
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
            // 已以段代显的 run：段入 timeline（淡一档正文段，与 live 交错形态对称）；
            // 未消费（如消息已删）也照常呈现，不丢内容
            timeline.push({
              ts: e.created_at,
              item: { kind: 'event', evType: 'message.segment', evData: { run_id: e.run_id, text: d.text ?? '' } },
            })
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
            item: { kind: 'event', evType: e.type, eventText: desc.text, eventErr: desc.err, eventWarn: desc.warn, evData: d },
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

  // 事件卡渲染（ThoughtChain 深度思考 / 工具详情 / 终态摘要）：
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
    // REQ-406①/M202：回放正文段（message.segment 以段代显的交错正文——淡一档 run-seg 呈现，
    // 与 live 模型步分段形态对称）
    if (it.evType === 'message.segment') {
      return (
        <div key={i} className="run-seg" data-testid="replay-text-segment" style={{ whiteSpace: 'pre-wrap', fontSize: 13.5, lineHeight: 1.72, padding: '2px 4px' }}>
          {it.evData?.text ?? ''}
        </div>
      )
    }
    if (it.evType === 'reasoning') {
      const len = it.reasoning?.length ?? 0
      const cardKey = it.evKey ?? `reasoning-${i}`
      const streaming = !!it.streamKey
      const open = reasoningOpen[cardKey] ?? streaming // 流式中默认展开；结束后默认收起，用户可手动开合
      return (
        <ThoughtChain
          key={i}
          className="event-side"
          style={depthStyle(it.subDepth)}
          expandedKeys={open ? [cardKey] : []}
          onExpand={(keys) => setReasoningOpen((prev) => ({ ...prev, [cardKey]: keys.includes(cardKey) }))}
          items={[
            {
              key: cardKey,
              title: `深度思考${len ? `（${len} 字）` : ''}`,
              status: streaming ? 'loading' : 'success',
              icon: <BulbOutlined />,
              collapsible: true,
              content: len ? <pre className="thinking">{it.reasoning}</pre> : undefined,
            },
          ]}
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
                if (it.evType === 'message.segment') return true // REQ-406①：正文段以段代显，恒显示
                if (granularity === 'off') return false
                if (granularity === 'key') {
                  return it.evType.startsWith('tool.') || it.evType.startsWith('run.') || it.evType === 'artifact.saved' ||
                    ['approval.granted', 'approval.denied', 'hook.denied', 'verify.completed', 'verify.failed', 'connector.degraded', 'context.stats'].includes(it.evType)
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
          } else {
            walk(it.body) // live/ended：按时间顺序平铺（时间交错呈现）
          }
          continue
        }
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
    return out
  }
  const listItems = useMemo(
    () => toBubbleList(displayItems, ''),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [displayItems, showRaw, reasoningOpen, granularity, showReasoning, debugLevel],
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
  const copyMessage = async (content: string) => {
    try {
      await navigator.clipboard.writeText(content)
      showToast('已复制')
    } catch {
      showToast('复制失败（浏览器剪贴板不可用）', 'err')
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
        return (
          <div data-testid="user-msg">
            <div style={{ whiteSpace: 'pre-wrap' }}>{text}</div>
            <Space size={4} style={{ marginTop: 4 }}>
              <Button type="link" size="small" style={{ color: 'rgba(255,255,255,.85)', fontSize: 11, padding: 0 }} onClick={() => copyMessage(text)}>
                复制
              </Button>
              <Button type="link" size="small" style={{ color: 'rgba(255,255,255,.85)', fontSize: 11, padding: 0 }} onClick={() => startEditMsg(text)}>
                编辑重发
              </Button>
            </Space>
          </div>
        )
      },
    }
    roles.ai = {
      ...roles.ai,
      contentRender: (content: any) => {
        const text = typeof content === 'string' ? content : String(content ?? '')
        return (
          <div data-testid="ai-msg">
            <div style={{ whiteSpace: 'pre-wrap' }}>{text}</div>
            <Space size={4} style={{ marginTop: 4 }}>
              <Button type="link" size="small" style={{ fontSize: 11, padding: 0 }} onClick={() => copyMessage(text)}>
                复制
              </Button>
              <Button type="link" size="small" style={{ fontSize: 11, padding: 0 }} disabled={running} onClick={regenerate}>
                重新生成
              </Button>
            </Space>
          </div>
        )
      },
    }
    return roles
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, running, cmp.on, cmp.panes.length])
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
            <div className="conv-settings" data-testid="conv-settings">
              <div className="proc-pop-title">会话设置</div>
              <div className="cs-row">
                <Input
                  size="small"
                  value={renameVal}
                  onChange={(e) => setRenameVal(e.target.value)}
                  placeholder="会话名称"
                  onPressEnter={() => { if (renameVal && renameVal !== conversation.title) void patchConv({ title: renameVal }) }}
                />
                <Button size="small" data-testid="conv-rename-save" onClick={() => { if (renameVal && renameVal !== conversation.title) void patchConv({ title: renameVal }) }}>保存</Button>
              </div>
              <div className="chip-picker-divider" />
              <div className="proc-pop-title">工具调用审批（会话级覆盖）</div>
              <Segmented
                size="small"
                value={conversation.tool_approval ?? ''}
                onChange={(v) => patchConv({ tool_approval: v as string })}
                options={[
                  { value: '', label: '跟随' },
                  { value: 'on', label: '开启' },
                  { value: 'off', label: '关闭' },
                ]}
              />
              {/* REQ-403/M199：会话生效配置总览（合并序一屏可见）+ 会话级模型覆盖入口恢复（REQ-291 数据层在、REQ-336 退役 UI 入口） */}
              <div className="proc-pop-title">会话级模型覆盖（REQ-291）</div>
              <Select
                size="small"
                allowClear
                value={conversation.model_conn_id || undefined}
                placeholder="跟随智能体默认"
                onChange={(v) => patchConv({ model_conn_id: v ?? '' })}
                options={(modelConns ?? []).map((c) => ({ value: c.id, label: c.name }))}
                data-testid="conv-model-override"
                style={{ width: '100%' }}
              />
              <div className="proc-pop-title">生效配置总览（合并序：会话 &gt; 智能体 &gt; 全局）</div>
              <div style={{ fontSize: 11.5, lineHeight: 1.9 }}>
                <div>
                  模型：{conversation.model_conn_id ? <Tag color="purple" style={{ margin: 0, fontSize: 10 }}>会话覆盖</Tag> : <Tag style={{ margin: 0, fontSize: 10 }}>智能体默认</Tag>}
                  {(modelConns ?? []).find((c) => c.id === conversation.model_conn_id)?.name ?? subjectName ?? '跟随'}
                </div>
                <div>
                  审批：{conversation.tool_approval === 'off' ? '会话关闭' : conversation.tool_approval === 'on' ? '会话开启' : '跟随智能体'}
                  {conversation.tool_approval !== 'off' && conversation.tool_approval !== 'on' && (
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>（{agentApprovalLabel}）</Typography.Text>
                  )}
                </div>
                <div>
                  上下文预算：{conversation.context_mode ? <Tag color="purple" style={{ margin: 0, fontSize: 10 }}>会话覆盖</Tag> : <Tag style={{ margin: 0, fontSize: 10 }}>智能体档</Tag>}
                  {conversation.context_mode || '跟随'}
                </div>
              </div>
              <div className="proc-pop-title">上下文预算档（会话级覆盖，REQ-393）</div>
              <Segmented
                size="small"
                value={conversation.context_mode ?? ''}
                onChange={(v) => patchConv({ context_mode: v as string })}
                options={[
                  { value: '', label: '跟随' },
                  { value: 'compact', label: '紧凑' },
                  { value: 'standard', label: '标准' },
                  { value: 'full', label: '完整' },
                ]}
                data-testid="conv-context-mode"
              />
              <div className="proc-pop-title">会话检索参数（0=跟随知识库默认）</div>
              <div className="cs-row">
                <Typography.Text type="secondary" style={{ fontSize: 11.5 }}>top_k</Typography.Text>
                <InputNumber
                  size="small" min={0} max={50} precision={0} style={{ width: 72 }}
                  value={conversation.top_k ?? 0}
                  onChange={(v) => patchConv({ top_k: typeof v === 'number' ? v : 0 })}
                  data-testid="conv-topk"
                />
                <Typography.Text type="secondary" style={{ fontSize: 11.5 }}>min_score</Typography.Text>
                <InputNumber
                  size="small" min={0} max={1} step={0.05} style={{ width: 84 }}
                  value={conversation.min_score ?? 0}
                  onChange={(v) => patchConv({ min_score: typeof v === 'number' ? v : 0 })}
                  data-testid="conv-minscore"
                />
              </div>
              {!isProjectScope && agent?.companion_ontology_id && (
                <>
                  <div className="chip-picker-divider" />
                  <div className="proc-pop-title">会话伴生本体（覆盖仅本会话生效）</div>
                  <Select
                    size="small"
                    style={{ width: '100%' }}
                    value={conversation.companion_ontology_id || ''}
                    onChange={(id) => {
                      void patchConv({ companion_ontology_id: id }, { silent: true }).then(() => {
                        showToast(id ? '本会话伴生已切换（抽取/召回走该本体；其他会话不受影响）' : '已清除覆盖——本会话跟随智能体默认伴生本体')
                      })
                    }}
                    options={[
                      { value: '', label: `跟随智能体默认${agent.companion_ontology_id ? `（${companionOntos.find((o) => o.id === agent.companion_ontology_id)?.name ?? '已绑定'}）` : ''}` },
                      ...companionOntos
                        .filter((o) => o.id !== agent.companion_ontology_id)
                        .map((o) => ({ value: o.id, label: o.name })),
                    ]}
                  />
                </>
              )}
              {onToggleSidePanel && ((isProjectScope && project) || (!isProjectScope && agent)) && (
                <>
                  <div className="chip-picker-divider" />
                  <Button
                    size="small"
                    block
                    data-testid="conv-open-panel"
                    onClick={() => { setConvSetOpen(false); onToggleSidePanel?.() }}
                  >
                    智能体配置（打开侧边栏）
                  </Button>
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
                    模型 / 技能挂载 / 提示词等智能体级配置在侧边栏维护
                  </Typography.Text>
                </>
              )}
            </div>
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
              <div className="proc-pop" data-testid="proc-pop">
                <div className="proc-pop-title">过程显示 · 聊天流展示多少（即时生效，会话级记忆）</div>
                <Segmented
                  size="small"
                  value={granularity}
                  onChange={(v) => patchConvCfg({ granularity: v as 'all' | 'key' | 'off' })}
                  options={[
                    { value: 'all', label: '全部' },
                    { value: 'key', label: '关键' },
                    { value: 'off', label: '精简' },
                  ]}
                />
                {/* REQ-336：自调用轨迹「显示设置」归并（轨迹页只留轨迹） */}
                <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginTop: 10 }}>
                  <span style={{ fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    深度思考过程 <Switch size="small" checked={showReasoning} onChange={(v) => patchConvCfg({ showReasoning: v })} />
                  </span>
                  <span style={{ fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    原始事件 JSON <Switch size="small" checked={showRaw} onChange={setShowRaw} />
                  </span>
                </div>
                <div className="chip-picker-divider" />
                <div className="proc-pop-title">采集档 · 后端记录多少（仅影响之后的运行）</div>
                <div className="proc-pop-opts">
                  {([
                    { k: 0, label: '简洁（默认）', hint: '常规过程卡' },
                    { k: 1, label: '详细', hint: '另记装配快照/分步用量' },
                    { k: 2, label: '调试', hint: '另附模型输入全文' },
                  ] as const).map((o) => (
                    <button
                      key={o.k}
                      type="button"
                      className={`chip-picker-item${debugLevel === o.k ? ' current' : ''}`}
                      data-testid={`proc-lv${o.k}`}
                      onClick={() => changeDebugLevel(o.k)}
                    >
                      <span className="chip-picker-name">{o.label}{debugLevel === o.k ? ' ✓' : ''}</span>
                      <span className="chip-picker-meta">{o.hint}</span>
                    </button>
                  ))}
                </div>
                <button
                  type="button"
                  className={`chip-picker-item${debugPersist ? ' current' : ''}`}
                  data-testid="proc-persist"
                  onClick={() => patchConvCfg({ debugPersist: !debugPersist })}
                >
                  <span className="chip-picker-name">{debugPersist ? '✓ ' : ''}调试事件入库</span>
                  <span className="chip-picker-meta">轨迹/回放可回看</span>
                </button>
                <div className="proc-pop-hint">
                  两档关系：采集档决定「记多少」，显示档决定「看多少」——「精简」显示下已记录的细节仍可在「调用轨迹」回看（入库后跨会话可查）。
                </div>
              </div>
            }
          >
            <Button size="small" data-testid="proc-btn" title="过程显示（看多少）与采集档（记多少）——面板内有关系说明">
              过程 · {granularity === 'all' ? '全部' : granularity === 'key' ? '关键' : '精简'}{debugLevel > 0 ? ` · ${['简洁', '详细', '调试'][debugLevel] ?? debugLevel}` : ''}{debugPersist ? '+入库' : ''}
            </Button>
          </Popover>
          {/* REQ-217①②④/M48：调用轨迹入口（重放/过程展示/调试三档/原始 JSON 收编面板，头部精简） */}
          <Button size="small" onClick={() => setTraceOpen(true)}>调用轨迹</Button>
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
                <section className="cmp-pane" aria-label={`对比窗格 ${i + 1}`}>
                  <header className="cmp-pane-head">
                    <span className="cmp-pane-title">窗格 {i + 1}</span>
                    <span className={`cmp-pane-status st-${paneStatus(i)}`} aria-label={`窗格 ${i + 1} 状态`}>
                      <i className="dot" aria-hidden="true" />
                      {{ idle: '待提问', streaming: '生成中', error: '出错', stopped: '已停止', done: '完成' }[paneStatus(i)]}
                    </span>
                    <span className="cmp-pane-agent" title={sel.agent ? '窗格级智能体（REQ-143）' : '继承对话智能体'}>
                      <AgentLogo agent={paneAgentOf(sel)} size={16} context="runtime" />
                      <Typography.Text strong style={{ fontSize: 12 }}>{paneAgentOf(sel)?.name ?? '—'}</Typography.Text>
                    </span>
                    {/* REQ-144 可选增强：采纳该窗格配置写回对话（仅 agent 直聊；运行中禁用） */}
                    {!isProjectScope && conversation.agent_id && !running && (
                      <Tooltip title="采纳：把该窗格的智能体/模型/知识库/方案/技能开关写回对话配置，之后单流对话沿用">
                        <Button
                          type="link" size="small" style={{ padding: '0 4px', fontSize: 11 }}
                          onClick={() => adoptPane(sel)}
                        >
                          采纳
                        </Button>
                      </Tooltip>
                    )}
                    <span className="cmp-pane-badges">
                      {paneBadgeSelect({
                        label: '模型', value: sel.model,
                        valueLabel: conns.find((c) => c.id === sel.model) ? connDisplayName(conns.find((c) => c.id === sel.model)!) : (sel.model || ''),
                        inheritText: paneInherit.model,
                        options: conns.map((c) => ({ value: c.id, label: connDisplayName(c) })),
                        onChange: (v) => setPaneSel(i, 'model', v),
                      })}
                      {paneBadgeSelect({
                        label: '库', value: sel.kb,
                        valueLabel: kbs.find((k) => k.id === sel.kb)?.name ?? sel.kb,
                        inheritText: paneInherit.kb,
                        options: kbs.map((k) => ({ value: k.id, label: k.name })),
                        onChange: (v) => setPaneSel(i, 'kb', v),
                      })}
                      {paneBadgeSelect({
                        label: '方案', value: sel.profile,
                        valueLabel: profiles.find((p) => p.id === sel.profile)?.name ?? sel.profile,
                        inheritText: paneInherit.profile,
                        options: profiles.filter((p) => p.status === 'running').map((p) => ({ value: p.id, label: p.name })),
                        onChange: (v) => setPaneSel(i, 'profile', v),
                      })}
                      {paneBadgeSelect({
                        label: '技能', value: sel.skills,
                        valueLabel: sel.skills === 'on' ? '开' : '关',
                        inheritText: paneInherit.skills,
                        options: [{ value: 'on', label: '开启' }, { value: 'off', label: '关闭' }],
                        onChange: (v) => setPaneSel(i, 'skills', v),
                      })}
                    </span>
                  </header>
                  <div className="cmp-pane-stream">
                    {(paneItems[i] ?? []).length > 0 ? (
                      <Bubble.List items={paneListItems(i)} role={bubbleRoles} />
                    ) : (
                      <div className="cmp-pane-empty">独立生成 · 等待提问</div>
                    )}
                  </div>
                  {/* REQ-19f 窗格独立配置区：未设置项继承对话当前配置（Q-6 不追溯，仅影响该窗格后续消息） */}
                  {/* REQ-150：模型/库/方案/技能 承载已迁窗格头徽标（继承/覆盖显性化）；此处留 Agent 选择与历史口径 */}
                  <footer className="cmp-pane-cfg">
                    <Select
                      size="small" allowClear disabled={running} showSearch
                      optionFilterProp="label"
                      placeholder="智能体 · 继承" value={sel.agent || undefined}
                      onChange={(v) => setPaneSel(i, 'agent', v ?? '')}
                      options={agentOptions}
                    />
                    <Checkbox
                      checked={sel.noHistory}
                      disabled={running}
                      onChange={(e) => setPaneSel(i, 'noHistory', e.target.checked)}
                      style={{ fontSize: 11 }}
                    >
                      不携带历史（干净对照，REQ-143）
                    </Checkbox>
                    {/* REQ-144：更多配置（推理参数/提示词改写/技能开关）+ 剖面与窗格间复制 */}
                    <Collapse
                      ghost
                      size="small"
                      className="cmp-more-cfg"
                      items={[{
                        key: 'more',
                        label: <span style={{ fontSize: 11, color: 'var(--ant-color-text-tertiary, #999)' }}>更多配置（温度 / 提示词改写 / 技能 · 剖面）</span>,
                        children: (
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                            <div style={{ display: 'flex', gap: 6 }}>
                              <InputNumber
                                size="small" style={{ flex: 1 }} min={0} max={2} step={0.1} disabled={running}
                                placeholder="温度 · 继承" value={sel.temperature ?? undefined}
                                onChange={(v) => setPaneSel(i, 'temperature', typeof v === 'number' ? v : null)}
                              />
                              <Typography.Text type="secondary" style={{ fontSize: 11, alignSelf: 'center' }}>
                                技能承载已迁窗格头徽标（REQ-150）
                              </Typography.Text>
                            </div>
                            <Input.TextArea
                              size="small" rows={2} disabled={running} maxLength={2000}
                              placeholder="系统提示词临时改写 · 继承（仅本窗格生效）"
                              value={sel.instruction || undefined}
                              onChange={(e) => setPaneSel(i, 'instruction', e.target.value)}
                            />
                            <Space size={4} wrap>
                              <Select
                                size="small" style={{ minWidth: 130 }} allowClear disabled={running}
                                placeholder="应用剖面…" value={undefined}
                                onChange={(name) => { const pr = cmpProfiles.find((x) => x.name === name); if (pr) applyProfile(i, pr.cfg) }}
                                options={cmpProfiles.map((x) => ({ value: x.name, label: x.name }))}
                              />
                              <Button size="small" disabled={running} onClick={() => saveProfile(i)}>存为剖面</Button>
                              {i > 0 && (
                                <Button size="small" disabled={running} onClick={() => copyPrevPane(i)}>
                                  复制上一窗格
                                </Button>
                              )}
                            </Space>
                          </div>
                        ),
                      }]}
                    />
                  </footer>
                </section>
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
            <Bubble.List items={listItems} role={BUBBLE_ROLES} />
          </>
        )}
        {running && (
          <div className="chat-stop">
            <button type="button" className="chat-stop-btn" onClick={stop} title={cmp.on ? `停止整组（${cmp.panes.length} 窗格一并终止，已生成内容保留）` : '停止本次生成'}>
              <span className="chat-stop-glyph" aria-hidden="true" />
              <span>停止生成</span>
            </button>
          </div>
        )}
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

      <div className="composer">
        <div className="composer-inner">
          {/* REQ-336：附件回显条（发送前可移除；文件已上传入智能体工作目录） */}
          {attachFiles.length > 0 && (
            <div className="attach-tags" data-testid="attach-tags">
              {attachFiles.map((f, i) => (
                <Tag
                  key={`${f.name}-${i}`}
                  closable
                  onClose={() => setAttachFiles((prev) => prev.filter((_, j) => j !== i))}
                  style={{ fontSize: 11 }}
                >
                  📎 {f.name}
                </Tag>
              ))}
            </div>
          )}
          {/* REQ-336：「/」触发快速配置面板（本体方案/知识库=会话级挂载即生效；技能=插入 / 引用） */}
          {slashOpen && (
            <div className="slash-picker" data-testid="slash-picker">
              <div className="chip-picker-title">快速配置（选择即对会话生效；Esc 关闭）</div>
              <div className="slash-section">本体运行方案{ontoDisabled ? '（暂无 running 方案）' : ''}</div>
              {runningProfiles.slice(0, 6).map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className="chip-picker-item"
                  onClick={() => {
                    patchConv({ runtime_profile_id: p.id, ontology_enabled: true })
                    setInput((v) => v.replace(/\/$/, ''))
                    setSlashOpen(false)
                  }}
                >
                  <span className="chip-picker-name">🗂 {p.name}</span>
                  <span className="chip-picker-meta">{p.engine}</span>
                </button>
              ))}
              <div className="slash-section">知识库{kbDisabled ? '（暂无知识库）' : ''}</div>
              {kbs.slice(0, 6).map((k) => (
                <button
                  key={k.id}
                  type="button"
                  className="chip-picker-item"
                  onClick={() => {
                    patchConv({ kb_id: k.id, enable_kb: true })
                    setInput((v) => v.replace(/\/$/, ''))
                    setSlashOpen(false)
                  }}
                >
                  <span className="chip-picker-name">📚 {k.name}</span>
                  {typeof k.doc_count === 'number' ? <span className="chip-picker-meta">{k.doc_count} 文档</span> : null}
                </button>
              ))}
              <div className="slash-section">已挂载技能{skillsDisabled ? '（该智能体未挂载技能）' : '（选择即引用，挂载在智能体配置）'}</div>
              {(agentSkills ?? []).slice(0, 6).map((s) => (
                <button
                  key={s}
                  type="button"
                  className="chip-picker-item"
                  onClick={() => {
                    setInput((v) => v.replace(/\/$/, `/${s} `))
                    setSlashOpen(false)
                  }}
                >
                  <span className="chip-picker-name">⚡ {s}</span>
                  <span className="chip-picker-meta">插入引用</span>
                </button>
              ))}
            </div>
          )}
          {/* REQ-322：过程显示分级已迁头部 ops 行（对话历史上方、观测旁），输入区回归「输入卡+会话配置行」两件 */}
          {/* 会话配置 chips 渲染在输入卡内底部（X Sender footer）：知识库 / 本体 / 技能 开关；模型归智能体配置 */}
          <Sender
            value={input}
            onChange={(v) => {
              setInput(v)
              // REQ-336：输入「/」触发快速配置（空输入或空白后 freshly typed 的斜杠）
              if (/(^|\s)\/$/.test(v)) setSlashOpen(true)
              else if (!v.endsWith('/') && slashOpen) setSlashOpen(false)
            }}
            prefix={
              <Tooltip title={convAgent?.id ? '附加本地文件（存入工作目录）' : '会话无智能体，无法附加文件'}>
                <Button
                  type="text"
                  size="small"
                  icon={<PaperClipOutlined />}
                  aria-label="附加文件"
                  data-testid="attach-btn"
                  disabled={!convAgent?.id || running}
                  loading={attaching}
                  onClick={() => document.getElementById('attach-file-input')?.click()}
                />
              </Tooltip>
            }
            onSubmit={() => send()}
            onCancel={stop}
            loading={running}
            placeholder={placeholder}
            disabled={!canSend}
            footer={
              <div className="chat-chips">
                {cmp.on ? (
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    对比模式 · {cmp.panes.length} 窗格独立配置（知识库/本体/技能承载于各窗格头，本区不参与配置）
                  </Typography.Text>
                ) : (<>
                {kbDisabled ? (
                  <Tooltip title={kbHint}><span className="chip-slot">{kbChip}</span></Tooltip>
                ) : (
                  <Popover
                    open={picker === 'kb'}
                    onOpenChange={(o) => { if (!o) setPicker(null) }}
                    trigger="click"
                    placement="topLeft"
                    arrow={false}
                    content={pickerList(
                      '选择知识库',
                      kbs.map((k) => ({ id: k.id, name: k.name, meta: typeof k.doc_count === 'number' ? `${k.doc_count} 文档` : undefined })),
                      conversation.kb_id,
                      (id) => patchConv({ kb_id: id, enable_kb: true }),
                    )}
                  >
                    <span className="chip-slot">{kbChip}</span>
                  </Popover>
                )}
                {ontoDisabled ? (
                  <Tooltip title={ontoHint}><span className="chip-slot">{ontoChip}</span></Tooltip>
                ) : (
                  <Popover
                    open={picker === 'onto'}
                    onOpenChange={(o) => { if (!o) setPicker(null) }}
                    trigger="click"
                    placement="topLeft"
                    arrow={false}
                    content={pickerList(
                      '选择本体运行方案（挂载方案即挂载其本体集合；多本体请先在「本体运行」栏组建方案）',
                      runningProfiles.map((p) => ({ id: p.id, name: p.name, meta: p.engine })),
                      conversation.runtime_profile_id,
                      (id) => patchConv({ runtime_profile_id: id, ontology_enabled: true }),
                      // REQ-215：已开启时提供关闭通路（本体 chip 点击=管理入口）
                      conversation.ontology_enabled ? {
                        icon: <StopOutlined style={{ color: 'var(--c-ink-2)', marginRight: 6 }} />,
                        label: '关闭本体增强（保留方案绑定）',
                        onClick: () => patchConv({ ontology_enabled: false }),
                      } : undefined,
                    )}
                  >
                    <span className="chip-slot">{ontoChip}</span>
                  </Popover>
                )}
                {skillsDisabled ? <Tooltip title={skillsHint}><span className="chip-slot">{skillsChip}</span></Tooltip> : skillsChip}
                {/* REQ-311 B3 检索参数迁头部⋮会话设置；REQ-336：模型 chip 退役（模型归智能体）、伴生 chip 迁⋮ */}
                <span className="chat-chips-hint">输入 / 快速配置 · Enter 发送 · Shift+Enter 换行</span>
                </>)}
              </div>
            }
          />
          {/* REQ-336：附加文件选择（隐藏 input；回形针触发；上传入智能体工作目录） */}
          <input
            id="attach-file-input"
            type="file"
            multiple
            style={{ display: 'none' }}
            onChange={(e) => {
              void uploadAttachments(e.target.files)
              e.target.value = ''
            }}
          />
        </div>
      </div>

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
    </div>
  )
}

// ---------------------------------------------------------------------------
// REQ-311 B4/M115：todo 产物卡——todo_write 机器侧清单渲染（conversation.todo_json，
// [{title,status}] 全量覆写语义；run 收尾 onConversationUpdated 刷新后可见）。
// 数据零新增（REQ-202 已落库），本卡只做进度外显：三态勾选渲染+完成计数。
// ---------------------------------------------------------------------------
type TodoItem = { title: string; status: string }

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
