// REQ-400①③/M196 收口：单源载荷构造器——后端 updateAgent/updateProject 为 full-replace 语义
// （decodeJSON 后整行 UPDATE，请求缺字段即清零），载荷必须完整。本模块以「服务端返回的实体
// 对象」为唯一基座（新增后端字段自动透传，根治手写字段清单漂移；实证两处现网清零=
// 手写 agentFullPayload 漏 companion_extract_cfg〔伴生抽取高级设置每次保存被清〕+
// 伴生绑定路径手写清单漏 connectors/tool_approval/approval_exempt/approval_timeout_hours/
// onchat_mode/onto_ops/hooks 七字段），表单值仅覆盖其管理字段。
// 字段级语义沿既有构造器保留：undefined 不覆盖（REQ-264 已挂载字段清空事故口径）、null 除
// 显式允许（model_conn_id）外回落实体（沿 v.x ?? agent.x 的 ?? 语义）。
import type { Agent, Project } from './types'

/** 实体基座：剔除服务端自有字段（id 由 URL 承载；created_at/updated_at 服务端管；is_builtin 不可改）。 */
function entityBase(e: Record<string, any>): Record<string, any> {
  const { id: _i, created_at: _c, updated_at: _u, is_builtin: _b, ...base } = e
  return base
}

/** 表单覆盖：undefined 跳过；null 除 allowNull 外跳过（?? 语义回落实体）。 */
function applyForm(base: Record<string, any>, v: Record<string, any>, allowNull: string[] = []): Record<string, any> {
  const out = { ...base }
  for (const k of Object.keys(v)) {
    const val = v[k]
    if (val === undefined) continue
    if (val === null && !allowNull.includes(k)) continue
    out[k] = val
  }
  return out
}

/** Agent 全量保存载荷（agent_as_tool/配置页/伴生绑定共用——③四份手写清单收敛于此）。
 *  mcp_serve 的 token 原样保留（重置走专用端点）；伴生字段自「伴生本体」视图维护，
 *  实体基座自动透传（不再逐字段手抄）。 */
export function agentFullPayload(agent: Agent, v: Record<string, any>): Record<string, any> {
  const out = applyForm(entityBase(agent as any), v, ['model_conn_id'])
  // 表单扁平命名收拢为实体形态（仅当表单提供时覆盖，沿既有字段级语义）
  out.mcp_serve = {
    enabled: v.mcp_serve_enabled !== undefined ? !!v.mcp_serve_enabled : !!agent.mcp_serve?.enabled,
    tool_name: (v.mcp_serve_tool_name !== undefined ? v.mcp_serve_tool_name : agent.mcp_serve?.tool_name ?? '').trim(),
    token: agent.mcp_serve?.token ?? '',
  }
  out.hooks = {
    fetch_guard_disabled: v.hooks_fetch_guard_disabled ?? agent.hooks?.fetch_guard_disabled ?? false, // REQ-232/M59
    deny_rules: (v.hooks_deny_rules ?? agent.hooks?.deny_rules ?? []).map((r: any) => ({
      name: (r.name ?? '').trim() || '规则',
      tool_pattern: (r.tool_pattern ?? '').trim(),
      args_pattern: (r.args_pattern ?? '').trim(),
      reason: (r.reason ?? '').trim(),
    })).filter((r: any) => r.tool_pattern),
  }
  // 表单专用键不出 wire（已收拢进 mcp_serve/hooks 实体形态）
  delete out.mcp_serve_enabled
  delete out.mcp_serve_tool_name
  delete out.hooks_fetch_guard_disabled
  delete out.hooks_deny_rules
  // 既有 trim/布尔收拢语义保留
  if (typeof out.work_dir === 'string') out.work_dir = out.work_dir.trim() // REQ-202/M38
  if (typeof out.verify_command === 'string') out.verify_command = out.verify_command.trim() // REQ-202/M38
  if (typeof out.logo_url === 'string') out.logo_url = out.logo_url.trim() // REQ-137
  if (out.onto_ops !== undefined) out.onto_ops = !!out.onto_ops // REQ-366
  return out
}

/** Project 全量保存载荷（配置视图与协作视图共用）。agent_ids/coordinator 不出 wire——
 *  成员面走 setProjectAgents 专用端点，后端 UPDATE 也不写这两列。 */
export function projectFullPayload(project: Project, v: Record<string, any>): Record<string, any> {
  const out = applyForm(entityBase(project as any), v)
  delete out.agent_ids
  delete out.coordinator
  return out
}
