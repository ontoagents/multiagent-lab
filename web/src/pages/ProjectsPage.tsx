import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Dropdown, Splitter, Typography } from 'antd'
import { DownOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent, Conversation, Project, Workspace } from '../api/types'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import Sidebar from '../components/Sidebar'
import ChatWindow from '../components/ChatWindow'
import ProjectModal from '../components/ProjectModal'
import ProjectSidePanel, { type PanelView } from '../components/ProjectSidePanel'
import HybridTeamTemplate from '../components/HybridTeamTemplate'
import DevTeamTemplate from '../components/DevTeamTemplate'

/**
 * 项目模块（REQ-614-9fb4/M422-9fb4 双模块分立 R1②，121 号 §4.1：REQ-597 页签结构退役，
 * 模块体=加工作区前的项目体验归位）：
 * 左栏为项目折叠列表（每节点常驻「＋新对话」「⚙配置」；已归属项目行「⌂ 工作区《X》」chip 互链跳
 * 工作区模块），右侧为与智能体视图一致的对话窗口；协作模板入口与模板弹窗原位保留（不迁移）。
 * REQ-103：项目配置改为右侧边栏配置视图（原 ProjectModal 编辑表单迁入）；ProjectModal 仅用于新建。
 * 深链（121 号 §5.2 重排后）：eino.focus.project→本模块选中项目+开侧板（eino:goto.project 事件+
 * 挂载消费双入口）；eino.focus.conv（scope=project）→本模块打开该会话（分流基座 lib/deepLink）。
 */
export default function ProjectsPage() {
  const { currentConvId, setCurrentConv, dataVersion, bumpData, showToast } = useUI()
  const [projects, setProjects] = useState<Project[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null) // REQ-515/M322 三态收口
  const [convs, setConvs] = useState<Conversation[]>([])
  // 归属 chip 名称解析（REQ-614-9fb4 R1②：⌂ 工作区《X》chip——清单拉取失败静默=chip 回落显 id）
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  // REQ-536/M390 现场恢复：选中项目进 sessionStorage（eino.project.active）
  const [activeProjectId, setActiveProjectId] = useState<string | null>(() => {
    try {
      return sessionStorage.getItem('eino.project.active')
    } catch {
      return null
    }
  })
  useEffect(() => {
    try {
      if (activeProjectId) sessionStorage.setItem('eino.project.active', activeProjectId)
      else sessionStorage.removeItem('eino.project.active')
    } catch {}
  }, [activeProjectId])
  // REQ-580/M395：项目侧深链（eino.focus.project）——选中该项目+开侧板+config 视图（SaveToNotebook
  // 未归属分支/工作区总览名下项目卡直达）。REQ-614-9fb4 R1② 重排：事件键 eino:goto.projtab.project
  // 退役改 eino:goto.project（页签语义退役）；并补挂载消费——跨模块导航（setPage('projects')）后本组件
  // 重新挂载、事件派发发生在挂载前收不到，键由挂载消费承接（121 号 §5.2）。
  // 删键时机=命中才删（AgentsPage 协议）：跨模块挂载首轮 projects 尚未载入，未命中即删会丢键。
  useEffect(() => {
    const consume = () => {
      try {
        const pid = sessionStorage.getItem('eino.focus.project')
        if (!pid) return
        if (projects.some((p) => p.id === pid)) {
          sessionStorage.removeItem('eino.focus.project')
          setActiveProjectId(pid)
          setSidePanelOpen(true)
          setPanelView('config')
        }
      } catch { /* sessionStorage 不可用静默 */ }
    }
    consume()
    window.addEventListener('eino:goto.project', consume)
    return () => window.removeEventListener('eino:goto.project', consume)
  }, [projects])
  // REQ-614-9fb4 R1②：focus.conv 分流到达侧（scope=project 会话→本模块打开；lib/deepLink 写入，
  // 消费协议沿 AgentsPage REQ-584 先例——载荷命中才消费删键，未命中保留给其他模块回落）。
  // 两段式：先提交容器选中（useState 批处理），归属匹配后的下一个 effect 再 setCurrentConv——
  // 防 zustand 同步更新与 useState 批处理的提交错位被归位守卫清空（冒烟 F1 竞态实录）。
  const pendingConvRef = useRef<string | null>(null)
  useEffect(() => {
    if (convs.length === 0) return
    try {
      const convId = sessionStorage.getItem('eino.focus.conv')
      if (!convId) return
      const conv = convs.find((c) => c.id === convId)
      if (conv && conv.scope === 'project') {
        sessionStorage.removeItem('eino.focus.conv')
        if (conv.project_id) setActiveProjectId(conv.project_id)
        pendingConvRef.current = conv.id
      }
    } catch { /* 静默 */ }
  }, [convs])
  useEffect(() => {
    const pend = pendingConvRef.current
    if (!pend) return
    const conv = convs.find((c) => c.id === pend)
    if (conv && conv.project_id === activeProjectId) {
      pendingConvRef.current = null
      setCurrentConv(conv.id)
    }
  }, [activeProjectId, convs, setCurrentConv])
  const [createOpen, setCreateOpen] = useState(false)
  // REQ-102/103：右侧侧边栏（文件 / Git / 配置）开合与当前视图
  const [sidePanelOpen, setSidePanelOpen] = useState(false)
  const [panelView, setPanelView] = useState<PanelView>('files')
  // REQ-489：模板弹窗入口句柄（空态菜单触发用）
  const devTplOpen = useRef<(() => void) | null>(null)
  const hybTplOpen = useRef<(() => void) | null>(null)

  const reload = () => {
    // REQ-515/M322 三态收口：主列表失败进错误态（LoadErrorAlert），不再吞成空列表
    api.listProjects().then((ps) => {
      setProjects(ps)
      setLoadErr(null)
      setActiveProjectId((cur) => (cur && ps.some((p) => p.id === cur) ? cur : (ps[0]?.id ?? null)))
    }).catch((e) => {
      setProjects([])
      setLoadErr(e?.message ?? '项目列表加载失败')
    })
    api.listAgents().then(setAgents).catch(() => setAgents([]))
    api.listConversations().then(setConvs).catch(() => setConvs([]))
    // REQ-614-9fb4 R1②：归属 chip 名称解析清单（全量；失败静默=chip 回落显 id）
    api.listWorkspaces().then(setWorkspaces).catch(() => setWorkspaces([]))
  }

  useEffect(reload, [dataVersion])

  const activeProject = projects.find((p) => p.id === activeProjectId) ?? null
  const currentConv = useMemo(() => convs.find((c) => c.id === currentConvId) ?? null, [convs, currentConvId])

  // 选中对话不属于当前项目（或属于智能体）时，归位到空态
  useEffect(() => {
    if (currentConv && !(currentConv.scope === 'project' && currentConv.project_id === activeProjectId)) {
      setCurrentConv(null)
    }
  }, [activeProjectId, currentConv])

  const newConversation = async (projectId: string) => {
    try {
      const c = await api.createConversation({ scope: 'project', project_id: projectId, title: '新对话' })
      bumpData()
      setActiveProjectId(projectId)
      setCurrentConv(c.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  /** 项目节点 ⚙ → 右侧边栏配置视图（侧边栏若未开则同时打开） */
  const configureProject = (projectId: string) => {
    setActiveProjectId(projectId)
    setSidePanelOpen(true)
    setPanelView('config')
  }

  /** REQ-614-9fb4 R1②：项目行「⌂ 工作区《X》」chip → 工作区模块并选中该工作区（121 号 §4.1 互链）。
   *  跨模块导航：写 focus.workspace 键后 setPage（WorkspacePage 挂载消费选中+开侧板绑定入口）；
   *  事件同批派发（同模块常驻场景由响应式监听承接——虽本 chip 恒跨模块，协议对称保留）。 */
  const openWorkspaceInWorkspaceModule = (wsId: string) => {
    try {
      sessionStorage.setItem('eino.focus.workspace', wsId)
    } catch { /* 配额满静默（沿 REQ-509 口径） */ }
    useUI.getState().setPage('workspaces')
    window.dispatchEvent(new CustomEvent('eino:goto.workspace'))
  }

  const onProjectCreated = (id: string) => {
    setCreateOpen(false)
    reload()
    setActiveProjectId(id)
    setSidePanelOpen(true)
    setPanelView('config')
    showToast('项目已创建，请在侧边栏完善配置')
  }

  return (
    <Splitter
      className="main sidebar-splitter"
      onResizeEnd={sidebarRemember}
    >
      <Splitter.Panel defaultSize={sidebarDefaultSize()} min={SIDEBAR_WIDTH.min} max={SIDEBAR_WIDTH.max} className="sidebar-panel">
        <Sidebar
          mode="project"
          agents={[]}
          projects={projects}
          workspaces={workspaces}
          conversations={convs}
          activeAgentId={null}
          activeProjectId={activeProjectId}
          onSelectAgent={() => {}}
          onSelectProject={setActiveProjectId}
          onNewAgent={() => {}}
          onNewProject={() => setCreateOpen(true)}
          onNewConversation={newConversation}
          onConfigureAgent={() => {}}
          onConfigureProject={configureProject}
          onOpenWorkspace={openWorkspaceInWorkspaceModule}
          loadErr={loadErr}
          onRetry={reload}
        />
      </Splitter.Panel>
      <Splitter.Panel className="content-panel">
        {/* REQ-102：对话区 + 右侧项目侧边栏（flex 兄弟节点，互不遮挡） */}
        <div className="proj-content-row">
          <div className="proj-content-main">
            {currentConv && currentConv.scope === 'project' ? (
              <ChatWindow
                conversation={currentConv}
                agents={agents}
                projects={projects}
                onConversationUpdated={reload}
                onToggleSidePanel={() => setSidePanelOpen((o) => !o)}
              />
            ) : (
              // REQ-489：空态一钮+模板收菜单（Result 盒与四钮群退役；对齐 REQ-488 空态灰字语言；
              // 项目配置入口由常驻侧板竖条承载〔REQ-237〕，此处不再重复）
              <div className="page" style={{ padding: '48px 40px' }}>
                <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="projects-empty-quiet">
                  在左侧选择项目，与其开始对话；会话界面与智能体视图一致。
                </Typography.Text>
                {activeProject && (
                  <div style={{ marginTop: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
                    <Button type="primary" size="small" onClick={() => newConversation(activeProject.id)}>
                      ＋ 新建对话
                    </Button>
                    <Dropdown
                      menu={{
                        items: [
                          { key: 'dev', label: '协作研发模板（规划/编码/审查）' },
                          { key: 'hybrid', label: '混合编排模板（本地+CLI+外部）' },
                        ],
                        onClick: ({ key }) => (key === 'dev' ? devTplOpen.current : hybTplOpen.current)?.(),
                      }}
                    >
                      <Button size="small">
                        协作模板 <DownOutlined />
                      </Button>
                    </Dropdown>
                  </div>
                )}
                {/* REQ-417/M213+REQ-431/M227：模板弹窗宿主（入口由上方菜单触发——onReady 暴露 open） */}
                {activeProject && <DevTeamTemplate agents={agents} onCreated={(pid) => { reload(); setActiveProjectId(pid) }} onReady={(o) => { devTplOpen.current = o }} />}
                {activeProject && <HybridTeamTemplate agents={agents} onCreated={(pid) => { reload(); setActiveProjectId(pid) }} onReady={(o) => { hybTplOpen.current = o }} />}
              </div>
            )}
          </div>

          {/* REQ-237：侧板常驻挂载（REQ-217⑤ 定案落地）——竖条随选中项目常驻，open 仅控制展开/收缩 */}
          {activeProject && (
            <ProjectSidePanel
              project={activeProject}
              agents={agents}
              open={sidePanelOpen}
              view={panelView}
              onViewChange={setPanelView}
              onOpenChange={setSidePanelOpen}
              onChanged={reload}
            />
          )}
        </div>

        {createOpen && <ProjectModal agents={agents} onClose={() => setCreateOpen(false)} onCreated={onProjectCreated} />}
      </Splitter.Panel>
    </Splitter>
  )
}
