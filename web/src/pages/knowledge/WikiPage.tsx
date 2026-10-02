import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Collapse,
  Drawer,
  Empty,
  Input,
  InputNumber,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { SyncOutlined } from '@ant-design/icons'
import XMarkdown from '@ant-design/x-markdown'
import type { KBDoc, KBHit, WikiBuildResult, WikiPage } from '../../api/types'
import { api } from '../../api/client'
import { useUI } from '../../store/ui'
import { DRAWER_SIZES, drawerSizeProps } from '../../lib/layout'
import {
  KbDetailHead,
  KbListColumn,
  HitsList,
  StatTile,
  UploadDocModal,
  WIKI_TYPE_META,
  modeOf,
  useDocColumns,
  type KbTypePageProps,
} from './shared'

// ---------------------------------------------------------------------------
// LLM Wiki 类型页（REQ-258 类型导航化拆分；REQ-241/M67 能力的独立实现与显示）：
// 库列表 + 详情两页签（Wiki 页面 / 文档与检索）。写时合成层：LLM 摄取生成互链
// Markdown 页面，检索读页非读片段，零 embedding 依赖；重建按文档数产生 LLM 调用。
// 检索参数仅 TopK / min_score（wiki 能力固定读页，无能力开关与向量后端语义）。
// ---------------------------------------------------------------------------

export default function WikiPage({ kbs, loadErr, onReloadKBs, onCreate, focusId, onFocusDone }: KbTypePageProps) {
  const { showToast, bumpData } = useUI()
  const libs = kbs.filter((k) => modeOf(k) === 'wiki')
  const [activeId, setActiveId] = useState<string | null>(null)
  const active = libs.find((k) => k.id === activeId) ?? libs[0] ?? null

  const [docs, setDocs] = useState<KBDoc[]>([])
  const [docsLoading, setDocsLoading] = useState(false)
  const [reindexing, setReindexing] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)

  const [query, setQuery] = useState('')
  const [topK, setTopK] = useState<number | null>(4)
  const [minScore, setMinScore] = useState<number | null>(0)
  const [savingCfg, setSavingCfg] = useState(false)
  const [searching, setSearching] = useState(false)
  const [hits, setHits] = useState<KBHit[] | null>(null)
  const [searchMeta, setSearchMeta] = useState<{ mode?: string; degraded?: boolean; error?: string } | null>(null)

  const [conns, setConns] = useState<{ id: string; name: string; model_name: string }[]>([])

  const hasReady = docs.some((d) => d.status === 'success')

  const reloadDocs = (kbId: string) => {
    setDocsLoading(true)
    api.listKBDocs(kbId)
      .then(setDocs)
      .catch(() => setDocs([]))
      .finally(() => setDocsLoading(false))
  }

  useEffect(() => {
    api.listConnections().then((cs) => setConns(cs.filter((c) => c.conn_type === 'chat')))
      .catch(() => setConns([]))
  }, [])

  useEffect(() => {
    if (focusId && libs.some((k) => k.id === focusId)) {
      setActiveId(focusId)
      onFocusDone()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId, kbs])

  useEffect(() => {
    setQuery('')
    setHits(null)
    setSearchMeta(null)
    if (!active?.id) {
      setDocs([])
      setTopK(4)
      setMinScore(0)
      return
    }
    const kb = libs.find((k) => k.id === active.id)
    setTopK(kb?.top_k ?? 4)
    setMinScore(kb?.min_score ?? 0)
    reloadDocs(active.id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active?.id])

  const saveConfig = async () => {
    if (!active) return
    setSavingCfg(true)
    try {
      await api.updateKB(active.id, {
        ...active,
        top_k: topK ?? active.top_k,
        min_score: minScore ?? active.min_score,
      })
      showToast('配置已保存')
      bumpData()
      onReloadKBs()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSavingCfg(false)
    }
  }

  const doSearch = async () => {
    if (!active) return
    const q = query.trim()
    if (!q) {
      showToast('请输入检索词', 'err')
      return
    }
    setSearching(true)
    try {
      const r = await api.searchPreview(active.id, q, topK ?? undefined, minScore ?? undefined)
      setSearchMeta({ mode: r.mode, degraded: r.degraded, error: r.error })
      setHits(r.hits ?? [])
    } catch (e: any) {
      showToast(e.message, 'err')
      setHits(null)
    } finally {
      setSearching(false)
    }
  }

  const reindex = async (doc: KBDoc) => {
    if (!active) return
    setReindexing(doc.id)
    try {
      await api.reindexKBDoc(active.id, doc.id)
      showToast('已提交重建索引')
      reloadDocs(active.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setReindexing(null)
    }
  }

  const removeDoc = async (doc: KBDoc) => {
    if (!active) return
    try {
      await api.deleteKBDoc(active.id, doc.id)
      showToast('已删除文档')
      reloadDocs(active.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const removeKB = async () => {
    if (!active) return
    try {
      await api.deleteKB(active.id)
      showToast('已删除知识库')
      setActiveId(null)
      bumpData()
      onReloadKBs()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const columns = useDocColumns(reindex, removeDoc, reindexing)

  const docRetrievalCards = (
    <>
      <Card
        className="work-card"
        size="small"
        title="检索参数"
        extra={<Button type="primary" loading={savingCfg} onClick={saveConfig}>保存</Button>}
      >
        <div className="cfg-row">
          <label className="cfg-field">
            <span className="cfg-label">TopK（返回页面数）</span>
            <InputNumber min={1} max={20} value={topK} onChange={(v) => setTopK(typeof v === 'number' ? v : null)} style={{ width: 140 }} />
          </label>
          <label className="cfg-field">
            <span className="cfg-label">min_score（相似度下限 0~1）</span>
            <InputNumber min={0} max={1} step={0.05} precision={2} value={minScore} onChange={(v) => setMinScore(typeof v === 'number' ? v : null)} style={{ width: 160 }} />
          </label>
          <Typography.Text type="secondary" style={{ fontSize: 12, alignSelf: 'center' }}>
            wiki 检索读页非读片段，无能力开关（REQ-241）
          </Typography.Text>
        </div>
      </Card>

      <Card
        className="work-card"
        size="small"
        title={`文档（${docs.length}）`}
        extra={
          <Button type="primary" onClick={() => setUploadOpen(true)}>
            上传文档
          </Button>
        }
      >
        <Table<KBDoc>
          rowKey="id"
          columns={columns}
          dataSource={docs}
          loading={docsLoading}
          pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
          size="middle"
          locale={{ emptyText: '暂无文档，点击右上「上传文档」导入 txt / md' }}
        />
      </Card>

      <Card
        className="work-card"
        size="small"
        title="检索试运行"
        extra={
          hasReady ? (
            <Space size={6}>
              {searchMeta?.mode && <Tag color="cyan" style={{ margin: 0 }}>{searchMeta.mode}</Tag>}
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                命中按相似度排序（index 页不参与）
              </Typography.Text>
            </Space>
          ) : (
            <Tag color="warning" style={{ margin: 0 }}>
              索引未就绪，暂不可检索
            </Tag>
          )
        }
      >
        <div className="search-row">
          <Input
            allowClear
            value={query}
            disabled={!hasReady}
            placeholder={hasReady ? '输入检索词，回车试运行' : '需至少一篇「就绪」文档'}
            onChange={(e) => setQuery(e.target.value)}
            onPressEnter={doSearch}
            style={{ maxWidth: 420 }}
          />
          <InputNumber
            min={1}
            max={20}
            value={topK}
            disabled={!hasReady}
            onChange={(v) => setTopK(typeof v === 'number' ? v : null)}
            style={{ width: 128 }}
            prefix={<Typography.Text type="secondary" style={{ fontSize: 12 }}>TopK</Typography.Text>}
          />
          <Button type="primary" disabled={!hasReady} loading={searching} onClick={doSearch}>
            检索
          </Button>
        </div>
        <HitsList hits={hits} />
      </Card>
    </>
  )

  return (
    <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
      <KbListColumn
        title="LLM Wiki 库"
        libs={libs}
        activeId={active?.id ?? null}
        onSelect={setActiveId}
        loadErr={loadErr}
        onRetry={onReloadKBs}
        onCreate={onCreate}
        emptySteps={[
          '点击下方「＋ 新建知识库」，类型选 LLM Wiki（写时合成互链页面）',
          '导入 txt / md 文档',
          '「重建 Wiki」生成文档摘要 / 实体 / 概念 / 主题 / 综合互链页面',
        ]}
        emptyFooter="wiki 检索读页非读片段，零 embedding 依赖；重建按文档数产生 LLM 调用。"
      />

      <div className="work-main" style={{ flex: 1 }}>
        {!active ? (
          <div className="work-empty" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}>
            <span style={{ color: 'var(--c-ink-3)' }}>左侧选择或创建一个 LLM Wiki 知识库</span>
          </div>
        ) : (
          <>
            <KbDetailHead kb={active} onDelete={removeKB} />

            <div className="stat-strip">
              <StatTile k="文档" v={docs.length} />
              <StatTile k="Chunks" v={docs.reduce((s, d) => s + (d.chunk_count ?? 0), 0)} />
              <StatTile k="TopK" v={active.top_k ?? '—'} />
              <StatTile k="min_score" v={active.min_score ?? '—'} />
            </div>

            {/* REQ-258：wiki 库两页签——写时合成页面 / 文档与检索（Tabs 懒挂载同 GraphRAG） */}
            <Tabs
              key={active.id}
              defaultActiveKey="pages"
              items={[
                {
                  key: 'pages',
                  label: 'Wiki 页面',
                  children: <WikiPagesCard kbID={active.id} docCount={docs.length} conns={conns} />,
                },
                { key: 'docs', label: '文档与检索', children: docRetrievalCards },
              ]}
            />
          </>
        )}

        {uploadOpen && active && (
          <UploadDocModal
            kb={active}
            onClose={() => setUploadOpen(false)}
            onUploaded={() => {
              setUploadOpen(false)
              reloadDocs(active.id)
              onReloadKBs()
            }}
          />
        )}
      </div>
    </div>
  )
}

/** REQ-241（M67）：wiki 页面卡——按类型分组列表 + Markdown 预览（可溯源）+ 手动重建（成本预估警告） */
function WikiPagesCard({ kbID, docCount, conns }: { kbID: string; docCount: number; conns: { id: string; name: string; model_name: string }[] }) {
  const { showToast } = useUI()
  const [pages, setPages] = useState<WikiPage[]>([])
  const [loading, setLoading] = useState(false)
  const [rebuilding, setRebuilding] = useState(false)
  const [connID, setConnID] = useState<string>('')
  const [preview, setPreview] = useState<WikiPage | null>(null)
  const [lastResult, setLastResult] = useState<WikiBuildResult | null>(null)

  const reload = useCallback(() => {
    setLoading(true)
    api.listWikiPages(kbID)
      .then((r) => setPages(r.pages ?? []))
      .catch((e: any) => showToast(e?.message ?? '页面加载失败', 'err'))
      .finally(() => setLoading(false))
  }, [kbID, showToast])
  useEffect(reload, [reload])

  const rebuild = async () => {
    setRebuilding(true)
    try {
      const r = await api.rebuildWiki(kbID, connID)
      setLastResult(r.result)
      showToast(`重建完成：${r.result.pages} 页，LLM 调用 ${r.result.llm_calls} 次，未变更跳过 ${r.result.skipped_docs} 篇`)
      reload()
    } catch (e: any) {
      showToast(e?.message ?? '重建失败', 'err')
    } finally {
      setRebuilding(false)
    }
  }

  const groups = useMemo(() => {
    const g: Record<string, WikiPage[]> = {}
    for (const p of pages) (g[p.page_type] ??= []).push(p)
    return g
  }, [pages])

  return (
    <Card
      className="work-card"
      size="small"
      title={`Wiki 页面（${pages.length}）`}
      extra={
        <Space size={8} wrap>
          <Select
            showSearch
            optionFilterProp="label"
            value={connID || undefined}
            onChange={(v) => setConnID(v || '')}
            allowClear
            placeholder="生成模型连接（默认 chat）"
            style={{ width: 240 }}
            options={conns.map((c) => ({ value: c.id, label: c.name + ' · ' + c.model_name }))}
          />
          <Popconfirm
            title="重建 Wiki 页面？"
            description={`将按当前文档全量重新生成（成本 ≈ 文档数 + 1 次 LLM 调用；内容未变更的文档自动跳过）。`}
            okText="重建"
            cancelText="取消"
            onConfirm={rebuild}
            disabled={docCount === 0}
          >
            <Button type="primary" icon={<SyncOutlined />} loading={rebuilding} disabled={docCount === 0}>
              重建 Wiki
            </Button>
          </Popconfirm>
        </Space>
      }
    >
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 12 }}
        title="LLM Wiki = 写时合成层（REQ-241）：重建按文档数产生 LLM 调用（每文档 1 次摘要 + 聚合页批量 1 次），内容未变更的文档自动跳过；检索读页非读片段，零 embedding 依赖。"
      />
      {lastResult && (
        <Alert
          type={lastResult.degraded ? 'warning' : 'success'}
          showIcon
          style={{ marginBottom: 12 }}
          title={`上次重建：${lastResult.pages} 页 · LLM 调用 ${lastResult.llm_calls} 次 · 跳过 ${lastResult.skipped_docs} 篇 · ${lastResult.duration_ms}ms`}
          description={
            lastResult.warnings?.length ? (
              <ul style={{ margin: 0, paddingLeft: 18 }}>
                {lastResult.warnings.map((w, i) => (
                  <li key={i}>{w}</li>
                ))}
              </ul>
            ) : undefined
          }
        />
      )}
      {pages.length === 0 && !loading ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚无 wiki 页面：导入文档后点击「重建 Wiki」生成（文档摘要 / 实体 / 概念 / 主题 / 综合五类互链页面）" />
      ) : (
        <Collapse
          size="small"
          defaultActiveKey={groups['synthesis']?.length ? ['synthesis'] : groups['summary'] ? ['summary'] : undefined}
          items={(['synthesis', 'topic', 'entity', 'concept', 'summary', 'index'] as const)
            .filter((t) => groups[t]?.length)
            .map((t) => ({
              key: t,
              label: (
                <Space size={6}>
                  <Tag color={WIKI_TYPE_META[t].color} style={{ margin: 0 }}>
                    {WIKI_TYPE_META[t].label}
                  </Tag>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {groups[t].length} 页
                  </Typography.Text>
                </Space>
              ),
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {groups[t].map((p) => (
                    <div
                      key={p.id}
                      className="wiki-page-row"
                      onClick={() => setPreview(p)}
                      style={{ cursor: 'pointer', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--c-border, #d9dcec)', display: 'flex', alignItems: 'center', gap: 8 }}
                    >
                      <Typography.Text strong style={{ fontSize: 13 }}>
                        {p.title}
                      </Typography.Text>
                      <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {p.content_md.replace(/[#*\[\]\n]/g, ' ').slice(0, 80)}
                      </Typography.Text>
                      <Tooltip title={`溯源 ${p.sources.length} 个 chunk（预览中可下钻原文）`}>
                        <Tag style={{ margin: 0, fontSize: 11 }}>溯源 {p.sources.length}</Tag>
                      </Tooltip>
                    </div>
                  ))}
                </div>
              ),
            }))}
        />
      )}
      <Drawer
        open={!!preview}
        {...drawerSizeProps('wikipage', DRAWER_SIZES.medium)}
        title={
          preview && (
            <Space size={8}>
              <Tag color={WIKI_TYPE_META[preview.page_type]?.color ?? 'default'} style={{ margin: 0 }}>
                {WIKI_TYPE_META[preview.page_type]?.label ?? preview.page_type}
              </Tag>
              <span>{preview.title}</span>
            </Space>
          )
        }
        onClose={() => setPreview(null)}
        destroyOnHidden
      >
        {preview && (
          <>
            {preview.sources.length > 0 && (
              <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
                来源 chunk：{preview.sources.length} 个（{preview.sources.slice(0, 6).join('、')}
                {preview.sources.length > 6 ? ' …' : ''}）——回答引用该页时可经此下钻原文
              </Typography.Paragraph>
            )}
            <div className="wiki-page-md">
              <XMarkdown>{preview.content_md}</XMarkdown>
            </div>
          </>
        )}
      </Drawer>
    </Card>
  )
}
