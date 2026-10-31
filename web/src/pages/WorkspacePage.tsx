import { useEffect, useMemo, useState } from 'react'
import { Button, Form, Input, Modal, Splitter, Typography } from 'antd'
import { api } from '../api/client'
import type { Agent, Conversation, Workspace } from '../api/types'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import Sidebar from '../components/Sidebar'
import ChatWindow from '../components/ChatWindow'
import WorkspaceSidePanel, { type WorkspacePanelView } from '../components/WorkspaceSidePanel'

/**
 * 工作区页（REQ-549/M356 M1b；Q-24 分层批②，详设 platform-knowledge/02_智能体/106 号 §四/§五）：
 * 与智能体页/项目页同构——左栏折叠树（工作区节点→直挂对话子树）+ 右侧 ChatWindow + 工作区侧板。
 * 双入口共存口径：工作区对话同现智能体页对应智能体节点（行加「·工作区」标注），两处删除一致。
 * 归位守卫：选中对话须 scope=agent 且归属当前工作区，不匹配归位空态（AgentsPage 同款）。
 */
export default function WorkspacePage() {
  const { currentConvId, setCurrentConv, dataVersion, bumpData, showToast } = useUI()
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [convs, setConvs] = useState<Conversation[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  // 工作区侧板（REQ-549：activity bar 范式沿 ProjectSidePanel；M1b 仅 config 视图）
  const [sidePanelOpen, setSidePanelOpen] = useState(false)
  const [panelView, setPanelView] = useState<WorkspacePanelView>('config')

  const reload = () => {
    api.listWorkspaces().then((ws) => {
      setWorkspaces(ws)
      setLoadErr(null)
      setActiveWorkspaceId((cur) => (cur && ws.some((w) => w.id === cur) ? cur : (ws[0]?.id ?? null)))
    }).catch((e) => {
      setWorkspaces([])
      setLoadErr(e?.message ?? '工作区列表加载失败')
    })
    api.listAgents().then(setAgents).catch(() => setAgents([]))
    api.listConversations().then(setConvs).catch(() => setConvs([]))
  }

  useEffect(reload, [dataVersion])

  const activeWorkspace = workspaces.find((w) => w.id === activeWorkspaceId) ?? null
  const currentConv = useMemo(() => convs.find((c) => c.id === currentConvId) ?? null, [convs, currentConvId])

  // 归位守卫：非 agent 会话或归属其他工作区的会话，归位到空态（ProjectsPage/AgentsPage 同款）
  useEffect(() => {
    if (currentConv && !(currentConv.scope === 'agent' && (currentConv.workspace_id ?? '') === activeWorkspaceId)) {
      setCurrentConv(null)
    }
  }, [activeWorkspaceId, currentConv])

  // 工作区直挂新对话：scope=agent + workspace_id（后端创建时继承默认库/方案，106 号 §三）；
  // 运行目标=工作区默认智能体，未设回落内置平台助手，再兜底首个智能体。
  const newConversation = async (wsId: string) => {
    const ws = workspaces.find((w) => w.id === wsId)
    const agentId = ws?.default_agent_id || agents.find((a) => a.is_builtin)?.id || agents[0]?.id || ''
    if (!agentId) {
      showToast('暂无可用智能体：请先到「智能体」页创建，或为工作区设置默认智能体', 'err')
      return
    }
    try {
      const c = await api.createConversation({ scope: 'agent', agent_id: agentId, workspace_id: wsId, title: '新对话' })
      bumpData()
      setActiveWorkspaceId(wsId)
      setCurrentConv(c.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  /** 工作区节点 ⚙ → 右侧边栏配置视图（侧边栏若未开则同时打开） */
  const configureWorkspace = (wsId: string) => {
    setActiveWorkspaceId(wsId)
    setSidePanelOpen(true)
    setPanelView('config')
  }

  const onWorkspaceCreated = (id: string) => {
    setCreateOpen(false)
    reload()
    setActiveWorkspaceId(id)
    setSidePanelOpen(true)
    setPanelView('config')
    showToast('工作区已创建，请在侧边栏设置会话默认值')
  }

  return (
    <Splitter
      className="main sidebar-splitter"
      onResizeEnd={sidebarRemember}
    >
      <Splitter.Panel defaultSize={sidebarDefaultSize()} min={SIDEBAR_WIDTH.min} max={SIDEBAR_WIDTH.max} className="sidebar-panel">
        <Sidebar
          mode="workspace"
          agents={agents}
          projects={[]}
          workspaces={workspaces}
          conversations={convs}
          activeAgentId={null}
          activeProjectId={null}
          activeWorkspaceId={activeWorkspaceId}
          onSelectAgent={() => {}}
          onSelectProject={() => {}}
          onSelectWorkspace={setActiveWorkspaceId}
          onNewAgent={() => {}}
          onNewProject={() => {}}
          onNewWorkspace={() => setCreateOpen(true)}
          onNewConversation={newConversation}
          onConfigureAgent={() => {}}
          onConfigureProject={() => {}}
          onConfigureWorkspace={configureWorkspace}
          loadErr={loadErr}
          onRetry={reload}
        />
      </Splitter.Panel>
      <Splitter.Panel className="content-panel">
        <div className="proj-content-row">
          <div className="proj-content-main">
            {currentConv && currentConv.scope === 'agent' ? (
              <ChatWindow
                conversation={currentConv}
                agents={agents}
                projects={[]}
                onConversationUpdated={reload}
                onToggleSidePanel={() => setSidePanelOpen((o) => !o)}
              />
            ) : (
              // REQ-488 空态灰字语言：单列一行 + 显式新建钮（选中工作区时）
              <div className="page" style={{ padding: '48px 40px' }}>
                <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="workspaces-empty-quiet">
                  在左侧选择工作区，与其开始对话；新对话自动继承工作区默认知识库与本体方案。
                </Typography.Text>
                {activeWorkspace && (
                  <div style={{ marginTop: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
                    <Button type="primary" size="small" onClick={() => newConversation(activeWorkspace.id)}>
                      ＋ 新建对话
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* 侧板常驻挂载（REQ-237 范式）——竖条随选中工作区常驻，open 仅控制展开/收缩 */}
          {activeWorkspace && (
            <WorkspaceSidePanel
              workspace={activeWorkspace}
              agents={agents}
              open={sidePanelOpen}
              view={panelView}
              onViewChange={setPanelView}
              onOpenChange={setSidePanelOpen}
              onChanged={reload}
            />
          )}
        </div>

        {createOpen && <WorkspaceModal onClose={() => setCreateOpen(false)} onCreated={onWorkspaceCreated} />}
      </Splitter.Panel>
    </Splitter>
  )
}

/** 新建工作区弹窗（M1b 最小件：名称+描述；默认值创建后经侧板配置视图完善） */
function WorkspaceModal({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const submit = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setSaving(true)
    try {
      const ws = await api.createWorkspace({ name: v.name, description: v.description ?? '' })
      onCreated(ws.id)
    } catch (e: any) {
      showToast(e.message, 'err')
      setSaving(false)
    }
  }
  return (
    <Modal
      title="新建工作区"
      open
      onCancel={onClose}
      onOk={submit}
      okText="创建"
      confirmLoading={saving}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]}>
          <Input placeholder="如：论文写作、合规审查" />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="一句话描述这个领域场景（可选）" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
