// REQ-395/M191：挂起触达——审批/追问挂起主动提醒（Web Notification API 统一覆盖
// 浏览器与桌面客户端 WebView——Tauri 原生插件接入留观察，真机轮评估）。
// 触达判定：页面不可见（document.hidden）才通知——可见时挂起卡已在眼前（不骚扰）；
// 开关 localStorage eino.notify.pending（默认开）；权限懒请求（首次挂起时尝试）。

const NOTIFY_KEY = 'eino.notify.pending'

export function pendingNotifyEnabled(): boolean {
  try {
    return localStorage.getItem(NOTIFY_KEY) !== '0'
  } catch {
    return true
  }
}

export function setPendingNotifyEnabled(on: boolean) {
  try {
    localStorage.setItem(NOTIFY_KEY, on ? '1' : '0')
  } catch {
    /* 存储不可用忽略 */
  }
}

/** 挂起触达：页面不可见时发系统通知（kind=审批挂起|追问挂起）；返回是否实际发出。 */
export function notifyPending(kind: string, conversationTitle: string, detail: string): boolean {
  if (!pendingNotifyEnabled()) return false
  if (typeof document !== 'undefined' && !document.hidden) return false
  if (typeof Notification === 'undefined') return false
  if (Notification.permission === 'denied') return false
  const fire = () => {
    try {
      const n = new Notification(`【${kind}】${conversationTitle || '会话'}`, {
        body: detail || '等待你的处理——点击回到会话',
        tag: `eino-pending-${kind}`, // 同类挂起去重（不叠弹）
      })
      n.onclick = () => {
        try {
          window.focus()
        } catch {
          /* 聚焦失败静默（通知仍已触达） */
        }
      }
      return true
    } catch {
      return false
    }
  }
  if (Notification.permission === 'granted') return fire()
  // 权限默认态：懒请求后按结果发（用户拒绝则此后静默——denied 分支已拦）
  if (Notification.permission === 'default') {
    void Notification.requestPermission().then((p) => {
      if (p === 'granted') fire()
    })
  }
  return false
}
