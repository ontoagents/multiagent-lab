/** REQ-240 前端优化⑥：全局明暗主题（亮为默认；localStorage eino.theme 记忆）。
 *  AntD 侧经 ConfigProvider darkAlgorithm 切换（main.tsx ThemedApp 监听），
 *  自研 token 侧经 html[data-theme] 覆盖（styles.css dark 块）。 */

export type ThemeName = 'light' | 'dark'
const KEY = 'eino.theme'
const EVT = 'eino-theme-change'

export function readTheme(): ThemeName {
  return localStorage.getItem(KEY) === 'dark' ? 'dark' : 'light'
}

export function applyTheme(t: ThemeName) {
  localStorage.setItem(KEY, t)
  document.documentElement.dataset.theme = t
  window.dispatchEvent(new CustomEvent(EVT))
}

export function toggleTheme() {
  applyTheme(readTheme() === 'dark' ? 'light' : 'dark')
}

/** 初始化（main 挂载前调用）：无记忆时跟随系统偏好。 */
export function initTheme(): ThemeName {
  const saved = localStorage.getItem(KEY) as ThemeName | null
  const t: ThemeName = saved ?? (window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  document.documentElement.dataset.theme = t
  return t
}

export const THEME_EVENT = EVT
