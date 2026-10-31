import { useEffect, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { Button, Divider, Form, Input, Popconfirm, Select, Tooltip, Typography } from 'antd'
import {
  DoubleLeftOutlined,
  DoubleRightOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent, KnowledgeBase, Notebook, RuntimeProfile, Workspace } from '../api/types'
import { useUI } from '../store/ui'

export type WorkspacePanelView = 'config'

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
