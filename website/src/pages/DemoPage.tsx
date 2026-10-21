import { useMemo, useState } from 'react'
import { Alert, Card, Select, Space, Tabs, Tag, Typography } from 'antd'
import Graph3D from '../../../web/src/pages/ontology/components/Graph3D'
import SpecGraph from '../../../web/src/pages/ontology/components/SpecGraph'
import HierarchyPane, { type OntoSelection } from '../../../web/src/pages/ontology/components/assets/HierarchyPane'
import { ChatWorkbenchMock, OntologyConsoleMock } from '../components/mockups'
import { SEEDS, seedCounts } from '../content'
import { useCopy } from '../i18n'

// ---------------------------------------------------------------------------
// 在线演示页（REQ-238）：内置种子本体（后端 internal/seed 启动播种同一批）构建产物的
// 可视化——三维图谱（Graph3D）/二维图谱（SpecGraph）/层级浏览（HierarchyPane，REQ-363
// 资产详情同款组件）均为纯前端渲染、零后端依赖，组件直接复用 web/src。
// REQ-365：演示与实际产品对齐——①新增「层级浏览」页签（类层级树/实例树/属性分组树/
// 实体描述面板/全局搜索/usage 反向引用，与平台资产详情同款交互；选择为演示页本地状态，
// 跨视图选择总线在平台内由 REQ-253① 承载）；②新增「spec 数据」页签（只读结构化查看，
// 平台内为可编辑资产、保存走版本门禁——静态站无后端不做编辑，诚实标注）；③新增工作台
// 预览区（复用 REQ-364 mockups 两张真实界面结构示意）+ 指路：对话建本体/智能体使用本体
// 等完整动线需部署后体验（不做假交互聊天框，与平台诚实文化一致）。
// REQ-360：页面 chrome 双语；种子 spec 数据（名称/描述/图内标签）与复用平台组件的
// 内置文案维持原样（内容与平台 i18n 范畴，不在本批）。
// ---------------------------------------------------------------------------

const COPY = {
  zh: {
    title: '在线演示 · 内置示例本体',
    taglinePre: '以下为平台内置种子本体的',
    taglineStrong: '可交互演示',
    taglineMid: '（与后端启动播种同源数据）——图谱可视化与层级浏览复用平台资产页同款组件，可拖拽、缩放、搜索、点选节点查看定义与引用关系。',
    alertTitle: '演示范围：静态数据上的真实组件',
    alertDesc: '此页所有交互均为平台同款前端组件加载内置种子数据，无需部署即可体验；对话建本体、智能体挂载本体查询、六条构建路径、运行方案、消费审计等完整动线依赖 LLM 与后端服务，需本地或服务器部署平台后体验（见「快速开始」，界面形态见下方预览图）。',
    concepts: '概念',
    relations: '关系',
    instances: '实例',
    tab3d: '三维浏览',
    tab2d: '二维图谱',
    tabHier: '层级浏览',
    tabSpec: 'spec 数据（只读）',
    specHint: 'spec_json 数据模型全文（只读）——平台内这是可编辑的版本化资产，保存经过校验门禁与版本快照。',
    moreTitle: '部署后的完整界面',
    moreDesc: '以下为真实界面结构示意（按平台布局绘制）：对话工作台承载智能体编排、工具调用过程与事件轨迹；本体工程控制台承载六条构建路径、质量门禁与运行消费。部署后即可得到同款界面。',
    moreCta: '查看快速开始',
  },
  en: {
    title: 'Online Demo · Built-in Example Ontologies',
    taglinePre: 'Below is an ',
    taglineStrong: 'interactive demo',
    taglineMid: ' of the platform’s built-in seed ontologies (same data the backend seeds on startup) — the graph views and hierarchy browser reuse the platform’s own asset-page components. Drag, zoom, search, and click nodes to see definitions and references.',
    alertTitle: 'Demo scope: real components on static data',
    alertDesc: 'Every interaction on this page is a real platform front-end component loading built-in seed data — no deployment needed. Full workflows — conversational ontology building, agents querying ontologies, the six build paths, runtime plans, consumption & audit — require an LLM connection and the backend services (see "Quick start"; interface shapes shown in the preview below).',
    concepts: 'Concepts',
    relations: 'Relations',
    instances: 'Instances',
    tab3d: '3D view',
    tab2d: '2D graph',
    tabHier: 'Hierarchy browser',
    tabSpec: 'spec data (read-only)',
    specHint: 'The full spec_json data model (read-only) — inside the platform this is an editable, versioned asset whose saves pass a validation gate and version snapshots.',
    moreTitle: 'The full interface after deployment',
    moreDesc: 'Structural mockups drawn from the real UI: the chat workbench hosts agent orchestration, tool-call timelines and event traces; the ontology console hosts the six build paths, quality gates and runtime consumption. Deploy the platform and you get the same thing.',
    moreCta: 'View quick start',
  },
}

export default function DemoPage() {
  const c = useCopy(COPY)
  const [key, setKey] = useState(SEEDS[0].key)
  const [ontoSel, setOntoSel] = useState<OntoSelection | null>(null)
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
              {
                key: 'hier',
                label: c.tabHier,
                children: (
                  <HierarchyPane
                    ontology={{ id: seed.key, name: seed.spec.name, description: seed.spec.description, version: 1 }}
                    spec={seed.spec}
                    selection={ontoSel}
                    onSelect={setOntoSel}
                  />
                ),
              },
              {
                key: 'spec',
                label: c.tabSpec,
                children: (
                  <div className="demo-spec-wrap">
                    <Typography.Paragraph type="secondary" className="demo-spec-hint">
                      {c.specHint}
                    </Typography.Paragraph>
                    <pre className="demo-spec-json">{JSON.stringify(seed.spec, null, 2)}</pre>
                  </div>
                ),
              },
            ]}
          />
        </Card>

        <section className="demo-more">
          <h2>{c.moreTitle}</h2>
          <p className="demo-more-desc">{c.moreDesc}</p>
          <div className="preview-grid">
            <ChatWorkbenchMock />
            <OntologyConsoleMock />
          </div>
          <p style={{ marginTop: 14 }}>
            <a className="ant-btn ant-btn-primary" href="#/deploy">{c.moreCta}</a>
          </p>
        </section>
      </div>
    </main>
  )
}
