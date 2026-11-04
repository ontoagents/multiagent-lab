/**
 * ⭐ 最近打开记录（REQ-616-aa64/M424-aa64，121 号 R3 A7——120 号评审 misalignment 件，
 * 宿主=项目/工作区双模块树顶虚拟分组）：重度项目用户「最近打开的项目/工作区」3~5 个直达，
 * 选 target ≤2 跳（导航→模块→⭐行）。
 * localStorage 键沿 eino.* 先例：eino.recent.projects / eino.recent.workspaces（各自独立清单，
 * 互不共享——两模块分组语义分立）；容量 5（MRU 去重置顶）；push 后派发 `eino:recent` 事件，
 * Sidebar 虚拟分组监听刷新（同页常驻挂载场景无 props 变化也能更新）。
 */

export type RecentKind = 'project' | 'workspace'

const KEYS: Record<RecentKind, string> = {
  project: 'eino.recent.projects',
  workspace: 'eino.recent.workspaces',
}
const CAP = 5
export const RECENT_EVENT = 'eino:recent'

function read(kind: RecentKind): string[] {
  try {
    const raw = localStorage.getItem(KEYS[kind])
    const arr = raw ? (JSON.parse(raw) as unknown) : []
    return Array.isArray(arr) ? arr.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return [] // 配额满/损坏静默（沿 REQ-509 口径）
  }
}

function write(kind: RecentKind, ids: string[]) {
  try {
    localStorage.setItem(KEYS[kind], JSON.stringify(ids))
  } catch { /* 配额满静默 */ }
}

/** 记一次打开（MRU 去重置顶，容量 5）；派发 eino:recent 供 Sidebar 虚拟分组刷新 */
export function pushRecent(kind: RecentKind, id: string) {
  if (!id) return
  const next = [id, ...read(kind).filter((x) => x !== id)].slice(0, CAP)
  write(kind, next)
  try {
    window.dispatchEvent(new CustomEvent(RECENT_EVENT))
  } catch { /* 静默 */ }
}

/** 读最近清单（旧 id 不清洗——调用方按现存清单过滤后展示） */
export function getRecent(kind: RecentKind): string[] {
  return read(kind)
}
