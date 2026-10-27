/**
 * REQ-133 桌面端原生目录选择（补丁式接入：main.tsx 引导时执行 installNativePickDir 一次）。
 *
 * 缘由（bugfix）：桌面客户端内 backend 由 Tauri 壳以 CREATE_NO_WINDOW 拉起，是无控制台、
 * 无前台会话的无头服务进程；其 POST /api/projects/pick-dir（Go 侧 powershell 起
 * FolderBrowserDialog）在该进程链下弹窗不可靠——对话框创建失败被归一为 400
 * 「目录选择已取消或不可用（远程部署请手填路径）」，或弹出到 WebView 窗口之后不可见。
 * 壳进程持有真实窗口，改走 tauri-plugin-dialog 原生对话框：owner/焦点/主线程调度
 * 由插件正确处理（Windows/macOS 同口径）。
 *
 * 实现：运行时改写 api.pickProjectDir（不动 client.ts，所有调用点自动生效）；浏览器 /
 * web 部署无 __TAURI__ 全局 → 原样走后端端点，行为不变。用户取消返回
 * { dir: '', cancelled: true }，由调用方静默处理（不清空已填路径、不弹错误 toast）。
 */
import { api } from './client'

export interface PickDirResult {
  dir: string
  cancelled?: boolean
}

export function installNativePickDir(): void {
  const t = (globalThis as any).__TAURI__
  // 浏览器 / 非 Tauri 环境（或插件 JS API 未注入）：保持后端端点原行为
  if (!t?.dialog?.open) return
  api.pickProjectDir = async (): Promise<PickDirResult> => {
    try {
      const sel: unknown = await t.dialog.open({
        directory: true,
        multiple: false,
        title: '选择项目本地目录',
      })
      if (typeof sel !== 'string' || !sel) return { dir: '', cancelled: true }
      return { dir: sel }
    } catch (e: any) {
      // 原生对话框唤起失败：保留可读错误（不再误报「远程部署」文案）
      throw new Error(e?.message ?? '无法唤起系统目录选择对话框')
    }
  }
}
