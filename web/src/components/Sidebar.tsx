import { useEffect, useMemo, useState } from 'react'
import { Button, Input } from 'antd'
import {
  PlusOutlined,
  AppstoreOutlined,
  DeleteOutlined,
  EditOutlined,
  DownOutlined,
  EllipsisOutlined,
  FolderOutlined,
  MessageOutlined,
  RightOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import { Conversations } from '@ant-design/x'
import { api } from '../api/client'
import EmptyGuide from './EmptyGuide'
import LoadErrorAlert from './LoadErrorAlert'
import type { Agent, Conversation, Project, Workspace } from '../api/types'
import { useUI } from '../store/ui'
import { confirmAction } from '../lib/antd'
import { CollapsedRail, SidebarCollapseButton, useSidebarCollapse } from '../lib/sidebar'
import { getRecent, pushRecent, RECENT_EVENT, type RecentKind } from '../lib/recent' // REQ-616-aa64/M424-aa64 A7（121 号 R3）
import { gotoConversation } from '../lib/deepLink' // REQ-616-aa64/M424-aa64 D1（121 号 R3）：跨模块命中分流打开
import { RELATION_SENTENCE } from '../lib/relations' // REQ-617-87d8/M425-87d8 R4（121 号 §4.4 正式关系句单源——两模块空态落点）
import NameModal from './NameModal'

interface TreeNode {
  key: string
  name: string
  logo?: string
  backend?: string
  builtin?: boolean // REQ-186：内置助手（置顶分区、内置徽标、无配置入口）
  convs: Conversation[]
  /** REQ-613-b6cc/M421-b6cc 容器视觉语言（121 号 R1① A3，仅 workspace 模式置位）：
   *  ▦ 容器图标（REQ-618-36a1 起 Appstore，⌂ house 退役）+ 图标数字紧凑徽标；空壳节点（convs 空 ∪ convCount=0）不渲染 chevron */
  container?: boolean
  projectCount?: number
  convCount?: number
  /** REQ-614-9fb4/M422-9fb4 归属 chip（121 号 §4.1 互链，仅 project 模式置位）：
   *  已归属项目行「▦ 工作区《X》」chip，点击跳工作区模块并选中该工作区 */
  workspaceId?: string
}

/**
 * 左栏树（原型 06 §3.1 / §3.2）：
 * - mode=agent：智能体节点（头像点 + 会话数轻量数字 + 常驻「＋新对话」「⚙配置」），其下挂该智能体的对话；
 *   双入口共存口径（REQ-549）：工作区直挂对话同现于所属智能体节点——REQ-616-aa64/M424-aa64 C2（121 号 R3）
 *   「·工作区」4 字灰字升级为可点击 chip「▦ 工作区《名》」（onOpenWorkspace 跳工作区模块）；
 * - mode=project：项目节点同样结构，其下挂项目对话；树顶 ⭐ 最近打开虚拟分组（A7，lib/recent.ts）；
 * - mode=workspace：工作区节点（REQ-549/M356 M1b），其下挂 scope=agent 且归属本工作区的直挂对话；
 *   REQ-613-b6cc/M421-b6cc 容器视觉语言：⌂ 图标+「N 项目 · M 对话」计数徽标（后端计数尾列），
 *   空壳节点不渲染 chevron（可展开性诚实化）；左栏标题「工作区」；树顶 ⭐ 最近打开虚拟分组（A7）；
 * - 搜索（REQ-409/528 基础上 REQ-616-aa64 D1 演进）：placeholder 动态化范围提示+命中条目来源
 *   标注（防同名误解）+命中不在当前载荷按 scope 分流跨模块打开（lib/deepLink gotoConversation）+
 *   空结果补跳智能体页引导；
 * - 自绘节点头部 + 子树缩进导轨（替代 antd Collapse），统一行高与间距；会话行仍用 X Conversations（含重命名/删除菜单）；
 * - 整栏可收起为 48px 图标 rail（界面瘦身：沿 Settings/Reference 同款收起基建，记忆 key eino.sidebar.collapsed——
 *   三变体共用、纯视觉收起，不改各模式树语义）。
 */
export default function Sidebar({
  mode,
  agents,
  projects,
  workspaces = [],
  conversations,
  activeAgentId,
  activeProjectId,
  activeWorkspaceId = null,
  onSelectAgent,
  onSelectProject,
  onSelectWorkspace,
  onNewAgent,
  onNewProject,
  onNewWorkspace,
  onNewConversation,
  onConfigureAgent,
  onConfigureProject,
  onConfigureWorkspace,
  onOpenWorkspace,
  loadErr,
  onRetry,
}: {
  mode: 'agent' | 'project' | 'workspace'
  agents: Agent[]
  projects: Project[]
  /** mode=workspace：工作区节点清单（REQ-549）；mode=project：归属 chip 名称解析清单（REQ-614-9fb4） */
  workspaces?: Workspace[]
  conversations: Conversation[]
  activeAgentId: string | null
  activeProjectId: string | null
  activeWorkspaceId?: string | null
  onSelectAgent: (id: string) => void
  onSelectProject: (id: string) => void
  onSelectWorkspace?: (id: string) => void
  onNewAgent: () => void
  onNewProject: () => void
  onNewWorkspace?: () => void
  onNewConversation: (nodeId: string) => void
  onConfigureAgent: (id: string) => void
  onConfigureProject: (id: string) => void
  onConfigureWorkspace?: (id: string) => void
  /** REQ-614-9fb4 R1②：项目行归属 chip 点击 → 工作区模块选中该工作区（121 号 §4.1 互链） */
  onOpenWorkspace?: (wsId: string) => void
  // REQ-515/M322 三态收口：主列表加载失败由页面透传——错误态不再被空态引导冒充
  loadErr?: string | null
  onRetry?: () => void
}) {
  const { currentConvId, setCurrentConv, bumpData, showToast, setPage } = useUI()
  // 界面瘦身：整栏可收起为 48px 图标 rail（沿 REQ-240① Settings/Reference 同款基建），
  // localStorage 记忆（用户偏好跨 agent/project/workspace 三变体共用——纯视觉收起，不改各模式树语义）
  const rail = useSidebarCollapse('eino.sidebar.collapsed')
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [renaming, setRenaming] = useState<Conversation | null>(null)
  // REQ-409/M205：跨会话搜索——标题子串过滤（前端内存过滤）
  // REQ-528/M335：内容级第二档——kw≥3 rune 时 FTS 检索消息正文（防抖 400ms），
  // 命中下拉（会话名+消息摘要），点击跳转对应会话；
  // REQ-616-aa64/M424-aa64 D1（121 号 R3）：hit 补 scope/workspace_id/project_id 判据——
  // 命中会话不在当前模块载荷（REQ-587 工作区侧过滤）时按 scope 分流跨模块打开（评审 gap-2「点了没反应」修复）
  const [kw, setKw] = useState('')
  const kwTrim = kw.trim().toLowerCase()
  const [contentHits, setContentHits] = useState<{ conversation_id: string; message_id: string; role: string; content: string; scope?: string; workspace_id?: string | null; project_id?: string | null }[] | null>(null)
  useEffect(() => {
    const kwRaw = kw.trim()
    if (kwRaw.length < 3) {
      setContentHits(null)
      return
    }
    const t = setTimeout(() => {
      api.searchMessages(kwRaw).then((r) => setContentHits(r.hits)).catch(() => setContentHits(null))
    }, 400)
    return () => clearTimeout(t)
  }, [kw])
  const convNameById = useMemo(() => {
    const m: Record<string, string> = {}
    for (const c of conversations) m[c.id] = c.title
    return m
  }, [conversations])

  // ---- REQ-616-aa64/M424-aa64 A7（121 号 R3）：⭐ 最近打开虚拟分组（项目/工作区两模块树顶）----
  // 共用同一 lib/recent.ts（各自独立键 eino.recent.projects / eino.recent.workspaces——两模块
  // 分组语义分立）；重度项目用户选 target ≤2 跳补偿（120 号评审 misalignment 件）。旧 id 不清洗，
  // 按现存清单过滤后展示（项目删除后自动消失）；pushRecent 派发 eino:recent 供本组件刷新。
  const [recentTick, setRecentTick] = useState(0)
  useEffect(() => {
    const h = () => setRecentTick((t) => t + 1)
    window.addEventListener(RECENT_EVENT, h)
    return () => window.removeEventListener(RECENT_EVENT, h)
  }, [])
  const recentKind: RecentKind | null = mode === 'project' ? 'project' : mode === 'workspace' ? 'workspace' : null
  const recentItems = useMemo<Array<{ id: string; name: string; workspaceId?: string }>>(() => {
    if (!recentKind) return []
    const list: Array<{ id: string; name: string; workspaceId?: string }> = recentKind === 'project' ? projects : workspaces
    const ids = getRecent(recentKind)
    return ids.map((id) => list.find((x) => x.id === id)).filter((x): x is { id: string; name: string; workspaceId?: string } => !!x)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recentKind, projects, workspaces, recentTick])

  // D1：搜索 placeholder 动态化（范围提示——工作区模块载荷按 REQ-587 仅含当前工作区）
  const searchPlaceholder =
    mode === 'agent'
      ? '搜索会话/智能体/消息内容'
      : mode === 'workspace'
        ? '搜索本工作区直挂对话/消息内容（仅当前工作区）'
        : '搜索全部项目与会话/消息内容'

  // D1：命中条目来源标注（防同名误解——同一会话名在不同项目/工作区下重名时定位归属）
  const hitSourceLabel = (h: { scope?: string; workspace_id?: string | null; project_id?: string | null; conversation_id: string }) => {
    const conv = conversations.find((c) => c.id === h.conversation_id)
    const scope = conv?.scope ?? h.scope
    const wsId = conv?.workspace_id ?? h.workspace_id ?? undefined
    const pId = conv?.project_id ?? h.project_id ?? undefined
    if (scope === 'project') {
      const p = projects.find((x) => x.id === pId)
      return p ? `项目《${p.name}》` : '项目会话'
    }
    if (wsId) {
      const w = workspaces.find((x) => x.id === wsId)
      return w ? `工作区《${w.name}》直挂` : '工作区直挂'
    }
    return '智能体页'
  }

  // D1：命中点击——当前载荷内且归属本模块语义 → 直接打开；否则按 scope 分流跨模块
  //（lib/deepLink gotoConversation 写 focus.conv + 容器键 + setPage + goto 事件——R1② 到达侧消费承接）
  const openHit = (h: { conversation_id: string; scope?: string; workspace_id?: string | null; project_id?: string | null }) => {
    setContentHits(null)
    setKw('')
    const conv = conversations.find((c) => c.id === h.conversation_id)
    const inModule =
      conv &&
      ((mode === 'agent' && conv.scope === 'agent') ||
        (mode === 'project' && conv.scope === 'project') ||
        (mode === 'workspace' && conv.scope === 'agent' && (conv.workspace_id ?? '') === (activeWorkspaceId ?? '')))
    if (conv && inModule) {
      openConversation(conv.id)
      return
    }
    if (h.scope === 'agent' || h.scope === 'project') {
      gotoConversation({ id: h.conversation_id, scope: h.scope, workspace_id: h.workspace_id ?? undefined, project_id: h.project_id ?? undefined })
    } else {
      showToast('该会话不在当前视图范围，请到对应模块查找', 'err')
    }
  }

  const nodes = useMemo<TreeNode[]>(() => {
    const byCreated = (a: Conversation, b: Conversation) => b.updated_at.localeCompare(a.updated_at) // REQ-409：最近优先
    const match = (c: Conversation) => !kwTrim || c.title.toLowerCase().includes(kwTrim)
    if (mode === 'agent') {
      // REQ-186：内置助手节点置顶（内置徽标、无配置入口）；用户智能体照常
      return agents
        .slice()
        .sort((x, y) => (y.is_builtin ? 1 : 0) - (x.is_builtin ? 1 : 0))
        .map((a) => ({
          key: a.id,
          name: a.name,
          logo: a.logo_url,
          backend: a.inference_backend,
          builtin: !!a.is_builtin,
          convs: conversations.filter((c) => c.scope === 'agent' && c.agent_id === a.id && match(c)).sort(byCreated),
        }))
        .filter((n) => !kwTrim || n.convs.length > 0 || n.name.toLowerCase().includes(kwTrim))
    }
    if (mode === 'workspace') {
      // REQ-549/M356：工作区树——scope=agent 且归属本工作区的直挂对话（无归属会话不出现在本树）；
      // REQ-613-b6cc：容器节点带 project_count/conv_count 计数（后端 JOIN 出数，非前端聚合）
      return workspaces
        .map((ws) => ({
          key: ws.id,
          name: ws.name,
          container: true,
          projectCount: ws.project_count ?? 0,
          convCount: ws.conv_count ?? 0,
          convs: conversations.filter((c) => c.scope === 'agent' && (c.workspace_id ?? '') === ws.id && match(c)).sort(byCreated),
        }))
        .filter((n) => !kwTrim || n.convs.length > 0 || n.name.toLowerCase().includes(kwTrim))
    }
    return projects
      .map((p) => ({
        key: p.id,
        name: p.name,
        // REQ-614-9fb4 R1②：归属 chip 数据（workspace_id 非空即已归属；名称由 workspaces 清单解析）
        workspaceId: p.workspace_id || undefined,
        convs: conversations.filter((c) => c.scope === 'project' && c.project_id === p.id && match(c)).sort(byCreated),
      }))
      .filter((n) => !kwTrim || n.convs.length > 0 || n.name.toLowerCase().includes(kwTrim))
  }, [mode, agents, projects, workspaces, conversations, kwTrim])

  const activeId = mode === 'agent' ? activeAgentId : mode === 'workspace' ? activeWorkspaceId : activeProjectId

  // 选中节点自动展开
  useEffect(() => {
    if (activeId) setOpen((o) => (o[activeId] ? o : { ...o, [activeId]: true }))
  }, [activeId])

  const selectNode = (id: string) => {
    if (mode === 'agent') onSelectAgent(id)
    else if (mode === 'workspace') {
      pushRecent('workspace', id) // A7：工作区树选中入最近记录
      onSelectWorkspace?.(id)
    } else {
      pushRecent('project', id) // A7：项目树选中入最近记录
      onSelectProject(id)
    }
  }

  // REQ-211：跨智能体直接点选会话——点击会话若归属智能体与当前选中不同，与 setCurrentConv
  // 同批原子切换选中智能体（React 18 自动批处理：AgentsPage 归位守卫 effect 读到的是切换后
  // 的归属，不再清空会话）；项目/工作区模式无跨实体切换语义，直接激活会话。
  const openConversation = (convId: string) => {
    if (mode === 'agent') {
      const conv = conversations.find((c) => c.id === convId)
      if (conv?.agent_id && conv.agent_id !== activeAgentId) onSelectAgent(conv.agent_id)
    }
    setCurrentConv(convId)
  }

  // 点击节点头部：展开/收起；展开时同时选中该节点
  const toggleNode = (id: string) => {
    const next = !open[id]
    setOpen((o) => ({ ...o, [id]: next }))
    if (next) selectNode(id)
  }

  const isAgent = mode === 'agent'
  const isWorkspace = mode === 'workspace'
  const configure = (id: string) => {
    if (isAgent) onConfigureAgent(id)
    else if (isWorkspace) onConfigureWorkspace?.(id)
    else onConfigureProject(id)
  }

  const removeConversation = (id: string) => {
    confirmAction('删除该对话及其全部消息？', '删除后不可恢复。', async () => {
      try {
        await api.deleteConversation(id)
        if (currentConvId === id) setCurrentConv(null)
        bumpData()
      } catch (e: any) {
        showToast(e.message, 'err')
      }
    })
  }

  const renameConversation = async (name: string) => {
    const target = renaming
    setRenaming(null)
    if (!target) return
    try {
      await api.updateConversation(target.id, { ...target, title: name })
      bumpData()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  // 收起态：48px 图标 rail（仅展开钮）——面板收窄由既有 .sidebar-panel:has 兜底规则承接
  if (rail.collapsed) {
    return <CollapsedRail onExpand={rail.toggle} ariaLabel="会话侧栏（已收起）" />
  }

  return (
    <div className="sidebar">
      <div className="side-head">
        <span className="side-title">{isAgent ? '智能体' : isWorkspace ? '工作区' : '项目'}</span>
        <span className="side-count">{nodes.length}</span>
        {/* REQ-326 迭代四：新建入口 + 化并入标题行右端（原整幅虚线钮退役；空态引导保留显式 CTA） */}
        <Button
          type="text"
          size="small"
          className="side-head-add"
          icon={<PlusOutlined />}
          title={`新建${isAgent ? '智能体' : isWorkspace ? '工作区' : '项目'}`}
          aria-label={`新建${isAgent ? '智能体' : isWorkspace ? '工作区' : '项目'}`}
          onClick={isAgent ? onNewAgent : isWorkspace ? (onNewWorkspace ?? (() => {})) : onNewProject}
        />
        {/* 界面瘦身：整栏收起为 48px 图标 rail（同 Settings/Reference 侧栏基建，状态记忆 eino.sidebar.collapsed） */}
        <SidebarCollapseButton onClick={rail.toggle} />
      </div>

      {/* REQ-409/M205：会话搜索（标题子串过滤+节点收敛；内容级检索随 KB FTS 演进）。
          REQ-616-aa64/M424-aa64 D1（121 号 R3）：placeholder 动态化范围提示（两模块各自语义）+
          命中条目来源标注（防同名误解）+ 命中不在当前载荷按 scope 分流跨模块打开（评审 gap-2）+
          空结果补跳智能体页引导 */}
      <div style={{ padding: '0 10px 6px' }}>
        <Input
          size="small"
          allowClear
          placeholder={searchPlaceholder}
          prefix={<SearchOutlined style={{ color: 'var(--c-ink-3, #9aa1bd)' }} />}
          value={kw}
          onChange={(e) => setKw(e.target.value)}
          data-testid="conv-search"
        />
          {/* D1 空结果引导仅双模块（agent 模式本身全量载荷，无「范围外」语义） */}
          {kwTrim && mode !== 'agent' && nodes.length === 0 && !(contentHits && contentHits.length > 0) && (
            <div className="side-node-empty" style={{ margin: '6px 2px 0' }} data-testid="search-empty-guide">
              本模块无命中——
              <a
                onClick={() => setPage('agents')}
                style={{ cursor: 'pointer' }}
                data-testid="search-empty-guide-agents"
              >到智能体页搜索全部会话</a>
            </div>
          )}
        {contentHits && contentHits.length > 0 && (
          <div data-testid="content-hits" style={{ margin: '6px 0 0', maxHeight: 200, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 4 }}>
            {contentHits.slice(0, 8).map((h) => (
              <div
                key={h.message_id}
                style={{ padding: '4px 8px', borderRadius: 6, cursor: 'pointer', background: 'var(--c-bg-soft, #f6f7fb)', fontSize: 11 }}
                onClick={() => openHit(h)}
              >
                <span style={{ color: 'var(--c-ink-3, #9aa1bd)', marginRight: 6 }}>
                  {h.role === 'assistant' ? '结论' : '提问'} · {convNameById[h.conversation_id] ?? '会话'}
                  <span title={`来源：${hitSourceLabel(h)}`} style={{ marginLeft: 4 }}>〔{hitSourceLabel(h)}〕</span>
                </span>
                <span style={{ color: 'var(--c-ink-2, #414a63)' }}>{h.content.slice(0, 60)}{h.content.length > 60 ? '…' : ''}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="conv-list">
        {loadErr ? (
          <LoadErrorAlert
            title={isAgent ? '智能体列表加载失败' : isWorkspace ? '工作区列表加载失败' : '项目列表加载失败'}
            message={loadErr}
            onRetry={onRetry}
            style={{ margin: 12 }}
          />
        ) : (
          nodes.length === 0 && (
            isAgent ? (
            <EmptyGuide
              title="创建第一个智能体"
              steps={[
                '起个名称，一句话描述职责',
                '模型连接可留空',
                '要执行动作就勾选工具',
              ]}
              actionLabel="＋ 新建智能体"
              onAction={onNewAgent}
              footer="创建后点节点上的 ＋ 新建对话。"
            />
          ) : isWorkspace ? (
            // REQ-549/M356：工作区空态引导（领域容器——默认值免每会话重选）
            // REQ-616-aa64/M424-aa64 D2（121 号 R3）：空态互链提示——提及对方模块
            // REQ-617-87d8/M425-87d8 R4（121 号 §4.4）：footer 升正式关系句（单源 lib/relations——互链语义由句内两概念并提承接）
            <EmptyGuide
              title="创建第一个工作区"
              steps={[
                '起个名称，一句话描述场景',
                '可选设默认知识库/方案/智能体',
                '新会话自动继承',
              ]}
              actionLabel="＋ 新建工作区"
              onAction={onNewWorkspace ?? (() => {})}
              footer={RELATION_SENTENCE}
            />
          ) : (
            // REQ-616-aa64/M424-aa64 D2（121 号 R3）：空态互链提示——提及对方模块
            // REQ-617-87d8/M425-87d8 R4（121 号 §4.4）：footer 升正式关系句（单源 lib/relations）
            <EmptyGuide
              title="创建第一个项目"
              steps={[
                '起个名称，选择协作模式',
                '可绑本地目录，产物与 Git 可见',
                '配置成员与主智能体',
              ]}
              actionLabel="＋ 新建项目"
              onAction={onNewProject}
              footer={RELATION_SENTENCE}
            />
          )
          )
        )}

        {/* REQ-616-aa64/M424-aa64 A7（121 号 R3）：⭐ 最近打开虚拟分组（项目/工作区两模块树顶；
            重度项目用户 ≤2 跳补偿——120 号评审 misalignment 件；智能体页不设） */}
        {recentItems.length > 0 && (
          <div className="side-recent" data-testid={`recent-group-${recentKind}`}>
            <div className="side-recent-title">⭐ 最近打开</div>
            {recentItems.map((r) => {
              const wsId = recentKind === 'project' ? (r as { workspaceId?: string }).workspaceId : undefined
              const wsName = wsId ? workspaces.find((w) => w.id === wsId)?.name : undefined
              return (
                <button
                  key={r.id}
                  type="button"
                  className="side-recent-row"
                  data-testid={`recent-row-${r.id}`}
                  title={`打开${recentKind === 'project' ? '项目' : '工作区'}「${r.name}」`}
                  onClick={() => selectNode(r.id)}
                >
                  <span className="side-recent-name">{r.name || (recentKind === 'project' ? '未命名项目' : '未命名工作区')}</span>
                  {wsName && <span className="proj-ws-chip" title={`已归属工作区「${wsName}」`}>▦ {wsName}</span>}
                </button>
              )
            })}
          </div>
        )}

        {nodes.map((n) => {
          // REQ-613-b6cc/M421-b6cc chevron 诚实化（121 号 R1① A4 工作区侧先行）：空壳节点
          // （无直挂对话且 conv_count=0）不渲染箭头、只显名称+徽标——可展开性如实。
          // agent/project 模式维持现状（A4 两模块同规剩余面不在 R3 批十件清单——121 号 §6，留后续）。
          const hasSub = mode !== 'workspace' || n.convs.length > 0 || (n.convCount ?? 0) > 0
          const expanded = hasSub && !!open[n.key]
          const active = activeId === n.key
          return (
            <div key={n.key} className={`side-node${active ? ' active' : ''}`}>
              <div className="side-node-row">
                <button
                  type="button"
                  className="side-node-head"
                  aria-expanded={hasSub ? expanded : undefined}
                  onClick={() => (hasSub ? toggleNode(n.key) : selectNode(n.key))}
                >
                  {hasSub ? (
                    <span className="side-node-chev">{expanded ? <DownOutlined /> : <RightOutlined />}</span>
                  ) : null}
                  {n.container && (
                    <span className="side-node-ico" aria-hidden="true"><AppstoreOutlined /></span>
                  )}
                  <span className="side-node-name" title={n.name}>{n.name || (mode === 'workspace' ? '未命名工作区' : mode === 'project' ? '未命名项目' : '未命名智能体')}</span>
                  {n.container && (
                    <span className="side-node-badge" data-testid={`ws-badge-${n.key}`} title={`名下项目 ${n.projectCount ?? 0} · 对话 ${n.convCount ?? 0}（直挂∪归属项目会话）`}>
                      {/* REQ-618-36a1/M426-36a1 徽标紧凑化：「N 项目 · M 对话」文字→图标数字芯片（栏宽收窄配套；完整口径在 title） */}
                      <span className="side-badge-item"><FolderOutlined /> {n.projectCount ?? 0}</span>
                      <span className="side-badge-item"><MessageOutlined /> {n.convCount ?? 0}</span>
                    </span>
                  )}
                  {/* REQ-488：彩色 Tag 灰字化——内置标注为中性灰字（非状态色） */}
                  {n.builtin && <span style={{ margin: 0, fontSize: 11, lineHeight: '16px', color: 'var(--c-ink-3)' }}>内置</span>}
                  {/* REQ-336：会话数裸数字退役（迭代四后行内常驻元素只剩它，孤数怪异；展开子树即见会话清单） */}
                </button>
                {/* REQ-614-9fb4 R1②：已归属项目行「⌂ 工作区《X》」chip（121 号 §4.1 项目→工作区互链；
                    REQ-577 徽标资产沿用灰字轻量语言——独立按钮 stopPropagation 不触发行选中/展开） */}
                {mode === 'project' && (() => {
                  const wsId = n.workspaceId
                  if (!wsId) return null
                  const wsName = workspaces.find((w) => w.id === wsId)?.name ?? wsId
                  return (
                    <button
                      type="button"
                      className="proj-ws-chip"
                      data-testid={`proj-ws-chip-${n.key}`}
                      title={`已归属工作区「${wsName}」——点击到工作区模块查看`}
                      onClick={(e) => { e.stopPropagation(); onOpenWorkspace?.(wsId) }}
                    >
                      ▦ 工作区《{wsName}》
                    </button>
                  )
                })()}
                {/* REQ-326 迭代四：行内操作悬停显现＋齿轮改三点；配置入口=三点钮（REQ-213 内置助手同入口归一不变） */}
                <span className="side-node-ops">
                  <Button type="text" size="small" icon={<PlusOutlined />} title={n.builtin ? '与平台助手开始对话' : '新建对话'} onClick={() => onNewConversation(n.key)} />
                  <Button type="text" size="small" icon={<EllipsisOutlined />} title={n.builtin ? '配置平台助手' : '配置'} onClick={() => configure(n.key)} />
                </span>
              </div>

              {expanded && (
                <div className="side-node-children">
                  {n.convs.length === 0 ? (
                    (n.convCount ?? 0) > 0 ? (
                      // REQ-613-b6cc：非选中工作区载荷按 REQ-587 过滤不含其对话——计数徽标诚实提示载入路径
                      <div className="side-node-empty">本工作区共 {n.convCount} 个对话——点名称选中载入</div>
                    ) : (
                      <div className="side-node-empty">暂无对话 · 点 ＋ 新建</div>
                    )
                  ) : (
                    <Conversations
                      items={n.convs.map((c) => ({
                        key: c.id,
                        // REQ-549 双入口共存标注 → REQ-616-aa64/M424-aa64 C2（121 号 R3）：智能体页
                        // 直挂对话「· 工作区」4 字灰字升级为可点击 chip「▦ 工作区《名》」——点击经
                        // onOpenWorkspace 跳工作区模块选中该工作区（stopPropagation 不触发行选中）
                        label:
                          mode === 'agent' && c.workspace_id ? (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, minWidth: 0 }}>
                              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.title || '未命名对话'}</span>
                              <button
                                type="button"
                                className="proj-ws-chip"
                                data-testid={`agent-ws-chip-${c.id}`}
                                title={`归属工作区「${workspaces.find((w) => w.id === c.workspace_id)?.name ?? c.workspace_id}」——点击到工作区模块查看`}
                                onClick={(e) => { e.stopPropagation(); onOpenWorkspace?.(c.workspace_id!) }}
                              >
                                ▦ 工作区《{workspaces.find((w) => w.id === c.workspace_id)?.name ?? c.workspace_id}》
                              </button>
                            </span>
                          ) : (
                            (c.title || '未命名对话')
                          ),
                      }))}
                      activeKey={currentConvId ?? undefined}
                      onActiveChange={openConversation}
                      menu={(c) => ({
                        items: [
                          { key: 'rename', label: '重命名', icon: <EditOutlined /> },
                          { key: 'delete', label: '删除', icon: <DeleteOutlined />, danger: true },
                        ],
                        onClick: ({ key }) => {
                          const conv = n.convs.find((x) => x.id === c.key)
                          if (!conv) return
                          if (key === 'rename') setRenaming(conv)
                          else if (key === 'delete') removeConversation(conv.id)
                        },
                      })}
                    />
                  )}
                </div>
              )}
            </div>
          )
        })}

        {nodes.length > 0 && <div className="empty-hint">点 ＋ 新建对话</div>}
      </div>

      {renaming && (
        <NameModal
          open
          title="重命名对话"
          placeholder="对话名称"
          okText="保存"
          onCancel={() => setRenaming(null)}
          onSubmit={renameConversation}
        />
      )}
    </div>
  )
}
