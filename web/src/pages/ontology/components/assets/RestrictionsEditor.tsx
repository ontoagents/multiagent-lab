import { useMemo, useState } from 'react'
import { Button, InputNumber, Modal, Popconfirm, Select, Space, Tag, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import { useUI } from '../../../../store/ui'
import type { Spec, SpecRestriction } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-472/M272（Protégé 编辑能力对齐批①）：概念限制公理表单化编辑——后端 REQ-376① 五型承载
// （模型/TTL/校验/门禁既有）此前前端零呈现（v1.93 B2「复杂度长在 JSON 里」），本组件在层级浏览
// 描述面板开放编辑：someValuesFrom/allValuesFrom（值域概念）+ cardinality/min/max（基数）；
// 走元素端点 concept upsert 的 restrictions 整组替换；改后经 onSaved 刷新 spec。
// ---------------------------------------------------------------------------

const RTYPES: { value: SpecRestriction['type']; label: string; needTarget: boolean; needValue: boolean }[] = [
  { value: 'some_values_from', label: '存在（someValuesFrom）', needTarget: true, needValue: false },
  { value: 'all_values_from', label: '全称（allValuesFrom）', needTarget: true, needValue: false },
  { value: 'cardinality', label: '恰好（cardinality）', needTarget: false, needValue: true },
  { value: 'min_cardinality', label: '至少（minCardinality）', needTarget: false, needValue: true },
  { value: 'max_cardinality', label: '至多（maxCardinality）', needTarget: false, needValue: true },
]

export function restrictionLabel(r: SpecRestriction): string {
  const t = RTYPES.find((x) => x.value === r.type)
  return t ? `${t.label.split('（')[0]} ${r.property}${r.target ? ` → ${r.target}` : ''}${r.value != null ? ` = ${r.value}` : ''}` : `${r.type} ${r.property}`
}

export default function RestrictionsEditor({
  ontologyId,
  conceptName,
  spec,
  onChanged,
}: {
  ontologyId: string
  conceptName: string
  spec: Spec
  onChanged?: () => void
}) {
  const { showToast } = useUI()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [idx, setIdx] = useState<number>(-1) // -1=新增
  const [form, setForm] = useState<{ type: SpecRestriction['type']; property?: string; target?: string; value?: number }>({ type: 'some_values_from' })

  const concept = useMemo(() => (spec.concepts ?? []).find((c) => c.name === conceptName), [spec, conceptName])
  const restrictions = concept?.restrictions ?? []
  const relationNames = (spec.relations ?? []).map((r) => r.name)
  const conceptNames = (spec.concepts ?? []).map((c) => c.name)
  const meta = RTYPES.find((x) => x.value === form.type)!

  const openCreate = () => {
    setIdx(-1)
    setForm({ type: 'some_values_from' })
    setOpen(true)
  }
  const openEdit = (i: number) => {
    setIdx(i)
    const r = restrictions[i]
    setForm({ type: r.type, property: r.property, target: r.target, value: r.value })
    setOpen(true)
  }

  const save = async (next: SpecRestriction[]) => {
    setBusy(true)
    try {
      await api.elementUpsert(ontologyId, 'concept', { name: conceptName, restrictions: next })
      showToast(idx >= 0 ? '限制公理已更新' : '限制公理已新增')
      setOpen(false)
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  const submit = () => {
    if (!form.property) {
      showToast('请选择限制适用的对象属性', 'err')
      return
    }
    if (meta.needTarget && !form.target) {
      showToast('someValuesFrom/allValuesFrom 需选择值域概念', 'err')
      return
    }
    if (meta.needValue && (form.value == null || form.value < 0)) {
      showToast('基数需为 ≥0 的整数', 'err')
      return
    }
    const item: SpecRestriction = { type: form.type, property: form.property }
    if (meta.needTarget && form.target) item.target = form.target
    if (meta.needValue) item.value = form.value ?? 0
    const next = [...restrictions]
    if (idx >= 0) next[idx] = item
    else next.push(item)
    save(next)
  }

  const remove = (i: number) => {
    save(restrictions.filter((_, j) => j !== i))
  }

  return (
    <div className="onto-hier-row">
      <span className="onto-hier-label">限制公理</span>
      <span>
        {restrictions.length > 0 ? (
          restrictions.map((r, i) => (
            <Tag key={`${r.type}-${r.property}-${i}`} color="orange" style={{ margin: '0 6px 4px 0' }}>
              {restrictionLabel(r)}
              <span style={{ marginLeft: 4, cursor: 'pointer' }} onClick={() => openEdit(i)} data-testid={`restriction-edit-${i}`}>✎</span>
              <Popconfirm title={`删除限制「${restrictionLabel(r)}」？`} okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(i)}>
                <span style={{ marginLeft: 4, cursor: 'pointer' }}>✕</span>
              </Popconfirm>
            </Tag>
          ))
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>
        )}
        <Button size="small" type="dashed" icon={<PlusOutlined />} style={{ margin: '0 0 4px 4px', height: 22, fontSize: 11 }} onClick={openCreate} data-testid="restriction-add-btn">
          限制
        </Button>
      </span>

      <Modal
        open={open}
        centered
        keyboard={false} /* Select 下拉 Esc 冒泡会误触 onCancel 关模态——表单模态禁 Esc 关闭 */
        title={idx >= 0 ? '编辑限制公理' : `新增限制公理（概念「${conceptName}」）`}
        width={480}
        onCancel={() => setOpen(false)}
        confirmLoading={busy}
        onOk={submit}
        okText="保存"
        cancelText="取消"
      >
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>限制类型</Typography.Text>
            <Select style={{ width: '100%' }} value={form.type} onChange={(v) => setForm((f) => ({ ...f, type: v }))} options={RTYPES.map((x) => ({ value: x.value, label: x.label }))} data-testid="restriction-type-select" />
          </div>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>对象属性（已声明关系）</Typography.Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              optionFilterProp="label"
              placeholder={relationNames.length ? '选择对象属性' : '暂无已声明关系——先到「关系与属性」声明'}
              value={form.property}
              onChange={(v) => setForm((f) => ({ ...f, property: v }))}
              options={relationNames.map((n) => ({ value: n, label: n }))}
            />
          </div>
          {meta.needTarget && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>值域概念</Typography.Text>
              <Select
                style={{ width: '100%' }}
                showSearch
                optionFilterProp="label"
                placeholder="选择值域概念"
                value={form.target}
                onChange={(v) => setForm((f) => ({ ...f, target: v }))}
                options={conceptNames.map((n) => ({ value: n, label: n }))}
              />
            </div>
          )}
          {meta.needValue && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>基数（≥0）</Typography.Text>
              <InputNumber min={0} style={{ width: '100%' }} value={form.value} onChange={(v) => setForm((f) => ({ ...f, value: v ?? 0 }))} />
            </div>
          )}
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            语义提示：存在=该类实例经此属性至少指向一个值域个体；全称=所有指向必在值域内；基数=实例该属性断言数的下/上界（质量门禁 restriction_cardinality 对照实例断言计数）。
          </Typography.Text>
        </Space>
      </Modal>
    </div>
  )
}
