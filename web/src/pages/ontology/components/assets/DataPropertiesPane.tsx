import { useMemo, useState } from 'react'
import { Alert, Button, Empty, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import { RenameButton } from './RenameButton'
import type { Spec, SpecDataProperty } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-268/M77（D-O22 实用化升级批次二）：数据属性管理页签——spec.data_properties 声明集中管理：
//   名称/标签/定义/定义域/值域类型/实例引用计数（instance attributes 键反查）。
// REQ-420/M216（v1.95 范围扩容）：v1 只读 → CRUD——新增/编辑/删除走构建面元素级端点
// （/elements/attribute，Validate+版本门禁服务端单源沿 REQ-436），消除「手动建模最大单点堵点」。
// ---------------------------------------------------------------------------

interface DpRow {
  key: string
  dp: SpecDataProperty
  refs: string[] // 引用该属性的实例名（instance attributes 键命中）
}

export default function DataPropertiesPane({ spec, ontologyId, onSaved }: { spec: Spec | null; ontologyId: string; onSaved: () => void }) {
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState(false)
  // REQ-420：编辑表单态（null=关闭；name=''=新增）
  const [editing, setEditing] = useState<{ name: string; label: string; definition: string; domain: string; range: string } | null>(null)
  const [form] = Form.useForm()
  const rows = useMemo<DpRow[]>(() => {
    if (!spec) return []
    const usage = new Map<string, string[]>()
    for (const inst of spec.instances ?? []) {
      for (const k of Object.keys(inst.attributes ?? {})) {
        if (!usage.has(k)) usage.set(k, [])
        usage.get(k)!.push(inst.name)
      }
    }
    return (spec.data_properties ?? []).map((dp) => ({ key: dp.name, dp, refs: usage.get(dp.name) ?? [] }))
  }, [spec])

  const undeclared = useMemo<string[]>(() => {
    if (!spec || !(spec.data_properties ?? []).length) return []
    const decl = new Set((spec.data_properties ?? []).map((d) => d.name))
    const out = new Set<string>()
    for (const inst of spec.instances ?? []) {
      for (const k of Object.keys(inst.attributes ?? {})) {
        if (!decl.has(k)) out.add(k)
      }
    }
    return [...out]
  }, [spec])

  const ql = q.trim().toLowerCase()
  const filtered = rows.filter(({ dp }) => !ql || [dp.name, dp.label, dp.definition, dp.domain, dp.range].some((s) => (s ?? '').toLowerCase().includes(ql)))

  const columns: ColumnsType<DpRow> = [
    { title: '属性名', dataIndex: ['dp', 'name'], width: 200, render: (_: string, r) => (
      <span>
        <Typography.Text strong style={{ fontSize: 12.5 }}>{r.dp.name}</Typography.Text>
        {r.dp.label && <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{r.dp.label}</Typography.Text>}
      </span>
    ) },
    { title: '定义', dataIndex: ['dp', 'definition'], ellipsis: true, render: (v: string) => v || <Typography.Text type="secondary">—</Typography.Text> },
    { title: '定义域', dataIndex: ['dp', 'domain'], width: 150, render: (v: string) => (v ? <Tag style={{ margin: 0 }}>{v}</Tag> : <Typography.Text type="secondary">不限</Typography.Text>) },
    {
      title: '值域类型',
      dataIndex: ['dp', 'range'],
      width: 110,
      render: (v: string) => {
        const r = v || 'string'
        // 自定义 IRI 显示 local 名（长 IRI 溢出挤压相邻列），Tooltip 展示全文
        const short = /^https?:\/\//.test(r) ? (r.split('#').pop() || r.split('/').pop() || r) : r
        return (
          <Tooltip title={r}>
            <Tag color="geekblue" style={{ margin: 0, maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis' }}>{short}</Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '操作',
      key: 'ops',
      width: 110,
      render: (_: unknown, r) => (
        <Space size={4}>
          <Button
            size="small"
            type="link"
            style={{ padding: 0 }}
            onClick={() => {
              setEditing({ name: r.dp.name, label: r.dp.label ?? '', definition: r.dp.definition ?? '', domain: r.dp.domain ?? '', range: r.dp.range || 'string' })
              form.setFieldsValue({ name: r.dp.name, label: r.dp.label ?? '', definition: r.dp.definition ?? '', domain: r.dp.domain ?? '', range: r.dp.range || 'string' })
            }}
          >
            编辑
          </Button>
          <RenameButton ontologyId={ontologyId} kind="attribute" name={r.dp.name} label="数据属性" onSaved={onSaved} />
          <Popconfirm
            title={`删除属性声明「${r.dp.name}」？实例侧同名属性值不动（将成为未声明键）。`}
            onConfirm={async () => {
              setBusy(true)
              try {
                await api.elementDelete(ontologyId, 'attribute', r.dp.name)
                message.success(`属性「${r.dp.name}」已删除`)
                onSaved()
              } catch (e: any) {
                message.error(e?.message ?? '删除失败')
              } finally {
                setBusy(false)
              }
            }}
          >
            <Button size="small" type="link" danger style={{ padding: 0 }} disabled={busy}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
    {
      title: '实例引用',
      dataIndex: 'refs',
      width: 110,
      render: (refs: string[]) =>
        refs.length ? (
          <Tooltip title={refs.slice(0, 20).join('、') + (refs.length > 20 ? ` 等 ${refs.length} 个` : '')}>
            <Tag color="green" style={{ margin: 0 }}>{refs.length} 实例</Tag>
          </Tooltip>
        ) : (
          <Tag style={{ margin: 0 }}>未使用</Tag>
        ),
    },
  ]


  // REQ-420：属性声明表单（服务端 Validate——400 校验明细如实回喂；空态/非空态共用）
  const dpModal = (
    <Modal
        open={!!editing}
        title={editing?.name ? `编辑属性「${editing.name}」` : '新增属性声明'}
        okText={editing?.name ? '保存' : '新增'}
        okButtonProps={{ loading: busy }}
        onCancel={() => setEditing(null)}
        cancelText="取消"
        onOk={async () => {
          try {
            const v = await form.validateFields()
            setBusy(true)
            const body: Record<string, unknown> = { name: v.name.trim(), label: v.label ?? '', definition: v.definition ?? '', domain: v.domain ?? '', range: v.range || 'string' }
            if (editing?.name) body.name = editing.name // 编辑时名不可改（按名定位锚）
            await api.elementUpsert(ontologyId, 'attribute', body)
            message.success(editing?.name ? `属性「${editing.name}」已更新` : `属性「${v.name.trim()}」已新增`)
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
          <Form.Item name="name" label="属性名（唯一标识）" rules={[{ required: true, message: '请输入属性名' }]}>
            <Input placeholder="如 重量" disabled={!!editing?.name} />
          </Form.Item>
          <Form.Item name="label" label="显示名（可选）">
            <Input />
          </Form.Item>
          <Form.Item name="definition" label="定义（可选）">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Form.Item name="domain" label="定义域（可选=不限）">
            <Select
              allowClear
              options={(spec?.concepts ?? []).map((c) => ({ value: c.name, label: c.label || c.name }))}
              placeholder="不限定义域"
            />
          </Form.Item>
          <Form.Item name="range" label="值域类型" initialValue="string">
            <Select options={['string', 'number', 'integer', 'boolean', 'date'].map((v) => ({ value: v, label: v }))} />
          </Form.Item>
        </Form>
      </Modal>
  )

  // REQ-420：新增表单态提炼（空态与非空态共用同一入口/Modal）
  const addBtn = (
    <Button
      size="small"
      icon={<PlusOutlined />}
      onClick={() => {
        setEditing({ name: '', label: '', definition: '', domain: '', range: 'string' })
        form.setFieldsValue({ name: '', label: '', definition: '', domain: '', range: 'string' })
      }}
    >
      新增属性声明
    </Button>
  )

  if (!rows.length) {
    return (
      <div>
        <Space style={{ marginBottom: 8, width: '100%', justifyContent: 'flex-end' }}>{addBtn}</Space>
        <Empty
          description={
            <span style={{ fontSize: 12, color: 'var(--c-ink-3)' }}>
              本体尚无数据属性声明——点上方「新增属性声明」直接建（REQ-420 声明面 CRUD）；或经导入（owl:DatatypeProperty 自动捕获）、Spec JSON、LLM/结构化生成路径产生
            </span>
          }
          style={{ padding: '32px 0' }}
        />
        {dpModal}
      </div>
    )
  }

  return (
    <div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 8 }}
        message="数据属性（字面量属性）的声明层管理；实例 attributes 键与声明同名关联，TTL 导出以 owl:DatatypeProperty 回写（attr: 命名空间）。v1 只读，编辑经 Spec JSON 或导入管线。"
      />
      {undeclared.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 8 }}
          message={`未声明的实例属性键：${undeclared.slice(0, 8).join('、')}${undeclared.length > 8 ? ` 等 ${undeclared.length} 个（qualitygate undeclared_attribute_key info 级）` : ''}`}
        />
      )}
      <Space style={{ marginBottom: 8, width: '100%', justifyContent: 'space-between' }}>
        <Input.Search
          allowClear
          size="small"
          placeholder="搜索属性名/标签/定义/定义域"
          style={{ width: 260 }}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        {/* REQ-420：声明面 CRUD 入口 */}
        <Button
          size="small"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditing({ name: '', label: '', definition: '', domain: '', range: 'string' })
            form.setFieldsValue({ name: '', label: '', definition: '', domain: '', range: 'string' })
          }}
        >
          新增属性声明
        </Button>
      </Space>
      <Table<DpRow>
        size="small"
        rowKey="key"
        columns={columns}
        dataSource={filtered}
        pagination={filtered.length > 20 ? { pageSize: 20 } : false}
      />
      {dpModal}
    </div>
  )
}
