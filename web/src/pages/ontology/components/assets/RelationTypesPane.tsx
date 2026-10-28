import { useMemo, useState } from 'react'
import { Button, Checkbox, Empty, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import { RenameButton } from './RenameButton'
import type { Spec } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-240④/M66：关系类型管理页签（57 号 V2）——spec.relations 定义集中管理：
//   关系名/标签/定义域→值域/实例引用计数（InstanceRel.rel 反查）。
// REQ-420/M216（关系声明编辑补全）：label/定义/域值域经元素级端点编辑（服务端 Validate 单源）。
// REQ-473/M273（Protégé 编辑能力对齐批②）：①from/to 解锁编辑（后端元素端点本就支持——原前端锁死）；
//   ②特性声明（transitive/symmetric/functional/inverse_functional Checkbox）+逆属性 Select；
//   ③删除声明按钮（被实例引用时 Validate 拒——悬空定位）；④特性列呈现。
// ---------------------------------------------------------------------------

interface RelRow {
  key: string
  name: string
  label: string
  from: string
  to: string
  characteristics: string[]
  inverseOf: string
  refs: string[] // 引用该类型的实例关系（实例名 × rel）
}

const CH_META: Record<string, { color: string; text: string }> = {
  transitive: { color: 'geekblue', text: '传递' },
  symmetric: { color: 'cyan', text: '对称' },
  functional: { color: 'orange', text: '函数' },
  inverse_functional: { color: 'purple', text: '逆函数' },
}

export default function RelationTypesPane({ spec, ontologyId, onSaved }: { spec: Spec | null; ontologyId: string; onSaved: () => void }) {
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState(false)
  // REQ-420：编辑表单态（key=name:from:to 三元组锚）
  const [editing, setEditing] = useState<{ key: string; name: string; from: string; to: string; label: string; definition: string } | null>(null)
  const [form] = Form.useForm()
  const rows = useMemo<RelRow[]>(() => {
    if (!spec) return []
    const usage = new Map<string, string[]>()
    for (const inst of spec.instances ?? []) {
      for (const rel of inst.relations ?? []) {
        if (!usage.has(rel.rel)) usage.set(rel.rel, [])
        usage.get(rel.rel)!.push(inst.name)
      }
    }
    return (spec.relations ?? []).map((r) => ({
      key: `${r.name}:${r.from}:${r.to}`,
      name: r.name,
      label: r.label || '',
      from: r.from,
      to: r.to,
      characteristics: r.characteristics ?? [],
      inverseOf: r.inverse_of ?? '',
      refs: usage.get(r.name) ?? [],
    }))
  }, [spec])

  const ql = q.trim().toLowerCase()
  const hit = (r: RelRow) =>
    !ql || [r.name, r.label, r.from, r.to].some((s) => s.toLowerCase().includes(ql))
  const filtered = rows.filter(hit)

  const columns: ColumnsType<RelRow> = [
    { title: '关系名', dataIndex: 'name', width: 200, render: (v: string, r) => (
      <span>
        <Typography.Text strong style={{ fontSize: 12.5 }}>{v}</Typography.Text>
        {r.label && <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{r.label}</Typography.Text>}
      </span>
    ) },
    { title: '定义域（from）', dataIndex: 'from', width: 150, render: (v: string) => <Tag style={{ margin: 0 }}>{v}</Tag> },
    { title: '值域（to）', dataIndex: 'to', width: 150, render: (v: string) => <Tag style={{ margin: 0 }}>{v}</Tag> },
    {
      title: '特性 / 逆属性',
      key: 'ch',
      width: 190,
      render: (_: unknown, r) => (
        <Space size={4} wrap>
          {r.characteristics.map((c) => (
            <Tag key={c} color={CH_META[c]?.color ?? 'default'} style={{ margin: 0 }}>{CH_META[c]?.text ?? c}</Tag>
          ))}
          {r.inverseOf && <Tooltip title={`owl:inverseOf → ${r.inverseOf}`}><Tag color="blue" style={{ margin: 0 }}>逆:{r.inverseOf}</Tag></Tooltip>}
          {r.characteristics.length === 0 && !r.inverseOf && <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>}
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'ops',
      width: 120,
      render: (_: unknown, r) => (
        <Space size={4}>
          <Button
            size="small"
            type="link"
            style={{ padding: 0 }}
            onClick={() => {
              const full = (spec?.relations ?? []).find((x) => x.name === r.name)
              setEditing({ key: r.key, name: r.name, from: r.from, to: r.to, label: r.label, definition: full?.definition ?? '' })
              form.setFieldsValue({
                label: r.label,
                definition: full?.definition ?? '',
                from: r.from,
                to: r.to,
                characteristics: r.characteristics,
                inverse_of: r.inverseOf || undefined,
              })
            }}
          >
            编辑
          </Button>
          <RenameButton ontologyId={ontologyId} kind="relation" name={r.name} label="关系" onSaved={onSaved} />
          <Popconfirm
            title={`删除关系声明「${r.name}」？`}
            description={r.refs.length > 0 ? `被 ${r.refs.length} 处实例断言引用——服务端校验将拒绝并给悬空定位` : '未被实例引用，可直接删除'}
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={async () => {
              try {
                await api.elementDelete(ontologyId, 'relation', r.name)
                message.success(`关系「${r.name}」已删除`)
                onSaved()
              } catch (e: any) {
                message.error(e?.message ?? '删除失败')
              }
            }}
          >
            <Button size="small" type="link" danger style={{ padding: 0 }}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
    {
      title: '实例引用',
      dataIndex: 'refs',
      width: 110,
      render: (refs: string[]) =>
        refs.length > 0 ? (
          <Tooltip title={refs.slice(0, 8).join('、') + (refs.length > 8 ? ` 等 ${refs.length} 处` : '')}>
            <Tag color="purple" style={{ margin: 0 }}>{refs.length} 处</Tag>
          </Tooltip>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>未使用</Typography.Text>
        ),
    },
  ]

  // REQ-420：空态也带声明入口（空态/非空态共用同一 Modal）
  // REQ-420：新增按钮与表单（空态/非空态共用）
  const addBtn = (
    <Button
      size="small"
      icon={<PlusOutlined />}
      onClick={() => {
        setEditing({ key: '', name: '', from: '', to: '', label: '', definition: '' })
        form.setFieldsValue({ name: '', from: '', to: '', label: '', definition: '' })
      }}
    >
      新增关系声明
    </Button>
  )
  const relModal = (
    <Modal
      open={!!editing}
      title={editing?.key ? `编辑关系「${editing.name}」` : '新增关系声明'}
      okText={editing?.key ? '保存' : '新增'}
      okButtonProps={{ loading: busy }}
      cancelText="取消"
      onCancel={() => setEditing(null)}
      onOk={async () => {
        try {
          const v = await form.validateFields()
          setBusy(true)
          if (editing?.key) {
            const target = (spec?.relations ?? []).find((x) => x.name === editing.name)
            // REQ-473：from/to 解锁（后端元素端点本就支持）+ 特性/逆属性整组提交
            await api.elementUpsert(ontologyId, 'relation', {
              name: editing.name, from: v.from, to: v.to,
              label: v.label ?? '', definition: v.definition ?? '',
              characteristics: v.characteristics ?? [],
              inverse_of: v.inverse_of ?? '',
              ...(target?.evidence ? { evidence: target.evidence } : {}),
            })
            message.success(`关系「${editing.name}」声明已更新`)
          } else {
            await api.elementUpsert(ontologyId, 'relation', {
              name: v.name.trim(), from: v.from, to: v.to,
              label: v.label ?? '', definition: v.definition ?? '',
              characteristics: v.characteristics ?? [],
              inverse_of: v.inverse_of ?? '',
            })
            message.success(`关系「${v.name.trim()}」已新增`)
          }
          setEditing(null)
          onSaved()
        } catch (e: any) {
          if (e?.errorFields) return
          message.error(e?.message ?? '保存失败（校验明细见响应）')
        } finally {
          setBusy(false)
        }
      }}
    >
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="关系名（唯一标识）" rules={[{ required: true, message: '请输入关系名' }]}>
          <Input placeholder="如 连接到" disabled={!!editing?.key} />
        </Form.Item>
        <Space.Compact block>
          <Form.Item name="from" label="定义域（from 概念）" style={{ width: '50%', paddingInlineEnd: 6 }} rules={[{ required: true, message: '必填' }]}>
            <Select
              allowClear
              showSearch
              options={(spec?.concepts ?? []).map((c) => ({ value: c.name, label: c.label || c.name }))}
              placeholder="from"
            />
          </Form.Item>
          <Form.Item name="to" label="值域（to 概念）" style={{ width: '50%' }} rules={[{ required: true, message: '必填' }]}>
            <Select
              allowClear
              showSearch
              options={(spec?.concepts ?? []).map((c) => ({ value: c.name, label: c.label || c.name }))}
              placeholder="to"
            />
          </Form.Item>
        </Space.Compact>
        <Form.Item name="label" label="显示名（可选）">
          <Input />
        </Form.Item>
        <Form.Item name="definition" label="定义（可选）">
          <Input.TextArea rows={2} />
        </Form.Item>
        {/* REQ-473/M273：特性声明（Protégé ObjectProperty characteristics 子集） */}
        <Form.Item name="characteristics" label="特性（TTL 导出为附加 rdf:type 行）">
          <Checkbox.Group
            options={Object.entries(CH_META).map(([v, m]) => ({ value: v, label: m.text }))}
            data-testid="rel-characteristics-group"
          />
        </Form.Item>
        <Form.Item name="inverse_of" label="逆属性（owl:inverseOf，可选——选择已声明关系）">
          <Select
            allowClear
            showSearch
            placeholder="无"
            options={(spec?.relations ?? []).filter((x) => x.name !== editing?.name).map((x) => ({ value: x.name, label: x.label || x.name }))}
          />
        </Form.Item>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>
          语义提示：函数=每主体至多一个值（门禁对实例多值断言 warning）；传递/对称影响推理语义；逆属性两关系互为反向。
        </Typography.Text>
      </Form>
    </Modal>
  )

  if (!spec || (spec.relations?.length ?? 0) === 0) {
    return (
      <div>
        <Space style={{ marginBottom: 8, width: '100%', justifyContent: 'flex-end' }}>{addBtn}</Space>
        <div className="work-empty" style={{ minHeight: 180 }}>
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无关系类型定义——点上方「新增关系声明」直接建（REQ-420 关系声明编辑补全）；或在 Spec 编辑/图形编辑中添加后此处集中呈现" />
        </div>
        {relModal}
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10, flexWrap: 'wrap' }}>
        <Typography.Text strong>关系类型（{rows.length}）</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          定义集中管理：定义域 → 值域与实例引用计数（57 号 V2；数据属性一等公民建模与 REQ-235 同轮定模型）
        </Typography.Text>
        <span style={{ flex: 1 }} />
        <Input
          allowClear
          size="small"
          placeholder="搜索名称 / 标签 / 端点"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{ width: 200 }}
          data-testid="reltype-search"
        />
        {addBtn}
      </div>
      <Table
        rowKey="key"
        columns={columns}
        dataSource={filtered}
        size="small"
        pagination={filtered.length > 20 ? { pageSize: 20, showSizeChanger: false } : false}
        locale={{ emptyText: `无匹配「${q.trim()}」的关系类型` }}
        data-testid="reltype-table"
      />
      {/* REQ-420：关系声明表单——新增=全字段；编辑=三元组锚（name:from:to 不可改，label/定义覆盖） */}
      {relModal}
    </div>
  )
}
