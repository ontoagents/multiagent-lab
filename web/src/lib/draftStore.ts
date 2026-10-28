// REQ-402/M198：构建流程草稿持久化（localStorage 起步——治「草稿暂存」承诺与实际不符：
// BuildPage 条件渲染切路径即卸载、刷新即丢，仅 OntoChat 服务端持久）。
// 约定：key 命名空间 eino.onto.draft.*；单键 ≤256KB（spec 草案量级，超限跳过持久化并
// 广播事件——REQ-466/M266：console.warn 用户不可见，升级为 onto-draft-discard 事件，
// OntologyModule 监听转 toast）；读取解析失败静默清键（防坏态卡流程）。
// saveDraft 返回是否真正落库（false=超限/配额失败，调用方可感知）。

const PREFIX = 'eino.onto.draft.'
const MAX_BYTES = 256 * 1024

export function loadDraft<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(PREFIX + key)
    if (!raw) return null
    return JSON.parse(raw) as T
  } catch {
    try { localStorage.removeItem(PREFIX + key) } catch { /* 忽略 */ }
    return null
  }
}

export function saveDraft(key: string, value: unknown): boolean {
  try {
    const raw = JSON.stringify(value)
    if (raw.length > MAX_BYTES) {
      console.warn(`[draft] ${key} 超限（${raw.length}B > ${MAX_BYTES}B），跳过持久化`)
      notifyDiscard(key, `草稿「${key}」体积超限（${(raw.length / 1024).toFixed(0)}KB > 256KB 上限），本次内容仅保留在页面内存中——刷新或离开页面会丢失，请及时保存或缩小内容`)
      return false
    }
    localStorage.setItem(PREFIX + key, raw)
    return true
  } catch {
    // 配额满/隐私模式：降级为内存态（主流程不受阻），但不再静默——REQ-466 广播告警
    notifyDiscard(key, `草稿「${key}」写入本地存储失败（配额满或隐私模式），仅保留在页面内存中——刷新或离开页面会丢失`)
    return false
  }
}

export function clearDraft(key: string): void {
  try { localStorage.removeItem(PREFIX + key) } catch { /* 忽略 */ }
}

/** REQ-466：丢弃/写入失败广播（单槽 toast 由 OntologyModule 呈现；window 不存在时静默） */
function notifyDiscard(key: string, message: string) {
  try {
    if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('onto-draft-discard', { detail: { key, message } }))
  } catch { /* 忽略 */ }
}
