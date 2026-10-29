import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Empty, Spin, Table, Tabs, Tag, Tooltip, Typography } from 'antd'
import { CopyOutlined, ReloadOutlined } from '@ant-design/icons'
import { api } from '../../api/client'
import type { A2aCardEntry, A2aTaskInfo, Agent, McpServeInfo } from '../../api/types'
import LoadErrorAlert from '../../components/LoadErrorAlert'

// REQ-418/M214：设置页「协议」总览面——MCP 服务面（mcp_serve 接入指引）/A2A 服务面
//（发现端点+agent card 只读预览+capabilities 如实标注）/A2A 任务列表（a2a_task 可视——
// 外部经协议进来的任务界面可见）。诚实边界：不做协议报文级调试器（showRaw 面承载）、
// agent card 只读预览不做在线编辑。

const STATE_COLOR: Record<string, string> = {
  working: 'processing', submitted: 'default', completed: 'success',
  failed: 'error', canceled: 'warning', rejected: 'error', 'input-required': 'warning',
}

function copyText(t: string, ok = '已复制') {
  navigator.clipboard.writeText(t).then(() => undefined).catch(() => undefined)
  // 消息提示交由调用侧（本面用静态文案，避免 message 依赖注入）
  void ok
}

export function ProtocolPane() {
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [loadErr, setLoadErr] = useState<string | null>(null) // REQ-515/M322 三态收口
  const [serves, setServes] = useState<Record<string, McpServeInfo | null>>({})
  const [cards, setCards] = useState<A2aCardEntry[] | null>(null)
  const [tasks, setTasks] = useState<A2aTaskInfo[] | null>(null)

  const load = () => {
    // REQ-515/M322 三态收口：智能体列表失败进错误态，不再吞成空列表
    api.listAgents().then((as) => {
      setAgents(as)
      setLoadErr(null)
      Promise.all(as.map(async (a) => [a.id, await api.getAgentMcpServe(a.id).catch(() => null)] as const))
        .then((entries) => setServes(Object.fromEntries(entries.filter(([, v]) => v != null))))
    }).catch((e) => {
      setAgents([])
      setLoadErr(e?.message ?? '智能体列表加载失败')
    })
    api.listA2aCards().then((r) => setCards(r.cards ?? [])).catch(() => setCards([]))
    api.listA2aTasks(100).then((r) => setTasks(r.tasks ?? [])).catch(() => setTasks([]))
  }
  useEffect(load, [])

  const serveEnabled = useMemo(() => Object.entries(serves).filter(([, v]) => v && v.enabled) as [string, McpServeInfo][], [serves])
  const agentName = useMemo(() => {
    const m: Record<string, string> = {}
    for (const a of agents ?? []) m[a.id] = a.name
    for (const c of cards ?? []) m[c.agent_id] = c.name
    return (id: string) => m[id] ?? id
  }, [agents, cards])

  if (agents === null || cards === null || tasks === null) {
    return <div style={{ padding: 40, textAlign: 'center' }}><Spin /></div>
  }

  const mcpExample = JSON.stringify({
    mcpServers: {
      'eino-agent': {
        url: `${location.origin}/mcp`,
        headers: { Authorization: 'Bearer <智能体对外 Token（侧板「对外服务」生成）>' },
      },
    },
  }, null, 2)

  const mcpTab = (
    <div data-testid="protocol-mcp">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        message="MCP 服务面：平台智能体可作为 MCP server 被外部客户端（Claude Desktop / Cursor 等）调用"
        description="每智能体一个工具（侧板「对外服务」开启并生成 Token）；外部客户端经 Streamable HTTP 握手 tools/list → tools/call。鉴权：智能体级 Token 或平台凭证（PLATFORM_TOKEN）。"
      />
      {loadErr && (
        <LoadErrorAlert title="智能体列表加载失败" message={loadErr} onRetry={load} style={{ marginBottom: 10 }} />
      )}
      {serveEnabled.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未有智能体开启 MCP 对外服务——在智能体侧板「对外服务」开启" />
      ) : (
        <Table
          size="small"
          rowKey={(r) => r.agent_id}
          dataSource={serveEnabled.map(([id, v]) => ({ agent_id: id, name: agentName(id), ...v }))}
          pagination={false}
          columns={[
            { title: '智能体', dataIndex: 'name', render: (v: string) => <Typography.Text strong style={{ fontSize: 13 }}>{v}</Typography.Text> },
            { title: '暴露工具', dataIndex: 'tool_name', render: (v: string) => <Tag color="purple" style={{ margin: 0 }}><code style={{ fontSize: 11 }}>{v}</code></Tag> },
            { title: '端点', dataIndex: 'endpoint', render: (v: string) => <Typography.Text code style={{ fontSize: 12 }}>{v}</Typography.Text> },
            { title: 'Token', dataIndex: 'token_mask', render: (v: string) => <Typography.Text type="secondary" style={{ fontSize: 12 }}>{v || '未配置'}</Typography.Text> },
          ]}
        />
      )}
      <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 4, fontSize: 12 }}>
        外部客户端接入示例（Claude Desktop / Cursor 的 mcpServers 配置）：
        <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyText(mcpExample)}>复制</Button>
      </Typography.Paragraph>
      <pre data-testid="protocol-mcp-example" style={{ background: 'var(--c-bg, #fafafa)', border: '1px solid var(--c-line)', borderRadius: 8, padding: 10, fontSize: 11.5, overflow: 'auto' }}>{mcpExample}</pre>
    </div>
  )

  const a2aTab = (
    <div data-testid="protocol-a2a">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        message="A2A 服务面：平台智能体经 A2A 标准化对外（发现 → 委派 → 任务轮询）"
        description={
          <span>
            发现端点 <Typography.Text code style={{ fontSize: 12 }}>{location.origin}/api/a2a/agents</Typography.Text>；
            每智能体一卡一端点（<Typography.Text code style={{ fontSize: 12 }}>{location.origin}/api/a2a/agents/{'{id}'}</Typography.Text>）；
            协议子集如实标注 capabilities（message/send + tasks/get 轮询；message/stream 与 push 不做）。
          </span>
        }
      />
      <Table
        size="small"
        rowKey="agent_id"
        dataSource={cards}
        pagination={false}
        expandable={{
          rowExpandable: () => true,
          expandedRowRender: (c) => (
            <pre style={{ background: 'var(--c-bg, #fafafa)', border: '1px solid var(--c-line)', borderRadius: 8, padding: 10, fontSize: 11.5, overflow: 'auto', maxHeight: 260 }}>
              {JSON.stringify(c.card, null, 2)}
            </pre>
          ),
        }}
        columns={[
          {
            title: '智能体', dataIndex: 'name', render: (v: string, c) => (
              <span><Typography.Text strong style={{ fontSize: 13 }}>{v}</Typography.Text>{c.is_builtin && <Tag style={{ marginInlineStart: 6 }}>内置</Tag>}</span>
            ),
          },
          { title: '协议端点', key: 'url', render: (_, c) => <Typography.Text code style={{ fontSize: 12 }}>{String(c.card?.url ?? '')}</Typography.Text> },
          { title: 'transport', key: 'tp', width: 110, render: (_, c) => <Tag style={{ margin: 0 }}>{String(c.card?.preferredTransport ?? 'JSONRPC')}</Tag> },
          {
            title: 'capabilities', key: 'cap', width: 200, render: (_, c) => {
              const cap = (c.card?.capabilities ?? {}) as Record<string, unknown>
              return (
                <span>
                  <Tooltip title="message/stream 订阅">stream {cap.streaming ? '✓' : '✗'}</Tooltip>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}> · </Typography.Text>
                  <Tooltip title="pushNotification 推送">push {cap.pushNotifications ? '✓' : '✗'}</Tooltip>
                </span>
              )
            },
          },
          { title: '技能数', key: 'skills', width: 80, render: (_, c) => Array.isArray(c.card?.skills) ? c.card.skills.length : 0 },
        ]}
      />
    </div>
  )

  const taskTab = (
    <div data-testid="protocol-a2a-tasks">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        message="A2A 任务列表：外部客户端经协议委派进来的任务（a2a_task；重启后终态任务仍可查）"
      />
      {tasks.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 A2A 任务——外部客户端 message/send 委派后在此可见（含平台作为 client 外委派的重启恢复查证）" />
      ) : (
        <Table
          size="small"
          rowKey="id"
          dataSource={tasks}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
          expandable={{
            rowExpandable: (t) => !!(t.artifact_text || t.error_text),
            expandedRowRender: (t) => (
              <pre style={{ background: 'var(--c-bg, #fafafa)', border: '1px solid var(--c-line)', borderRadius: 8, padding: 10, fontSize: 11.5, overflow: 'auto', maxHeight: 220, whiteSpace: 'pre-wrap' }}>
                {t.artifact_text || t.error_text}
              </pre>
            ),
          }}
          columns={[
            { title: '任务', dataIndex: 'id', render: (v: string) => <Typography.Text code style={{ fontSize: 11 }}>{v.slice(0, 12)}…</Typography.Text> },
            {
              title: '状态', dataIndex: 'state', width: 110, render: (v: string) => (
                <Tag color={STATE_COLOR[v] ?? 'default'} style={{ margin: 0 }}>{v}</Tag>
              ),
            },
            { title: '目标智能体', dataIndex: 'agent_id', render: (v: string) => agentName(v) },
            { title: 'contextId', dataIndex: 'context_id', render: (v: string) => <Typography.Text code style={{ fontSize: 11 }}>{v ? `${v.slice(0, 12)}…` : '—'}</Typography.Text> },
            { title: '更新时间', dataIndex: 'updated_at', width: 170, render: (v: string) => <Typography.Text type="secondary" style={{ fontSize: 12 }}>{v ? new Date(v).toLocaleString() : '—'}</Typography.Text> },
            { title: '产物', key: 'art', width: 70, render: (_, t) => (t.artifact_text ? <Tag color="success" style={{ margin: 0 }}>有</Tag> : <Typography.Text type="secondary">—</Typography.Text>) },
          ]}
        />
      )}
    </div>
  )

  return (
    <>
      <div className="settings-head">
        <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 4 }}>
          协议（MCP / A2A）
          <Button size="small" type="text" icon={<ReloadOutlined />} onClick={load} style={{ marginInlineStart: 8 }}>刷新</Button>
        </Typography.Title>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          平台双协议总览：对外服务面（MCP server / A2A server）与任务面（A2A 任务）；接入外部 A2A 智能体见「连接器」（kind=A2A）。
        </Typography.Text>
      </div>
      <Tabs
        defaultActiveKey="mcp"
        items={[
          { key: 'mcp', label: 'MCP 服务面', children: mcpTab },
          { key: 'a2a', label: 'A2A 服务面', children: a2aTab },
          { key: 'tasks', label: `A2A 任务${tasks.length ? `（${tasks.length}）` : ''}`, children: taskTab },
        ]}
      />
    </>
  )
}
