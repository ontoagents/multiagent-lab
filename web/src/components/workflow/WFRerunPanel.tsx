import { useMemo, useState } from 'react'
import { Alert, Button, Popconfirm, Tag, Typography } from 'antd'
import { RedoOutlined, SaveOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-569/M376 WF-⑧ 节点级重跑 UI+编排图可视化（动态工作流编排批 L3 后半；109 号 §WF-⑧）——
// 消费 WF-④ RunSnapshot 节点状态与 rerun 语义 + WF-② 一等节点事件（wf.node.*）的呈现面：
// ①编排图实时呈现节点状态（事件驱动——node_id/status 四态色标）；②失败节点一键重跑
// （消费 WF-④ RerunFrom——下游作废语义一致）；③产物一键存笔记本（REQ-126 去向——Q-24 后
// 由工作区绑定承载）。画像词表净化沿 WF-⑦（「步骤」不用「节点」）。
// 装配层：Runner 事件回调驱动节点状态 map，快照持久化由 WF-④ store 表承载（chat 接线随 REQ-552）。
// ---------------------------------------------------------------------------

export interface WFNodeRunState {
  id: string
  type: string
  name?: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped'
  attempt?: number
  output?: string
  error?: string
}

const STATUS_COLOR: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  completed: 'success',
  failed: 'error',
  skipped: 'warning',
}
const STATUS_LABEL: Record<string, string> = {
  pending: '待执行',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  skipped: '已跳过',
}

export default function WFRerunPanel({ nodes, edges, onRerun, onSaveToNotebook }: {
  /** WF-④ 快照节点状态（事件驱动实时更新） */
  nodes: WFNodeRunState[]
  /** 依赖边（下游作废提示） */
  edges: { from: string; to: string }[]
  /** 一键重跑（消费 WF-④ RerunFrom——调用方执行并回传新快照） */
  onRerun?: (nodeId: string) => void
  /** 产物存笔记本（REQ-126 去向——Q-24 后由工作区绑定承载） */
  onSaveToNotebook?: (nodeId: string, output: string) => void
}) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes])

  const downstreamCount = (id: string) => {
    const adj = new Map<string, string[]>()
    for (const e of edges) {
      if (!adj.has(e.from)) adj.set(e.from, [])
      adj.get(e.from)!.push(e.to)
    }
    const seen = new Set<string>()
    const q = [id]
    while (q.length > 0) {
      const cur = q.shift()!
      for (const to of adj.get(cur) ?? []) {
        if (!seen.has(to)) { seen.add(to); q.push(to) }
      }
    }
    return seen.size - 1 // 不含自身
  }

  if (nodes.length === 0) {
    return (
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        暂无执行计划步骤。
      </Typography.Text>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <Typography.Text strong style={{ fontSize: 13 }}>执行进度</Typography.Text>
      {nodes.map((n) => (
        <div key={n.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 8px', border: '1px solid var(--c-border, #eee)', borderRadius: 6 }}>
          <Tag color={STATUS_COLOR[n.status] ?? 'default'} style={{ margin: 0, fontSize: 11 }}>
            {STATUS_LABEL[n.status] ?? n.status}
          </Tag>
          <Typography.Text style={{ fontSize: 13, flex: 1 }}>
            {n.name || n.id}
            {n.attempt != null && n.attempt > 0 && (
              <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 4 }}>（第 {n.attempt + 1} 次）</Typography.Text>
            )}
          </Typography.Text>
          {/* ② 失败节点一键重跑（消费 WF-④ rerun 端点——下游作废语义一致） */}
          {n.status === 'failed' && onRerun && (
            <Popconfirm
              title={`重跑此步骤？下游 ${downstreamCount(n.id)} 个步骤的结果将作废重执行。`}
              okText="重跑"
              cancelText="取消"
              onConfirm={() => onRerun(n.id)}
            >
              <Button size="small" type="text" icon={<RedoOutlined />} aria-label="重跑此步骤" />
            </Popconfirm>
          )}
          {/* ③ 产物一键存笔记本（REQ-126 去向——工作区绑定笔记本） */}
          {n.status === 'completed' && n.output && onSaveToNotebook && (
            <Button
              size="small"
              type="text"
              icon={<SaveOutlined />}
              aria-label="存到笔记本"
              onClick={() => onSaveToNotebook(n.id, n.output ?? '')}
            />
          )}
          {n.status === 'failed' && n.error && (
            <Button size="small" type="text" onClick={() => setExpanded(expanded === n.id ? null : n.id)}>
              {expanded === n.id ? '收起' : '详情'}
            </Button>
          )}
        </div>
      ))}
      {/* 失败原因展开 */}
      {expanded && byId.get(expanded)?.error && (
        <Alert type="error" showIcon title="失败原因" description={byId.get(expanded)!.error} />
      )}
      {expanded && byId.get(expanded)?.output && (
        <Alert type="info" showIcon title="产出" description={byId.get(expanded)!.output} />
      )}
    </div>
  )
}
