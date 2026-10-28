// REQ-466/M266：离开防护（单槽登记）——「Spec 编辑」有未保存修改时拦截栏位切换/返回列表/
// 上一条下一条切换，确认后才放行（P5 试错安全）。单槽足够：当前唯一产生方=Spec 编辑脏标记；
// 将来多产生方（如图形编辑草稿）再扩多槽。
import { Modal } from 'antd'

let guard: { id: string; why: string } | null = null

export function setLeaveGuard(id: string, why: string) {
  guard = { id, why }
}

export function clearLeaveGuard(id: string) {
  if (guard?.id === id) guard = null
}

/** 无守卫直行；有守卫弹「放弃修改并离开」确认（确认后清守卫放行，取消留在原地）。 */
export function guardLeave(next: () => void) {
  if (!guard) {
    next()
    return
  }
  const g = guard
  Modal.confirm({
    title: '放弃未保存的修改？',
    content: g.why,
    okText: '放弃修改并离开',
    okButtonProps: { danger: true },
    cancelText: '继续编辑',
    onOk: () => {
      if (guard?.id === g.id) guard = null
      next()
    },
  })
}
