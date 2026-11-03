import { Button, Tag, Typography } from 'antd'
import {
  ApartmentOutlined,
  CloudDownloadOutlined,
  CommentOutlined,
  DatabaseOutlined,
  InboxOutlined,
  PartitionOutlined,
  RightOutlined,
  TableOutlined,
} from '@ant-design/icons'
import ProductionGuide from './ProductionGuide'
import { PATHS, STATE_TAG } from './paths'
import type { BuildPath } from './paths'
import { readJobs } from './jobRegistry'
import { deepLinkOnto } from '../shared/ontoRoute'

// ---------------------------------------------------------------------------
// REQ-598/M409 构建入口 Hub（111 号 §3.1/§3.3）：路径选择、路径宿主、跨栏跳板三种
// 角色分离后的轻量 Hub——只做三件事：按「手里有什么」选起点、看进行中任务、看生产
// 动线引导；不再是路径宿主（各路径升独占页 #/onto/build/{path}）。
// 页面节奏：页头 → A 进行中（有内容才显示，非空壳）→ B 按手里有什么选起点（构建路径
// 分类学落地）→ C 生产动线（默认折叠）→ D KG 消费注记。视觉静音 C7：灰字组标题+
// 行式条目，同屏点击目标 ≈13 且语义单一（全部=进某条路径或某栏）。
// ---------------------------------------------------------------------------

const PATH_ICON: Record<string, React.ReactNode> = {
  custom: <ApartmentOutlined />,
  ontochat: <CommentOutlined />,
  kb: <DatabaseOutlined />,
  parking: <InboxOutlined />,
  structured: <TableOutlined />,
  ontoextend: <PartitionOutlined />,
  catalog: <CloudDownloadOutlined />,
  datasource: <TableOutlined />,
}

type HubEntry = {
  key: string
  go: () => void
  label: string
  desc: string
  state: { color: string; text: string }
}

function EntryRow({ e, testid }: { e: HubEntry; testid: string }) {
  return (
    <div
      onClick={e.go}
      data-testid={testid}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '7px 10px',
        borderRadius: 8,
        cursor: 'pointer',
      }}
      className="hub-entry-row"
    >
      <span style={{ color: 'var(--c-ink-2, #5a6478)', fontSize: 15 }}>{PATH_ICON[e.key] ?? <ApartmentOutlined />}</span>
      <span style={{ fontSize: 13.5, fontWeight: 600, flex: 'none' }}>{e.label}</span>
      <Tag color={e.state.color} style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 6px', flex: 'none' }}>
        {e.state.text}
      </Tag>
      <Typography.Text type="secondary" style={{ fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
        {e.desc}
      </Typography.Text>
      <Button
        size="small"
        type="text"
        onClick={(ev) => {
          ev.stopPropagation()
          e.go()
        }}
        data-testid={`${testid}-go`}
      >
        去 <RightOutlined style={{ fontSize: 10 }} />
      </Button>
    </div>
  )
}

export default function BuildHub({
  onGo,
  onMaterialStart,
}: {
  onGo: (key: Exclude<BuildPath, 'home'>, params?: Record<string, string>) => void
  onMaterialStart: () => void
}) {
  // 区块 A：异步 job 注册表（REQ-600 各流程登记后可见；无内容整块不渲染——非空壳）
  const jobs = readJobs(['custom', 'ontochat', 'kb', 'ontoextend', 'catalog', 'datasource'])

  const entries: Record<string, HubEntry> = {
    ontochat: {
      key: 'ontochat',
      go: () => onGo('ontochat'),
      label: PATHS.find((p) => p.key === 'ontochat')!.label,
      desc: PATHS.find((p) => p.key === 'ontochat')!.desc,
      state: STATE_TAG.ontochat,
    },
    parking: {
      key: 'parking',
      go: onMaterialStart,
      label: '概念停车场',
      desc: '粘贴 / 上传学习材料 → AI 铺候选概念 → 勾选成骨架（P1 现为弹窗动线，页面化随后续批次）',
      state: { color: 'green', text: '可用' },
    },
    datasource: {
      key: 'datasource',
      go: () => onGo('datasource'),
      label: PATHS.find((p) => p.key === 'datasource')!.label,
      desc: 'SQLite 库只读内省：表→概念、列→属性、外键→关系，人审后落库',
      state: STATE_TAG.datasource,
    },
    structured: {
      key: 'structured',
      go: () => onGo('kb'),
      label: 'CSV / JSON 表格映射',
      desc: '表格数据直接映射本体骨架（现挂「由知识库构建」页内「另一种方式」段）',
      state: { color: 'green', text: '可用' },
    },
    kb: {
      key: 'kb',
      go: () => onGo('kb'),
      label: PATHS.find((p) => p.key === 'kb')!.label,
      desc: PATHS.find((p) => p.key === 'kb')!.desc,
      state: STATE_TAG.kb,
    },
    catalog: {
      key: 'catalog',
      go: () => onGo('catalog'),
      label: PATHS.find((p) => p.key === 'catalog')!.label,
      desc: PATHS.find((p) => p.key === 'catalog')!.desc,
      state: STATE_TAG.catalog,
    },
    ontoextend: {
      key: 'ontoextend',
      go: () => onGo('ontoextend'),
      label: PATHS.find((p) => p.key === 'ontoextend')!.label,
      desc: PATHS.find((p) => p.key === 'ontoextend')!.desc,
      state: STATE_TAG.ontoextend,
    },
    custom: {
      key: 'custom',
      go: () => onGo('custom'),
      label: PATHS.find((p) => p.key === 'custom')!.label,
      desc: PATHS.find((p) => p.key === 'custom')!.desc,
      state: STATE_TAG.custom,
    },
  }

  const groups: { title: string; entries: HubEntry[] }[] = [
    { title: '有一段描述', entries: [entries.ontochat] },
    { title: '有学习材料', entries: [entries.parking] },
    { title: '有结构化数据', entries: [entries.datasource, entries.structured] },
    { title: '有现成语料或标准', entries: [entries.kb, entries.catalog] },
    { title: '已有本体要改', entries: [entries.ontoextend] },
    { title: '从零手动起步', entries: [entries.custom] },
  ]

  return (
    <div className="work-main" data-testid="build-hub">
      {/* 页头：标题+一句话定位；右端无主操作钮 */}
      <div style={{ marginBottom: 6 }}>
        <Typography.Title level={5} style={{ margin: 0 }}>
          构建本体
        </Typography.Title>
        <Typography.Text type="secondary" style={{ fontSize: 12.5 }}>
          从你手里的材料出发，殊途同归收敛到同一套校验与资产
        </Typography.Text>
      </div>

      {/* A · 进行中（有内容才显示）：活跃异步任务续接入口；行式条目+「继续」深链 */}
      {jobs.length > 0 && (
        <div style={{ margin: '10px 0 4px', padding: '8px 10px', border: '1px solid var(--c-border, #e6e8f0)', borderRadius: 10 }} data-testid="hub-jobs">
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
            进行中
          </Typography.Text>
          {jobs.map((j, i) => (
            <div key={`${j.kind}-${j.path}-${i}`} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '3px 0' }}>
              <span style={{ fontSize: 13.5 }}>{j.label}</span>
              <Typography.Text type="secondary" style={{ fontSize: 12, flex: 1 }}>
                任务进行中——进度以路径页内轮询为准
              </Typography.Text>
              <Button size="small" onClick={() => onGo(j.path as Exclude<BuildPath, 'home'>)} data-testid={`hub-job-resume-${j.path}`}>
                继续
              </Button>
            </div>
          ))}
        </div>
      )}

      {/* B · 按手里有什么选起点（hub 一级组织=构建路径分类学本身；新手最常见「一段描述」置顶） */}
      <div style={{ marginTop: 8 }}>
        {groups.map((g, gi) => (
          <div key={g.title} style={{ marginTop: gi === 0 ? 4 : 10 }}>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', padding: '0 10px 2px' }}>
              {g.title}
            </Typography.Text>
            {g.entries.map((e) => (
              <EntryRow key={e.key} e={e} testid={`hub-entry-${e.key}`} />
            ))}
            {gi === 0 && (
              <div style={{ padding: '0 10px', marginTop: 2 }}>
                {/* 空白起步唯一入口在此（消解同屏两处）；零模型连接场景空白手动是唯一原生构建路径 */}
                <Button
                  size="small"
                  type="link"
                  style={{ padding: '2px 0', fontSize: 12 }}
                  onClick={() => onGo('custom', { source: 'blank' })}
                  data-testid="hub-blank-go"
                >
                  无模型连接？从空白手动起步
                </Button>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* C · 生产动线（默认折叠；双模式叙事保留——生产交付看动线，学习构建看上方起点分组） */}
      <div style={{ marginTop: 12 }}>
        <ProductionGuide onGoBuild={() => onGo('custom')} />
      </div>

      {/* D · KG 消费注记（替代整张 kg 卡/页签——kg 路径退役为重定向） */}
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 10, marginBottom: 0 }} data-testid="hub-kg-note">
        要看 KG 图谱抽取与消费（GraphRAG 试查 / 决策溯源）？<a onClick={() => deepLinkOnto({ col: 'audit' })}>前往消费与审计栏</a>
      </Typography.Paragraph>
    </div>
  )
}
