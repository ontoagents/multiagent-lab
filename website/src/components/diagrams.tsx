/**
 * 官网图示组件库（REQ-238 扩展）：纯内联 SVG，与品牌视觉同源（#4f46e5 系）。
 * 设计目标：直观易读——分层清晰 / 连线带语义标签 / 配色区分平面与事件类别；
 * 全部为无外部依赖的 React 组件，可直接构建进静态站。
 */

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

export function ArchDiagram() {
  return (
    <svg viewBox="0 0 1180 660" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label="系统架构：三平面 + 双代理">
      <ArrowDefs id="arrow-arch" />

      {/* ===== 客户端层 ===== */}
      <Text x={34} y={52} size={11.5} weight={700} fill={INK2} anchor="start">客户端层</Text>
      <InnerCard x={100} y={24} w={300} h={56} title="浏览器 Web UI" desc="React 18 · AntD 6 · hash 路由" tint={BRAND} />
      <InnerCard x={440} y={24} w={330} h={56} title="桌面客户端（Tauri 2 壳）" desc="Windows 安装包 / macOS DMG · 系统托盘" tint={BRAND} />
      <InnerCard x={810} y={24} w={310} h={56} title="外部 MCP 客户端" desc="Claude Desktop / Cursor 直连" tint={BRAND} />

      {/* 客户端 → 主平台 */}
      <VLine x={250} y1={80} y2={128} label="HTTP · REST + SSE" />
      <VLine x={605} y1={80} y2={128} label="127.0.0.1:8080（壳托管服务）" labelSide="left" />
      <VLine x={965} y1={80} y2={128} label="/mcp · Streamable HTTP" />

      {/* ===== 主平台 ===== */}
      <Box x={60} y={130} w={1060} h={172} fill="#eef4ff" stroke="#a8bdf0" />
      <Text x={84} y={156} size={13.5} weight={800} anchor="start">主平台 backend（Go · Hertz · :8080）</Text>
      <Text x={84} y={174} size={10.5} fill={INK2} anchor="start">智能体装配与运行 · 知识库 · 设置 · 平台助手</Text>
      <InnerCard x={84} y={188} w={310} h={52} title="Eino ADK 装配与运行" desc="对话 / 对比模式 / 多智能体编排" tint={BRAND} />
      <InnerCard x={414} y={188} w={310} h={52} title="SSE 事件流" desc="工具 / 子智能体 / 本体查询全留痕" tint={BRAND} />
      <InnerCard x={744} y={188} w={352} h={52} title="知识库 RAG / GraphRAG" desc="Qdrant 向量（主）· SQLite 余弦 fallback" tint={BRAND} />
      <InnerCard x={84} y={246} w={310} h={44} title="技能 / 工具 / 外部连接器" tint={BRAND} />
      <InnerCard x={414} y={246} w={310} h={44} title="模型代理（双协议）" desc="openai_compat / anthropic" tint={BRAND} />
      <InnerCard x={744} y={246} w={352} h={44} title="Agent 对外 MCP 服务化（/mcp）" tint={BRAND} />

      {/* 主平台 → 本体两平面 */}
      <VLine x={310} y1={302} y2={360} label="反向代理 · 同源嵌入" labelSide="left" />
      <VLine x={870} y1={302} y2={360} label="反向代理 + MCP Client" labelSide="right" />

      {/* ===== 本体两平面 ===== */}
      <Box x={60} y={362} w={520} h={104} fill="#eef2ff" stroke="#b6bcf2" />
      <Text x={84} y={388} size={13.5} weight={800} anchor="start">本体构建平面 ontology-service（:8091）</Text>
      <Text x={84} y={410} size={11} fill={INK2} anchor="start">六条构建路径 · LLM 生成必过质量门禁</Text>
      <Text x={84} y={430} size={11} fill={INK2} anchor="start">spec_json 多形态资产仓库 · 导入审查合并 · 工具链</Text>

      <Box x={600} y={362} w={520} h={104} fill="#ecfdf5" stroke="#9fd8bd" />
      <Text x={624} y={388} size={13.5} weight={800} anchor="start">本体运行平面 runtime-manager（:8090）</Text>
      <Text x={624} y={410} size={11} fill={INK2} anchor="start">运行方案编排（Oxigraph / Fuseki）</Text>
      <Text x={624} y={430} size={11} fill={INK2} anchor="start">统一 MCP facade：onto_* 五工具 + sparql_query</Text>

      {/* 构建 → 运行（资产交付） */}
      <line x1={580} y1={414} x2={600} y2={414} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-arch)" />
      <Text x={590} y={400} size={10} fill={INK2}>资产</Text>

      {/* 平面 → 引擎/存储 */}
      <VLine x={860} y1={466} y2={520} label="SPARQL / 推理" labelSide="right" />

      {/* ===== 引擎与存储层 ===== */}
      <Text x={34} y={546} size={11.5} weight={700} fill={INK2} anchor="start">引擎与存储</Text>
      <InnerCard x={100} y={522} w={210} h={48} title="Qdrant（向量，主）" desc="SQLite 余弦 fallback" tint="#9db8e8" />
      <InnerCard x={340} y={522} w={240} h={48} title="SQLite 业务库" desc="AES-256-GCM 密钥加密" tint="#9db8e8" />
      <InnerCard x={700} y={522} w={190} h={48} title="Oxigraph" desc="SPARQL 型引擎（默认）" tint="#7fc8a4" />
      <InnerCard x={920} y={522} w={180} h={48} title="Fuseki" desc="OWL 推理对照引擎" tint="#7fc8a4" />

      {/* backend → Qdrant/SQLite（向量检索链） */}
      <VLine x={200} y1={290} y2={520} dashed label="向量检索" labelSide="right" />
      {/* backend → LLM */}
      <VLine x={1150} y1={290} y2={590} dashed color="#c9b2e8" />

      {/* ===== 外部 LLM 供应商 ===== */}
      <Box x={60} y={594} w={1060} h={44} fill="#faf8ff" stroke="#d9c8f0" rx={10} />
      <Text x={590} y={616} size={12} weight={700} fill={INK2}>外部 LLM 供应商 —— OpenAI 兼容 / Anthropic 兼容（多连接 · 自动发现 · 厂商预设 · Key 本地 AES-256-GCM 加密）</Text>
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

export function SseTimeline() {
  const c = SSE_COLORS
  return (
    <svg viewBox="0 0 1180 300" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label="SSE 事件流时序">
      <ArrowDefs id={`arrow-sse-${c.life.replace('#', '')}`} color={c.life} />
      <ArrowDefs id={`arrow-sse-${c.delta.replace('#', '')}`} color={c.delta} />
      <ArrowDefs id={`arrow-sse-${c.tool.replace('#', '')}`} color={c.tool} />
      <ArrowDefs id={`arrow-sse-${c.onto.replace('#', '')}`} color={c.onto} />

      <Lane y={LANE_Y.client} title="浏览器" sub="事件卡实时渲染" color="#eef2ff" />
      <Lane y={LANE_Y.backend} title="backend" sub="Eino ADK 装配运行" color="#eef4ff" />
      <Lane y={LANE_Y.ext} title="扩展执行面" sub="工具 · 子智能体 · 本体 · 知识" color="#f2fbf6" />

      {/* 生命周期 */}
      <Event x={210} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="run.started" tint={c.life} />
      <Event x={300} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="agent.started" tint={c.life} />
      {/* 增量 */}
      <Event x={405} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="message.delta ×N" tint={c.delta} />
      {/* 工具调用（backend → 扩展面往返） */}
      <Event x={530} laneFrom={LANE_Y.backend} laneTo={LANE_Y.ext} label="tool.call" tint={c.tool} />
      <Event x={640} laneFrom={LANE_Y.ext} laneTo={LANE_Y.backend} label="tool.result" tint={c.tool} />
      <Event x={715} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="工具/委派事件卡" tint={c.tool} />
      {/* 本体/知识 */}
      <Event x={830} laneFrom={LANE_Y.backend} laneTo={LANE_Y.ext} label="ontology.query / retrieval" tint={c.onto} />
      <Event x={955} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="message.delta（续）" tint={c.delta} />
      <Event x={1075} laneFrom={LANE_Y.backend} laneTo={LANE_Y.client} label="run.finished | run.error" tint={c.life} />

      <Text x={585} y={288} size={11} fill={INK2}>事件 JSON 统一 {'{type, run_id, ts, data}'} · 调试模式三档观测级别 · 原始事件可展开查看 · 对话可导出重放（REQ-117）</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 3. 本体构建六路径 → 质量门禁 → 资产仓库（源：docs/03 / 04 · REQ-171 qualitygate）
// ---------------------------------------------------------------------------

export function QualityGateFlow() {
  const paths = ['手工向导（自定义）', 'OntoChat 对话引导', 'OntoExtend 增量扩展', 'Open Ontologies 双轨导入', '由知识库构建', 'CSV 灌装']
  return (
    <svg viewBox="0 0 1180 330" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label="本体构建路径与质量门禁">
      <ArrowDefs id="arrow-qg" />

      {/* 六条路径 */}
      {paths.map((p, i) => {
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
      <Text x={540} y={122} size={14} weight={800}>质量门禁 qualitygate</Text>
      <Text x={540} y={148} size={11.5} fill={INK2}>11 检查项 · 三维评分（REQ-171）</Text>
      <Text x={540} y={170} size={11.5} fill={INK2}>LLM 生成产物必须通过 · 必要时自动修复</Text>
      <Text x={540} y={196} size={11.5} fill={INK2}>含导入审查合并（六路径收敛于资产段）</Text>
      <Text x={540} y={218} size={10.5} fill="#b45309">未通过 → 自修复 / 退回修改</Text>
      {/* 自修复循环 */}
      <path d="M 540 230 C 480 285 420 260 405 232" fill="none" stroke="#d97706" strokeWidth="1.3" strokeDasharray="5 4" markerEnd="url(#arrow-qg)" />
      <Text x={470} y={278} size={10.5} fill="#b45309">自修复循环</Text>

      {/* 门禁 → 资产 */}
      <line x1={680} y1={163} x2={770} y2={163} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-qg)" />
      <Text x={725} y={150} size={10.5} fill={INK2}>通过</Text>

      {/* 资产仓库 */}
      <Box x={774} y={76} w={370} h={178} fill="#ecfdf5" stroke="#9fd8bd" rx={12} />
      <Text x={959} y={104} size={14} weight={800}>本体资产仓库</Text>
      <Text x={959} y={132} size={11.5} fill={INK2}>spec_json 多形态资产（TTL / JSON-LD / GraphML / CSV）</Text>
      <Text x={959} y={158} size={11.5} fill={INK2}>版本管理与 diff · 质量卡 · 源码视图</Text>
      <Text x={959} y={184} size={11.5} fill={INK2}>进化候选受控生长 · 被引用聚合</Text>
      <Text x={959} y={210} size={11.5} fill={INK2}>SPARQL 工作台 · 三维浏览 / WebVOWL 对照</Text>
      <Text x={959} y={236} size={11.5} fill={INK2}>伴生本体页签（动态薄本体）</Text>

      {/* 资产 → 运行平面 */}
      <line x1={959} y1={254} x2={959} y2={300} stroke={LINE} strokeWidth="1.3" markerEnd="url(#arrow-qg)" />
      <Text x={959} y={316} size={11} fill={INK2}>发布为运行方案（Oxigraph / Fuseki）→ MCP facade 供智能体消费</Text>
    </svg>
  )
}

// ---------------------------------------------------------------------------
// 4. 伴生本体（动态薄本体）旁路管线（源：REQ-170 · docs/03 §2.31）
// ---------------------------------------------------------------------------

export function CompanionPipeline() {
  return (
    <svg viewBox="0 0 1180 250" style={{ width: '100%', minWidth: 860, display: 'block' }} role="img" aria-label="伴生本体旁路管线">
      <ArrowDefs id="arrow-comp" />

      {/* 主链 */}
      <Box x={30} y={28} w={1120} h={54} fill="#eef4ff" stroke="#a8bdf0" rx={12} />
      <Text x={590} y={50} size={13} weight={800}>对话主链（Eino ADK · SSE 事件流 · 完全可观测）</Text>
      <Text x={590} y={70} size={11} fill={INK2}>旁路只在对话收尾时触发，不阻塞、不侵入主流程</Text>

      {/* 主链 → 旁路入口 */}
      <line x1={210} y1={82} x2={210} y2={128} stroke="#8a90ad" strokeWidth="1.3" strokeDasharray="5 4" markerEnd="url(#arrow-comp)" />
      <Text x={218} y={108} size={10.5} fill={INK2} anchor="start">对话收尾触发</Text>

      {/* 旁路四节点 */}
      {[
        { x: 60, t: '① 旁路抽取', d: 'LLM 抽取知识候选', tint: '#eef2ff', s: '#b6bcf2' },
        { x: 340, t: '② 人工确认', d: '候选确认 UI（可批量）', tint: '#fff7ed', s: '#f0b269' },
        { x: 620, t: '③ 会话图引擎', d: '确认后入会话图（薄本体）', tint: '#ecfdf5', s: '#9fd8bd' },
        { x: 900, t: '④ 并入检索源', d: '自动成为 KG 检索 TTL 源', tint: '#ecfdf5', s: '#9fd8bd' },
      ].map((n, i) => (
        <g key={n.t}>
          <Box x={n.x} y={132} w={220} h={64} fill={n.tint} stroke={n.s} rx={10} />
          <Text x={n.x + 110} y={156} size={12.5} weight={700}>{n.t}</Text>
          <Text x={n.x + 110} y={178} size={10.5} fill={INK2}>{n.d}</Text>
          {i < 3 && <line x1={n.x + 220} y1={164} x2={n.x + 280} y2={164} stroke={LINE} strokeWidth="1.4" markerEnd="url(#arrow-comp)" />}
        </g>
      ))}

      {/* 低侵入三原则 */}
      <Text x={590} y={228} size={11.5} fill={INK2}>低侵入三原则：开关默认关 · 产物可摘除 · 失败静默降级不影响主链（REQ-170，平台横切原则的实例化）</Text>
    </svg>
  )
}
