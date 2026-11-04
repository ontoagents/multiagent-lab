import { useEffect, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { Button, Divider, Form, Input, Popconfirm, Segmented, Select, Tooltip, Typography, Modal, Space } from 'antd'
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
import RelationConsequenceBlock from './RelationConsequenceBlock'

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
 * config 视图（名称/描述/默认知识库/默认本体方案/默认智能体/笔记本绑定）；
 * REQ-495②段尾项：笔记本绑定自只读占位转正为绑定/换绑/解绑交互（PUT /api/notebooks 全量语义，
 * 解绑空串后端归一 NULL）——「存笔记本」产物去向动作（ProjectSidePanel FilesView）按此绑定解析目标。
 * 目录绑定与 Git 视图刻意不做——那是项目（执行容器）职责（106 号 §四 IA 边界）。
 * M2 projects 视图已交付（REQ-550/M357 归属项目管理）。
 */
let wsMemoryRefreshBound = false

export default function WorkspaceSidePanel({
  workspace,
  agents,
  open,
  view,
  onViewChange,
  onOpenChange,
  onChanged,
  onOpenProject,
}: {
  workspace: Workspace
  agents: Agent[]
  /** 展开态（true=内容面板，false=竖条）；面板常驻挂载 */
  open: boolean
  view: WorkspacePanelView
  onViewChange: (v: WorkspacePanelView) => void
  onOpenChange?: (open: boolean) => void
  onChanged?: () => void
  /** REQ-616-aa64/M424-aa64 C4（121 号 R3）：归属项目行「打开」→ 项目模块一步直达（工作区→项目
   *  反向互链——总览名下项目卡同协议；管理动作仍归本视图） */
  onOpenProject?: (pid: string, pname: string) => void
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
    // REQ-579/M394：记忆候选数行动信号（run.finished 后经 ws-memory-refresh 事件触发拉取；采纳后消减）
  const [memCandidates, setMemCandidates] = useState(0)
  const refreshMemCandidates = () => {
    api.listMemories(workspace.id, 'candidate').then((r) => setMemCandidates((r ?? []).length)).catch(() => {})
  }
  useEffect(() => {
    refreshMemCandidates()
    if (!wsMemoryRefreshBound) {
      wsMemoryRefreshBound = true
      window.addEventListener('ws-memory-refresh', refreshMemCandidates as EventListener)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace.id])
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
        <Tooltip title={`记忆${memCandidates > 0 ? `（${memCandidates} 条候选待确认）` : ''}`} placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'memory' && !collapsed ? ' active' : ''}`}
            aria-label={`记忆${memCandidates > 0 ? `（${memCandidates} 条候选待确认）` : ''}`}
            aria-selected={view === 'memory' && !collapsed}
            role="tab"
            onClick={() => switchView('memory')}
            style={{ position: 'relative' }}
            data-testid="ws-memory-btn"
          >
            <HistoryOutlined />
            {/* REQ-579/M394：候选数行动信号（run.finished 后拉取 REST 计数，不依赖实时事件；采纳后消减） */}
            {memCandidates > 0 && (
              <span
                style={{ position: 'absolute', top: 2, right: 2, minWidth: 14, height: 14, borderRadius: 7, background: '#faad14', color: '#fff', fontSize: 10, lineHeight: '14px', textAlign: 'center', padding: '0 3px' }}
                data-testid="ws-memory-badge"
              >
                {memCandidates > 99 ? '99+' : memCandidates}
              </span>
            )}
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
          {view === 'projects' && <ProjectsView workspace={workspace} onChanged={onChanged} onOpenProject={onOpenProject} />}
          {view === 'memory' && <MemoryView workspace={workspace} />}
        </div>
      )}
    </aside>
  )
}

// ---------------------------------------------------------------------------
// 配置视图（REQ-549/M356）：名称/描述/默认知识库/默认本体方案/默认智能体 + 笔记本绑定
// （REQ-495②段尾项转正：绑定/换绑/解绑交互并持久化）
// ---------------------------------------------------------------------------

function ConfigView({ workspace, agents, onChanged }: { workspace: Workspace; agents: Agent[]; onChanged?: () => void }) {
  const { showToast, bumpData, setPage } = useUI()
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  const [notebooks, setNotebooks] = useState<Notebook[]>([])
  const [refsLoading, setRefsLoading] = useState(false)
  const [bindSel, setBindSel] = useState('') // 待绑定笔记本选择

  useEffect(() => {
    form.setFieldsValue({
      name: workspace.name,
      description: workspace.description,
      default_kb_id: workspace.default_kb_id || undefined,
      default_profile_id: workspace.default_profile_id || undefined,
      default_agent_id: workspace.default_agent_id || undefined,
    })
  }, [workspace.id, form])

  // 笔记本清单独立可重载（绑定/解绑后刷新本区块；REQ-495②段尾项）
  const loadNotebooks = () => {
    api.listNotebooks().then(setNotebooks).catch(() => setNotebooks([]))
  }

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
      bumpData()
      onChanged?.()
      // REQ-583/M398：作用范围提示+显式追赶（「改默认不自动追溯已建会话」拍板不反转——追赶为用户显式点击）
      try {
        const { total, stale } = await api.workspaceDirectDefaultsCount(workspace.id)
        if (stale > 0) {
          Modal.confirm({
            title: `区内 ${total} 个直挂会话中 ${stale} 个仍为创建时默认值`,
            content: '把当前默认知识库/本体方案应用到这些会话？（只填充仍为空的项——显式关闭/改选过的会话不被触碰）',
            okText: `应用到 ${stale} 个会话`,
            cancelText: '暂不',
            onOk: async () => {
              const r = await api.applyWorkspaceDefaultsDirect(workspace.id)
              showToast(`已应用 ${r.updated} 个会话`)
              bumpData()
            },
          })
        } else {
          showToast('已保存（区内直挂会话均已继承或显式设置，无需追赶）')
        }
      } catch {
        showToast('已保存')
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  // REQ-584/M399：删除前实时枚举名下资产（直挂对话/已确认记忆/候选/归属项目/笔记本绑定——五项 count）；
  // 删除完成 toast 升级「查看回落对话」直达（eino.focus.conv+eino:goto.agents——AgentsPage 选中回落会话）
  const [delModalOpen, setDelModalOpen] = useState(false)
  const [delStats, setDelStats] = useState<{ convs: number; accepted: number; candidates: number; projects: number; notebooks: number } | null>(null)
  const [delStatsLoading, setDelStatsLoading] = useState(false)
  const [fallbackConvId, setFallbackConvId] = useState('')

  const openDeleteModal = async () => {
    setDelModalOpen(true)
    setDelStatsLoading(true)
    try {
      const convs = await api.listConversations({ workspace_id: workspace.id }).catch(() => [])
      const mems = await api.listMemories(workspace.id).catch(() => [])
      const projects = await api.listProjects({ workspace_id: workspace.id }).catch(() => [])
      const nbs = await api.listNotebooks(workspace.id).catch(() => [])
      setFallbackConvId(convs[0]?.id ?? '')
      setDelStats({
        convs: convs.length,
        accepted: mems.filter((m) => m.status === 'accepted').length,
        candidates: mems.filter((m) => m.status === 'candidate').length,
        projects: projects.length,
        notebooks: nbs.length,
      })
    } catch {
      setDelStats(null)
    } finally {
      setDelStatsLoading(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    try {
      await api.deleteWorkspace(workspace.id)
      // REQ-581/M396：结果 toast 补记忆面后果（CASCADE 语义如实透出）+REQ-584「查看回落对话」直达
      showToast('已删除（其对话已回落智能体页，消息资产保留；该工作区的记忆与候选条目已一并删除）')
      bumpData()
      onChanged?.()
      setDelModalOpen(false)
      if (fallbackConvId) {
        // 直达：置 focus.conv 并切导航到智能体页（AgentsPage 挂载后 convs 载入 effect 选中回落会话；
        // 事件派发在跨页场景收不到——AgentsPage 此刻未挂载）
        try {
          sessionStorage.setItem('eino.focus.conv', fallbackConvId)
          setPage('agents')
        } catch { /* 静默 */ }
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setDeleting(false)
    }
  }

  // 106 号 §四：默认方案只列 running 状态（与本体 chip 同口径）
  const runningProfiles = profiles.filter((p) => p.status === 'running')
  // REQ-495②段尾项：绑定/换绑/解绑交互（一对多宿主端=workspace 实体，Q-24 拍板②；
  // 写链=PUT /api/notebooks/{id} 全量语义，解绑空串由后端归一 NULL）
  const boundNotebooks = notebooks.filter((n) => n.workspace_id === workspace.id)
  const availableNotebooks = notebooks.filter((n) => (n.workspace_id ?? '') !== workspace.id)

  const bindNotebook = async (nbId: string) => {
    const nb = notebooks.find((x) => x.id === nbId)
    if (!nb) return
    try {
      await api.updateNotebook(nbId, { name: nb.name, description: nb.description, workspace_id: workspace.id })
      showToast(`笔记本「${nb.name}」已绑定本工作区——项目会话产物面板的「存笔记本」动作将写入这里（REQ-586 承接直挂对话产物）`)
      setBindSel('')
      loadNotebooks()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }
  const unbindNotebook = async (nb: Notebook) => {
    try {
      await api.updateNotebook(nb.id, { name: nb.name, description: nb.description, workspace_id: '' })
      showToast(`笔记本「${nb.name}」已解除绑定（页面保留，回落个人资产）`)
      loadNotebooks()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  return (
    <div className="proj-view-body">
      {/* REQ-578/M393：实物锚+白话区别句（配置视图顶部——「工作区 vs 项目」一句话） */}
      <div style={{ marginBottom: 10 }} data-testid="ws-anchor">
        <div style={{ fontSize: 13, fontWeight: 600 }}>⌂ 工作区《{workspace.name}》</div>
        <Typography.Text type="secondary" style={{ fontSize: 11.5, lineHeight: 1.6 }}>
          工作区管「新对话自带什么知识背景」；项目管「产物写到哪」——两者互不替代。
        </Typography.Text>
      </div>
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

        <Section>笔记本绑定</Section>
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
          项目会话产物面板的「存笔记本」动作写入这里绑定的笔记本（一对多；自产成果资产层 REQ-495；直挂对话产物承接=REQ-586）。
        </Typography.Text>
        {boundNotebooks.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }} data-testid="ws-bind-empty">
            暂无绑定——从下方选择笔记本绑定，或先到「知识库」模块笔记本页签创建。
          </Typography.Text>
        ) : (
          <ul className="proj-entries" style={{ margin: 0 }} data-testid="ws-bound-notebooks">
            {boundNotebooks.map((n) => (
              <li key={n.id} className="proj-entry">
                <span className="proj-entry-name" title={n.name}>{n.name}</span>
                <Button size="small" type="text" aria-label={`解绑 ${n.name}`} data-testid={`ws-unbind-${n.name}`} onClick={() => unbindNotebook(n)}>
                  解绑
                </Button>
              </li>
            ))}
          </ul>
        )}
        <div style={{ display: 'flex', gap: 6, marginTop: 8 }} data-testid="ws-bind-row">
          <Select
            size="small"
            style={{ flex: 1 }}
            allowClear
            placeholder={availableNotebooks.length === 0 ? '暂无可绑定的笔记本' : '选择要绑定的笔记本…'}
            loading={refsLoading}
            value={bindSel || undefined}
            onChange={(v) => setBindSel(v ?? '')}
            options={availableNotebooks.map((n) => ({ value: n.id, label: n.workspace_id ? `${n.name}（已绑其他工作区）` : n.name }))}
            data-testid="ws-bind-select"
          />
          <Button
            size="small"
            type="primary"
            disabled={!bindSel}
            aria-label="绑定笔记本"
            data-testid="ws-bind-btn"
            onClick={() => bindNotebook(bindSel)}
          >
            绑定
          </Button>
        </div>
      </Form>

      <div className="proj-view-actions">
        <Button type="primary" size="small" loading={saving} onClick={save}>
          保存
        </Button>
        {/* REQ-584/M399：删除前实时枚举名下资产 Modal（沿 REQ-507 两段流先例；CASCADE/SET NULL 语义零改动仅枚举不拦截） */}
        <Button danger size="small" loading={deleting} onClick={openDeleteModal} data-testid="ws-del-open">
          删除工作区
        </Button>
        <Modal
          open={delModalOpen}
          onCancel={() => setDelModalOpen(false)}
          title={`删除工作区「${workspace.name}」？`}
          okText="删除"
          okButtonProps={{ danger: true, loading: deleting, disabled: delStatsLoading }}
          cancelText="取消"
          width={460}
        >
          {/* REQ-615-721e/M423-721e C6（121 号 §4.2-C6/§4.4）：顶部「关系与后果」对照块——
              删工作区=容器脱钩唯记忆 CASCADE，与删项目（内容级联硬删）并置对照（文案单源=共享组件） */}
          <RelationConsequenceBlock kind="workspace" />
          {delStatsLoading || !delStats ? (
            <Typography.Text type="secondary" style={{ fontSize: 12.5 }}>正在统计名下资产…</Typography.Text>
          ) : (
            <div data-testid="ws-del-stats">
              <Typography.Text type="secondary" style={{ fontSize: 12.5, display: 'block', marginBottom: 8, lineHeight: 1.7 }}>
                删除后其直挂对话将回落智能体页（消息资产保留）；该工作区已确认的记忆与候选条目将一并删除（CASCADE）。名下资产：
              </Typography.Text>
              <ul style={{ fontSize: 12.5, lineHeight: 2, margin: '0 0 8px', paddingLeft: 18 }}>
                <li>直挂对话：{delStats.convs} 个（回落保留）</li>
                <li>已确认记忆：{delStats.accepted} 条（一并删除）</li>
                <li>记忆候选：{delStats.candidates} 条（一并删除）</li>
                <li>归属项目：{delStats.projects} 个（解除归属保留）</li>
                <li>笔记本绑定：{delStats.notebooks} 个（随笔记本保留、解绑）</li>
              </ul>
              <Space style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button size="small" onClick={() => setDelModalOpen(false)}>取消</Button>
                <Button danger size="small" loading={deleting} onClick={remove} disabled={delStatsLoading}>
                  {delStats.convs + delStats.accepted + delStats.candidates + delStats.projects + delStats.notebooks > 0 ? '确认删除' : '确认删除（空工作区）'}
                </Button>
              </Space>
            </div>
          )}
        </Modal>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// 归属项目视图（REQ-550/M357 M2）：归属/解除 + 「应用到现有会话」幂等批量
// ---------------------------------------------------------------------------

function ProjectsView({ workspace, onChanged, onOpenProject }: { workspace: Workspace; onChanged?: () => void; onOpenProject?: (pid: string, pname: string) => void }) {
  const { showToast, bumpData } = useUI()
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState('')

  const load = () => {
    setLoading(true)
    // REQ-587/M402：服务端过滤接线（listProjects({workspace_id})——客户端 filter 退化兜底）
    api
      .listProjects({ workspace_id: workspace.id })
      .then((ps) => setProjects(ps.filter((p) => p.workspace_id === workspace.id)))
      .catch(() => setProjects([]))
      .finally(() => setLoading(false))
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
      // REQ-581/M396：补记忆面后果（库/方案默认值保持不变——写时复制已固化；记忆注入自下轮起停）
      showToast(`项目「${p.name}」已解除归属：已建会话的库/方案默认值保持不变；跨会话记忆自下轮起不再注入`)
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
                {/* REQ-616-aa64/M424-aa64 C4（121 号 R3）：归属项目行「打开」→ 项目模块一步直达
                    （工作区→项目反向互链；管理动作「应用到现有会话/解除归属」仍归本视图） */}
                {onOpenProject && (
                  <Tooltip title={`到项目模块打开「${p.name}」`}>
                    <Button size="small" type="text" aria-label={`打开项目 ${p.name}`} data-testid={`ws-side-open-proj-${p.id}`} onClick={() => onOpenProject(p.id, p.name)}>
                      打开
                    </Button>
                  </Tooltip>
                )}
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
