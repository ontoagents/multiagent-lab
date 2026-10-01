import { useEffect, useMemo, useState } from 'react'
import { Checkbox, Empty, Splitter, Space, Tag, Typography } from 'antd'
import { Background, BackgroundVariant, Controls, Handle, MarkerType, MiniMap, Position, ReactFlow, ReactFlowProvider, useEdgesState, useNodesState, useReactFlow } from '@xyflow/react'
import type { Edge, Node, NodeProps, NodeTypes } from '@xyflow/react'
// React Flow 基础样式表（v12 必需）：缺失时画布/节点/连线/MiniMap 全部无样式错乱（bugfix：此前从未引入）
import '@xyflow/react/dist/style.css'
import type { Spec, SpecConcept } from '../../../api/types'

// ---------------------------------------------------------------------------
// S4 可视化（React Flow 交互式图谱，@xyflow/react v12；D-O12 收口：内置默认定案）
// REQ-240①/M66：一等公民分类筛选——节点（概念/实例）与边（关系/继承/属于）正交组合，
//   与根概念过滤（三维图例面板）同语义；实例节点挂其概念下方。
// REQ-240②/M66：选中聚焦标准化——单击节点=邻居与关联边保持、无关节点边降透明度（.dimmed），
//   点击空白区域（onPaneClick）恢复全亮；与三维（Graph3D applyHighlight+onBackgroundClick）、
//   运行态实渲（复用 Graph3D）三视图行为一致。
// ---------------------------------------------------------------------------

const NODE_W = 168
const COL_GAP = 46
const ROW_STEP = 112
const PAD = 32
/** 实例节点行相对概念节点的纵向偏移（REQ-240① 显示实例时） */
const INST_OFFSET_Y = 78

function computeDepths(concepts: SpecConcept[]): Map<string, number> {
  const byName = new Map(concepts.map((c) => [c.name, c]))
  const depth = new Map<string, number>()
  const visiting = new Set<string>()
  const calc = (name: string): number => {
    if (depth.has(name)) return depth.get(name)!
    if (visiting.has(name)) return 0 // 环：降级
    visiting.add(name)
    const c = byName.get(name)
    const parents = (c?.parents ?? []).filter((p) => byName.has(p))
    const d = parents.length ? 1 + Math.max(...parents.map(calc)) : 0
    visiting.delete(name)
    depth.set(name, d)
    return d
  }
  for (const c of concepts) calc(c.name)
  return depth
}

interface NodePos {
  c: SpecConcept
  x: number
  y: number
}

function layoutGraph(spec: Spec): NodePos[] {
  const concepts = spec.concepts ?? []
  const depth = computeDepths(concepts)
  const levels = new Map<number, SpecConcept[]>()
  for (const c of concepts) {
    const d = depth.get(c.name) ?? 0
    if (!levels.has(d)) levels.set(d, [])
    levels.get(d)!.push(c)
  }
  const rows = [...levels.entries()].sort((a, b) => a[0] - b[0])
  const maxCols = Math.max(1, ...rows.map(([, cs]) => cs.length))
  const width = PAD * 2 + maxCols * NODE_W + Math.max(0, maxCols - 1) * COL_GAP
  const nodes: NodePos[] = []
  for (const [d, cs] of rows) {
    cs.sort((a, b) => a.name.localeCompare(b.name))
    const rowW = cs.length * NODE_W + Math.max(0, cs.length - 1) * COL_GAP
    const startX = (width - rowW) / 2
    cs.forEach((c, i) => nodes.push({ c, x: startX + i * (NODE_W + COL_GAP), y: PAD + d * ROW_STEP }))
  }
  return nodes
}

/** 节点数据（React Flow node.data）：显示名 + 原名 + 实例数 + 定义（悬停/详情） */
interface ConceptData extends Record<string, unknown> {
  label: string
  name: string
  count: number
  definition?: string
  /** REQ-240②：无关节点淡化标记（外部经 setNodes 更新，不进数据流） */
  dimmed?: boolean
}
type ConceptFlowNode = Node<ConceptData, 'concept'>

/** 实例节点（REQ-240①：2D 显示实例开关） */
interface InstanceData extends Record<string, unknown> {
  label: string
  name: string
  concept: string
  definition?: string
  dimmed?: boolean
}
type InstanceFlowNode = Node<InstanceData, 'instance'>

/** 每个概念的实例数（节点徽标 / 详情） */
export function instanceCounts(spec: Spec): Map<string, number> {
  const m = new Map<string, number>()
  for (const inst of spec.instances ?? []) m.set(inst.concept, (m.get(inst.concept) ?? 0) + 1)
  return m
}

/** REQ-240①：一等公民筛选（与三维图例面板同语义正交组合） */
export interface SpecVizFilter {
  /** 边类型：关系（实线）/继承（虚线）/属于（概念→实例） */
  links: Set<'rel' | 'parent' | 'instance'>
  /** 显示实例节点（挂其概念下方；关闭时仅概念层） */
  showInstances: boolean
  /** 仅显示有实例的概念（实例挂载筛选） */
  onlyWithInstances: boolean
}

export const DEFAULT_VIZ_FILTER: SpecVizFilter = {
  links: new Set(['rel', 'parent', 'instance']),
  showInstances: false,
  onlyWithInstances: false,
}

/** Spec → React Flow 节点：按父深度分层给初始坐标；实例节点挂其概念下方（showInstances） */
function buildNodes(spec: Spec, filter: SpecVizFilter): ConceptFlowNode[] {
  const counts = instanceCounts(spec)
  const poses = layoutGraph(spec)
  const instByConcept = new Map<string, Spec['instances']>()
  if (filter.showInstances) {
    for (const inst of spec.instances ?? []) {
      if (!instByConcept.has(inst.concept)) instByConcept.set(inst.concept, [])
      instByConcept.get(inst.concept)!.push(inst)
    }
  }
  const nodes: any[] = []
  const instNodes: any[] = []
  for (const { c, x, y } of poses) {
    if (filter.onlyWithInstances && (counts.get(c.name) ?? 0) === 0) continue
    nodes.push({
      id: c.name,
      type: 'concept' as const,
      position: { x, y },
      style: { width: NODE_W },
      data: { label: c.label || c.name, name: c.name, count: counts.get(c.name) ?? 0, definition: c.definition },
    })
    const list = instByConcept.get(c.name) ?? []
    list.slice(0, 8).forEach((inst, i) => {
      instNodes.push({
        id: `i:${inst.name}`,
        type: 'instance' as const,
        position: { x: x + i * 96 - Math.min(list.length - 1, 7) * 48 + 24, y: y + INST_OFFSET_Y },
        data: { label: inst.name, name: inst.name, concept: c.name },
      })
    })
  }
  return [...nodes, ...instNodes]
}

/**
 * Spec → React Flow 边（按 filter.links 正交筛选）：
 *  - 关系：实线 + 标签 + 箭头（source=from → target=to）；
 *  - 父子：虚线（source=父 → target=子，保证自上而下走向）；
 *  - 属于：概念→实例（showInstances 时）。
 */
function buildEdges(spec: Spec, filter: SpecVizFilter): Edge[] {
  const names = new Set((spec.concepts ?? []).map((c) => c.name))
  const counts = instanceCounts(spec)
  const edges: Edge[] = []
  for (const r of spec.relations ?? []) {
    if (!filter.links.has('rel')) break
    if (!names.has(r.from) || !names.has(r.to)) continue
    edges.push({
      id: `rel:${r.name}:${r.from}:${r.to}`,
      source: r.from,
      target: r.to,
      label: r.label || r.name,
      type: 'smoothstep',
      className: 'onto-flow-edge-rel',
      markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 },
    })
  }
  if (filter.links.has('parent')) {
    for (const c of spec.concepts ?? []) {
      for (const p of c.parents ?? []) {
        if (!names.has(p)) continue
        edges.push({
          id: `parent:${p}:${c.name}`,
          source: p,
          target: c.name,
          type: 'smoothstep',
          className: 'onto-flow-edge-parent',
          style: { strokeDasharray: '5 4' },
        })
      }
    }
  }
  if (filter.showInstances && filter.links.has('instance')) {
    for (const inst of spec.instances ?? []) {
      if (!names.has(inst.concept)) continue
      edges.push({
        id: `belongs:${inst.concept}:${inst.name}`,
        source: inst.concept,
        target: `i:${inst.name}`,
        type: 'smoothstep',
        className: 'onto-flow-edge-parent',
        style: { strokeDasharray: '2 3', opacity: 0.7 },
      })
    }
  }
  // onlyWithInstances：端点概念被滤掉的边显式剔除（React Flow 对缺失端点会告警）
  const visibleConcepts = new Set((spec.concepts ?? []).filter((c) => !filter.onlyWithInstances || (counts.get(c.name) ?? 0) > 0).map((c) => c.name))
  return edges.filter((e) => {
    const s = e.source.startsWith('i:') || visibleConcepts.has(e.source)
    const t = e.target.startsWith('i:') || visibleConcepts.has(e.target)
    return s && t
  })
}

/** 自定义概念节点：名称 + 挂载徽标（实例数=右上；缺失定义=左上「?」）；title 承载定义 */
function ConceptNode({ data, selected }: NodeProps<ConceptFlowNode>) {
  return (
    <div
      className={`onto-flow-node${selected ? ' selected' : ''}${data.dimmed ? ' dimmed' : ''}`}
      title={data.definition || data.label}
    >
      <Handle type="target" position={Position.Top} className="onto-flow-handle" />
      <span className="onto-flow-node-label">{data.label}</span>
      {/* REQ-240⑤/M66：节点挂载徽标——有实例一眼可见（数字）；无定义警示（?） */}
      {data.count > 0 && <span className="onto-flow-node-badge">{data.count}</span>}
      {!data.definition && <span className="onto-flow-node-badge missing" title="缺失定义注释（qualitygate missing_definition 对应）">?</span>}
      <Handle type="source" position={Position.Bottom} className="onto-flow-handle" />
    </div>
  )
}

/** 实例节点（REQ-240①）：小圆点形态，title 承载归属概念 */
function InstanceNode({ data, selected }: NodeProps<InstanceFlowNode>) {
  return (
    <div
      className={`onto-flow-inst${selected ? ' selected' : ''}${data.dimmed ? ' dimmed' : ''}`}
      title={`${data.name}（实例 · 属于 ${data.concept}）`}
    >
      <Handle type="target" position={Position.Top} className="onto-flow-handle" />
      <span className="onto-flow-inst-label">{data.label}</span>
      <Handle type="source" position={Position.Bottom} className="onto-flow-handle" />
    </div>
  )
}

// nodeTypes 必须定义在组件外，避免每次渲染重建导致 React Flow 重挂载
const nodeTypes: NodeTypes = { concept: ConceptNode, instance: InstanceNode }

function SpecGraphInner({ spec, focusName }: { spec: Spec | null; focusName?: string | null }) {
  const hasConcepts = !!spec && (spec.concepts?.length ?? 0) > 0
  // REQ-240①：一等公民筛选（边类型/实例维度；与三维根过滤正交）
  const [filter, setFilter] = useState<SpecVizFilter>(DEFAULT_VIZ_FILTER)
  const initialNodes = useMemo(() => (spec ? buildNodes(spec, filter) : []), [spec, filter])
  const initialEdges = useMemo(() => (spec ? buildEdges(spec, filter) : []), [spec, filter])
  const [nodes, setNodes, onNodesChange] = useNodesState<any>(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(initialEdges)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const { setCenter } = useReactFlow()

  // R3（VIZ-2）：3D→2D 联动——focusName 变化时居中该概念并选中（bugfix：节点 id 无 c: 前缀）
  useEffect(() => {
    if (!focusName) return
    const n = initialNodes.find((x) => x.id === focusName || x.id === `c:${focusName}`)
    if (!n) return
    setSelectedId(focusName)
    setCenter(n.position.x + NODE_W / 2, n.position.y + 40, { zoom: 1.1, duration: 600 })
  }, [focusName, initialNodes, setCenter])

  // Spec 变化时重置图谱（拖拽后的坐标不跨 Spec 版本保留）
  useEffect(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
    setSelectedId(null)
  }, [initialNodes, initialEdges, setNodes, setEdges])

  // REQ-240②：选中聚焦——邻居与关联边保持、其余降透明度（.dimmed）；空白恢复经 onPaneClick 清除
  useEffect(() => {
    const dim = selectedId != null
    setNodes((cur) =>
      cur.map((n) => {
        let d = false
        if (dim && n.id !== selectedId) {
          const isNeighbor = initialEdges.some(
            (e) => (e.source === selectedId && e.target === n.id) || (e.target === selectedId && e.source === n.id),
          )
          d = !isNeighbor
        }
        return d === !!n.data.dimmed ? n : { ...n, data: { ...n.data, dimmed: d } }
      }),
    )
    setEdges((cur) =>
      cur.map((e) => {
        const keep = selectedId == null || e.source === selectedId || e.target === selectedId
        const base = (e.className ?? '').replace(/\s*dimmed/g, '')
        const cls = keep ? base : `${base} dimmed`.trim()
        return cls === e.className ? e : { ...e, className: cls }
      }),
    )
  }, [selectedId, initialEdges, setNodes, setEdges])

  const counts = useMemo(() => (spec ? instanceCounts(spec) : new Map<string, number>()), [spec])
  const selected = useMemo(
    () => (spec && selectedId && !selectedId.startsWith('i:') ? spec.concepts.find((c) => c.name === selectedId) ?? null : null),
    [spec, selectedId],
  )
  const selectedInstance = useMemo(
    () => (spec && selectedId?.startsWith('i:') ? spec.instances?.find((i) => `i:${i.name}` === selectedId) ?? null : null),
    [spec, selectedId],
  )
  const selectedInstances = useMemo(
    () => (spec && selected ? (spec.instances ?? []).filter((i) => i.concept === selected.name) : []),
    [spec, selected],
  )

  if (!hasConcepts || !spec) {
    return (
      <div className="work-empty" style={{ minHeight: 220 }}>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无概念可可视化；请先在构建流程保存含概念的 Spec" />
      </div>
    )
  }

  return (
    <Splitter className="onto-flow-split" orientation="horizontal">
      <Splitter.Panel defaultSize="68%" min="40%">
        <div className="onto-flow-pane">
          {/* REQ-240①：筛选条（边类型/实例维度，与三维图例面板正交同语义） */}
          <div className="onto-viz-filter" data-testid="spec-viz-filter">
            <span className="onto-viz-filter-label">要素筛选</span>
            <Checkbox.Group
              value={[...filter.links]}
              onChange={(vs) => setFilter((f) => ({ ...f, links: new Set(vs.length ? (vs as any) : ['rel', 'parent']) }))}
              options={[
                { value: 'rel', label: `关系 ${spec.relations?.length ?? 0}` },
                { value: 'parent', label: '继承' },
                { value: 'instance', label: '属于' },
              ]}
            />
            <Checkbox checked={filter.showInstances} onChange={(e) => setFilter((f) => ({ ...f, showInstances: e.target.checked }))}>
              显示实例 {spec.instances?.length ?? 0}
            </Checkbox>
            <Checkbox checked={filter.onlyWithInstances} onChange={(e) => setFilter((f) => ({ ...f, onlyWithInstances: e.target.checked }))}>
              仅显示有实例的概念
            </Checkbox>
          </div>
          <ReactFlow
            key={`${initialNodes.map((n) => n.id).join('|')}|${filter.showInstances}|${filter.onlyWithInstances}|${[...filter.links].join()}`}
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            minZoom={0.2}
            maxZoom={2}
            nodesConnectable={false}
            onNodeClick={(_, n) => setSelectedId(n.id)}
            onPaneClick={() => setSelectedId(null)}
            className="onto-flow"
          >
            <Background variant={BackgroundVariant.Dots} gap={18} size={1.5} color="#c9cee0" />
            <MiniMap position="top-right" pannable zoomable nodeColor="#c9cef3" maskColor="rgba(246, 247, 251, 0.72)" />
            <Controls showInteractive={false} position="bottom-left" />
          </ReactFlow>
        </div>
      </Splitter.Panel>
      <Splitter.Panel min="22%">
        <div className="onto-flow-info">
          <div className="onto-flow-info-title">图例</div>
          <div className="onto-flow-legend">
            <span className="onto-flow-legend-line rel" />
            <span>实线 = 关系（from → to）</span>
          </div>
          <div className="onto-flow-legend">
            <span className="onto-flow-legend-line parent" />
            <span>虚线 = 继承 / 属于</span>
          </div>
          <div className="onto-flow-legend">
            <span className="onto-flow-legend-badge">n</span>
            <span>节点徽标 = 实例数</span>
          </div>

          <div className="onto-flow-info-title spaced">统计</div>
          <div className="onto-flow-stats">
            <span>概念 <b>{spec.concepts.length}</b></span>
            <span>关系 <b>{spec.relations?.length ?? 0}</b></span>
            <span>实例 <b>{spec.instances?.length ?? 0}</b></span>
          </div>

          <div className="onto-flow-info-title spaced">选中节点</div>
          {selected ? (
            <div className="onto-flow-detail">
              <div className="onto-flow-detail-name">{selected.label || selected.name}</div>
              <div className="onto-flow-detail-key">{selected.name}</div>
              <p className={`onto-flow-detail-def${selected.definition ? '' : ' muted'}`}>
                {selected.definition || '未填写定义'}
              </p>
              <div className="onto-flow-detail-row">
                <span className="onto-flow-detail-label">父概念</span>
                {selected.parents && selected.parents.length > 0 ? (
                  <Space size={4} wrap>
                    {selected.parents.map((p) => (
                      <Tag key={p} style={{ margin: 0 }}>{p}</Tag>
                    ))}
                  </Space>
                ) : (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>
                )}
              </div>
              <div className="onto-flow-detail-row">
                <span className="onto-flow-detail-label">实例</span>
                <Typography.Text style={{ fontSize: 12 }}>{counts.get(selected.name) ?? 0} 个</Typography.Text>
              </div>
              {selectedInstances.length > 0 && (
                <Space size={4} wrap style={{ marginTop: 6 }}>
                  {selectedInstances.slice(0, 12).map((i) => (
                    <Tag key={i.name} color="purple" style={{ margin: 0 }}>{i.name}</Tag>
                  ))}
                  {selectedInstances.length > 12 && (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>等 {selectedInstances.length} 个</Typography.Text>
                  )}
                </Space>
              )}
            </div>
          ) : selectedInstance ? (
            <div className="onto-flow-detail">
              <div className="onto-flow-detail-name">{selectedInstance.name}</div>
              <div className="onto-flow-detail-key">实例 · 属于 {selectedInstance.concept}</div>
              {selectedInstance.attributes && Object.keys(selectedInstance.attributes).length > 0 && (
                <div className="onto-flow-detail-row">
                  <span className="onto-flow-detail-label">属性</span>
                  <Typography.Text style={{ fontSize: 12 }}>{Object.entries(selectedInstance.attributes).map(([k, v]) => `${k}=${String(v)}`).slice(0, 6).join('；')}</Typography.Text>
                </div>
              )}
            </div>
          ) : (
            <p className="onto-flow-hint">点击节点聚焦其邻居（其余淡化），点击空白恢复；滚轮缩放、拖拽平移 / 节点。</p>
          )}
        </div>
      </Splitter.Panel>
    </Splitter>
  )
}

/** R3（VIZ-2）：Provider 包装（useReactFlow 需要上下文）；3D↔2D 联动经 focusName 传入 */
export default function SpecGraph(props: { spec: Spec | null; focusName?: string | null }) {
  return (
    <ReactFlowProvider>
      <SpecGraphInner {...props} />
    </ReactFlowProvider>
  )
}
