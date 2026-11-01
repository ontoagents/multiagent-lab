import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Checkbox, Empty, Input, Modal, Popconfirm, Select, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { companionApi, type CompanionGraph, type PromotionProposal } from '../../../../api/companion'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// REQ-546/M353（决策结构化借鉴批③；105 号 P1-5）：learning proposal 暂存区——晋升
// 「提案→人批→入库」显式化（原 REQ-529 直接 PUT 退役）。本面=伴生型资产详情「晋升提案」
// 分区：①目标本体+实体勾选（REQ-529 挂账「逐条勾选」合流）+提案理由→发起提案；
// ②pending 提案裁决卡（approve=重取伴生图取交集入库〔漂移防护〕/ reject=拒绝理由留痕，
// 被拒提案即 REQ-544 reject route 素材）；③已裁决历史回看（审计半边）。
// 诚实边界：approval.* 为对话内 run 事件，promote 在对话外无 run 上下文——v1 不接 SSE，
// 复用「提案→人批→入库」交互形态；伴生图漂移以交集为准（approve 时点活跃集）。
// ---------------------------------------------------------------------------

export default function CompanionPromotePane({ ontologyId, compGraph, targets, onChanged }: {
  ontologyId: string
  compGraph: CompanionGraph | null
  /** 候选晋升目标本体（spec 系——非伴生绑定型） */
  targets: { id: string; name: string }[]
  /** 发起/裁决后通知宿主刷新（伴生图计数等） */
  onChanged?: () => void
}) {
  const { showToast } = useUI()
  const [targetId, setTargetId] = useState<string>()
  const [selected, setSelected] = useState<string[]>([])
  const [note, setNote] = useState('')
  const [creating, setCreating] = useState(false)
  const [proposals, setProposals] = useState<PromotionProposal[] | null>(null)
  const [rejecting, setRejecting] = useState<PromotionProposal | null>(null)
  const [rejectNote, setRejectNote] = useState('')
  const [busyId, setBusyId] = useState('')

  const activeLabels = useMemo(
    () => (compGraph?.nodes ?? []).map((n) => n.label).filter(Boolean),
    [compGraph],
  )

  const reload = useCallback(async () => {
    try {
      const list = await companionApi.listPromotionProposals({ source_ontology_id: ontologyId })
      setProposals(list)
    } catch (e: any) {
      showToast(e?.message ?? '提案清单加载失败', 'err')
    }
  }, [ontologyId, showToast])

  useEffect(() => {
    setProposals(null)
    setSelected([])
    setNote('')
    setTargetId(undefined)
    void reload()
  }, [ontologyId, reload])

  const create = async () => {
    if (!targetId) {
      showToast('先选择晋升目标本体', 'err')
      return
    }
    setCreating(true)
    try {
      const p = await companionApi.promoteCompanion(ontologyId, {
        target_ontology_id: targetId,
        entity_ids: selected.length > 0 && selected.length < activeLabels.length ? selected : undefined,
        note: note.trim() || undefined,
      })
      showToast(`提案已创建（实体 ${p.entity_count}·关系 ${p.relation_count}），待人工裁决`)
      setNote('')
      await reload()
      onChanged?.()
    } catch (e: any) {
      showToast(e?.message ?? '提案创建失败', 'err')
    } finally {
      setCreating(false)
    }
  }

  const decide = async (p: PromotionProposal, action: 'approve' | 'reject', noteText = '') => {
    setBusyId(p.id)
    try {
      await companionApi.decidePromotionProposal(p.id, action, noteText)
      showToast(action === 'approve' ? '已批准并入库目标本体草稿' : '已拒绝（理由已留痕）')
      setRejecting(null)
      setRejectNote('')
      await reload()
      onChanged?.()
    } catch (e: any) {
      showToast(e?.message ?? '裁决失败', 'err')
    } finally {
      setBusyId('')
    }
  }

  const pending = (proposals ?? []).filter((p) => p.status === 'pending')
  const decided = (proposals ?? []).filter((p) => p.status !== 'pending')

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div>
        <Typography.Title level={5} style={{ margin: 0 }}>晋升提案（提案 → 人批 → 入库）</Typography.Title>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          晋升不再直接写入目标本体——先创建提案，批准时按当时伴生图活跃集取交集入库（漂移防护）；
          拒绝须留理由（沉淀为决策素材）。
        </Typography.Text>
      </div>

      {/* ① 发起提案：目标本体+实体勾选+理由 */}
      <div style={{ border: '1px solid var(--c-border, #eee)', borderRadius: 6, padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: 'var(--c-ink-2, #666)', whiteSpace: 'nowrap' }}>目标本体</span>
          <Select
            style={{ minWidth: 220 }}
            placeholder="选择写入的正式本体"
            value={targetId}
            onChange={setTargetId}
            options={targets.map((o) => ({ value: o.id, label: o.name }))}
            showSearch
            optionFilterProp="label"
          />
        </div>
        {activeLabels.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            伴生图暂无活跃实体（引擎不可达或未沉淀）——无法发起提案。
          </Typography.Text>
        ) : (
          <div>
            <div style={{ fontSize: 12, color: 'var(--c-ink-2, #666)', marginBottom: 4 }}>
              勾选要晋升的实体（缺省=全选 {activeLabels.length} 个；关系随两端实体自动携带）
            </div>
            <Checkbox.Group
              value={selected}
              onChange={(v) => setSelected(v as string[])}
              options={activeLabels.map((l) => ({ value: l, label: l }))}
              style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 12px' }}
            />
          </div>
        )}
        <Input.TextArea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="提案理由（为什么晋升——选填，随提案留痕）"
          autoSize={{ minRows: 1, maxRows: 3 }}
          maxLength={300}
        />
        <div>
          <Button type="primary" size="small" loading={creating} onClick={create}>发起晋升提案</Button>
        </div>
      </div>

      {/* ② 待裁决 */}
      <div>
        <div style={{ fontSize: 13, margin: '4px 0' }}>待裁决（{pending.length}）</div>
        {pending.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {proposals === null ? '提案清单加载中…' : '暂无待裁决提案。'}
          </Typography.Text>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {pending.map((p) => (
              <div key={p.id} style={{ border: '1px solid var(--c-border, #eee)', borderRadius: 6, padding: 10, display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ fontSize: 12, color: 'var(--c-ink-2, #666)' }}>
                  实体 {p.entity_count} · 关系 {p.relation_count} · 发起于 {(p.created_at || '').slice(0, 16).replace('T', ' ')}
                  {p.note ? ` · 理由：${p.note}` : ''}
                </div>
                <div style={{ fontSize: 12, fontFamily: 'monospace', color: 'var(--c-ink-2, #666)' }}>
                  {p.payload.entities.join('、') || '（无实体）'}
                </div>
                <div style={{ display: 'flex', gap: 8 }}>
                  <Popconfirm
                    title="批准后将写入目标本体草稿并走统一校验（校验失败提案保持待裁决）"
                    okText="批准"
                    cancelText="取消"
                    onConfirm={() => void decide(p, 'approve')}
                  >
                    <Button size="small" type="primary" loading={busyId === p.id}>批准入库</Button>
                  </Popconfirm>
                  <Button size="small" onClick={() => { setRejecting(p); setRejectNote('') }}>拒绝…</Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ③ 已裁决历史（审计回看） */}
      {decided.length > 0 && (
        <div>
          <div style={{ fontSize: 13, margin: '4px 0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>已裁决（{decided.length}）</span>
            <Button size="small" type="text" icon={<ReloadOutlined />} onClick={() => void reload()} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {decided.map((p) => (
              <div key={p.id} style={{ fontSize: 12, color: 'var(--c-ink-2, #666)' }}>
                <span style={{ color: p.status === 'approved' ? 'var(--c-ok, #389e0d)' : '#bfbfbf' }}>
                  {p.status === 'approved' ? '已批准' : '已拒绝'}
                </span>
                {' · '}实体 {p.entity_count}·关系 {p.relation_count}
                {(p.decided_at || '').slice(0, 16).replace('T', ' ')}
                {p.decided_note ? ` · ${p.decided_note}` : ''}
              </div>
            ))}
          </div>
        </div>
      )}

      {proposals !== null && proposals.length === 0 && (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<span style={{ fontSize: 12, color: 'var(--c-ink-2, #666)' }}>暂无提案历史</span>} />
      )}

      {/* 拒绝理由 Modal（留痕=reject route 素材，衔接 REQ-544 decisions[]） */}
      <Modal
        title="拒绝提案"
        open={!!rejecting}
        onCancel={() => setRejecting(null)}
        onOk={() => rejecting && void decide(rejecting, 'reject', rejectNote.trim())}
        okText="拒绝（留痕）"
        cancelText="取消"
        okButtonProps={{ disabled: !rejectNote.trim(), loading: busyId === rejecting?.id }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            拒绝理由将随提案留痕——被拒提案是「何时不应晋升」的决策素材（reject route）。
          </Typography.Text>
          <Input.TextArea
            value={rejectNote}
            onChange={(e) => setRejectNote(e.target.value)}
            placeholder="拒绝理由（必填——如与目标本体定位不符/与既有概念冲突）"
            autoSize={{ minRows: 2, maxRows: 4 }}
            maxLength={300}
          />
        </div>
      </Modal>
    </div>
  )
}
