// REQ-401/M197：事件流归约（自 ChatWindow.tsx 拆出——单路消息区与对比窗格共用的
// 事件→条目翻译状态机；行为零变更）。
import type { Message } from '../../api/types'
import { describeEvent } from '../../lib/eventRegistry'

export interface ChatItem {
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
  seg?: boolean // REQ-477①：回放 message.segment 以段代显的助手正文段（块体内淡一档；末段可被提升为最终回答）
  reasonStart?: number // REQ-477②：本步思考起始时刻（live；sealReasoning 封口时折算 reasonMs）
  reasonMs?: number // REQ-477②：本步思考耗时（「思考 · 持续 Xs」行显示；回放卡无此数据只显字数）
}

/** 会话挂起中断卡数据（run.interrupted payload → 组件状态；实时与对比窗格共用） */
export function toInterruptState(payload: any) {
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
export function applyRunEvent(prev: ChatItem[], event: string, payload: any, runKey: string): ChatItem[] {
  switch (event) {
    case 'run.started':
    case 'run.finished': {
      const desc = describeEvent(event, payload)
      const next = sealReasoning([...prev])
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
      const next = sealReasoning([...prev])
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
      next.push({ kind: 'event', evType: 'reasoning', reasoning: payload.delta ?? '', streamKey: runKey, evKey: `${runKey}-${step}`, reasonStart: Date.now() })
      return next
    }
    case 'message.delta': {
      const next = sealReasoning([...prev])
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
      const next = sealReasoning([...prev])
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
        const next = sealReasoning([...prev])
        closeTailSegment(next)
        next.push({ kind: 'event', evType: event, eventText: desc.text, eventErr: desc.err, eventWarn: desc.warn, evData: payload })
        return next
      }
      return prev
  }
}

/** REQ-362：关闭尾部开放正文段——有内容置为完结段（时间交错分段点），空段弹出（防幽灵打字气泡） */
export function closeTailSegment(list: ChatItem[]): void {
  const ti = list.length - 1
  const last = list[ti]
  if (!last || last.kind !== 'msg' || !last.streaming) return
  if (!last.content) list.pop()
  else list[ti] = { ...last, streaming: false }
}

/** REQ-477②：思考卡封口——尾部思考卡在后续非思考事件到达时折算耗时（reasonMs），
 *  供「思考 · 持续 Xs」紧凑行显示；流式键保留（run 收尾统一置空收起，语义不变） */
export function sealReasoning(list: ChatItem[]): ChatItem[] {
  const ti = list.length - 1
  const last = list[ti]
  if (!last || last.kind !== 'event' || last.evType !== 'reasoning' || !last.streamKey) return list
  if (last.reasonStart == null || last.reasonMs != null) return list
  list[ti] = { ...last, reasonMs: Date.now() - last.reasonStart }
  return list
}
