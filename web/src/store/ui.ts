import { create } from 'zustand'
import { toast } from '../lib/antd'

export type PageKey = 'agents' | 'projects' | 'workspaces' | 'ontology' | 'knowledge' | 'skills' | 'reference' | 'settings'

/** PageKey ↔ URL path 双向映射（刷新/前进后退保持当前页面） */
const PAGE_PATHS: Record<PageKey, string> = {
  agents: '/',
  projects: '/projects',
  // REQ-613-b6cc/M421-b6cc 工作区独立模块路由（121 号 R1①：/workspaces 新增）；R1② 起 /projects
  //   语义归位纯项目页（页签退役，REQ-614-9fb4/M422-9fb4）
  workspaces: '/workspaces',
  ontology: '/ontology',
  knowledge: '/knowledge',
  skills: '/skills',
  reference: '/reference',
  settings: '/settings',
}
function pageFromPath(pathname: string): PageKey {
  const p = pathname.replace(/\/+$/, '') || '/'
  // D-O15：原 Semantica 独立栏（/semantica）并入本体模块第五栏「消费与审计」，旧链接回落本体页
  if (p === '/semantica') return 'ontology'
  const hit = (Object.entries(PAGE_PATHS) as [PageKey, string][]).find(([, v]) => v === p)
  return hit ? hit[0] : 'agents' // 未知路径回落首页
}

interface UIState {
  page: PageKey
  setPage: (p: PageKey) => void

  /** 当前选中的对话 */
  currentConvId: string | null
  setCurrentConv: (id: string | null) => void

  /** 数据版本号：增删改后 +1 触发刷新 */
  dataVersion: number
  bumpData: () => void

  /** 全局提示（antd message 桥接，见 lib/antd.ts） */
  showToast: (msg: string, kind?: 'ok' | 'err') => void
}

// 初始 page 从 URL 恢复（刷新保持在当前页面）
const initialPage = pageFromPath(window.location.pathname)

// REQ-536/M390 会话现场恢复：currentConvId 进 sessionStorage（eino.conv.current）——
// 刷新后停留原会话（验收口径）；setCurrentConv(null) 即清除。
const CONV_KEY = 'eino.conv.current'
const initialConvId = (): string | null => {
  try {
    return sessionStorage.getItem(CONV_KEY)
  } catch {
    return null
  }
}

export const useUI = create<UIState>((set) => ({
  page: initialPage,
  setPage: (p) => {
    // 同步浏览器地址（pushState 不触发刷新）；同页幂等
    if (pageFromPath(window.location.pathname) !== p) {
      history.pushState({ page: p }, '', PAGE_PATHS[p])
    }
    set({ page: p })
  },
  currentConvId: initialConvId(),
  setCurrentConv: (id) => {
    try {
      if (id) sessionStorage.setItem(CONV_KEY, id)
      else sessionStorage.removeItem(CONV_KEY)
    } catch {}
    set({ currentConvId: id })
  },
  dataVersion: 0,
  bumpData: () => set((s) => ({ dataVersion: s.dataVersion + 1 })),
  showToast: (msg, kind = 'ok') => toast(msg, kind),
}))

// 浏览器前进/后退：popstate 时按 URL 恢复页面
window.addEventListener('popstate', () => {
  useUI.setState({ page: pageFromPath(window.location.pathname) })
})
