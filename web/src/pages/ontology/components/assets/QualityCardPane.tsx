import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Empty, Input, Popconfirm, Select, Space, Spin, Switch, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AlertOutlined, CheckCircleOutlined, CloseCircleOutlined, ReloadOutlined, SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { QualityReport } from '../../../../api/client'
import type { OntoChatCQVerdict, RuntimeProfile } from '../../../../api/types'
import QualityRadar, { radarDimsOf } from '../../../../components/QualityRadar'

// ---------------------------------------------------------------------------
// REQ-156/M-O15 资产详情「质量卡」页签：qualitygate 报告呈现（复用 REQ-171 引擎，零重复建设）。
//   三维评分（完备性/一致性/可维护性，扣分制加权综合）+ 检查项命中明细 + 本体级 strict 门禁开关
//   （开启后保存/导入合并的错误级命中将被 400 拦截——默认宽松仅告警）。
// ---------------------------------------------------------------------------

const SEV_META: Record<string, { color: string; text: string }> = {
  error: { color: 'red', text: '错误' },
  warning: { color: 'orange', text: '告警' },
  info: { color: 'default', text: '提示' },
}

export default function QualityCardPane({ ontologyId, onReport }: { ontologyId: string; onReport?: (r: QualityReport | null) => void }) {
  const [config, setConfig] = useState<{ strict: boolean; reasoning_check?: boolean } | null>(null)
  const [report, setReport] = useState<QualityReport | null>(null)
  const [reportAt, setReportAt] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [err, setErr] = useState<string | null>(null)

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
  }, [ontologyId])

  useEffect(load, [load])

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

  // ---- REQ-274：CQ 覆盖测试快筛（OntoChat 论文路线；默认关=本按钮显式开启；成本=N 次逐条调用）----
  const [covBusy, setCovBusy] = useState(false)
  const [covProgress, setCovProgress] = useState('')
  const [covResult, setCovResult] = useState<{ verdicts: OntoChatCQVerdict[]; passed: number; total: number; pass_rate: number } | null>(null)
  const [covErr, setCovErr] = useState<string | null>(null)
  const [covCQs, setCovCQs] = useState<string[]>([])
  useEffect(() => {
    api
      .getSpec(ontologyId)
      .then((sp: any) => setCovCQs(Array.isArray(sp?.cq) ? sp.cq.map((c: string) => String(c)) : []))
      .catch(() => setCovCQs([]))
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

  const columns: ColumnsType<NonNullable<QualityReport['findings'][number]>> = [
    {
      title: '级别',
      dataIndex: 'severity',
      width: 70,
      render: (v: string) => <Tag color={SEV_META[v]?.color ?? 'default'} style={{ margin: 0 }}>{SEV_META[v]?.text ?? v}</Tag>,
    },
    { title: '检查项', dataIndex: 'title', ellipsis: true },
    { title: '维度', dataIndex: 'dimension', width: 84, render: (v: string) => ({ completeness: '完备性', consistency: '一致性', maintainability: '可维护性' }[v] ?? v) },
    { title: '命中', dataIndex: 'count', width: 60, render: (v: number) => <b>{v}</b> },
    {
      title: '样例',
      dataIndex: 'samples',
      ellipsis: true,
      render: (v: string[]) => <Typography.Text type="secondary" style={{ fontSize: 12 }}>{(v ?? []).slice(0, 3).join('；')}</Typography.Text>,
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


          {/* REQ-274（63 号 §1.3 模块 4）：CQ 覆盖测试快筛——口语化+逐条独立判定（默认关=本卡显式开启；存疑转上方 SPARQL 精判） */}
          <Card size="small" className="work-card" data-testid="cq-coverage-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 8 }}>
              <AlertOutlined style={{ color: 'var(--c-brand)' }} />
              <Typography.Text strong>覆盖测试快筛（口语化判定）</Typography.Text>
              <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1, minWidth: 220 }}>
                {covCQs.length === 0
                  ? '本体 spec.CQ 为空：请先在构建会话中「抽取 CQ」并确认（生成草稿时回写），再回来快筛。'
                  : '本体口语化为纯文本后，逐条能力问题独立判定 Yes/No（每条单独调用防泄漏——成本=N 次模型调用）。默认关闭，点「开始快筛」显式执行；判定「No」的存疑项请到上方「CQ 验收（SPARQL）」卡精判。'}
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
            {covProgress && (
              <div style={{ marginBottom: 8 }}>
                <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>{covProgress}</Typography.Text>
              </div>
            )}
            {covErr && <Alert type="error" showIcon style={{ marginBottom: 8 }} title={covErr} />}
            {covResult && (
              <>
                <Space size={6} style={{ marginBottom: 8 }}>
                  <Tag color="green" data-testid="coverage-pass-tag">通过 {covResult.passed}/{covResult.total}</Tag>
                  <Tag color="geekblue">快筛通过率 {Math.round(covResult.pass_rate * 100)}%</Tag>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    快筛为口语化判定（对可推断不显式的需求偏乐观）——「No」项以上方 SPARQL 精判为准。
                  </Typography.Text>
                </Space>
                <Table
                  size="small"
                  rowKey={(r) => r.cq}
                  dataSource={covResult.verdicts}
                  pagination={{ pageSize: 6 }}
                  columns={[
                    { title: '能力问题', dataIndex: 'cq', ellipsis: true },
                    {
                      title: '判定',
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
                    { title: '解释', dataIndex: 'explanation', ellipsis: true },
                  ]}
                />
              </>
            )}
          </Card>

          <Table
            rowKey={(r) => r.check_id}
            columns={columns}
            dataSource={report.findings}
            size="small"
            pagination={false}
            locale={{ emptyText: '全部检查项通过，无命中' }}
          />
        </>
      )}
    </div>
  )
}
