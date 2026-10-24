// REQ-401/M197：事件注册表单源——新增事件类型只改本文件一处：
//   describeEvent   事件卡文案（实时流/历史回放/轨迹/重放四路共用）
//   levelGated      展示级别门控（REQ-149①简洁档隐藏调试族）
//   isKeyGranularity 粒度「关键」档白名单（REQ-217 显示档）
//   TRACE_GROUPS    轨迹面板过滤组（REQ-218，原 TraceDrawer 本地清单退役）
//   eventSource     事件来源分类（原型 06 §10 色彩语义）
// 自 ChatWindow.tsx 拆出（REQ-401 拆分轮；四份事件清单收敛于此，行为零变更）。

// 子 Agent 名（§6.5 subagent.enter/exit payload = 子 Agent 名；字段名做兼容取值）
// 子 Agent 名（§6.5 subagent.enter/exit payload = 子 Agent 名；字段名做兼容取值）
export function subagentName(d: any): string {
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
      return { text: `⚙ 调用工具 ${d?.tool_name ?? ''}${mcpSourceSuffix(d)}` }
    case 'tool.result':
      return { text: `⚙ 工具结果 ${d?.tool_name ?? ''}${mcpSourceSuffix(d)}` }
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
          // REQ-426/M222：CLI 外部成员（附后端名）
          if (d?.channel === 'cli') {
            const bk = d?.backend ? ` · ${String(d.backend)}` : ''
            return { text: `⌨️ 委派外部成员${who}（CLI${bk}）· ${String(d?.task ?? '').slice(0, 60)}` }
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
    // REQ-428/M224：外部成员过程转译（external.audit——CLI stream-json/A2A history 统一转译）
    case 'external.audit': {
      const summary = String(d?.summary ?? '').slice(0, 80)
      if (d?.member) {
        const ch = d?.channel === 'cli' ? 'CLI' : 'A2A'
        return { text: `🔍 外部成员过程 · ${String(d.member)}（${ch}）· ${String(d?.proto_event ?? '')} · ${summary}` }
      }
      return { text: `🧾 外部后端审计 · ${summary}` }
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
// REQ-281：数值紧凑展示（0.95→0.95；undefined→'—'），供伴生判定文案
export function fmtNum(v: any): string {
  return typeof v === 'number' ? String(Math.round(v * 100) / 100) : '—'
}

// REQ-311 A6：context.stats 注入源人话标签（账单卡与摘要文案共用）
// REQ-311 A6：context.stats 注入源人话标签（账单卡与摘要文案共用）
export function injectSourceLabel(src?: string): string {
  switch (src) {
    case 'kb': return '知识库召回'
    case 'companion': return '伴生本体召回'
    case 'ontology_guide': return '本体方案指引'
    case 'project_dir': return '项目本地目录'
    default: return src || '其他来源'
  }
}

// run.finished 摘要：耗时 / token 用量 / finish_reason
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

export type EventSource = 'builtin' | 'skill' | 'mcp' | 'onto' | 'subagent' | 'retrieval'

/**
 * 事件来源分类（原型 06 §10 色彩语义：内置 灰 / 技能 绿 / MCP 紫 / 本体 青 / 异常 红；
 * 子智能体取紫族、知识召回取绿族）。
 * §6.5：tool.call/result 已带 source（builtin | skill:{id} | mcp:{server} | ontology:facade | agent:{id}），按前缀采信；
 * 缺失时回退到 tool_name 命名约定：onto_* → 本体；mcp_ 前缀或 server__tool 双下划线 → MCP。
 */
export function eventSource(evType: string | undefined, evData: any): EventSource {
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

/** REQ-418②/M214：MCP 工具调用连接器来源标注——source=mcp:{server} 或 {server}__{tool} 命名 */
export function mcpSourceSuffix(d: any): string {
  const src = typeof d?.source === 'string' ? d.source : ''
  if (src.startsWith('mcp:')) return ` · MCP:${src.slice(4)}`
  const name = String(d?.tool_name ?? '')
  const i = name.indexOf('__')
  if (i > 0 && !name.startsWith('mcp_')) return ` · MCP:${name.slice(0, i)}`
  return ''
}

/** REQ-149① 展示级别门控：简洁档（level 0）隐藏调试细节事件（model.step 等），详细/调试档展开 */
const LEVEL_DEBUG_TYPES = new Set(['model.step', 'debug.cli'])
export function levelGated(type: string, level: number): boolean {
  return level < 1 && LEVEL_DEBUG_TYPES.has(type)
}

/** REQ-217 粒度「关键」档白名单（工具/运行/产物/审计族——原 displayItems 内联清单退役） */
const KEY_GRANULARITY_PREFIXES = ['tool.', 'run.']
const KEY_GRANULARITY_TYPES = new Set([
  'artifact.saved', 'approval.granted', 'approval.denied', 'hook.denied',
  'verify.completed', 'verify.failed', 'connector.degraded', 'context.stats',
])
export function isKeyGranularity(type: string): boolean {
  return KEY_GRANULARITY_PREFIXES.some((p) => type.startsWith(p)) || KEY_GRANULARITY_TYPES.has(type)
}

/** REQ-218 轨迹过滤组单源（原 TraceDrawer 本地 TYPE_GROUPS 清单退役——新增事件只在此登记） */
export const TRACE_GROUPS: { value: string; label: string; types: string[] }[] = [
  { value: 'tool', label: '工具调用', types: ['tool.call', 'tool.result'] },
  { value: 'approval', label: '审批挂起', types: ['run.interrupted'] },
  // REQ-224/M52（51 号 W1）：harness 事件域（结构化审计）
  { value: 'harness', label: 'Harness 审计', types: ['approval.granted', 'approval.denied', 'hook.denied', 'verify.completed', 'verify.failed', 'connector.degraded'] },
  { value: 'subagent', label: '子智能体', types: ['subagent.enter', 'subagent.exit', 'subagent.delegate'] },
  { value: 'workflow', label: '工作流节点', types: ['workflow.node'] }, // REQ-205/M40：节点开始/完成进度
  { value: 'retrieval', label: '知识召回', types: ['retrieval'] },
  // REQ-281：伴生沉淀过程事件域（抽取/候选/入图判定/拒绝镜像）
  { value: 'companion', label: '伴生沉淀', types: ['companion.extract', 'companion.candidates', 'companion.decision', 'companion.ingest', 'companion.reject'] },
  { value: 'ontohit', label: '本体消费', types: ['ontology.hit'] }, // REQ-224 H10：onto_* 工具命中
  { value: 'state', label: '状态机', types: ['state.changed'] }, // REQ-224①：会话状态变更（中断挂起/放弃/恢复）
  { value: 'artifact', label: '产物', types: ['artifact.saved'] },
  { value: 'cli', label: 'CLI 输出', types: ['debug.cli'] },
  { value: 'run', label: '运行始末', types: ['run.started', 'run.finished', 'run.error', 'context.stats'] },
]
