import { useMemo, useState } from 'react'
import { Alert, Button, Empty, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../../../../api/client'
import { useUI } from '../../../../store/ui'
import type { Spec, SpecAxiom, SpecConcept } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-269/M78（D-O22 实用化升级批次三）：公理管理页签——spec.axioms 保留层集中管理：
//   类型（disjoint_with/equivalent_class）/主体/目标（实例无公理语义，纯声明层）；纯前端从 spec 派生零接口。
//   v1 只读（公理由真实资产导入保真承载，LLM 生成路径不产出公理——严格语义防幻觉，诚实边界随行）。
// REQ-472/M272（Protégé 编辑能力对齐批①）：v1 只读 → CRUD——AxiomsPane 走元素端点 kind=axiom
//   （组键 (type,subject)，targets 整组替换；Validate 拦悬空引用）。REQ-376④「独立页签等诉求触发」
//   条件由本轮开发者指令（protege 能做的基本都要支持）满足——LLM 生成面仍不产公理（77 号闸门维持）。
// ---------------------------------------------------------------------------

const TYPE_META: Record<string, { color: string; label: string }> = {
  disjoint_with: { color: 'red', label: '互斥（disjointWith）' },
  equivalent_class: { color: 'geekblue', label: '等价（equivalentClass）' },
}

interface AxRow {
  key: string
  ax: SpecAxiom
}

export default function AxiomsPane({ spec, ontologyId, onSaved }: { spec: Spec | null; ontologyId: string; onSaved: () => void }) {
  const { showToast } = useUI()
  const [q, setQ] = useState('')
  const [editOpen, setEditOpen] = useState(false)
  const [editing, setEditing] = useState<SpecAxiom | null>(null) // null=新增
  const [busy, setBusy] = useState(false)
  const [form, setForm] = useState<{ type: string; subject?: string; targets: string[] }>({ type: 'disjoint_with', targets: [] })

  const conceptNames = useMemo(() => (spec?.concepts ?? []).map((c: SpecConcept) => c.name), [spec])

  const rows = useMemo<AxRow[]>(
    () =>
      (spec?.axioms ?? []).map((ax, i) => ({
        key: `${ax.type}-${ax.subject}-${i}`,
        ax,
      })),
    [spec],
  )

  const ql = q.trim().toLowerCase()
  const filtered = rows.filter(({ ax }) => !ql || [ax.type, ax.subject, ...(ax.targets ?? [])].some((s) => (s ?? '').toLowerCase().includes(ql)))

  const openCreate = () => {
    setEditing(null)
    setForm({ type: 'disjoint_with', subject: undefined, targets: [] })
    setEditOpen(true)
  }
  const openEdit = (ax: SpecAxiom) => {
    setEditing(ax)
    setForm({ type: ax.type, subject: ax.subject, targets: [...(ax.targets ?? [])] })
    setEditOpen(true)
  }

  const submit = async () => {
    if (!form.subject || form.targets.length === 0) {
      showToast('主体概念与目标概念（≥1）必填', 'err')
      return
    }
    if (form.targets.includes(form.subject)) {
      showToast('目标不应包含主体自身', 'err')
      return
    }
    setBusy(true)
    try {
      const r = await api.elementUpsert(ontologyId, 'axiom', { type: form.type, subject: form.subject, targets: form.targets })
      showToast(r.detail ?? '公理已保存')
      setEditOpen(false)
      onSaved()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  const remove = async (ax: SpecAxiom) => {
    try {
      const r = await api.elementDelete(ontologyId, 'axiom', `${ax.type}:${ax.subject}`)
      showToast(r.detail ?? '公理已删除')
      onSaved()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const columns: ColumnsType<AxRow> = [
    {
      title: '类型',
      dataIndex: ['ax', 'type'],
      width: 200,
      render: (v: string) => {
        const m = TYPE_META[v] ?? { color: 'default', label: v }
        return <Tag color={m.color} style={{ margin: 0 }}>{m.label}</Tag>
      },
    },
    { title: '主体概念', dataIndex: ['ax', 'subject'], width: 180, render: (v: string) => <Typography.Text strong style={{ fontSize: 12.5 }}>{v}</Typography.Text> },
    {
      title: '目标概念',
      dataIndex: ['ax', 'targets'],
      render: (targets: string[]) => (
        <span>
          {(targets ?? []).map((t) => (
            <Tag key={t} style={{ margin: '0 4px 0 0' }}>{t}</Tag>
          ))}
        </span>
      ),
    },
    {
      title: '',
      width: 130,
      render: (_: unknown, row: AxRow) => (
        <Space size={4}>
          <Button size="small" onClick={() => openEdit(row.ax)}>编辑</Button>
          <Popconfirm title={`删除公理 ${row.ax.type}(${row.ax.subject})？`} okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(row.ax)}>
            <Button size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  if (!spec) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="加载中" />
  return (
    <div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        title="公理保留层（REQ-269；REQ-472 升可编辑）"
        description="类级 owl:disjointWith / owl:equivalentClass——导入保真捕获或在此手工增删（组键=类型+主体，目标整组替换）；TTL 导出按概念同 IRI 口径回写；悬空引用被 spec 校验与质量门禁双重拦截。LLM 生成路径不产出公理（严格语义防幻觉，77 号闸门维持）。"
      />
      <div style={{ marginBottom: 8, display: 'flex', gap: 8 }}>
        <Input.Search
          size="small"
          allowClear
          placeholder="搜索类型/概念名"
          style={{ maxWidth: 240 }}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <span style={{ flex: 1 }} />
        <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate} data-testid="axiom-create-btn">新增公理</Button>
      </div>
      {rows.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本本体无公理——导入含 owl:disjointWith / owl:equivalentClass 的 OWL/TTL 资产自动捕获，或点「新增公理」手工声明" />
      ) : (
        <Table
          size="small"
          rowKey={(r) => r.key}
          columns={columns}
          dataSource={filtered}
          pagination={rows.length > 10 ? { pageSize: 10 } : false}
        />
      )}

      <Modal
        open={editOpen}
        centered
        keyboard={false} /* Select 下拉 Esc 冒泡会误触 onCancel 关模态（真机实锤）——表单模态禁 Esc 关闭 */
        title={editing ? `编辑公理（${TYPE_META[editing.type]?.label ?? editing.type}）` : '新增公理'}
        width={520}
        onCancel={() => setEditOpen(false)}
        confirmLoading={busy}
        onOk={submit}
        okText="保存"
        cancelText="取消"
      >
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>公理类型</Typography.Text>
            <Select
              style={{ width: '100%' }}
              value={form.type}
              disabled={!!editing}
              onChange={(v) => setForm((f) => ({ ...f, type: v }))}
              options={Object.entries(TYPE_META).map(([v, m]) => ({ value: v, label: m.label }))}
            />
          </div>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>主体概念{editing ? '（组键，不可改）' : ''}</Typography.Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              optionFilterProp="label"
              placeholder="选择主体概念"
              value={form.subject}
              disabled={!!editing}
              onChange={(v) => setForm((f) => ({ ...f, subject: v }))}
              options={conceptNames.map((n) => ({ value: n, label: n }))}
            />
          </div>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>目标概念（多选 ≥1）</Typography.Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              showSearch
              optionFilterProp="label"
              placeholder="选择目标概念"
              value={form.targets}
              onChange={(v) => setForm((f) => ({ ...f, targets: v }))}
              options={conceptNames.map((n) => ({ value: n, label: n }))}
              data-testid="axiom-targets-select"
            />
          </div>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            语义提示：互斥=两类无共同实例（推理器可检出不一致）；等价=两类同延。保存即触发校验（悬空引用拒绝）。
          </Typography.Text>
        </Space>
      </Modal>
    </div>
  )
}
