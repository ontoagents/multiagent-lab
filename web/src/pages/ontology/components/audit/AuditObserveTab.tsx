import { useState } from 'react'
import { Alert, Button, Card, Empty, Input, InputNumber, Select, Space, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ExportOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import LoadErrorAlert from '../../../../components/LoadErrorAlert'
import { api } from '../../../../api/client'
import type { RuntimeProfile } from '../../../../api/types'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// 消费与审计 · 消费观测页签（REQ-290/M94 内容置换新增：本栏自 KB 抽取 KG 观测
// 切换为本体消费面观测）。三卡：
//   1 运行方案消费一览（装载版本 loaded_status + 质量快照 loaded_quality 低分橙标，
//     口径与 RuntimePage qualityWarn/REQ-234① 同源——只警示不阻断）
//   2 TTL 关键词快查（跨谓词字面量包含检索，原 KG 检索页签 TTL 臂迁入；
//     KB 文本抽取对照源退役归知识库「全局问答」）
//   3 消费面导航（运行态实渲/SPARQL 工作台/AI 消费/CQ 验收 栏级指路 +
//     知识库抽取 KG 出口——D-O19 边界②的显式出口）
// ---------------------------------------------------------------------------

/** TTL 链路：跨谓词字面量包含检索（不绑定具体词表——装载 TTL 的谓词形态由导入器决定） */
const TTL_SEARCH = (kw: string, limit: number) =>
  `SELECT DISTINCT ?s ?label WHERE { ?s ?p ?label . FILTER(isLiteral(?label) && CONTAINS(LCASE(STR(?label)), LCASE("${kw.replace(/"/g, '')}"))) } LIMIT ${limit}`

/** 装载发布状态快照解析（loaded_status JSON；REQ-293③ v2 扩展 generation/triple_count/loaded_at/gate，解析失败静默） */
export function parseLoadedStatus(p: RuntimeProfile): { oid: string; status?: string; version_name?: string; generation?: string; triple_count?: number; loaded_at?: string; gate?: string }[] {
  if (!p.loaded_status) return []
  try {
    return Object.entries(JSON.parse(p.loaded_status) as Record<string, { status?: string; version_name?: string; generation?: string; triple_count?: number; loaded_at?: string; gate?: string }>).map(
      ([oid, v]) => ({ oid, status: v.status, version_name: v.version_name, generation: v.generation, triple_count: v.triple_count, loaded_at: v.loaded_at, gate: v.gate }),
    )
  } catch {
    return []
  }
}

/** 装载质量快照解析（loaded_quality JSON {oid:{overall,error_count,warning_count}}；解析失败静默） */
export function parseLoadedQuality(p: RuntimeProfile): { oid: string; overall: number; errors: number; warnings: number }[] {
  if (!p.loaded_quality) return []
  try {
    return Object.entries(JSON.parse(p.loaded_quality) as Record<string, { overall?: number; error_count?: number; warning_count?: number }>).map(
      ([oid, v]) => ({ oid, overall: Math.round(v.overall ?? 0), errors: v.error_count ?? 0, warnings: v.warning_count ?? 0 }),
    )
  } catch {
    return []
  }
}

/** 低分判定（与 RuntimePage qualityWarn 同规则：overall<80 或存在 error 级） */
export function hasQualityWarn(p: RuntimeProfile): boolean {
  return parseLoadedQuality(p).some((q) => q.overall < 80 || q.errors > 0)
}

/** 栏级跳转（eino.onto.sidebar + onto-sidebar-change 全模块通用动线） */
function gotoPane(key: 'assets' | 'runtime') {
  localStorage.setItem('eino.onto.sidebar', key)
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
}

const NAV_ITEMS: { key: string; title: string; tag: string; color: string; body: string; pane?: 'assets' | 'runtime' }[] = [
  {
    key: 'runtime-graph',
    title: '运行态实渲',
    tag: '本体资产 · 可视化',
    color: 'geekblue',
    body: '本体 TTL 装载引擎后的 TBox/ABox 实渲图谱（SPARQL 拉取，REQ-234②）——「本体被装载成什么样」。',
    pane: 'assets',
  },
  {
    key: 'sparql',
    title: 'SPARQL 工作台',
    tag: '本体运行',
    color: 'blue',
    body: '面向已知结构的精确三元组查询（REQ-92）；本页「关键词快查」即其轻量封装。',
    pane: 'runtime',
  },
  {
    key: 'ai-consume',
    title: 'AI 消费 · onto_* 工具',
    tag: '本体资产 · 被引用',
    color: 'purple',
    body: '智能体经 facade 六工具（guide/sparql_query/list_concepts 等）消费本体——资产详情「被引用」页签可见工具清单。',
    pane: 'assets',
  },
  {
    key: 'cq-sparql',
    title: 'CQ → SPARQL 验收',
    tag: '本体资产 · 质量卡',
    color: 'cyan',
    body: '能力问题翻译为只读 SELECT 并在运行方案上执行验证（REQ-255）——消费侧的验收闭环。',
    pane: 'assets',
  },
]

export default function AuditObserveTab({
  profiles,
  loading,
  onReload,
}: {
  profiles: RuntimeProfile[]
  loading: boolean
  onReload: () => void
}) {
  const { setPage } = useUI()
  const running = profiles.filter((p) => p.status === 'running')
  const [profileId, setProfileId] = useState<string>('')
  const [kw, setKw] = useState('')
  const [maxResults, setMaxResults] = useState<number>(20)
  const [querying, setQuerying] = useState(false)
  const [rows, setRows] = useState<{ uri: string; label: string }[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const doQuery = async () => {
    const pid = profileId || running[0]?.id
    if (!pid) return
    if (!kw.trim()) return
    setQuerying(true)
    setErr(null)
    setRows(null)
    try {
      const { json } = await api.runSparql(pid, TTL_SEARCH(kw.trim(), maxResults))
      setRows(
        (json?.results?.bindings ?? []).map((b: Record<string, any>) => ({
          uri: b.s?.value ?? '',
          label: b.label?.value ?? '',
        })),
      )
    } catch (e: any) {
      setErr(e?.message ?? 'SPARQL 查询失败')
    } finally {
      setQuerying(false)
    }
  }

  const columns: ColumnsType<RuntimeProfile> = [
    { title: '方案', dataIndex: 'name', ellipsis: true, render: (v, r) => (
      <Space size={6}>
        <Typography.Text strong>{v || '未命名方案'}</Typography.Text>
        <Tag color="green" style={{ margin: 0 }}>running</Tag>
        {r.port ? <Typography.Text type="secondary" style={{ fontSize: 12 }}>:{r.port}</Typography.Text> : null}
      </Space>
    ) },
    { title: '引擎', dataIndex: 'engine', width: 110, render: (v) => v || '—' },
    { title: '本体', width: 90, render: (_, r) => r.ontology_ids?.length ?? 0 },
    {
      title: '装载',
      dataIndex: 'loaded_status',
      ellipsis: true,
      render: (_, r) => {
        const ls = parseLoadedStatus(r)
        if (ls.length === 0) return <Typography.Text type="secondary">—</Typography.Text>
        return (
          <Tooltip
            title={ls
              .map(
                (x) =>
                  `${x.oid}${x.version_name ? ` · ${x.version_name}` : ''}（${x.status ?? '?'}）${x.generation ? `\n装载代次 ${x.generation}${x.triple_count != null ? ` · ${x.triple_count} 三元组` : ''}${x.loaded_at ? ` · ${x.loaded_at.replace('T', ' ').slice(0, 16)}` : ''}${x.gate ? ` · 对账 ${x.gate}` : ''}` : ''}`,
              )
              .join('\n---\n')}
          >
            <span data-testid="loaded-generation-cell">
              {ls.map((x) => `${x.oid.slice(0, 8)}${x.version_name ? ` · ${x.version_name}` : ''}${x.generation ? ` · ${x.generation}` : ''}`).join('；')}
            </span>
          </Tooltip>
        )
      },
    },
    {
      title: '质量快照',
      dataIndex: 'loaded_quality',
      width: 220,
      render: (_, r) => {
        const qs = parseLoadedQuality(r)
        if (qs.length === 0) return <Typography.Text type="secondary">无快照</Typography.Text>
        return (
          <Space size={4} wrap>
            {qs.map((q) => {
              const low = q.overall < 80 || q.errors > 0
              return (
                <Tooltip
                  key={q.oid}
                  title={`本体 ${q.oid} 装载质量分 ${q.overall}（error ${q.errors} / warning ${q.warnings}）——仅警示不阻断；详情见本体资产「质量卡」页签`}
                >
                  <Tag color={low ? 'orange' : 'green'} style={{ margin: 0 }}>
                    质量 {q.overall}
                  </Tag>
                </Tooltip>
              )
            })}
          </Space>
        )
      },
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
            <span>运行方案消费一览</span>
          </Space>
        }
        extra={
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={onReload}>
            刷新
          </Button>
        }
      >
        {running.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="无运行中的本体方案——到「本体运行」栏启动后，此处观测装载版本与消费质量"
          />
        ) : (
          <Table<RuntimeProfile>
            rowKey="id"
            columns={columns}
            dataSource={running}
            loading={loading}
            pagination={false}
            size="small"
            scroll={{ x: 'max-content' }}
          />
        )}
      </Card>

      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">2</span>
            <span>关键词快查 · 本体 TTL 装载源</span>
            <Tag color="green" style={{ margin: 0 }}>
              TTL
            </Tag>
          </Space>
        }
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 10 }}
          title="对已装载本体做关键词级实体检索（跨谓词字面量包含匹配）——「本体被消费成什么样」的直观试查；知识库文本抽取 KG 的 GraphRAG 试查归知识库模块（REQ-290 内容置换）。"
        />
        <Space size={10} wrap style={{ marginBottom: 10 }}>
          <Select
            style={{ minWidth: 260 }}
            placeholder="选择运行中的本体方案"
            value={profileId || undefined}
            onChange={setProfileId}
            options={running.map((p) => ({ value: p.id, label: p.name || p.id }))}
            notFoundContent="无运行中的方案（到「本体运行」栏启动）"
          />
          <Input
            style={{ width: 240 }}
            placeholder="关键词（按标签/注释/名称包含匹配）"
            value={kw}
            onChange={(e) => setKw(e.target.value)}
            onPressEnter={doQuery}
          />
          <InputNumber
            min={1}
            max={50}
            value={maxResults}
            onChange={(v) => setMaxResults(typeof v === 'number' ? v : 20)}
            addonBefore="limit"
            style={{ width: 140 }}
          />
          <Button type="primary" icon={<SearchOutlined />} loading={querying} disabled={running.length === 0} onClick={doQuery}>
            检索
          </Button>
        </Space>
        {err && <LoadErrorAlert title="TTL 检索失败" message={err} onRetry={doQuery} style={{ marginTop: 6 }} />}
        {rows !== null && rows.length === 0 && !err && (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ margin: '16px 0' }} description="无命中（换关键词，或确认方案已装载本体）" />
        )}
        {rows !== null && rows.length > 0 && (
          <div className="sema-claims">
            {rows.map((r, i) => (
              <div className="sema-claim" key={i}>
                <p className="sema-claim-text">{r.label}</p>
                <div className="sema-claim-meta">
                  <Tag color="green" style={{ margin: 0 }}>
                    TTL 来源
                  </Tag>
                  <Tag style={{ margin: 0, maxWidth: 420, overflow: 'hidden', textOverflow: 'ellipsis' }}>{r.uri}</Tag>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">3</span>
            <span>消费面导航</span>
          </Space>
        }
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
          {NAV_ITEMS.map((n) => (
            <Card
              key={n.key}
              type="inner"
              size="small"
              title={
                <Space size={6}>
                  <Tag color={n.color} style={{ margin: 0 }}>
                    {n.tag}
                  </Tag>
                  <span>{n.title}</span>
                </Space>
              }
              style={{ width: 330 }}
              extra={
                n.pane ? (
                  <Button size="small" type="link" icon={<ExportOutlined />} onClick={() => gotoPane(n.pane!)}>
                    前往
                  </Button>
                ) : undefined
              }
            >
              <p className="sema-feature-body" style={{ margin: 0 }}>
                {n.body}
              </p>
            </Card>
          ))}
          <Card
            key="kb-exit"
            type="inner"
            size="small"
            title={
              <Space size={6}>
                <Tag color="default" style={{ margin: 0 }}>
                  知识库模块
                </Tag>
                <span>KB 抽取 KG · GraphRAG</span>
              </Space>
            }
            style={{ width: 330 }}
            extra={
              <Button size="small" type="link" icon={<ExportOutlined />} onClick={() => setPage('knowledge')}>
                前往
              </Button>
            }
          >
            <p className="sema-feature-body" style={{ margin: 0 }}>
              知识库文本抽取 KG 的图谱展示与治理在知识库模块 GraphRAG 页（REQ-290 内容置换：本栏不再重复展示）。
            </p>
          </Card>
        </div>
      </Card>
    </div>
  )
}
