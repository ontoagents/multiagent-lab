import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Collapse, Empty, Input, Popconfirm, Select, Space, Spin, Switch, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AlertOutlined, ArrowRightOutlined, CheckCircleOutlined, CloseCircleOutlined, HistoryOutlined, QuestionCircleOutlined, ReloadOutlined, SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { QualityReport } from '../../../../api/client'
import type { CqBenchmarkRun, OntoChatCQVerdict, OntoContribution, QualityScoreRow, RuntimeProfile } from '../../../../api/types'
import QualityRadar, { radarDimsOf } from '../../../../components/QualityRadar'

// ---------------------------------------------------------------------------
// REQ-156/M-O15 资产详情「质量卡」页签：qualitygate 报告呈现（复用 REQ-171 引擎，零重复建设）。
//   三维评分（完备性/一致性/可维护性，扣分制加权综合）+ 检查项命中明细 + 本体级 strict 门禁开关
//   （开启后保存/导入合并的错误级命中将被 400 拦截——默认宽松仅告警）。
// REQ-331/M131 资产级质量档案升级：①草稿→入库→当前三点对照+版本×综合分趋势线+评估历史
//   （评分随版本追加记录，写入点=质量卡生成 manual/入库动线 ingest/OntoChat 草稿 draft）；
//   ②Findings 按严重级分组+「怎么改」人话指引+error 定位跳转（沿 REQ-235 前往图形编辑先例）。
//   口径=机器产报告人做决策，不新增自动改写通道。
// ---------------------------------------------------------------------------

const SEV_META: Record<string, { color: string; text: string }> = {
  error: { color: 'red', text: '错误' },
  warning: { color: 'orange', text: '告警' },
  info: { color: 'default', text: '提示' },
}

/** REQ-331①：版本×综合分趋势线（SVG 折线零依赖；点=一次评分记录，悬停看版本/来源/时间） */
function TrendChart({ rows }: { rows: QualityScoreRow[] }) {
  if (rows.length < 2) return null
  const W = 560
  const H = 120
  const PAD = 24
  const xs = (i: number) => PAD + (i * (W - PAD * 2)) / (rows.length - 1)
  const ys = (v: number) => H - PAD - (v / 100) * (H - PAD * 2)
  const path = rows.map((r, i) => `${i === 0 ? 'M' : 'L'}${xs(i).toFixed(1)},${ys(r.overall).toFixed(1)}`).join(' ')
  const srcColor = (s: string) => (s === 'draft' ? '#d97706' : s === 'ingest' ? '#4f46e5' : '#16a34a')
  return (
    <svg width="100%" viewBox={`0 0 ${W} ${H}`} style={{ maxWidth: 640 }} data-testid="quality-trend-chart">
      <line x1={PAD} y1={H - PAD} x2={W - PAD} y2={H - PAD} stroke="var(--ant-color-border, #ddd)" strokeWidth={1} />
      <line x1={PAD} y1={PAD} x2={PAD} y2={H - PAD} stroke="var(--ant-color-border, #ddd)" strokeWidth={1} />
      <path d={path} fill="none" stroke="var(--c-brand, #4f46e5)" strokeWidth={2} />
      {rows.map((r, i) => (
        <circle key={r.id} cx={xs(i)} cy={ys(r.overall)} r={4} fill={srcColor(r.source)}>
          <title>{`v${r.version} · ${{ draft: '草稿', ingest: '入库', manual: '手动' }[r.source] ?? r.source} · ${r.overall.toFixed(1)} 分 · ${r.created_at.slice(0, 19).replace('T', ' ')}`}</title>
        </circle>
      ))}
      <text x={PAD} y={12} fontSize={10} fill="var(--ant-color-text-tertiary, #999)">综合分 {rows[rows.length - 1].overall.toFixed(1)}（{rows.length} 次评估）</text>
    </svg>
  )
}

/** REQ-331③：草稿→入库→当前三点对照（缺段诚实标注「—」——手写/导入路径无草稿分降级两点） */
function EvolutionTriple({ history }: { history: QualityScoreRow[] }) {
  const draft = [...history].reverse().find((r) => r.source === 'draft')
  const ingest = [...history].reverse().find((r) => r.source === 'ingest')
  const current = history[history.length - 1]
  const node = (label: string, r?: QualityScoreRow, hint?: string, testid?: string) => (
    <div style={{ textAlign: 'center', minWidth: 96 }} data-testid={testid}>
      <Typography.Text type="secondary" style={{ fontSize: 11 }}>{label}</Typography.Text>
      <div style={{ fontSize: 22, fontWeight: 700, color: r ? (r.overall >= 90 ? '#16a34a' : r.overall >= 70 ? '#d97706' : '#dc2626') : 'var(--ant-color-text-quaternary, #ccc)' }}>
        {r ? r.overall.toFixed(1) : '—'}
      </div>
      <Typography.Text type="secondary" style={{ fontSize: 10 }}>{r ? `v${r.version} · ${{ draft: '草稿', ingest: '入库', manual: '手动' }[r.source]}` : hint}</Typography.Text>
    </div>
  )
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }} data-testid="quality-evolution-triple">
      {node('草稿分', draft, '无草稿分（手写/导入路径）', 'triple-draft')}
      <ArrowRightOutlined style={{ color: 'var(--ant-color-text-quaternary, #ccc)' }} />
      {node('入库分', ingest, '尚未入库评分', 'triple-ingest')}
      <ArrowRightOutlined style={{ color: 'var(--ant-color-text-quaternary, #ccc)' }} />
      {node('当前分', current, '生成报告后呈现', 'triple-current')}
      <Typography.Text type="secondary" style={{ fontSize: 11, flex: 1, minWidth: 180 }}>
        三点=草稿（生成稿入库前）→ 入库（保存/合并动线自动评）→ 当前（最近一次评估）；存量资产与手写/导入路径无草稿分为预期降级。
      </Typography.Text>
    </div>
  )
}


/** REQ-352/M152：基准报告版本对比视图——选两 run 并排三率+口径声明对照；
 *  dataset_rev/generator_ver 不一致 = 口径漂移，警示跨版本对比不可比（消费 metric_definition 字段面）。 */
function BenchCompareView({ runs }: { runs: CqBenchmarkRun[] }) {
  const sorted = [...runs].reverse() // 新在前
  const label = (r: CqBenchmarkRun) => `${r.dataset}·v${r.version}·${r.ts.slice(5, 16).replace('T', ' ')}`
  const keyOf = (r: CqBenchmarkRun) => `${r.dataset}-${r.ts}`
  const [aId, setA] = useState<string>(() => (sorted[1] ? keyOf(sorted[1]) : ''))
  const [bId, setB] = useState<string>(() => (sorted[0] ? keyOf(sorted[0]) : ''))
  const find = (id: string) => runs.find((r) => keyOf(r) === id)
  const ra = find(aId)
  const rb = find(bId)
  if (!ra || !rb) return null
  const rate = (r: CqBenchmarkRun, k: 'translated' | 'whitelist' | 'gold_aligned') => (r.total ? Math.round((r[k] / r.total) * 100) : 0)
  const drift = (ra.metric_definition?.dataset_rev ?? '') !== (rb.metric_definition?.dataset_rev ?? '')
    || (ra.metric_definition?.generator_ver ?? '') !== (rb.metric_definition?.generator_ver ?? '')
  const delta = (da: number, db: number) => {
    const d = da - db
    if (d === 0) return <Tag style={{ margin: 0 }}>持平</Tag>
    return <Tag color={d > 0 ? 'green' : 'red'} style={{ margin: 0 }}>{d > 0 ? '+' : ''}{d}pp</Tag>
  }
  const rows: { k: string; va: string; vb: string; da?: number; db?: number }[] = [
    { k: '翻译', va: `${ra.translated}/${ra.total}`, vb: `${rb.translated}/${rb.total}`, da: rate(ra, 'translated'), db: rate(rb, 'translated') },
    { k: '白名单', va: `${ra.whitelist}/${ra.total}`, vb: `${rb.whitelist}/${rb.total}`, da: rate(ra, 'whitelist'), db: rate(rb, 'whitelist') },
    { k: 'gold 对齐', va: `${ra.gold_aligned}/${ra.total}`, vb: `${rb.gold_aligned}/${rb.total}`, da: rate(ra, 'gold_aligned'), db: rate(rb, 'gold_aligned') },
    { k: '数据集版本', va: ra.metric_definition?.dataset_rev ?? '—', vb: rb.metric_definition?.dataset_rev ?? '—' },
    { k: '评测器口径', va: ra.metric_definition?.generator_ver ?? '—', vb: rb.metric_definition?.generator_ver ?? '—' },
  ]
  return (
    <div data-testid="cq-bench-compare" style={{ margin: '4px 0 8px' }}>
      <Space size={8} wrap style={{ marginBottom: 4 }}>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>版本对比：</Typography.Text>
        <Select size="small" style={{ minWidth: 220 }} value={aId} onChange={setA} data-testid="bench-compare-a"
          options={sorted.map((r) => ({ value: keyOf(r), label: `A：${label(r)}` }))} />
        <Select size="small" style={{ minWidth: 220 }} value={bId} onChange={setB} data-testid="bench-compare-b"
          options={sorted.map((r) => ({ value: keyOf(r), label: `B：${label(r)}` }))} />
      </Space>
      {drift && (
        <Alert type="warning" showIcon style={{ marginBottom: 4 }}
          title="口径漂移：两次运行的数据集版本或评测器口径不同——三率差异含口径因素，跨版本对比不可比（REQ-352）。" />
      )}
      <Table
        size="small"
        pagination={false}
        rowKey="k"
        dataSource={rows}
        columns={[
          { title: '指标', dataIndex: 'k', width: 110 },
          { title: `A（${label(ra)}）`, dataIndex: 'va', render: (v: string, row) => (row.da !== undefined ? <span>{v} <Tag style={{ margin: 0 }}>{row.da}%</Tag></span> : v) },
          { title: `B（${label(rb)}）`, dataIndex: 'vb', render: (v: string, row) => (row.db !== undefined ? <span>{v} <Tag style={{ margin: 0 }}>{row.db}%</Tag></span> : v) },
          { title: '差值', render: (_: unknown, row) => (row.da !== undefined && row.db !== undefined ? delta(row.da, row.db) : '—') },
        ]}
      />
    </div>
  )
}

export default function QualityCardPane({ ontologyId, onReport, onGoGraphEdit }: { ontologyId: string; onReport?: (r: QualityReport | null) => void; onGoGraphEdit?: () => void }) {
  const [config, setConfig] = useState<{ strict: boolean; reasoning_check?: boolean } | null>(null)
  const [report, setReport] = useState<QualityReport | null>(null)
  const [reportAt, setReportAt] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  // REQ-331/M131：评分历史（三点对照+趋势线+评估历史同源）与修复指引（check_id→人话）
  const [history, setHistory] = useState<QualityScoreRow[]>([])
  const [remediations, setRemediations] = useState<Record<string, string>>({})
  // REQ-346/M146：CQ 基准集「测我」（数据集清单/运行/报告历史）
  const [benchDatasets, setBenchDatasets] = useState<{ name: string; ontology: string; count: number; revision?: string }[]>([]) // REQ-367：revision 透出（r2=60 条分层）
  const [benchDataset, setBenchDataset] = useState<string | undefined>(undefined)
  const [benchBusy, setBenchBusy] = useState(false)
  const [benchProgress, setBenchProgress] = useState('')
  const [benchErr, setBenchErr] = useState<string | null>(null)
  const [benchRuns, setBenchRuns] = useState<CqBenchmarkRun[] | null>(null)
  // REQ-224 H10：本体消费贡献（onto_* 工具命中次数/被引用概念 TopN 入质量卡）
  const [contribution, setContribution] = useState<OntoContribution | null>(null)
  // REQ-376③：「spec 外比例」一等指标（导入对账 artifact → 质量卡常驻）
  const [recon, setRecon] = useState<{ triples_in: number; remainder: number } | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    api
      .qualityConfig(ontologyId)
      .then((c) => setConfig({ strict: c.strict }))
      .catch(() => setConfig(null))
    api
      .qualityReport(ontologyId)
      .then((r) => {
        setReport(r.report)
        setReportAt(r.imported_at)
        setErr(null)
        onReport?.(r.report)
      })
      .catch(() => {
        setReport(null)
        setReportAt('')
      })
      .finally(() => setLoading(false))
    api
      .getQualityHistory(ontologyId)
      .then((r) => setHistory(r.history ?? []))
      .catch(() => setHistory([]))
    api
      .cqBenchmarkDatasets()
      .then((r) => setBenchDatasets(r.datasets ?? []))
      .catch(() => setBenchDatasets([]))
    api
      .cqBenchmarkReport(ontologyId)
      .then((r) => setBenchRuns(r.report?.runs ?? []))
      .catch(() => setBenchRuns(null))
    api
      .ontologyContribution(ontologyId)
      .then(setContribution)
      .catch(() => setContribution(null))
    api
      .getArtifactContent(ontologyId, 'import-reconciliation')
      .then((t) => setRecon(JSON.parse(t)))
      .catch(() => setRecon(null))
  }, [ontologyId])

  useEffect(load, [load])
  // 修复指引全量（只读静态，挂载拉一次）
  useEffect(() => {
    api
      .getQualityRemediations()
      .then(setRemediations)
      .catch(() => setRemediations({}))
  }, [])

  const run = (strictFlag: boolean, reasoningFlag = false) => {
    setRunning(true)
    api
      .qualityRun(ontologyId, strictFlag, reasoningFlag)
      .then((r) => {
        setReport(r.report)
        setReportAt(new Date().toISOString())
        setErr(null)
        onReport?.(r.report) // REQ-240③/M66：资产卡微型雷达同源刷新
      })
      .catch((e: any) => setErr(e?.message ?? '质量检查失败'))
      .finally(() => setRunning(false))
  }

  const flipStrict = (v: boolean) => {
    api
      .setQualityConfig(ontologyId, { strict: v })
      .then((c) => {
        setConfig({ strict: c.strict, reasoning_check: c.reasoning_check })
      })
      .catch((e: any) => setErr(e?.message ?? '开关更新失败'))
  }

  // REQ-255②：推理级检查档开关（owlrl OWL 2 RL；默认关）
  const flipReasoning = (v: boolean) => {
    api
      .setQualityConfig(ontologyId, { reasoning_check: v })
      .then((c) => {
        setConfig({ strict: c.strict, reasoning_check: c.reasoning_check })
      })
      .catch((e: any) => setErr(e?.message ?? '开关更新失败'))
  }

  // ---- REQ-255①（60 号 H2）：CQ→SPARQL 验收闭环（LLM 翻译→人工确认→运行方案执行→通过率）----
  const [cqItems, setCqItems] = useState<{ cq: string; sparql: string }[] | null>(null)
  const [cqBusy, setCqBusy] = useState(false)
  const [cqRunning, setCqRunning] = useState(false)
  const [cqResults, setCqResults] = useState<{ cq: string; sparql: string; rows: number | null; ok: boolean; err?: string }[] | null>(null)
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  const [profileId, setProfileId] = useState<string | null>(null)

  // ---- REQ-274 快筛 + REQ-342/OC-2 双轨测试站（口语化快筛+SPARQL 精判并轨+报告持久化+三级覆盖）----
  const [covBusy, setCovBusy] = useState(false)
  const [covProgress, setCovProgress] = useState('')
  const [covResult, setCovResult] = useState<{ verdicts: OntoChatCQVerdict[]; passed: number; total: number; pass_rate: number } | null>(null)
  const [covErr, setCovErr] = useState<string | null>(null)
  const [covCQs, setCovCQs] = useState<string[]>([])
  const [specShape, setSpecShape] = useState<{ concepts: string[]; relations: string[]; dataProperties: string[] }>({ concepts: [], relations: [], dataProperties: [] })
  // 双轨精判段（No/Unknown 存疑项 SPARQL 翻译+可选自动执行）与三级覆盖量化
  const [sparqlTrack, setSparqlTrack] = useState<{ cq: string; sparql: string; ok: boolean; rows: number | null; err?: string }[] | null>(null)
  const [trackBusy, setTrackBusy] = useState(false)
  const [trackNote, setTrackNote] = useState<string | null>(null)
  // 上次持久化报告（artifact cq-test-report——回归基准：版本超前即过期提示重跑）
  const [lastReport, setLastReport] = useState<{ created_at?: string; base_version?: number; summary?: { passed: number; total: number; pass_rate: number } } | null>(null)
  const [ontoVersion, setOntoVersion] = useState(0)
  useEffect(() => {
    api
      .getSpec(ontologyId)
      .then((sp: any) => {
        setCovCQs(Array.isArray(sp?.cq) ? sp.cq.map((c: string) => String(c)) : [])
        setSpecShape({
          concepts: (sp?.concepts ?? []).map((c: any) => String(c?.name ?? '')),
          relations: (sp?.relations ?? []).map((r: any) => String(r?.name ?? '')),
          dataProperties: (sp?.data_properties ?? []).map((d: any) => String(d?.name ?? '')),
        })
      })
      .catch(() => setCovCQs([]))
    api
      .getOntology(ontologyId)
      .then((o) => setOntoVersion(o?.version ?? 0))
      .catch(() => setOntoVersion(0))
    // 上次报告（404=从未跑过，静默）
    api
      .getArtifactContent(ontologyId, 'cq-test-report')
      .then((raw: any) => {
        try {
          setLastReport(typeof raw === 'string' ? JSON.parse(raw) : raw)
        } catch {
          setLastReport(null)
        }
      })
      .catch(() => setLastReport(null))
  }, [ontologyId])
  useEffect(() => {
    api
      .listRuntimeProfiles()
      .then((ps) => {
        setProfiles(ps)
        setProfileId(ps.find((p) => p.status === 'running')?.id ?? null)
      })
      .catch(() => setProfiles([]))
  }, [])

  const runCoverage = async (cqs: string[]) => {
    setCovBusy(true)
    setCovErr(null)
    setCovProgress('任务排队中…')
    setSparqlTrack(null)
    setTrackNote(null)
    try {
      const j = await api.coverageCQTest(ontologyId, cqs)
      const timer = setInterval(async () => {
        try {
          const job = await api.getOntoChatJob(j.job_id)
          if (job.progress) setCovProgress(job.progress)
          if (job.status === 'done' || job.status === 'error' || job.status === 'cancelled') {
            clearInterval(timer)
            setCovBusy(false)
            setCovProgress('')
            if (job.status === 'error') setCovErr(job.error ?? '测试失败')
            else if (job.status === 'cancelled') setCovErr('已取消')
            else if (job.result) setCovResult(job.result as { verdicts: OntoChatCQVerdict[]; passed: number; total: number; pass_rate: number })
          }
        } catch {
          /* 单次轮询失败下次重试 */
        }
      }, 1500)
    } catch (e: any) {
      setCovBusy(false)
      setCovProgress('')
      setCovErr(e?.message ?? '提交失败')
    }
  }

  // REQ-342/OC-2：双轨并轨——存疑项（No/Unknown）SPARQL 翻译+可选自动执行（需选 running 方案）
  const runDualTrack = async () => {
    if (!covResult) return
    const doubtful = covResult.verdicts.filter((v) => v.verdict !== 'Yes').map((v) => v.cq)
    if (doubtful.length === 0) {
      setTrackNote('快筛全部通过——无双轨精判存疑项')
      return
    }
    setTrackBusy(true)
    setTrackNote(null)
    try {
      const r = await api.cqSparql(ontologyId, doubtful)
      const items = r.items ?? []
      const out: { cq: string; sparql: string; ok: boolean; rows: number | null; err?: string }[] = items.map((it) => ({ cq: it.cq, sparql: it.sparql, ok: false, rows: null }))
      setSparqlTrack(out)
      if (profileId) {
        for (const row of out) {
          try {
            const { json } = await api.runSparql(profileId, row.sparql)
            row.rows = json?.results?.bindings?.length ?? 0
            row.ok = (row.rows ?? 0) > 0
          } catch (e: any) {
            row.err = e?.message ?? '执行失败'
          }
        }
        setSparqlTrack([...out])
        await persistDualReport(out)
      } else {
        setTrackNote(`已翻译 ${out.length} 条存疑项 SPARQL（上方「CQ 验收（SPARQL）」卡选运行方案后可执行）——报告已保存快筛段`)
        await persistDualReport(out)
      }
    } catch (e: any) {
      setTrackNote(`SPARQL 精判失败：${e?.message ?? '未知错误'}（快筛段已保存）`)
    } finally {
      setTrackBusy(false)
    }
  }

  // 三级覆盖量化（81 号 CQ4OE 条件覆盖口径的本体侧映射）+ 术语/属性层覆盖 + 报告持久化
  const persistDualReport = async (track: { cq: string; ok: boolean; rows: number | null; err?: string }[] | null) => {
    if (!covResult) return
    const vs = covResult.verdicts
    const trackByCq = new Map((track ?? []).map((t) => [t.cq, t]))
    const sparqlOk = (cq: string) => {
      const t = trackByCq.get(cq)
      return t ? t.ok && !t.err : null // null=未精判
    }
    const judged = track && track.length > 0
    let full = 0, atLeastOne = 0
    for (const v of vs) {
      const so = sparqlOk(v.cq)
      const oral = v.verdict === 'Yes'
      if (judged) {
        if (oral && so === true) full++
        if (oral || so === true) atLeastOne++
      } else if (oral) {
        atLeastOne++
      }
    }
    const oralRate = covResult.total ? vs.filter((v) => v.verdict === 'Yes').length / covResult.total : 0
    const sparqlRate = judged && track ? track.filter((t) => t.ok && !t.err).length / track.length : 0
    const mean = judged ? (oralRate + sparqlRate) / 2 : oralRate
    // 术语/属性层覆盖：SPARQL 文本引用的 concept:/relation:/attr: IRI（URIPrefix 之后的名称）
    const sparqlTexts = (sparqlTrack ?? []).map((t) => t.sparql).join('\n')
    const refd = new Set<string>()
    for (const m of sparqlTexts.matchAll(/urn:o:[^:]+:(?:concept|relation|attr):([^>\s"']+)/g)) refd.add(m[1])
    const termCovered = specShape.concepts.filter((c) => refd.has(c)).length
    const propTotal = specShape.relations.length + specShape.dataProperties.length
    const propCovered = [...specShape.relations, ...specShape.dataProperties].filter((p) => refd.has(p)).length
    const report = {
      created_at: new Date().toISOString(),
      base_version: ontoVersion,
      summary: { verdicts: vs, passed: covResult.passed, total: covResult.total, pass_rate: covResult.pass_rate },
      sparql_track: sparqlTrack ?? undefined,
      coverage: {
        full: judged ? full / covResult.total : null, // 双轨全过（Full）
        at_least_one: atLeastOne / (covResult.total || 1), // 至少一轨通过（AtLeastOne）
        mean, // 双轨平均（Mean）
        term_covered: termCovered,
        term_total: specShape.concepts.length,
        property_covered: propCovered,
        property_total: propTotal,
        judged,
      },
    }
    try {
      await api.saveCQTestReport(ontologyId, report)
      setLastReport({ created_at: report.created_at, base_version: report.base_version, summary: { passed: covResult.passed, total: covResult.total, pass_rate: covResult.pass_rate } })
    } catch {
      /* 报告持久化失败不阻断呈现 */
    }
  }

  // REQ-346/M146：用基准集测我（202 job → 轮询 → 报告刷新）
  const runBenchmark = async () => {
    if (!benchDataset) return
    setBenchBusy(true)
    setBenchErr(null)
    setBenchProgress('任务排队中…')
    try {
      const j = await api.cqBenchmarkRun(ontologyId, benchDataset)
      const timer = setInterval(async () => {
        try {
          const job: any = await api.aiDraftJob(j.job_id)
          if (job.progress) setBenchProgress(job.progress)
          if (job.status === 'done' || job.status === 'failed') {
            clearInterval(timer)
            setBenchBusy(false)
            setBenchProgress('')
            if (job.status === 'failed') setBenchErr(job.error ?? '测我失败')
            else {
              const r = await api.cqBenchmarkReport(ontologyId)
              setBenchRuns(r.report?.runs ?? [])
            }
          }
        } catch {
          /* 单次轮询失败下次重试 */
        }
      }, 2000)
    } catch (e: any) {
      setBenchBusy(false)
      setBenchProgress('')
      setBenchErr(e?.message ?? '提交失败')
    }
  }

  const covStale = !!(lastReport?.base_version && ontoVersion && lastReport.base_version < ontoVersion)

  const translateCq = () => {
    setCqBusy(true)
    setErr(null)
    api
      .cqSparql(ontologyId)
      .then((r) => {
        setCqItems(r.items)
        setCqResults(null)
      })
      .catch((e: any) => setErr(e?.message ?? 'CQ 翻译失败'))
      .finally(() => setCqBusy(false))
  }

  const runCq = async () => {
    if (!profileId || !cqItems?.length) return
    setCqRunning(true)
    const out: { cq: string; sparql: string; rows: number | null; ok: boolean; err?: string }[] = []
    for (const it of cqItems) {
      try {
        const { json } = await api.runSparql(profileId, it.sparql)
        const rows = json?.results?.bindings?.length ?? null
        out.push({ ...it, rows, ok: (rows ?? 0) > 0 })
      } catch (e: any) {
        out.push({ ...it, rows: null, ok: false, err: e?.message ?? '执行失败' })
      }
    }
    setCqResults(out)
    setCqRunning(false)
  }

  const cqPassRate = cqResults ? `${cqResults.filter((r) => r.ok).length}/${cqResults.length}` : null

  // REQ-331②：分组内列（级别列由分组 Collapse 承载；error 项带定位跳转=前往图形编辑按样例名定位）
  const findingColumns: ColumnsType<NonNullable<QualityReport['findings'][number]>> = [
    { title: '检查项', dataIndex: 'title', ellipsis: true },
    { title: '维度', dataIndex: 'dimension', width: 84, render: (v: string) => ({ completeness: '完备性', consistency: '一致性', maintainability: '可维护性' }[v] ?? v) },
    { title: '命中', dataIndex: 'count', width: 60, render: (v: number) => <b>{v}</b> },
    {
      title: '样例',
      dataIndex: 'samples',
      ellipsis: true,
      render: (v: string[]) => <Typography.Text type="secondary" style={{ fontSize: 12 }}>{(v ?? []).slice(0, 3).join('；')}</Typography.Text>,
    },
    {
      title: '修复',
      width: 96,
      render: (_: unknown, r: NonNullable<QualityReport['findings'][number]>) =>
        r.severity === 'error' && (r.samples?.length ?? 0) > 0 && onGoGraphEdit ? (
          <Tooltip title="前往图形编辑按样例名定位修补（展开行看「怎么改」指引）">
            <Button
              size="small"
              type="link"
              style={{ padding: 0 }}
              onClick={() => onGoGraphEdit()}
              data-testid={`finding-fix-${r.check_id}`}
            >
              前往修复
            </Button>
          </Tooltip>
        ) : r.severity === 'info' ? (
          <Tooltip title="学习中心方法论卡有对应建模方法（七阶段·概念化/关系设计）">
            <Button
              size="small"
              type="link"
              style={{ padding: 0 }}
              onClick={() => {
                localStorage.setItem('eino.onto.sidebar', 'learn')
                window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
              }}
            >
              方法论
            </Button>
          </Tooltip>
        ) : null,
    },
  ]

  if (loading) return <div style={{ padding: '24px 0', textAlign: 'center' }}><Spin /></div>

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Card size="small" className="work-card">
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
          <Space size={6}>
            <SafetyOutlined style={{ color: 'var(--c-brand)' }} />
            <Typography.Text strong>strict 门禁</Typography.Text>
            <Switch checked={config?.strict ?? false} onChange={flipStrict} disabled={!config} />
          </Space>
          <Space size={6}>
            <ThunderboltOutlined style={{ color: 'var(--c-brand)' }} />
            <Tooltip title="REQ-255②：开启后质量检查附跑 OWL 2 RL 推理一致性（owlrl；优先 original 形态），命中以错误级检查项 reasoning_owlrl 并入报告。默认关。">
              <Typography.Text strong>推理检查</Typography.Text>
            </Tooltip>
            <Switch checked={config?.reasoning_check ?? false} onChange={flipReasoning} disabled={!config} data-testid="reasoning-switch" />
          </Space>
          <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, minWidth: 240 }}>
            开启后，保存 Spec / 导入合并时若质量检查存在错误级命中将被拦截（400 + 明细）；默认宽松（仅本卡告警不阻断）。检查项清单见 qualitygate（REQ-171，11 项数据驱动）。
          </Typography.Text>
          <Button size="small" icon={<ReloadOutlined />} loading={running} onClick={() => run(config?.strict ?? false, config?.reasoning_check ?? false)}>
            {report ? '重新生成报告' : '生成质量报告'}
          </Button>
        </div>
      </Card>

      {err && <Alert type="error" showIcon title="质量检查失败" description={err} onClose={() => setErr(null)} />}

      {!report ? (
        <div className="work-empty" style={{ minHeight: 200 }}>
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <>
                <p>尚未生成质量报告。</p>
                <p style={{ fontSize: 12, color: 'var(--ant-color-text-tertiary, #888)' }}>
                  点上方「生成质量报告」运行 qualitygate（11 检查项：孤立概念 / 缺失定义 / 层级循环 / 命名风格 / 悬空引用 / 重复命名等）。
                </p>
              </>
            }
          />
        </div>
      ) : (
        <>
          <Card size="small" className="work-card">
            <div style={{ display: 'flex', gap: 24, alignItems: 'center', flexWrap: 'wrap' }}>
              {/* REQ-240③/M66：能力雷达五维图（三维分+stats 派生两维；SVG 自绘零依赖） */}
              <div style={{ textAlign: 'center', flexShrink: 0 }} data-testid="quality-radar-card">
                <QualityRadar dims={radarDimsOf(report.score, report.stats)} size={186} />
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>能力雷达（五维）</Typography.Text>
              </div>
              <div style={{ textAlign: 'center' }}>
                <Typography.Title level={2} style={{ margin: 0, color: report.score.overall >= 90 ? '#16a34a' : report.score.overall >= 70 ? '#d97706' : '#dc2626' }}>
                  {report.score.overall.toFixed(1)}
                </Typography.Title>
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>综合质量分</Typography.Text>
              </div>
              {(
                [
                  ['完备性', report.score.completeness],
                  ['一致性', report.score.consistency],
                  ['可维护性', report.score.maintainability],
                ] as [string, number][]
              ).map(([name, v]) => (
                <div key={name} style={{ minWidth: 160, flex: 1 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                    <span>{name}</span>
                    <Typography.Text type="secondary">{v.toFixed(0)}</Typography.Text>
                  </div>
                  <div style={{ height: 6, borderRadius: 3, background: '#eef0f6', marginTop: 4 }}>
                    <div style={{ width: `${v}%`, height: '100%', borderRadius: 3, background: v >= 90 ? '#16a34a' : v >= 70 ? '#d97706' : '#dc2626' }} />
                  </div>
                </div>
              ))}
              <Space size={6} wrap>
                <Tag color="red" style={{ margin: 0 }}>错误 {report.error_count}</Tag>
                <Tag color="orange" style={{ margin: 0 }}>告警 {report.warning_count}</Tag>
                <Tag style={{ margin: 0 }}>提示 {report.info_count}</Tag>
                <Tag color={report.pass ? 'green' : 'default'} style={{ margin: 0 }}>{report.pass ? '门禁通过' : '存在错误级命中'}</Tag>
                {cqPassRate && (
                  <Tooltip title="CQ 验收通过率（REQ-255①）：SPARQL 查询有结果=通过；人工确认模板后经运行方案执行">
                    <Tag color="geekblue" style={{ margin: 0 }} data-testid="cq-pass-tag"><CheckCircleOutlined /> CQ 验收 {cqPassRate}</Tag>
                  </Tooltip>
                )}
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                  规模：{report.stats.concepts} 概念 / {report.stats.relations} 关系 / {report.stats.instances} 实例{reportAt ? ` · ${reportAt.slice(0, 19).replace('T', ' ')}` : ''}
                </Typography.Text>
              </Space>
            </div>
          </Card>

          {/* REQ-331①③：草稿→入库→当前三点对照 + 版本×综合分趋势线（评分随版本追加记录） */}
          {history.length > 0 && (
            <Card size="small" className="work-card" data-testid="quality-archive-card">
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <EvolutionTriple history={history} />
                {history.length >= 2 && <TrendChart rows={history} />}
              </div>
            </Card>
          )}

          {/* REQ-255①（60 号 H2）：CQ→SPARQL 验收闭环——LLM 翻译→人工确认模板→运行方案执行→通过率 */}
          <Card size="small" className="work-card" data-testid="cq-verify-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: cqItems ? 10 : 0 }}>
              <CheckCircleOutlined style={{ color: 'var(--c-brand)' }} />
              <Typography.Text strong>CQ 验收（SPARQL）</Typography.Text>
              <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, minWidth: 220 }}>
                能力问题逐条翻译为只读 SELECT（LLM 辅助+人工确认模板），经运行方案引擎执行：有结果=通过。
              </Typography.Text>
              <Select
                size="small"
                style={{ minWidth: 170 }}
                placeholder="选择运行方案"
                value={profileId ?? undefined}
                onChange={setProfileId}
                options={profiles.map((p) => ({ value: p.id, label: `${p.name}${p.status === 'running' ? '' : `（${p.status}）`}` }))}
              />
              {!cqItems ? (
                <Button size="small" loading={cqBusy} onClick={translateCq} data-testid="cq-translate-btn">
                  翻译 CQ
                </Button>
              ) : (
                <Space size={6}>
                  <Button size="small" loading={cqRunning} disabled={!profileId} onClick={runCq} data-testid="cq-run-btn">
                    执行验证
                  </Button>
                  <Button size="small" onClick={translateCq} loading={cqBusy}>重新翻译</Button>
                </Space>
              )}
            </div>
            {cqItems && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                {cqItems.map((it, i) => (
                  <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
                    <Typography.Text style={{ fontSize: 12, width: 260, flexShrink: 0 }}>{it.cq}</Typography.Text>
                    <Input.TextArea
                      size="small"
                      value={it.sparql}
                      rows={2}
                      style={{ fontFamily: 'monospace', fontSize: 11, flex: 1 }}
                      onChange={(e) =>
                        setCqItems((cur) => (cur ? cur.map((x, j) => (j === i ? { ...x, sparql: e.target.value } : x)) : cur))
                      }
                    />
                    {cqResults?.[i] && (
                      <Tag color={cqResults[i].ok ? 'green' : 'default'} style={{ margin: 0, flexShrink: 0 }}>
                        {cqResults[i].err ? '错误' : cqResults[i].ok ? `通过 ${cqResults[i].rows} 行` : '无结果'}
                      </Tag>
                    )}
                  </div>
                ))}
              </div>
            )}
          </Card>


          {/* REQ-274 快筛 + REQ-342/OC-2 覆盖测试站（双轨并轨+报告持久化+三级覆盖——CQ 升级为回归基准） */}
          <Card size="small" className="work-card" data-testid="cq-coverage-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 8 }}>
              <AlertOutlined style={{ color: 'var(--c-brand)' }} />
              <Typography.Text strong>覆盖测试站（快筛+SPARQL 精判双轨）</Typography.Text>
              <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, minWidth: 220 }}>
                {covCQs.length === 0
                  ? '本体 spec.CQ 为空：请先在构建会话中「抽取 CQ」并确认（生成草稿时回写；会话内也可直接对草稿「覆盖测试」），再回来测试。'
                  : '①快筛：本体口语化后逐条 CQ 独立判定（每条单独调用防泄漏，对「可推断不显式」偏乐观）；②精判：存疑项自动翻译 SPARQL 并对运行方案执行（有结果=通过）；双轨结果统一入报告并落 artifact——本体变更后（版本超前）报告过期提示重跑（回归基准）。'}
              </Typography.Text>
              <Popconfirm
                title={`将对 ${covCQs.length} 条能力问题各执行 1 次模型调用，确认开始？`}
                onConfirm={() => runCoverage(covCQs)}
                disabled={covBusy || covCQs.length === 0}
              >
                <Button size="small" loading={covBusy} disabled={covBusy || covCQs.length === 0} data-testid="coverage-run-btn">
                  开始快筛
                </Button>
              </Popconfirm>
            </div>
            {lastReport?.summary && (
              <div style={{ marginBottom: 8, fontSize: 12, color: 'var(--ant-color-text-tertiary, #888)' }} data-testid="cq-report-last">
                上次报告：{(lastReport.created_at ?? '').slice(0, 19).replace('T', ' ') || '—'} · 通过 {lastReport.summary.passed}/{lastReport.summary.total}
                （{Math.round((lastReport.summary.pass_rate ?? 0) * 100)}%）
                {covStale ? (
                  <Tag color="orange" style={{ marginInlineStart: 8, fontSize: 11 }} data-testid="cq-report-stale">
                    报告过期——本体已变更至 v{ontoVersion}（报告基于 v{lastReport.base_version}），建议重跑
                  </Tag>
                ) : (
                  <Tag style={{ marginInlineStart: 8, fontSize: 11 }}>基于 v{lastReport.base_version}</Tag>
                )}
              </div>
            )}
            {covProgress && (
              <div style={{ marginBottom: 8 }}>
                <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>{covProgress}</Typography.Text>
              </div>
            )}
            {covErr && <Alert type="error" showIcon style={{ marginBottom: 8 }} title={covErr} />}
            {covResult && (
              <>
                <Space size={6} style={{ marginBottom: 8 }} wrap>
                  <Tag color="green" data-testid="coverage-pass-tag">快筛通过 {covResult.passed}/{covResult.total}</Tag>
                  <Tag color="geekblue">快筛通过率 {Math.round(covResult.pass_rate * 100)}%</Tag>
                  <Button size="small" loading={trackBusy} disabled={trackBusy} onClick={runDualTrack} data-testid="dual-track-btn">
                    存疑项 SPARQL 精判（双轨）
                  </Button>
                </Space>
                {trackNote && (
                  <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }} data-testid="dual-track-note">{trackNote}</Typography.Paragraph>
                )}
                <Table
                  size="small"
                  rowKey={(r) => r.cq}
                  dataSource={covResult.verdicts}
                  pagination={{ pageSize: 6 }}
                  columns={[
                    { title: '能力问题', dataIndex: 'cq', ellipsis: true },
                    {
                      title: '快筛',
                      dataIndex: 'verdict',
                      width: 90,
                      render: (v: string) =>
                        v === 'Yes' ? (
                          <Tag color="green" icon={<CheckCircleOutlined />}>Yes</Tag>
                        ) : v === 'No' ? (
                          <Tag color="red" icon={<CloseCircleOutlined />}>No</Tag>
                        ) : (
                          <Tag icon={<AlertOutlined />}>Unknown</Tag>
                        ),
                    },
                    {
                      title: 'SPARQL 精判',
                      width: 110,
                      render: (_: unknown, v: OntoChatCQVerdict) => {
                        const t = sparqlTrack?.find((x) => x.cq === v.cq)
                        if (!t) return <Typography.Text type="secondary" style={{ fontSize: 11 }}>未精判</Typography.Text>
                        if (t.err) return <Tag color="orange">错误</Tag>
                        return t.ok ? <Tag color="green" icon={<CheckCircleOutlined />}>通过 {t.rows} 行</Tag> : <Tag color="red" icon={<CloseCircleOutlined />}>无结果</Tag>
                      },
                    },
                    { title: '解释', dataIndex: 'explanation', ellipsis: true },
                  ]}
                />
              </>
            )}
          </Card>

          {/* REQ-346/M146：CQ 基准集「测我」（数据集清单动线+报告历史——测试资产产品化 v1） */}
          {benchDatasets.length > 0 && (
            <Card size="small" className="work-card" data-testid="cq-bench-card">
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 8 }}>
                <SafetyOutlined style={{ color: 'var(--c-brand)' }} />
                <Typography.Text strong>CQ 基准集测我</Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, minWidth: 220 }}>
                  选一份基准集（按源种子域编写），对本本体逐条翻译评测：翻译成功率/白名单通过率/gold 对齐率——域外本体 gold 对齐率低为预期负信号。报告历史随版本留存可比。
                </Typography.Text>
                <Select
                  size="small"
                  style={{ minWidth: 180 }}
                  placeholder="选基准集…"
                  value={benchDataset}
                  onChange={setBenchDataset}
                  options={benchDatasets.map((d) => ({ value: d.name, label: `${d.name}（${d.count} 条·${d.ontology} 域·${d.revision ?? 'r1'}）` }))} // REQ-367：revision 随选项透出
                  data-testid="cq-bench-dataset"
                />
                <Button size="small" type="primary" loading={benchBusy} disabled={benchBusy || !benchDataset} onClick={runBenchmark} data-testid="cq-bench-run-btn">
                  用基准集测我
                </Button>
              </div>
              {benchProgress && (
                <div style={{ marginBottom: 8 }}>
                  <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>{benchProgress}</Typography.Text>
                </div>
              )}
              {benchErr && <Alert type="error" showIcon style={{ marginBottom: 8 }} title={benchErr} onClose={() => setBenchErr(null)} />}
              {benchRuns && benchRuns.length >= 2 && <BenchCompareView runs={benchRuns} />}
              {benchRuns && benchRuns.length > 0 && (
                <Table
                  size="small"
                  rowKey={(r) => `${r.dataset}-${r.ts}`}
                  dataSource={[...benchRuns].reverse()}
                  pagination={{ pageSize: 3 }}
                  data-testid="cq-bench-report"
                  columns={[
                    { title: '时间', dataIndex: 'ts', width: 150, render: (v: string) => <Typography.Text style={{ fontSize: 12 }}>{v.slice(0, 19).replace('T', ' ')}</Typography.Text> },
                    { title: '基准集', dataIndex: 'dataset', width: 100 },
                    { title: '本体版本', dataIndex: 'version', width: 80, render: (v: number) => <Tag style={{ margin: 0 }}>v{v}</Tag> },
                    { title: '翻译', width: 70, render: (_: unknown, r: CqBenchmarkRun) => `${r.translated}/${r.total}` },
                    { title: '白名单', width: 70, render: (_: unknown, r: CqBenchmarkRun) => `${r.whitelist}/${r.total}` },
                    {
                      // REQ-348/M148：gold 对齐口径透出——防「对齐率=答案正确率」误读（文案与 cqbench.go Run 注释一致）
                      title: (
                        <span>
                          gold 对齐
                          <Tooltip title="gold 对齐 = 查询引用了基准 CQ 答案依赖的概念/关系 URN，非答案内容比对；答案级评估随 REQ-346 Judge 共契落地。跨域本体该率低是预期信号（基准集按种子域编写，域外本体不覆盖该域=合理的负信号，非缺陷）。">
                            <QuestionCircleOutlined style={{ marginLeft: 4, color: 'var(--c-ink-3, #999)' }} data-testid="gold-align-hint" />
                          </Tooltip>
                        </span>
                      ),
                      width: 100,
                      render: (_: unknown, r: CqBenchmarkRun) => {
                        const rate = r.total ? r.gold_aligned / r.total : 0
                        return <Tag color={rate >= 0.8 ? 'green' : rate > 0 ? 'orange' : 'red'} style={{ margin: 0 }}>{r.gold_aligned}/{r.total}</Tag>
                      },
                    },
                  ]}
                  expandable={{
                    rowExpandable: (r: CqBenchmarkRun) => r.signals.some((x) => !x.gold_aligned || x.error),
                    expandedRowRender: (r: CqBenchmarkRun) => (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        {r.signals.filter((x) => !x.gold_aligned || x.error).map((x) => (
                          <Typography.Text key={x.id} type="secondary" style={{ fontSize: 11 }}>
                            {x.cq} — {x.error || `翻译=${x.translated} 白名单=${x.whitelist_ok} gold 对齐=${x.gold_aligned}`}
                          </Typography.Text>
                        ))}
                      </div>
                    ),
                  }}
                />
              )}
            </Card>
          )}

          {/* REQ-331②：Findings 按严重级分组+「怎么改」人话指引+error 定位跳转（沿 REQ-235 前往图形编辑先例）——
              口径=机器产报告人做决策：只给动线与指引，不新增自动改写通道 */}
          {(report.findings?.length ?? 0) > 0 ? (
            <Card size="small" className="work-card" data-testid="quality-findings-groups">
              <Collapse
                size="small"
                defaultActiveKey={(['error', 'warning', 'info'] as const).filter((sev) => report.findings!.some((f) => f.severity === sev))}
                items={(['error', 'warning', 'info'] as const)
                  .map((sev) => ({ sev, items: report.findings!.filter((f) => f.severity === sev) }))
                  .filter(({ items }) => items.length > 0)
                  .map(({ sev, items }) => ({
                    key: sev,
                    label: (
                      <Space size={8}>
                        <Tag color={SEV_META[sev]?.color} style={{ margin: 0 }}>{SEV_META[sev]?.text}</Tag>
                        <Typography.Text strong>{items.length} 项检查命中</Typography.Text>
                        {sev === 'error' && <Typography.Text type="secondary" style={{ fontSize: 11 }}>（strict 开启时阻断入库）</Typography.Text>}
                      </Space>
                    ),
                    children: (
                      <Table
                        rowKey={(r) => r.check_id}
                        columns={findingColumns}
                        dataSource={items}
                        size="small"
                        pagination={false}
                        expandable={{
                          rowExpandable: (r) => !!remediations[r.check_id],
                          expandedRowRender: (r) => (
                            <Typography.Text style={{ fontSize: 12 }}>
                              <SafetyOutlined style={{ color: 'var(--c-brand)', marginInlineEnd: 6 }} />
                              怎么改：{remediations[r.check_id]}
                            </Typography.Text>
                          ),
                        }}
                      />
                    ),
                  }))}
              />
            </Card>
          ) : (
            <Card size="small" className="work-card">
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="全部检查项通过，无命中" />
            </Card>
          )}

          {/* REQ-331①：评估历史（每次评分一行——报告全文 artifact 仅存最新一份，历史=评分行，如实标注） */}
          {history.length > 0 && (
            <Collapse
              size="small"
              items={[
                {
                  key: 'quality-history',
                  label: (
                    <Space size={8}>
                      <HistoryOutlined />
                      <Typography.Text strong>评估历史（{history.length} 次）</Typography.Text>
                      <Typography.Text type="secondary" style={{ fontSize: 11 }}>报告全文仅存最新一份 artifact，此处为历次评分记录</Typography.Text>
                    </Space>
                  ),
                  children: (
                    <Table
                      size="small"
                      rowKey={(r) => r.id}
                      dataSource={[...history].reverse()}
                      pagination={{ pageSize: 5 }}
                      data-testid="quality-history-table"
                      columns={[
                        { title: '时间', dataIndex: 'created_at', width: 150, render: (v: string) => <Typography.Text style={{ fontSize: 12 }}>{v.slice(0, 19).replace('T', ' ')}</Typography.Text> },
                        { title: '版本', dataIndex: 'version', width: 60, render: (v: number) => <Tag style={{ margin: 0 }}>v{v}</Tag> },
                        { title: '来源', dataIndex: 'source', width: 70, render: (v: string) => <Tag color={{ draft: 'orange', ingest: 'geekblue', manual: 'green' }[v] ?? 'default'} style={{ margin: 0 }}>{({ draft: '草稿', ingest: '入库', manual: '手动' } as Record<string, string>)[v] ?? v}</Tag> },
                        { title: '综合分', dataIndex: 'overall', width: 80, render: (v: number) => <b>{v.toFixed(1)}</b> },
                        { title: '完备', dataIndex: 'completeness', width: 60, render: (v: number) => v.toFixed(0) },
                        { title: '一致', dataIndex: 'consistency', width: 60, render: (v: number) => v.toFixed(0) },
                        { title: '可维护', dataIndex: 'maintainability', width: 60, render: (v: number) => v.toFixed(0) },
                        { title: '错误/告警', width: 90, render: (_: unknown, r: QualityScoreRow) => `${r.error_count}/${r.warning_count}` },
                      ]}
                    />
                  ),
                },
              ]}
              style={{ marginTop: 12 }}
            />
          )}

          {/* REQ-224 H10：本体消费贡献（onto_* 工具命中次数/被引用概念 TopN；companion.hit 先例推广） */}
          {contribution && contribution.total_hits > 0 && (
            <Collapse
              size="small"
              items={[
                {
                  key: 'onto-contribution',
                  label: (
                    <Space size={8}>
                      <ThunderboltOutlined />
                      <Typography.Text strong>消费贡献（被智能体消费 {contribution.total_hits} 次）</Typography.Text>
                      <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                        onto_* 工具命中统计；最近 {contribution.last_hit_at?.slice(0, 16).replace('T', ' ') ?? '—'}
                      </Typography.Text>
                    </Space>
                  ),
                  children: (
                    <div data-testid="onto-contribution-body">
                      <Space size={6} wrap style={{ marginBottom: 8 }}>
                        {(contribution.by_tool ?? []).map((t) => (
                          <Tooltip key={t.tool} title={`${t.tool} 工具命中 ${t.count} 次`}>
                            <Tag color="purple" style={{ margin: 0 }}>{t.tool} × {t.count}</Tag>
                          </Tooltip>
                        ))}
                        {contribution.ok_hits < contribution.total_hits && (
                          <Tag color="orange" style={{ margin: 0 }}>失败 {contribution.total_hits - contribution.ok_hits} 次</Tag>
                        )}
                      </Space>
                      {(contribution.top_targets?.length ?? 0) > 0 && (
                        <>
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>被引用概念/实例 TopN（按成功命中计）：</Typography.Text>
                          <div style={{ marginTop: 4 }}>
                            {contribution.top_targets!.map((t) => (
                              <Tag key={t.target} style={{ margin: '2px 6px 2px 0' }}>{t.target} <b>×{t.count}</b></Tag>
                            ))}
                          </div>
                        </>
                      )}
                    </div>
                  ),
                },
              ]}
              style={{ marginTop: 12 }}
            />
          )}
        </>
      )}

      {/* REQ-376③：「spec 外比例」一等指标常驻——余量占比越高=门禁/进化补丁/overlay 编辑作用面越窄 */}
      {recon && recon.triples_in > 0 && (
        <Alert
          type={recon.remainder / recon.triples_in > 0.3 ? 'warning' : 'info'}
          showIcon
          style={{ marginTop: 12 }}
          data-testid="spec-external-ratio"
          title={`spec 外比例 ${Math.round((recon.remainder / recon.triples_in) * 1000) / 10}%（${recon.remainder}/${recon.triples_in} 三元组在余量账本）——质量门禁/进化补丁/overlay 编辑仅作用于 spec 内形态`}
          description="来自最近一次导入对账（REQ-328 恒等式：triples_in = vocabulary + projected + remainder）；占比高时建议在源系统补具名结构声明后重新导入。"
        />
      )}
    </div>
  )
}
