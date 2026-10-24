import { useEffect, useMemo, useState } from 'react'
import { Background, BackgroundVariant, Handle, Position, ReactFlow, MarkerType } from '@xyflow/react'
import type { Edge, Node, NodeProps, NodeTypes } from '@xyflow/react'
import { Empty, Select, Space, Spin, Tag, Typography } from 'antd'
import { api } from '../api/client'
import type { Agent, Project, RunEventDTO } from '../api/types'

// ---------------------------------------------------------------------------
// REQ-311 B1/M115：项目协作拓扑图（CollabView「协作拓扑」段）——静态拓扑（协调者→成员
// 有向图 + collab_mode/workflow_mode 标注）+ 运行回放（选项目会话拉 subagent.enter/exit
// 事件：成员被委派计数与在栈高亮）。数据零新后端（project/conversations/events 既有 API）；
// @xyflow/react 复用 SpecGraph/KGGraphView 先例。
// 诚实定位（71 号 §4.2/§7）：单 agent 直聊无对象，价值集中在项目会话；拓扑呈现「发生了什么」
// 而非「为什么」——委派决策理由系模型侧行为，平台只能透出任务描述与 enter 事件。
// ---------------------------------------------------------------------------

type TopoNodeData = {
  name: string
  role: 'coordinator' | 'member'
  count: number
  inStack: boolean
  via?: string
  isPlaceholder?: boolean
  /** REQ-416/M212：外部 A2A 成员 / REQ-426：外部 CLI 成员（channel=a2a|cli 委派聚合；endpoint/backend 悬浮展示） */
  external?: boolean
  endpoint?: string
  channel?: string
  backend?: string
  /** REQ-428/M224：过程转译事件计数（external.audit 聚合——拓扑回放活动态） */
  activity?: number
}

function TopoNode({ data }: NodeProps) {
  const d = data as unknown as TopoNodeData
  return (
    <div
      data-testid={d.role === 'coordinator' ? 'topo-coordinator' : 'topo-member'}
      title={d.external ? (d.channel === 'cli' ? `CLI 外部成员（后端 ${d.backend ?? '—'}）` : `A2A 端点：${d.endpoint ?? ''}`) : undefined}
      style={{
        padding: '6px 12px',
        borderRadius: 10,
        border: `1.5px solid ${d.inStack ? 'var(--c-brand, #4f7cff)' : 'var(--c-line)'}`,
        background: d.inStack ? 'rgba(79,124,255,0.08)' : 'var(--c-panel)',
        boxShadow: d.inStack ? '0 0 0 3px rgba(79,124,255,0.15)' : 'none',
        textAlign: 'center',
        minWidth: 96,
      }}
    >
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden' }} />
      <div style={{ fontSize: 12, fontWeight: 700, color: d.isPlaceholder ? 'var(--c-ink-3)' : 'var(--c-ink)' }}>
        {d.isPlaceholder ? '未指定主智能体' : d.name}
      </div>
      <div style={{ fontSize: 10.5, color: 'var(--c-ink-3)', marginTop: 2 }}>
        {d.role === 'coordinator'
          ? '协调者'
          : d.external
            ? `${d.channel === 'cli' ? '外部 CLI 成员' : '外部 A2A 成员'}${d.count > 0 ? ` · 委派 ${d.count} 次` : ''}${d.activity ? ` · 过程 ${d.activity} 条` : ''}`
            : `成员${d.count > 0 ? ` · 委派 ${d.count} 次` : ''}${d.via === 'transfer' ? ' · transfer' : ''}`}
      </div>
      {d.inStack && (
        <div style={{ fontSize: 10, color: 'var(--c-brand, #4f7cff)', marginTop: 1 }}>● 运行中</div>
      )}
      <Handle type="source" position={Position.Bottom} style={{ visibility: 'hidden' }} />
    </div>
  )
}

const nodeTypes: NodeTypes = { topo: TopoNode }

type SubagentStat = { enters: number; exits: number; inStack: boolean; via?: string; delegates?: number; external?: boolean; endpoint?: string; channel?: string; backend?: string; activity?: number }

/** subagent.enter/exit 事件升序聚合：每成员委派次数与在栈状态（transfer 也计入并标注）；
 * REQ-416/REQ-426：subagent.delegate channel=a2a/cli 聚合为外部成员（delegates 计数+
 * endpoint/backend，无 enter/exit 栈语义），拓扑按 channel 区分外部成员两类 */
function aggregateSubagentEvents(events: RunEventDTO[]): Record<string, SubagentStat> {
  const stats: Record<string, SubagentStat> = {}
  for (const e of events) {
    let d: any = {}
    try { d = e.data ? JSON.parse(e.data) : {} } catch { /* 跳过坏载荷 */ }
    if (e.type === 'subagent.delegate') {
      if ((d.channel !== 'a2a' && d.channel !== 'cli') || !d.member) continue
      const st = (stats[String(d.member)] ??= { enters: 0, exits: 0, inStack: false })
      st.delegates = (st.delegates ?? 0) + 1
      st.external = true
      st.channel = String(d.channel)
      if (d.endpoint) st.endpoint = String(d.endpoint)
      if (d.backend) st.backend = String(d.backend)
      continue
    }
    if (e.type === 'external.audit') {
      // REQ-428/M224：过程转译事件聚合——外部成员节点回放活动计数（委派单之外的过程态）
      if (!d.member) continue
      const st = (stats[String(d.member)] ??= { enters: 0, exits: 0, inStack: false })
      st.activity = (st.activity ?? 0) + 1
      st.external = true
      if (!st.channel && d.channel) st.channel = String(d.channel)
      continue
    }
    const name = String(d.agent ?? '')
    if (!name) continue
    const st = (stats[name] ??= { enters: 0, exits: 0, inStack: false })
    if (e.type === 'subagent.enter') {
      st.enters += 1
      st.inStack = true
      if (d.via) st.via = String(d.via)
    } else if (e.type === 'subagent.exit') {
      st.exits += 1
      st.inStack = false
    }
  }
  return stats
}

export default function CollabTopology({ project, agents }: { project: Project; agents: Agent[] }) {
  const [convs, setConvs] = useState<{ id: string; title: string }[]>([])
  const [convId, setConvId] = useState<string | null>(null)
  const [stats, setStats] = useState<Record<string, SubagentStat>>({})
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let alive = true
    api
      .listConversations({ project_id: project.id, scope: 'project' })
      .then((ls) => { if (alive) setConvs(ls.map((c) => ({ id: c.id, title: c.title || c.id }))) })
      .catch(() => { if (alive) setConvs([]) })
    return () => { alive = false }
  }, [project.id])

  useEffect(() => {
    setStats({})
    if (!convId) return
    let alive = true
    setLoading(true)
    api
      .listEvents(convId, { type_prefix: 'subagent', limit: 500 })
      .then((r) => { if (alive) setStats(aggregateSubagentEvents(Array.isArray(r) ? r : [])) })
      .catch(() => { if (alive) setStats({}) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [convId])

  const { nodes, edges } = useMemo(() => {
    const memberIds = (project.agent_ids ?? []).filter((id) => id !== project.coordinator)
    const coord = agents.find((a) => a.id === project.coordinator)
    const nodes: Node[] = []
    const edges: Edge[] = []
    const coordCount = coord ? Object.entries(stats).filter(([name]) => name === coord.name).reduce((s, [, v]) => s + v.enters, 0) : 0
    nodes.push({
      id: 'coord',
      type: 'topo',
      position: { x: 130, y: 8 },
      data: {
        name: coord?.name ?? '',
        role: 'coordinator',
        count: coordCount,
        inStack: coord ? !!stats[coord.name]?.inStack : false,
        isPlaceholder: !coord,
      } as unknown as Record<string, unknown>,
    })
    memberIds.forEach((id, i) => {
      const a = agents.find((x) => x.id === id)
      const name = a?.name ?? id
      const st = stats[name]
      nodes.push({
        id: `m:${id}`,
        type: 'topo',
        position: { x: 20 + i * 132, y: 128 },
        data: { name, role: 'member', count: st?.enters ?? 0, inStack: !!st?.inStack, via: st?.via } as unknown as Record<string, unknown>,
      })
      edges.push({
        id: `e:${id}`,
        source: 'coord',
        target: `m:${id}`,
        animated: !!st?.inStack,
        label: st && st.enters > 0 ? `${project.collab_mode === 'transfer' ? '移交' : '委派'} ${st.enters}` : (project.collab_mode === 'transfer' ? '移交' : '委派'),
        style: st && st.enters > 0 ? { strokeWidth: 1.6 } : { strokeDasharray: '4 3' },
        markerEnd: { type: MarkerType.ArrowClosed },
      })
    })
    // REQ-416/M212 + REQ-426：外部成员节点（channel=a2a/cli 委派聚合；静态拓扑无此成员——
    // 运行回放出现）；边色区分：A2A 紫 / CLI volcano（泳道同色系）
    const memberNames = new Set(memberIds.map((id) => agents.find((x) => x.id === id)?.name ?? id))
    let extIdx = 0
    for (const [name, st] of Object.entries(stats)) {
      if (!st.external || memberNames.has(name)) continue
      const x = 20 + (memberIds.length + extIdx) * 132
      extIdx++
      nodes.push({
        id: `ext:${name}`,
        type: 'topo',
        position: { x, y: 128 },
        data: { name, role: 'member', count: st.delegates ?? 0, inStack: false, external: true, endpoint: st.endpoint, channel: st.channel, backend: st.backend, activity: st.activity } as unknown as Record<string, unknown>,
      })
      edges.push({
        id: `exte:${name}`,
        source: 'coord',
        target: `ext:${name}`,
        label: `${st.channel === 'cli' ? 'CLI' : 'A2A'} 委派 ${st.delegates ?? 0}`,
        style: { strokeWidth: 1.6, stroke: st.channel === 'cli' ? '#d46b08' : '#531dab' },
        markerEnd: { type: MarkerType.ArrowClosed },
      })
    }
    return { nodes, edges }
  }, [project, agents, stats])

  const totalCalls = Object.values(stats).reduce((s, v) => s + v.enters + (v.delegates ?? 0), 0)
  const transferCalls = Object.values(stats).filter((v) => v.via === 'transfer').reduce((s, v) => s + v.enters, 0)
  const a2aCalls = Object.values(stats).filter((v) => v.external).reduce((s, v) => s + (v.delegates ?? 0), 0)

  return (
    <div data-testid="collab-topology">
      <Space size={6} wrap style={{ marginBottom: 6 }}>
        <Tag style={{ margin: 0 }}>{project.collab_mode === 'transfer' ? 'transfer（路由移交）' : 'agent_as_tool（委派为工具）'}</Tag>
        <Tag style={{ margin: 0 }}>workflow: {project.workflow_mode ?? 'free'}</Tag>
        {totalCalls > 0 && (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            已选会话委派 {totalCalls} 次{transferCalls > 0 ? `（transfer ${transferCalls}）` : ''}{a2aCalls > 0 ? `（外部成员 ${a2aCalls}）` : ''}
          </Typography.Text>
        )}
      </Space>
      <Select
        size="small"
        allowClear
        placeholder="选择项目会话回放委派拓扑（静态拓扑无需选择）"
        style={{ width: '100%', marginBottom: 8 }}
        value={convId}
        onChange={(v) => setConvId(v ?? null)}
        options={convs.map((c) => ({ value: c.id, label: c.title }))}
        notFoundContent="该项目暂无会话"
        data-testid="topo-conv-select"
      />
      <div style={{ height: 232, border: '1px solid var(--c-line)', borderRadius: 10, overflow: 'hidden', background: 'var(--c-bg, #fff)' }}>
        {(project.agent_ids ?? []).length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="先在下方勾选成员智能体，拓扑随之生成" style={{ marginTop: 72 }} />
        ) : (
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            proOptions={{ hideAttribution: true }}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
            panOnScroll
            zoomOnScroll={false}
          >
            <Background variant={BackgroundVariant.Dots} gap={14} size={1} />
          </ReactFlow>
        )}
      </div>
      {loading && (
        <div style={{ textAlign: 'center', marginTop: 4 }}>
          <Spin size="small" />
        </div>
      )}
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
        拓扑呈现「发生了什么」（委派次数/在栈状态），不含委派决策理由（模型侧行为不可结构化捕获）；项目会话的逐事件明细见会话「调用轨迹」。
      </Typography.Text>
    </div>
  )
}
