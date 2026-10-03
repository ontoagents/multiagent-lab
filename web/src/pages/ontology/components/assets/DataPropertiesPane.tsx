import { useMemo, useState } from 'react'
import { Alert, Empty, Input, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Spec, SpecDataProperty } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-268/M77（D-O22 实用化升级批次二）：数据属性管理页签——spec.data_properties 声明集中管理：
//   名称/标签/定义/定义域/值域类型/实例引用计数（instance attributes 键反查）；纯前端从 spec 派生零接口。
//   v1 只读（编辑经 Spec JSON / 导入捕获 / LLM·结构化路径生成）；图形编辑器画布渲染未含数据属性（诚实边界）。
// ---------------------------------------------------------------------------

interface DpRow {
  key: string
  dp: SpecDataProperty
  refs: string[] // 引用该属性的实例名（instance attributes 键命中）
}

export default function DataPropertiesPane({ spec }: { spec: Spec | null }) {
  const [q, setQ] = useState('')
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

  if (!rows.length) {
    return (
      <Empty
        description={
          <span style={{ fontSize: 12, color: 'var(--c-ink-3)' }}>
            本体尚无数据属性声明——可经导入（owl:DatatypeProperty 自动捕获）、Spec JSON 编辑 data_properties 字段、或 LLM/结构化生成路径产生
          </span>
        }
        style={{ padding: '32px 0' }}
      />
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
      <Input.Search
        allowClear
        size="small"
        placeholder="搜索属性名/标签/定义/定义域"
        style={{ width: 260, marginBottom: 8 }}
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <Table<DpRow>
        size="small"
        rowKey="key"
        columns={columns}
        dataSource={filtered}
        pagination={filtered.length > 20 ? { pageSize: 20 } : false}
      />
    </div>
  )
}
