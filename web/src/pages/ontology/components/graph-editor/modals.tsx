import { Form, Input, Modal, Radio, Select, Tag, Typography } from 'antd'
import { useMemo } from 'react'
import type { FormInstance } from 'antd'
import type { Spec, SpecConcept } from '../../../../api/types'
import { instNameOf } from './model'
import type { ConnDraft } from './types'
import JsonEditor from '../JsonEditor'

// ---------------------------------------------------------------------------
// GraphEditor 三个弹窗（B1 拆分，REQ-145）：添加概念 / 连线 / 添加实例。
// 表单实例由主组件持有（回调逻辑留主组件），此处仅受控呈现。
// A2：添加实例的属性输入用 CodeMirror JSON（语法高亮 + 行内 lint），校验规则不变。
// ---------------------------------------------------------------------------

export function AddConceptModal({
  open,
  form,
  draft,
  onOk,
  onCancel,
}: {
  open: boolean
  form: FormInstance<{ name: string; label?: string; definition?: string; parents?: string[] }>
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
  return (
    <Modal title="添加实例" open={open} centered okText="添加" cancelText="取消" onOk={onOk} onCancel={onCancel} destroyOnHidden>
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="实例名（唯一标识）" rules={[{ required: true, message: '请输入实例名' }]}>
          <Input placeholder="如 《知识图谱》" />
        </Form.Item>
        <Form.Item name="concept" label="所属概念" rules={[{ required: true, message: '请选择所属概念' }]}>
          <Select showSearch optionFilterProp="label" placeholder="选择概念" options={draft.concepts.map((c) => ({ value: c.name, label: c.label || c.name }))} />
        </Form.Item>
        <Form.Item
          name="attributes"
          label="属性（可选，JSON 对象）"
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
          <JsonEditor height="110px" placeholder='如 {"year": 2024}' />
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
