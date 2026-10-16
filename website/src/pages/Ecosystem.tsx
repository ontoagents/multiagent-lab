import { Tag } from 'antd'
import { REPO_URL } from '../content'

// ---------------------------------------------------------------------------
// 开源生态页（REQ-238 官网）：把 docs/15《开源项目及论文登记簿》转成站点可读版。
// 内容单源：全部条目与结论提炼自 docs/15（v2.35）与 platform-knowledge/08_调研预研；
// 本页只做"图谱 + 对比表"的整理呈现，登记/状态以 docs/15 台账为准。
// ---------------------------------------------------------------------------

type StatusKey = 'integrated' | 'guided' | 'planned' | 'reference' | 'removed'

const STATUS_META: Record<StatusKey, { label: string; color: string }> = {
  integrated: { label: '✅ 已集成', color: 'green' },
  guided: { label: '🔧 引导执行', color: 'gold' },
  planned: { label: '📋 规划集成', color: 'blue' },
  reference: { label: '🔭 对照参考', color: 'default' },
  removed: { label: '⏸ 留档/已移除', color: 'purple' },
}

/** 七簇对比表（源：docs/15 七类，语言/技术栈与定位为登记簿字段整理） */
const GROUPS: { id: string; title: string; desc: string; rows: { name: string; license: string; stack: string; status: StatusKey; note: string }[] }[] = [
  {
    id: 'core',
    title: '一、智能体平台核心（Go 栈）',
    desc: '项目主骨架与运行底座——「Go + Eino ADK 迷你版 Coze」的直接构成。',
    rows: [
      { name: 'Eino / Eino ADK', license: 'Apache-2.0', stack: 'Go', status: 'integrated', note: '智能体编排主骨架：组件抽象、图编排、ReAct / 多智能体运行时' },
      { name: 'CloudWeGo Hertz', license: 'Apache-2.0', stack: 'Go', status: 'integrated', note: 'backend HTTP 服务与 API 网关（构建 / 管理面 REST）' },
      { name: 'mcp-go', license: 'MIT', stack: 'Go', status: 'integrated', note: 'MCP 协议 Go 实现，本体 MCP facade（onto_* 工具）基座' },
      { name: 'eino-ext', license: 'Apache-2.0', stack: 'Go', status: 'integrated', note: 'MCP Client 组件 + Claude 模型适配器（anthropic 协议通道）' },
      { name: 'santhosh-tekuri/jsonschema', license: 'MIT', stack: 'Go', status: 'integrated', note: 'spec_json 结构校验（Spec.Validate 底层）' },
      { name: 'gonum', license: 'BSD-3', stack: 'Go', status: 'planned', note: '图算法包：KG 社区检测 Louvain 现成（KB 升级草案 KB-5）' },
      { name: 'DeepSeek Harness（dsh）', license: 'MIT', stack: 'TypeScript', status: 'guided', note: '官方 agent harness，第 4 推理后端适配器（M24 / REQ-160）' },
      { name: 'Tauri 2', license: 'MIT / Apache-2.0', stack: 'Rust', status: 'planned', note: '桌面客户端壳：WebView 直载本机服务，Rust 仅壳零业务侵入（REQ-321）' },
    ],
  },
  {
    id: 'build',
    title: '二、本体构建与校验工具链',
    desc: '本体工程前半程：建模、解析、校验与推理，多走 Python sidecar 与引导执行。',
    rows: [
      { name: 'rdflib', license: 'BSD', stack: 'Python', status: 'integrated', note: 'sidecar 三职责：OWL/RDF 解析 + spec_json→TTL 导出 + 图校验（D-O5）' },
      { name: 'pySHACL', license: 'Apache-2.0', stack: 'Python', status: 'integrated', note: 'SHACL 业务约束校验执行器（质量门禁三层之业务层）' },
      { name: 'Protégé 桌面版', license: 'BSD', stack: 'Java', status: 'guided', note: '业界标准建模器，guided 路线；架构深研借鉴八点（59 号）' },
      { name: 'WebProtégé', license: 'Apache-2.0', stack: 'Web', status: 'guided', note: '在线协作建模，导入→编辑→导出往返' },
      { name: 'ROBOT', license: 'Apache-2.0', stack: 'Java', status: 'guided', note: '命令行本体处理（template / merge / subset），进阶批处理' },
      { name: 'OOPS!', license: 'Apache-2.0', stack: 'Web', status: 'guided', note: '建模陷阱扫描 34 类，质量门禁对照维度（D-O8 已采纳）' },
      { name: 'ELK / HermiT', license: 'LGPL', stack: 'Java', status: 'guided', note: 'OWL 推理器（ELK 快 / HermiT 全量），逻辑一致性校验' },
      { name: 'OWLAPY', license: 'MIT', stack: 'Python', status: 'reference', note: 'LLM 本体构建框架，借鉴提示词与「生成-校验」循环设计' },
    ],
  },
  {
    id: 'runtime',
    title: '三、运行平面：图引擎与向量存储',
    desc: '本体 TTL 与知识向量的存放与查询——SPARQL 引擎双方案 + 可插拔向量后端。',
    rows: [
      { name: 'Oxigraph', license: 'MIT / Apache-2.0', stack: 'Rust', status: 'integrated', note: 'P1 唯一 SPARQL 引擎（单二进制），onto_*→SPARQL 翻译层（D-O5）' },
      { name: 'Apache Jena Fuseki', license: 'Apache-2.0', stack: 'Java', status: 'integrated', note: 'SPARQL + OWL 推理开关，推理型运行方案（含推理对账下限闸）' },
      { name: 'Qdrant', license: 'Apache-2.0', stack: 'Rust', status: 'integrated', note: '向量检索主后端；SQLite 余弦兜底，KB_VECTOR_BACKEND 可插拔' },
      { name: 'Cayley', license: 'Apache-2.0', stack: 'Go', status: 'planned', note: 'Go 嵌入式图库，memory_graph P2（LG-12 自研取消，直接集成）' },
      { name: 'HelixDB', license: 'Apache-2.0', stack: 'Rust', status: 'reference', note: '图+向量+BM25 三模态引擎——非 RDF 不引入；借鉴 pre-filter→ranking 检索形态（67 号）' },
    ],
  },
  {
    id: 'frontend',
    title: '四、前端可视化与编辑器',
    desc: '本体图交互、源码编辑与版本对照的前端构成。',
    rows: [
      { name: 'ReactFlow（@xyflow）', license: 'MIT', stack: 'TypeScript', status: 'integrated', note: '结构视图交互画布（D-O9 对比择优胜出），spec_json 直渲' },
      { name: 'CodeMirror 6', license: 'MIT', stack: 'JavaScript', status: 'integrated', note: '源码 / SPARQL / JSON 编辑器（REQ-92 / 93 / 145）' },
      { name: 'react-diff-viewer-continued', license: 'MIT', stack: 'TypeScript', status: 'integrated', note: '版本 diff 文本对照（VersionDiff 页签）' },
      { name: '3d-force-graph', license: 'MIT', stack: 'JavaScript', status: 'integrated', note: '三维力导向本体可视化（REQ-154 / 179）' },
      { name: 'WebVOWL + OWL2VOWL', license: 'MIT', stack: 'JS / Java', status: 'integrated', note: 'TBox 语义标准可视化，VOWL 对照视图（owl2vowl 原生导出）' },
      { name: 'Sigma.js + graphology', license: 'MIT', stack: 'JavaScript', status: 'reference', note: '2D 大图渲染候选（VIZ-4 3D 性能退路）' },
      { name: 'AntV G6 5 / Graphin', license: 'MIT', stack: 'JavaScript', status: 'planned', note: '大图 / 3D 能力需 PoC 实证（生态一致性优先项）' },
      { name: 'LodLive', license: 'MIT', stack: 'JavaScript', status: 'planned', note: 'ABox 实例关系探索（需 SPARQL 端点）' },
      { name: 'Yasgui', license: 'MIT', stack: 'JavaScript', status: 'removed', note: '曾为 SPARQL 工作台主入口；REQ-92 改 CM6+AntD 自组装后移除' },
    ],
  },
  {
    id: 'source',
    title: '五、权威数据源与沙箱运行时',
    desc: '教学种子本体的数据基准，以及智能体执行环境的隔离候选。',
    rows: [
      { name: 'HGNC', license: '公开访问', stack: '数据源', status: 'integrated', note: '基因命名权威库——gene_core 种子命名基准（REQ-153）' },
      { name: 'Gene Ontology（GO）', license: 'CC-BY 4.0', stack: '数据源', status: 'integrated', note: '三轴功能树结构参考（自建教学子集，未复制记录）' },
      { name: 'HPO', license: 'CC-BY 3.0/4.0', stack: '数据源', status: 'integrated', note: '人类表型本体——med_common 分类结构参考' },
      { name: 'gVisor', license: 'Apache-2.0', stack: 'Go', status: 'reference', note: '用户态内核沙箱（runsc），M10 强隔离候选' },
      { name: 'Kata Containers', license: 'Apache-2.0', stack: 'Go', status: 'reference', note: '轻量 VM 级隔离，与 gVisor 二选一对照' },
      { name: 'E2B', license: 'Apache-2.0', stack: 'Python', status: 'reference', note: 'AI 代码执行沙箱——工具级定位与执行体沙箱错配，参考不引入' },
      { name: 'Podman', license: 'Apache-2.0', stack: 'Go', status: 'reference', note: 'rootless 容器运行时（M10 10c 运行时参数化候选）' },
      { name: 'Agent Substrate / K8s Agent Sandbox', license: 'Apache-2.0', stack: 'Go / K8s', status: 'reference', note: 'K8s 高密度沙箱运行时与生命周期管理（M10 10d 后期备选）' },
    ],
  },
  {
    id: 'llm',
    title: '六、LLM 本体工程（思想借鉴）',
    desc: '对话式构建与质量门禁的方法论来源——不引代码，采纳 prompt 素材与流程设计。',
    rows: [
      { name: 'OntoChat', license: 'MIT', stack: 'Python', status: 'reference', note: '对话式需求工程四步流（用户故事→CQ→聚类→测试），REQ-82/103 蓝本；开源 prompt 全量可复用（63 号）' },
      { name: 'OntoExtend', license: '未标 LICENSE', stack: 'Python', status: 'reference', note: '需求驱动三段式扩展（Retriever→Extender→Integrator），REQ-83/103 参考架构（64 号）' },
      { name: 'OrionBelt', license: 'MIT', stack: 'Python', status: 'reference', note: 'Streamlit 本体工作台，导入三策略与审查参照（23 号）' },
      { name: 'OLIVAW', license: 'MIT', stack: 'Python', status: 'reference', note: '本体质量 CI 门禁三模式，qualitygate 对照维度（23 号）' },
      { name: 'Domain-OntoGen', license: '开源', stack: '—', status: 'reference', note: 'LLM 需求引出→本体生成管线（23 号）' },
    ],
  },
  {
    id: 'compare',
    title: '七、平台与范式对照',
    desc: '同赛道产品与设计范式的深度调研对象——只对照、不集成（除注明外均为 🔭 不引代码）。',
    rows: [
      { name: 'semantica', license: 'MIT', stack: 'Python', status: 'reference', note: '曾集成后 D-O15 反转去依赖；三层架构 / GraphRAG / PROV-O 溯源思路留存对照' },
      { name: 'Open Ontologies', license: 'MIT', stack: 'Rust', status: 'reference', note: '122 MCP 工具 + 形式化验证内核；2026-10-08 拍板摘除集成，三层借鉴（65/66 号）' },
      { name: 'OpenBKN', license: 'Apache-2.0 + 定制', stack: 'TypeScript', status: 'reference', note: '企业本体平台（对标 Palantir）；BKN Lang DSL / 知识包化 / 本体→SKILL.md（D-O12 仅参考）' },
      { name: 'Ontology-Playground', license: 'MIT', stack: 'TypeScript', status: 'reference', note: '微软本体学习 Playground，定位同构直接对照：Quest 激励 / 领域路径 / round-trip 保真度测试' },
      { name: 'Helix Foundry', license: 'Apache-2.0', stack: 'TypeScript', status: 'reference', note: '数据驱动本体推断 + base/overlay/audit 三层编辑保护（67 号，REQ-292~295）' },
      { name: 'ontology-on-snowflake', license: 'Apache-2.0', stack: 'SQL', status: 'reference', note: 'KG_NODE/KG_EDGE 建模 + 官方 benchmark 四档（50~78.2%），qualitygate 量标（REQ-313）' },
      { name: 'Graphify', license: 'Apache-2.0', stack: 'Python', status: 'reference', note: '边级可信度标注 / 增量更新 / 「确定性解析优先 + LLM 兜底」混合管线' },
      { name: 'kubernetes-ontology', license: 'Apache-2.0', stack: 'Go', status: 'reference', note: '边级溯源声明层 / AI 消费契约 / canonicalId 身份键（62 号 K1~K4）' },
      { name: 'Kartograph', license: 'Apache-2.0', stack: 'Python', status: 'reference', note: '架构边界可执行化 / 统一写路径契约 / MCP 防御契约（62 号 G1~G6）' },
      { name: 'LLM Wiki 生态（nashsu/llm_wiki）', license: 'GPL-3.0', stack: 'Rust', status: 'reference', note: '「写时积累」范式源——另立 LLM Wiki 知识库类型（51 号三问裁决不引入）' },
      { name: 'OntoFlow / AbutionGraph', license: '未标 LICENSE', stack: '—', status: 'reference', note: '二元本体模型概念参考；开源零实质推进，待可验证再评估（REQ-315）' },
      { name: 'EBI OLS4', license: 'Apache-2.0', stack: 'Java', status: 'reference', note: '本体检索服务标杆（287 本体 / 1084 万类）；资产栏语义地图与 ZOOMA 思路参考（23 号）' },
      { name: 'BioPortal / OBO Foundry / LOV', license: '各异', stack: '—', status: 'reference', note: '权威本体资源平台三源，「权威本体库引入」路径的来源面（23 号）' },
      { name: 'LangChain / LlamaIndex', license: 'MIT', stack: 'Python', status: 'reference', note: '作外部 MCP client 消费同一 facade——跨生态复用验证（P3）' },
      { name: 'Snowflake 本体方案', license: '商业为主', stack: '—', status: 'reference', note: 'Semantic Views / Cortex Agents 闭源商业为主，REQ-313 只立项不开发（74 号）' },
    ],
  },
]

/** 状态计数（源：docs/15 v2.35 七类主条目口径，含批次登记约数） */
const STATS = [
  { num: '60+', label: '调研登记项目', sub: 'docs/15 七类台账' },
  { num: '15', label: '生产在用', sub: '✅ 已集成 / 已用作' },
  { num: '25+', label: '论文与标准', sub: 'T1×10 · T2×6 · T3×9' },
  { num: '40+', label: '专项调研档', sub: 'platform-knowledge/08' },
]

// ---------------------------------------------------------------------------
// 关联图谱：中心项目 + 七簇（SVG 手绘，节点坐标手工布局）
// 节点样式：已集成 = 簇色实线框；引导/规划 = 虚线框；对照 = 灰点线框
// ---------------------------------------------------------------------------

const C = {
  core: '#3b82f6',      // 智能体核心：蓝
  build: '#8b5cf6',     // 构建校验：紫
  runtime: '#0ea5e9',   // 运行平面：青
  frontend: '#10b981',  // 可视化：绿
  source: '#f59e0b',    // 数据源：琥珀
  llm: '#ec4899',       // LLM 工程：粉
  compare: '#6b7280',   // 对照：灰
}

type GraphNode = { label: string; x: number; y: number; w: number; st: StatusKey }

type GraphCluster = {
  id: string; title: string; color: string;
  box: [number, number, number, number]; // x y w h
  hub: [number, number];                 // 连线起点（框边）
  tag: string; tagPos: [number, number]; // 线上状态标签
  nodes: GraphNode[];
}

const CLUSTERS: GraphCluster[] = [
  {
    id: 'core', title: '智能体平台核心（Go 栈）', color: C.core,
    box: [30, 26, 396, 196], hub: [228, 222], tag: '✅ 生产在用 ×5', tagPos: [300, 300],
    nodes: [
      { label: 'Eino + Eino ADK', x: 48, y: 62, w: 172, st: 'integrated' },
      { label: 'CloudWeGo Hertz', x: 238, y: 62, w: 170, st: 'integrated' },
      { label: 'mcp-go', x: 48, y: 94, w: 172, st: 'integrated' },
      { label: 'eino-ext', x: 238, y: 94, w: 170, st: 'integrated' },
      { label: 'jsonschema', x: 48, y: 126, w: 172, st: 'integrated' },
      { label: 'gonum', x: 238, y: 126, w: 170, st: 'planned' },
      { label: 'DeepSeek Harness', x: 48, y: 158, w: 172, st: 'guided' },
      { label: 'Tauri 2', x: 238, y: 158, w: 170, st: 'planned' },
    ],
  },
  {
    id: 'build', title: '本体构建与校验', color: C.build,
    box: [446, 26, 300, 196], hub: [596, 222], tag: '🔧 引导执行为主', tagPos: [596, 300],
    nodes: [
      { label: 'rdflib', x: 462, y: 62, w: 128, st: 'integrated' },
      { label: 'pySHACL', x: 602, y: 62, w: 128, st: 'integrated' },
      { label: 'Protégé', x: 462, y: 94, w: 128, st: 'guided' },
      { label: 'WebProtégé', x: 602, y: 94, w: 128, st: 'guided' },
      { label: 'ROBOT', x: 462, y: 126, w: 128, st: 'guided' },
      { label: 'OOPS!', x: 602, y: 126, w: 128, st: 'guided' },
      { label: 'ELK · HermiT', x: 462, y: 158, w: 128, st: 'guided' },
      { label: 'OWLAPY', x: 602, y: 158, w: 128, st: 'reference' },
    ],
  },
  {
    id: 'runtime', title: '运行平面引擎', color: C.runtime,
    box: [766, 26, 364, 196], hub: [948, 222], tag: '✅ 三引擎在用', tagPos: [872, 300],
    nodes: [
      { label: 'Oxigraph', x: 782, y: 62, w: 164, st: 'integrated' },
      { label: 'Jena Fuseki', x: 958, y: 62, w: 156, st: 'integrated' },
      { label: 'Qdrant', x: 782, y: 94, w: 164, st: 'integrated' },
      { label: 'Cayley', x: 958, y: 94, w: 156, st: 'planned' },
      { label: 'HelixDB', x: 782, y: 126, w: 164, st: 'reference' },
    ],
  },
  {
    id: 'frontend', title: '前端可视化与编辑器', color: C.frontend,
    box: [30, 610, 396, 200], hub: [228, 610], tag: '✅ 五件套在用', tagPos: [316, 516],
    nodes: [
      { label: 'ReactFlow', x: 48, y: 646, w: 172, st: 'integrated' },
      { label: 'CodeMirror 6', x: 238, y: 646, w: 170, st: 'integrated' },
      { label: '3d-force-graph', x: 48, y: 678, w: 172, st: 'integrated' },
      { label: 'WebVOWL', x: 238, y: 678, w: 170, st: 'integrated' },
      { label: 'react-diff-viewer', x: 48, y: 710, w: 172, st: 'integrated' },
      { label: 'Sigma.js · G6 · LodLive', x: 238, y: 710, w: 170, st: 'planned' },
    ],
  },
  {
    id: 'source', title: '权威资源与数据源', color: C.source,
    box: [446, 610, 264, 200], hub: [578, 610], tag: '🔭 权威来源', tagPos: [578, 516],
    nodes: [
      { label: 'EBI OLS4', x: 462, y: 646, w: 232, st: 'reference' },
      { label: 'BioPortal · OBO · LOV', x: 462, y: 678, w: 232, st: 'reference' },
      { label: 'HGNC · GO · HPO', x: 462, y: 710, w: 232, st: 'integrated' },
      { label: 'Ontology-Playground', x: 462, y: 742, w: 232, st: 'reference' },
    ],
  },
  {
    id: 'compare', title: '平台与范式对照（🔭）', color: C.compare,
    box: [730, 610, 400, 200], hub: [930, 610], tag: '🔭 思想借鉴 ×15', tagPos: [852, 516],
    nodes: [
      { label: 'semantica · Open Ontologies', x: 746, y: 646, w: 368, st: 'reference' },
      { label: 'OpenBKN · Helix Foundry', x: 746, y: 678, w: 368, st: 'reference' },
      { label: 'kubernetes-ontology · Kartograph', x: 746, y: 710, w: 368, st: 'reference' },
      { label: 'Graphify · OntoChat · OntoExtend', x: 746, y: 742, w: 368, st: 'reference' },
    ],
  },
  {
    id: 'sandbox', title: '沙箱与隔离', color: C.source,
    box: [944, 300, 186, 180], hub: [944, 400], tag: '🔭 候选', tagPos: [820, 400],
    nodes: [
      { label: 'gVisor', x: 958, y: 336, w: 156, st: 'reference' },
      { label: 'Kata Containers', x: 958, y: 368, w: 156, st: 'reference' },
      { label: 'E2B', x: 958, y: 400, w: 156, st: 'reference' },
      { label: 'Podman', x: 958, y: 432, w: 156, st: 'reference' },
    ],
  },
]

function nodeStyle(st: StatusKey, color: string): { stroke: string; dash?: string; opacity: number } {
  if (st === 'integrated') return { stroke: color, opacity: 1 }
  if (st === 'reference') return { stroke: '#9ca3af', dash: '2 2', opacity: 0.9 }
  return { stroke: color, dash: '5 3', opacity: 1 }
}

function EcoGraph() {
  return (
    <svg viewBox="0 0 1160 830" className="eco-svg" role="img" aria-label="开源项目关联图谱">
      {/* 连线（框 → 中心） */}
      {CLUSTERS.map((cl) => (
        <line key={`l-${cl.id}`} x1={cl.hub[0]} y1={cl.hub[1]} x2={cl.id === 'sandbox' ? 690 : cl.id === 'build' || cl.id === 'source' ? 580 : 480} y2={cl.id === 'sandbox' ? 400 : 410} stroke={cl.color} strokeWidth={2} opacity={0.55} />
      ))}
      {/* 连线状态标签 */}
      {CLUSTERS.map((cl) => (
        <g key={`t-${cl.id}`}>
          <rect x={cl.tagPos[0] - cl.tag.length * 5.4 - 8} y={cl.tagPos[1] - 11} width={cl.tag.length * 10.8 + 16} height={22} rx={11} fill="#fff" stroke={cl.color} strokeWidth={1} />
          <text x={cl.tagPos[0]} y={cl.tagPos[1] + 4} textAnchor="middle" fontSize={11.5} fill={cl.color} fontWeight={600}>{cl.tag}</text>
        </g>
      ))}

      {/* 中心节点 */}
      <g>
        <rect x={480} y={368} width={200} height={84} rx={14} fill="#1e293b" />
        <text x={580} y={404} textAnchor="middle" fontSize={14} fill="#fff" fontWeight={700}>eino-multiagent-lab</text>
        <text x={580} y={424} textAnchor="middle" fontSize={10.5} fill="#94a3b8">Go + Eino ADK 迷你版 Coze</text>
        <text x={580} y={440} textAnchor="middle" fontSize={10.5} fill="#94a3b8">本体 · 知识库 · 智能体</text>
      </g>

      {/* 簇 */}
      {CLUSTERS.map((cl) => (
        <g key={cl.id}>
          <rect x={cl.box[0]} y={cl.box[1]} width={cl.box[2]} height={cl.box[3]} rx={12} fill={cl.color} opacity={0.06} />
          <rect x={cl.box[0]} y={cl.box[1]} width={cl.box[2]} height={cl.box[3]} rx={12} fill="none" stroke={cl.color} strokeWidth={1} opacity={0.4} />
          <text x={cl.box[0] + 16} y={cl.box[1] + 24} fontSize={12.5} fill={cl.color} fontWeight={700}>{cl.title}</text>
          {cl.nodes.map((n) => {
            const s = nodeStyle(n.st, cl.color)
            return (
              <g key={n.label} opacity={s.opacity}>
                <rect x={n.x} y={n.y} width={n.w} height={26} rx={6} fill="#fff" stroke={s.stroke} strokeWidth={1.2} strokeDasharray={s.dash} />
                <circle cx={n.x + 11} cy={n.y + 13} r={3.2} fill={n.st === 'integrated' ? cl.color : n.st === 'reference' ? '#9ca3af' : 'none'} stroke={s.stroke} strokeWidth={1.2} strokeDasharray={n.st === 'integrated' ? 'none' : '2 1.5'} />
                <text x={n.x + 20} y={n.y + 17} fontSize={11} fill="#374151">{n.label}</text>
              </g>
            )
          })}
        </g>
      ))}

      {/* 图例（左中空白区） */}
      <g>
        <rect x={30} y={300} width={380} height={180} rx={12} fill="#f8fafc" stroke="#e2e8f0" />
        <text x={48} y={326} fontSize={12.5} fill="#334155" fontWeight={700}>图例 · 关联方式</text>
        <circle cx={56} cy={348} r={4} fill={C.core} />
        <text x={68} y={352} fontSize={11} fill="#475569">✅ 已集成 —— 生产在用，构成项目骨架</text>
        <circle cx={56} cy={372} r={4} fill="none" stroke={C.core} strokeDasharray="5 3" />
        <text x={68} y={376} fontSize={11} fill="#475569">🔧 📋 引导执行 / 规划集成 —— 走引导或已排期</text>
        <circle cx={56} cy={396} r={4} fill="none" stroke="#9ca3af" strokeDasharray="2 2" />
        <text x={68} y={400} fontSize={11} fill="#475569">🔭 对照参考 —— 不引代码，采纳思想与设计</text>
        <circle cx={56} cy={420} r={4} fill={C.compare} />
        <text x={68} y={424} fontSize={11} fill="#475569">簇色 = 技术领域；灰点线 = 仅对照不引入</text>
        <text x={48} y={452} fontSize={10.5} fill="#94a3b8">台账单源：docs/15《开源项目及论文登记簿》v2.35</text>
      </g>
    </svg>
  )
}

const GUIDES: { title: string; desc: string; hash: string }[] = [
  { title: '实现原理', desc: '机制设计、论文与代表性依据', hash: '#/principles' },
  { title: '整体架构', desc: '三平面 + 双代理总图与技术栈', hash: '#/architecture' },
  { title: '部署运行', desc: '安装包 / 服务端 / 本地三种方式', hash: '#/deploy' },
]

export default function EcosystemPage() {
  return (
    <main className="page">
      <div className="page-inner">
        {/* ---------- 页头 ---------- */}
        <div className="page-hero">
          <span className="eyebrow">Open Source Ecosystem</span>
          <h1>开源生态</h1>
          <p className="lead">
            这个平台不从零发明：调研过的 <strong>60+ 个开源项目</strong>构成它的骨架、参照系与方法论来源。
            下面的<strong>关联图谱</strong>梳理各项目与 AgentLab 的关联点，
            <strong>对比表</strong>按七类选取许可证、技术栈、关联状态等关键特征呈现异同。
            完整台账（含状态流转与变更记录）见{' '}
            <a href={`${REPO_URL}/blob/main/docs/15_开源项目及论文登记簿.md`} target="_blank" rel="noreferrer">docs/15 登记簿</a>，
            每个项目的深度调研档见{' '}
            <a href={`${REPO_URL}/tree/main/platform-knowledge`} target="_blank" rel="noreferrer">platform-knowledge/08_调研预研</a>。
          </p>
        </div>

        {/* ---------- 统计条 ---------- */}
        <section className="sec">
          <div className="eco-stats">
            {STATS.map((s) => (
              <div key={s.label} className="eco-stat">
                <div className="eco-stat-num">{s.num}</div>
                <div className="eco-stat-label">{s.label}</div>
                <div className="eco-stat-sub">{s.sub}</div>
              </div>
            ))}
          </div>
        </section>

        {/* ---------- 关联图谱 ---------- */}
        <section className="sec">
          <div className="sec-head">
            <span className="eyebrow">01 · Relation Graph</span>
            <h2>项目关联图谱</h2>
            <p>以 AgentLab 为中心，七个领域簇按主导关联方式连线：实线簇多为生产在用，虚线与灰点线簇为引导、规划与思想借鉴。</p>
          </div>
          <div className="diag-card">
            <EcoGraph />
          </div>
          <p className="diag-note">
            读法：中心为当前项目；簇框颜色区分技术领域，簇内节点左侧圆点标识关联状态（实心 = 已集成，虚线空心 = 引导 / 规划，灰色 = 仅对照）。
            收录口径：对话中调研、评估或深度参考过的开源项目，仅收「本体构建」与「智能体工程落地」两大重点（docs/15 收录闸门）。
          </p>
        </section>

        {/* ---------- 七簇对比表 ---------- */}
        <section className="sec">
          <div className="sec-head">
            <span className="eyebrow">02 · Comparison Table</span>
            <h2>关键特征对比表</h2>
            <p>按 docs/15 七类分组，选取许可证、技术栈、关联状态三项关键特征；「与本项目的关系」列为异同细节摘要，完整登记字段见 docs/15。</p>
          </div>
          {GROUPS.map((g) => (
            <div key={g.id} className="eco-group">
              <div className="eco-group-head">
                <h3>{g.title}</h3>
                <p>{g.desc}</p>
              </div>
              <div className="eco-table-wrap">
                <table className="eco-table">
                  <thead>
                    <tr>
                      <th style={{ width: 210 }}>项目</th>
                      <th style={{ width: 150 }}>许可证</th>
                      <th style={{ width: 110 }}>技术栈</th>
                      <th style={{ width: 130 }}>关联状态</th>
                      <th>与本项目的关系（借鉴 / 差异）</th>
                    </tr>
                  </thead>
                  <tbody>
                    {g.rows.map((r) => (
                      <tr key={r.name}>
                        <td className="eco-name">{r.name}</td>
                        <td>{r.license}</td>
                        <td>{r.stack}</td>
                        <td><Tag color={STATUS_META[r.status].color} style={{ marginInlineEnd: 0 }}>{STATUS_META[r.status].label}</Tag></td>
                        <td className="eco-note">{r.note}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </section>

        {/* ---------- 状态口径 ---------- */}
        <section className="sec">
          <div className="sec-head">
            <span className="eyebrow">03 · Status Vocabulary</span>
            <h2>状态口径</h2>
            <p>登记簿六状态枚举——同一项目随项目演进而流转（如 semantica 曾为「已集成」，D-O15 拍板后翻转为「对照参考」）。</p>
          </div>
          <div className="eco-vocab">
            {(Object.keys(STATUS_META) as StatusKey[]).map((k) => (
              <div key={k} className="eco-vocab-item">
                <Tag color={STATUS_META[k].color}>{STATUS_META[k].label}</Tag>
                <span>
                  {k === 'integrated' && '已进入生产依赖或作为数据基准直接使用'}
                  {k === 'guided' && '不进代码依赖，作为引导路线交给学习者在本机执行'}
                  {k === 'planned' && '已排期或已在需求池，待实施集成'}
                  {k === 'reference' && '深度调研后仅采纳思想 / 设计 / 素材，不引入代码'}
                  {k === 'removed' && '曾集成后移除，或调研后落选留档备查'}
                </span>
              </div>
            ))}
          </div>
        </section>

        {/* ---------- 延伸阅读 ---------- */}
        <section className="sec">
          <div className="sec-head" id="next"><span className="eyebrow">Next</span><h2>延伸阅读</h2></div>
          <div className="guide-grid">
            {GUIDES.map((g) => (
              <a key={g.hash} className="guide-card" href={g.hash}>
                <h3>{g.title}</h3>
                <p>{g.desc}</p>
                <span className="guide-arrow">→</span>
              </a>
            ))}
          </div>
        </section>
      </div>
    </main>
  )
}
