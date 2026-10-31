import { useEffect, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { Button, Divider, Form, Input, Popconfirm, Segmented, Select, Tooltip, Typography } from 'antd'
import {
  DoubleLeftOutlined,
  DoubleRightOutlined,
  HistoryOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent, KnowledgeBase, MemoryRec, Notebook, Project, RuntimeProfile, Workspace } from '../api/types'
import { useUI } from '../store/ui'

export type WorkspacePanelView = 'config' | 'projects' | 'memory'

/** 侧边栏小节标题（与 ProjectSidePanel 同语言） */
function Section({ children, first }: { children: React.ReactNode; first?: boolean }) {
  return (
    <Divider titlePlacement="left" plain style={{ margin: first ? '0 0 12px' : '4px 0 12px' }}>
      {children}
    </Divider>
  )
}

// 拖拽宽度记忆（与 ProjectSidePanel REQ-21803 对称；独立 key 防两面板互踩）
const WS_PANEL_WIDTH_KEY = 'eino.wspanel.width'
const WS_PANEL_DEFAULT = 364
const WS_PANEL_MIN = 320
const WS_PANEL_MAX = 720
// activity bar 竖条态宽度（默认收缩为一竖行按钮常驻右侧，REQ-217⑤ 范式）
const PANEL_BAR_WIDTH = 44

/**
 * 工作区右侧侧边栏（REQ-549/M356 M1b；activity bar 范式沿 ProjectSidePanel）：
 * M1b 仅 config 视图（名称/描述/默认知识库/默认本体方案/默认智能体/笔记本绑定只读占位）；
 * 目录绑定与 Git 视图刻意不做——那是项目（执行容器）职责（106 号 §四 IA 边界）。
 * M2 将增 projects 视图（归属项目管理）。
 */
export default function WorkspaceSidePanel({
  workspace,
  agents,
  open,
  view,
  onViewChange,
  onOpenChange,
  onChanged,
}: {
  workspace: Workspace
  agents: Agent[]
  /** 展开态（true=内容面板，false=竖条）；面板常驻挂载 */
  open: boolean
  view: WorkspacePanelView
  onViewChange: (v: WorkspacePanelView) => void
  onOpenChange?: (open: boolean) => void
  onChanged?: () => void
}) {
  const [panelWidth, setPanelWidth] = useState(() => {
    const saved = Number(localStorage.getItem(WS_PANEL_WIDTH_KEY))
    return saved >= WS_PANEL_MIN && saved <= WS_PANEL_MAX ? saved : WS_PANEL_DEFAULT
  })
  const startResize = (e: ReactMouseEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startW = panelWidth
    const onMove = (ev: MouseEvent) => {
      const w = Math.min(WS_PANEL_MAX, Math.max(WS_PANEL_MIN, startW + (startX - ev.clientX)))
      setPanelWidth(w)
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      setPanelWidth((w) => {
        localStorage.setItem(WS_PANEL_WIDTH_KEY, String(w))
        return w
      })
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
  const [collapsed, setCollapsed] = useState(true)
  useEffect(() => {
    setCollapsed(!open)
  }, [open])
  const switchView = (v: WorkspacePanelView) => {
    if (v === view && !collapsed) {
      setCollapsed(true)
      onOpenChange?.(false)
      return
    }
    onViewChange(v)
    if (collapsed) onOpenChange?.(true)
    setCollapsed(false)
  }
  return (
    <aside className="proj-panel" style={{ width: collapsed ? PANEL_BAR_WIDTH : panelWidth }}>
      {!collapsed && (
        <div
          className="proj-panel-resizer"
          role="separator"
          aria-label="拖拽调整侧边栏宽度"
          aria-orientation="vertical"
          onMouseDown={(e) => startResize(e)}
          onDoubleClick={() => { setPanelWidth(WS_PANEL_DEFAULT); localStorage.setItem(WS_PANEL_WIDTH_KEY, String(WS_PANEL_DEFAULT)) }}
        />
      )}
      <div className="proj-panel-bar" role="tablist" aria-label="工作区侧边栏视图">
        <Tooltip title="配置" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'config' && !collapsed ? ' active' : ''}`}
            aria-label="配置"
            aria-selected={view === 'config' && !collapsed}
            role="tab"
            onClick={() => switchView('config')}
          >
            <SettingOutlined />
          </button>
        </Tooltip>
        {/* REQ-550/M357：归属项目管理（M2 projects 视图） */}
        <Tooltip title="归属项目" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'projects' && !collapsed ? ' active' : ''}`}
            aria-label="归属项目"
            aria-selected={view === 'projects' && !collapsed}
            role="tab"
            onClick={() => switchView('projects')}
          >
            <TeamOutlined />
          </button>
        </Tooltip>
        {/* REQ-551/M358：记忆候选确认（三桶列表沿伴生范式；可审计口径） */}
        <Tooltip title="记忆" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'memory' && !collapsed ? ' active' : ''}`}
            aria-label="记忆"
            aria-selected={view === 'memory' && !collapsed}
            role="tab"
            onClick={() => switchView('memory')}
          >
            <HistoryOutlined />
          </button>
        </Tooltip>
        <span className="proj-bar-spacer" />
        <Tooltip title={collapsed ? '展开侧边栏' : '收起为竖条'} placement="left">
          <button
            type="button"
            className="proj-bar-btn"
            aria-label={collapsed ? '展开侧边栏' : '收起为竖条'}
            onClick={() => {
              if (collapsed) {
                onOpenChange?.(true)
                setCollapsed(false)
              } else {
                switchView(view)
              }
            }}
          >
            {collapsed ? <DoubleLeftOutlined /> : <DoubleRightOutlined />}
          </button>
        </Tooltip>
      </div>

      {!collapsed && (
        <div className="proj-panel-view">
          {view === 'config' && <ConfigView workspace={workspace} agents={agents} onChanged={onChanged} />}
          {view === 'projects' && <ProjectsView workspace={workspace} onChanged={onChanged} />}
          {view === 'memory' && <MemoryView workspace={workspace} />}
        </div>
      )}
    </aside>
  )
}

// ---------------------------------------------------------------------------
// 配置视图（REQ-549/M356）：名称/描述/默认知识库/默认本体方案/默认智能体 + 笔记本绑定只读
// ---------------------------------------------------------------------------

function ConfigView({ workspace, agents, onChanged }: { workspace: Workspace; agents: Agent[]; onChanged?: () => void }) {
  const { showToast, bumpData } = useUI()
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  const [notebooks, setNotebooks] = useState<Notebook[]>([])
  const [refsLoading, setRefsLoading] = useState(false)

  useEffect(() => {
    form.setFieldsValue({
      name: workspace.name,
      description: workspace.description,
      default_kb_id: workspace.default_kb_id || undefined,
      default_profile_id: workspace.default_profile_id || undefined,
      default_agent_id: workspace.default_agent_id || undefined,
    })
  }, [workspace.id, form])

  useEffect(() => {
    setRefsLoading(true)
    Promise.all([api.listKBs(), api.listRuntimeProfiles(), api.listNotebooks()])
      .then(([ks, ps, ns]) => {
        setKbs(ks)
        setProfiles(ps)
        setNotebooks(ns)
      })
      .catch(() => {
        setKbs([])
        setProfiles([])
        setNotebooks([])
      })
      .finally(() => setRefsLoading(false))
  }, [])

  const save = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setSaving(true)
    try {
      await api.updateWorkspace(workspace.id, {
        name: v.name,
        description: v.description ?? '',
        default_kb_id: v.default_kb_id ?? '',
        default_profile_id: v.default_profile_id ?? '',
        default_agent_id: v.default_agent_id ?? '',
      })
      showToast('已保存')
      bumpData()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    try {
      await api.deleteWorkspace(workspace.id)
      showToast('已删除（其对话已回落智能体页，消息资产保留）')
      bumpData()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setDeleting(false)
    }
  }

  // 106 号 §四：默认方案只列 running 状态（与本体 chip 同口径）
  const runningProfiles = profiles.filter((p) => p.status === 'running')
  // REQ-495 笔记本绑定只读占位：一对多宿主端=workspace 实体（Q-24 拍板②）
  const boundNotebooks = notebooks.filter((n) => n.workspace_id === workspace.id)

  return (
    <div className="proj-view-body">
      <Form form={form} layout="vertical" requiredMark={false} size="small">
        <Section first>基本信息</Section>
        <Form.Item name="name" label="名称" className="item-inline" rules={[{ required: true, message: '名称必填' }]}>
          <Input />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 5 }} />
        </Form.Item>

        <Section>会话默认值（创建时继承，写时复制）</Section>
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
          新建的对话自动继承以下默认值；对话内仍可随手切换，关闭后不被本工作区复活。
        </Typography.Text>
        <Form.Item name="default_kb_id" label="默认知识库" className="item-inline">
          <Select
            allowClear
            placeholder="未设置——新对话不启用知识召回"
            loading={refsLoading}
            options={kbs.map((k) => ({ value: k.id, label: k.name }))}
          />
        </Form.Item>
        <Form.Item
          name="default_profile_id"
          label="默认本体方案"
          className="item-inline"
          tooltip="仅列出 running 状态的方案（与本体 chip 同口径）；方案失效时新对话静默回落零值"
        >
          <Select
            allowClear
            placeholder="未设置——新对话不启用本体指引"
            loading={refsLoading}
            options={runningProfiles.map((p) => ({ value: p.id, label: p.name }))}
          />
        </Form.Item>
        <Form.Item
          name="default_agent_id"
          label="默认智能体"
          className="item-inline"
          tooltip="工作区直挂对话的运行目标；未设置时回落平台助手（内置）"
        >
          <Select
            allowClear
            placeholder="未设置——回落平台助手"
            options={agents.map((a) => ({ value: a.id, label: a.is_builtin ? `${a.name}（内置）` : a.name }))}
          />
        </Form.Item>

        <Section>笔记本绑定（只读）</Section>
        {boundNotebooks.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
            暂无笔记本——到「知识库」模块笔记本页签创建并绑定本工作区。
          </Typography.Text>
        ) : (
          <ul className="proj-entries" style={{ margin: 0 }}>
            {boundNotebooks.map((n) => (
              <li key={n.id} className="proj-entry">
                <span className="proj-entry-name" title={n.name}>{n.name}</span>
              </li>
            ))}
          </ul>
        )}
      </Form>

      <div className="proj-view-actions">
        <Button type="primary" size="small" loading={saving} onClick={save}>
          保存
        </Button>
        <Popconfirm
          title={`删除工作区「${workspace.name}」？`}
          description="其直挂对话将回落智能体页（消息资产保留）。"
          okText="删除"
          okButtonProps={{ danger: true }}
          cancelText="取消"
          onConfirm={remove}
        >
          <Button danger size="small" loading={deleting}>
            删除工作区
          </Button>
        </Popconfirm>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// 归属项目视图（REQ-550/M357 M2）：归属/解除 + 「应用到现有会话」幂等批量
// ---------------------------------------------------------------------------

function ProjectsView({ workspace, onChanged }: { workspace: Workspace; onChanged?: () => void }) {
  const { showToast, bumpData } = useUI()
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState('')

  const load = () => {
    setLoading(true)
    api.listProjects().then(setProjects).catch(() => setProjects([])).finally(() => setLoading(false))
  }
  useEffect(load, [workspace.id])

  const assign = async (p: Project) => {
    setBusy(p.id)
    try {
      await api.setProjectWorkspace(p.id, workspace.id)
      showToast(`项目「${p.name}」已归属本工作区（新会话起继承默认值）`)
      bumpData()
      onChanged?.()
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy('')
    }
  }
  const unassign = async (p: Project) => {
    setBusy(p.id)
    try {
      await api.setProjectWorkspace(p.id, '')
      showToast(`项目「${p.name}」已解除归属（已建会话不受影响）`)
      bumpData()
      onChanged?.()
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy('')
    }
  }
  // 幂等补齐：updated_at 必 bump——开着的会话窗下一次 patchConv 409 为预期行为（106 号 obj-7，
  // 前端 toast 刷新兜底），提示语如实告知
  const apply = async (p: Project) => {
    setBusy(p.id)
    try {
      const r = await api.applyProjectWorkspace(p.id)
      showToast(r.updated > 0 ? `已为 ${r.updated} 个会话补齐默认值（打开中的会话窗请刷新后继续编辑）` : '所有会话均已是最新，无需变更')
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy('')
    }
  }

  const assignedHere = projects.filter((p) => p.workspace_id === workspace.id)
  const others = projects.filter((p) => p.workspace_id !== workspace.id)

  return (
    <div className="proj-view-body">
      <Section first>已归属项目（{assignedHere.length}）</Section>
      {loading ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载中…</Typography.Text>
      ) : assignedHere.length === 0 ? (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
          暂无归属项目——从下方列表归属；归属后项目新会话自动继承本工作区默认知识库与本体方案。
        </Typography.Text>
      ) : (
        <ul className="proj-entries" style={{ margin: 0 }}>
          {assignedHere.map((p) => (
            <li key={p.id} className="proj-entry">
              <span className="proj-entry-name" title={p.name}>{p.name}</span>
              <span style={{ marginLeft: 'auto', display: 'flex', gap: 4 }}>
                <Tooltip title="把当前工作区默认值补齐到项目既有会话（幂等：只补空缺，不覆盖显式选择；会话更新时间会刷新，打开中的会话窗需刷新后继续编辑）">
                  <Button size="small" type="text" loading={busy === p.id} aria-label={`应用到现有会话 ${p.name}`} onClick={() => apply(p)}>
                    应用到现有会话
                  </Button>
                </Tooltip>
                <Button size="small" type="text" loading={busy === p.id} aria-label={`解除归属 ${p.name}`} onClick={() => unassign(p)}>
                  解除归属
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}

      <Section>未归属项目（{others.length}）</Section>
      {others.length === 0 ? (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>全部项目均已归属。</Typography.Text>
      ) : (
        <ul className="proj-entries" style={{ margin: 0 }}>
          {others.map((p) => (
            <li key={p.id} className="proj-entry">
              <span className="proj-entry-name" title={p.name}>{p.name}</span>
              <Button size="small" type="text" style={{ marginLeft: 'auto' }} loading={busy === p.id} aria-label={`归属 ${p.name}`} onClick={() => assign(p)}>
                归属本工作区
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// 记忆视图（REQ-551/M358）：候选/已入/已否三桶（沿伴生三桶列表范式）+手工录入。
// 可审计口径：每条带来源会话/消息行级溯源（手工=无溯源标注）；无自动入记忆（黑箱记忆不做）。
// ---------------------------------------------------------------------------

const MEMORY_BUCKETS = [
  { value: 'candidate', label: '候选' },
  { value: 'accepted', label: '已入' },
  { value: 'rejected', label: '已否' },
]

function MemoryView({ workspace }: { workspace: Workspace }) {
  const { showToast } = useUI()
  const [bucket, setBucket] = useState<'candidate' | 'accepted' | 'rejected'>('candidate')
  const [items, setItems] = useState<MemoryRec[]>([])
  const [loading, setLoading] = useState(false)
  const [draft, setDraft] = useState('')
  const [adding, setAdding] = useState(false)

  const load = () => {
    setLoading(true)
    api.listMemories(workspace.id).then(setItems).catch(() => setItems([])).finally(() => setLoading(false))
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace.id])

  const rows = items.filter((m) => m.status === bucket)
  const decide = async (m: MemoryRec, accept: boolean) => {
    try {
      await api.decideMemory(m.id, accept)
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }
  const remove = async (m: MemoryRec) => {
    try {
      await api.deleteMemory(m.id)
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }
  const add = async () => {
    const content = draft.trim()
    if (!content) return
    setAdding(true)
    try {
      await api.createMemory(workspace.id, content)
      setDraft('')
      showToast('已记入（手工录入即显式确认）')
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="proj-view-body">
      <Section first>工作区记忆（跨会话，显式确认）</Section>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
        对话收尾自动生成候选，采纳后跨会话注入本工作区对话；全程可审计（黑箱记忆不做）。
      </Typography.Text>
      <Segmented
        size="small"
        value={bucket}
        onChange={(v) => setBucket(v as typeof bucket)}
        options={MEMORY_BUCKETS}
        data-testid="ws-memory-buckets"
      />
      <div style={{ display: 'flex', gap: 6, margin: '10px 0' }}>
        <Input
          size="small"
          placeholder="手工记一条（如：论文截止 3 月）"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onPressEnter={add}
          data-testid="ws-memory-input"
        />
        <Button size="small" type="primary" loading={adding} onClick={add} data-testid="ws-memory-add">
          记入
        </Button>
      </div>
      {loading ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载中…</Typography.Text>
      ) : rows.length === 0 ? (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
          {bucket === 'candidate' ? '暂无候选——对话收尾自动生成，或手工记入。' : bucket === 'accepted' ? '暂无已入记忆。' : '暂无已否记录。'}
        </Typography.Text>
      ) : (
        <ul className="proj-entries" style={{ margin: 0, flexDirection: 'column' }}>
          {rows.map((m) => (
            <li key={m.id} className="proj-entry" style={{ alignItems: 'flex-start', flexDirection: 'column', gap: 4 }}>
              <span style={{ fontSize: 12.5, lineHeight: 1.6 }}>{m.content}</span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 4, width: '100%' }}>
                <span style={{ fontSize: 11, color: 'var(--c-ink-3)' }}>
                  {m.source_conversation_id ? `溯源 · 会话 ${m.source_conversation_id.slice(0, 8)}` : '手工录入'}
                </span>
                <span style={{ marginLeft: 'auto', display: 'flex', gap: 2 }}>
                  {m.status === 'candidate' && (
                    <>
                      <Button size="small" type="text" aria-label="采纳" onClick={() => decide(m, true)}>采纳</Button>
                      <Button size="small" type="text" aria-label="拒绝" onClick={() => decide(m, false)}>拒绝</Button>
                    </>
                  )}
                  <Popconfirm title="删除这条记忆？" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(m)}>
                    <Button size="small" type="text" danger aria-label="删除">删除</Button>
                  </Popconfirm>
                </span>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
