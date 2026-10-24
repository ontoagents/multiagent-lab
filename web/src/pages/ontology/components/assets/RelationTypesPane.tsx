import { useMemo, useState } from 'react'
import { Button, Empty, Form, Input, Modal, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { Spec } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-240④/M66：关系类型管理页签（57 号 V2）——spec.relations 定义集中管理：
//   关系名/标签/定义域→值域/实例引用计数（InstanceRel.rel 反查）。
// REQ-420/M216（关系声明编辑补全）：label/定义/域值域经元素级端点编辑（服务端 Validate 单源）；
//   编辑锚=三元组名+from+to（组合键——改 from/to 需删除重建，诚实提示）。
// ---------------------------------------------------------------------------

interface RelRow {
  key: string
  name: string
  label: string
  from: string
  to: string
  refs: string[] // 引用该类型的实例关系（实例名 × rel）
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
    { title: '定义域（from）', dataIndex: 'from', width: 180, render: (v: string) => <Tag style={{ margin: 0 }}>{v}</Tag> },
    { title: '值域（to）', dataIndex: 'to', width: 180, render: (v: string) => <Tag style={{ margin: 0 }}>{v}</Tag> },
    {
      title: '操作',
      key: 'ops',
      width: 90,
      render: (_: unknown, r) => (
        <Button
          size="small"
          type="link"
          style={{ padding: 0 }}
          onClick={() => {
            setEditing({ key: r.key, name: r.name, from: r.from, to: r.to, label: r.label, definition: (spec?.relations ?? []).find((x) => x.name === r.name)?.definition ?? '' })
            form.setFieldsValue({ label: r.label, definition: (spec?.relations ?? []).find((x) => x.name === r.name)?.definition ?? '' })
          }}
        >
          编辑声明
        </Button>
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
            const target = (spec?.relations ?? []).find((x) => x.name === editing.name && x.from === editing.from && x.to === editing.to)
            await api.elementUpsert(ontologyId, 'relation', {
              name: editing.name, from: editing.from, to: editing.to,
              label: v.label ?? '', definition: v.definition ?? '',
              ...(target?.evidence ? { evidence: target.evidence } : {}),
            })
            message.success(`关系「${editing.name}」声明已更新`)
          } else {
            await api.elementUpsert(ontologyId, 'relation', {
              name: v.name.trim(), from: v.from.trim(), to: v.to.trim(),
              label: v.label ?? '', definition: v.definition ?? '',
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
              disabled={!!editing?.key}
              options={(spec?.concepts ?? []).map((c) => ({ value: c.name, label: c.label || c.name }))}
              placeholder="from"
            />
          </Form.Item>
          <Form.Item name="to" label="值域（to 概念）" style={{ width: '50%' }} rules={[{ required: true, message: '必填' }]}>
            <Select
              allowClear
              showSearch
              disabled={!!editing?.key}
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
        {editing?.key && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            编辑锚=三元组（名+域值域）：改端点需删除后重建；label/定义直接覆盖（presence 语义整体覆盖）。
          </Typography.Text>
        )}
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
