// ---------------------------------------------------------------------------
// 全站布局常量与助手（REQ-237，57 号《前端_页面布局体检与优化分析》P2 整改落地）
//   F4 左栏宽度三分天下 → 单一约定：同 key/默认/边界，七页共用；
//   F10 一屏四套调宽度交互 → Drawer 收敛为三档默认 + 原生 resizable 拖拽 + 宽度记忆。
// ---------------------------------------------------------------------------

/** 模块左栏宽度（Splitter 左面板）：七页共用同一 key/默认/边界——「全站左栏同宽」为产品意图 */
export const SIDEBAR_WIDTH = {
  // REQ-618-36a1/M426-36a1：计数徽标「N 项目 · M 对话」→图标数字芯片后栏宽再收 208→188；
  // storageKey 升 v2 让存量 208px+ 拖宽记忆失效、新默认即时生效（用户再拖仍会记忆）。
  storageKey: 'eino.sidebar.width.v2',
  default: 188, // REQ-240 前端优化③：280→240；界面瘦身 240→208；REQ-618 紧凑徽标配套 208→188（min 176 仍保搜索框可用；用户拖宽记忆优先）
  min: 176,
  max: 480,
} as const

export function sidebarDefaultSize(): number {
  return Number(localStorage.getItem(SIDEBAR_WIDTH.storageKey)) || SIDEBAR_WIDTH.default
}

export function sidebarRemember(sizes: number[]): void {
  localStorage.setItem(SIDEBAR_WIDTH.storageKey, String(Math.round(sizes[0])))
}

/** Drawer 三档默认宽（57 号 F10：原 560/640/680/820/860/880 六种取值收敛） */
export const DRAWER_SIZES = { small: 480, medium: 640, large: 820 } as const

/**
 * Drawer 档位 + 原生可拖拽 + 宽度记忆（antd 6 Drawer `size` 数字 + `resizable`）。
 * 用法：<Drawer {...drawerSizeProps('trace', DRAWER_SIZES.medium)} .../>
 */
export function drawerSizeProps(key: string, def: number, min = 400, max = 1080) {
  const storageKey = `eino.drawer.${key}`
  const saved = Number(localStorage.getItem(storageKey))
  const initial = saved >= min && saved <= max ? saved : def
  let latest = initial
  return {
    size: initial,
    resizable: true,
    onResize: (s: number) => {
      latest = s
    },
    onResizeEnd: () => localStorage.setItem(storageKey, String(latest)),
  }
}
