import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Dropdown, Splitter, Tooltip, Typography } from 'antd'
import { BlockOutlined, DownOutlined, ProjectOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent, Conversation, Project } from '../api/types'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import Sidebar from '../components/Sidebar'
import ChatWindow from '../components/ChatWindow'
import ProjectModal from '../components/ProjectModal'
import ProjectSidePanel, { type PanelView } from '../components/ProjectSidePanel'
import HybridTeamTemplate from '../components/HybridTeamTemplate'
import DevTeamTemplate from '../components/DevTeamTemplate'
import WorkspacePage from './WorkspacePage'

/** REQ-549/M356：模块内页签选中记忆（切走再切回不丢位置；沿 eino.onto.sidebar 先例） */
const PROJ_TAB_KEY = 'eino.proj.tab'
type ProjTab = 'project' | 'workspace'

/**
 * 项目模块（REQ-549/M356 M1b）：内容区顶部「项目 | 工作区」模块内页签（module-tabs 先例，
 * 导航六项不动、ui.ts/App.tsx 零改动——106 号 mustFix-3 终值），页签下为两套页面体。
 */
export default function ProjectsPage() {
  const [tab, setTab] = useState<ProjTab>(() => (localStorage.getItem(PROJ_TAB_KEY) === 'workspace' ? 'workspace' : 'project'))
  const selectTab = (t: ProjTab) => {
    setTab(t)
    try { localStorage.setItem(PROJ_TAB_KEY, t) } catch { /* 配额满静默（沿 REQ-509 本地存档口径） */ }
  }
  return (
    <div className="module-shell">
      <div className="module-tabs" role="tablist" aria-label="项目模块导航" data-testid="proj-module-tabs">
        <Tooltip title="执行容器：目录/Git/文件/成员协作" placement="bottom" mouseEnterDelay={0.4}>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'project'}
            aria-current={tab === 'project' ? 'page' : undefined}
            className={`module-tab${tab === 'project' ? ' active' : ''}`}
            onClick={() => selectTab('project')}
            data-testid="proj-tab-projects"
          >
            <span className="module-tab-icon"><ProjectOutlined /></span>
            <span className="module-tab-label">项目</span>
          </button>
        </Tooltip>
        <Tooltip title="领域容器：会话默认值与直挂对话" placement="bottom" mouseEnterDelay={0.4}>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'workspace'}
            aria-current={tab === 'workspace' ? 'page' : undefined}
            className={`module-tab${tab === 'workspace' ? ' active' : ''}`}
            onClick={() => selectTab('workspace')}
            data-testid="proj-tab-workspaces"
          >
            <span className="module-tab-icon"><BlockOutlined /></span>
            <span className="module-tab-label">工作区</span>
          </button>
        </Tooltip>
      </div>
      <div className="module-body">{tab === 'workspace' ? <WorkspacePage /> : <ProjectsBody />}</div>
    </div>
  )
}

/**
 * 项目页体（原型 06 §3.2）：
 * 左栏为项目折叠列表（每节点常驻「＋新对话」「⚙配置」），右侧为与智能体视图一致的对话窗口。
 * REQ-103：项目配置改为右侧边栏配置视图（原 ProjectModal 编辑表单迁入）；ProjectModal 仅用于新建。
 */
function ProjectsBody() {
  const { currentConvId, setCurrentConv, dataVersion, bumpData, showToast } = useUI()
  const [projects, setProjects] = useState<Project[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null) // REQ-515/M322 三态收口
  const [convs, setConvs] = useState<Conversation[]>([])
  const [activeProjectId, setActiveProjectId] = useState<string | null>(null)
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
