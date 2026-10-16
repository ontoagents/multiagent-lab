/**
 * 模型管理分组模型与工具函数（自 SettingsPage 抽取，REQ-172 二级配置界面组件化）。
 */
import type { ModelConnection } from '../../api/types'
import { PROVIDER_PRESETS } from '../../api/providerPresets'

/**
 * 提供商分组（REQ-148 起分组标识与 BaseURL 解耦）：
 * 后端 model_connection 带 provider_group_id（provider_group 表，含展示别名），同一供应商可多实例
 * （同 BaseURL 不同账号/Key）。老数据由后端按 (protocol, base_url) 幂等回填，存量行为不变。
 * - 连接命名约定：`{提供商名}·{模型名}`（间隔符 U+00B7），保证 name 的 UNIQUE 约束不冲突；
 * - 提供商展示名 = 组别名（仅展示层）> 连接名中第一个 · 之前的部分（锚点派生）；
 * - 提供商改名（真名变更）= 按新前缀批量重生成组内全部连接名；别名 = 仅展示层，两者语义分离；
 * - 合并列表：提供商为可展开行（聚合行），其模型直接嵌套在展开区（明细行）。
 */
export interface ProviderGroup {
  key: string // `pg:${provider_group_id}`（空 id 回退 `${protocol}::${base_url}`，兼容未回填数据）
  id: string // provider_group_id（空 = 回退分组）
  alias: string // 组别名（展示层）
  name: string // 提供商展示名（别名优先，缺省从锚点派生）
  baseUrl: string
  protocol: string
  anchor: ModelConnection
  members: ModelConnection[]
}

export function groupOf(c: ModelConnection): string {
  return c.provider_group_id ? `pg:${c.provider_group_id}` : `${c.protocol}::${c.base_url}`
}

export const NAME_SEP = '·'

/** 提供商名 = 连接名中第一个 · 之前的部分；老数据（无 ·）取整名 */
export function providerOfName(name: string): string {
  const i = name.indexOf(NAME_SEP)
  return i > 0 ? name.slice(0, i) : name
}

/** 生成唯一连接名 `{provider}·{model}`；与 taken 冲突时追加 ` (n)`（n 从 2 起） */
export function uniqueConnName(provider: string, model: string, taken: Set<string>): string {
  const base = `${provider}${NAME_SEP}${model}`
  let name = base
  for (let n = 2; taken.has(name); n += 1) {
    name = `${base} (${n})`
  }
  return name
}

/** 协议展示名（REQ-172：openai_compat / anthropic 双协议） */
export function protocolLabel(p: string): string {
  return p === 'anthropic' ? 'anthropic（Messages API）' : 'openai_compat（OpenAI 兼容）'
}

/** 协议是否为 anthropic（REQ-172：决定表单联动——BaseURL 口径/类型限制/模型名占位） */
export function isAnthropicProtocol(p: string | undefined | null): boolean {
  return p === 'anthropic'
}

/**
 * 供应商归组（界面优化批 2026-10-16——开发者指令「供应商列表不要把供应商和协议做正交，
 * 只显示内置的供应商和自定义供应商」）：同一厂商的多协议/多地址连接归入**同一条供应商**，
 * 协议与接入地址作为组内配对展示（SettingsPage「协议 / 接入地址」列），不再按 (protocol, base_url)
 * 拆成多条供应商行。
 * - 厂商键（vendorKey）：连接名「·」前缀的首词小写（DeepSeek 深度求索·x → deepseek）；能匹配到
 *   厂商预设（PROVIDER_PRESETS 名/键）时用预设 key（内置供应商）；自定义供应商=前缀词本身。
 * - 内置/自定义：能匹配预设=内置（Tag 内置），否则=自定义。
 * - 老的 ProviderGroup（provider_group_id 分组，REQ-148）仍用于 Key 共享（copy_key_from 锚点）
 *   与别名展示——归组键变化仅影响列表聚合粒度，不改数据。
 */

export function vendorPrefixOf(name: string): string {
  const base = providerOfName(name).trim()
  const i = base.indexOf(' ')
  return i > 0 ? base.slice(0, i) : base
}

export function vendorKeyOf(name: string): string {
  const prefix = vendorPrefixOf(name)
  const lower = prefix.toLowerCase()
  const hit = PROVIDER_PRESETS.find(
    (pr) => pr.key.toLowerCase() === lower || pr.name.toLowerCase().includes(lower) && lower.length >= 2,
  )
  return hit ? `preset:${hit.key}` : `custom:${lower}`
}

export function isBuiltinVendor(name: string): boolean {
  return vendorKeyOf(name).startsWith('preset:')
}

/** 供应商展示名：组内首个成员的「·」前缀（别名优先逻辑在调用侧处理） */
export function vendorDisplayOf(name: string): string {
  return providerOfName(name)
}
