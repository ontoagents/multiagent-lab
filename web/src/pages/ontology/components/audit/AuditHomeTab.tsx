import { Card, Collapse, Space, Steps, Tag, Typography } from 'antd'

// ---------------------------------------------------------------------------
// 消费与审计 · 学习引导页签（三段式：功能 / 原理 / 使用说明）。
// REQ-290/M94 内容置换：叙事自「KB 语料→抽取 KG→GraphRAG 检索」改写为本体消费
// 与审计口径（消费链路四步 + 决策留痕/PROV-O）；顺修原③「重建 KG」步骤
// （M36/KB-13 已退役该按钮，治理在知识库模块）。
// ---------------------------------------------------------------------------

/** ① 功能：它是什么 / 解决什么 / 不做什么（REQ-290 口径） */
const FEATURES: { key: string; title: string; tag: string; color: string; body: string }[] = [
  {
    key: 'what',
    title: '它是什么',
    tag: '定位',
    color: 'blue',
    body: '本体叙事「构建 → 运行 → 消费 → 审计」中的消费与审计环节：本体经 TTL 装载进运行平面后，被 SPARQL 查询、智能体 onto_* 工具、运行态实渲、CQ 验收等消费；每次构建/治理决策落 onto_decision 留痕，可沿溯源链回溯并导出 PROV-O。',
  },
  {
    key: 'why',
    title: '解决什么',
    tag: '痛点',
    color: 'geekblue',
    body: '「本体被谁消费、消费成什么样、决策为何这样定」不可见：方案装载了哪些本体、质量快照如何、消费面入口在哪、治理动作依据什么，往往散落在各栏与日志里。本栏把消费面收拢为观测台，把决策固化为可查询的溯源链。',
  },
  {
    key: 'not',
    title: '不做什么',
    tag: '边界',
    color: 'default',
    body: '不做知识库文本抽取 KG 的展示与治理（归知识库模块 GraphRAG，REQ-290 内容置换）；不替代 SPARQL 工作台精确查询（本栏快查是其轻量封装）；审计只留痕不阻断任何治理动作。',
  },
]

/** ② 原理：本体消费链路四步 */
const CONSUME_STEPS = [
  { title: '构建', description: 'spec_json 校验 → TTL 导出（构建平面质量门禁）' },
  { title: '装载', description: '运行方案启动装载 TTL，异步快评写质量快照与发布状态快照' },
  { title: '消费', description: 'SPARQL 查询 / onto_* 智能体工具 / 运行态实渲 / CQ 验收' },
  { title: '审计', description: '治理与构建决策落 onto_decision → 溯源链 → PROV-O 导出' },
]

/** ② 原理：审计链路 */
const AUDIT_STEPS = [
  { title: '决策留痕', description: '本体治理动作（保存/导入/fork/删除）与 KG 抽取自动落 onto_decision' },
  { title: '溯源链', description: '决策沿 derived_from 逐级回溯（32 跳封顶 + 环防御）' },
  { title: 'PROV-O 导出', description: '按 W3C PROV-O 语义生成 Turtle，供外部工具检查' },
]

/** ③ 使用说明：四步演练 */
const DRILL_STEPS = [
  { title: '启动方案', description: '到「本体运行」栏启动（或创建）运行方案——装载版本与质量快照自动出现在「消费观测」页。' },
  { title: '观测消费', description: '「消费观测」页看运行方案消费一览（装载版本/质量分）；「关键词快查」对 TTL 内容做字面量试查。' },
  { title: '进入消费面', description: '经「消费面导航」前往：运行态实渲（资产·可视化）/ SPARQL 工作台（运行）/ AI 消费（资产·被引用）/ CQ→SPARQL 验收（资产·质量卡）。' },
  { title: '审计与导出', description: '「决策审计」页看留痕与溯源链（默认本体类别，清空筛选可看 kg/kb 全部），导出 PROV-O Turtle 供外部工具检查。' },
]

export default function AuditHomeTab() {
  // REQ-490：说明页退役为默认收起的折叠节（三张带框说明卡不再常驻开场）
  return (
    <Collapse
      size="small"
      ghost
      items={[
        {
          key: 'guide',
          label: '功能与使用说明（点开查看——它是什么 / 原理 / 怎么用）',
          children: (
    <div className="sema-home">
      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">①</span>
            <span>功能 · 它是什么、解决什么、不做什么</span>
          </Space>
        }
      >
        <div className="sema-grid-3">
          {FEATURES.map((f) => (
            <div className="sema-feature" key={f.key}>
              <div className="sema-feature-head">
                <span className="sema-feature-title">{f.title}</span>
                <Tag color={f.color} style={{ margin: 0 }}>
                  {f.tag}
                </Tag>
              </div>
              <p className="sema-feature-body">{f.body}</p>
            </div>
          ))}
        </div>
      </Card>

      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">②</span>
            <span>原理 · 本体消费链路与审计链路</span>
          </Space>
        }
      >
        <div className="onto-sec" style={{ marginTop: 0 }}>
          <span className="onto-sec-title">本体消费链路（构建 → 装载 → 消费 → 审计）</span>
        </div>
        <Steps size="small" orientation="horizontal" titlePlacement="vertical" responsive={false} items={CONSUME_STEPS} />

        <div className="onto-sec">
          <span className="onto-sec-title">审计链路（决策留痕 → 溯源链 → PROV-O）</span>
        </div>
        <Steps size="small" orientation="horizontal" titlePlacement="vertical" responsive={false} items={AUDIT_STEPS} />

        <div className="onto-sec">
          <span className="onto-sec-title">PROV-O 溯源机制</span>
        </div>
        <p className="sema-p">
          决策按 W3C PROV-O 语义留痕：<Typography.Text code>Activity</Typography.Text>（一次构建/治理/手工决策）、
          <Typography.Text code>Entity</Typography.Text>（作用主体 本体/库）、
          <Typography.Text code>wasDerivedFrom</Typography.Text>（前置决策）。「导出 PROV-O」生成 Turtle 供外部工具检查。
        </p>
        <p className="sema-p sema-p-muted">
          边界（REQ-290 内容置换）：知识库文本抽取 KG（chunk → 实体/关系/claim）的展示与治理在知识库模块 GraphRAG
          页「图谱与统计/抽取治理/全局问答」；本栏只观测本体 TTL 装载来源的消费面。
        </p>
      </Card>

      <Card
        size="small"
        className="work-card sema-card"
        title={
          <Space size={8}>
            <span className="sema-card-no">③</span>
            <span>使用说明 · 四步演练</span>
          </Space>
        }
      >
        <Steps size="small" orientation="vertical" items={DRILL_STEPS} />
      </Card>
    </div>
        ),
      },
      ]}
    />
  )
}
