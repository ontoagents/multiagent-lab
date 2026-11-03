import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Card, Drawer, Empty, Form, Input, Select, Skeleton, Space, Table, Tag, Timeline, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { BranchesOutlined, ExportOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../../../api/client'
import type { OntoDecision } from '../../../../api/types'
import { useUI } from '../../../../store/ui'
import { deepLinkOnto } from '../../shared/ontoRoute'
import EmptyGuide from '../../../../components/EmptyGuide'
import { DRAWER_SIZES, drawerSizeProps } from '../../../../lib/layout'

// ---------------------------------------------------------------------------
// 消费与审计 · 决策审计页签（SQLite 决策表 + derived_from 溯源链 + PROV-O 导出）。
// A4（REQ-145/M22）：决策表分页统一 pageSize 10；溯源链抽屉拆入本文件。
// REQ-290/M94 内容置换：去掉 KB 选择器依赖（subject_id 过滤随 KB 组织对象退役），
// kind 筛选默认 ontology（本体消费侧视角；清空筛选可看 kg/kb/manual 全部留痕——
// 决策审计是跨模块审计表，整表保留在本体侧）。
// ---------------------------------------------------------------------------

/** 客户端下载文本（PROV-O Turtle 导出） */
function downloadText(filename: string, text: string, mime: string) {
  const blob = new Blob([text], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

const KIND_TAG: Record<string, { color: string; text: string }> = {
  kg: { color: 'purple', text: 'kg' },
  ontology: { color: 'geekblue', text: 'ontology' },
  kb: { color: 'blue', text: 'kb' },
  manual: { color: 'default', text: 'manual' },
}

/** REQ-599：机器前缀人话映射（[spec.save] 类 kind+method 前缀 → 表列人话标题；
 *  原文保留详情抽屉内。kg/kb 类标题无前缀原样透传。 */
const TITLE_PREFIX_MAP: Record<string, string> = {
  'spec.save': '保存规格',
  import: '导入本体',
  'merge.apply': '合并入库',
  fork: '派生新本体',
  'ingest.csv': '数据灌装',
  'overlay.edit': '人工编辑',
  'overlay.revert': '撤回编辑',
  delete: '删除本体',
}

function humanTitle(title: string): string {
  const m = title.match(/^\[([\w.]+)\]\s*(.+)$/)
  if (!m) return title
  return TITLE_PREFIX_MAP[m[1]] ? m[2] : title
}

const KIND_OPTIONS = [
  { value: 'kg', label: 'kg（KG 抽取/重建）' },
  { value: 'ontology', label: 'ontology（本体构建/版本）' },
  { value: 'kb', label: 'kb（知识库操作）' },
  { value: 'manual', label: 'manual（手工补录）' },
]

export default function AuditDecisionTab({ reloadKey = 0 }: { reloadKey?: number }) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [busy, setBusy] = useState(false)
  const [decisions, setDecisions] = useState<OntoDecision[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [kindFilter, setKindFilter] = useState<string | undefined>('ontology')
  // REQ-599：本体选择器（subject_id 筛选，listDecisions 已支持、UI 此前无入口）
  const [subjectFilter, setSubjectFilter] = useState<string | undefined>(undefined)
  // REQ-599：对象名名称解析（listOntologies 下拉+表格渲染；fallback 裸 ID 灰字如实）
  const [ontos, setOntos] = useState<{ id: string; name: string }[]>([])
  const [kbs, setKbs] = useState<{ id: string; name: string }[]>([])
  const [detailTarget, setDetailTarget] = useState<OntoDecision | null>(null)
  const [lastId, setLastId] = useState<string | null>(null)
  const [chainTarget, setChainTarget] = useState<OntoDecision | null>(null)
  const [exporting, setExporting] = useState(false)
  const manualFormRef = useRef<HTMLDivElement | null>(null)

  // 后端单次取回上限 200（store/kg.go ListDecisions limit≤200）——如实取 200 并明示截断
  const DECISION_LIMIT = 200

  const load = () => {
    setLoading(true)
    api
      .listDecisions({ subject_kind: kindFilter, subject_id: subjectFilter, limit: DECISION_LIMIT })
      .then((r) => {
        setDecisions(r)
        setErr(null)
      })
      .catch((e: any) => {
        setDecisions([])
        setErr(e?.message ?? '决策列表获取失败')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kindFilter, subjectFilter])

  // 名称解析数据源（本体清单+知识库清单——补录 subject 下拉同源）
  useEffect(() => {
    api.listOntologies().then((os) => setOntos(os.map((o) => ({ id: o.id, name: o.name })))).catch(() => setOntos([]))
    api.listKBs().then((ks) => setKbs(ks.map((k) => ({ id: k.id, name: k.name })))).catch(() => setKbs([]))
  }, [])

  // REQ-599：页头「刷新」刷全部——reloadKey 变化重拉决策表
  useEffect(() => {
    if (reloadKey === 0) return
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadKey])

  const resolveName = (kind: string, id: string): string | null => {
    if (!id) return null
    if (kind === 'ontology') return ontos.find((o) => o.id === id)?.name ?? null
    if (kind === 'kb') return kbs.find((k) => k.id === id)?.name ?? null
    return null
  }

  const submit = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setBusy(true)
    try {
      const r = await api.createDecision({
        subject_kind: v.subject_kind,
        subject_id: v.subject_id || undefined,
        title: v.title,
        rationale: v.rationale || undefined,
        derived_from: v.derived_from || undefined,
      })
      setLastId(r.id)
      showToast('决策已留痕')
      form.resetFields()
      load()
    } catch (e: any) {
      showToast(e instanceof ApiError ? e.message : (e?.message ?? '记录失败'), 'err')
    } finally {
      setBusy(false)
    }
  }

  const doExport = async () => {
    setExporting(true)
    try {
      // REQ-599：筛选联动——subject_kind 透传（后端 handlers_kg.go 已支持）；按 kind 聚合导出（单本体导出暂不支持，端点仅 kb_id+subject_kind 两参）
      const text = await api.provExport(undefined, kindFilter || undefined)
      downloadText(`prov-audit-${kindFilter || 'all'}.ttl`, text, 'text/turtle;charset=utf-8')
      showToast('已导出 PROV-O Turtle（Go 原生模板）')
    } catch (e: any) {
      showToast(e?.message ?? '导出失败', 'err')
    } finally {
      setExporting(false)
    }
  }

  const columns: ColumnsType<OntoDecision> = [
    { title: '时间', dataIndex: 'created_at', width: 165, render: (v) => v || '—' },
    {
      title: '类别',
      dataIndex: 'subject_kind',
      width: 100,
      render: (v) => <Tag color={KIND_TAG[v]?.color ?? 'default'} style={{ margin: 0 }}>{KIND_TAG[v]?.text ?? v}</Tag>,
    },
    {
      title: '对象',
      dataIndex: 'subject_id',
      width: 170,
      ellipsis: true,
      render: (v, r) => {
        if (!v) return <Typography.Text type="secondary">—</Typography.Text>
        const name = resolveName(r.subject_kind, v)
        return name ? (
          <span data-testid={`decision-name-${r.id}`}>{name}</span>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }} data-testid={`decision-name-${r.id}`}>
            {v}
          </Typography.Text>
        )
      },
    },
    { title: '决策', dataIndex: 'title', ellipsis: true, render: (v) => humanTitle(v) },
    { title: '依据', dataIndex: 'rationale', ellipsis: true, render: (v) => v || '—' },
    {
      title: '操作',
      width: 150,
      fixed: 'right' as const,
      render: (_, r) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<BranchesOutlined />} onClick={() => setChainTarget(r)}>
            溯源链
          </Button>
          <Button type="link" size="small" onClick={() => setDetailTarget(r)}>
            详情
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div className="sema-home">
      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">1</span>
            <span>决策留痕（抽取/构建自动记录 + 手工补录）</span>
            <Typography.Text type="secondary" style={{ fontSize: 11, fontWeight: 400 }} data-testid="export-scope-note">
              导出按类别聚合（单本体导出暂不支持）
            </Typography.Text>
          </Space>
        }
        extra={
          <Space size={8}>
            <Select
              size="small"
              style={{ width: 170 }}
              allowClear
              placeholder="全部类别"
              value={kindFilter}
              onChange={setKindFilter}
              options={KIND_OPTIONS}
            />
            {/* REQ-599：本体选择器（subject_id 筛选；清单=listOntologies 名称解析） */}
            <Select
              size="small"
              style={{ width: 170 }}
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="全部本体"
              value={subjectFilter}
              onChange={setSubjectFilter}
              options={ontos.map((o) => ({ value: o.id, label: o.name }))}
              data-testid="decision-subject-filter"
            />
            <Button size="small" icon={<ExportOutlined />} loading={exporting} onClick={doExport}>
              导出 PROV-O
            </Button>
          </Space>
        }
      >
        {err && <Alert type="warning" showIcon style={{ marginBottom: 10 }} title="决策列表获取失败" description={err} />}
        {!err && decisions.length === 0 ? (
          // REQ-599：空态=一行灰字正向引导（治理动作自动留痕无需开启）+手工补录入口
          <EmptyGuide
            title="暂无决策留痕"
            steps={['治理动作（保存/导入/fork/合并/灌装/删除）与 KG 抽取会自动留痕，无需手动开启', '有需要时可在下方手工补录一条决策，串入溯源链']}
            actionLabel="手工补录一条"
            onAction={() => manualFormRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })}
            footer="清空类别/本体筛选可看全部留痕（默认按本体类别过滤）。"
          />
        ) : (
          <>
            <Table<OntoDecision>
              rowKey="id"
              columns={columns}
              dataSource={decisions}
              loading={loading}
              pagination={{ pageSize: 10, hideOnSinglePage: true }}
              size="small"
              scroll={{ x: 'max-content' }}
            />
            {/* REQ-599：后端单次上限 200 截断明示（不再静默） */}
            {decisions.length >= DECISION_LIMIT && (
              <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 8, marginBottom: 0 }} data-testid="decision-cap-note">
                已显示最近 {DECISION_LIMIT} 条（服务端单次取回上限 {DECISION_LIMIT}——更早留痕可用类别/本体筛选收窄范围后查看）
              </Typography.Paragraph>
            )}
          </>
        )}
      </Card>

      <div ref={manualFormRef}>
      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">2</span>
            <span>手工补录决策（PROV-O 溯源链节点）</span>
          </Space>
        }
      >
        <Form form={form} layout="vertical" requiredMark={false}>
          <div className="sema-form-grid">
            <Form.Item name="subject_kind" label="类别 subject_kind" initialValue="manual">
              <Select options={KIND_OPTIONS} />
            </Form.Item>
            {/* REQ-599：主体改下拉选择（本体/知识库枚举，不再手填裸 UUID）；manual/kg 类保留手输 */}
            <Form.Item noStyle shouldUpdate={(pv, pc) => pv.subject_kind !== pc.subject_kind}>
              {({ getFieldValue }) => {
                const sk = getFieldValue('subject_kind')
                if (sk === 'ontology' || sk === 'kb') {
                  const opts = (sk === 'ontology' ? ontos : kbs).map((x) => ({ value: x.id, label: x.name }))
                  return (
                    <Form.Item name="subject_id" label="主体（可选，从清单选择）">
                      <Select allowClear showSearch optionFilterProp="label" placeholder="选择本体 / 知识库" options={opts} />
                    </Form.Item>
                  )
                }
                return (
                  <Form.Item name="subject_id" label="主体 subject_id（可选）">
                    <Input placeholder="如本体 id / 库 id" />
                  </Form.Item>
                )
              }}
            </Form.Item>
          </div>
          <Form.Item name="title" label="决策 title" rules={[{ required: true, message: '决策标题必填' }]}>
            <Input placeholder="如：采用 kg-direct 策略由 KB 直转本体" />
          </Form.Item>
          <Form.Item name="rationale" label="依据 rationale">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="依据哪些事实/来源、经何推理" />
          </Form.Item>
          {/* REQ-599：前置决策从上方列表点选预填（不再手工串链） */}
          <Form.Item name="derived_from" label="前置决策 derived_from（可选，串成溯源链）">
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="从留痕列表点选前置决策"
              options={decisions.map((d) => ({ value: d.id, label: `${humanTitle(d.title)}（${d.created_at?.slice(0, 16).replace('T', ' ')}）` }))}
              notFoundContent="上方留痕列表为空——先产生或补录一条决策"
            />
          </Form.Item>
          <Space size={10} wrap>
            <Button type="primary" icon={<SaveOutlined />} loading={busy} onClick={submit}>
              记录决策
            </Button>
            {lastId && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                最近记录 ID：<Typography.Text code style={{ fontSize: 12 }}>{lastId}</Typography.Text>
              </Typography.Text>
            )}
          </Space>
        </Form>
      </Card>
      </div>

      {chainTarget && <ChainDrawer decision={chainTarget} onClose={() => setChainTarget(null)} />}
      {detailTarget && <DetailDrawer decision={detailTarget} resolveName={resolveName} onClose={() => setDetailTarget(null)} />}
    </div>
  )
}

/** 溯源链抽屉：沿 derived_from 回溯（后端 32 跳封顶 + 环防御） */
function ChainDrawer({ decision, onClose }: { decision: OntoDecision; onClose: () => void }) {
  const [chain, setChain] = useState<OntoDecision[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    setErr(null)
    api
      .decisionChain(decision.id)
      .then((r) => {
        setChain(r)
        setErr(null)
      })
      .catch((e: any) => {
        setChain([])
        setErr(e?.message ?? '溯源链获取失败')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [decision.id])

  return (
    <Drawer
      open
      {...drawerSizeProps('chain', DRAWER_SIZES.medium)}
      title={
        <Space size={8}>
          <BranchesOutlined />
          <span>溯源链 · {decision.title}</span>
        </Space>
      }
      onClose={onClose}
      extra={
        <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={load}>
          刷新
        </Button>
      }
    >
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        决策 id：<Typography.Text code style={{ fontSize: 12 }}>{decision.id}</Typography.Text>；链路 = 本条沿
        derived_from 逐级回溯（32 跳封顶，环防御）。
      </Typography.Text>

      {err && <Alert type="error" showIcon style={{ marginTop: 10 }} title="溯源链获取失败" description={err} />}

      <div className="sema-audit-chain" style={{ marginTop: 12 }}>
        {loading && chain.length === 0 ? (
          <Skeleton active title={false} paragraph={{ rows: 5 }} />
        ) : chain.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无链路" />
        ) : (
          <Timeline
            items={chain.map((d, i) => ({
              key: d.id,
              color: i === 0 ? 'green' : 'blue',
              title: (
                <Space size={6} wrap>
                  <Typography.Text code style={{ fontSize: 12 }}>
                    {d.id}
                  </Typography.Text>
                  <Tag color={KIND_TAG[d.subject_kind]?.color ?? 'default'} style={{ margin: 0 }}>
                    {d.subject_kind}
                  </Tag>
                  {i === 0 && (
                    <Tag color="green" style={{ margin: 0 }}>
                      起点
                    </Tag>
                  )}
                </Space>
              ),
              content: (
                <div className="sema-audit-node">
                  <div>
                    <span className="sema-audit-k">决策</span>
                    {d.title}
                  </div>
                  {d.rationale && (
                    <div>
                      <span className="sema-audit-k">依据</span>
                      {d.rationale}
                    </div>
                  )}
                  {d.derived_from && (
                    <div>
                      <span className="sema-audit-k">前置</span>
                      <Typography.Text code style={{ fontSize: 12 }}>{d.derived_from}</Typography.Text>
                    </div>
                  )}
                  <div>
                    <span className="sema-audit-k">时间</span>
                    {d.created_at}
                  </div>
                </div>
              ),
            }))}
          />
        )}
      </div>
    </Drawer>
  )
}

/** 详情抽屉（REQ-599：meta_json 首次渲染——治理事件 method/path/变更摘要；
 *  决策标题原文保留；本体主体给「前往资产详情」深链） */
function DetailDrawer({
  decision,
  resolveName,
  onClose,
}: {
  decision: OntoDecision
  resolveName: (kind: string, id: string) => string | null
  onClose: () => void
}) {
  const subjectName = resolveName(decision.subject_kind, decision.subject_id)
  let metaPretty: string | null = null
  let metaErr = false
  if (decision.meta_json) {
    try {
      metaPretty = JSON.stringify(JSON.parse(decision.meta_json), null, 2)
    } catch {
      metaErr = true
    }
  }
  return (
    <Drawer
      open
      {...drawerSizeProps('decision-detail', DRAWER_SIZES.medium)}
      title={
        <Space size={8}>
          <Tag color={KIND_TAG[decision.subject_kind]?.color ?? 'default'} style={{ margin: 0 }}>
            {KIND_TAG[decision.subject_kind]?.text ?? decision.subject_kind}
          </Tag>
          <span>决策详情</span>
        </Space>
      }
      onClose={onClose}
    >
      <div className="sema-audit-node" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div>
          <span className="sema-audit-k">决策</span>
          {decision.title}
        </div>
        <div>
          <span className="sema-audit-k">对象</span>
          {decision.subject_id ? (
            subjectName ? `${subjectName}` : decision.subject_id
          ) : '—'}
        </div>
        {decision.rationale && (
          <div>
            <span className="sema-audit-k">依据</span>
            {decision.rationale}
          </div>
        )}
        {decision.derived_from && (
          <div>
            <span className="sema-audit-k">前置</span>
            <Typography.Text code style={{ fontSize: 12 }}>{decision.derived_from}</Typography.Text>
          </div>
        )}
        <div>
          <span className="sema-audit-k">时间</span>
          {decision.created_at}
        </div>
        <div>
          <span className="sema-audit-k">留痕 id</span>
          <Typography.Text code style={{ fontSize: 12 }}>{decision.id}</Typography.Text>
        </div>
      </div>
      {decision.subject_kind === 'ontology' && decision.subject_id && (
        <Button
          size="small"
          type="primary"
          ghost
          style={{ marginTop: 10 }}
          onClick={() => {
            deepLinkOnto({ col: 'assets', assetId: decision.subject_id }, [['eino.onto.asset.focus', decision.subject_id]])
          }}
        >
          前往资产详情
        </Button>
      )}
      <div className="onto-sec" style={{ marginTop: 14 }}>
        <span className="onto-sec-title">meta_json（治理事件载荷）</span>
      </div>
      {decision.meta_json ? (
        metaErr ? (
          <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
            {decision.meta_json}（非结构化载荷，原文如实呈现）
          </Typography.Paragraph>
        ) : (
          <pre style={{ background: 'var(--c-bg-soft, #f6f7fb)', padding: 10, borderRadius: 8, fontSize: 12, overflow: 'auto', maxHeight: 320 }} data-testid="decision-meta-json">
            {metaPretty}
          </pre>
        )
      ) : (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>本条决策无 meta_json 载荷（如：手工补录条目）。</Typography.Text>
      )}
    </Drawer>
  )
}
