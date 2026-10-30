import { api } from '../api/client'
import { SIDEBAR_WIDTH, sidebarDefaultSize, sidebarRemember } from '../lib/layout'
import { useEffect, useMemo, useState, type MouseEvent, type ReactNode } from 'react'
import { Alert, Button, Card, Menu, Select, Space, Spin, Splitter, Tag, Tooltip, Typography, message } from 'antd'
import {
  ApartmentOutlined,
  FileTextOutlined,
  CompassOutlined,
  DatabaseOutlined,
  DownOutlined,
  LinkOutlined,
  ProjectOutlined,
  RobotOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  UpOutlined,
} from '@ant-design/icons'
import XMarkdown from '@ant-design/x-markdown'
import DocViewerModal from '../components/DocViewerModal'
import { docFileOf, resolveRef } from '../lib/docref'
import HintIcon from './settings/HintIcon'

// 平台知识内容（REQ-116 / REQ-161 v2）：构建期内联扫描 platform-knowledge/ 全目录。
// REQ-188 起全链目录驱动：目录即页面树——顶层目录=L1 组、子目录=L2、文档=主题页；
// 组序/组内序均由名称数字前缀（NN_）决定，新增目录/文档落入即上页并自动入位，无需改本文件；
// 未登记的新顶层目录兜底成组置尾（不丢档）。个别跨目录挂载见 TOPIC_MOUNT。
const KB_RAW = import.meta.glob('../../../platform-knowledge/**/*.md', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

// REQ-184：docs/ 十编号文档挂载为 L1「需求与进度」组（需求/方案/治理/冒烟事实源直达）
const DOCS_RAW = import.meta.glob('../../../docs/*.md', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

// 「外部资源」主题页（REQ-109/162）：内容单源仍在 seeds/learning/external-resources.md，构建期内联挂载
import EXTERNAL_RESOURCES_MD from '../../../seeds/learning/external-resources.md?raw'

/** 图标表（可选定制，键 = 去前缀目录名；未登记目录用默认图标，补一行即生效） */
/** REQ-184：docs 组显示名（目录驱动保持，仅展示层覆盖） */
const GROUP_LABELS: Record<string, string> = { docs: '需求与进度' }
/** REQ-184：docs 组图标 */
const GROUP_ICONS_EXT: Record<string, ReactNode> = { docs: <FileTextOutlined /> }
/** REQ-184：docs 文件友好标题（编号文档名 → 页面标题） */
const DOCS_TITLES: Record<string, string> = {
  '01_智能体_需求文档_PRD': '智能体需求文档（PRD）',
  '02_智能体_技术方案设计': '智能体技术方案设计',
  '03_本体_需求文档': '本体需求文档',
  '04_本体_方案设计': '本体方案设计',
  '11_知识库_需求文档': '知识库需求文档',
  '12_知识库_方案设计': '知识库方案设计',
  '14_本体_前端改造方案': '本体前端改造方案',
  '15_开源项目及论文登记簿': '开源项目及论文登记簿',
  '18_REQ编号注册表': 'REQ 编号注册表',
  '20_回归冒烟清单': '回归冒烟清单',
}

const GROUP_ICONS: Record<string, ReactNode> = {
  整体设计: <CompassOutlined />,
  智能体: <RobotOutlined />,
  项目: <ProjectOutlined />,
  本体: <ApartmentOutlined />,
  知识库: <DatabaseOutlined />,
  技能: <ThunderboltOutlined />,
  设置: <SettingOutlined />,
}
const DEFAULT_ICON = <LinkOutlined />

/** 目录/文件名前缀解析：`NN_名称` → { order, label }；无前缀 order=Infinity（排序置尾） */
function parsePrefix(name: string): { order: number; label: string } {
  const m = /^(\d+)_(.+)$/.exec(name)
  return m ? { order: Number(m[1]), label: m[2] } : { order: Number.POSITIVE_INFINITY, label: name }
}

/** 跨目录挂载（REQ-169 二轮，开发者指定）：键=主题 key（存放目录/文件），value=挂载的 L1 组 + 展示名。
 *  仅改页面归属与标题；互引解析基准 base 仍按真实存放目录，保证文内相对链接不失效 */
const TOPIC_MOUNT: Record<string, { group: string; label: string }> = {
  '外部资源/外部资源导航': { group: '本体', label: '本体学习外部资源导航' },
}

interface Topic {
  key: string
  /** 文件路径（目录下相对，保留编号前缀，供排序） */
  file: string
  /** 展示标题：去 `NN_`/`NN-` 前缀，`_`转间隔点；模块主页归一「模块导读」 */
  title: string
  md: string
  /** 主题页所在仓库目录——正文相对引用的解析基准（REQ-161 补充：文档互引用相对路径） */
  base: string
  group: string
  groupLabel: string
  icon: ReactNode
  isHome: boolean
  /** 组排序值（原始顶层目录的数字前缀；挂载主题随目标组） */
  groupOrder: number
}

/** 文件名 → 展示标题（通用规则，新文档落目录即自动获得可读标题） */
function topicTitle(file: string, groupLabel: string): string {
  const baseName = file.slice(file.lastIndexOf('/') + 1)
  const m = /^\d+[_-](.+)$/.exec(baseName)
  let base = m ? m[1] : baseName
  if (base === groupLabel) return base // 组主页（如 00_整体设计）
  if (base === `${groupLabel}模块`) return '模块导读' // 模块导读（如 00_本体模块）
  return base.replace(/_/g, '·')
}

const TOPICS: Topic[] = (() => {
  const out: Topic[] = []
  for (const [path, raw] of Object.entries({ ...KB_RAW, ...DOCS_RAW })) {
    const marker = 'platform-knowledge/'
    const docsMarker = '/docs/'
    const di = path.indexOf(docsMarker)
    const idx = path.indexOf(marker)
    if (idx < 0 && di < 0) continue
    const rel = idx >= 0
      ? path.slice(idx + marker.length)
      : 'docs/' + path.slice(di + docsMarker.length)
    const slash = rel.indexOf('/')
    if (slash < 0) continue // 根级 README.md 等不进页面
    const dir = rel.slice(0, slash)
    const file = rel.slice(slash + 1).replace(/\.md$/, '')
    const { label } = parsePrefix(dir)
    const key = `${dir}/${file}`
    // 跨目录挂载（TOPIC_MOUNT）：归属组/展示名/图标随目标组，base 保持真实存放目录
    const mount = TOPIC_MOUNT[key]
    const effGroup = mount?.group ?? label
    const docTitle = dir === 'docs' ? DOCS_TITLES[file] : undefined
    out.push({
      key,
      file,
      title: mount?.label ?? docTitle ?? topicTitle(file, label),
      md: raw,
      base: `platform-knowledge/${dir}/${file}`.split('/').slice(0, -1).join('/'),
      group: effGroup,
      groupLabel: GROUP_LABELS[mount?.group ?? label] ?? mount?.group ?? label,
      icon: GROUP_ICONS[mount?.group ?? label] ?? GROUP_ICONS_EXT[mount?.group ?? label] ?? DEFAULT_ICON,
      isHome: /^00[_-]/.test(file) || file === `${label}模块`,
      groupOrder: parsePrefix(dir).order,
    })
  }
  // 外部资源主题页（seeds 单源）按 TOPIC_MOUNT 挂载到目标组
  const extMount = TOPIC_MOUNT['外部资源/外部资源导航']
  out.push({
    key: '外部资源/外部资源导航',
    file: '外部资源导航',
    title: extMount?.label ?? '外部资源导航',
    md: EXTERNAL_RESOURCES_MD,
    base: 'seeds/learning',
    group: extMount?.group ?? '外部资源',
    groupLabel: extMount?.group ?? '外部资源',
    icon: GROUP_ICONS[extMount?.group ?? ''] ?? DEFAULT_ICON,
    isHome: false,
    groupOrder: Number.POSITIVE_INFINITY,
  })
  return out
})()

interface Frontmatter {
  module?: string
  topic?: string
  desc?: string
  req?: string[]
  docs?: string[]
  decisions?: string[]
  synced?: string
}

/** 组内排序：组主页置顶（00_ 前缀/模块导读）→ 有数字前缀者按编号升序（兼容 NN_/NN-）→ 无前缀者按标题（zh）殿后 */
function compareTopics(a: Topic, b: Topic): number {
  if (a.isHome !== b.isHome) return a.isHome ? -1 : 1
  const aBase = a.file.slice(a.file.lastIndexOf('/') + 1)
  const bBase = b.file.slice(b.file.lastIndexOf('/') + 1)
  const na = /^(\d+)[_-]/.exec(aBase)?.[1]
  const nb = /^(\d+)[_-]/.exec(bBase)?.[1]
  const va = na ? Number(na) : Number.POSITIVE_INFINITY
  const vb = nb ? Number(nb) : Number.POSITIVE_INFINITY
  if (va !== vb) return va - vb
  return a.title.localeCompare(b.title, 'zh-Hans-CN')
}

/** 解析文章头部 `---` frontmatter（轻量 key: [a, b] 格式，无需引入 YAML 依赖） */
function parseFrontmatter(raw: string): { meta: Frontmatter; body: string } {
  const m = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?/)
  if (!m) return { meta: {}, body: raw }
  const meta: Frontmatter = {}
  for (const line of m[1].split('\n')) {
    const kv = line.match(/^(\w+):\s*(.*)$/)
    if (!kv) continue
    const [, key, value] = kv
    if (value.startsWith('[')) {
      const items = value
        .slice(1, value.lastIndexOf(']'))
        .split(',')
        .map((s) => s.trim().replace(/^["']|["']$/g, ''))
        .filter(Boolean)
      ;(meta as Record<string, unknown>)[key] = items
    } else {
      ;(meta as Record<string, unknown>)[key] = value.trim()
    }
  }
  return { meta, body: raw.slice(m[0].length) }
}

/** 源文档地图（frontmatter → 文末导读卡）：权威事实在 docs/ 与 platform-knowledge/，此处只做指路（research/ 已退役并入 platform-knowledge，REQ-184 收尾 2026-10-22） */
function SourceMap({ meta, onOpenDoc }: { meta: Frontmatter; onOpenDoc: (path: string) => void }) {
  const rows: { label: string; items: string[] }[] = [
    { label: '需求编号', items: meta.req ?? [] },
    { label: '文档章节', items: meta.docs ?? [] },
    { label: '关联决策', items: meta.decisions ?? [] },
  ].filter((r) => r.items.length > 0 && !(r.items.length === 1 && r.items[0] === '—'))
  if (rows.length === 0) return null
  return (
    <Card size="small" className="ref-sourcemap" title="深入阅读 · 源文档地图">
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
        本页是学习视图；权威事实以下列出处为准（编号见 docs/18 注册表）。
      </Typography.Paragraph>
      {rows.map((r) => (
        <div key={r.label} className="ref-sourcemap-row">
          <span className="ref-sourcemap-label">{r.label}</span>
          <span>
            {r.items.map((it) => {
              const file = docFileOf(it)
              const clickable = r.label !== '需求编号' && file
              return clickable ? (
                <Tag
                  key={it}
                  style={{ marginInlineEnd: 6, cursor: 'pointer', color: 'var(--c-brand)', borderColor: 'var(--c-brand)' }}
                  onClick={() => onOpenDoc(file)}
                >
                  {it} · 点击查看
                </Tag>
              ) : (
                <Tag key={it} style={{ marginInlineEnd: 6 }}>
                  {it}
                </Tag>
              )
            })}
          </span>
        </div>
      ))}
      {meta.synced && (
        <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 0, marginTop: 6 }}>
          最后同步：{meta.synced}（语义级变更须按 AGENTS.md 纪律 7 同步本页）
        </Typography.Paragraph>
      )}
    </Card>
  )
}

/** L1 分组：按顶层目录自动派生（REQ-188）——前缀序 → 未登记/无前缀目录兜底置尾（不丢档） */
const GROUPS = (() => {
  const m = new Map<string, Topic[]>()
  for (const t of TOPICS) {
    const arr = m.get(t.group)
    if (arr) arr.push(t)
    else m.set(t.group, [t])
  }
  return [...m.entries()]
    .map(([g, topics]) => ({
      group: g,
      label: GROUP_LABELS[g] ?? g,
      icon: GROUP_ICONS[g] ?? GROUP_ICONS_EXT[g] ?? DEFAULT_ICON,
      order: Math.min(...topics.map((t) => t.groupOrder)),
      topics: topics.sort(compareTopics),
    }))
    .sort((a, b) => a.order - b.order || a.label.localeCompare(b.label, 'zh-Hans-CN'))
})()

/** 界面优化批（REQ-341）：一级目录 → 内容区顶部水平页签（本体 module-tabs 同款范式）；
 *  页签记忆 eino.ref.tab（缺省/失效回落首组）；二级清单进左侧栏（子目录降为不可折叠分组标题）。
 *  默认选中：当前分组主页（无主页取首篇）。 */
const REF_TAB_KEY = 'eino.ref.tab'
const REF_DOC_KEY = 'eino.ref.doc'
// 初始化序：文档记忆（跨分组有效，topic key 全局唯一）→ 推导分组 → 分组记忆回落首组
const INIT_DOC = (() => {
  const v = localStorage.getItem(REF_DOC_KEY)
  return TOPICS.some((t) => t.key === v) ? (v as string) : ''
})()
const INIT_GROUP_KEY = (() => {
  const fromDoc = TOPICS.find((t) => t.key === INIT_DOC)?.group
  if (fromDoc) return fromDoc
  const v = localStorage.getItem(REF_TAB_KEY)
  return GROUPS.some((g) => g.group === v) ? (v as string) : GROUPS[0]?.group ?? ''
})()
const DEFAULT_ACTIVE = INIT_DOC
  || (GROUPS.find((g) => g.group === INIT_GROUP_KEY) ?? GROUPS[0])?.topics.find((t) => t.isHome)?.key
  || (GROUPS.find((g) => g.group === INIT_GROUP_KEY) ?? GROUPS[0])?.topics[0]?.key
  || ''

import { CollapsedRail, SidebarCollapseButton, useSidebarCollapse } from '../lib/sidebar'
export default function ReferencePage() {
  const rail = useSidebarCollapse('eino.ref.sidebar.collapsed')
  const [groupKey, setGroupKey] = useState(INIT_GROUP_KEY)
  const [active, setActive] = useState(DEFAULT_ACTIVE)
  const [viewDoc, setViewDoc] = useState<string | null>(null) // REQ-169：点击互引相对路径 → 右侧 Drawer 阅读，默认关闭
  const group = GROUPS.find((g) => g.group === groupKey) ?? GROUPS[0]
  // 切分组：落默认文档（主页优先）；active 失效（跨组残留）时回落组默认
  const selectGroup = (key: string) => {
    localStorage.setItem(REF_TAB_KEY, key)
    setGroupKey(key)
    const g = GROUPS.find((x) => x.group === key)
    if (g) activate(g.topics.find((t) => t.isHome)?.key ?? g.topics[0]?.key ?? '')
  }
  // REQ-341：选中文档持久化（刷新/重开回看同一篇）
  const activate = (key: string) => {
    localStorage.setItem(REF_DOC_KEY, key)
    setActive(key)
  }
  // REQ-372 P2 修复轮：图谱面板卡点击直达——节点 id（platform-knowledge/ 前缀、.md 后缀）
  // → 主题 key（TOPICS file 已去 .md 后缀，同去后匹配）
  const openCardByID = (id: string) => {
    const key = id.replace(/^platform-knowledge\//, '').replace(/\.md$/, '')
    if (TOPICS.some((t) => t.key === key)) activate(key)
  }
  const topic = group.topics.find((t) => t.key === active) ?? group.topics.find((t) => t.isHome) ?? group.topics[0]
  const { meta, body } = useMemo(() => parseFrontmatter(topic.md), [topic])
  /** 正文相对引用点击（REQ-169）：拦截指向仓库内 .md 的相对路径 → 右侧 Drawer 阅读；http/锚点走默认 */
  const onBodyClick = (e: MouseEvent) => {
    const a = (e.target as HTMLElement).closest?.('a')
    if (!a) return
    const resolved = resolveRef(a.getAttribute('href') ?? '', topic.base)
    if (!resolved) return
    e.preventDefault()
    setViewDoc(resolved)
  }
  // 左栏二级清单：根文档平铺 + 子目录降为不可折叠分组标题（REQ-341：树深 3→2）
  const menuItems = (() => {
    const roots = group.topics.filter((t) => !t.file.includes('/'))
    const subMap = new Map<string, Topic[]>()
    for (const t of group.topics) {
      if (!t.file.includes('/')) continue
      const sub = t.file.slice(0, t.file.indexOf('/'))
      if (!subMap.has(sub)) subMap.set(sub, [])
      subMap.get(sub)!.push(t)
    }
    const items: any[] = roots.map((t) => ({ key: t.key, label: t.title }))
    const subs = [...subMap.keys()].sort((a, b) => {
      const pa = parsePrefix(a)
      const pb = parsePrefix(b)
      return pa.order - pb.order || pa.label.localeCompare(pb.label, 'zh-Hans-CN')
    })
    for (const sub of subs) {
      items.push({ type: 'group', label: parsePrefix(sub).label, children: subMap.get(sub)!.map((t) => ({ key: t.key, label: t.title })) })
    }
    return items
  })()

  return (
    <>
    <div className="module-shell">
      {/* REQ-341：一级目录 → 内容区顶部水平页签（学习中心等分区收尾）；溢出横向滚动兜底 */}
      <div className="module-tabs ref-tabs" role="tablist" aria-label="平台知识分区导航" data-testid="ref-tabs" style={{ overflowX: 'auto' }}>
        {GROUPS.map((g) => (
          <Tooltip key={g.group} title={`平台知识 · ${g.label}`} placement="bottom" mouseEnterDelay={0.4}>
            <button
              type="button"
              role="tab"
              aria-selected={g.group === groupKey}
              aria-current={g.group === groupKey ? 'page' : undefined}
              className={`module-tab${g.group === groupKey ? ' active' : ''}`}
              onClick={() => selectGroup(g.group)}
            >
              <span className="module-tab-icon">{g.icon}</span>
              <span className="module-tab-label">{g.label}</span>
            </button>
          </Tooltip>
        ))}
      </div>
      <div className="module-body" style={{ flex: 1, minHeight: 0 }}>
      {/* REQ-237（57 号 F4）：左栏宽度并入全站单一约定（eino.sidebar.width/280/220/480 + 写入回填；原独用 eino.ref.width 退役） */}
      <Splitter className="main sidebar-splitter" onResizeEnd={sidebarRemember}>
      <Splitter.Panel
        defaultSize={sidebarDefaultSize()}
        min={SIDEBAR_WIDTH.min}
        max={SIDEBAR_WIDTH.max}
        className="sidebar-panel"
      >
        {rail.collapsed ? (
        <CollapsedRail onExpand={rail.toggle} ariaLabel="平台知识侧栏（已收起）" />
      ) : (
        <aside className="sidebar">
          <div className="side-head">
            <span className="side-title">平台知识 · {group.label}</span>
            <SidebarCollapseButton onClick={rail.toggle} />
          </div>
          <div className="ref-menu">
            {/* REQ-341：二级清单单层化——根文档平铺、子目录降为不可折叠分组标题（Menu group） */}
            <Menu
              mode="inline"
              selectedKeys={[active]}
              onClick={({ key }) => activate(String(key))}
              style={{ background: 'transparent' }}
              items={menuItems}
            />
          </div>
          <div className="settings-note" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <HintIcon text="内容收录目录驱动（platform-knowledge/）；新增目录/文档按数字前缀落入后重建即生效（REQ-188）。" />
            <span>目录驱动收录</span>
          </div>
        </aside>
      )}
      </Splitter.Panel>
      <Splitter.Panel className="content-panel">
        <div className="ref-main">
          {/* REQ-372 P2 修复轮：面板移入内容列（原顶层渲染成 .app-body 并列 flex 子项，
              挤压页签与阅读区至 210px 宽——布局崩坏根因）；仅知识体系组挂载 */}
          {group.label === '知识体系' && <KnowledgeGraphPanel onOpenCard={openCardByID} />}
          <div className="settings-head">
            <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 4 }}>
              <Space>{topic.icon}{topic.groupLabel}<Typography.Text type="secondary">/ {topic.title}</Typography.Text></Space>
            </Typography.Title>
            {meta.desc || meta.topic ? (
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                {meta.desc || meta.topic}
              </Typography.Paragraph>
            ) : (
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                产品定位 → 设计原理 → 相关资料；权威全文见源文档地图与 docs/ 对应文档编号。
              </Typography.Paragraph>
            )}
          </div>
          <div className="ref-body" onClick={onBodyClick}>
            <XMarkdown content={body} openLinksInNewTab />
            <SourceMap meta={meta} onOpenDoc={setViewDoc} />
          </div>
        </div>
      </Splitter.Panel>
      </Splitter>
      </div>
    </div>
    {/* 抽屉必须挂在 Splitter 之外：AntD Splitter 只认 Splitter.Panel 子元素，
        混入其他组件会被吞成一个空白面板（REQ-169「右侧空白栏/链接无反应」的根因） */}
    <DocViewerModal path={viewDoc} open={!!viewDoc} onClose={() => setViewDoc(null)} onNavigate={setViewDoc} />
    </>
  )
}


// ---------------------------------------------------------------------------
// REQ-372 P2：知识体系 L2 图谱层面板——前置图投影（节点=卡三型、边=prereq）+学习路径
// （prereq 闭包拓扑序，依赖先行）+断链警示。卡为事实源单源，KG 为投影（每次刷新全量重建）
// ——语义检索/GraphRAG 归 L3（P3 待验证拍板）。修复轮（2026-10-27）：面板移入内容列、
// 加载/错误/空态三态（REQ-115）、断链随 GET 同源返回（消除 refresh→load 竞态清空）、
// 卡选择器替代手输 id（原 placeholder 示例 id 与后端契约错位必失败）、路径结果与卡列表
// 可点击直达（onOpenCard）、刷新 message 反馈、面板可收起（记忆 eino.kg.panel.collapsed）。
// 阅读面修复（2026-10-30）：面板默认收起为单行工具条（显式展开过=0 才保持展开）——
// 30+ 卡全量列表展开常态占内容列 ~300px，正文只剩 ~460px 视窗无法阅读；展开态正文
// 42vh 封顶内滚（沿全站 max-height 惯例），头部补节点/边计数保收起态概览不丢。
// ---------------------------------------------------------------------------
type KGNode = { id: string; kind: string; domain: string; topic: string; desc?: string }
type KGData = { nodes: KGNode[]; edges: { src: string; dst: string; kind: string }[]; broken?: string[]; ingestError?: string }

const KG_KIND_META: Record<string, { label: string; color: string }> = {
  concept: { label: '概念', color: 'blue' },
  decision: { label: '决策', color: 'purple' },
  practice: { label: '实践', color: 'green' },
}

function KnowledgeGraphPanel({ onOpenCard }: { onOpenCard: (id: string) => void }) {
  const [data, setData] = useState<KGData | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [target, setTarget] = useState<string | undefined>()
  const [path, setPath] = useState<{ target: string; ids: string[] } | null>(null)
  const [pathErr, setPathErr] = useState<string | null>(null)
  // 默认收起：面板是学习工具非正文，全量展开挤压阅读区（2026-10-30）；显式展开（'0'）才保持展开
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('eino.kg.panel.collapsed') !== '0')

  const load = () => {
    setLoading(true)
    setLoadErr(null)
    api.knowledgeGraph()
      .then((r) => setData(r))
      .catch((e) => { setLoadErr(e?.message ?? '网络异常'); setData(null) })
      .finally(() => setLoading(false))
  }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { load() }, [])

  // 断链随 GET 同源返回（后端 ScanBroken 随读随检）——load 不再清空 broken，竞态消除
  const refresh = async () => {
    setRefreshing(true)
    try {
      const r = await api.knowledgeGraphRefresh()
      await load()
      if ((r.broken?.length ?? 0) > 0) message.warning(`已重新摄取，发现 ${r.broken!.length} 条 prereq 断链（见警示）`)
      else message.success(r.nodes > 0 ? `已重新摄取：${r.nodes} 节点 / ${r.edges} 边` : '已重新摄取')
    } catch (e: any) {
      message.error(e?.message ?? '重新摄取失败')
    } finally {
      setRefreshing(false)
    }
  }

  const queryPath = async (id?: string) => {
    const t = (id ?? target ?? '').trim()
    if (!t) return
    setPathErr(null)
    try {
      const r = await api.knowledgeGraphPath(t)
      setPath({ target: t, ids: r.path })
    } catch (e: any) {
      setPath(null)
      setPathErr(e?.message ?? '查询失败')
    }
  }

  const toggleCollapsed = () => {
    const next = !collapsed
    setCollapsed(next)
    localStorage.setItem('eino.kg.panel.collapsed', next ? '1' : '0')
  }

  const nodes = data?.nodes ?? []
  const edges = data?.edges ?? []
  const broken = data?.broken ?? []
  const kindCount = (k: string) => nodes.filter((n) => n.kind === k).length
  const cardName = (id: string) => id.split('/').pop()?.replace(/\.md$/, '') ?? id
  const selectOptions = nodes.map((n) => ({
    value: n.id,
    label: `${KG_KIND_META[n.kind]?.label ?? '卡'} · ${n.topic || cardName(n.id)}`,
  }))

  return (
    <Card size="small" style={{ flex: 'none', margin: '0 0 8px' }} data-testid="knowledge-graph-panel"
      title={<Space size={8}><span>知识图谱 · 前置图投影</span><Tag color="purple" style={{ margin: 0 }}>L2</Tag>
        {data && (
          <Typography.Text type="secondary" style={{ fontSize: 12, fontWeight: 400 }}>
            节点 {nodes.length} · 边 {edges.length}{broken.length > 0 ? ` · 断链 ${broken.length}` : ''}
          </Typography.Text>
        )}
      </Space>}
      extra={<Space size={4}>
        <Button size="small" onClick={refresh} loading={refreshing}>重新摄取</Button>
        <Button size="small" type="text" icon={collapsed ? <DownOutlined /> : <UpOutlined />}
          onClick={toggleCollapsed} aria-label={collapsed ? '展开图谱面板' : '收起图谱面板'} />
      </Space>}>
      {collapsed ? null : loading ? (
        <div style={{ padding: '4px 0' }}><Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>图谱加载中…</Typography.Text></div>
      ) : (
        /* 展开态 42vh 封顶内滚：30+ 卡列表不再把正文挤出视窗（2026-10-30 阅读面修复） */
        <div style={{ maxHeight: '42vh', overflowY: 'auto' }}>
          {loadErr && (
            <Alert type="error" showIcon style={{ marginBottom: 8 }}
              title="图谱数据加载失败"
              description={`${loadErr}——可能原因：后端服务不可达。可点击右上「重新摄取」重试。`} />
          )}
          {data && nodes.length === 0 && !loadErr && (
            <Alert type={data.ingestError ? 'error' : 'info'} showIcon style={{ marginBottom: 8 }}
              title={data.ingestError ? '知识图谱为空且自动摄取失败' : '知识图谱为空（尚无卡节点）'}
              description={data.ingestError ?? '知识体系目录下暂无可摄取的卡。按卡规范落档后点击右上「重新摄取」。'} />
          )}
          {nodes.length > 0 && (
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
              概念 {kindCount('concept')} · 决策 {kindCount('decision')} · 实践 {kindCount('practice')}，边为 prereq 前置——卡为事实源、图谱为投影（每次摄取全量重建）
            </Typography.Text>
          )}
          {broken.length > 0 && (
            <Alert type="warning" showIcon style={{ marginBottom: 8 }}
              title={`prereq 断链 ${broken.length} 条（目标卡不存在——补卡或修链接）`}
              description={broken.slice(0, 5).join('；')} />
          )}
          <Space.Compact style={{ width: '100%', maxWidth: 720 }}>
            <Select
              showSearch
              allowClear
              size="small"
              style={{ minWidth: 320 }}
              placeholder="选择卡查学习路径（可输入标题/文件名过滤）"
              value={target}
              onChange={(v) => { setTarget(v); if (v) queryPath(v) }}
              onClear={() => { setTarget(undefined); setPath(null); setPathErr(null) }}
              filterOption={(input, option) => `${(option?.label ?? '')} ${(option?.value ?? '')}`.toLowerCase().includes(input.toLowerCase())}
              options={selectOptions}
              notFoundContent={loading ? <Spin size="small" /> : '无卡节点'}
            />
            <Button size="small" onClick={() => queryPath()}>查前置路径</Button>
          </Space.Compact>
          {pathErr && <Typography.Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 6 }}>{pathErr}</Typography.Text>}
          {path && path.ids.length > 0 && (
            <div style={{ marginTop: 8 }}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>前置路径（{path.ids.length} 步，依赖先行）——点击任一步打开卡片：</Typography.Text>
              <ol style={{ margin: '4px 0 0', paddingLeft: 20 }}>
                {path.ids.map((id) => (
                  <li key={id} style={{ fontSize: 12, lineHeight: '20px' }}>
                    <a onClick={() => onOpenCard(id)} style={{ fontSize: 12 }}>{cardName(id)}</a>
                    {id === path.target && <Tag color="purple" style={{ marginLeft: 6, fontSize: 11, lineHeight: '16px' }}>目标</Tag>}
                  </li>
                ))}
              </ol>
            </div>
          )}
          {(['concept', 'decision', 'practice'] as const).map((k) => {
            const kn = nodes.filter((n) => n.kind === k)
            if (kn.length === 0) return null
            const meta = KG_KIND_META[k]
            return (
              <div key={k} style={{ marginTop: 6 }}>
                <Tag color={meta.color} style={{ fontSize: 11 }}>{meta.label}卡 {kn.length}</Tag>
                {kn.map((n) => (
                  <Tooltip key={n.id} title={n.desc || n.id}>
                    <a onClick={() => onOpenCard(n.id)} style={{ fontSize: 12, marginRight: 10 }}>{n.topic || cardName(n.id)}</a>
                  </Tooltip>
                ))}
              </div>
            )
          })}
        </div>
      )}
    </Card>
  )
}
