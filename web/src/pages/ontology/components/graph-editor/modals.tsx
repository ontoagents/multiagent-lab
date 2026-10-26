import { Form, Input, InputNumber, Modal, Radio, Select, Switch, DatePicker, Tag, Typography } from 'antd'
import { useMemo } from 'react'
import type { FormInstance } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import type { Spec, SpecConcept, SpecDataProperty } from '../../../../api/types'
import { instNameOf } from './model'
import type { ConnDraft } from './types'
import JsonEditor from '../JsonEditor'

// ---------------------------------------------------------------------------
// GraphEditor 三个弹窗（B1 拆分，REQ-145）：添加概念 / 连线 / 添加实例。
// 表单实例由主组件持有（回调逻辑留主组件），此处仅受控呈现。
// A2：添加实例的属性输入用 CodeMirror JSON（语法高亮 + 行内 lint），校验规则不变。
// REQ-449/M245（Protégé 手动建模兑现批②）：添加实例表单化——选中概念后按数据属性声明
// （直接 Domain + 沿 Parents 链继承，环防护）生成类型化表单字段（range 五型映射控件），
// 提交时合并进 attributes JSON（表单为声明属性正源，同名键以表单为准）；JSON 编辑器降级
// 「高级」形态（未声明键/高级用法）。Protégé 式实例表单对应物；编辑路径沿 InstancePanel 既有形态。
// ---------------------------------------------------------------------------

export function AddConceptModal({
  open,
  form,
  draft,
  onOk,
  onCancel,
}: {
  open: boolean
  form: FormInstance<{ name: string; label?: string; definition?: string; parents?: string[]; synonyms?: string }>
  draft: Spec
  onOk: () => void
  onCancel: () => void
}) {
  return (
    <Modal title="添加概念" open={open} centered okText="添加" cancelText="取消" onOk={onOk} onCancel={onCancel} destroyOnHidden>
      <Form form={form} layout="vertical" initialValues={{ parents: [] }}>
        <Form.Item
          name="name"
          label="名称（唯一标识）"
          rules={[{ required: true, message: '请输入概念名' }]}
          extra={<NamingHint draft={draft} form={form} />}
        >
          <Input placeholder="如 Paper" />
        </Form.Item>
        <Form.Item name="label" label="显示名（可选）">
          <Input placeholder="如 论文" />
        </Form.Item>
        <Form.Item name="definition" label="定义（可选）">
          <Input.TextArea rows={2} placeholder="一句话说明该概念是什么（缺失定义会在质量卡告警）" />
        </Form.Item>
        <Form.Item name="parents" label="父概念（可选，多选；相关候选排前）">
          <ParentSelect draft={draft} form={form} />
        </Form.Item>
        <Form.Item name="synonyms" label="同义词（可选）" extra="顿号/逗号分隔，如 客户、甲方——业务口语命中面（REQ-419，check_answerable 预检消费）">
          <Input placeholder="客户、甲方" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

export function ConnectModal({
  connDraft,
  form,
  onOk,
  onCancel,
  onKindChange,
}: {
  connDraft: ConnDraft | null
  form: FormInstance<{ relName: string; relLabel?: string }>
  onOk: () => void
  onCancel: () => void
  onKindChange: (kind: 'relation' | 'parent') => void
}) {
  return (
    <Modal
      title={
        connDraft ? (
          <span>
            连线 <Tag style={{ margin: 0 }}>{connDraft.source.startsWith('inst:') ? instNameOf(connDraft.source) : connDraft.source}</Tag> →{' '}
            <Tag style={{ margin: 0 }}>{connDraft.target.startsWith('inst:') ? instNameOf(connDraft.target) : connDraft.target}</Tag>
          </span>
        ) : (
          '连线'
        )
      }
      open={!!connDraft}
      centered
      okText="创建"
      cancelText="取消"
      onOk={onOk}
      onCancel={onCancel}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        {connDraft?.kind !== 'instance-relation' && (
          <Form.Item name="kind" label="连线类型" initialValue="relation">
            <Radio.Group
              onChange={(e) => onKindChange(e.target.value)}
              options={[
                { value: 'relation', label: '关系（实线，from → to）' },
                { value: 'parent', label: `继承（虚线，${connDraft?.source} 为父）` },
              ]}
            />
          </Form.Item>
        )}
        {connDraft?.kind === 'relation' && (
          <>
            <Form.Item name="relName" label="关系名（唯一标识）" rules={[{ required: true, message: '请输入关系名' }]}>
              <Input placeholder="如 cites" />
            </Form.Item>
            <Form.Item name="relLabel" label="显示名（可选）">
              <Input placeholder="如 引用" />
            </Form.Item>
          </>
        )}
        {connDraft?.kind === 'parent' && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            将为 {connDraft.target} 增加父概念 {connDraft.source}（若已存在则忽略）。
          </Typography.Text>
        )}
        {connDraft?.kind === 'instance-relation' && (
          <>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
              实例间关系（记录在 {instNameOf(connDraft.source)} 的 relations 上，指向 {instNameOf(connDraft.target)}）。
            </Typography.Text>
            <Form.Item name="relName" label="关系名（对应概念层关系）" rules={[{ required: true, message: '请输入关系名' }]}>
              <Input placeholder="如 cites（建议与概念层关系同名）" />
            </Form.Item>
          </>
        )}
      </Form>
    </Modal>
  )
}

/** REQ-449：收集概念可见的数据属性声明——直接 Domain 命中 + 沿 Parents 链向上继承（环防护） */
function declaredPropsFor(draft: Spec, concept: string | undefined): SpecDataProperty[] {
  if (!concept) return []
  const byName = new Map((draft.concepts ?? []).map((c) => [c.name, c] as const))
  const visited = new Set<string>([concept])
  const scope: string[] = [concept]
  let i = 0
  while (i < scope.length) {
    const cur = scope[i++]
    for (const p of byName.get(cur)?.parents ?? []) {
      if (!visited.has(p) && byName.has(p)) {
        visited.add(p)
        scope.push(p)
      }
    }
  }
  const inScope = new Set(scope)
  return (draft.data_properties ?? []).filter((d) => d.domain && inScope.has(d.domain))
}

/** REQ-449：range → 控件映射（string/未知 IRI→Input；integer/number→InputNumber；boolean→Switch；date→DatePicker） */
function DeclaredPropItem({ dp }: { dp: SpecDataProperty }) {
  const range = dp.range || 'string'
  const label = (
    <span>
      {dp.name}
      {dp.label && dp.label !== dp.name && <Typography.Text type="secondary" style={{ fontWeight: 400 }}>（{dp.label}）</Typography.Text>}
      <Tag style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>{range}</Tag>
    </span>
  )
  return (
    <Form.Item name={['dpVals', dp.name]} label={label}>
      {range === 'boolean' ? (
        <Switch checkedChildren="真" unCheckedChildren="假" />
      ) : range === 'integer' ? (
        <InputNumber precision={0} style={{ width: '100%' }} placeholder={`整数${dp.definition ? `——${dp.definition}` : ''}`} />
      ) : range === 'number' ? (
        <InputNumber style={{ width: '100%' }} placeholder={`数值${dp.definition ? `——${dp.definition}` : ''}`} />
      ) : range === 'date' ? (
        <DatePicker style={{ width: '100%' }} />
      ) : (
        <Input placeholder={dp.definition || '文本'} />
      )}
    </Form.Item>
  )
}

export function AddInstanceModal({
  open,
  form,
  draft,
  onOk,
  onCancel,
}: {
  open: boolean
  form: FormInstance<{ name: string; concept: string; attributes?: string }>
  draft: Spec
  onOk: () => void
  onCancel: () => void
}) {
  // REQ-449：选中概念 → 声明属性表单（直接+继承）；提交时合并进 attributes（表单同名键为准）
  const concept = Form.useWatch('concept', form) as string | undefined
  const declared = useMemo(() => declaredPropsFor(draft, concept), [draft, concept])
  const inherited = declared.filter((d) => d.domain !== concept).length

  const handleOk = async () => {
    try {
      const v = await form.validateFields()
      let base: Record<string, unknown> = {}
      if (v.attributes?.trim()) {
        try {
          base = JSON.parse(v.attributes)
          if (typeof base !== 'object' || base === null || Array.isArray(base)) base = {}
        } catch {
          base = {}
        }
      }
      const vals = ((v as any).dpVals ?? {}) as Record<string, unknown>
      for (const [k, val] of Object.entries(vals)) {
        if (val === undefined || val === null || val === '') continue
        base[k] = val instanceof dayjs || (val as Dayjs).format ? (val as Dayjs).format('YYYY-MM-DD') : val
      }
      form.setFieldsValue({ attributes: JSON.stringify(base) })
    } catch (e: any) {
      if (e?.errorFields) return
    }
    onOk()
  }

  return (
    <Modal title="添加实例" open={open} centered okText="添加" cancelText="取消" onOk={handleOk} onCancel={onCancel} destroyOnHidden>
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="实例名（唯一标识）" rules={[{ required: true, message: '请输入实例名' }]}>
          <Input placeholder="如 《知识图谱》" />
        </Form.Item>
        <Form.Item name="concept" label="所属概念" rules={[{ required: true, message: '请选择所属概念' }]}>
          <Select showSearch optionFilterProp="label" placeholder="选择概念" options={draft.concepts.map((c) => ({ value: c.name, label: c.label || c.name }))} />
        </Form.Item>
        {declared.length > 0 && (
          <>
            <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
              按概念声明生成的属性表单（{declared.length} 项{inherited > 0 ? `，其中 ${inherited} 项继承自父概念` : ''}）——留空跳过
            </Typography.Text>
            {declared.map((dp) => (
              <DeclaredPropItem key={dp.name} dp={dp} />
            ))}
          </>
        )}
        <Form.Item
          name="attributes"
          label={declared.length > 0 ? '高级：属性 JSON（未声明键/覆盖声明值）' : '属性（可选，JSON 对象——该概念暂无声明属性，可先在「数据属性」页签声明后获得表单）'}
          rules={[
            {
              validator: (_: unknown, v: string) => {
                if (!v?.trim()) return Promise.resolve()
                try {
                  const p = JSON.parse(v)
                  if (typeof p !== 'object' || p === null || Array.isArray(p)) return Promise.reject('需为 JSON 对象，如 {"year": 2024}')
                } catch {
                  return Promise.reject('JSON 语法不合法')
                }
                return Promise.resolve()
              },
            },
          ]}
        >
          <JsonEditor height={declared.length > 0 ? '70px' : '110px'} placeholder='如 {"year": 2024}' />
        </Form.Item>
      </Form>
    </Modal>
  )
}

/** REQ-249/G3：命名风格即时提示（qualitygate naming_style 规则前移——CJK 不参与判定，与门禁同口径）。 */
function NamingHint({ draft, form }: { draft: Spec; form: FormInstance }) {
  const name = Form.useWatch('name', form) as string | undefined
  if (!name || !/[A-Za-z]/.test(name)) return null // CJK 名不参与（与 qualitygate 一致）
  const latin = draft.concepts.filter((c) => /[A-Za-z]/.test(c.name))
  if (latin.length < 2) return null
  const camel = latin.filter((c) => /[a-z][A-Z]/.test(c.name)).length
  const snake = latin.filter((c) => c.name.includes('_')).length
  const style = camel >= snake ? 'camelCase（如 PaperVersion）' : 'snake_case（如 paper_version）'
  const deviates = camel >= snake ? !/[a-z][A-Z]/.test(name) && !/^[a-z0-9]+$/.test(name) : !name.includes('_') && !/^[A-Z][a-z0-9]+$/.test(name)
  if (!deviates) return null
  return (
    <Typography.Text type="warning" style={{ fontSize: 11 }}>
      现有概念以{style}为主，当前命名可能风格不一致（质量卡将告警）
    </Typography.Text>
  )
}

/** REQ-249/G3：父概念候选——与当前输入名（label/名称）相似度优先排序。 */
function ParentSelect({ draft, form }: { draft: Spec; form: FormInstance }) {
  const name = (Form.useWatch('name', form) as string | undefined) ?? ''
  const options = useMemo(() => {
    const q = name.trim().toLowerCase()
    const score = (c: SpecConcept): number => {
      let s = 0
      const hay = (c.label || c.name).toLowerCase()
      if (q && (hay.includes(q) || q.includes(c.name.toLowerCase()))) s += 10
      if (q && c.name.toLowerCase().startsWith(q.slice(0, 3))) s += 5
      return s
    }
    return [...draft.concepts].sort((a, b) => score(b) - score(a)).map((c) => ({ value: c.name, label: c.label || c.name }))
  }, [draft.concepts, name])
  return <Select mode="multiple" allowClear showSearch optionFilterProp="label" placeholder="选择已有概念" options={options} />
}
