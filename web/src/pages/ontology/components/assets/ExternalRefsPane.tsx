import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Popconfirm, Select, Space, Table, Tag, Tooltip, Input } from 'antd'
import { api } from '../../../../api/client'
import type { ExternalRef } from '../../../../api/types'

/** REQ-262/M256：跨本体引用登记「关联本体」区块（external_refs 单源，D-O27 三形态收敛）。
 *  登记表 + 悬空引用提醒（目标本体删除/目标概念不存在——只提醒不阻断，68 号 §3.4）+ 删除摘除。
 *  三形态呈现：手动登记（本表单）/ owl:imports 装载侧自动登记（justification=import_statement，
 *  note 注明自动登记）/ 导入外引 IRI（to_onto_id 空=外部词表登记态）。 */

const REF_KINDS = [
  { value: 'import', label: 'import（依赖对方概念体系）' },
  { value: 'alignment', label: 'alignment（语义对应）' },
  { value: 'reuse', label: 'reuse（复用对方概念）' },
]
const PREDICATES = ['exactMatch', 'closeMatch', 'broadMatch', 'narrowMatch', 'relatedMatch'].map((v) => ({ value: v, label: v }))

export default function ExternalRefsPane({ ontologyId, concepts, ontos }: { ontologyId: string; concepts: string[]; ontos: { id: string; name: string }[] }) {
  const [refs, setRefs] = useState<ExternalRef[]>([])
  const [dangling, setDangling] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [form, setForm] = useState<{ from_concept?: string; to_onto_id?: string; to_concept?: string; ref_kind?: string; predicate?: string; note?: string }>({})

  const reload = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const d = await api.listExternalRefs(ontologyId)
      setRefs(d.refs ?? [])
      setDangling(d.dangling ?? [])
    } catch (e: any) {
      setErr(e.message ?? String(e))
    } finally {
      setLoading(false)
    }
  }, [ontologyId])

  useEffect(() => { reload() }, [reload])

  const add = async () => {
    if (!form.from_concept || !form.to_concept || !form.ref_kind) return
    try {
      await api.createExternalRef(ontologyId, {
        from_concept: form.from_concept, to_onto_id: form.to_onto_id ?? '', to_concept: form.to_concept,
        ref_kind: form.ref_kind, predicate: form.ref_kind === 'alignment' ? (form.predicate ?? '') : '',
        note: form.note ?? '',
      })
      setForm({})
      reload()
    } catch (e: any) {
      setErr(e.message ?? String(e))
    }
  }
  const remove = async (refId: string) => {
    try {
      await api.deleteExternalRef(ontologyId, refId)
      reload()
    } catch (e: any) {
      setErr(e.message ?? String(e))
    }
  }

  return (
    <div className="onto-sec" data-testid="external-refs-pane">
      <span className="onto-sec-title">关联本体（跨本体引用登记，REQ-262）</span>
      {err && <Alert type="error" showIcon message={err} style={{ marginBottom: 8 }} />}
      {dangling.length > 0 && (
        <Alert
          type="warning" showIcon style={{ marginBottom: 8 }}
          message={`存在 ${dangling.length} 条悬空引用（目标本体删除或概念不存在；只提醒不阻断）`}
          description={<ul style={{ margin: 0, paddingLeft: 18 }}>{dangling.map((d, i) => <li key={i}>{d}</li>)}</ul>}
        />
      )}
      <Space.Compact style={{ width: '100%', marginBottom: 8, flexWrap: 'wrap', gap: 4 }} data-testid="extref-form">
        <Select
          style={{ width: 180 }} placeholder="源概念（本体内）" showSearch optionFilterProp="label"
          value={form.from_concept} onChange={(v) => setForm((f) => ({ ...f, from_concept: v }))}
          options={concepts.map((c) => ({ value: c, label: c }))}
        />
        <Select
          style={{ width: 150 }} placeholder="引用类型" value={form.ref_kind}
          onChange={(v) => setForm((f) => ({ ...f, ref_kind: v, predicate: v === 'alignment' ? f.predicate : undefined }))}
          options={REF_KINDS}
        />
        {form.ref_kind === 'alignment' && (
          <Select
            style={{ width: 130 }} placeholder="对齐谓词" allowClear value={form.predicate}
            onChange={(v) => setForm((f) => ({ ...f, predicate: v }))} options={PREDICATES}
          />
        )}
        <Select
          style={{ width: 190 }} placeholder="目标本体（外部词表留空）" allowClear showSearch optionFilterProp="label"
          value={form.to_onto_id || undefined} onChange={(v) => setForm((f) => ({ ...f, to_onto_id: v }))}
          options={ontos.filter((o) => o.id !== ontologyId).map((o) => ({ value: o.id, label: o.name }))}
        />
        <Tooltip title="目标概念名；目标本体留空时填外部词表概念或 IRI">
          <Input style={{ width: 200 }} placeholder="目标概念 / 外部 IRI" value={form.to_concept} onChange={(e) => setForm((f) => ({ ...f, to_concept: e.target.value }))} />
        </Tooltip>
        <Button type="primary" onClick={add} disabled={!form.from_concept || !form.to_concept || !form.ref_kind}>登记</Button>
      </Space.Compact>
      <Table
        size="small" rowKey="ref_id" loading={loading}
        dataSource={refs}
        pagination={refs.length > 8 ? { pageSize: 8 } : false}
        columns={[
          { title: '源概念', dataIndex: 'from_concept', width: 140 },
          {
            title: '引用', dataIndex: 'ref_kind', width: 130,
            render: (k: string, r: ExternalRef) => (
              <Space size={4}>
                <Tag color={k === 'import' ? 'blue' : k === 'alignment' ? 'purple' : 'green'}>{k}</Tag>
                {r.predicate && <Tag>{r.predicate}</Tag>}
              </Space>
            ),
          },
          {
            title: '目标', key: 'target', ellipsis: true,
            render: (_: any, r: ExternalRef) =>
              r.to_onto_id ? <span>{(ontos.find((o) => o.id === r.to_onto_id)?.name ?? r.to_onto_id)}:{r.to_concept}</span> : <span title="外部词表/未导入资产（IRI 登记态）">{r.to_concept} <Tag style={{ marginLeft: 4 }}>外部</Tag></span>,
          },
          {
            title: '来源', dataIndex: 'justification', width: 130,
            render: (j: string) => <Tag>{j === 'import_statement' ? 'owl:imports 自动登记' : j}</Tag>,
          },
          { title: '备注', dataIndex: 'note', ellipsis: true },
          {
            title: '', key: 'op', width: 60,
            render: (_: any, r: ExternalRef) => (
              <Popconfirm title="摘除该引用登记？" onConfirm={() => remove(r.ref_id)}>
                <Button size="small" type="text" danger>删除</Button>
              </Popconfirm>
            ),
          },
        ]}
      />
    </div>
  )
}
