import { useMemo, useState } from 'react'
import { Alert, Card, Select, Space, Tabs, Tag, Typography } from 'antd'
import Graph3D from '../../../web/src/pages/ontology/components/Graph3D'
import SpecGraph from '../../../web/src/pages/ontology/components/SpecGraph'
import { SEEDS, seedCounts } from '../content'
import { useCopy } from '../i18n'

// ---------------------------------------------------------------------------
// 在线演示页（REQ-238）：内置种子本体（后端 internal/seed 启动播种同一批）构建产物的
// 可视化——三维图谱（Graph3D）与二维图谱（SpecGraph）均为纯前端渲染、零后端依赖，
// 组件直接复用 web/src（平台资产可视化同款）。诚实边界：仅展示静态构建产物，
// 构建/运行/对话/抽取/审计等完整能力需部署平台。
// REQ-360：页面 chrome 双语；种子 spec 数据（名称/描述/图内标签）与复用平台组件的
// 内置文案维持原样（内容与平台 i18n 范畴，不在本批）。
// ---------------------------------------------------------------------------

const COPY = {
  zh: {
    title: '在线演示 · 内置示例本体',
    taglinePre: '以下为平台内置种子本体的',
    taglineStrong: '静态可视化演示',
    taglineMid: '（与后端启动播种同源数据）——三维/二维图谱渲染复用平台资产可视化组件，可拖拽、缩放、搜索、点选节点查看定义。',
    alertTitle: '这是静态演示数据',
    alertDesc: '六条构建路径、运行方案、对话挂载、知识抽取、消费审计等完整能力需要本地或服务器部署平台后体验（见「快速开始」）。',
    concepts: '概念',
    relations: '关系',
    instances: '实例',
    tab3d: '三维浏览',
    tab2d: '二维图谱',
  },
  en: {
    title: 'Online Demo · Built-in Example Ontologies',
    taglinePre: 'Below is a ',
    taglineStrong: 'static visualization demo',
    taglineMid: ' of the platform’s built-in seed ontologies (same data the backend seeds on startup) — the 3D/2D graph views reuse the platform’s asset-visualization components. Drag, zoom, search, and click nodes to see definitions.',
    alertTitle: 'This is static demo data',
    alertDesc: 'Full capabilities — the six build paths, runtime plans, chat mounting, knowledge extraction, consumption & audit — require deploying the platform locally or on a server (see "Quick start").',
    concepts: 'Concepts',
    relations: 'Relations',
    instances: 'Instances',
    tab3d: '3D view',
    tab2d: '2D graph',
  },
}

export default function DemoPage() {
  const c = useCopy(COPY)
  const [key, setKey] = useState(SEEDS[0].key)
  const seed = useMemo(() => SEEDS.find((s) => s.key === key) ?? SEEDS[0], [key])
  const counts = seedCounts(seed.spec)

  return (
    <main className="page">
      <div className="page-inner">
        <h1>{c.title}</h1>
        <p className="module-tagline">
          {c.taglinePre}
          <strong>{c.taglineStrong}</strong>
          {c.taglineMid}
        </p>
        <Alert
          type="info"
          showIcon
          style={{ margin: '12px 0 16px' }}
          title={c.alertTitle}
          description={c.alertDesc}
        />

        <div className="demo-bar">
          <Space size={10} wrap>
            <Select
              value={key}
              onChange={setKey}
              style={{ width: 320 }}
              options={SEEDS.map((s) => ({ value: s.key, label: s.spec.name }))}
            />
            <Tag style={{ margin: 0 }}>
              {c.concepts} {counts.concepts}
            </Tag>
            <Tag style={{ margin: 0 }}>
              {c.relations} {counts.relations}
            </Tag>
            <Tag style={{ margin: 0 }}>
              {c.instances} {counts.instances}
            </Tag>
            <Tag color="geekblue" style={{ margin: 0 }}>
              seed/{seed.file}
            </Tag>
          </Space>
        </div>
        {seed.spec.description && (
          <Typography.Paragraph type="secondary" className="demo-desc">
            {seed.spec.description}
          </Typography.Paragraph>
        )}

        <Card size="small" className="demo-viz-card">
          <Tabs
            key={seed.key}
            defaultActiveKey="3d"
            items={[
              {
                key: '3d',
                label: c.tab3d,
                children: (
                  <div className="demo-viz">
                    <Graph3D spec={seed.spec} />
                  </div>
                ),
              },
              {
                key: '2d',
                label: c.tab2d,
                children: (
                  <div className="demo-viz">
                    <SpecGraph spec={seed.spec} />
                  </div>
                ),
              },
            ]}
          />
        </Card>
      </div>
    </main>
  )
}
