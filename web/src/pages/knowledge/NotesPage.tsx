import { useEffect, useState } from 'react'
import { Button, Empty, Input, Modal, Popconfirm, Select, Space, Tag, Tooltip, Typography } from 'antd'
import { FileAddOutlined, PlusOutlined, ReadOutlined, SyncOutlined } from '@ant-design/icons'
import XMarkdown from '@ant-design/x-markdown'
import { api } from '../../api/client'
import type { KnowledgeBase, Notebook, NotePage } from '../../api/types'
import { useUI } from '../../store/ui'

// ---------------------------------------------------------------------------
// REQ-495/M304 笔记本 Notes（自产成果文档资产层——Q-23 定位「自产成果（沉淀）」载体）。
// IA=知识库姊妹页签（三拍板①：外源/自产对仗，升格独立模块沿 REQ-180 先例留触发）；
// 绑定=工作区一对多（三拍板②+Q-24 实体反转宿主端=REQ-548 workspace 实体）；
// 源=MVP 两源（本地目录+git 本地克隆路径 pull+walk，mtime 触碰同步跳过未变页）；
// KB 回流=引用语义（三拍板③：KB 登记笔记本为语料源、笔记本单源不拷贝，页触碰自动重索引——
// 文档标题确定性前缀【笔记本】{pageID 前 8 位}）；编辑渲染沿 chat-md 同构。
// 基底 2da4421（REQ-548 M1a workspace 实体先行——notebook.workspace_id 绑定零返工）。
// ---------------------------------------------------------------------------

export default function NotesPage() {
  const { showToast } = useUI()
  const [notebooks, setNotebooks] = useState<Notebook[] | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [pages, setPages] = useState<NotePage[]>([])
  const [editing, setEditing] = useState<NotePage | null>(null)
  const [previewing, setPreviewing] = useState<NotePage | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [newDesc, setNewDesc] = useState('')
  const [newWS, setNewWS] = useState<string | undefined>(undefined)
  const [workspaces, setWorkspaces] = useState<{ id: string; name: string }[]>([])
  const [syncPath, setSyncPath] = useState('')
  const [syncKind, setSyncKind] = useState<'local' | 'git'>('local')
  const [syncing, setSyncing] = useState(false)
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [kbSources, setKbSources] = useState<string[]>([])
  const [kbPick, setKbPick] = useState<string | undefined>(undefined)
  const [busy, setBusy] = useState(false)

  const active = notebooks?.find((n) => n.id === activeId) ?? null

  const loadNotebooks = () => {
    api.listNotebooks()
      .then((ns) => setNotebooks(ns ?? []))
      .catch((e) => { setNotebooks([]); showToast(e?.message ?? '笔记本列表加载失败', 'err') })
  }
  const loadPages = (id: string) => {
    api.listNotePages(id).then(setPages).catch(() => setPages([]))
    api.listNotebookSources(id).then((rs) => setKbSources((rs ?? []).map((r) => r.kb_id))).catch(() => setKbSources([]))
  }
  useEffect(() => { loadNotebooks() }, [])
  useEffect(() => { api.listWorkspaces().then((ws) => setWorkspaces(ws ?? [])).catch(() => setWorkspaces([])) }, [])
  useEffect(() => { api.listKBs().then(setKbs).catch(() => setKbs([])) }, [])
  useEffect(() => { if (activeId) loadPages(activeId) }, [activeId])

  const openNotebook = (id: string) => {
    setActiveId(id)
    setEditing(null)
    setPreviewing(null)
  }

  const createNotebook = async () => {
    if (!newName.trim()) { showToast('请输入名称', 'err'); return }
    setBusy(true)
    try {
      const n = await api.createNotebook({ name: newName.trim(), description: newDesc.trim(), workspace_id: newWS || undefined })
      showToast('笔记本已创建')
      setCreating(false)
      setNewName(''); setNewDesc(''); setNewWS(undefined)
      loadNotebooks()
      openNotebook(n.id)
    } catch (e: any) {
      showToast(e?.message ?? '创建失败', 'err')
    } finally { setBusy(false) }
  }

  const savePage = async (p: NotePage) => {
    setBusy(true)
    try {
      const saved = p.id
        ? await api.updateNotePage(p.id, { title: p.title, content_md: p.content_md })
        : await api.createNotePage(p.notebook_id!, { title: p.title, content_md: p.content_md })
      showToast('已保存（引用 KB 将随触碰自动同步）')
      setEditing(null)
      loadPages(p.notebook_id!)
      setPreviewing(saved)
    } catch (e: any) {
      showToast(e?.message ?? '保存失败', 'err')
    } finally { setBusy(false) }
  }

  const removePage = async (p: NotePage) => {
    try {
      await api.deleteNotePage(p.id)
      showToast('页面已删除（引用 KB 语料同步移除）')
      loadPages(p.notebook_id!)
      setPreviewing(null)
    } catch (e: any) { showToast(e?.message ?? '删除失败', 'err') }
  }

  const doSync = async () => {
    if (!activeId || !syncPath.trim()) { showToast('请输入源路径（本地目录或 git 本地克隆路径）', 'err'); return }
    setSyncing(true)
    try {
      const r = await api.syncNotebook(activeId, { path: syncPath.trim(), kind: syncKind })
      showToast(`同步完成：新增 ${r.added} · 更新 ${r.updated} · 跳过 ${r.skipped} · 移除 ${r.removed}`)
      loadPages(activeId)
    } catch (e: any) {
      showToast(e?.message ?? '同步失败', 'err')
    } finally { setSyncing(false) }
  }

  const registerKB = async () => {
    if (!activeId || !kbPick) { showToast('请选择要登记的知识库', 'err'); return }
    try {
      await api.createNotebookSource(activeId, kbPick)
      showToast('已登记引用——既有页全量回流，此后增量随触碰同步')
      loadPages(activeId)
      setKbPick(undefined)
    } catch (e: any) { showToast(e?.message ?? '登记失败', 'err') }
  }

  const unregisterKB = async (kbID: string) => {
    if (!activeId) return
    try {
      await api.deleteNotebookSource(activeId, kbID)
      showToast('已解除登记（KB 内已回流文档保留，不再随触碰同步）')
      loadPages(activeId)
    } catch (e: any) { showToast(e?.message ?? '解除失败', 'err') }
  }

  const removeNotebook = async (n: Notebook) => {
    try {
      await api.deleteNotebook(n.id)
      showToast('笔记本已删除（页面级联；引用 KB 语料同步移除）')
      if (activeId === n.id) { setActiveId(null); setPages([]) }
      loadNotebooks()
    } catch (e: any) { showToast(e?.message ?? '删除失败', 'err') }
  }

  return (
    <div style={{ display: 'flex', gap: 12, alignItems: 'stretch', minHeight: 480 }}>
      {/* 左栏：笔记本列表（容器一级） */}
      <div style={{ width: 260, flexShrink: 0, borderInlineEnd: '1px solid var(--c-line, #e6e8f0)', paddingInlineEnd: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
          <Typography.Text strong style={{ fontSize: 13 }}>笔记本</Typography.Text>
          <Button size="small" type="text" icon={<PlusOutlined />} aria-label="新建笔记本" onClick={() => setCreating(true)} data-testid="notebook-new" />
        </div>
        {notebooks === null ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载中…</Typography.Text>
        ) : notebooks.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有笔记本——自产成果的沉淀层：写页面、同步目录/git 源、登记引用回流知识库" />
        ) : (
          notebooks.map((n) => (
            <div
              key={n.id}
              onClick={() => openNotebook(n.id)}
              style={{ padding: '7px 9px', borderRadius: 8, marginBottom: 2, cursor: 'pointer', background: activeId === n.id ? 'var(--c-hover-soft, #eceef5)' : 'none' }}
              data-testid="notebook-item"
            >
              <Typography.Text strong style={{ fontSize: 13 }}>{n.name}</Typography.Text>
              <div style={{ fontSize: 11, color: 'var(--c-ink-3)' }}>
                {n.workspace_id ? '已绑工作区' : '个人资产'} · {n.description || '未填写描述'}
              </div>
            </div>
          ))
        )}
      </div>

      {/* 右侧：详情（页面列表/编辑/渲染+源同步+回流登记） */}
      {active ? (
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
            <Typography.Title level={5} style={{ margin: 0 }}>{active.name}</Typography.Title>
            {active.workspace_id && <Tag style={{ margin: 0, fontSize: 11 }}>已绑工作区</Tag>}
            <span style={{ flex: 1 }} />
            <Popconfirm title={`删除笔记本「${active.name}」？`} description="页面级联删除；已回流 KB 的语料文档同步移除。" okText="删除" okButtonProps={{ danger: true }} onConfirm={() => removeNotebook(active)}>
              <Button size="small" type="text" danger aria-label="删除笔记本">删除笔记本</Button>
            </Popconfirm>
          </div>

          {/* 源同步（MVP 两源） */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 10 }} data-testid="note-sync">
            <Select value={syncKind} onChange={(v) => setSyncKind(v)} size="small" style={{ width: 120 }} options={[{ value: 'local', label: '本地目录源' }, { value: 'git', label: 'git 仓库源' }]} />
            <Input size="small" style={{ width: 320 }} value={syncPath} onChange={(e) => setSyncPath(e.target.value)} placeholder={syncKind === 'git' ? '本地克隆路径（同步时 git pull --ff-only 后扫 .md）' : '含 .md 的目录路径（触碰同步：mtime 未变跳过）'} />
            <Button size="small" icon={<SyncOutlined />} loading={syncing} onClick={doSync} data-testid="note-sync-btn">同步</Button>
            <Typography.Text type="secondary" style={{ fontSize: 11 }}>源页只读单向同步（笔记本单源不回写源）；手写页不受同步影响</Typography.Text>
          </div>

          {/* KB 回流登记（引用语义） */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 10 }} data-testid="note-kbflow">
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>回流引用：</Typography.Text>
            {kbSources.length === 0 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>未登记——登记后页触碰自动重索引该库</Typography.Text>}
            {kbSources.map((id) => (
              <Tag key={id} style={{ margin: 0, fontSize: 11 }} closable onClose={() => unregisterKB(id)}>
                {(kbs.find((k) => k.id === id)?.name ?? id).slice(0, 24)}（引用）
              </Tag>
            ))}
            <Select size="small" style={{ width: 200 }} placeholder="登记到知识库…" value={kbPick} onChange={setKbPick} options={kbs.filter((k) => !kbSources.includes(k.id)).map((k) => ({ value: k.id, label: k.name }))} />
            <Button size="small" onClick={registerKB} disabled={!kbPick} data-testid="note-kbflow-add">登记引用</Button>
          </div>

          {/* 页面列表（两级树第二级） */}
          <div style={{ display: 'flex', gap: 12 }}>
            <div style={{ width: 240, flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>页面 {pages.length}</Typography.Text>
                <Button size="small" type="text" icon={<FileAddOutlined />} aria-label="新建页面" onClick={() => setEditing({ notebook_id: active.id, title: '', content_md: '', source: 'manual', source_ref: '', source_mtime: '' } as NotePage)} data-testid="note-page-new" />
              </div>
              {pages.map((p) => (
                <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 4, padding: '4px 6px', borderRadius: 6, cursor: 'pointer', background: previewing?.id === p.id || editing?.id === p.id ? 'var(--c-hover-soft, #eceef5)' : 'none' }} onClick={() => { setPreviewing(p); setEditing(null) }} data-testid="note-page-item">
                  <ReadOutlined style={{ color: 'var(--c-ink-3)', fontSize: 12 }} />
                  <Typography.Text ellipsis style={{ fontSize: 12.5, flex: 1 }}>{p.title}</Typography.Text>
                  {p.source !== 'manual' && <Tooltip title={`源：${p.source} · ${p.source_ref}`}><Tag style={{ margin: 0, fontSize: 10 }}>{p.source}</Tag></Tooltip>}
                </div>
              ))}
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              {editing ? (
                <div data-testid="note-editor">
                  <Space style={{ marginBottom: 6 }} size={6}>
                    <Input style={{ width: 320 }} value={editing.title} onChange={(e) => setEditing({ ...editing, title: e.target.value })} placeholder="页面标题" />
                    <Button size="small" type="primary" loading={busy} onClick={() => savePage(editing)} disabled={!editing.title.trim()}>保存</Button>
                    <Button size="small" onClick={() => setEditing(null)}>取消</Button>
                  </Space>
                  <Input.TextArea rows={16} value={editing.content_md} onChange={(e) => setEditing({ ...editing, content_md: e.target.value })} placeholder="markdown 正文…" style={{ fontFamily: 'monospace' }} />
                </div>
              ) : previewing ? (
                <div data-testid="note-preview">
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                    <Typography.Title level={5} style={{ margin: 0 }}>{previewing.title}</Typography.Title>
                    {previewing.source !== 'manual' && <Tooltip title={`源：${previewing.source} · ${previewing.source_ref}（源页单向同步）`}><Tag style={{ margin: 0, fontSize: 10 }}>{previewing.source}</Tag></Tooltip>}
                    <span style={{ flex: 1 }} />
                    {previewing.source === 'manual' && <Button size="small" type="text" aria-label="编辑页面" onClick={() => setEditing(previewing)}>编辑</Button>}
                    <Popconfirm title={`删除页面「${previewing.title}」？`} okText="删除" okButtonProps={{ danger: true }} onConfirm={() => removePage(previewing)}>
                      <Button size="small" type="text" danger aria-label="删除页面">删除</Button>
                    </Popconfirm>
                  </div>
                  <div data-testid="note-preview-body">
                    {/* chat-md 挂 XMarkdown 本体（.chat-md.x-markdown 15px/1.75——与 ChatWindow 同构） */}
                    <XMarkdown className="chat-md" content={previewing.content_md || '（空页面）'} openLinksInNewTab />
                  </div>
                </div>
              ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择左侧页面，或新建/同步生成" />
              )}
            </div>
          </div>
        </div>
      ) : (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="notes-empty-quiet">
            在左侧选择笔记本，或新建一本——笔记本是自产成果的沉淀层（四类资产：知识库=外源输入 / 工作区=过程 / 笔记本=自产沉淀 / 本体=结构化终态）。
          </Typography.Text>
        </div>
      )}

      {/* 新建笔记本弹窗（绑定工作区一对多——可空=个人资产） */}
      <Modal
        open={creating}
        title="新建笔记本"
        onCancel={() => setCreating(false)}
        onOk={createNotebook}
        okText="创建"
        okButtonProps={{ loading: busy }}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={8}>
          <Input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="名称（如：Eino 学习笔记）" />
          <Input value={newDesc} onChange={(e) => setNewDesc(e.target.value)} placeholder="描述（可选）" />
          <Select
            allowClear
            style={{ width: '100%' }}
            value={newWS}
            onChange={setNewWS}
            placeholder="绑定工作区（可选——一对多；不绑=个人资产）"
            options={workspaces.map((w) => ({ value: w.id, label: w.name }))}
          />
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            绑定后该笔记本作为工作区成果输出端（M1a workspace 实体一对多）；引用回流：知识库可登记本笔记本为语料源（引用语义+触碰同步）。
          </Typography.Text>
        </Space>
      </Modal>
    </div>
  )
}
