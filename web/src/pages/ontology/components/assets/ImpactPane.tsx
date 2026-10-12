import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Select, Space, Spin, Table, Tag, Typography } from 'antd'
import { api } from '../../../../api/client'
import type { ImpactReport, Ontology } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-278/M103 变更影响分析（66 号 §2⑤ 唯一真缺口，oo plan/blast radius 借鉴自研）：
// 两版本 spec 结构 diff + 引用闭包波及面 + 风险分级（removed+被引用=high / changed+被引用=
// medium / 无引用=low，65 号 A1 判词纪律——每条带引用者清单）+ B1 疑似蕴含边检测
//（67 号扩注判据：A→C 直达边存在 A→B→C 二跳路径，measurement 口径只检测不自动删）。
// 落点=34 号 §3.5 原始诉求位（编辑/发布前影响反馈），挂「质量与演进」分区组。
// ---------------------------------------------------------------------------

const RISK_TAG: Record<string, { color: string; text: string }> = {
  high: { color: 'red', text: 'high' },
  medium: { color: 'orange', text: 'medium' },
  low: { color: 'default', text: 'low' },
}

export default function ImpactPane({ ontology }: { ontology: Ontology }) {
  const [versions, setVersions] = useState<{ version: number }[]>([])
  const curVersion = ontology.version ?? 0
  const [fromV, setFromV] = useState<number | null>(null)
  const [toV, setToV] = useState<number | null>(null)
  const [report, setReport] = useState<ImpactReport | null>(null)
  const [meta, setMeta] = useState<{ from: number; to: number } | null>(null)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const opts = (versions ?? []).map((v) => ({ value: v.version, label: `v${v.version}` }))

  const analyze = useCallback(
    async (from?: number, to?: number) => {
      setLoading(true)
      setErr(null)
      try {
        setReport(await api.impactOntology(ontology.id, from, to))
        if (from && to) setMeta({ from, to })
      } catch (e: any) {
        setErr(e.message)
        setReport(null)
      } finally {
        setLoading(false)
      }
    },
    [ontology.id],
  )

  const loadVersions = useCallback(async () => {
    try {
      const r = await api.listVersions(ontology.id)
      setVersions((r.versions ?? []).map((v) => ({ version: v.version })))
    } catch {
      setVersions([{ version: curVersion }])
    }
  }, [ontology.id, ontology.version])

  useEffect(() => {
    loadVersions()
    // 默认口径：上一版 vs 当前版（发布前影响报告的直接形态）
    if (curVersion > 1) analyze(curVersion - 1, curVersion)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ontology.id, ontology.version])

  const changeKinds = report ? Object.entries(report.changes).filter(([, c]) => (c.added?.length ?? 0) + (c.removed?.length ?? 0) + (c.changed?.length ?? 0) > 0) : []

  return (
    <div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="变更影响分析（REQ-278）"
        description="两版本结构 diff → 引用闭包波及面（悬空引用/子树波及）+ 风险分级（删除被引用概念=high）+ 疑似蕴含边检测（A→C 已有 A→B→C 二跳路径，只检测不自动删）。发布前建议先看本报告。"
      />
      <Space style={{ marginBottom: 12 }} wrap>
        <Select
          size="small"
          style={{ minWidth: 110 }}
          placeholder="from 版本"
          options={opts}
          value={fromV ?? undefined}
          onChange={(v) => setFromV(v)}
        />
        <span>→</span>
        <Select size="small" style={{ minWidth: 110 }} placeholder="to 版本" options={opts} value={toV ?? undefined} onChange={(v) => setToV(v)} />
        <Button
          type="primary"
          size="small"
          loading={loading}
          onClick={() => {
            if (fromV && toV && fromV !== toV) analyze(fromV, toV)
          }}
          disabled={!fromV || !toV || fromV === toV}
        >
          分析
        </Button>
        <Button size="small" onClick={loadVersions}>
          刷新版本
        </Button>
      </Space>
      {err && <Alert type="error" showIcon style={{ marginBottom: 12 }} title={err} />}
      {loading && !report && <Spin style={{ display: 'block', margin: '40px auto' }} />}
      {report && (
        <>
          <div data-testid="impact-summary">
          <Space size={8} wrap style={{ marginBottom: 12 }}>
            {meta && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                v{meta.from} → v{meta.to}
              </Typography.Text>
            )}
            <Tag color="red" style={{ margin: 0 }} data-testid="impact-high">
              high {report.summary.high}
            </Tag>
            <Tag color="orange" style={{ margin: 0 }}>
              medium {report.summary.medium}
            </Tag>
            <Tag style={{ margin: 0 }}>low {report.summary.low}</Tag>
            {changeKinds.map(([kind, c]) => (
              <Tag key={kind} style={{ margin: 0 }}>
                {kind} +{c.added?.length ?? 0}/-{c.removed?.length ?? 0}/~{c.changed?.length ?? 0}
              </Tag>
            ))}
          </Space>
          </div>
          {report.blast.length > 0 && (
            <Card size="small" title={`波及面（${report.blast.length} 条）`} style={{ marginBottom: 12 }}>
              <Table
                rowKey={(r, i) => `${r.kind}:${r.name}:${i}`}
                size="small"
                pagination={false}
                dataSource={report.blast}
                columns={[
                  { title: '风险', dataIndex: 'risk', width: 80, render: (v: string) => <Tag color={RISK_TAG[v]?.color ?? 'default'} style={{ margin: 0 }}>{RISK_TAG[v]?.text ?? v}</Tag> },
                  { title: '类型', dataIndex: 'kind', width: 100 },
                  { title: '对象', dataIndex: 'name', width: 160, ellipsis: true },
                  { title: '动作', dataIndex: 'action', width: 80 },
                  {
                    title: '判词（引用者/波及路径）',
                    dataIndex: 'reasons',
                    render: (rs: string[] | undefined) => (
                      <Typography.Text style={{ fontSize: 12 }}>{(rs ?? []).join('；')}</Typography.Text>
                    ),
                  },
                ]}
                scroll={{ x: 'max-content' }}
              />
            </Card>
          )}
          {report.implied.length > 0 && (
            <Alert
              type="warning"
              showIcon
              title={`疑似蕴含边（B1，${report.implied.length} 条）——直达关系已可由二跳路径到达`}
              description={
                <ul style={{ margin: 0, paddingLeft: 18, maxHeight: 200, overflow: 'auto' }} data-testid="impact-implied">
                  {report.implied.map((s, i) => (
                    <li key={i} style={{ fontSize: 12 }}>
                      {s}
                    </li>
                  ))}
                </ul>
              }
            />
          )}
        </>
      )}
    </div>
  )
}
