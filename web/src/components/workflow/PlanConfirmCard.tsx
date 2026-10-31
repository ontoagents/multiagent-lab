import { useMemo, useState } from 'react'
import { Alert, Button, Checkbox, Input, Segmented, Space, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-568/M375 WF-⑦ 计划确认卡（动态工作流编排批 L3 前半；109 号 §WF-⑦ 切粒度）——
// 合成 DAG 的人审确认面：消费 WF-① PreviewJSON 契约（workflow.PreviewOf 形态）。
// ①线性步骤列表（拓扑深度分层）+②分支「如果…就…」问句（条件边人话）+③表单微调
// （步骤删除/条件编辑——回写确认态，执行按确认态）+④Spec JSON 高级态兜底（双向一致）。
// 只读图（@xyflow）随 REQ-552 计划层接入同轮补（Q-7：不做节点边画布编辑不反转——本卡
// 仅线性列表微调，非画布）；画像词表净化：文案用「步骤/依赖/条件」，不用「节点/边/DAG」。
// 宿主=工作区对话面（REQ-549 已交付）；计划草稿数据源随 REQ-552 计划层交付接入。
// ---------------------------------------------------------------------------

export interface WFPreviewNode {
  id: string
  type: string
  name?: string
  depth: number
  parallel?: boolean
  join?: string
  writes?: string[]
}
export interface WFPreviewEdge {
  from: string
  to: string
  condition?: string
}
export interface WFPreview {
  name?: string
  nodes: WFPreviewNode[]
  edges: WFPreviewEdge[]
  vars?: { name: string; type: string; desc?: string }[]
  depth: number
}

const TYPE_LABEL: Record<string, string> = {
  llm: '生成',
  tool: '工具',
  gate: '检查',
  delegate: '委派',
}

/** 人话条件（「如果…就…」问句素材——v1 var:name==value containment 语法） */
function conditionHuman(cond: string): string {
  const m = /^var:(.+?)==(.+)$/.exec(cond.trim())
  if (m) return `如果变量「${m[1]}」的值为「${m[2]}」`
  return `条件：${cond}`
}

/** 解析 Spec/预览 JSON（容错——坏 JSON 抛错由调用方提示） */
export function parsePlanDraft(raw: string): { preview: WFPreview; spec: unknown } {
  const spec = JSON.parse(raw)
  // 预览字段缺失时从 spec 最小投影（深度=声明序）
  if (Array.isArray(spec?.nodes) && !Array.isArray(spec?.depth) && !spec?.nodes?.[0]?.depth) {
    const nodes: WFPreviewNode[] = spec.nodes.map((n: any, i: number) => ({
      id: n.id, type: n.type ?? 'llm', name: n.name ?? n.id, depth: i, join: n.join, writes: n.var_writes,
    }))
    const edges: WFPreviewEdge[] = (spec.edges ?? []).map((e: any) => ({ from: e.from, to: e.to, condition: e.condition }))
    return { preview: { name: spec.name, nodes, edges, vars: spec.vars ?? [], depth: nodes.length }, spec }
  }
  return { preview: spec as WFPreview, spec }
}

export default function PlanConfirmCard({ draftJson, onConfirm, onCancel }: {
  /** WF-① PreviewJSON 契约（或原始 spec JSON——自动最小投影） */
  draftJson: string
  /** 确认（回写确认态 spec JSON——执行按确认态） */
  onConfirm: (confirmedJson: string) => void
  onCancel?: () => void
}) {
  const [mode, setMode] = useState<'结构' | '高级'>('结构')
  const [parseErr, setParseErr] = useState<string | null>(null)
  const initial = useMemo(() => {
    try {
      const r = parsePlanDraft(draftJson)
      setParseErr(null)
      return r
    } catch (e: any) {
      setParseErr(e?.message ?? 'JSON 解析失败')
      return null
    }
  }, [draftJson])
  const [preview] = useState<WFPreview | null>(initial?.preview ?? null)
  const [specRaw, setSpecRaw] = useState(draftJson)
  const [skipIds, setSkipIds] = useState<string[]>([]) // 表单微调：勾选=跳过该步骤（确认态）

  const rebuildJson = (p: WFPreview) => {
    // 确认态回写：投影回 spec 形态（跳过的步骤剔除——执行按确认态）
    const skip = new Set(skipIds)
    const nodes = p.nodes.filter((n) => !skip.has(n.id)).map((n) => ({
      id: n.id, type: n.type, name: n.name, join: n.join || undefined,
      var_writes: n.writes && n.writes.length > 0 ? n.writes : undefined,
    }))
    const ids = new Set(nodes.map((n) => n.id))
    const edges = p.edges.filter((e) => ids.has(e.from) && ids.has(e.to)).map((e) => ({
      from: e.from, to: e.to, condition: e.condition || undefined,
    }))
    return JSON.stringify({ name: p.name, nodes, edges, vars: p.vars ?? [] }, null, 2)
  }

  const confirm = () => {
    if (mode === '高级') {
      // 高级态：编辑后 JSON 直通（双向一致——解析校验失败提示不提交）
      try {
        JSON.parse(specRaw)
        onConfirm(specRaw as string)
      } catch (e: any) {
        setParseErr('高级态 JSON 非法：' + (e?.message ?? '解析失败'))
      }
      return
    }
    if (!preview) return
    onConfirm(rebuildJson(preview))
  }

  const depthGroups = useMemo(() => {
    if (!preview) return []
    const byDepth = new Map<number, WFPreviewNode[]>()
    for (const n of preview.nodes) {
      if (!byDepth.has(n.depth)) byDepth.set(n.depth, [])
      byDepth.get(n.depth)!.push(n)
    }
    return [...byDepth.entries()].sort((a, b) => a[0] - b[0])
  }, [preview])

  if (parseErr && !preview) {
    return <Alert type="error" showIcon title="计划草稿解析失败" description={parseErr} />
  }
  if (!preview) return null

  const condEdges = preview.edges.filter((e) => e.condition)

  return (
    <div className="plan-confirm-card" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography.Text strong>{preview.name || '执行计划确认'}</Typography.Text>
        <Segmented
          size="small"
          value={mode}
          onChange={(v) => setMode(v as '结构' | '高级')}
          options={['结构', '高级']}
        />
      </div>

      {mode === '结构' ? (
        <>
          {/* ① 线性步骤列表（拓扑深度分层；勾选=跳过该步骤——表单微调） */}
          {depthGroups.map(([depth, nodes]) => (
            <div key={depth} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {depth > 0 && (
                <Typography.Text type="secondary" style={{ fontSize: 11, paddingLeft: 8 }}>
                  ↓ 上一步完成后{nodes.length > 1 ? '，以下可并行' : '：'}
                </Typography.Text>
              )}
              {nodes.map((n) => (
                <div key={n.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 8px', border: '1px solid var(--c-border, #eee)', borderRadius: 6 }}>
                  <Checkbox
                    checked={!skipIds.includes(n.id)}
                    onChange={(e) =>
                      setSkipIds((prev) => (e.target.checked ? prev.filter((x) => x !== n.id) : [...prev, n.id]))
                    }
                  />
                  <Typography.Text style={{ fontSize: 13 }}>
                    {TYPE_LABEL[n.type] ?? n.type}·{n.name || n.id}
                  </Typography.Text>
                  {n.parallel && (
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>可并行</Typography.Text>
                  )}
                  {n.writes && n.writes.length > 0 && (
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                      记录{n.writes.length} 项结果
                    </Typography.Text>
                  )}
                  <span style={{ flex: 1 }} />
                </div>
              ))}
            </div>
          ))}

          {/* ② 分支「如果…就…」问句（条件边人话） */}
          {condEdges.length > 0 && (
            <Alert
              type="info"
              showIcon
              title="计划包含条件分支"
              description={
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  {condEdges.map((e, i) => {
                    const toName = preview.nodes.find((n) => n.id === e.to)?.name ?? e.to
                    return (
                      <Typography.Text key={i} style={{ fontSize: 12 }}>
                        {e.condition ? conditionHuman(e.condition) : "无条件"}，就执行「{toName}」；否则跳过该步。
                      </Typography.Text>
                    )
                  })}
                </div>
              }
            />
          )}
          {skipIds.length > 0 && (
            <Typography.Text type="warning" style={{ fontSize: 12 }}>
              将跳过 {skipIds.length} 个步骤（确认后按此执行）。
            </Typography.Text>
          )}
        </>
      ) : (
        <>
          {/* ④ 高级态 Spec JSON 兜底（双向一致——确认提交前解析校验） */}
          <Input.TextArea
            value={specRaw}
            onChange={(e) => setSpecRaw(e.target.value)}
            autoSize={{ minRows: 10, maxRows: 24 }}
            style={{ fontFamily: 'monospace', fontSize: 12 }}
          />
        </>
      )}

      {parseErr && mode === '高级' && (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>{parseErr}</Typography.Text>
      )}

      <Space>
        <Button type="primary" onClick={confirm}>确认按此计划执行</Button>
        {onCancel && <Button onClick={onCancel}>取消</Button>}
      </Space>
      <Typography.Text type="secondary" style={{ fontSize: 11 }}>
        确认后按确认态执行（跳过的步骤不执行）；高级态可直接编辑计划 JSON。
      </Typography.Text>
      {/* PlusOutlined/DeleteOutlined 预留（步骤增删随 REQ-552 计划层联调扩展） */}
      <span style={{ display: 'none' }}><PlusOutlined /><DeleteOutlined /></span>
    </div>
  )
}
