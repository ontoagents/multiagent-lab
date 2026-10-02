import { useState } from 'react'
import { Badge, Button, Empty, Form, Input, Modal, Popconfirm, Space, Tag, Tooltip, Typography, Upload } from 'antd'
import type { BadgeProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ReloadOutlined, UploadOutlined } from '@ant-design/icons'
import { api } from '../../api/client'
import type { KBDoc, KBHit, KnowledgeBase } from '../../api/types'
import { useUI } from '../../store/ui'
import HighlightSpans from '../../components/HighlightSpans'
import EmptyGuide from '../../components/EmptyGuide'
import LoadErrorAlert from '../../components/LoadErrorAlert'

// ---------------------------------------------------------------------------
// 知识库三类型共享件（REQ-258：类型导航化拆分——RAG / GraphRAG / LLM Wiki 各自独立
// 页面实现与显示，本文件只放三者共用的常量与显示组件，不含类型专属逻辑）。
// ---------------------------------------------------------------------------

export type KBMode = 'rag' | 'graphrag' | 'wiki'
export const modeOf = (k?: KnowledgeBase | null): KBMode =>
  k?.mode === 'graphrag' || k?.mode === 'wiki' ? k.mode : 'rag'

/** 类型页公共 props（壳层下发：库清单/刷新/建库/跨模块聚焦 handoff） */
export interface KbTypePageProps {
  kbs: KnowledgeBase[]
  loadErr: string | null
  onReloadKBs: () => void
  onCreate: () => void
  focusId: string | null
  onFocusDone: () => void
}

export const MODE_TAG: Record<KBMode, { color: string; text: string }> = {
  rag: { color: 'blue', text: 'RAG' },
  graphrag: { color: 'purple', text: 'GraphRAG' },
  wiki: { color: 'cyan', text: 'LLM Wiki' },
}

/** 文档索引状态 → antd Badge 状态（后端未知状态优雅回退） */
const DOC_STATUS: Record<string, { status: BadgeProps['status']; text: string }> = {
  pending: { status: 'default', text: '待索引' },
  indexing: { status: 'processing', text: '索引中' },
  success: { status: 'success', text: '成功' },
  failed: { status: 'error', text: '失败' },
}
export function docStatusOf(s?: string): { status: BadgeProps['status']; text: string } {
  return DOC_STATUS[s ?? ''] ?? { status: 'default', text: s || '未知' }
}

/** 相似度格式化（容忍字符串 / 缺失） */
export function fmtScore(v: unknown): string {
  const n = Number(v)
  return Number.isFinite(n) ? n.toFixed(3) : '—'
}

export function StatTile({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="stat-tile">
      <span className="k">{k}</span>
      <span className="v">{v}</span>
    </div>
  )
}

/** REQ-241（M67）：wiki 页面类型分组元数据（LLM Wiki 类型页专用显示） */
export const WIKI_TYPE_META: Record<string, { label: string; color: string }> = {
  index: { label: '目录', color: 'default' },
  summary: { label: '文档摘要', color: 'blue' },
  entity: { label: '实体', color: 'green' },
  concept: { label: '概念', color: 'geekblue' },
  topic: { label: '主题', color: 'orange' },
  synthesis: { label: '综合', color: 'purple' },
}

/** 类型页详情头（名称 + 类型徽标 + 描述 + 删除库；extra 追加类型专属徽标/动作） */
export function KbDetailHead({
  kb,
  onDelete,
  extra,
}: {
  kb: KnowledgeBase
  onDelete: () => void
  extra?: React.ReactNode
}) {
  return (
    <div className="work-head">
      <div className="work-head-text">
        <div className="work-head-title">
          <Typography.Title level={4} style={{ margin: 0 }}>
            {kb.name}
            <Tag color={MODE_TAG[modeOf(kb)].color} style={{ marginInlineStart: 8, verticalAlign: 'middle' }}>
              {MODE_TAG[modeOf(kb)].text}
            </Tag>
            {kb.kb_vector && kb.kb_graph && (
              <Tag color="geekblue" style={{ marginInlineStart: 4, verticalAlign: 'middle' }}>
                双路
              </Tag>
            )}
            {extra}
          </Typography.Title>
        </div>
        <p className="work-head-desc">{kb.description || '未填写描述'}</p>
      </div>
      <Popconfirm
        title={`删除知识库「${kb.name}」？`}
        description="将删除其全部文档、chunk 与向量数据。"
        okText="删除"
        okButtonProps={{ danger: true }}
        cancelText="取消"
        onConfirm={onDelete}
      >
        <Button danger>删除库</Button>
      </Popconfirm>
    </div>
  )
}

/** 类型页库列表列（固定宽、自带纵向滚动；空态/加载失败引导自包含） */
export function KbListColumn({
  title,
  libs,
  activeId,
  onSelect,
  loadErr,
  onRetry,
  onCreate,
  emptySteps,
  emptyFooter,
}: {
  title: string
  libs: KnowledgeBase[]
  activeId: string | null
  onSelect: (id: string) => void
  loadErr: string | null
  onRetry: () => void
  onCreate: () => void
  emptySteps: string[]
  emptyFooter?: string
}) {
  return (
    <div
      style={{
        width: 232,
        flexShrink: 0,
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
        borderRight: '1px solid var(--c-line)',
        background: 'var(--c-bg-soft)',
      }}
    >
      <div className="side-head">
        <span className="side-title">{title}</span>
        <span className="side-count">{libs.length}</span>
      </div>
      <div className="side-list" style={{ padding: '0 8px 8px' }}>
        {libs.map((k) => (
          <div key={k.id} className={`side-item${k.id === activeId ? ' active' : ''}`} onClick={() => onSelect(k.id)}>
            <div className="side-item-top">
              <span className="side-item-name" title={k.name}>
                {k.name}
              </span>
            </div>
            <div className="side-item-meta">
              <span>文档 {k.doc_count ?? '—'}</span>
              <span className="dot">·</span>
              <span>chunks {k.chunk_count ?? '—'}</span>
            </div>
          </div>
        ))}
        {libs.length === 0 &&
          (loadErr ? (
            <LoadErrorAlert title="知识库列表加载失败" message={loadErr} onRetry={onRetry} style={{ margin: 12 }} />
          ) : (
            <EmptyGuide
              title="创建第一个知识库"
              steps={emptySteps}
              actionLabel="＋ 新建知识库"
              onAction={onCreate}
              footer={emptyFooter}
            />
          ))}
      </div>
    </div>
  )
}

/** 文档表列（三类型共用显示；KG 抽取徽标仅 graphrag 库文档携带数据时出现） */
export function useDocColumns(onReindex: (d: KBDoc) => void, onRemove: (d: KBDoc) => void, reindexing: string | null): ColumnsType<KBDoc> {
  return [
    {
      title: '文档',
      dataIndex: 'title',
      ellipsis: true,
      render: (v: string) => <Typography.Text strong>{v}</Typography.Text>,
    },
    {
      title: 'Chunks',
      dataIndex: 'chunk_count',
      width: 90,
      align: 'right',
      render: (v?: number) => (v ?? '—'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: (_, d: KBDoc) => {
        const st = docStatusOf(d.status)
        const badge = <Badge status={st.status} text={st.text} />
        const tip = [d.status === 'failed' ? d.error : '', d.graphrag?.degraded ? `KG 抽取降级：${d.graphrag.error ?? '抽取异常'}` : ''].filter(Boolean).join('；')
        const withGr = d.graphrag && (
          <span style={{ marginInlineStart: 6 }}>
            {d.graphrag.degraded ? (
              <Tag color="warning" style={{ margin: 0, fontSize: 11 }}>KG 降级</Tag>
            ) : (
              <Tooltip title={`KG 抽取完成（${d.graphrag.method ?? 'llm'}：实体 ${d.graphrag.entities ?? 0} / 关系 ${d.graphrag.relationships ?? 0}）`}>
                <Tag color="purple" style={{ margin: 0, fontSize: 11 }}>KG ✓</Tag>
              </Tooltip>
            )}
          </span>
        )
        return (
          <Space size={4}>
            {tip ? <Tooltip title={tip}>{badge}</Tooltip> : badge}
            {withGr}
          </Space>
        )
      },
    },
    {
      title: '操作',
      width: 170,
      render: (_, d: KBDoc) => (
        <Space size={0}>
          <Button
            type="link"
            size="small"
            icon={<ReloadOutlined />}
            loading={reindexing === d.id}
            onClick={() => onReindex(d)}
          >
            重建索引
          </Button>
          <Popconfirm
            title={`删除文档「${d.title}」？`}
            description="将级联删除其全部 chunk 与向量。"
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => onRemove(d)}
          >
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]
}

/** 检索命中列表（三类型共用显示；来源标签 / spans 高亮 / 相似度分档） */
export function HitsList({ hits }: { hits: KBHit[] | null }) {
  if (!hits) return null
  return (
    <>
      {hits.length === 0 && (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="无命中（可尝试降低 min_score 或补充文档）" style={{ marginTop: 16 }} />
      )}
      {hits.length > 0 && (
        <div className="hits">
          {hits.map((h, i) => {
            const score = Number(h.score)
            const band = score >= 0.8 ? 'hi' : score >= 0.5 ? 'mid' : 'lo'
            return (
              <div className={`hit ${band}`} key={`${h.doc}-${h.seq}-${i}`}>
                <div className="hit-top">
                  <span className="hit-doc">{h.doc}</span>
                  <span className="hit-seq">#{h.seq}</span>
                  <span className="hit-spacer" />
                  {h.sources && h.sources.length > 0 && (
                    <Tooltip title={`来源 chunk：${h.sources.map((s) => `${s.doc ?? '?'}#${s.seq}`).join('、')}（回答引用该页可下钻原文）`}>
                      <Tag color="default" style={{ margin: 0, fontSize: 11 }}>
                        来源 {h.sources.length}
                      </Tag>
                    </Tooltip>
                  )}
                  <Tag className="hit-score" color={band === 'hi' ? 'green' : band === 'mid' ? 'blue' : 'gold'} style={{ margin: 0 }}>
                    score {fmtScore(h.score)}
                  </Tag>
                </div>
                <div className="hit-excerpt">
                  <HighlightSpans text={h.excerpt} spans={h.spans} />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </>
  )
}

/** 上传文档：粘贴文本，或选择本地 txt / md 读取文本填入（仅读取文本，不传文件本体） */
export function UploadDocModal({ kb, onClose, onUploaded }: { kb: KnowledgeBase; onClose: () => void; onUploaded: () => void }) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [busy, setBusy] = useState(false)

  const save = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    if (!String(v.content ?? '').trim()) {
      showToast('文档内容不能为空', 'err')
      return
    }
    setBusy(true)
    try {
      await api.uploadKBDoc(kb.id, { name: v.name, content: v.content })
      showToast('已上传，索引在后台进行')
      onUploaded()
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
      title={`上传文档 · ${kb.name}`}
      width={620}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={busy} onClick={save}>
            上传
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item label="从文件读取（txt / md）" extra="仅在浏览器本地读取文本填入下方，不上传文件本体。">
          <Upload
            accept=".txt,.md,text/plain,text/markdown"
            maxCount={1}
            showUploadList={false}
            beforeUpload={(file) => {
              file
                .text()
                .then((t) => {
                  form.setFieldsValue({ content: t, name: form.getFieldValue('name') || file.name })
                })
                .catch(() => showToast('读取文件失败', 'err'))
              return false
            }}
          >
            <Button icon={<UploadOutlined />}>选择文件</Button>
          </Upload>
        </Form.Item>
        <Form.Item name="name" label="文档名" rules={[{ required: true, message: '文档名必填' }]}>
          <Input placeholder="如：deployment-guide.md" maxLength={120} />
        </Form.Item>
        <Form.Item name="content" label="内容" rules={[{ required: true, message: '内容必填' }]}>
          <Input.TextArea autoSize={{ minRows: 8, maxRows: 18 }} placeholder="粘贴文本，或从上方选择 txt / md 文件自动填入" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
