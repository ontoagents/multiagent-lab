import { Button, Card, Checkbox, Collapse, Input, Space, Tag, Typography } from 'antd'
import { DeleteOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-337/M137：OntoChat 五阶段视图分离——cq（能力问题）阶段视图。
// REQ-273 聚类确认卡 + REQ-272 CQ 候选确认卡自 OntoChatFlow 迁出（交互零变更；
// REQ-338 的入库前增量确认复用同款 checkbox 交互模式）。
// ---------------------------------------------------------------------------

export interface CqDraftRow {
  text: string
  origin?: string
  checked: boolean
}

export interface ClusterDraftGroup {
  label: string
  rows: { text: string; checked: boolean }[]
}

export function ClusterConfirmCard({
  clusterDraft,
  turning,
  onChange,
  onApply,
  onDiscard,
}: {
  clusterDraft: ClusterDraftGroup[]
  turning: boolean
  onChange: (next: ClusterDraftGroup[]) => void
  onApply: () => void
  onDiscard: () => void
}) {
  return (
    <Card size="small" style={{ marginTop: 10 }} title="CQ 主题聚类（人工确认后应用）">
      <Collapse
        size="small"
        defaultActiveKey={clusterDraft.map((_, i) => String(i))}
        items={clusterDraft.map((cl, i) => ({
          key: String(i),
          label: `${cl.label}（${cl.rows.filter((r) => r.checked).length}/${cl.rows.length}）`,
          children: cl.rows.map((row, j) => (
            <div key={j} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 4 }}>
              <Checkbox
                checked={row.checked}
                onChange={(e) =>
                  onChange(
                    clusterDraft.map((c2, k) =>
                      k === i ? { ...c2, rows: c2.rows.map((r2, m) => (m === j ? { ...r2, checked: e.target.checked } : r2)) } : c2,
                    ),
                  )
                }
              />
              <Typography.Text style={{ fontSize: 13, flex: 1 }}>{row.text}</Typography.Text>
            </div>
          )),
        }))}
      />
      <Space style={{ marginTop: 8 }}>
        <Button size="small" type="primary" loading={turning} onClick={onApply}>
          应用去重结果（写回 {clusterDraft.reduce((n, cl) => n + cl.rows.filter((r) => r.checked && r.text.trim()).length, 0)} 条）
        </Button>
        <Button size="small" onClick={onDiscard}>
          放弃
        </Button>
      </Space>
      <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
        诚实边界：聚类仅供参考分组、不能单独支撑完整分析——写回后生成草稿仍以全部勾选问题为能力问题依据（spec.CQ 可追溯）。
      </Typography.Paragraph>
    </Card>
  )
}

export function CqCandidateCard({
  cqDraft,
  turning,
  onChange,
  onConfirm,
  onAdd,
  onDiscard,
}: {
  cqDraft: CqDraftRow[]
  turning: boolean
  onChange: (next: CqDraftRow[]) => void
  onConfirm: () => void
  onAdd: () => void
  onDiscard?: () => void
}) {
  const pickedCount = cqDraft.filter((c) => c.checked && c.text.trim()).length
  return (
    <Card size="small" style={{ marginTop: 10 }} title={`能力问题候选确认（勾选 ${pickedCount}/${cqDraft.length}）`}>
      {cqDraft.map((row, i) => (
        <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6 }}>
          <Checkbox checked={row.checked} onChange={(e) => onChange(cqDraft.map((r2, j2) => (j2 === i ? { ...r2, checked: e.target.checked } : r2)))} />
          <Input
            size="small"
            value={row.text}
            onChange={(e) => onChange(cqDraft.map((r2, j2) => (j2 === i ? { ...r2, text: e.target.value } : r2)))}
            style={{ flex: 1 }}
          />
          {row.origin && <Tag style={{ margin: 0, fontSize: 11 }}>{row.origin}</Tag>}
          <Button size="small" type="text" icon={<DeleteOutlined />} aria-label="删除候选" onClick={() => onChange(cqDraft.filter((_, j2) => j2 !== i))} />
        </div>
      ))}
      <Space style={{ marginTop: 4 }} size={8}>
        <Button size="small" onClick={onAdd}>
          手动添加
        </Button>
        <Button size="small" type="primary" loading={turning} disabled={pickedCount === 0} onClick={onConfirm}>
          确认写入 {pickedCount} 条
        </Button>
        {onDiscard && (
          <Button size="small" onClick={onDiscard}>
            放弃
          </Button>
        )}
      </Space>
      <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
        确认后生成草稿将据此补充建模，并回写 spec.CQ 入本体资产（可追溯）。
      </Typography.Paragraph>
    </Card>
  )
}
