import { useState } from 'react'
import { Button, Card, Input, Space, Tag, Typography } from 'antd'
import ReactDiffViewer from 'react-diff-viewer-continued'
import type { QualityReport, Spec } from '../../../api/types'

// REQ-503/M311②：逐 Finding 提案卡（小步提案化——承 69 号 A4 差量修正转正；替代整稿重生成主路径）。
// 范围口径（REQ 行）：局部客观项（孤立概念/重复）产出可采纳提案（改前/改后对照+采纳/修改/忽略三裁决）；
// 层级结构类 findings 仅提示不代改（hintOnly——81 号「整份重生成不可控」结构层根源教训）。
// 提案语义=从草案移除目标概念（采纳即裁剪工作副本；修改=编辑移除清单后采纳；忽略=跳过）。

interface Proposal {
  message: string
  severity: string
  targets: string[] // 建议移除的概念名（孤立/未连接类；空=仅提示）
  hintOnly: boolean
}

// 运行时 finding 文本=REST title 字段（Go Finding json tag；QualityReportCard f.message 同源读取防御）
const textOf = (f: { message?: string; title?: string }) => f.message || f.title || ''
// samples 形如 concepts[2](孤立品甲)——提取概念名
const conceptOf = (sample: string): string | null => {
  const m = /concepts\[\d+\]\((.+)\)$/.exec(sample)
  return m ? m[1] : null
}

export default function FindingProposalCards({
  report,
  spec,
  onApply,
}: {
  report: QualityReport
  spec: Spec
  onApply: (next: Spec, appliedMessages: string[]) => void
}) {
  const [ignored, setIgnored] = useState<Set<number>>(new Set())
  const [applied, setApplied] = useState<Set<number>>(new Set())
  const [editing, setEditing] = useState<number | null>(null)
  const [editText, setEditText] = useState('')

  const findings = report.findings ?? []
  const proposals: Proposal[] = findings.map((f) => {
    const text = textOf(f)
    const isStruct = /(循环|悬空|父概念引用|实例无类型|层级)/.test(text)
    let targets: string[] = []
    if (!isStruct && /(未连接概念|孤立)/.test(text) && f.samples?.length) {
      // 孤立/未连接概念提案：移除样本中仍存在于草案的概念（samples=concepts[i](名) 路径级样例）
      targets = f.samples
        .map(conceptOf)
        .filter((n): n is string => !!n)
        .filter((n) => (spec.concepts ?? []).some((c) => c.name === n))
    }
    return { message: text, severity: f.severity, targets, hintOnly: isStruct || targets.length === 0 }
  })

  const applyProposal = (idx: number, targets: string[]) => {
    const next = { ...spec, concepts: (spec.concepts ?? []).filter((c) => !targets.includes(c.name)) }
    setApplied(new Set(applied).add(idx))
    setEditing(null)
    onApply(next, [findings[idx]?.message ?? ''])
  }

  if (findings.length === 0) return null
  return (
    <Card size="small" style={{ marginBottom: 10 }} data-testid="finding-proposals" title="逐条提案（小步修正——局部客观项可采纳，结构类仅提示）">
      {proposals.map((p, idx) => {
        if (ignored.has(idx)) return null
        const before = JSON.stringify({ concepts: (spec.concepts ?? []).map((c) => c.name) }, null, 2)
        const afterTargets = p.targets
        const after = JSON.stringify({ concepts: (spec.concepts ?? []).filter((c) => !afterTargets.includes(c.name)).map((c) => c.name) }, null, 2)
        return (
          <Card key={idx} size="small" style={{ marginBottom: 8 }} data-testid={`proposal-card-${idx}`}>
            <Space size={6} wrap style={{ marginBottom: 4 }}>
              <Tag color={p.severity === 'error' ? 'red' : p.severity === 'warning' ? 'orange' : 'default'} style={{ margin: 0, fontSize: 10 }}>
                {p.severity}
              </Tag>
              <Typography.Text style={{ fontSize: 12.5 }}>{p.message}</Typography.Text>
              {applied.has(idx) && <Tag color="green" style={{ margin: 0, fontSize: 10 }}>已采纳</Tag>}
            </Space>
            {p.hintOnly ? (
              <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
                结构类建议仅提示，不代改——请在「结构（层级树）」视图或资产层级浏览中人工调整。
              </Typography.Text>
            ) : applied.has(idx) ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>已从草案移除：{afterTargets.join('、')}</Typography.Text>
            ) : (
              <>
                <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                  提案：从草案移除 {afterTargets.join('、')}（消解该问题）
                </Typography.Text>
                <div style={{ fontSize: 11, marginBottom: 6, maxHeight: 220, overflow: 'auto' }}>
                  <ReactDiffViewer oldValue={before} newValue={after} splitView={false} useDarkTheme={false} />
                </div>
                {editing === idx ? (
                  <div style={{ marginBottom: 6 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 2 }}>
                      编辑移除清单（每行一个概念名）：
                    </Typography.Text>
                    <Input.TextArea rows={Math.min(6, afterTargets.length + 1)} value={editText} onChange={(e) => setEditText(e.target.value)} style={{ marginBottom: 4 }} />
                    <Button
                      size="small"
                      type="primary"
                      onClick={() => applyProposal(idx, editText.split('\n').map((x) => x.trim()).filter(Boolean))}
                    >
                      按清单采纳
                    </Button>
                    <Button size="small" style={{ marginLeft: 6 }} onClick={() => setEditing(null)}>
                      取消
                    </Button>
                  </div>
                ) : (
                  <Space size={6}>
                    <Button size="small" type="primary" data-testid={`proposal-adopt-${idx}`} onClick={() => applyProposal(idx, afterTargets)}>
                      采纳
                    </Button>
                    <Button size="small" data-testid={`proposal-edit-${idx}`} onClick={() => { setEditing(idx); setEditText(afterTargets.join('\n')) }}>
                      修改
                    </Button>
                    <Button size="small" data-testid={`proposal-ignore-${idx}`} onClick={() => setIgnored(new Set(ignored).add(idx))}>
                      忽略
                    </Button>
                  </Space>
                )}
              </>
            )}
          </Card>
        )
      })}
    </Card>
  )
}
