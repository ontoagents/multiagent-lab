import { useMemo, useState } from 'react'
import { Alert, Empty, Input, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Spec, SpecAxiom } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-269/M78（D-O22 实用化升级批次三）：公理管理页签——spec.axioms 保留层集中管理：
//   类型（disjoint_with/equivalent_class）/主体/目标（实例无公理语义，纯声明层）；纯前端从 spec 派生零接口。
//   v1 只读（公理由真实资产导入保真承载，LLM 生成路径不产出公理——严格语义防幻觉，诚实边界随行）。
// ---------------------------------------------------------------------------

const TYPE_META: Record<string, { color: string; label: string }> = {
  disjoint_with: { color: 'red', label: '互斥（disjointWith）' },
  equivalent_class: { color: 'geekblue', label: '等价（equivalentClass）' },
}

interface AxRow {
  key: string
  ax: SpecAxiom
}

export default function AxiomsPane({ spec }: { spec: Spec | null }) {
  const [q, setQ] = useState('')
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
    { title: '主体概念', dataIndex: ['ax', 'subject'], width: 200, render: (v: string) => <Typography.Text strong style={{ fontSize: 12.5 }}>{v}</Typography.Text> },
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
  ]

  if (!spec) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="加载中" />
  return (
    <div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        title="公理保留层（REQ-269）"
        description="由真实资产导入保真承载（owl:disjointWith / owl:equivalentClass，导入时未注册端点如实丢弃并计入有损清单）；TTL 导出按概念同 IRI 口径回写；悬空引用被 spec 校验与质量门禁双重拦截。LLM 生成路径不产出公理（严格语义防幻觉）。"
      />
      {rows.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本本体无公理——导入含 owl:disjointWith / owl:equivalentClass 的 OWL/TTL 资产即自动捕获入此层" />
      ) : (
        <>
          <Input.Search
            size="small"
            allowClear
            placeholder="搜索类型/概念名"
            style={{ maxWidth: 240, marginBottom: 8 }}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <Table
            size="small"
            rowKey={(r) => r.key}
            columns={columns}
            dataSource={filtered}
            pagination={rows.length > 10 ? { pageSize: 10 } : false}
          />
        </>
      )}
    </div>
  )
}
