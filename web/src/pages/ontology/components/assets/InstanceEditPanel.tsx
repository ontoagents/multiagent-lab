import { useMemo, useState } from 'react'
import { Button, DatePicker, Input, InputNumber, Modal, Select, Space, Switch, Tag, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import { api } from '../../../../api/client'
import { useUI } from '../../../../store/ui'
import type { Spec, SpecInstance } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-476/M276（Protégé 编辑能力对齐批⑤）：实例编辑表单化——DescPanel 实例卡三段表单
//（属性值按数据属性声明 range 类型化〔复用 REQ-449 AddInstanceModal 口径〕/关系断言增删行/
//   换所属概念），走 instance 元素端点 upsert（attributes/relations 整组替换——前端首次消费）。
// 数据属性适用集=直接 Domain + 沿 parents 链继承（环防护）——与 REQ-449 同口径。
// ---------------------------------------------------------------------------

export function applicableDataProps(spec: Spec, conceptName: string) {
  const byName = new Map((spec.concepts ?? []).map((c) => [c.name, c] as const))
  const domains = new Set<string>()
  let cur: string | undefined = conceptName
  let guard = 0
  while (cur && guard++ < 50) {
    domains.add(cur)
    const c = byName.get(cur)
    cur = (c?.parents ?? [])[0]
  }
  return (spec.data_properties ?? []).filter((d) => !d.domain || domains.has(d.domain))
}

/** 类型化值控件（range → antd 控件）；值为 spec JSON 形态（boolean/number/string）。 */
export function TypedValueInput({
  range,
  value,
  onChange,
}: {
  range: string
  value: unknown
  onChange: (v: unknown) => void
}) {
  switch (range) {
    case 'boolean':
      return <Switch checked={!!value} onChange={(v) => onChange(v)} size="small" />
    case 'integer':
    case 'number':
      return <InputNumber size="small" style={{ width: '100%' }} value={typeof value === 'number' ? value : undefined} onChange={(v) => onChange(v ?? 0)} />
    case 'date':
      return (
        <DatePicker
          size="small"
          style={{ width: '100%' }}
          value={typeof value === 'string' && value ? dayjs(value) : null}
          onChange={(d: Dayjs | null) => onChange(d ? d.format('YYYY-MM-DD') : '')}
        />
      )
    default:
      return <Input size="small" value={value == null ? '' : String(value)} onChange={(e) => onChange(e.target.value)} />
  }
}

export default function InstanceEditPanel({
  ontologyId,
  spec,
  instance,
  onChanged,
}: {
  ontologyId: string
  spec: Spec
  instance: SpecInstance
  onChanged: () => void
}) {
  const { showToast } = useUI()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [concept, setConcept] = useState(instance.concept)
  const [attrs, setAttrs] = useState<Record<string, unknown>>({ ...(instance.attributes ?? {}) })
  const [rels, setRels] = useState<{ rel: string; target: string }[]>([...(instance.relations ?? [])])

  const dps = useMemo(() => applicableDataProps(spec, concept), [spec, concept])
  const conceptNames = (spec.concepts ?? []).map((c) => c.name)
  const relNames = (spec.relations ?? []).map((r) => r.name)
  const instanceNames = (spec.instances ?? []).map((i) => i.name).filter((n) => n !== instance.name)

  const openEdit = () => {
    setConcept(instance.concept)
    setAttrs({ ...(instance.attributes ?? {}) })
    setRels([...(instance.relations ?? [])])
    setOpen(true)
  }

  const submit = async () => {
    if (!concept) {
      showToast('请选择所属概念', 'err')
      return
    }
    for (const r of rels) {
      if (!r.rel || !r.target) {
        showToast('关系断言行存在空值（关系类型/目标实例必填）', 'err')
        return
      }
    }
    setBusy(true)
    try {
      const clean = rels.filter((r) => r.rel && r.target)
      await api.elementUpsert(ontologyId, 'instance', {
        name: instance.name,
        concept,
        attributes: attrs,
        relations: clean,
      })
      showToast(`实例「${instance.name}」已更新（属性 ${Object.keys(attrs).length} 项，断言 ${clean.length} 条）`)
      setOpen(false)
      onChanged()
    } catch (e: any) {
      showToast(e?.message ?? '保存失败', 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ marginTop: 8, display: 'flex', justifyContent: 'flex-end' }}>
      <Button size="small" icon={<PlusOutlined />} onClick={openEdit} data-testid="instance-edit-btn">
        编辑实例（表单）
      </Button>

      <Modal
        open={open}
        centered
        keyboard={false}
        title={`编辑实例「${instance.name}」`}
        width={560}
        okText="保存"
        okButtonProps={{ loading: busy }}
        cancelText="取消"
        onCancel={() => setOpen(false)}
        onOk={submit}
      >
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>所属概念（换类型）</Typography.Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              optionFilterProp="label"
              value={concept || undefined}
              onChange={setConcept}
              options={conceptNames.map((n) => ({ value: n, label: n }))}
              data-testid="instance-concept-select"
            />
          </div>

          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
              属性值（按声明 range 类型化{dps.length === 0 ? '——本概念无声明数据属性' : ''}；未声明键在下方高级区）
            </Typography.Text>
            {dps.length > 0 && (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                {dps.map((d) => (
                  <div key={d.name}>
                    <Typography.Text style={{ fontSize: 12 }}>
                      {d.label || d.name}
                      <Tag style={{ margin: '0 0 0 4px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }} color="cyan">{d.range || 'string'}</Tag>
                    </Typography.Text>
                    <TypedValueInput
                      range={d.range || 'string'}
                      value={attrs[d.name]}
                      onChange={(v) => setAttrs((a) => ({ ...a, [d.name]: v }))}
                    />
                  </div>
                ))}
              </div>
            )}
            <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
              其他键值（JSON 编辑）：
            </Typography.Text>
            <Input.TextArea
              size="small"
              rows={2}
              style={{ fontFamily: 'monospace', fontSize: 12 }}
              value={JSON.stringify(Object.fromEntries(Object.entries(attrs).filter(([k]) => !dps.some((d) => d.name === k))), null, 0)}
              onChange={(e) => {
                try {
                  const v = JSON.parse(e.target.value || '{}')
                  const merged: Record<string, unknown> = {}
                  for (const d of dps) if (attrs[d.name] !== undefined) merged[d.name] = attrs[d.name]
                  setAttrs({ ...merged, ...v })
                } catch {
                  /* 非法 JSON 不即时回写 */
                }
              }}
            />
          </div>

          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>关系断言（关系类型 + 目标实例）</Typography.Text>
            {rels.map((r, i) => (
              <Space key={i} size={4} style={{ display: 'flex', marginBottom: 6 }} align="center">
                <Select
                  size="small"
                  style={{ width: 160 }}
                  showSearch
                  placeholder="关系类型"
                  value={r.rel || undefined}
                  onChange={(v) => setRels((rs) => rs.map((x, j) => (j === i ? { ...x, rel: v } : x)))}
                  options={relNames.map((n) => ({ value: n, label: n }))}
                />
                <span style={{ color: 'var(--c-ink-3)' }}>→</span>
                <Select
                  size="small"
                  style={{ width: 160 }}
                  showSearch
                  placeholder="目标实例"
                  value={r.target || undefined}
                  onChange={(v) => setRels((rs) => rs.map((x, j) => (j === i ? { ...x, target: v } : x)))}
                  options={instanceNames.map((n) => ({ value: n, label: n }))}
                />
                <Button size="small" type="text" danger onClick={() => setRels((rs) => rs.filter((_, j) => j !== i))}>✕</Button>
              </Space>
            ))}
            <Button size="small" type="dashed" icon={<PlusOutlined />} onClick={() => setRels((rs) => [...rs, { rel: '', target: '' }])} data-testid="instance-rel-add">
              添加断言
            </Button>
          </div>
        </Space>
      </Modal>
    </div>
  )
}
