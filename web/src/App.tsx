import { useUI } from './store/ui'
import GlobalNav from './components/GlobalNav'
import AgentsPage from './pages/AgentsPage'
import ProjectsPage from './pages/ProjectsPage'
import WorkspacePage from './pages/WorkspacePage'
import SettingsPage from './pages/SettingsPage'
import KnowledgePage from './pages/KnowledgePage'
import SkillsPage from './pages/SkillsPage'
import ReferencePage from './pages/ReferencePage'
import OntologyPage from './pages/OntologyPage'

// REQ-326①：全局导航左置——.app 由「顶栏 + 主区」改为「左侧全局导航（可收缩）+ 主区」，
// TopNav 退役（导航/主题/设置入口迁 GlobalNav）。
export default function App() {
  const { page } = useUI()
  return (
    <div className="app">
      <GlobalNav />
      {/* 主区：左栏/右栏均为满高分栏（.app-body 为行向 flex，子页撑满剩余高度） */}
      <div className="app-body">
        {page === 'agents' && <AgentsPage />}
        {/* REQ-614-9fb4/M422-9fb4 R1②：项目模块恢复纯项目页（页签退役，/projects 语义归位） */}
        {page === 'projects' && <ProjectsPage />}
        {/* REQ-613-b6cc/M421-b6cc 工作区独立模块页（121 号 R1①；R1② 起为工作区唯一宿主） */}
        {page === 'workspaces' && <WorkspacePage />}
        {page === 'ontology' && <OntologyPage />}
        {page === 'knowledge' && <KnowledgePage />}
        {page === 'skills' && <SkillsPage />}
        {page === 'reference' && <ReferencePage />}
        {page === 'settings' && <SettingsPage />}
      </div>
    </div>
  )
}
