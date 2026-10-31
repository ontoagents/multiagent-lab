import { useState } from 'react'
import { Checkbox, Collapse, Space, Tag, Typography } from 'antd'
import type { Spec } from '../../../api/types'

// REQ-503/M311①：AI 草案确认分组勾选（自产成果裁剪——沿 DraftConfirmPanel DraftPicks 先例平移，
// 脱离 OntoChat 会话依赖；origin 徽标=组级「AI 草稿」标注——单轮生成来源统一如实呈现）。
// onChange 回传裁剪后 spec；父组件持工作副本（aiTrimmed），saveAiDraft 消费。

type Group = 'concepts' | 'relations' | 'data_properties' | 'instances'

const GROUP_META: { key: Group; label: string; nameOf: (x: any) => string }[] = [
  { key: 'concepts', label: '概念', nameOf: (x) => x.name },
  { key: 'relations', label: '关系', nameOf: (x) => `${x.from} →${x.name}→ ${x.to}` },
  { key: 'data_properties', label: '数据属性', nameOf: (x) => `${x.domain || '*'} · ${x.name}` },
  { key: 'instances', label: '实例', nameOf: (x) => x.name },
]

const nameOf = (group: Group, x: any): string => (GROUP_META.find((g) => g.key === group))!.nameOf(x)

export default function DraftTrimPanel({ spec, onChange }: { spec: Spec; onChange: (next: Spec) => void }) {
  const [removed, setRemoved] = useState<Record<Group, Set<string>>>({
    concepts: new Set(),
    relations: new Set(),
    data_properties: new Set(),
    instances: new Set(),
  })

  const applyTrim = (group: Group, names: Set<string>) => {
    const items = (spec[group] as any[] | undefined) ?? []
    const kept = items.filter((x) => !names.has(nameOf(group, x)))
    onChange({ ...spec, [group]: kept } as Spec)
  }

  const toggleItem = (group: Group, name: string, checked: boolean) => {
    const next = new Set(removed[group])
    if (checked) {
      next.delete(name)
    } else {
      next.add(name)
    }
    setRemoved({ ...removed, [group]: next })
    applyTrim(group, next)
  }

  const groupPane = (group: Group) => {
    const meta = GROUP_META.find((g) => g.key === group)!
    const items = (spec[group] as any[] | undefined) ?? []
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }} data-testid={`trim-group-${group}`}>
        {items.length === 0 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>（无）</Typography.Text>}
        {items.map((x) => {
          const name = meta.nameOf(x)
          const checked = !removed[group].has(name)
          return (
            <Checkbox key={name} checked={checked} onChange={(e) => toggleItem(group, name, e.target.checked)}>
              <Typography.Text style={{ fontSize: 12.5 }}>{name}</Typography.Text>
            </Checkbox>
          )
        })}
      </div>
    )
  }

  const totalCount = GROUP_META.reduce((acc, g) => acc + ((spec[g.key] as any[] | undefined) ?? []).length, 0)
  return (
    <div data-testid="ai-trim-panel">
      <Space size={6} wrap style={{ marginBottom: 6 }}>
        <Typography.Text strong style={{ fontSize: 12.5 }}>草案确认（勾选保留，去勾即从草案移除）</Typography.Text>
        <Tag style={{ margin: 0, fontSize: 10 }}>AI 草稿</Tag>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>共 {totalCount} 项 · 裁剪后点「创建并保存」生效</Typography.Text>
      </Space>
      <Collapse
        size="small"
        items={GROUP_META.map((g) => ({
          key: g.key,
          label: `${g.label}（${((spec[g.key] as any[] | undefined) ?? []).length}）`,
          children: groupPane(g.key),
          forceRender: true, // REQ-503：测试/无头环境可达（Collapse 默认懒渲染）
        }))}
      />
    </div>
  )
}
