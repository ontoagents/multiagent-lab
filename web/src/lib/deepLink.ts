import type { Conversation } from '../api/types'
import { useUI } from '../store/ui'

/**
 * 会话深链分流基座（REQ-614-9fb4/M422-9fb4，121 号 R1② §5.2：eino.focus.conv 按 scope 分流）：
 * 「打开一个会话」的宿主模块由会话归属决定——
 *   scope=project            → 项目模块（/projects，选中归属项目+打开会话）；
 *   scope=agent 且带 workspace_id（直挂）→ 工作区模块（/workspaces，选中归属工作区+打开会话）；
 *   scope=agent 无 workspace_id          → 智能体页（/，REQ-584 回落链原语义）。
 * 写入协议沿 AgentsPage 先例（REQ-584）：focus.conv 为正键，消费方（AgentsPage/WorkspacePage/
 * ProjectsBody）在数据载入后命中才消费（未命中保留给其他模块回落）；跨模块导航时事件收不到、
 * 键由挂载消费承接；同模块常驻挂载场景派发 goto.workspace/goto.project 事件由响应式监听承接
 * （121 号 §5.2 gap-5 教训：IIFE 一次性消费在常驻挂载后会失灵）。
 * 本批交付=分流基座+两模块到达侧消费；跨模块命中写入侧（搜索命中按 scope 分流打开）随 R3 D1 接线
 * （121 号 §4.2-D1——hits 载荷需携带 scope 判据）。
 */
export function gotoConversation(conv: Pick<Conversation, 'id' | 'scope'> & Partial<Pick<Conversation, 'workspace_id' | 'project_id'>>) {
  const setPage = useUI.getState().setPage
  try {
    sessionStorage.setItem('eino.focus.conv', conv.id)
    if (conv.scope === 'project' && conv.project_id) {
      sessionStorage.setItem('eino.focus.project', conv.project_id)
    } else if (conv.scope === 'agent' && conv.workspace_id) {
      sessionStorage.setItem('eino.focus.workspace', conv.workspace_id)
    }
  } catch { /* 配额满静默（沿 REQ-509 口径） */ }
  if (conv.scope === 'project' && conv.project_id) {
    setPage('projects')
    window.dispatchEvent(new CustomEvent('eino:goto.project'))
  } else if (conv.scope === 'agent' && conv.workspace_id) {
    setPage('workspaces')
    window.dispatchEvent(new CustomEvent('eino:goto.workspace'))
  } else {
    setPage('agents')
  }
}
