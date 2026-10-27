import type { InferenceBackendStatus } from '../api/types'

/** M13 §6.16：推理后端下拉选项——外部后端未发现（PATH 探测失败）时置灰不可选 */
export function inferenceBackendOptions(backends?: InferenceBackendStatus[]) {
  const list = backends ?? []
  // REQ-450：探测接口失败/为空时保底 eino-adk 默认项（创建表单不因探测异常无可选项）
  const withDefault = list.some((b) => b.default)
    ? list
    : ([{ name: 'eino-adk', default: true, available: true } as InferenceBackendStatus, ...list])
  return withDefault.map((b) => ({
    value: b.name,
    label: b.default
      ? 'eino-adk（自研默认）'
      : b.available
        ? `${b.name}（已发现${b.version ? ` · ${b.version}` : ''}）`
        : `${b.name}（未发现）`,
    disabled: !b.default && !b.available,
    // REQ-450：未发现项 hover 提示安装指引入口（下方「安装指引」Popover）
    ...(!b.default && !b.available
      ? { title: `${b.name} 未在 PATH 中发现（未安装或未加入 PATH）——可点下方「安装指引」` }
      : {}),
  }))
}

/** REQ-450：未发现的非默认后端清单（安装指引 Popover 数据源） */
export function uninstalledBackends(backends?: InferenceBackendStatus[]) {
  return (backends ?? []).filter((b) => !b.default && !b.available)
}
