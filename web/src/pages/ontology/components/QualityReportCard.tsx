import { Alert, Button, Card, Collapse, Progress, Space, Tag, Typography } from 'antd'
import { ThunderboltOutlined } from '@ant-design/icons'
import type { QualityFinding, QualityReport } from '../../../api/types'

// ---------------------------------------------------------------------------
// REQ-312/M116：草稿质量报告卡——完整 qualitygate.Report 结构化渲染（此前仅 toast 摘要）。
// 三维分（完备性 0.35/一致性 0.45/可维护性 0.2 扣分制）+Findings 按严重级分组+Stats 规模
// +新旧对照（prev 存在时显示维度分 delta 与修复说明）+报告驱动优化入口（onOptimize）。
// AI 创建（S1Source）与 OntoChat draft 阶段共用。
// ---------------------------------------------------------------------------

const SEV: Record<string, { color: string; label: string }> = {
  error: { color: 'red', label: '错误' },
  warning: { color: 'orange', label: '告警' },
  info: { color: 'blue', label: '提示' },
}

/** findings → 修正意见文本（报告驱动优化：作为 extra 上下文回喂重生成，REQ-312 A2） */
export function qualityFindingsText(report?: QualityReport | null): string {
  if (!report?.findings?.length) return ''
  return report.findings
    .map((f) => `- [${SEV[f.severity]?.label ?? f.severity}] ${f.message}${f.samples?.length ? `（如 ${f.samples[0]}）` : ''}`)
    .join('\n')
}

function DimBar({ label, value, before }: { label: string; value?: number; before?: number }) {
  if (value == null) return null
  const d = before != null ? Math.round((value - before) * 10) / 10 : null
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
      <span style={{ width: 60, fontSize: 12, flexShrink: 0 }}>{label}</span>
      <Progress percent={Math.max(0, Math.min(100, value))} size="small" strokeColor={value >= 80 ? '#52c41a' : value >= 50 ? '#faad14' : '#ff4d4f'} style={{ flex: 1, margin: 0 }} />
      <span style={{ width: 90, fontSize: 12, flexShrink: 0, textAlign: 'right' }}>
        {value.toFixed(1)}
        {d != null && d !== 0 && (
          <Typography.Text type={d > 0 ? 'success' : 'danger'} style={{ fontSize: 11, marginLeft: 4 }}>
            {d > 0 ? `+${d}` : d}
          </Typography.Text>
        )}
      </span>
    </div>
  )
}

function FindingList({ items }: { items: QualityFinding[] }) {
  return (
    <ul style={{ margin: 0, paddingLeft: 18, maxHeight: 220, overflowY: 'auto', fontSize: 12 }}>
      {items.map((f, i) => (
        <li key={i} style={{ marginBottom: 3 }}>
          {f.message}
          {f.samples?.slice(0, 2).map((s, j) => (
            <Tag key={j} style={{ marginInlineStart: 6, fontSize: 11 }}>
              {s}
            </Tag>
          ))}
        </li>
      ))}
    </ul>
  )
}

export default function QualityReportCard({
  report,
  prev,
  onOptimize,
  optimizing,
  title,
}: {
  report: QualityReport
  prev?: QualityReport | null
  onOptimize?: () => void
  optimizing?: boolean
  title?: string
}) {
  if (!report?.score) return null
  const { score } = report
  const groups: Record<string, QualityFinding[]> = { error: [], warning: [], info: [] }
  for (const f of report.findings ?? []) (groups[f.severity] ??= []).push(f)
  const hasGroups = groups.error.length + groups.warning.length + groups.info.length > 0
  const fixedCount =
    prev?.findings?.filter((p) => !report.findings?.some((f) => f.message === p.message)).length ?? null
  const newCount = prev?.findings?.length == null ? null : report.findings?.filter((f) => !prev.findings?.some((p) => p.message === f.message)).length ?? 0
  return (
    <Card
      size="small"
      title={title ?? '草案质量报告'}
      extra={
        onOptimize && (
          <Button size="small" icon={<ThunderboltOutlined />} loading={optimizing} onClick={onOptimize} data-testid="quality-optimize-btn">
            按报告优化
          </Button>
        )
      }
      data-testid="quality-report-card"
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap', marginBottom: 6 }}>
        <Space size={6}>
          <Tag color={report.error_count > 0 ? 'red' : report.warning_count > 0 ? 'orange' : 'green'} style={{ margin: 0 }}>
            错误 {report.error_count}
          </Tag>
          <Tag color={report.warning_count > 0 ? 'orange' : 'default'} style={{ margin: 0 }}>
            告警 {report.warning_count}
          </Tag>
          {report.info_count != null && report.info_count > 0 && (
            <Tag style={{ margin: 0 }}>提示 {report.info_count}</Tag>
          )}
        </Space>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>
          综合 {score.overall?.toFixed?.(1) ?? score.overall}
          {prev?.score?.overall != null && `（上次 ${prev.score.overall?.toFixed?.(1)}）`}
          ，扣分制：错误 -10 / 告警 -3 / 提示 -1（完备性 0.35 + 一致性 0.45 + 可维护性 0.2）
        </Typography.Text>
      </div>
      <DimBar label="完备性" value={score.completeness} before={prev?.score?.completeness} />
      <DimBar label="一致性" value={score.consistency} before={prev?.score?.consistency} />
      <DimBar label="可维护性" value={score.maintainability} before={prev?.score?.maintainability} />
      {prev?.score?.overall != null && (fixedCount != null || newCount != null) && (fixedCount || newCount) !== 0 && (
        <Alert
          type={(fixedCount ?? 0) > 0 && score.overall >= (prev.score.overall ?? 0) ? 'success' : 'info'}
          showIcon
          style={{ margin: '6px 0' }}
          title={`与上次生成对照：修复 ${fixedCount ?? 0} 个问题${newCount ? `，新引入 ${newCount} 个` : ''}`}
        />
      )}
      {hasGroups && (
        <Collapse
          size="small"
          style={{ marginTop: 4 }}
          items={(['error', 'warning', 'info'] as const)
            .filter((k) => groups[k].length > 0)
            .map((k) => ({
              key: k,
              label: `${SEV[k].label}（${groups[k].length}）`,
              children: <FindingList items={groups[k]} />,
            }))}
        />
      )}
      {report.stats && (
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
          规模：概念 {report.stats.concepts} · 关系 {report.stats.relations} · 实例 {report.stats.instances}
          {report.stats.data_properties ? ` · 数据属性 ${report.stats.data_properties}` : ''}
          {report.stats.axioms ? ` · 公理 ${report.stats.axioms}` : ''}
        </Typography.Text>
      )}
    </Card>
  )
}
