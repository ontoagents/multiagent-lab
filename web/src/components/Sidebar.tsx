import { useEffect, useMemo, useState } from 'react'
import { Button, Input } from 'antd'
import {
  PlusOutlined,
  EllipsisOutlined,
  DeleteOutlined,
  EditOutlined,
  DownOutlined,
  RightOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import { Conversations } from '@ant-design/x'
import { api } from '../api/client'
import EmptyGuide from './EmptyGuide'
import LoadErrorAlert from './LoadErrorAlert'
import type { Agent, Conversation, Project } from '../api/types'
import { useUI } from '../store/ui'
import { confirmAction } from '../lib/antd'
import NameModal from './NameModal'

interface TreeNode {
  key: string
  name: string
  logo?: string
  backend?: string
  builtin?: boolean // REQ-186：内置助手（置顶分区、内置徽标、无配置入口）
  convs: Conversation[]
}

/**
 * 左栏树（原型 06 §3.1 / §3.2）：
 * - mode=agent：智能体节点（头像点 + 会话数轻量数字 + 常驻「＋新对话」「⚙配置」），其下挂该智能体的对话；
 * - mode=project：项目节点同样结构，其下挂项目对话；
 * - 自绘节点头部 + 子树缩进导轨（替代 antd Collapse），统一行高与间距；会话行仍用 X Conversations（含重命名/删除菜单）。
 */
export default function Sidebar({
  mode,
  agents,
  projects,
  conversations,
  activeAgentId,
  activeProjectId,
  onSelectAgent,
  onSelectProject,
  onNewAgent,
  onNewProject,
  onNewConversation,
  onConfigureAgent,
  onConfigureProject,
  loadErr,
  onRetry,
}: {
  mode: 'agent' | 'project'
  agents: Agent[]
  projects: Project[]
  conversations: Conversation[]
  activeAgentId: string | null
  activeProjectId: string | null
  onSelectAgent: (id: string) => void
  onSelectProject: (id: string) => void
  onNewAgent: () => void
  onNewProject: () => void
  onNewConversation: (nodeId: string) => void
  onConfigureAgent: (id: string) => void
  onConfigureProject: (id: string) => void
  // REQ-515/M322 三态收口：主列表加载失败由页面透传——错误态不再被空态引导冒充
  loadErr?: string | null
  onRetry?: () => void
}) {
  const { currentConvId, setCurrentConv, bumpData, showToast } = useUI()
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [renaming, setRenaming] = useState<Conversation | null>(null)
  // REQ-409/M205：跨会话搜索——标题子串过滤（前端内存过滤）
  // REQ-528/M335：内容级第二档——kw≥3 rune 时 FTS 检索消息正文（防抖 400ms），
  // 命中下拉（会话名+消息摘要），点击跳转对应会话
  const [kw, setKw] = useState('')
  const kwTrim = kw.trim().toLowerCase()
  const [contentHits, setContentHits] = useState<{ conversation_id: string; message_id: string; role: string; content: string }[] | null>(null)
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
    return projects
      .map((p) => ({
        key: p.id,
        name: p.name,
        convs: conversations.filter((c) => c.scope === 'project' && c.project_id === p.id && match(c)).sort(byCreated),
      }))
      .filter((n) => !kwTrim || n.convs.length > 0 || n.name.toLowerCase().includes(kwTrim))
  }, [mode, agents, projects, conversations, kwTrim])

  const activeId = mode === 'agent' ? activeAgentId : activeProjectId

  // 选中节点自动展开
  useEffect(() => {
    if (activeId) setOpen((o) => (o[activeId] ? o : { ...o, [activeId]: true }))
  }, [activeId])

  const selectNode = (id: string) => {
    if (mode === 'agent') onSelectAgent(id)
    else onSelectProject(id)
  }

  // REQ-211：跨智能体直接点选会话——点击会话若归属智能体与当前选中不同，与 setCurrentConv
  // 同批原子切换选中智能体（React 18 自动批处理：AgentsPage 归位守卫 effect 读到的是切换后
  // 的归属，不再清空会话）；项目模式无跨实体切换语义，直接激活会话。
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
  const configure = (id: string) => {
    if (isAgent) onConfigureAgent(id)
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

  return (
    <div className="sidebar">
      <div className="side-head">
        <span className="side-title">{isAgent ? '智能体' : '项目'}</span>
        <span className="side-count">{nodes.length}</span>
        {/* REQ-326 迭代四：新建入口 + 化并入标题行右端（原整幅虚线钮退役；空态引导保留显式 CTA） */}
        <Button
          type="text"
          size="small"
          className="side-head-add"
          icon={<PlusOutlined />}
          title={`新建${isAgent ? '智能体' : '项目'}`}
          aria-label={`新建${isAgent ? '智能体' : '项目'}`}
          onClick={isAgent ? onNewAgent : onNewProject}
        />
      </div>

      {/* REQ-409/M205：会话搜索（标题子串过滤+节点收敛；内容级检索随 KB FTS 演进） */}
      <div style={{ padding: '0 10px 6px' }}>
        <Input
          size="small"
          allowClear
          placeholder="搜索会话/智能体/消息内容"
          prefix={<SearchOutlined style={{ color: 'var(--c-ink-3, #9aa1bd)' }} />}
          value={kw}
          onChange={(e) => setKw(e.target.value)}
          data-testid="conv-search"
        />
        {contentHits && contentHits.length > 0 && (
          <div data-testid="content-hits" style={{ margin: '6px 0 0', maxHeight: 200, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 4 }}>
            {contentHits.slice(0, 8).map((h) => (
              <div
                key={h.message_id}
                style={{ padding: '4px 8px', borderRadius: 6, cursor: 'pointer', background: 'var(--c-bg-soft, #f6f7fb)', fontSize: 11 }}
                onClick={() => {
                  setCurrentConv(h.conversation_id)
                  setContentHits(null)
                  setKw('')
                }}
              >
                <span style={{ color: 'var(--c-ink-3, #9aa1bd)', marginRight: 6 }}>{h.role === 'assistant' ? '结论' : '提问'} · {convNameById[h.conversation_id] ?? '会话'}</span>
                <span style={{ color: 'var(--c-ink-2, #414a63)' }}>{h.content.slice(0, 60)}{h.content.length > 60 ? '…' : ''}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="conv-list">
        {loadErr ? (
          <LoadErrorAlert
            title={isAgent ? '智能体列表加载失败' : '项目列表加载失败'}
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
                '模型连接可留空（跟随全局默认）',
                '需要执行动作就勾选工具',
              ]}
              actionLabel="＋ 新建智能体"
              onAction={onNewAgent}
              footer="创建后点节点上的 ＋ 新建对话即可开始。"
            />
          ) : (
            <EmptyGuide
              title="创建第一个项目"
              steps={[
                '起个名称，选择协作模式',
                '可绑定本地目录（产物写入，文件/Git 可见）',
                '配置成员与主智能体',
              ]}
              actionLabel="＋ 新建项目"
              onAction={onNewProject}
              footer="项目会话由成员智能体协作处理。"
            />
          )
          )
        )}

        {nodes.map((n) => {
          const expanded = !!open[n.key]
          const active = activeId === n.key
          return (
            <div key={n.key} className={`side-node${active ? ' active' : ''}`}>
              <div className="side-node-row">
                <button
                  type="button"
                  className="side-node-head"
                  aria-expanded={expanded}
                  onClick={() => toggleNode(n.key)}
                >
                  <span className="side-node-chev">{expanded ? <DownOutlined /> : <RightOutlined />}</span>
                  <span className="side-node-name" title={n.name}>{n.name || (mode === 'project' ? '未命名项目' : '未命名智能体')}</span>
                  {/* REQ-488：彩色 Tag 灰字化——内置标注为中性灰字（非状态色） */}
                  {n.builtin && <span style={{ margin: 0, fontSize: 11, lineHeight: '16px', color: 'var(--c-ink-3)' }}>内置</span>}
                  {/* REQ-336：会话数裸数字退役（迭代四后行内常驻元素只剩它，孤数怪异；展开子树即见会话清单） */}
                </button>
                {/* REQ-326 迭代四：行内操作悬停显现＋齿轮改三点；配置入口=三点钮（REQ-213 内置助手同入口归一不变） */}
                <span className="side-node-ops">
                  <Button type="text" size="small" icon={<PlusOutlined />} title={n.builtin ? '与平台助手开始对话' : '新建对话'} onClick={() => onNewConversation(n.key)} />
                  <Button type="text" size="small" icon={<EllipsisOutlined />} title={n.builtin ? '配置平台助手' : '配置'} onClick={() => configure(n.key)} />
                </span>
              </div>

              {expanded && (
                <div className="side-node-children">
                  {n.convs.length === 0 ? (
                    <div className="side-node-empty">暂无对话 · 点 ＋ 新建</div>
                  ) : (
                    <Conversations
                      items={n.convs.map((c) => ({ key: c.id, label: c.title || '未命名对话' }))}
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

        {nodes.length > 0 && <div className="empty-hint">展开节点后点 ＋ 新建对话</div>}
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
