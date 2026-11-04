import { useEffect, useMemo, useRef, useState } from 'react'
import { Breadcrumb, Button, Empty, Form, Input, Modal, Segmented, Select, Splitter, Tooltip, Typography } from 'antd'
import { BookOutlined, HistoryOutlined, ProjectOutlined, SettingOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent, Conversation, KnowledgeBase, Project, RuntimeProfile, Workspace } from '../api/types'
// REQ-590/M431：场景模板单源（seeds/workspace-templates——?raw 编译期内联，沿 REQ-505 单源先例；改 seeds 文件即改向导模板）
import WS_TEMPLATES_RAW from '../../../seeds/workspace-templates/templates.json?raw'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import { pushRecent } from '../lib/recent' // REQ-616-aa64/M424-aa64 A7（121 号 R3）
import Sidebar from '../components/Sidebar'
import ChatWindow from '../components/ChatWindow'
import WorkspaceSidePanel, { type WorkspacePanelView } from '../components/WorkspaceSidePanel'

/**
 * 工作区专页（REQ-549/M356 M1b 立页；REQ-613-b6cc/M421-b6cc 起为「工作区」独立模块页，121 号
 * R1① 路由 /workspaces；REQ-614-9fb4/M422-9fb4 R1② 页签退役后为唯一宿主）：左栏折叠树（工作区
 * 容器节点[⌂+计数徽标]→直挂对话子树；空壳节点不渲染 chevron）+ 右侧 ChatWindow / 工作区总览
 * （配置/笔记本绑定/记忆入口+名下项目卡列）+ 工作区侧板。
 * 双入口共存口径：工作区对话同现智能体页对应智能体节点（行加「·工作区」标注），两处删除一致。
 * 归位守卫：选中对话须 scope=agent 且归属当前工作区，不匹配归位空态（AgentsPage 同款）。
 * 深链（121 号 §5.2 重排后）：eino.focus.workspace→本模块（挂载 IIFE 消费+eino:goto.workspace
 * 响应式监听双入口）；eino.focus.conv（直挂会话）→本模块打开该会话（分流基座 lib/deepLink）。
 */
export default function WorkspacePage() {
  const { currentConvId, setCurrentConv, dataVersion, bumpData, showToast, setPage } = useUI()
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [convs, setConvs] = useState<Conversation[]>([])
  // REQ-613-b6cc/M421-b6cc 名下项目卡列数据源（121 号 R1① C4 前半）；
  // REQ-587 语义保留——工作区模块载荷按当前工作区过滤（listProjects({workspace_id})）
  const [wsProjects, setWsProjects] = useState<Project[]>([])
  // REQ-616-aa64/M424-aa64 D1（121 号 R3）：全量项目清单——内容级 FTS 命中条目「来源」标注
  // 解析项目名（工作区载荷不含其他项目；仅标注消费，不进树）
  const [allProjects, setAllProjects] = useState<Project[]>([])
  // REQ-495②段尾项：「存笔记本」未绑定引导「前往绑定」聚焦——挂载一次性读取（命中即选中工作区+
  // 直开侧板配置=绑定入口）。REQ-614-9fb4 R1② 补响应式监听（121 号 §5.2 gap-5 教训：页签退役后
  // WorkspacePage 为常驻挂载模块页——用户已在工作区模块时深链派发事件，IIFE 不再跑，由监听承接）。
  const focusWs = (() => {
    try {
      const id = sessionStorage.getItem('eino.focus.workspace') || ''
      if (id) sessionStorage.removeItem('eino.focus.workspace')
      return id
    } catch {
      return ''
    }
  })()
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(focusWs || null)
  useEffect(() => {
    if (focusWs) pushRecent('workspace', focusWs) // A7：挂载聚焦亦入最近记录
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  useEffect(() => {
    const gotoWorkspace = () => {
      try {
        const id = sessionStorage.getItem('eino.focus.workspace')
        if (!id) return
        sessionStorage.removeItem('eino.focus.workspace')
        setActiveWorkspaceId(id)
        pushRecent('workspace', id) // A7：深链到达亦入最近记录
        setSidePanelOpen(true)
        setPanelView('config')
      } catch { /* 静默 */ }
    }
    window.addEventListener('eino:goto.workspace', gotoWorkspace)
    return () => window.removeEventListener('eino:goto.workspace', gotoWorkspace)
  }, [])
  // REQ-614-9fb4 R1②：focus.conv 分流到达侧（直挂会话→本模块打开；lib/deepLink 写入，消费协议沿
  // AgentsPage REQ-584 先例——载荷命中才消费删键，未命中保留给智能体页等其他模块回落）。
  // 两段式同 ProjectsPage：先提交工作区选中，归属匹配后的下一个 effect 再 setCurrentConv
  //（防 zustand 同步更新与 useState 批处理的提交错位被归位守卫清空）。
  const pendingConvRef = useRef<string | null>(null)
  useEffect(() => {
    if (convs.length === 0) return
    try {
      const convId = sessionStorage.getItem('eino.focus.conv')
      if (!convId) return
      const conv = convs.find((c) => c.id === convId)
      if (conv && conv.scope === 'agent') {
        sessionStorage.removeItem('eino.focus.conv')
        if (conv.workspace_id) {
          setActiveWorkspaceId(conv.workspace_id)
          pushRecent('workspace', conv.workspace_id) // A7：深链到达亦入最近记录
        }
        pendingConvRef.current = conv.id
      }
    } catch { /* 静默 */ }
  }, [convs])
  useEffect(() => {
    const pend = pendingConvRef.current
    if (!pend) return
    const conv = convs.find((c) => c.id === pend)
    if (conv && (conv.workspace_id ?? '') === activeWorkspaceId) {
      pendingConvRef.current = null
      setCurrentConv(conv.id)
    }
  }, [activeWorkspaceId, convs, setCurrentConv])
  const [createOpen, setCreateOpen] = useState(false)
  // 工作区侧板（REQ-549：activity bar 范式沿 ProjectSidePanel；M1b 仅 config 视图）
  // REQ-578/M393：展开态 localStorage 持久化（eino.workspace.panel.open——REQ-571 rail 先例）
  const [sidePanelOpen, setSidePanelOpenState] = useState<boolean>(() => {
    try {
      return sessionStorage.getItem('eino.workspace.panel.open') === '1' || !!focusWs
    } catch {
      return !!focusWs
    }
  })
  const setSidePanelOpen = (o: boolean) => {
    try {
      if (o) sessionStorage.setItem('eino.workspace.panel.open', '1')
      else sessionStorage.removeItem('eino.workspace.panel.open')
    } catch {}
    setSidePanelOpenState(o)
  }
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
    // REQ-587/M402：服务端过滤接线（listConversations({workspace_id})——工作区页载荷仅含当前
    // 工作区数据；activeWorkspaceId 变化时重拉；空=全量（归位守卫语义保留））
    api.listConversations(activeWorkspaceId ? { workspace_id: activeWorkspaceId } : {})
      .then(setConvs)
      .catch(() => setConvs([]))
    // REQ-613-b6cc/M421-b6cc：名下项目同按当前工作区过滤（REQ-587 语义；空=全量兜底）
    api.listProjects(activeWorkspaceId ? { workspace_id: activeWorkspaceId } : {})
      .then((ps) => setWsProjects(ps.filter((p) => (p.workspace_id ?? '') === (activeWorkspaceId ?? ''))))
      .catch(() => setWsProjects([]))
    // REQ-616-aa64/M424-aa64 D1（121 号 R3）：全量项目清单（FTS 命中「来源」标注解析项目名——
    // 工作区过滤载荷不含其他项目；仅标注消费不进树；失败静默=标注回落 id）
    api.listProjects().then(setAllProjects).catch(() => setAllProjects([]))
  }

  useEffect(reload, [dataVersion, activeWorkspaceId])

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

  /** REQ-614-9fb4 R1② 总览名下项目卡 → 项目模块一步直达（121 号 §5.2 focus.project 监听原位回归，
   * 替换 R1① 过渡态两步动线）：写 eino.focus.project（ProjectsBody 挂载消费=选中+开侧板 config 视图；
   * REQ-580 同协议）+ eino.project.active 预置（REQ-536 现场恢复兜底）+ 派发 eino:goto.project
   * （同模块常驻场景由响应式监听承接——页签退役后项目模块为常驻挂载，协议对称保留）。 */
  const openProjectInProjectsModule = (pid: string, pname: string) => {
    try {
      sessionStorage.setItem('eino.project.active', pid)
      sessionStorage.setItem('eino.focus.project', pid)
    } catch { /* 配额满静默（沿 REQ-509 口径） */ }
    pushRecent('project', pid) // A7：互链直达亦入项目最近记录
    setPage('projects')
    window.dispatchEvent(new CustomEvent('eino:goto.project'))
    showToast(`已在项目模块打开「${pname}」（侧板配置视图）`)
  }

  const onWorkspaceCreated = (id: string) => {
    setCreateOpen(false)
    reload()
    setActiveWorkspaceId(id)
    pushRecent('workspace', id) // A7：新建即入最近记录
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
          projects={allProjects}
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
            {/* REQ-578/M393：常驻当前工作区名头部 → REQ-616-aa64/M424-aa64 A5（121 号 R3）升级为
                模块内独立面包屑「工作区 › 工作区 A › 直挂对话」（VS Code Profiles 语义保留） */}
            {activeWorkspace && (
              <div style={{ padding: '8px 16px 0' }} data-testid="ws-head">
                <Breadcrumb
                  data-testid="ws-breadcrumb"
                  items={[
                    { title: <Typography.Text style={{ fontSize: 12 }}>⌂ 工作区</Typography.Text> },
                    { title: <Typography.Text type="secondary" style={{ fontSize: 12 }}>{activeWorkspace.name}</Typography.Text> },
                    ...(currentConv && currentConv.scope === 'agent'
                      ? [{ title: <Typography.Text type="secondary" style={{ fontSize: 12 }}>{currentConv.title || '未命名对话'}</Typography.Text> }]
                      : []),
                  ]}
                />
              </div>
            )}
            {currentConv && currentConv.scope === 'agent' ? (
              <ChatWindow
                conversation={currentConv}
                agents={agents}
                projects={[]}
                workspaces={workspaces}
                onConversationUpdated={reload}
                onToggleSidePanel={() => setSidePanelOpen(!sidePanelOpen)}
                onFocusWorkspaceMemory={() => { setSidePanelOpen(true); setPanelView('memory') }}
              />
            ) : (
              // REQ-488 空态灰字语言保留为未选中兜底；选中工作区时升级为工作区总览
              //（REQ-613-b6cc/M421-b6cc 121 号 R1①：配置/笔记本绑定/记忆入口+名下项目卡列）
              <div className="page" style={{ padding: '28px 36px', overflowY: 'auto' }}>
                {activeWorkspace ? (
                  <WorkspaceOverview
                    ws={activeWorkspace}
                    projects={wsProjects}
                    onConfigure={() => { setSidePanelOpen(true); setPanelView('config') }}
                    onNotebooks={() => { setSidePanelOpen(true); setPanelView('config') }}
                    onMemory={() => { setSidePanelOpen(true); setPanelView('memory') }}
                    onNewConversation={() => newConversation(activeWorkspace.id)}
                    onOpenProject={openProjectInProjectsModule}
                  />
                ) : (
                  <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="workspaces-empty-quiet">
                    在左侧选择工作区，与其开始对话；新对话自动继承工作区默认知识库与本体方案。工作区宜少而精——一个业务场景一个工作区即可，不必为每个项目单建。
                  </Typography.Text>
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
              onOpenProject={openProjectInProjectsModule}
            />
          )}
        </div>

        {createOpen && <WorkspaceModal onClose={() => setCreateOpen(false)} onCreated={onWorkspaceCreated} workspaces={workspaces} />}
      </Splitter.Panel>
    </Splitter>
  )
}

/**
 * 工作区总览（REQ-613-b6cc/M421-b6cc，121 号 R1①）：未打开会话时替代空态灰字——
 * 计数行（N 项目 · M 对话，后端计数尾列）+ 三个侧板入口（配置/笔记本绑定/记忆）+
 * 名下项目卡列（listProjects({workspace_id}) REQ-587 过滤载荷；卡点击跳项目模块选中该项目）。
 * 归属可选如实（121 号 §4.0-4）：无归属项目给指引不强迫；管理动作仍归侧板 projects 视图（C4 边界）。
 */
function WorkspaceOverview({
  ws,
  projects,
  onConfigure,
  onNotebooks,
  onMemory,
  onNewConversation,
  onOpenProject,
}: {
  ws: Workspace
  projects: Project[]
  onConfigure: () => void
  onNotebooks: () => void
  onMemory: () => void
  onNewConversation: () => void
  onOpenProject: (pid: string, pname: string) => void
}) {
  return (
    <div data-testid="ws-overview">
      <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 4 }}>
        ⌂ 工作区：{ws.name}
      </Typography.Title>
      {ws.description && (
        <Typography.Text type="secondary" style={{ fontSize: 12.5, display: 'block', marginBottom: 8 }}>
          {ws.description}
        </Typography.Text>
      )}
      {/* 计数行：与树徽标同源（GET /api/workspaces 计数尾列），口径=归属项目 ∪（直挂∪项目会话） */}
      <Typography.Text type="secondary" style={{ fontSize: 12.5, display: 'block', marginBottom: 12 }} data-testid="ws-ov-stats">
        {ws.project_count ?? 0} 个项目 · {ws.conv_count ?? 0} 个对话（新对话自动继承本工作区默认知识库与本体方案）
      </Typography.Text>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }} data-testid="ws-ov-entries">
        <Button size="small" icon={<SettingOutlined />} onClick={onConfigure} data-testid="ws-ov-config">配置</Button>
        <Tooltip title="笔记本绑定在侧板「配置」视图（项目会话产物「存笔记本」的写入目标）">
          <Button size="small" icon={<BookOutlined />} onClick={onNotebooks} data-testid="ws-ov-notebook">笔记本绑定</Button>
        </Tooltip>
        <Button size="small" icon={<HistoryOutlined />} onClick={onMemory} data-testid="ws-ov-memory">记忆</Button>
        <Button size="small" type="primary" onClick={onNewConversation} data-testid="ws-ov-new-conv">＋ 新建对话</Button>
      </div>

      <Typography.Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
        名下项目（{projects.length}）
      </Typography.Text>
      {projects.length === 0 ? (
        // 归属可选如实：未归属项目在项目模块平铺树原样运行，不设惩罚性分区（121 号 §4.0-4）
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={
            <Typography.Text type="secondary" style={{ fontSize: 12.5 }} data-testid="ws-ov-projects-empty">
              暂无名下项目——可在「项目」模块把项目归属到本工作区（新会话自动继承默认知识背景），项目也可独立运行。
            </Typography.Text>
          }
          style={{ margin: '12px 0' }}
        />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: 10 }} data-testid="ws-ov-projects">
          {projects.map((p) => (
            <div
              key={p.id}
              data-testid={`ws-ov-project-${p.id}`}
              role="button"
              tabIndex={0}
              title={`打开项目「${p.name}」（跳转项目模块）`}
              onClick={() => onOpenProject(p.id, p.name)}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onOpenProject(p.id, p.name) }}
              style={{ border: '1px solid var(--c-line, #e3e6f0)', borderRadius: 10, padding: '10px 12px', cursor: 'pointer', background: 'var(--c-panel, #fff)', transition: 'box-shadow .15s ease' }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLDivElement).style.boxShadow = '0 2px 10px rgba(28,33,48,.10)' }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLDivElement).style.boxShadow = 'none' }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                <ProjectOutlined style={{ color: 'var(--c-brand)', flex: 'none' }} />
                <span style={{ fontWeight: 600, fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{p.name}</span>
              </div>
              <Typography.Text type="secondary" style={{ fontSize: 11.5, display: 'block', marginTop: 4 }}>
                {p.conv_count ?? 0} 个对话 · 点击到项目模块打开
              </Typography.Text>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/** 新建工作区向导（REQ-590/M431，116 号 §REQ-590；前身=M1b 最小件名称+描述弹窗）：
 *  三选起点——①全新空 ②从现有工作区复制默认值（快照继承：新工作区落自身三默认列，
 *  源后续改动不回传，与写时复制同构）③场景模板起（内容单源 seeds/workspace-templates/，
 *  仅预填命名引导与描述，不含本体示例——含本体示例按归属走本体侧领号，v1 不含）。
 *  创建前预览=将携带的默认值清单如实呈现（copy 起点解析源三默认名称，未设置/不可解析如实标注）；
 *  名称 placeholder 用途句式（116 号口径「这个工作区放哪类知识？」）。
 *  REQ-615-721e B3 描述场景示例句保留（模板预填优先）。零后端——POST /api/workspaces 全结构体
 *  透传（handlers_workspaces.go decodeJSON store.Workspace），复制起点由前端快照三默认字段。 */
type WsOrigin = 'blank' | 'copy' | 'template'

const WS_TEMPLATES = (() => {
  try {
    const parsed = JSON.parse(WS_TEMPLATES_RAW as string) as { templates: { key: string; label: string; description: string; guidance: string }[] }
    return parsed.templates ?? []
  } catch {
    return [] // 模板单源解析失败=向导降级为两起点（不阻塞建区主流程）
  }
})()

function WorkspaceModal({ onClose, onCreated, workspaces }: { onClose: () => void; onCreated: (id: string) => void; workspaces: Workspace[] }) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [origin, setOrigin] = useState<WsOrigin>('blank')
  const [sourceId, setSourceId] = useState<string | undefined>(undefined)
  const [templateKey, setTemplateKey] = useState<string | undefined>(undefined)
  // 预览名称解析（copy 起点）：沿 WorkspaceSidePanel 同源清单（KB 全量/方案 running/智能体全量）
  const [refs, setRefs] = useState<{ kbs: [string, string][]; profiles: [string, string][]; agents: [string, string][] } | null>(null)
  useEffect(() => {
    if (origin !== 'copy' || refs) return
    Promise.all([
      api.listKBs().catch(() => [] as KnowledgeBase[]),
      api.listRuntimeProfiles().catch(() => [] as RuntimeProfile[]),
      api.listAgents().catch(() => [] as Agent[]),
    ]).then(([kbs, profiles, agents]) => {
      setRefs({
        kbs: (kbs as KnowledgeBase[]).map((k) => [k.id, k.name]),
        profiles: (profiles as RuntimeProfile[]).filter((p) => p.status === 'running').map((p) => [p.id, p.name || p.id]),
        agents: (agents as Agent[]).map((a) => [a.id, a.is_builtin ? `${a.name}（内置）` : a.name]),
      })
    })
  }, [origin, refs])

  const source = origin === 'copy' ? workspaces.find((w) => w.id === sourceId) : undefined
  const template = origin === 'template' ? WS_TEMPLATES.find((t) => t.key === templateKey) : undefined

  const pickTemplate = (key?: string) => {
    setTemplateKey(key)
    const tpl = WS_TEMPLATES.find((t) => t.key === key)
    if (tpl) form.setFieldsValue({ description: tpl.description })
  }

  const submit = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setSaving(true)
    try {
      // 快照继承：复制起点把源工作区三默认落新工作区自身列（改动不回传；空值=未设置同既有零值语义）
      const payload: Partial<Workspace> = { name: v.name, description: v.description ?? '' }
      if (origin === 'copy' && source) {
        payload.default_kb_id = source.default_kb_id || ''
        payload.default_profile_id = source.default_profile_id || ''
        payload.default_agent_id = source.default_agent_id || ''
      }
      const ws = await api.createWorkspace(payload)
      onCreated(ws.id)
    } catch (e: any) {
      showToast(e.message, 'err')
      setSaving(false)
    }
  }

  const resolveName = (list: [string, string][] | null, id: string): string => {
    if (!id) return '未设置'
    const hit = list?.find(([k]) => k === id)
    return hit ? hit[1] : `${id.slice(0, 8)}…（源值当前不可解析——创建后可调整）`
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
      width={560}
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        {/* REQ-590 三选起点 */}
        <Form.Item label="起点" style={{ marginBottom: 12 }}>
          <Segmented
            value={origin}
            onChange={(v) => setOrigin(v as WsOrigin)}
            options={[
              { value: 'blank', label: '全新空工作区' },
              { value: 'copy', label: '复制现有工作区默认值' },
              { value: 'template', label: '场景模板起' },
            ]}
            data-testid="ws-wizard-origin"
          />
        </Form.Item>
        {origin === 'copy' && (
          <Form.Item label="源工作区（快照继承其会话默认值）" style={{ marginBottom: 12 }}>
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="选择要复制默认值的工作区"
              value={sourceId}
              onChange={setSourceId}
              options={workspaces.map((w) => ({ value: w.id, label: w.name }))}
              data-testid="ws-wizard-copy-source"
            />
          </Form.Item>
        )}
        {origin === 'template' && (
          <Form.Item label="场景模板（单源 seeds/workspace-templates）" style={{ marginBottom: 12 }}>
            <Select
              allowClear
              placeholder="选择场景模板（预填描述与配置指引）"
              value={templateKey}
              onChange={pickTemplate}
              options={WS_TEMPLATES.map((t) => ({ value: t.key, label: t.label }))}
              data-testid="ws-wizard-template"
            />
          </Form.Item>
        )}
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]}>
          <Input placeholder="这个工作区放哪类知识？" data-testid="ws-wizard-name" />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea
            autoSize={{ minRows: 2, maxRows: 4 }}
            placeholder="一句话描述这个领域场景（可选）。例：机器学习论文研读——区内对话默认使用论文知识库与对应本体方案"
          />
        </Form.Item>
        {/* REQ-590 创建前预览：将携带的默认值清单（如实呈现） */}
        <div
          data-testid="ws-wizard-preview"
          style={{ border: '1px dashed var(--c-border, #e6e8f0)', borderRadius: 8, padding: '8px 12px', marginBottom: 4, fontSize: 12, color: 'var(--c-ink-2, #5a6478)' }}
        >
          {origin === 'blank' && <div>将携带的默认值：无——创建后可在右侧板「会话默认值」配置（新对话暂回落平台助手/不启用召回）。</div>}
          {origin === 'copy' && !source && <div>选择源工作区后，此处预览将复制过去的默认值清单。</div>}
          {origin === 'copy' && source && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }} data-testid="ws-wizard-preview-copy">
              <div>将携带的默认值（快照继承自「{source.name}」，创建后源改动不回传）：</div>
              <div data-testid="ws-wizard-preview-kb">· 默认知识库：{refs ? resolveName(refs.kbs, source.default_kb_id) : '解析中…'}</div>
              <div data-testid="ws-wizard-preview-profile">· 默认本体方案：{refs ? resolveName(refs.profiles, source.default_profile_id) : '解析中…'}{source.default_profile_id && refs && !refs.profiles.some(([k]) => k === source.default_profile_id) ? '（源值当前不在 running 列表——创建后可调整）' : ''}</div>
              <div data-testid="ws-wizard-preview-agent">· 默认智能体：{refs ? resolveName(refs.agents, source.default_agent_id) : '解析中…'}</div>
              {source.default_kb_id === '' && source.default_profile_id === '' && source.default_agent_id === '' && (
                <div>源工作区未配置默认值——创建后可在右侧板配置。</div>
              )}
            </div>
          )}
          {origin === 'template' && (
            <div data-testid="ws-wizard-preview-template">
              {template ? (
                <>
                  <div>模板「{template.label}」将预填描述（可改）；{template.guidance}</div>
                  <div>默认值不在模板内预置——创建后按上述指引配置。</div>
                </>
              ) : (
                <div>选择场景模板后，此处显示将预填的内容与配置指引。</div>
              )}
            </div>
          )}
        </div>
      </Form>
    </Modal>
  )
}
