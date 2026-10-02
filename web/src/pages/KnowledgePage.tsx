import { useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Select, Checkbox, Space, Splitter, Tag, Result } from 'antd'
import { BookOutlined, ApartmentOutlined, FileSearchOutlined, PlusOutlined } from '@ant-design/icons'
import type { KnowledgeBase } from '../api/types'
import { api } from '../api/client'
import { useUI } from '../store/ui'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import { CollapsedRail, SidebarCollapseButton, useSidebarCollapse } from '../lib/sidebar'
import { modeOf, type KBMode } from './knowledge/shared'
import RagPage from './knowledge/RagPage'
import GraphRagPage from './knowledge/GraphRagPage'
import WikiPage from './knowledge/WikiPage'

// ---------------------------------------------------------------------------
// 知识库模块壳（REQ-258，开发者指令「三种类型改为类似本体模块的左侧边栏标题形式，
// 每种类型的页面都不一样，实现和显示各自独立」）：左侧边栏 = 三类型导航项
// （图标+标题+一句话描述+库计数徽标，本体五栏同款范式）；右侧内容 = 各类型独立页面
// （RagPage / GraphRagPage / WikiPage，各自实现库列表与详情，互不共享类型专属逻辑）。
// 旧实现（单页 + 左栏 Tabs 切类型）退役：类型页内列表替代原 modeTab 过滤列表。
// ---------------------------------------------------------------------------

type KbView = KBMode

const NAV: { key: KbView; label: string; icon: React.ReactNode; desc: string }[] = [
  { key: 'rag', label: 'RAG 检索', icon: <FileSearchOutlined />, desc: '向量检索 · 文档接入 · 召回试运行' },
  { key: 'graphrag', label: 'GraphRAG', icon: <ApartmentOutlined />, desc: '自研 KG 抽取 · 图谱 · 全局问答' },
  { key: 'wiki', label: 'LLM Wiki', icon: <BookOutlined />, desc: 'LLM 写时合成互链页面 · 零 embedding' },
]

const VIEW_KEY = 'eino.kb.view'

function readView(): KbView {
  const v = localStorage.getItem(VIEW_KEY)
  return v === 'graphrag' || v === 'wiki' ? v : 'rag'
}

/** 新建知识库：name / description / mode / store_backend（后端无状态字段，索引状态在文档级） */
function CreateKBModal({ defaultMode, onClose, onCreated }: { defaultMode: KBMode; onClose: () => void; onCreated: (kb: KnowledgeBase) => void }) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [busy, setBusy] = useState(false)

  const mode = Form.useWatch('mode', form) ?? 'rag'
  const save = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setBusy(true)
    try {
      const kb = await api.createKB({
        name: v.name, description: v.description ?? '', mode: v.mode, store_backend: v.store_backend,
        kb_vector: v.kb_vector ?? false, kb_graph: v.kb_graph ?? false, // KB-11：能力开关（全关由后端按 mode 派生）
      })
      showToast('知识库已创建')
      onCreated(kb)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      centered
      title="新建知识库"
      width={520}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={busy} onClick={save}>
            创建
          </Button>
        </Space>
      }
    >
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={{ store_backend: 'qdrant', description: '', mode: defaultMode, kb_vector: defaultMode === 'rag', kb_graph: defaultMode === 'graphrag' }}
      >
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]}>
          <Input placeholder="如：K8s 运维手册" maxLength={60} />
        </Form.Item>
        <Form.Item
          name="mode"
          label="知识库类型（M14 双子模块 + REQ-241 第三类型）"
          extra={
            mode === 'graphrag'
              ? 'GraphRAG：chunks 额外抽取为自存 KG（D-O15 自研，零外部进程）；KG 无命中自动回退向量检索。'
              : mode === 'wiki'
                ? 'LLM Wiki（REQ-241）：LLM 摄取生成互链 Markdown 页面层（写时合成），检索读页非读片段，零 embedding 依赖；重建按文档数产生 LLM 调用。'
                : 'RAG：向量检索（默认）。GraphRAG 模式额外构建 KG，wiki 模式为写时合成页面层，三类型并存可对照。'
          }
        >
          <Select
            options={[
              { value: 'rag', label: 'RAG（向量检索）' },
              { value: 'graphrag', label: 'GraphRAG（KG + 向量混合检索）' },
              { value: 'wiki', label: 'LLM Wiki（写时合成互链页面，REQ-241）' },
            ]}
            onChange={(val) => form.setFieldsValue({ kb_vector: val === 'rag', kb_graph: val === 'graphrag' })}
          />
        </Form.Item>
        {mode !== 'wiki' && (
          <Form.Item label="检索能力（KB-11：可多选，同库双路自动融合路由）">
            <Space>
              <Form.Item name="kb_vector" valuePropName="checked" noStyle>
                <Checkbox>向量检索（含 BM25 混合）</Checkbox>
              </Form.Item>
              <Form.Item name="kb_graph" valuePropName="checked" noStyle>
                <Checkbox>图谱检索（多跳 + 社区全局）</Checkbox>
              </Form.Item>
            </Space>
          </Form.Item>
        )}
        <Form.Item name="description" label="描述">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="用途说明（可选）" />
        </Form.Item>
        <Form.Item
          name="store_backend"
          label="向量后端"
          extra={mode === 'wiki' ? 'wiki 库检索读页非片段，不使用向量后端（本项保留仅作兼容）。' : 'qdrant：eino-ext 适配 / 直连 REST（P1 首选）；sqlite：无外部依赖对照（fallback）。创建后不可切换。'}
        >
          <Select
            options={[
              { value: 'qdrant', label: 'qdrant' },
              { value: 'sqlite', label: 'sqlite' },
            ]}
          />
        </Form.Item>
      </Form>
    </Modal>
  )
}

export default function KnowledgePage() {
  const { bumpData } = useUI()
  const rail = useSidebarCollapse('eino.kb.sidebar.collapsed')
  const [view, setView] = useState<KbView>(readView)
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  /** 跨模块聚焦 handoff（消费与审计「前往知识库治理」）+ 建库后聚焦，消费后置空 */
  const [focusId, setFocusId] = useState<string | null>(null)

  const reloadKBs = () => {
    api.listKBs()
      .then((ks) => {
        setKbs(ks)
        setLoadErr(null)
      })
      .catch((e: any) => {
        setKbs([])
        setLoadErr(e?.message ?? '加载失败')
      })
  }

  useEffect(reloadKBs, [])

  // eino.kb.focus：指定选中库（自动切到其类型页）
  useEffect(() => {
    if (!kbs.length) return
    const focus = localStorage.getItem('eino.kb.focus')
    if (!focus) return
    const kb = kbs.find((k) => k.id === focus)
    if (kb) {
      localStorage.removeItem('eino.kb.focus')
      setView(modeOf(kb))
      setFocusId(kb.id)
    }
  }, [kbs])

  const select = (key: KbView) => {
    localStorage.setItem(VIEW_KEY, key)
    setView(key)
  }

  const pageProps = {
    kbs,
    loadErr,
    onReloadKBs: reloadKBs,
    onCreate: () => setCreateOpen(true),
    focusId,
    onFocusDone: () => setFocusId(null),
  }

  return (
    // 左栏宽度并入全站单一约定（lib/layout.ts 单源，REQ-237 布局基线）
    <Splitter className="main sidebar-splitter" onResizeEnd={sidebarRemember}>
      <Splitter.Panel defaultSize={sidebarDefaultSize()} min={SIDEBAR_WIDTH.min} max={SIDEBAR_WIDTH.max} className="sidebar-panel">
        {rail.collapsed ? (
          <CollapsedRail onExpand={rail.toggle} ariaLabel="知识库侧栏（已收起）" />
        ) : (
          <aside className="sidebar">
            <div className="side-head">
              <span className="side-title">知识库</span>
              <SidebarCollapseButton onClick={rail.toggle} />
              <span className="side-count">{kbs.length}</span>
            </div>
            {/* REQ-258：三类型导航（本体五栏同款范式：图标+标题+一句话描述，库计数徽标） */}
            <div className="onto-nav">
              {NAV.map((n) => (
                <button
                  key={n.key}
                  type="button"
                  className={`onto-nav-item${view === n.key ? ' active' : ''}`}
                  aria-current={view === n.key || undefined}
                  onClick={() => select(n.key)}
                >
                  <span className="onto-nav-icon">{n.icon}</span>
                  <span className="onto-nav-text">
                    <span className="onto-nav-label">
                      {n.label}
                      <Tag
                        color={view === n.key ? 'geekblue' : 'default'}
                        style={{ margin: 0, marginInlineStart: 6, fontSize: 10, lineHeight: '16px', padding: '0 5px' }}
                      >
                        {kbs.filter((k) => modeOf(k) === n.key).length}
                      </Tag>
                    </span>
                    <span className="onto-nav-desc">{n.desc}</span>
                  </span>
                </button>
              ))}
            </div>
            <div className="side-actions">
              <Button type="primary" block icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                新建库
              </Button>
            </div>
            <div className="side-reserve-wrap">
              <div className="reserve-note">
                <div className="reserve-title">模块定位</div>
                知识资产管理模块（M14 双子模块 + REQ-241 第三类型）：RAG / GraphRAG / LLM Wiki 三类型独立页面，检索增强对话的知识源。
              </div>
            </div>
          </aside>
        )}
      </Splitter.Panel>
      <Splitter.Panel className="content-panel">
        {loadErr && !kbs.length ? (
          <div className="work-empty">
            <Result
              status="warning"
              title="知识库后端未就绪"
              subTitle={loadErr}
              extra={<Button onClick={() => reloadKBs()}>重试</Button>}
            />
          </div>
        ) : view === 'graphrag' ? (
          <GraphRagPage {...pageProps} />
        ) : view === 'wiki' ? (
          <WikiPage {...pageProps} />
        ) : (
          <RagPage {...pageProps} />
        )}

        {createOpen && (
          <CreateKBModal
            defaultMode={view}
            onClose={() => setCreateOpen(false)}
            onCreated={(kb) => {
              setCreateOpen(false)
              bumpData()
              setView(modeOf(kb))
              setFocusId(kb.id)
              reloadKBs()
            }}
          />
        )}
      </Splitter.Panel>
    </Splitter>
  )
}
