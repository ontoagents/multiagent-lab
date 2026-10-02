import { useState } from 'react'
import { Button, Card, Collapse, Empty, Input, Space, Spin, Tag, Typography } from 'antd'
import { SearchOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'

// ---------------------------------------------------------------------------
// REQ-246/G1：S1 空白新建「参考素材」面板——从零建模不再面对全空白：
// ① 7 份种子建模说明可见化（Vite ?raw 构建期内联，单源 seeds/learning/examples/，
//    REQ-109 external-resources 同范式）；② 一键「从种子起步」（fork 语义：seedLearning
//    灌装后 onSelect 跳编辑）；③ ODP 12 模式浏览（此前锁死 OntoExtend 路径）；④ LOV 词表搜索。
// ---------------------------------------------------------------------------

// 构建期内联种子建模说明（背景/CQ/决策记录/局限）
import readmeK8s from '../../../../../seeds/learning/examples/onto_k8s_ops.README.md?raw'
import readmeDefects from '../../../../../seeds/learning/examples/defects.README.md?raw'
import readmeOrgs from '../../../../../seeds/learning/examples/orgs.README.md?raw'
import readmeFailure from '../../../../../seeds/learning/examples/failure.README.md?raw'
import readmeMed from '../../../../../seeds/learning/examples/med_common.README.md?raw'
import readmeGene from '../../../../../seeds/learning/examples/gene_core.README.md?raw'
import readmeDevice from '../../../../../seeds/learning/examples/device_fault.README.md?raw'

const SEED_READMES: { key: string; label: string; md: string; seedKey: string }[] = [
  { key: 'k8s', label: 'K8s 迷你运维', md: readmeK8s, seedKey: 'onto_k8s_ops' },
  { key: 'defects', label: '软件缺陷管理', md: readmeDefects, seedKey: 'defects' },
  { key: 'orgs', label: '组织与人员', md: readmeOrgs, seedKey: 'orgs' },
  { key: 'failure', label: '设备故障知识', md: readmeFailure, seedKey: 'failure' },
  { key: 'med', label: '医学常识（百级）', md: readmeMed, seedKey: 'med_common' },
  { key: 'gene', label: '基因与中心法则（百级）', md: readmeGene, seedKey: 'gene_core' },
  { key: 'device', label: '设备故障 KB 语料对照', md: readmeDevice, seedKey: 'device_fault' },
]

type Odp = { id: string; name: string; description: string }
type LovHit = { label?: string; uri?: string; [k: string]: unknown }

export default function ReferenceMaterialsPanel({ onStartFromSeed }: { onStartFromSeed?: (seedKey: string) => void }) {
  const [tab, setTab] = useState<'seeds' | 'odp' | 'lov'>('seeds')
  const [odps, setOdps] = useState<Odp[] | null>(null)
  const [odpsErr, setOdpsErr] = useState<string | null>(null)
  const [lovQ, setLovQ] = useState('')
  const [lovHits, setLovHits] = useState<LovHit[]>([])
  const [lovLoading, setLovLoading] = useState(false)
  const [lovErr, setLovErr] = useState<string | null>(null)

  const loadOdps = () => {
    if (odps) return
    api
      .ontoExtendODPs()
      .then((d: any) => setOdps((d.odps ?? []) as Odp[]))
      .catch((e: any) => setOdpsErr(e?.message ?? 'ODP 加载失败'))
  }
  const searchLov = () => {
    const q = lovQ.trim()
    if (!q) return
    setLovLoading(true)
    setLovErr(null)
    api
      .vocabSearch(q)
      .then((d) => setLovHits(((d.results ?? d.items ?? []) as LovHit[]).slice(0, 10)))
      .catch((e: any) => setLovErr(e?.message ?? 'LOV 检索失败（上游不可达时为预期降级）'))
      .finally(() => setLovLoading(false))
  }

  return (
    <Card
      size="small"
      title="参考素材（从零建模不必从空白开始想）"
      style={{ marginTop: 12 }}
      styles={{ body: { paddingTop: 8 } }}
      data-testid="reference-materials"
    >
      <Space size={6} style={{ marginBottom: 8 }}>
        {(
          [
            ['seeds', '种子建模说明'],
            ['odp', 'ODP 模式库'],
            ['lov', 'LOV 词表'],
          ] as const
        ).map(([k, label]) => (
          <Button key={k} size="small" type={tab === k ? 'primary' : 'default'} onClick={() => { setTab(k); if (k === 'odp') loadOdps() }}>
            {label}
          </Button>
        ))}
      </Space>

      {tab === 'seeds' && (
        <Collapse
          size="small"
          items={SEED_READMES.map((s) => ({
            key: s.key,
            label: (
              <Space size={6}>
                <span>{s.label}</span>
                <Tag style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>建模说明</Tag>
              </Space>
            ),
            children: (
              <div>
                <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12, maxHeight: 220, overflow: 'auto', margin: 0 }}>{s.md}</pre>
                {onStartFromSeed && (
                  <Button size="small" type="link" style={{ paddingLeft: 0 }} onClick={() => onStartFromSeed(s.seedKey)}>
                    从这份种子起步（灌装后到资产编辑） →
                  </Button>
                )}
              </div>
            ),
          }))}
        />
      )}

      {tab === 'odp' &&
        (odpsErr ? (
          <Typography.Text type="danger" style={{ fontSize: 12 }}>{odpsErr}</Typography.Text>
        ) : !odps ? (
          <Spin size="small" />
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 6 }}>
            {odps.map((o) => (
              <Card key={o.id} size="small" style={{ background: 'var(--c-panel)' }}>
                <Typography.Text strong style={{ fontSize: 12 }}>{o.name}</Typography.Text>
                <Typography.Paragraph type="secondary" style={{ fontSize: 11, margin: '4px 0 0', minHeight: 30 }}>
                  {o.description}
                </Typography.Paragraph>
              </Card>
            ))}
            {odps.length === 0 && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="无 ODP" />}
          </div>
        ))}

      {tab === 'lov' && (
        <div>
          <Space.Compact style={{ width: '100%', maxWidth: 480 }}>
            <Input
              size="small"
              prefix={<SearchOutlined />}
              placeholder="检索 LOV 词表术语（如 foaf、schema.org 概念）"
              value={lovQ}
              onChange={(e) => setLovQ(e.target.value)}
              onPressEnter={searchLov}
            />
            <Button size="small" loading={lovLoading} onClick={searchLov}>
              搜索
            </Button>
          </Space.Compact>
          {lovErr && <Typography.Text type="warning" style={{ fontSize: 12, display: 'block', marginTop: 6 }}>{lovErr}</Typography.Text>}
          <div style={{ marginTop: 8, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {lovHits.map((h, i) => (
              <Tag key={i} style={{ margin: 0 }}>
                {String(h.label ?? h.uri ?? JSON.stringify(h)).slice(0, 60)}
              </Tag>
            ))}
            {lovHits.length > 0 && (
              <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                术语可作为概念名/标签参考（复用成熟词表是建模方法论之一）
              </Typography.Text>
            )}
          </div>
        </div>
      )}
    </Card>
  )
}
