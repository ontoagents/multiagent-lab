import { useState } from 'react'
import { App, Button, Empty, Form, Input, Modal, Popconfirm, Segmented, Select, Space, Table, Tag, Tooltip, Typography } from 'antd'
import { MergeOutlined, PlusOutlined, RestOutlined, EditOutlined } from '@ant-design/icons'
import { companionApi } from '../../../../api/companion'
import type { CompanionGraph } from '../../../../api/companion'

// ---------------------------------------------------------------------------
// REQ-285③：伴生内容清单——图看结构（沉淀总览）、表看明细（本分区）。
// REQ-286 C1/B4：行级编辑——实体表（改名称〔迁移式重命名，旧名转别名〕/改定义/删除/
// 合并到已有实体）；关系表（删除）；补关系（选两端实体+关系名）。
// provenance 字段（置信/时点/入图时间）锁定不可编辑；编辑动作落 onto_decision 审计+快照同步。
// ---------------------------------------------------------------------------

const fmtTime = (s?: string) => (s ? s.slice(0, 19).replace('T', ' ') : '—')

export default function CompanionContentPane({ graph, ontologyId, onChanged }: { graph: CompanionGraph | null; ontologyId: string; onChanged?: () => void }) {
  const { message: toast } = App.useApp()
  const [editOpen, setEditOpen] = useState(false)
  const [editRow, setEditRow] = useState<{ label: string; definition?: string } | null>(null)
  const [editForm] = Form.useForm<{ label: string; definition?: string }>()
  const [addRelOpen, setAddRelOpen] = useState(false)
  const [addRelForm] = Form.useForm<{ source: string; rel_name: string; target: string; definition?: string }>()
  const [mergeFrom, setMergeFrom] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  // REQ-298：内容清单搜索（实体/关系前端过滤——治「无搜索功能」）
  const [query, setQuery] = useState('')
  const [table, setTable] = useState<'entity' | 'relation'>('entity')
  const run = async (fn: () => Promise<unknown>, okMsg: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(okMsg)
      onChanged?.()
    } catch (e: any) {
      toast.error(e?.message ?? '操作失败')
      throw e
    } finally {
      setBusy(false)
    }
  }
  if (!graph || graph.nodes.length === 0) {
    return <Empty description="伴生图暂无沉淀内容（绑定智能体对话收尾后自动生长）" style={{ padding: '40px 0' }} />
  }
  const concepts = graph.nodes.filter((n) => n.kind === 'Concept')
  const events = graph.nodes.filter((n) => n.kind === 'Event')
  const q = query.trim().toLowerCase()
  const hit = (...fields: (string | undefined)[]) => !q || fields.some((f) => (f ?? '').toLowerCase().includes(q))
  const labels = graph.nodes.map((n) => n.label) // 实体（概念/事件）名清单（补关系端点候选）
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      <Input
        allowClear
        placeholder="搜索实体名称 / 定义 / 关系（前端实时过滤）…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        style={{ maxWidth: 420 }}
      />
      <div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
          <Space size={8} align="center">
            <Typography.Title level={5} style={{ margin: 0 }}>
              实体（概念 {concepts.length} · 事件 {events.length}）
            </Typography.Title>
            <Segmented size="small" value={table} onChange={(v) => setTable(v as 'entity' | 'relation')} options={[{ value: 'entity', label: '实体' }, { value: 'relation', label: '关系' }]} />
          </Space>
          <Button
            size="small"
            icon={<PlusOutlined />}
            disabled={busy}
            onClick={() => {
              addRelForm.resetFields()
              setAddRelOpen(true)
            }}
          >
            补关系
          </Button>
        </div>
        <div style={{ display: table === 'entity' ? 'block' : 'none' }}>
        <Table
          size="small"
          dataSource={graph.nodes
            .filter((n) => hit(n.label, n.definition))
            .map((n, i) => ({ key: i, ...n }))}
          pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
          columns={[
            {
              title: '类型', width: 70,
              render: (_: unknown, r: any) => (
                <Tag color={r.kind === 'Event' ? 'purple' : 'cyan'} style={{ margin: 0 }}>{r.kind === 'Event' ? '事件' : '概念'}</Tag>
              ),
            },
            { title: '名称', dataIndex: 'label', width: 180, render: (v: string) => <Typography.Text strong style={{ fontSize: 12 }}>{v}</Typography.Text> },
            {
              title: '定义', dataIndex: 'definition', ellipsis: { showTitle: false },
              render: (v?: string) => (
                <Tooltip title={v || undefined}><Typography.Text type="secondary" style={{ fontSize: 12 }}>{v || '（未沉淀定义）'}</Typography.Text></Tooltip>
              ),
            },
            { title: '置信', dataIndex: 'confidence', width: 70, render: (v?: number) => (typeof v === 'number' ? v.toFixed(2) : '—') },
            { title: '时点', dataIndex: 'time_scope', width: 90, render: (v?: string) => v || '—' },
            { title: '入图时间', dataIndex: 'created_at', width: 150, render: (v?: string) => <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>{fmtTime(v)}</span> },
            {
              title: '操作', width: 200,
              render: (_: unknown, r: any) => (
                <Space size={2}>
                  <Button
                    size="small" type="text" icon={<EditOutlined />} aria-label={`编辑 ${r.label}`}
                    onClick={() => {
                      setEditRow({ label: r.label, definition: r.definition })
                      editForm.setFieldsValue({ label: r.label, definition: r.definition })
                      setEditOpen(true)
                    }}
                  />
                  {mergeFrom === null && (
                    <Tooltip title="合并到其他实体（本实体作为别名并入目标，关系边重定向）">
                      <Button
                        size="small" type="text" icon={<MergeOutlined />} aria-label={`合并 ${r.label}`}
                        disabled={busy}
                        onClick={() => setMergeFrom(r.label)}
                      />
                    </Tooltip>
                  )}
                  <Popconfirm
                    title={`删除实体「${r.label}」？`}
                    description="删除主体三元组及其全部关系边（不可恢复）。"
                    okText="删除" okButtonProps={{ danger: true }} cancelText="取消"
                    onConfirm={() => run(() => companionApi.deleteEntity(ontologyId, r.label), '实体已删除').catch(() => {})}
                  >
                    <Button size="small" type="text" danger icon={<RestOutlined />} aria-label={`删除 ${r.label}`} disabled={busy} />
                  </Popconfirm>
                  {mergeFrom !== null && mergeFrom !== r.label && (
                    <Button
                      size="small" type="link" style={{ padding: 0, fontSize: 12 }}
                      disabled={busy}
                      onClick={async () => {
                        try {
                          await run(() => companionApi.mergeEntity(ontologyId, mergeFrom, r.label), `已合并 ${mergeFrom} → ${r.label}`)
                        } finally {
                          setMergeFrom(null)
                        }
                      }}
                    >
                      合并「{mergeFrom}」到此
                    </Button>
                  )}
                </Space>
              ),
            },
          ]}
        />
        {mergeFrom !== null && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            正在合并「{mergeFrom}」——在目标实体行点「合并到此」；<Button size="small" type="link" style={{ padding: 0 }} onClick={() => setMergeFrom(null)}>取消</Button>
          </Typography.Text>
        )}
      </div>
      </div>
      <div style={{ display: table === 'relation' ? 'block' : 'none' }}>
        <Typography.Title level={5} style={{ marginTop: 0 }}>关系（{graph.edges.length}）</Typography.Title>
        {table === 'relation' && graph.edges.length === 0 ? (
          <Empty description="暂无关系边——可在「伴生候选」页挖掘关系" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : table === 'relation' && (
          <Table
            size="small"
            dataSource={graph.edges
              .filter((e) => hit(e.source, e.rel, e.target))
              .map((e, i) => ({ key: i, ...e }))}
            pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
            columns={[
              { title: '主体', dataIndex: 'source', width: 150, render: (v: string) => <Typography.Text style={{ fontSize: 12 }}>{v}</Typography.Text> },
              { title: '关系', dataIndex: 'rel', width: 110, render: (v: string) => <Tag color="geekblue" style={{ margin: 0 }}>{v}</Tag> },
              { title: '客体', dataIndex: 'target', width: 150, render: (v: string) => <Typography.Text style={{ fontSize: 12 }}>{v}</Typography.Text> },
              {
                title: '印证', dataIndex: 'confirm_count', width: 60,
                render: (v?: number) => (
                  <Tooltip title="同事实被确认次数（REQ-227① 印证聚合）">
                    <Tag color={(v ?? 1) > 1 ? 'orange' : 'default'} style={{ margin: 0 }}>{v ?? 1}×</Tag>
                  </Tooltip>
                ),
              },
              { title: '入图时间', dataIndex: 'created_at', width: 150, render: (v?: string) => <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>{fmtTime(v)}</span> },
              {
                title: '操作', width: 60,
                render: (_: unknown, r: any) =>
                  r.edge_uri ? (
                    <Popconfirm title="删除该关系边？" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => run(() => companionApi.deleteRelation(ontologyId, r.edge_uri), '关系已删除').catch(() => {})}>
                      <Button size="small" type="text" danger icon={<RestOutlined />} aria-label={`删除关系 ${r.source} ${r.rel} ${r.target}`} disabled={busy} />
                    </Popconfirm>
                  ) : null,
              },
            ]}
          />
        )}
      </div>
      <Modal
        title={`编辑实体「${editRow?.label ?? ''}」`}
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        confirmLoading={busy}
        onOk={async () => {
          const v = await editForm.validateFields()
          try {
            await run(() => companionApi.editEntity(ontologyId, { label: editRow!.label, new_label: v.label !== editRow!.label ? v.label : undefined, new_definition: v.definition ?? '' }), '实体已更新')
            setEditOpen(false)
          } catch { /* 保持弹窗 */ }
        }}
        okText="保存"
        cancelText="取消"
      >
        <Form form={editForm} layout="vertical" requiredMark={false}>
          <Form.Item name="label" label="名称" extra="改名=迁移式重命名（关系边随之重定向，旧名保留为别名）" rules={[{ required: true, message: '名称必填' }]}>
            <Input maxLength={120} />
          </Form.Item>
          <Form.Item name="definition" label="定义">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} maxLength={500} />
          </Form.Item>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>置信/来源/入图时间为对话沉淀事实，不可编辑。</Typography.Text>
        </Form>
      </Modal>
      <Modal
        title="补关系"
        open={addRelOpen}
        onCancel={() => setAddRelOpen(false)}
        confirmLoading={busy}
        onOk={async () => {
          const v = await addRelForm.validateFields()
          try {
            await run(() => companionApi.addRelation(ontologyId, { source: v.source, rel_name: v.rel_name, target: v.target, definition: v.definition }), '关系已入图')
            setAddRelOpen(false)
          } catch { /* 保持弹窗 */ }
        }}
        okText="入图"
        cancelText="取消"
      >
        <Form form={addRelForm} layout="vertical" requiredMark={false}>
          <Form.Item name="source" label="主体实体" rules={[{ required: true, message: '选择主体' }]}>
            <Select showSearch optionFilterProp="label" options={labels.map((l) => ({ value: l, label: l }))} placeholder="选择图内实体" />
          </Form.Item>
          <Form.Item name="rel_name" label="关系名（动名词）" rules={[{ required: true, message: '关系名必填' }]}>
            <Input maxLength={120} placeholder="如：属于 / 依赖 / 引发" />
          </Form.Item>
          <Form.Item name="target" label="客体实体" rules={[{ required: true, message: '选择客体' }]}>
            <Select showSearch optionFilterProp="label" options={labels.map((l) => ({ value: l, label: l }))} placeholder="选择图内实体" />
          </Form.Item>
          <Form.Item name="definition" label="依据说明（可选）">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 3 }} maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
