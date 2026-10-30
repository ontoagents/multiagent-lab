import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Collapse, Empty, Space, Spin, Table, Tag, Typography } from 'antd'
import { ApiOutlined, ReloadOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { ClosurePreview } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-542/M349 构建面只读闭包预览（103 号 §6.5 步骤二前半，修痛点 P2 剩余面）：
// 运行面 expandClosure 算法前移的只读端点——三出料=①全量依赖闭包图（nodes+edges）
// ②解析后 schema 预览（引用方+被引方按 pin/head 解析合成）③循环引用专项报告。
// 不回写不阻断（D-O27 边界扩写，03 §5 行注）：悬空引用入报告不拦截、零写副作用。
// 与 REQ-278 变更影响分析（/impact 版本 diff+引用波及面）为扩面衔接——本面板补全量
// 闭包图与解析后 schema 视图；版本演进前先看「变更影响分析」、依赖全貌看本面板。
// ---------------------------------------------------------------------------

const RES_TAG: Record<string, { color: string; text: string }> = {
  root: { color: 'geekblue', text: 'root' },
  head: { color: 'cyan', text: 'head 浮动' },
  pin: { color: 'purple', text: 'pin 钉住' },
  missing: { color: 'red', text: 'missing 已删' },
}

export default function ClosurePreviewPane({ ontologyId }: { ontologyId: string }) {
  const [preview, setPreview] = useState<ClosurePreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      setPreview(await api.closurePreview(ontologyId))
    } catch (e: any) {
      setErr(e.message ?? String(e))
      setPreview(null)
    } finally {
      setLoading(false)
    }
  }, [ontologyId])

  useEffect(() => {
    load()
  }, [load])

  if (loading && !preview) return <Spin style={{ display: 'block', margin: '24px auto' }} />
  if (err)
    return (
      <Alert type="error" showIcon title="闭包预览失败" description={err} style={{ marginBottom: 8 }} />
    )
  if (!preview) return <Empty description="无预览数据" />

  const nodeById = new Map(preview.nodes.map((n) => [n.onto_id, n]))
  const nodeLabel = (id: string) => {
    const n = nodeById.get(id)
    const tag = RES_TAG[n?.resolution ?? ''] ?? { color: 'default', text: n?.resolution ?? '' }
    return (
      <span>
        <Typography.Text strong style={{ fontSize: 12 }}>{id}</Typography.Text>
        {n && n.resolution !== 'root' && (
          <Tag color={tag.color} style={{ marginInlineStart: 6, fontSize: 11 }}>
            {tag.text}
            {n.resolution !== 'missing' ? ` v${n.version}` : ''}
          </Tag>
        )}
        {n?.name && n.name !== id && (
          <Typography.Text type="secondary" style={{ marginInlineStart: 4, fontSize: 11 }}>{n.name}</Typography.Text>
        )}
      </span>
    )
  }

  return (
    <div data-testid="closure-preview-pane">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="依赖闭包预览（REQ-542，只读不回写不阻断）"
        description="运行面装载闭包算法前移的构建面只读预览：全量依赖闭包图 + 解析后 schema（pin/head 版本解析）+ 循环引用专项报告。悬空引用只报告不阻断（装载拦截在运行平面）；远端 IRI 不拉取（D-O27）。版本演进波及面请配合「变更影响分析」（REQ-278）。"
      />
      <Space size={8} wrap style={{ marginBottom: 12 }}>
        <Button size="small" icon={<ReloadOutlined />} onClick={load} loading={loading}>
          刷新
        </Button>
        <Tag data-testid="closure-nodes-count" style={{ margin: 0 }}>
          本体 {preview.nodes.length}
        </Tag>
        <Tag style={{ margin: 0 }}>装载边 {preview.edges.length}</Tag>
        <Tag data-testid="closure-concepts-count" style={{ margin: 0 }}>
          合成概念 {preview.schema.total_concepts}
        </Tag>
        <Tag color={preview.cycles.length > 0 ? 'red' : 'default'} style={{ margin: 0 }} data-testid="closure-cycles-count">
          环 {preview.cycles.length}
        </Tag>
        {preview.dangling && preview.dangling.length > 0 && (
          <Tag color="orange" style={{ margin: 0 }} data-testid="closure-dangling-count">
            悬空 {preview.dangling.length}
          </Tag>
        )}
      </Space>

      {/* ①全量依赖闭包图 */}
      <div className="onto-sec" style={{ marginTop: 0 }}>
        <span className="onto-sec-title">① 全量依赖闭包图（{preview.nodes.length} 节点 / {preview.edges.length} 边）</span>
      </div>
      <div style={{ marginBottom: 8, display: 'flex', flexWrap: 'wrap', gap: 6 }} data-testid="closure-graph-nodes">
        {preview.nodes.map((n) => (
          <Tag key={n.onto_id} color={RES_TAG[n.resolution]?.color ?? 'default'} style={{ margin: 0 }} data-testid={`closure-node-${n.onto_id}`}>
            {n.onto_id}
            {n.resolution === 'root' ? '（root）' : n.resolution === 'missing' ? '（已删）' : ` v${n.version}`}
            {n.concepts > 0 ? ` · ${n.concepts} 概念` : ''}
          </Tag>
        ))}
      </div>
      <Table
        size="small"
        rowKey={(r, i) => `${r.from_onto_id}->${r.to_onto_id}:${i}`}
        pagination={false}
        dataSource={preview.edges}
        locale={{ emptyText: '无仓内依赖（闭包=根本体自身）' }}
        columns={[
          { title: '引用方', dataIndex: 'from_onto_id', width: 160, render: (v: string) => nodeLabel(v) },
          { title: '被引方', dataIndex: 'to_onto_id', width: 160, render: (v: string) => nodeLabel(v) },
          {
            title: '类型', dataIndex: 'ref_kind', width: 110,
            render: (v: string, r) => (
              <span>
                <Tag style={{ margin: 0 }}>{v}</Tag>
                {r.via_imports && <Tag style={{ margin: 0, fontSize: 11 }}>owl:imports</Tag>}
              </span>
            ),
          },
          { title: '概念引用', dataIndex: 'from_concept', ellipsis: true, render: (_: unknown, r) => (
            <Typography.Text style={{ fontSize: 12 }}>{r.from_concept} → {r.to_concept}</Typography.Text>
          ) },
          {
            title: '状态', dataIndex: 'dangling', width: 110,
            render: (v: string | undefined) =>
              v ? <Tag color="orange" style={{ margin: 0 }}>{v === 'missing_ontology' ? '目标已删' : '概念不存在'}</Tag> : <Tag color="green" style={{ margin: 0 }}>正常</Tag>,
          },
        ]}
        style={{ marginBottom: 16 }}
      />

      {/* ②解析后 schema 预览 */}
      <div className="onto-sec">
        <span className="onto-sec-title">② 解析后 schema 预览（{preview.schema.total_concepts} 概念合成视图，pin/head 版本解析）</span>
      </div>
      {preview.pin_conflicts && Object.keys(preview.pin_conflicts).length > 0 && (
        <Alert
          type="warning" showIcon style={{ marginBottom: 8 }}
          title="pin 冲突（REQ-540）：同目标多登记 pin 不一致——解析取最低版本（最保守），较高 pin 期望的概念可能不在下方合成视图"
          description={Object.entries(preview.pin_conflicts).map(([t, v]) => `${t}: ${v}`).join('；')}
        />
      )}
      {preview.schema.name_conflicts && preview.schema.name_conflicts.length > 0 && (
        <Alert
          type="warning" showIcon style={{ marginBottom: 8 }}
          title={`跨本体同名概念 ${preview.schema.name_conflicts.length} 处（合成视图需人工裁决）`}
          description={preview.schema.name_conflicts.map((c) => `${c.name} ← [${c.onto_ids.join(', ')}]`).join('；')}
        />
      )}
      <Collapse
        size="small"
        style={{ marginBottom: 16 }}
        items={preview.schema.sections.map((sec) => ({
          key: sec.onto_id,
          label: (
            <span data-testid={`closure-schema-${sec.onto_id}`}>
              <Typography.Text strong style={{ fontSize: 12 }}>{sec.onto_id}</Typography.Text>
              <Tag color={RES_TAG[sec.resolution]?.color ?? 'default'} style={{ marginInlineStart: 6, fontSize: 11 }}>
                {sec.resolution} v{sec.version}
              </Tag>
              <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                概念 {sec.concepts.length} · 关系 {sec.relations.length}
                {sec.data_properties?.length ? ` · 数据属性 ${sec.data_properties.length}` : ''}
              </Typography.Text>
            </span>
          ),
          children: (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {sec.concepts.map((c) => (
                <div key={c.name} style={{ fontSize: 12 }}>
                  <Typography.Text strong>{c.name}</Typography.Text>
                  {c.parents && c.parents.length > 0 && (
                    <Typography.Text type="secondary" style={{ marginInlineStart: 6 }}>
                      ⊂ {c.parents.join(', ')}
                    </Typography.Text>
                  )}
                  {c.definition && (
                    <div><Typography.Text type="secondary" style={{ fontSize: 11 }}>{c.definition}</Typography.Text></div>
                  )}
                </div>
              ))}
              {sec.relations.map((r) => (
                <div key={r.name} style={{ fontSize: 12 }}>
                  <Tag style={{ marginInlineEnd: 6, fontFamily: 'monospace' }}>{r.name}</Tag>
                  <Typography.Text type="secondary">{r.from} → {r.to}</Typography.Text>
                </div>
              ))}
              {sec.data_properties?.map((p) => (
                <div key={p.name} style={{ fontSize: 12 }}>
                  <Tag style={{ marginInlineEnd: 6, fontFamily: 'monospace' }}>{p.name}</Tag>
                  <Typography.Text type="secondary">
                    {p.domain ? `${p.domain} · ` : ''}{p.range ?? 'string'}
                  </Typography.Text>
                </div>
              ))}
            </div>
          ),
        }))}
      />

      {/* ③循环引用专项报告 */}
      <div className="onto-sec">
        <span className="onto-sec-title">③ 循环引用专项报告</span>
      </div>
      {preview.cycles.length > 0 ? (
        <Alert
          type="warning" showIcon style={{ marginBottom: 8 }}
          title={`检出 ${preview.cycles.length} 条互引环（环语义=并集装载不受影响，报告仅为可观测）`}
          description={
            <ul style={{ margin: 0, paddingLeft: 18 }} data-testid="closure-cycles">
              {preview.cycles.map((cyc, i) => (
                <li key={i} style={{ fontSize: 12 }}>{cyc.join(' → ')}</li>
              ))}
            </ul>
          }
        />
      ) : (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }} data-testid="closure-cycles">
          无循环引用。
        </Typography.Text>
      )}
      {preview.dangling && preview.dangling.length > 0 && (
        <Alert
          type="warning" showIcon style={{ marginBottom: 8 }}
          title={`悬空引用 ${preview.dangling.length} 条（预览不阻断；装载由运行面门禁拦截）`}
          description={
            <ul style={{ margin: 0, paddingLeft: 18 }} data-testid="closure-dangling">
              {preview.dangling.map((d, i) => (
                <li key={i} style={{ fontSize: 12 }}>{d.detail}</li>
              ))}
            </ul>
          }
        />
      )}
      {preview.pins && Object.keys(preview.pins).length > 0 && (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }} data-testid="closure-pins">
          <ApiOutlined style={{ marginInlineEnd: 4 }} />
          pin 解析（REQ-540）：{Object.entries(preview.pins).map(([t, v]) => `${t}→v${v}`).join('、')}；被引方头版本演进不自动跟随，显式刷新 pin 才取新版。
        </Typography.Text>
      )}
      {preview.remote_skipped && preview.remote_skipped.length > 0 && (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
          远端 IRI 跳过（不拉取，D-O27）：{preview.remote_skipped.join('、')}
        </Typography.Text>
      )}
    </div>
  )
}
