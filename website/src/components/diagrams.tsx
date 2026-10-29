/**
 * 官网图示组件库（REQ-238 扩展）：纯内联 SVG，与品牌视觉同源（#4f46e5 系）。
 * 设计目标：直观易读——分层清晰 / 连线带语义标签 / 配色区分平面与事件类别；
 * 全部为无外部依赖的 React 组件，可直接构建进静态站。
 * REQ-360：图内文字随站点语言切换（useLocale；坐标布局双语言共用）。
 */

import { useLocale } from '../i18n'
import type { L10n } from '../locale'

const INK = '#1c2130'
const INK2 = '#575e75'
const LINE = '#b9bed6'
const BRAND = '#4f46e5'

/** 通用小工具：带箭头 marker 的 defs（每个组件独立 id，避免同页冲突） */
function ArrowDefs({ id, color = '#8a90ad' }: { id: string; color?: string }) {
  return (
    <defs>
      <marker id={id} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M0 0L10 5L0 10z" fill={color} />
      </marker>
    </defs>
  )
}

function Box({
  x, y, w, h, fill, stroke, rx = 10,
}: { x: number; y: number; w: number; h: number; fill: string; stroke: string; rx?: number }) {
  return <rect x={x} y={y} width={w} height={h} rx={rx} fill={fill} stroke={stroke} strokeWidth="1.2" />
}

function Text({
  x, y, children, size = 12, weight = 400, fill = INK, anchor = 'middle',
}: { x: number; y: number; children: React.ReactNode; size?: number; weight?: number; fill?: string; anchor?: 'middle' | 'start' | 'end' }) {
  return (
    <text x={x} y={y} fontSize={size} fontWeight={weight} fill={fill} textAnchor={anchor} dominantBaseline="middle">
      {children}
    </text>
  )
}

/** 带标题的内嵌小卡（架构图内部模块） */
function InnerCard({ x, y, w, h, title, desc, tint }: { x: number; y: number; w: number; h: number; title: string; desc?: string; tint: string }) {
  return (
    <g>
      <Box x={x} y={y} w={w} h={h} fill="#ffffff" stroke={tint} rx={8} />
      <Text x={x + w / 2} y={y + (desc ? h / 2 - 7 : h / 2)} size={12} weight={700}>{title}</Text>
      {desc && <Text x={x + w / 2} y={y + h / 2 + 10} size={10.5} fill={INK2}>{desc}</Text>}
    </g>
  )
}

// ---------------------------------------------------------------------------
// 1. 整体架构：三平面 + 双代理（源：platform-knowledge/01_整体设计/00_整体设计.md §2）
// ---------------------------------------------------------------------------

/** 层间连线（竖直，带标签） */
function VLine({ x, y1, y2, label, labelSide = 'right', dashed = false, color = LINE }: {
  x: number; y1: number; y2: number; label?: string; labelSide?: 'left' | 'right'; dashed?: boolean; color?: string
}) {
  return (
    <g>
      <line x1={x} y1={y1} x2={x} y2={y2} stroke={color} strokeWidth="1.4" strokeDasharray={dashed ? '5 4' : undefined} markerEnd="url(#arrow-arch)" />
      {label && (
        <Text x={labelSide === 'right' ? x + 8 : x - 8} y={(y1 + y2) / 2} size={10.5} fill={INK2} anchor={labelSide === 'right' ? 'start' : 'end'}>
          {label}
        </Text>
      )}
    </g>
  )
}

const ARCH: {
  clientLayer: L10n<string>
  engines: L10n<string>
  browser: L10n<{ t: string; d: string }>
  desktop: L10n<{ t: string; d: string }>
  mcpClient: L10n<{ t: string; d: string }>
  mainTitle: L10n<string>
  mainSub: L10n<string>
  m1: L10n<{ t: string; d: string }>
  m2: L10n<{ t: string; d: string }>
  m3: L10n<{ t: string; d: string }>
  m4: L10n<{ t: string }>
  m5: L10n<{ t: string; d: string }>
  m6: L10n<{ t: string }>
  build: L10n<{ t: string; d1: string; d2: string }>
  runtime: L10n<{ t: string; d1: string; d2: string }>
  asset: L10n<string>
  qdrant: L10n<{ t: string; d: string }>
  sqlite: L10n<{ t: string; d: string }>
  oxigraph: L10n<{ t: string; d: string }>
  fuseki: L10n<{ t: string; d: string }>
  vectorLine: L10n<string>
  llmLine: L10n<string>
} = {
  clientLayer: { zh: '客户端层', en: 'Client layer' },
  engines: { zh: '引擎与存储', en: 'Engines & storage' },
  browser: { zh: { t: '浏览器 Web UI', d: 'React 18 · AntD 6 · hash 路由' }, en: { t: 'Browser Web UI', d: 'React 18 · AntD 6 · hash routing' } },
  desktop: { zh: { t: '桌面客户端（Tauri 2 壳）', d: 'Windows 安装包 / macOS DMG · 系统托盘' }, en: { t: 'Desktop client (Tauri 2 shell)', d: 'Windows installer / macOS DMG · system tray' } },
  mcpClient: { zh: { t: '外部客户端（MCP / A2A）', d: 'MCP：Claude Desktop / Cursor · A2A：其他 Agent 平台' }, en: { t: 'External clients (MCP / A2A)', d: 'MCP: Claude Desktop / Cursor · A2A: other agent platforms' } },
  mainTitle: { zh: '主平台 backend（Go · Hertz · :8080）', en: 'Main platform backend (Go · Hertz · :8080)' },
  mainSub: { zh: '智能体装配与运行 · 知识库 · 设置 · 平台助手', en: 'Agent assembly & running · knowledge base · settings · assistant' },
  m1: { zh: { t: 'Eino ADK 装配与运行', d: '对话 / 对比模式 / 多智能体编排' }, en: { t: 'Eino ADK assembly & run', d: 'Chat / compare mode / multi-agent orchestration' } },
  m2: { zh: { t: 'SSE 事件流', d: '工具 / 子智能体 / 本体查询全留痕' }, en: { t: 'SSE event stream', d: 'Tools / sub-agents / ontology queries fully traced' } },
  m3: { zh: { t: '知识库 RAG / GraphRAG', d: 'Qdrant 向量（主）· SQLite 余弦 fallback' }, en: { t: 'Knowledge base RAG / GraphRAG', d: 'Qdrant vectors (primary) · SQLite cosine fallback' } },
  m4: { zh: { t: '技能 / 工具 / 外部连接器' }, en: { t: 'Skills / tools / external connectors' } },
  m5: { zh: { t: '模型代理（双协议）', d: 'OpenAI 兼容 / Anthropic 兼容' }, en: { t: 'Model proxy (dual protocol)', d: 'OpenAI-compatible / Anthropic-compatible' } },
  m6: { zh: { t: 'Agent 对外 MCP 服务化' }, en: { t: 'Agent MCP serving' } },
  build: {
    zh: { t: '本体构建平面 ontology-service（:8091）', d1: '六条构建路径 · LLM 生成必过质量门禁', d2: 'spec_json 多形态资产仓库 · 导入审查合并 · 工具链' },
    en: { t: 'Ontology build plane — ontology-service (:8091)', d1: 'Six build paths · LLM generations must pass the quality gate', d2: 'Multi-form spec_json asset store · import review/merge · tooling' },
  },
  runtime: {
    zh: { t: '本体运行平面 runtime-manager（:8090）', d1: '运行方案编排（Oxigraph / Fuseki）', d2: '统一 MCP facade 供智能体消费本体' },
    en: { t: 'Ontology runtime plane — runtime-manager (:8090)', d1: 'Run-plan orchestration (Oxigraph / Fuseki)', d2: 'Unified MCP facade for agent consumption' },
  },
  asset: { zh: '资产', en: 'assets' },
  qdrant: { zh: { t: 'Qdrant（向量，主）', d: 'SQLite 余弦 fallback' }, en: { t: 'Qdrant (vectors, primary)', d: 'SQLite cosine fallback' } },
  sqlite: { zh: { t: 'SQLite 业务库', d: '密钥加密存储' }, en: { t: 'SQLite business DB', d: 'keys stored encrypted' } },
  oxigraph: { zh: { t: 'Oxigraph', d: 'SPARQL 型引擎（默认）' }, en: { t: 'Oxigraph', d: 'SPARQL engine (default)' } },
  fuseki: { zh: { t: 'Fuseki', d: 'OWL 推理对照引擎' }, en: { t: 'Fuseki', d: 'OWL reasoning engine' } },
  vectorLine: { zh: '向量检索', en: 'vector retrieval' },
  llmLine: { zh: '外部 LLM 供应商 —— OpenAI 兼容 / Anthropic 兼容（多连接 · 自动发现 · 厂商预设 · 密钥仅存本地加密）', en: 'External LLM vendors — OpenAI-compatible / Anthropic-compatible (multi-connection · auto-discovery · vendor presets · keys stored locally, encrypted only)' },
}

export function ArchDiagram() {
  const { locale } = useLocale()
  const a = ARCH
  return (
    <svg viewBox="0 0 1180 660" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label={locale === 'zh' ? '系统架构：三平面 + 双代理' : 'System architecture: three planes + dual proxy'}>
      <ArrowDefs id="arrow-arch" />

      {/* ===== 客户端层 ===== */}
      <Text x={34} y={52} size={11.5} weight={700} fill={INK2} anchor="start">{a.clientLayer[locale]}</Text>
      <InnerCard x={100} y={24} w={300} h={56} title={a.browser[locale].t} desc={a.browser[locale].d} tint={BRAND} />
      <InnerCard x={440} y={24} w={330} h={56} title={a.desktop[locale].t} desc={a.desktop[locale].d} tint={BRAND} />
      <InnerCard x={810} y={24} w={310} h={56} title={a.mcpClient[locale].t} desc={a.mcpClient[locale].d} tint={BRAND} />

      {/* 客户端 → 主平台 */}
      <VLine x={250} y1={80} y2={128} label="HTTP · REST + SSE" />
      <VLine x={605} y1={80} y2={128} label="127.0.0.1:8080（壳托管服务）" labelSide="left" />
      <VLine x={965} y1={80} y2={128} label="MCP /mcp · A2A /api/a2a/agents" />

      {/* ===== 主平台 ===== */}
      <Box x={60} y={130} w={1060} h={172} fill="#eef4ff" stroke="#a8bdf0" />
      <Text x={84} y={156} size={13.5} weight={800} anchor="start">{a.mainTitle[locale]}</Text>
      <Text x={84} y={174} size={10.5} fill={INK2} anchor="start">{a.mainSub[locale]}</Text>
      <InnerCard x={84} y={188} w={310} h={52} title={a.m1[locale].t} desc={a.m1[locale].d} tint={BRAND} />
      <InnerCard x={414} y={188} w={310} h={52} title={a.m2[locale].t} desc={a.m2[locale].d} tint={BRAND} />
      <InnerCard x={744} y={188} w={352} h={52} title={a.m3[locale].t} desc={a.m3[locale].d} tint={BRAND} />
      <InnerCard x={84} y={246} w={310} h={44} title={a.m4[locale].t} tint={BRAND} />
      <InnerCard x={414} y={246} w={310} h={44} title={a.m5[locale].t} desc={a.m5[locale].d} tint={BRAND} />
      <InnerCard x={744} y={246} w={352} h={44} title={a.m6[locale].t} tint={BRAND} />

      {/* 主平台 → 本体两平面 */}
      <VLine x={310} y1={302} y2={360} label="反向代理 · 同源嵌入" labelSide="left" />
      <VLine x={870} y1={302} y2={360} label="反向代理 + MCP Client" labelSide="right" />

      {/* ===== 本体两平面 ===== */}
      <Box x={60} y={362} w={520} h={104} fill="#eef2ff" stroke="#b6bcf2" />
      <Text x={84} y={388} size={13.5} weight={800} anchor="start">{a.build[locale].t}</Text>
      <Text x={84} y={410} size={11} fill={INK2} anchor="start">{a.build[locale].d1}</Text>
      <Text x={84} y={430} size={11} fill={INK2} anchor="start">{a.build[locale].d2}</Text>

      <Box x={600} y={362} w={520} h={104} fill="#ecfdf5" stroke="#9fd8bd" />
      <Text x={624} y={388} size={13.5} weight={800} anchor="start">{a.runtime[locale].t}</Text>
      <Text x={624} y={410} size={11} fill={INK2} anchor="start">{a.runtime[locale].d1}</Text>
      <Text x={624} y={430} size={11} fill={INK2} anchor="start">{a.runtime[locale].d2}</Text>

      {/* 构建 → 运行（资产交付） */}
      <line x1={580} y1={414} x2={600} y2={414} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-arch)" />
      <Text x={590} y={400} size={10} fill={INK2}>{a.asset[locale]}</Text>

      {/* 平面 → 引擎/存储 */}
      <VLine x={860} y1={466} y2={520} label="SPARQL / 推理" labelSide="right" />

      {/* ===== 引擎与存储层 ===== */}
      <Text x={34} y={546} size={11.5} weight={700} fill={INK2} anchor="start">{a.engines[locale]}</Text>
      <InnerCard x={100} y={522} w={210} h={48} title={a.qdrant[locale].t} desc={a.qdrant[locale].d} tint="#9db8e8" />
      <InnerCard x={340} y={522} w={240} h={48} title={a.sqlite[locale].t} desc={a.sqlite[locale].d} tint="#9db8e8" />
      <InnerCard x={700} y={522} w={190} h={48} title={a.oxigraph[locale].t} desc={a.oxigraph[locale].d} tint="#7fc8a4" />
      <InnerCard x={920} y={522} w={180} h={48} title={a.fuseki[locale].t} desc={a.fuseki[locale].d} tint="#7fc8a4" />

      {/* backend → Qdrant/SQLite（向量检索链） */}
      <VLine x={200} y1={290} y2={520} dashed label={a.vectorLine[locale]} labelSide="right" />
      {/* backend → LLM */}
      <VLine x={1150} y1={290} y2={590} dashed color="#c9b2e8" />

      {/* ===== 外部 LLM 供应商 ===== */}
      <Box x={60} y={594} w={1060} h={44} fill="#faf8ff" stroke="#d9c8f0" rx={10} />
      <Text x={590} y={616} size={12} weight={700} fill={INK2}>{a.llmLine[locale]}</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 2. SSE 事件流时序（源：docs/02 §8 / 00_整体设计.md §5）
// ---------------------------------------------------------------------------

const LANE_Y = { client: 88, backend: 168, ext: 248 } as const

function Lane({ y, title, sub, color }: { y: number; title: string; sub: string; color: string }) {
  return (
    <g>
      <Box x={20} y={y - 18} w={128} h={40} fill={color} stroke={LINE} rx={8} />
      <Text x={84} y={y - 5} size={12} weight={700}>{title}</Text>
      <Text x={84} y={y + 11} size={9.5} fill={INK2}>{sub}</Text>
      <line x1={160} y1={y} x2={1150} y2={y} stroke="#dfe2f0" strokeWidth="1.2" />
    </g>
  )
}

function Event({
  x, laneFrom, laneTo, label, tint,
}: { x: number; laneFrom: number; laneTo?: number; label: string; tint: string }) {
  const to = laneTo ?? laneFrom
  return (
    <g>
      <line x1={x} y1={laneFrom} x2={x} y2={to} stroke={tint} strokeWidth="1.3" markerEnd={laneFrom !== to ? `url(#arrow-sse-${tint.replace('#', '')})` : undefined} />
      <circle cx={x} cy={laneFrom} r={3.2} fill={tint} />
      <circle cx={x} cy={to} r={3.2} fill={tint} />
      <Text x={x} y={laneFrom <= to ? laneFrom - 22 : to + 22} size={10.5} weight={700} fill={tint}>
        {label}
      </Text>
    </g>
  )
}

const SSE_COLORS = { life: '#4f46e5', delta: '#2563eb', tool: '#d97706', onto: '#059669' }

const SSE_COPY = {
  zh: {
    laneClient: { t: '浏览器', s: '事件卡实时渲染' },
    laneBackend: { t: 'backend', s: 'Eino ADK 装配运行' },
    laneExt: { t: '扩展执行面', s: '工具 · 子智能体 · 本体 · 知识' },
    toolCard: '工具/委派事件卡',
    deltaCont: 'message.delta（续）',
    note: '事件 JSON 统一 {type, run_id, ts, data} · 调试模式三档观测级别 · 原始事件可展开查看 · 对话可导出重放（REQ-117）',
  },
  en: {
    laneClient: { t: 'Browser', s: 'live event cards' },
    laneBackend: { t: 'backend', s: 'Eino ADK assembly & run' },
    laneExt: { t: 'Extended execution', s: 'tools · sub-agents · ontology · knowledge' },
    toolCard: 'tool/delegation event cards',
    deltaCont: 'message.delta (cont.)',
    note: 'Unified event JSON {type, run_id, ts, data} · three debug observation levels · expandable raw events · export & replay conversations (REQ-117)',
  },
}

export function SseTimeline() {
  const { locale } = useLocale()
  const c = SSE_COLORS
  const t = SSE_COPY[locale]
  return (
    <svg viewBox="0 0 1180 300" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label={locale === 'zh' ? 'SSE 事件流时序' : 'SSE event stream timeline'}>
      <ArrowDefs id={`arrow-sse-${c.life.replace('#', '')}`} color={c.life} />
      <ArrowDefs id={`arrow-sse-${c.delta.replace('#', '')}`} color={c.delta} />
      <ArrowDefs id={`arrow-sse-${c.tool.replace('#', '')}`} color={c.tool} />
      <ArrowDefs id={`arrow-sse-${c.onto.replace('#', '')}`} color={c.onto} />

      <Lane y={LANE_Y.client} title={t.laneClient.t} sub={t.laneClient.s} color="#eef2ff" />
      <Lane y={LANE_Y.backend} title={t.laneBackend.t} sub={t.laneBackend.s} color="#eef4ff" />
      <Lane y={LANE_Y.ext} title={t.laneExt.t} sub={t.laneExt.s} color="#f2fbf6" />

      {/* 生命周期 */}
      <Event x={210} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="run.started" tint={c.life} />
      <Event x={300} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="agent.started" tint={c.life} />
      {/* 增量 */}
      <Event x={405} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="message.delta ×N" tint={c.delta} />
      {/* 工具调用（backend → 扩展面往返） */}
      <Event x={530} laneFrom={LANE_Y.backend} laneTo={LANE_Y.ext} label="tool.call" tint={c.tool} />
      <Event x={640} laneFrom={LANE_Y.ext} laneTo={LANE_Y.backend} label="tool.result" tint={c.tool} />
      <Event x={715} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label={t.toolCard} tint={c.tool} />
      {/* 本体/知识 */}
      <Event x={830} laneFrom={LANE_Y.backend} laneTo={LANE_Y.ext} label="ontology.query / retrieval" tint={c.onto} />
      <Event x={955} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label={t.deltaCont} tint={c.delta} />
      <Event x={1075} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="run.finished | run.error" tint={c.life} />

      <Text x={585} y={288} size={11} fill={INK2}>{t.note}</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 3. 本体构建六路径 → 质量门禁 → 资产仓库（源：docs/03 / 04 · REQ-171 qualitygate）
// ---------------------------------------------------------------------------

const QG_COPY = {
  zh: {
    paths: ['手工向导（自定义）', 'OntoChat 对话引导', 'OntoExtend 增量扩展', '导入外部本体', '由知识库构建', '权威词表引入'],
    gate: '质量门禁 qualitygate',
    g1: '11 检查项 · 三维评分（REQ-171）',
    g2: 'LLM 生成产物必须通过 · 必要时自动修复',
    g3: '含导入审查合并（六路径收敛于资产段）',
    gFail: '未通过 → 自修复 / 退回修改',
    repair: '自修复循环',
    pass: '通过',
    store: '本体资产仓库',
    s1: 'spec_json 多形态资产（TTL / JSON-LD / GraphML / CSV）',
    s2: '版本管理与 diff · 质量卡 · 源码视图',
    s3: '进化候选受控生长 · 被引用聚合',
    s4: 'SPARQL 工作台 · 三维浏览 / WebVOWL 对照',
    s5: '伴生本体页签（动态薄本体）',
    toRun: '发布为运行方案（Oxigraph / Fuseki）→ MCP facade 供智能体消费',
  },
  en: {
    paths: ['Manual wizard (custom)', 'OntoChat guided chat', 'OntoExtend evolution', 'External ontology import', 'Build from knowledge base', 'Authoritative vocabularies'],
    gate: 'Quality gate (qualitygate)',
    g1: '11 checks · three-dimension score (REQ-171)',
    g2: 'LLM generations must pass · auto-repair when needed',
    g3: 'Includes import review/merge (six paths converge on the asset stage)',
    gFail: 'Fail → self-repair / return for edits',
    repair: 'self-repair loop',
    pass: 'Pass',
    store: 'Ontology asset store',
    s1: 'Multi-form spec_json assets (TTL / JSON-LD / GraphML / CSV)',
    s2: 'Versions & diff · quality card · source view',
    s3: 'Controlled growth of evolution candidates · reference aggregation',
    s4: 'SPARQL workbench · 3D browsing / WebVOWL comparison',
    s5: 'Companion ontology tab (dynamic thin ontology)',
    toRun: 'Publish as run plans (Oxigraph / Fuseki) → MCP facade for agent consumption',
  },
}

export function QualityGateFlow() {
  const { locale } = useLocale()
  const t = QG_COPY[locale]
  return (
    <svg viewBox="0 0 1180 330" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label={locale === 'zh' ? '本体构建路径与质量门禁' : 'Ontology build paths and the quality gate'}>
      <ArrowDefs id="arrow-qg" />

      {/* 六条路径 */}
      {t.paths.map((p, i) => {
        const y = 30 + i * 50
        return (
          <g key={p}>
            <Box x={30} y={y} w={230} h={38} fill="#eef2ff" stroke="#b6bcf2" rx={8} />
            <Text x={145} y={y + 19} size={12} weight={600}>{p}</Text>
            <line x1={260} y1={y + 19} x2={396} y2={163} stroke={LINE} strokeWidth="1.2" markerEnd="url(#arrow-qg)" />
          </g>
        )
      })}

      {/* 质量门禁 */}
      <Box x={400} y={96} w={280} h={134} fill="#fff7ed" stroke="#f0b269" rx={12} />
      <Text x={540} y={122} size={14} weight={800}>{t.gate}</Text>
      <Text x={540} y={148} size={11.5} fill={INK2}>{t.g1}</Text>
      <Text x={540} y={170} size={11.5} fill={INK2}>{t.g2}</Text>
      <Text x={540} y={196} size={11.5} fill={INK2}>{t.g3}</Text>
      <Text x={540} y={218} size={10.5} fill="#b45309">{t.gFail}</Text>
      {/* 自修复循环 */}
      <path d="M 540 230 C 480 285 420 260 405 232" fill="none" stroke="#d97706" strokeWidth="1.3" strokeDasharray="5 4" markerEnd="url(#arrow-qg)" />
      <Text x={470} y={278} size={10.5} fill="#b45309">{t.repair}</Text>

      {/* 门禁 → 资产 */}
      <line x1={680} y1={163} x2={770} y2={163} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-qg)" />
      <Text x={725} y={150} size={10.5} fill={INK2}>{t.pass}</Text>

      {/* 资产仓库 */}
      <Box x={774} y={76} w={370} h={178} fill="#ecfdf5" stroke="#9fd8bd" rx={12} />
      <Text x={959} y={104} size={14} weight={800}>{t.store}</Text>
      <Text x={959} y={132} size={11.5} fill={INK2}>{t.s1}</Text>
      <Text x={959} y={158} size={11.5} fill={INK2}>{t.s2}</Text>
      <Text x={959} y={184} size={11.5} fill={INK2}>{t.s3}</Text>
      <Text x={959} y={210} size={11.5} fill={INK2}>{t.s4}</Text>
      <Text x={959} y={236} size={11.5} fill={INK2}>{t.s5}</Text>

      {/* 资产 → 运行平面 */}
      <line x1={959} y1={254} x2={959} y2={300} stroke={LINE} strokeWidth="1.3" markerEnd="url(#arrow-qg)" />
      <Text x={959} y={316} size={11} fill={INK2}>{t.toRun}</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 4. 伴生本体（动态薄本体）旁路管线（源：REQ-170 · docs/03 §2.31）
// ---------------------------------------------------------------------------

const COMP_COPY = {
  zh: {
    main: '对话主链（Eino ADK · SSE 事件流 · 完全可观测）',
    mainSub: '旁路只在对话收尾时触发，不阻塞、不侵入主流程',
    trigger: '对话收尾触发',
    steps: [
      { t: '① 旁路抽取', d: 'LLM 抽取知识候选' },
      { t: '② 人工确认', d: '候选确认 UI（可批量）' },
      { t: '③ 会话图引擎', d: '确认后入会话图（薄本体）' },
      { t: '④ 并入检索源', d: '自动成为 KG 检索 TTL 源' },
    ],
    principle: '低侵入三原则：开关默认关 · 产物可摘除 · 失败静默降级不影响主链（REQ-170，平台横切原则的实例化）',
  },
  en: {
    main: 'Conversation main path (Eino ADK · SSE events · fully observable)',
    mainSub: 'The bypass only triggers as the conversation wraps up — never blocks or intrudes on the main flow',
    trigger: 'triggers at conversation end',
    steps: [
      { t: '① Bypass extraction', d: 'LLM extracts knowledge candidates' },
      { t: '② Human confirmation', d: 'candidate review UI (batch-capable)' },
      { t: '③ Session graph engine', d: 'confirmed entries enter the session graph (thin ontology)' },
      { t: '④ Joins retrieval sources', d: 'automatically becomes a KG retrieval TTL source' },
    ],
    principle: 'Three low-intrusion principles: off by default · removable artifacts · silent degradation on failure never affects the main path (REQ-170, an instantiation of the platform’s cross-cutting principles)',
  },
}

export function CompanionPipeline() {
  const { locale } = useLocale()
  const t = COMP_COPY[locale]
  return (
    <svg viewBox="0 0 1180 250" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label={locale === 'zh' ? '伴生本体旁路管线' : 'Companion-ontology bypass pipeline'}>
      <ArrowDefs id="arrow-comp" />

      {/* 主链 */}
      <Box x={30} y={28} w={1120} h={54} fill="#eef4ff" stroke="#a8bdf0" rx={12} />
      <Text x={590} y={50} size={13} weight={800}>{t.main}</Text>
      <Text x={590} y={70} size={11} fill={INK2}>{t.mainSub}</Text>

      {/* 主链 → 旁路入口 */}
      <line x1={210} y1={82} x2={210} y2={128} stroke="#8a90ad" strokeWidth="1.3" strokeDasharray="5 4" markerEnd="url(#arrow-comp)" />
      <Text x={218} y={108} size={10.5} fill={INK2} anchor="start">{t.trigger}</Text>

      {/* 旁路四节点 */}
      {t.steps.map((n, i) => (
        <g key={n.t}>
          <Box x={[60, 340, 620, 900][i]} y={132} w={220} h={64} fill={['#eef2ff', '#fff7ed', '#ecfdf5', '#ecfdf5'][i]} stroke={['#b6bcf2', '#f0b269', '#9fd8bd', '#9fd8bd'][i]} rx={10} />
          <Text x={[60, 340, 620, 900][i] + 110} y={156} size={12.5} weight={700}>{n.t}</Text>
          <Text x={[60, 340, 620, 900][i] + 110} y={178} size={10.5} fill={INK2}>{n.d}</Text>
          {i < 3 && <line x1={[60, 340, 620, 900][i] + 220} y1={164} x2={[60, 340, 620, 900][i] + 280} y2={164} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-comp)" />}
        </g>
      ))}

      {/* 低侵入三原则 */}
      <Text x={590} y={228} size={11.5} fill={INK2}>{t.principle}</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 5. 协议枢纽：MCP / A2A 双向开放 + 外部智能体接入（源：REQ-131/369/416/426~434/441~446
//    —— platform-knowledge/02_智能体/00 能力总表「对外服务化 / 外部智能体接入」行）
// ---------------------------------------------------------------------------

const HUB_COPY = {
  zh: {
    serveHead: '对外供给 —— 平台智能体被外部调用',
    serveMcp: { t: '外部 MCP 客户端', d: 'Claude Desktop / Cursor 直连' },
    serveMcpLine: '标准 MCP 服务',
    serveA2a: { t: '其他 Agent 平台 / 应用', d: 'A2A 客户端经 agent card 发现调用' },
    serveA2aLine: 'A2A · agent card 发现',
    hubTitle: '平台智能体运行时',
    hub1: '本地 / CLI / A2A 三色成员同队',
    hub2: '协调者按成员能力画像智能路由',
    hub3: 'workflow 顺序·并行·循环 · transfer 移交',
    hub4: 'SSE 事件流 · 全程可观测可回放',
    inHead: '对内接入 —— 外部智能体进协作',
    inCli: { t: 'CLI 编码智能体', d: 'Claude Code / OpenCode / Aider / DSH' },
    inCliLine: '委派 · 命令行接入',
    inA2a: { t: 'A2A 远程智能体', d: '任意 A2A server 均可接入' },
    inA2aLine: '委派 · 远程接入 · 流式',
    inStdio: { t: '本地 stdio MCP server', d: '本地服务白名单托管' },
    inStdioLine: '接入 · 本地服务托管',
    govTitle: '统一治理（内外一体）',
    gov1: '危险操作分级审批 · 过程审计与泳道回放 · 沙箱隔离 · 凭据加密托管',
    gov2: '委派携带上下文：项目文件 / 知识库 / 本体 guide 摘要（连接器开关显式确认）',
    note: '目录互发现：外部智能体可反向发现平台暴露的智能体与外部成员元数据（凭据与配置细节不透出）',
  },
  en: {
    serveHead: 'Serve outward — platform agents get called',
    serveMcp: { t: 'External MCP clients', d: 'Claude Desktop / Cursor direct' },
    serveMcpLine: 'Standard MCP service',
    serveA2a: { t: 'Other agent platforms / apps', d: 'A2A clients discover via agent card' },
    serveA2aLine: 'A2A · agent-card discovery',
    hubTitle: 'Platform agent runtime',
    hub1: 'Local / CLI / A2A members in one team',
    hub2: 'Coordinator routes by member capability profiles',
    hub3: 'Sequential · parallel · loop workflows · transfer',
    hub4: 'SSE event stream — observable & replayable',
    inHead: 'Bring in — external agents join collaboration',
    inCli: { t: 'CLI coding agents', d: 'Claude Code / OpenCode / Aider / DSH' },
    inCliLine: 'delegate · CLI channel',
    inA2a: { t: 'A2A remote agents', d: 'any A2A server plugs in' },
    inA2aLine: 'delegate · remote · streaming',
    inStdio: { t: 'Local stdio MCP servers', d: 'Local services, allowlisted & hosted' },
    inStdioLine: 'attach · hosted local service',
    govTitle: 'Unified governance (both ways)',
    gov1: 'Tiered approvals for dangerous operations · process audit & swimlane replay · sandbox isolation · encrypted credentials',
    gov2: 'Delegations carry context: project files / knowledge base / ontology guide (explicit connector opt-in)',
    note: 'Mutual discovery: external agents can reverse-discover platform-exposed agents and external-member metadata (credentials and config details never leak)',
  },
}

/** 水平连线（带标签；标签长度受连线区宽度约束，超长文案落在两侧卡与页面卡片里） */
function HLine({ x1, x2, y, label }: { x1: number; x2: number; y: number; label: string }) {
  return (
    <g>
      <line x1={x1} y1={y} x2={x2} y2={y} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-hub)" />
      <Text x={(x1 + x2) / 2} y={y - 9} size={10} fill={INK2}>{label}</Text>
    </g>
  )
}

export function ProtocolHubDiagram() {
  const { locale } = useLocale()
  const t = HUB_COPY[locale]
  return (
    <svg viewBox="0 0 1180 424" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label={locale === 'zh' ? '协议枢纽：MCP 与 A2A 双向开放' : 'Protocol hub: MCP and A2A, open both ways'}>
      <ArrowDefs id="arrow-hub" />

      {/* ===== 左列：对外供给（280~420 为连线区，标签受其宽度约束） ===== */}
      <Text x={30} y={36} size={11.5} weight={700} fill={INK2} anchor="start">{t.serveHead}</Text>
      <InnerCard x={30} y={52} w={250} h={72} title={t.serveMcp.t} desc={t.serveMcp.d} tint={BRAND} />
      <InnerCard x={30} y={168} w={250} h={72} title={t.serveA2a.t} desc={t.serveA2a.d} tint={BRAND} />
      <HLine x1={280} x2={416} y={88} label={t.serveMcpLine} />
      <HLine x1={280} x2={416} y={204} label={t.serveA2aLine} />

      {/* ===== 中心：平台智能体运行时 ===== */}
      <Box x={420} y={52} w={340} h={228} fill="#eef4ff" stroke="#a8bdf0" rx={12} />
      <Text x={590} y={80} size={14} weight={800}>{t.hubTitle}</Text>
      <Text x={590} y={116} size={11.5} fill={INK}>{t.hub1}</Text>
      <Text x={590} y={148} size={11.5} fill={INK}>{t.hub2}</Text>
      <Text x={590} y={180} size={11.5} fill={INK}>{t.hub3}</Text>
      <Text x={590} y={212} size={11.5} fill={INK}>{t.hub4}</Text>
      <Text x={590} y={252} size={10.5} fill={INK2}>MCP · A2A · CLI</Text>

      {/* ===== 右列：对内接入（760~900 为连线区） ===== */}
      <Text x={900} y={36} size={11.5} weight={700} fill={INK2} anchor="start">{t.inHead}</Text>
      <InnerCard x={900} y={52} w={250} h={64} title={t.inCli.t} desc={t.inCli.d} tint="#58a27c" />
      <InnerCard x={900} y={134} w={250} h={64} title={t.inA2a.t} desc={t.inA2a.d} tint="#58a27c" />
      <InnerCard x={900} y={216} w={250} h={64} title={t.inStdio.t} desc={t.inStdio.d} tint="#58a27c" />
      <HLine x1={760} x2={896} y={84} label={t.inCliLine} />
      <HLine x1={760} x2={896} y={166} label={t.inA2aLine} />
      <HLine x1={760} x2={896} y={248} label={t.inStdioLine} />

      {/* ===== 底部：统一治理 ===== */}
      <Box x={30} y={300} w={1120} h={84} fill="#fff7ed" stroke="#f0b269" rx={12} />
      <Text x={590} y={324} size={12.5} weight={800}>{t.govTitle}</Text>
      <Text x={590} y={348} size={10.5} fill={INK2}>{t.gov1}</Text>
      <Text x={590} y={368} size={10.5} fill={INK2}>{t.gov2}</Text>

      {/* 底注 */}
      <Text x={590} y={410} size={11} fill={INK2}>{t.note}</Text>
    </svg>
  )
}
