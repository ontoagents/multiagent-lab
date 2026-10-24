// REQ-402/M198：构建流程草稿持久化（localStorage 起步——治「草稿暂存」承诺与实际不符：
// BuildPage 条件渲染切路径即卸载、刷新即丢，仅 OntoChat 服务端持久）。
// 约定：key 命名空间 eino.onto.draft.*；单键 ≤256KB（spec 草案量级，超限静默丢弃并告警——
// localStorage 配额保护）；读取解析失败静默清键（防坏态卡流程）。

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

export function saveDraft(key: string, value: unknown): void {
  try {
    const raw = JSON.stringify(value)
    if (raw.length > MAX_BYTES) {
      console.warn(`[draft] ${key} 超限（${raw.length}B > ${MAX_BYTES}B），跳过持久化`)
      return
    }
    localStorage.setItem(PREFIX + key, raw)
  } catch {
    // 配额满/隐私模式：静默降级（草稿仍是内存态，主流程不受阻）
  }
}

export function clearDraft(key: string): void {
  try { localStorage.removeItem(PREFIX + key) } catch { /* 忽略 */ }
}
