import { useMemo, useState } from 'react'
import { Empty, Input, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Spec } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-240④/M66：关系类型管理页签（57 号 V2）——spec.relations 定义集中管理：
//   关系名/标签/定义域→值域/实例引用计数（InstanceRel.rel 反查）；纯前端从 spec 派生零接口。
//   与 REQ-235 数据属性建模的衔接口径见 03 §2.16（本页签为其管理面前身）。
// ---------------------------------------------------------------------------

interface RelRow {
  key: string
  name: string
  label: string
  from: string
  to: string
  refs: string[] // 引用该类型的实例关系（实例名 × rel）
}

export default function RelationTypesPane({ spec }: { spec: Spec | null }) {
  const [q, setQ] = useState('')
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

  if (!spec || (spec.relations?.length ?? 0) === 0) {
    return (
      <div className="work-empty" style={{ minHeight: 180 }}>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无关系类型定义——在 Spec 编辑或图形编辑中添加关系后此处集中呈现" />
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
    </div>
  )
}
