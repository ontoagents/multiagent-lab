import { useEffect, useMemo, useState } from 'react'
import { Empty, Spin, Typography } from 'antd'
import { Background, BackgroundVariant, Controls, Handle, MarkerType, Position, ReactFlow, ReactFlowProvider } from '@xyflow/react'
import type { Edge, Node, NodeProps, NodeTypes } from '@xyflow/react'
import { api } from '../../../api/client'
// React Flow 基础样式表（v12 必需）——与 SpecGraph 同源
import '@xyflow/react/dist/style.css'

/**
 * 概念子图掌握度（REQ-572/M380——闭环段③）：mastery-graph 端点（spec concepts+relations ×
 * mastery_level 合并）的子图渲染——节点=概念（covered 按掌握度着色：绿 ≥0.8 / 金 ≥0.5 / 红 <0.5；
 * 灰=从未考察），边=声明关系（实线）+父继承（虚线）。复用 onto-flow-* 视觉语言（与构建面图谱
 * 同族）；出题定向即本图消费面（薄弱/未考察概念下一轮优先出题）。
 */

const NODE_W = 148
const COL_GAP = 40
const ROW_STEP = 96
const PAD = 28

interface GraphResp {
  nodes: { name: string; label: string; covered: boolean; score: number; asked: number; correct: number; parents?: string[] }[]
  edges: { from: string; to: string; kind: 'relation' | 'parent'; rel?: string; rel_label?: string }[]
}

/** 掌握度着色（与 QuizPane 掌握度 Tag 同阈值） */
export function masteryColor(m: { covered: boolean; score: number }): string {
  if (!m.covered) return '#b9bfd2' // 未考察=灰
  if (m.score >= 0.8) return '#52c41a'
  if (m.score >= 0.5) return '#faad14'
  return '#ff4d4f'
}

interface MasteryData extends Record<string, unknown> {
  label: string
  name: string
  covered: boolean
  score: number
  asked: number
}

function MasteryNode({ data }: NodeProps<Node<MasteryData>>) {
  const color = masteryColor(data)
  return (
    <div
      className="onto-flow-node"
      style={{ width: NODE_W, borderColor: data.covered ? color : undefined }}
      title={data.covered ? `${data.name}：掌握度 ${data.score.toFixed(2)}（${data.correct}/${data.asked}）` : `${data.name}：从未考察`}
    >
      <Handle type="target" position={Position.Top} className="onto-flow-handle" />
      <span className="onto-flow-node-label" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis' }}>{data.label}</span>
      {data.covered && (
        <span className="onto-flow-node-badge" style={{ background: color, color: '#fff' }}>{data.score.toFixed(1)}</span>
      )}
      {!data.covered && <span className="onto-flow-node-badge missing" title="从未考察">?</span>}
      <Handle type="source" position={Position.Bottom} className="onto-flow-handle" />
    </div>
  )
}

const nodeTypes: NodeTypes = { mastery: MasteryNode }

/** 父深度分层布局（SpecGraph computeDepths 同法收简） */
function layout(resp: GraphResp): { nodes: Node[]; edges: Edge[] } {
  const names = new Set(resp.nodes.map((n) => n.name))
  const byName = new Map(resp.nodes.map((n) => [n.name, n]))
  const depth = new Map<string, number>()
  const calc = (name: string): number => {
    if (depth.has(name)) return depth.get(name)!
    const parents = (byName.get(name)?.parents ?? []).filter((p) => names.has(p))
    let d = 0
    if (parents.length) {
      const seen = new Set<string>()
      seen.add(name)
      const safe = parents.filter((p) => !seen.has(p))
      if (safe.length) d = 1 + Math.max(...safe.map(calc))
    }
    depth.set(name, d)
    return d
  }
  for (const n of resp.nodes) calc(n.name)
  const levels = new Map<number, string[]>()
  for (const n of resp.nodes) {
    const d = depth.get(n.name) ?? 0
    if (!levels.has(d)) levels.set(d, [])
    levels.get(d)!.push(n.name)
  }
  const nodes: Node[] = []
  const rows = [...levels.entries()].sort((a, b) => a[0] - b[0])
  const maxCols = Math.max(1, ...rows.map(([, ns]) => ns.length))
  const width = PAD * 2 + maxCols * NODE_W + Math.max(0, maxCols - 1) * COL_GAP
  for (const [d, ns] of rows) {
    ns.sort((a, b) => a.localeCompare(b))
    const rowW = ns.length * NODE_W + Math.max(0, ns.length - 1) * COL_GAP
    const startX = (width - rowW) / 2
    ns.forEach((name, i) => {
      const m = byName.get(name)!
      nodes.push({
        id: name,
        type: 'mastery' as const,
        position: { x: startX + i * (NODE_W + COL_GAP), y: PAD + d * ROW_STEP },
        style: { width: NODE_W },
        data: { label: m.label || m.name, name: m.name, covered: m.covered, score: m.score, asked: m.asked },
      })
    })
  }
  const edges: Edge[] = resp.edges.map((e, i) => ({
    id: `e${i}:${e.from}:${e.to}`,
    source: e.kind === 'parent' ? e.to : e.from, // 继承边父→子（自上而下）
    target: e.kind === 'parent' ? e.from : e.to,
    label: e.rel_label || e.rel,
    type: 'smoothstep',
    className: e.kind === 'relation' ? 'onto-flow-edge-rel' : 'onto-flow-edge-parent',
    style: e.kind === 'parent' ? { strokeDasharray: '5 4' } : undefined,
    markerEnd: e.kind === 'relation' ? { type: MarkerType.ArrowClosed, width: 12, height: 12 } : undefined,
  }))
  return { nodes, edges }
}

function MasteryGraphInner({ ontoId, refreshKey }: { ontoId: string; refreshKey: number }) {
  const [resp, setResp] = useState<{ loading: boolean; data: GraphResp | null; error: string | null }>({ loading: true, data: null, error: null })
  useEffect(() => {
    if (!ontoId) return
    let live = true
    api
      .quizMasteryGraph(ontoId)
      .then((r) => live && setResp({ loading: false, data: r, error: null }))
      .catch((e) => live && setResp({ loading: false, data: null, error: e?.message ?? '加载失败' }))
    return () => {
      live = false
    }
  }, [ontoId, refreshKey])

  const { nodes, edges } = useMemo(() => (resp.data ? layout(resp.data) : { nodes: [], edges: [] }), [resp.data])

  if (resp.loading) return <Spin size="small" />
  if (resp.error) return <Typography.Text type="secondary" style={{ fontSize: 12 }}>掌握度子图加载失败：{resp.error}</Typography.Text>
  if (!resp.data || resp.data.nodes.length === 0)
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该本体无概念，无法生成掌握度子图" />

  return (
    <div style={{ height: 300, border: '1px solid var(--c-border, #e5e7f0)', borderRadius: 8, overflow: 'hidden' }} data-testid="mastery-graph">
      <ReactFlow
        key={nodes.map((n) => n.id).join('|')}
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        nodesConnectable={false}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={1.5}
        proOptions={{ hideAttribution: true }}
        className="onto-flow"
      >
        <Background variant={BackgroundVariant.Dots} gap={18} size={1.5} color="#c9cee0" />
        <Controls showInteractive={false} position="bottom-left" />
      </ReactFlow>
    </div>
  )
}

export default function MasteryGraph(props: { ontoId: string; refreshKey: number }) {
  return (
    <ReactFlowProvider>
      <MasteryGraphInner {...props} />
    </ReactFlowProvider>
  )
}
