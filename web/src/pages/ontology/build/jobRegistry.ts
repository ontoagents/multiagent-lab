import { clearDraft, loadDraft, saveDraft } from '../../../lib/draftStore'

// ---------------------------------------------------------------------------
// REQ-598/M409（111 号 §3.4-③）：异步 job 轻量注册表——draftStore 单键数组
// {kind, path, label, startedAt}，流程启动 job 时登记、终态/入库/取消时清除；
// Hub 区块 A 读此表渲染「进行中」+「继续」深链。
// 诚实边界：注册表是导航聚合不是状态复制——job 真实进度以各路径页读自己的 job 键
// 轮询为准，Hub 不轮询；孤儿登记项由读取方惰性容错（非法 path 丢弃）。
// ---------------------------------------------------------------------------

export const ONTO_JOBS_DRAFT_KEY = 'jobs' // draftStore 前缀合成 eino.onto.draft.jobs

export type OntoJobEntry = {
  kind: string
  path: string
  label: string
  startedAt: number
}

/** 读取注册表（孤儿项惰性容错：非法 path 丢弃；解析失败清键防坏态）。 */
export function readJobs(validPaths: string[]): OntoJobEntry[] {
  const raw = loadDraft<OntoJobEntry[]>(ONTO_JOBS_DRAFT_KEY)
  if (!Array.isArray(raw)) return []
  return raw.filter(
    (j) => j && typeof j.path === 'string' && validPaths.includes(j.path) && typeof j.label === 'string',
  )
}

/** 登记/刷新一条（同 kind+path 幂等覆盖）。 */
export function registerJob(entry: Omit<OntoJobEntry, 'startedAt'> & { startedAt?: number }): void {
  const list = loadDraft<OntoJobEntry[]>(ONTO_JOBS_DRAFT_KEY)
  const base = Array.isArray(list) ? list.filter((j) => j && typeof j.path === 'string') : []
  const next = base.filter((j) => !(j.kind === entry.kind && j.path === entry.path))
  next.push({ kind: entry.kind, path: entry.path, label: entry.label, startedAt: entry.startedAt ?? Date.now() })
  saveDraft(ONTO_JOBS_DRAFT_KEY, next)
}

/** 清除（终态/入库/取消时调用；kind 缺省=清该 path 全部）。 */
export function clearJob(path: string, kind?: string): void {
  const list = loadDraft<OntoJobEntry[]>(ONTO_JOBS_DRAFT_KEY)
  if (!Array.isArray(list)) return
  const next = list.filter((j) => !(j.path === path && (kind === undefined || j.kind === kind)))
  if (next.length === 0) clearDraft(ONTO_JOBS_DRAFT_KEY)
  else saveDraft(ONTO_JOBS_DRAFT_KEY, next)
}
