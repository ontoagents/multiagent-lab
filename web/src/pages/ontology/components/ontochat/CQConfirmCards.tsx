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

/** REQ-342/OC-1：净化统计（拆分率/抽象率的分子分母，供人工核验） */
export interface CqPurifyStats {
  compound_found: number
  split_out: number
  named_found: number
  abstracted: number
}

/** REQ-342/OC-3：paraphrase 去重明细（保留哪条、合并了哪些） */
export interface CqMergeGroup {
  keep: string
  duplicates: string[]
}

export interface ClusterDraftGroup {
  label: string
  rows: { text: string; checked: boolean }[]
}

export function ClusterConfirmCard({
  clusterDraft,
  turning,
  mergedGroups,
  dedupSkipped,
  onChange,
  onApply,
  onDiscard,
}: {
  clusterDraft: ClusterDraftGroup[]
  turning: boolean
  /** REQ-342/OC-3：paraphrase 去重明细（有合并项时逐组展示保留/合并对照） */
  mergedGroups?: CqMergeGroup[]
  dedupSkipped?: boolean
  onChange: (next: ClusterDraftGroup[]) => void
  onApply: () => void
  onDiscard: () => void
}) {
  const merged = (mergedGroups ?? []).filter((g) => g.duplicates?.length)
  return (
    <Card size="small" style={{ marginTop: 10 }} title="CQ 主题聚类（人工确认后应用）">
      {merged.length > 0 && (
        <div style={{ marginBottom: 8 }} data-testid="cq-merge-detail">
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>去重明细（语义等价合并，保留最清晰表述）：</Typography.Text>
          {merged.map((g, i) => (
            <div key={i} style={{ fontSize: 12, marginInlineStart: 12 }}>
              保留「{g.keep}」← 合并 {g.duplicates.join('、')}
            </div>
          ))}
        </div>
      )}
      {dedupSkipped && (
        <Typography.Paragraph type="warning" style={{ fontSize: 12, marginBottom: 8 }}>
          去重步降级（输出不合法）——本结果只聚类未去重，可人工在下方勾选中剔除重复项。
        </Typography.Paragraph>
      )}
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
  purification,
  purifyDegraded,
  onChange,
  onConfirm,
  onAdd,
  onDiscard,
}: {
  cqDraft: CqDraftRow[]
  turning: boolean
  /** REQ-342/OC-1：净化统计行（识别复合 N 拆出 M / 含专名 K 抽象 L）；降级时提示用原清单 */
  purification?: CqPurifyStats | null
  purifyDegraded?: boolean
  onChange: (next: CqDraftRow[]) => void
  onConfirm: () => void
  onAdd: () => void
  onDiscard?: () => void
}) {
  const pickedCount = cqDraft.filter((c) => c.checked && c.text.trim()).length
  return (
    <Card size="small" style={{ marginTop: 10 }} title={`能力问题候选确认（勾选 ${pickedCount}/${cqDraft.length}）`}>
      {purification && (
        <div style={{ marginBottom: 8 }} data-testid="cq-purify-stats">
          <Tag color="geekblue" style={{ fontSize: 11 }}>净化：复合 {purification.compound_found} 条→拆出 {purification.split_out} 条</Tag>
          <Tag color="cyan" style={{ fontSize: 11 }}>含专名 {purification.named_found} 条→抽象 {purification.abstracted} 条</Tag>
        </div>
      )}
      {purifyDegraded && (
        <Typography.Paragraph type="warning" style={{ fontSize: 12, marginBottom: 8 }}>
          净化步降级（输出不合法）——本清单为抽取原稿未净化，可手动编辑后再确认。
        </Typography.Paragraph>
      )}
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
