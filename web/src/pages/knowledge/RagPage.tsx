import { useEffect, useState } from 'react'
import { Button, Card, Checkbox, Input, InputNumber, Space, Table, Tag, Typography } from 'antd'
import type { KBDoc, KBHit } from '../../api/types'
import { api } from '../../api/client'
import { useUI } from '../../store/ui'
import {
  KbDetailHead,
  KbListColumn,
  HitsList,
  StatTile,
  UploadDocModal,
  modeOf,
  useDocColumns,
  type KbTypePageProps,
} from './shared'

// ---------------------------------------------------------------------------
// RAG 类型页（REQ-258 类型导航化拆分）：向量检索库的独立实现与显示——
// 库列表 + 详情（统计条 / 检索参数 / 文档 / 检索试运行）。GraphRAG / LLM Wiki
// 的对应能力在各自类型页内独立实现（同名能力按类型裁剪，不共享类型专属逻辑）。
// ---------------------------------------------------------------------------

export default function RagPage({ kbs, loadErr, onReloadKBs, onCreate, focusId, onFocusDone }: KbTypePageProps) {
  const { showToast, bumpData } = useUI()
  const libs = kbs.filter((k) => modeOf(k) === 'rag')
  const [activeId, setActiveId] = useState<string | null>(null)
  const active = libs.find((k) => k.id === activeId) ?? libs[0] ?? null

  const [docs, setDocs] = useState<KBDoc[]>([])
  const [docsLoading, setDocsLoading] = useState(false)
  const [reindexing, setReindexing] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)

  const [query, setQuery] = useState('')
  const [topK, setTopK] = useState<number | null>(4)
  const [minScore, setMinScore] = useState<number | null>(0)
  const [kbVec, setKbVec] = useState(true)
  const [kbGraphOn, setKbGraphOn] = useState(false)
  const [savingCfg, setSavingCfg] = useState(false)
  const [searching, setSearching] = useState(false)
  const [hits, setHits] = useState<KBHit[] | null>(null)
  const [searchMeta, setSearchMeta] = useState<{ mode?: string; degraded?: boolean; error?: string } | null>(null)

  const hasReady = docs.some((d) => d.status === 'success')

  const reloadDocs = (kbId: string) => {
    setDocsLoading(true)
    api.listKBDocs(kbId)
      .then(setDocs)
      .catch(() => setDocs([]))
      .finally(() => setDocsLoading(false))
  }

  // 跨模块聚焦 handoff（消费与审计「前往知识库治理」）
  useEffect(() => {
    if (focusId && libs.some((k) => k.id === focusId)) {
      setActiveId(focusId)
      onFocusDone()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId, kbs])

  // 切换库：重置检索态，载入文档，并以库当前检索参数预填配置
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
    setKbVec(kb ? kb.kb_vector ?? true : true)
    setKbGraphOn(kb ? kb.kb_graph ?? false : false)
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
        kb_vector: kbVec, // KB-11：能力开关（全关由后端按 mode 派生兜底）
        kb_graph: kbGraphOn,
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

  return (
    <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
      <KbListColumn
        title="RAG 库"
        libs={libs}
        activeId={active?.id ?? null}
        onSelect={setActiveId}
        loadErr={loadErr}
        onRetry={onReloadKBs}
        onCreate={onCreate}
        emptySteps={[
          '点击下方「＋ 新建知识库」，类型选 RAG（向量检索）',
          '导入 txt / md 文档（自动切分并向量化）',
          '对话中点亮「知识」chip 即可召回',
        ]}
        emptyFooter="嵌入模型使用设置页标记默认的向量连接。"
      />

      <div className="work-main" style={{ flex: 1 }}>
        {!active ? (
          <div className="work-empty" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}>
            <span style={{ color: 'var(--c-ink-3)' }}>左侧选择或创建一个 RAG 知识库</span>
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

            <Card
              className="work-card"
              size="small"
              title="检索参数"
              extra={<Button type="primary" loading={savingCfg} onClick={saveConfig}>保存</Button>}
            >
              <div className="cfg-row">
                <label className="cfg-field">
                  <span className="cfg-label">TopK（返回片段数）</span>
                  <InputNumber min={1} max={20} value={topK} onChange={(v) => setTopK(typeof v === 'number' ? v : null)} style={{ width: 140 }} />
                </label>
                <label className="cfg-field">
                  <span className="cfg-label">min_score（相似度下限 0~1）</span>
                  <InputNumber min={0} max={1} step={0.05} precision={2} value={minScore} onChange={(v) => setMinScore(typeof v === 'number' ? v : null)} style={{ width: 160 }} />
                </label>
                <label className="cfg-field">
                  <span className="cfg-label">检索能力（KB-11 可多选）</span>
                  <Space size={12}>
                    <Checkbox checked={kbVec} onChange={(e) => setKbVec(e.target.checked)}>向量</Checkbox>
                    <Checkbox checked={kbGraphOn} onChange={(e) => setKbGraphOn(e.target.checked)}>图谱</Checkbox>
                  </Space>
                </label>
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
                    {searchMeta?.mode && <Tag color="blue" style={{ margin: 0 }}>{searchMeta.mode}</Tag>}
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      命中按相似度排序
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
              {searchMeta?.degraded && (
                <div style={{ marginTop: 12 }}>
                  <span style={{ color: 'var(--c-ink-2)', fontSize: 12 }}>降级：{searchMeta.error ?? '检索降级'}</span>
                </div>
              )}
              <HitsList hits={hits} />
            </Card>
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
