import { useEffect, useState } from 'react'
import { Alert, Button, Card, Checkbox, Input, InputNumber, Select, Space, Table, Tabs, Tag, Typography } from 'antd'
import type { KBDoc, KBHit } from '../../api/types'
import { api } from '../../api/client'
import { useUI } from '../../store/ui'
import KGGraphView, { KGGovernancePanel, KGGlobalPanel } from '../../components/KGGraphView'
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
// GraphRAG 类型页（REQ-258 类型导航化拆分）：KG+向量混合检索库的独立实现与显示——
// 库列表 + 详情四页签（图谱与统计 / 抽取治理 / 全局问答 / 文档与检索）。
// 检索参数含 KG 抽取配置（抽取模型 / 提示词覆写 / 本体约束 / chunks 预算，KB-6③）。
// ---------------------------------------------------------------------------

export default function GraphRagPage({ kbs, loadErr, onReloadKBs, onCreate, focusId, onFocusDone }: KbTypePageProps) {
  const { showToast, bumpData } = useUI()
  const libs = kbs.filter((k) => modeOf(k) === 'graphrag')
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
  const [kbGraphOn, setKbGraphOn] = useState(true)
  const [savingCfg, setSavingCfg] = useState(false)
  const [searching, setSearching] = useState(false)
  const [hits, setHits] = useState<KBHit[] | null>(null)
  const [searchMeta, setSearchMeta] = useState<{ mode?: string; degraded?: boolean; error?: string } | null>(null)

  const [conns, setConns] = useState<{ id: string; name: string; model_name: string }[]>([])
  const [kgConnID, setKgConnID] = useState<string | null>(null)
  const [kgPrompt, setKgPrompt] = useState<string | null>(null)
  const [kgOntoID, setKgOntoID] = useState<string | null>(null)
  const [kgMaxChunks, setKgMaxChunks] = useState<number | null>(null)
  const [ontos, setOntos] = useState<{ id: string; name: string }[]>([])

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
  // M36/KB-6③：本体约束候选（构建平面资产列表；不可达静默降级=仅无挂载可选）
  useEffect(() => {
    api.listOntologies().then((os) => setOntos(os ?? []))
      .catch(() => setOntos([]))
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
    setKgConnID(kb?.kg_conn_id ?? '')
    setKgPrompt(kb?.kg_prompt ?? '')
    setKgOntoID(kb?.kg_ontology_id ?? '')
    setKgMaxChunks(kb?.kg_max_chunks ?? 0)
    setKbVec(kb ? kb.kb_vector ?? false : true)
    setKbGraphOn(kb ? kb.kb_graph ?? true : true)
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
        kg_conn_id: kgConnID ?? active.kg_conn_id ?? '',
        kg_prompt: kgPrompt ?? active.kg_prompt ?? '',
        kg_ontology_id: kgOntoID ?? active.kg_ontology_id ?? '',
        kg_max_chunks: kgMaxChunks ?? active.kg_max_chunks ?? 0,
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

  /** GraphRAG 直查（KB-12 语义）：KG 无命中自动回退向量，降级不阻断 */
  const graphragSearch = async () => {
    if (!active) return
    const q = query.trim()
    if (!q) {
      showToast('请输入检索词', 'err')
      return
    }
    setSearching(true)
    try {
      const r = await api.graphragSearchKB(active.id, q, topK ?? undefined)
      setSearchMeta({ mode: r.mode, degraded: r.degraded, error: r.error })
      setHits(r.hits ?? [])
      if (r.degraded) showToast('KG 无命中或不可用，GraphRAG 检索已降级为向量', 'err')
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
        <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            KG 抽取配置（REQ-129①）：抽取模型与提示词按库覆写；留空 = 跟随全局默认
          </Typography.Text>
          <Space size={8} wrap>
            <Select
              showSearch
              optionFilterProp="label"
              value={kgConnID || undefined}
              onChange={(v) => setKgConnID(v || '')}
              allowClear
              placeholder="抽取模型连接（默认 chat 连接）"
              style={{ width: 320 }}
              options={conns.map((c) => ({ value: c.id, label: c.name + ' · ' + c.model_name }))}
            />
            <InputNumber
              min={0}
              max={2000}
              value={kgMaxChunks ?? 0}
              onChange={(v) => setKgMaxChunks(typeof v === 'number' ? v : 0)}
              style={{ width: 180 }}
              addonAfter="chunks 预算"
            />
          </Space>
          <Space size={8} wrap align="center">
            <Select
              showSearch
              optionFilterProp="label"
              value={kgOntoID || undefined}
              onChange={(v) => setKgOntoID(v || '')}
              allowClear
              placeholder="本体约束抽取（选择挂载本体，可清除）"
              style={{ width: 320 }}
              options={ontos.map((o) => ({ value: o.id, label: o.name }))}
            />
            <Typography.Text type="secondary" style={{ fontSize: 11 }}>
              挂载后抽取 prompt 注入该本体概念/关系词表白名单（本体→KB 出向，差异化能力）；重建 KG 生效
            </Typography.Text>
          </Space>
          <Input.TextArea
            value={kgPrompt ?? ''}
            onChange={(e) => setKgPrompt(e.target.value)}
            placeholder="提示词覆写（追加领域抽取约束，JSON 输出契约保留）"
            autoSize={{ minRows: 2, maxRows: 6 }}
          />
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
              <Button size="small" loading={searching} onClick={graphragSearch}>
                GraphRAG 直查
              </Button>
              {searchMeta?.mode && <Tag color={searchMeta.mode === 'graphrag' ? 'purple' : 'blue'} style={{ margin: 0 }}>{searchMeta.mode}</Tag>}
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
          <Button type="primary" icon={undefined} disabled={!hasReady} loading={searching} onClick={doSearch}>
            检索
          </Button>
        </div>
        {searchMeta?.degraded && (
          <Alert
            type="warning"
            showIcon
            style={{ marginTop: 12 }}
            title="GraphRAG worker 不可达，本次结果来自向量检索回退（降级不阻断）"
            description={searchMeta.error}
          />
        )}
        <HitsList hits={hits} />
      </Card>
    </>
  )

  return (
    <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
      <KbListColumn
        title="GraphRAG 库"
        libs={libs}
        activeId={active?.id ?? null}
        onSelect={setActiveId}
        loadErr={loadErr}
        onRetry={onReloadKBs}
        onCreate={onCreate}
        emptySteps={[
          '点击下方「＋ 新建知识库」，类型选 GraphRAG（KG + 向量混合检索）',
          '导入 txt / md 文档（索引时自动抽取自存 KG）',
          '对话中点亮「知识」chip：KG 命中优先，自动回退向量',
        ]}
        emptyFooter="KG 抽取使用设置页 chat 连接（可按库覆写）；本体约束抽取可选挂载本体词表。"
      />

      <div className="work-main" style={{ flex: 1 }}>
        {!active ? (
          <div className="work-empty" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}>
            <span style={{ color: 'var(--c-ink-3)' }}>左侧选择或创建一个 GraphRAG 知识库</span>
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

            {/* REQ-258：GraphRAG 详情四页签（图谱与统计/抽取治理/全局问答/文档与检索），
                Tabs 懒挂载：图谱/治理/全局问答首访才取数 */}
            <Tabs
              key={active.id}
              defaultActiveKey="graph"
              items={[
                {
                  key: 'graph',
                  label: '图谱与统计',
                  children: (
                    <>
                      <Alert
                        type="info"
                        showIcon
                        style={{ marginBottom: 12 }}
                        title="GraphRAG 子模块（M14；M16 图谱增强已启用）"
                        description="文档索引后自动把 chunks 同步抽取为自存 KG（D-O15 自研抽取：REQ-98 LLM 主路径 + 规则回退，零外部进程）；检索优先 GraphRAG，KG 无命中自动回退向量检索（不阻断）。图谱视图支持实体搜索、邻域展开、claims 溯源与聚焦检索（REQ-127/128）。"
                      />
                      <KGGraphView kbID={active.id} />
                    </>
                  ),
                },
                { key: 'governance', label: '抽取治理', children: <KGGovernancePanel kbID={active.id} /> },
                { key: 'global', label: '全局问答', children: <KGGlobalPanel kbID={active.id} /> },
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
