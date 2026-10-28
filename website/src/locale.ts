// ---------------------------------------------------------------------------
// 站点语言常量（REQ-360 多语言）：中/英双语言。纯数据无 JSX——content.ts 等
// .ts 文件可安全 import；Provider 与 hook 见 i18n.tsx。
// 默认语言：首次访问跟随浏览器（zh* → 中文，其余 → English），手动切换后持久化。
// ---------------------------------------------------------------------------

export type Locale = 'zh' | 'en'

/** 双语文案字典通用形态：zh 为事实源，en 同形约束（en: typeof zh 由各调用处以 typeof 固化） */
export type L10n<T> = { zh: T; en: T }

const STORAGE_KEY = 'agentlab.locale'

export function detectLocale(): Locale {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'zh' || saved === 'en') return saved
  } catch {
    /* 隐私模式等 storage 不可用：回落浏览器语言探测 */
  }
  return typeof navigator !== 'undefined' && navigator.language?.toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

export function persistLocale(l: Locale): void {
  try {
    localStorage.setItem(STORAGE_KEY, l)
  } catch {
    /* 同上，降级为会话内切换 */
  }
}

/** <html lang> / <title> / meta description 随语言同步（SSR 无；本站纯 CSR） */
export const SITE_META: Record<Locale, { htmlLang: string; title: string; description: string }> = {
  zh: {
    htmlLang: 'zh-CN',
    title: 'AgentLab · 智能体构建平台',
    description:
      'AgentLab——面向专业个人的工作学习助手，以本体为知识底座：知识不是一堆文档，而是可查询、可审计、可出题的结构化资产；单机自部署（Go + CloudWeGo Eino/ADK），智能体工程 + 本体工程双核心，Apache-2.0 开源。',
  },
  en: {
    htmlLang: 'en',
    title: 'AgentLab · Agent Construction Platform',
    description:
      'AgentLab — a personal work & learning assistant for knowledge-intensive professionals, grounded on ontologies: knowledge as a structured asset you can query, audit and quiz — not a pile of documents. Single-machine self-hosted, built on Go + CloudWeGo Eino/ADK. Dual cores: agent engineering + ontology engineering. Apache-2.0.',
  },
}
