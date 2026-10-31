import { useEffect, useMemo, useRef, useState } from 'react'
import { Splitter, Typography } from 'antd'
import { api } from '../api/client'
import type { Agent, Conversation } from '../api/types'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import Sidebar from '../components/Sidebar'
import OnboardingGuide from '../components/OnboardingGuide'
import ChatWindow from '../components/ChatWindow'
import AgentModal from '../components/AgentModal'
import AgentSidePanel from '../components/AgentSidePanel'

/**
 * 智能体视图（原型 06 §3.1）：
 * 左栏为智能体折叠列表（每节点常驻「＋新对话」「⚙配置」），右侧为对话窗口。
 * REQ-103：智能体配置改为右侧边栏配置视图（原 AgentModal 编辑表单迁入）；AgentModal 仅用于新建。
 */
export default function AgentsPage() {
  const { currentConvId, setCurrentConv, dataVersion, bumpData, showToast } = useUI()
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null) // REQ-515/M322 三态收口
  const [convs, setConvs] = useState<Conversation[]>([])
  const [activeAgentId, setActiveAgentId] = useState<string | null>(null)
  // REQ-461/M261：被引用面板深链（eino.agents.focus）——agents 载入后自动选中
  useEffect(() => {
    if (agents.length === 0) return
    const focus = localStorage.getItem('eino.agents.focus')
    if (focus && agents.some((a) => a.id === focus)) {
      localStorage.removeItem('eino.agents.focus')
      setActiveAgentId(focus)
      setSidePanelOpen(true)
    }
  }, [agents])
  const [createOpen, setCreateOpen] = useState(false)
  // REQ-103：右侧智能体侧边栏（配置视图）开合
  const [sidePanelOpen, setSidePanelOpen] = useState(false)
  // REQ-360/M161：跨模块预填启动消息（构建平面「在智能体对话中构建/转到对话继续」跳转载荷）——
  // 挂载时一次性消费 eino.chat.preset：新建该智能体会话并预填（预填不代发，最后一脚留给用户）。
  // 以会话 id 匹配下发，防后续手动新建会话吃到陈旧载荷。
  const draftRef = useRef<{ convId: string; msg: string } | null>(null)

  useEffect(() => {
    const raw = localStorage.getItem('eino.chat.preset')
    if (!raw) return
    localStorage.removeItem('eino.chat.preset')
    let agentId = ''
    let msg = ''
    try {
      const p = JSON.parse(raw)
      agentId = p?.agentId ?? ''
      msg = p?.msg ?? ''
    } catch {
      return
    }
    if (!agentId || !msg) return
    ;(async () => {
      try {
        const c = await api.createConversation({ scope: 'agent', agent_id: agentId, title: '新对话' })
        bumpData()
        setActiveAgentId(agentId)
        setCurrentConv(c.id)
        draftRef.current = { convId: c.id, msg }
      } catch {
        /* 建会话失败静默（跳转方已有 toast 语境） */
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const reload = () => {
    // REQ-515/M322 三态收口：主列表失败进错误态（LoadErrorAlert），不再吞成空列表
    api.listAgents().then((as) => {
      setAgents(as)
      setLoadErr(null)
      setActiveAgentId((cur) => (cur && as.some((a) => a.id === cur) ? cur : (as[0]?.id ?? null)))
    }).catch((e) => {
      setAgents([])
      setLoadErr(e?.message ?? '智能体列表加载失败')
    })
    api.listConversations().then(setConvs).catch(() => setConvs([]))
  }

  useEffect(reload, [dataVersion])

  const activeAgent = agents.find((a) => a.id === activeAgentId) ?? null
  const currentConv = useMemo(() => convs.find((c) => c.id === currentConvId) ?? null, [convs, currentConvId])

  // 选中对话不属于当前智能体（或属于项目）时，归位到空态
  useEffect(() => {
    if (currentConv && !(currentConv.scope === 'agent' && currentConv.agent_id === activeAgentId)) {
      setCurrentConv(null)
    }
  }, [activeAgentId, currentConv])

  // 为指定智能体新建对话
  const newConversation = async (agentId: string) => {
    try {
      const c = await api.createConversation({ scope: 'agent', agent_id: agentId, title: '新对话' })
      bumpData()
      setActiveAgentId(agentId)
      setCurrentConv(c.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  /** 智能体节点 ⚙ → 右侧边栏配置视图（侧边栏若未开则同时打开） */
  const configureAgent = (agentId: string) => {
    // REQ-213：内置助手与普通智能体统一——配置入口同为配置侧板（身份字段只读、
    // 基座工具禁用、提案横幅内置行呈现）；REQ-192 的「跳设置页」路径退役。
    setActiveAgentId(agentId)
    setSidePanelOpen(true)
  }

  /** ChatWindow「配置」→ 打开当前智能体侧边栏配置视图 */

  const onAgentCreated = (id: string) => {
    setCreateOpen(false)
    reload()
    setActiveAgentId(id)
    setSidePanelOpen(true)
    showToast('智能体已创建，请在侧边栏完善配置')
  }

  return (
    <>
    <OnboardingGuide />
    <Splitter
      className="main sidebar-splitter"
      onResizeEnd={sidebarRemember}
    >
      <Splitter.Panel defaultSize={sidebarDefaultSize()} min={SIDEBAR_WIDTH.min} max={SIDEBAR_WIDTH.max} className="sidebar-panel">
        <Sidebar
          mode="agent"
          agents={agents}
          projects={[]}
          conversations={convs}
          activeAgentId={activeAgentId}
          activeProjectId={null}
          onSelectAgent={setActiveAgentId}
          onSelectProject={() => {}}
          onNewAgent={() => setCreateOpen(true)}
          onNewProject={() => {}}
          onNewConversation={newConversation}
          onConfigureAgent={configureAgent}
          onConfigureProject={() => {}}
          loadErr={loadErr}
          onRetry={reload}
        />
      </Splitter.Panel>
      <Splitter.Panel className="content-panel">
        {/* REQ-103：对话区 + 右侧智能体侧边栏（flex 兄弟节点，互不遮挡） */}
        <div className="proj-content-row">
          <div className="proj-content-main">
            {currentConv && currentConv.scope === 'agent' ? (
              <ChatWindow
                conversation={currentConv}
                agents={agents}
                projects={[]}
                onConversationUpdated={reload}
                onToggleSidePanel={() => setSidePanelOpen((o) => !o)}
                initialDraft={draftRef.current?.convId === currentConv.id ? draftRef.current.msg : undefined}
              />
            ) : (
              // REQ-488：空态单列一行灰字（REQ-483 视觉语言——去 Result 盒与开场大钮）
              <div className="page" style={{ padding: '48px 40px' }}>
                <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="agents-empty-quiet">
                  在左侧选择智能体，与其开始对话；节点上的 ＋ 可新建对话。
                </Typography.Text>
              </div>
            )}
          </div>

          {/* REQ-237：侧板常驻挂载（REQ-217⑤「默认收缩 44px 竖条常驻右侧」定案落地）——
              竖条随选中智能体常驻，open 仅控制展开/收缩，头部收放按钮与竖条入口同源 */}
          {activeAgent && (
            <AgentSidePanel
              agent={activeAgent}
              open={sidePanelOpen}
              onOpenChange={setSidePanelOpen}
              onChanged={reload}
            />
          )}
        </div>

        {createOpen && <AgentModal onClose={() => setCreateOpen(false)} onCreated={onAgentCreated} />}
      </Splitter.Panel>
    </Splitter>
    </>
  )
}