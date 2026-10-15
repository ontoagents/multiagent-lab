import { REPO_SLUG } from '../content'

// ---------------------------------------------------------------------------
// 实现原理页（REQ-238 官网）：机制如何工作 + 为什么这样设计 + 理论与实践依据
// 内容单源：提炼自 platform-knowledge/ 整体设计、五层视角（53~56）、本体（02~04）、
// 知识库（05）与 docs/15 开源项目及论文登记簿；本文只做"可读版"转述，台账以 15 号为准。
// ---------------------------------------------------------------------------

/** 五层视角：智能体执行模型的五个正交切面（源自 platform-knowledge/02_智能体/53~56） */
const LAYERS: { key: string; title: string; scope: string; points: string[] }[] = [
  {
    key: 'Model',
    title: 'Model · 模型层',
    scope: '连哪个模型、怎么采样',
    points: [
      '模型连接持久化，openai_compat / anthropic 双协议统一适配温度、MaxTokens 等采样参数',
      '推理后端可插拔（D-O13）：eino-adk / claude-code / opencode / aider / deepseek-harness 五种 Harness 后端，模型驱动的循环交给谁执行是可替换决策',
    ],
  },
  {
    key: 'Context',
    title: 'Context · 上下文层',
    scope: '模型每轮真正看到什么',
    points: [
      '预算三档（compact≈6k / standard≈24k / full），超预算先前缀摘要、再八段式压缩持久化跨轮复用',
      '工具结果剪枝（>4000 字符留头 2400 + 尾 800）；tool 轮次保全——悬空调用补"结果未知"，不破坏消息结构',
      '召回链：知识库命中与伴生本体三元组按序注入，retrieval 事件全程可观测',
    ],
  },
  {
    key: 'Harness',
    title: 'Harness · 执行层',
    scope: '怎么把事做安全、做扎实',
    points: [
      '8 步工具装配链：内置 → 技能 → 连接器 → 本体 facade → 项目文件 → 执行原语 → Hook 包装 → 审批包装',
      'HookChain 防护：SSRF 预检、危险命令模式拦截；审批三档（off / danger / all）+ 豁免清单，挂起请求随 checkpoint 持久化',
      '验证背压 verify_on_stop 与沙箱后端（inprocess / docker / k8s）构成"生成 → 校验 → 阻断"闭环',
    ],
  },
  {
    key: 'Loop',
    title: 'Loop · 循环层',
    scope: '跑多久、断了怎么办、花了多少',
    points: [
      'ReAct 迭代上限默认 25，超限即停可恢复；checkpoint 快照落库，进程重启后挂起运行可续',
      '定时续跑：调度状态入库（间隔 ≥1min、次数封顶），长期任务不依赖进程常驻',
      'todo.md 进度产物 + run 级 usage 统计，token 消耗与轮次可回放',
    ],
  },
  {
    key: 'Graph',
    title: 'Graph · 协作层',
    scope: '多个智能体怎么组队',
    points: [
      'agent_as_tool 为推荐协作路径：成员智能体包装为工具，协调者按"能力描述"路由',
      'subagent.enter / exit 事件记录嵌套轨迹；子智能体上下文隔离，起到"防火墙"语义',
      'transfer 控制权移交仅保留兼容位；协作与工作流编排配置全部在项目侧收敛',
    ],
  },
]

/** 本体工程机制（源自 platform-knowledge/04_本体/02~04 与整体设计） */
const ONTOLOGY: { title: string; desc: string; points: string[] }[] = [
  {
    title: 'spec_json 单一资产',
    desc: '仓库即唯一事实源（D-O1）：本体以 spec_json 形态落库，TTL / VOWL JSON / CSV 等都是它的导出视图；版本快照、diff 与 fork 溯源全部围绕这一份资产展开，运行方案只是它的"部署视图"。',
    points: [],
  },
  {
    title: '六路径殊途同归',
    desc: '手工向导、AI 对话（OntoChat 流程）、由知识库构建、导入外部本体、OntoExtend 扩展、权威本体库引入——六条构建路径最终走同一套校验循环、收敛到同一段资产能力，入口不同、门禁一致。',
    points: [],
  },
  {
    title: '质量门禁与修复环',
    desc: 'Spec.Validate 单一结构校验 → qualitygate 11 项三维评分 → LLM 生成走"双校验源"修复环，≤3 轮铁律；进化候选以 vN-cK 配对门控评估，质量回退超过阈值不予合入。',
    points: [],
  },
  {
    title: '多引擎运行平面',
    desc: 'Oxigraph（SPARQL 1.1，单二进制）与 Fuseki（OWL 推理）按方案编排启停；启动对账支持重启领养，逐本体图隔离装载（REQ-320），推理对账下限闸（REQ-329）保证"引擎三元组 ≥ 源"才放行。',
    points: [],
  },
  {
    title: 'MCP facade 消费',
    desc: '本体能力以 6 个只读 onto_* 工具（概念查询 / 实例枚举 / 邻居扩展 / SPARQL）经 MCP facade 暴露给智能体，只读白名单与图隔离保证消费侧不污染数据源。',
    points: [],
  },
  {
    title: '伴生本体旁路',
    desc: '对话中增量抽取候选三元组，人工确认后进入会话图，可并入检索源——旁路落地、默认关闭、随时可摘除，不侵入主链路。',
    points: [],
  },
]

/** 知识增强机制（源自 platform-knowledge/05_知识库/00） */
const KNOWLEDGE: { title: string; desc: string }[] = [
  {
    title: 'RAG 管道',
    desc: '固定长度 + 重叠切分 → 向量化 → TopK 检索注入，retrieval 事件打点来源；向量后端可插拔（Qdrant 主力，SQLite 余弦兜底），单机部署零外部依赖。',
  },
  {
    title: 'GraphRAG',
    desc: '索引时由 LLM 抽取知识图谱（失败回退规则抽取，degraded 不阻断导入）；实体 / 关系 / 主张三表带 chunk_id 溯源；检索时向量命中 → 图一跳扩展 → 关系与主张汇出，多跳问题优于纯向量召回。',
  },
  {
    title: 'LLM Wiki',
    desc: '写时合成为互链 Markdown 页面层，检索"读页不读片段"，零 embedding 依赖；与 RAG、GraphRAG 三类知识库并存互补，按语料形态选择。',
  },
]

/** 设计原则四条（源自 platform-knowledge/01_整体设计/00 §7） */
const PRINCIPLES = [
  {
    title: '开源优先',
    desc: '凡有成熟开源实现，不重复造轮子：Eino / Oxigraph / Fuseki / rdflib / Qdrant 等核心组件均为社区验证过的项目，自研代码聚焦胶水与编排。',
  },
  {
    title: '可插拔抽象',
    desc: '模型、推理后端、向量库、SPARQL 引擎、沙箱后端全部面向接口编程，换实现不动业务代码。',
  },
  {
    title: '诚实降级',
    desc: '依赖缺失时明确声明 degraded 并降级运行（如 Qdrant 不可用退 SQLite 余弦、KG 抽取失败回退规则抽取），绝不静默假装成功。',
  },
  {
    title: '低侵入扩展',
    desc: '伴生本体旁路、知识库导入、外部工具接入均遵循"可并行、可关闭、可摘除"，扩展不绑架主链路。',
  },
]

/** 标准与协议（第二册 T3 类） */
const STANDARDS: { name: string; by: string; use: string; url: string }[] = [
  { name: 'Model Context Protocol (MCP)', by: 'Anthropic · 2024', use: '智能体 ↔ 工具 / 本体 facade 的统一协议底座', url: 'https://modelcontextprotocol.io' },
  { name: 'RDF 1.1', by: 'W3C', use: '本体数据模型与序列化基础（Turtle）', url: 'https://www.w3.org/TR/rdf11-concepts/' },
  { name: 'OWL 2', by: 'W3C', use: '本体表达力来源（类 / 属性 / 公理），Fuseki 推理目标方言', url: 'https://www.w3.org/TR/owl2-overview/' },
  { name: 'SHACL', by: 'W3C', use: '约束校验与 qualitygate 结构类规则的依据', url: 'https://www.w3.org/TR/shacl/' },
  { name: 'SPARQL 1.1', by: 'W3C', use: '图查询语言，工作台与 MCP facade 的查询接口', url: 'https://www.w3.org/TR/sparql11-query/' },
  { name: 'PROV-O', by: 'W3C', use: '本体构建过程的来源追溯建模参考', url: 'https://www.w3.org/TR/prov-o/' },
]

/** 论文与文献（第二册 T1 / T3 代表条目，完整台账见 docs/15） */
const PAPERS: { name: string; meta: string; use: string; url: string }[] = [
  { name: 'ReAct: Synergizing Reasoning and Acting', meta: 'Yao et al. · arXiv 2210.03629 · 2022', use: '推理-行动交织范式，智能体运行循环（Loop）的理论原型', url: 'https://arxiv.org/abs/2210.03629' },
  { name: 'GraphRAG (From Local to Global)', meta: 'Microsoft Research · arXiv 2404.16130 · 2024', use: '知识图谱增强 RAG 的方法论源头，GraphRAG 知识库实现依据', url: 'https://arxiv.org/abs/2404.16130' },
  { name: 'OntoChat', meta: 'ESWC 2024 · arXiv 2403.05921', use: '对话式本体工程四步工作流（用户故事 → 能力问题 → 聚类 → 测试），"AI 对话构建"路径蓝本', url: 'https://arxiv.org/abs/2403.05921' },
  { name: 'Participatory Prompting for Ontology Construction', meta: 'arXiv 2408.15256 · 2024', use: '参与式提示设计，人机协作建本的交互策略', url: 'https://arxiv.org/abs/2408.15256' },
  { name: 'OntoExtend', meta: 'arXiv 2607.17963 · 2026', use: '需求驱动的三段式本体扩展（Retriever → Extender → Integrator），增量进化路径参照', url: 'https://arxiv.org/abs/2607.17963' },
  { name: 'Online Clustering-based Ontology Extension', meta: 'Frontiers in Big Data · 2024', use: '对话中自动聚簇生长本体的路线，伴生本体机制的理论支撑', url: 'https://www.frontiersin.org/articles/10.3389/fdata.2024.1463543/full' },
  { name: 'NeOn Methodology', meta: 'Suárez-Figueroa et al. · 2012', use: '协作式本体工程七阶段模型，多路径构建流程的方法论源头', url: 'https://link.springer.com/chapter/10.1007/978-3-642-24794-1_5' },
  { name: 'TOVE: Towards Enterprise Ontologies', meta: 'Gruninger & Fox · 1995', use: '能力问题（Competency Questions）评估法原始出处，CQ 测试的传统依据', url: 'https://doi.org/10.1007/BFb0026565' },
  { name: 'Ontology Development 101', meta: 'Noy & McGuinness · 2001', use: '本体建模入门基线，新手向导与文档措辞的参照', url: 'https://protege.stanford.edu/publications/ontology_development/ontology101.pdf' },
  { name: 'LLM Wiki（写时积累范式）', meta: 'A. Karpathy · gist · 2026', use: '"写时合成、读页不读片段"知识层范式，LLM Wiki 知识库类型依据', url: 'https://gist.github.com/karpathy/2364b8bcb5b1b5cf4d58' },
]

/** 已集成开源（第一册 ✅ 已集成 类，完整台账见 docs/15） */
const INTEGRATED: { name: string; license: string; use: string }[] = [
  { name: 'CloudWeGo Eino + Eino ADK', license: 'Apache-2.0', use: 'Go 智能体编排主骨架：组件抽象、图编排、ReAct / 多智能体运行时' },
  { name: 'CloudWeGo Hertz', license: 'Apache-2.0', use: 'backend HTTP 服务与 API 网关' },
  { name: 'mcp-go', license: 'MIT', use: 'MCP 协议 Go 实现，facade 与客户端连接的基座' },
  { name: 'eino-ext', license: 'Apache-2.0', use: 'MCP Client 组件与 claude 模型适配器（REQ-172）' },
  { name: 'rdflib (Python)', license: 'BSD-3', use: 'sidecar 进程：TTL 解析 / 图校验 / 导出三职责（D-O5 轻量路线）' },
  { name: 'pySHACL', license: 'Apache-2.0', use: 'SHACL 约束校验执行器' },
  { name: 'Oxigraph', license: 'Apache-2.0 / MIT', use: 'P1 唯一 SPARQL 引擎（pin v0.5.11），单二进制免部署' },
  { name: 'Apache Jena Fuseki', license: 'Apache-2.0', use: 'P2 OWL 推理引擎，方案化编排启停' },
  { name: 'Qdrant', license: 'Apache-2.0', use: '向量检索主后端（KB_VECTOR_BACKEND 可插拔）' },
  { name: 'ReactFlow (@xyflow/react)', license: 'MIT', use: '工作流编辑器画布（D-O9 择优定案）' },
  { name: 'CodeMirror 6', license: 'MIT', use: '代码 / SPARQL 编辑器' },
  { name: 'react-diff-viewer-continued', license: 'MIT', use: '本体版本 diff 可视化' },
  { name: '3d-force-graph / react-force-graph-3d', license: 'MIT', use: '三维力导向本体可视化（REQ-154 / 179）' },
  { name: 'WebVOWL', license: 'MIT', use: 'VOWL 视觉规范参考，owl2vowl 导出形态对齐' },
  { name: 'santhosh-tekuri/jsonschema', license: 'MIT', use: 'spec_json 结构校验（Spec.Validate 底层）' },
]

/** 借鉴思想的开源项目（第一册 🔭 类：未引入代码，采纳设计思想） */
const INSPIRED: { name: string; takeaway: string }[] = [
  { name: 'semantica（D-O15）', takeaway: '项目退役后按 D-O15 反转"去-semantica 化"，仍保留其三层架构与 GraphRAG / PROV-O 溯源思路作对照参考' },
  { name: 'OntoChat', takeaway: '对话式构建流程与开源 prompt 库可复用，REQ-82/103 的蓝本' },
  { name: 'OntoExtend', takeaway: '三段式扩展管线（检索 → 扩展 → 整合）映射到六路径之"OntoExtend"' },
  { name: 'Open Ontologies（65/66 号）', takeaway: '判词纪律与 plan-apply 工作流借鉴，2026-10-08 拍板摘除集成（D-O 编号登记）' },
  { name: 'Helix Foundry', takeaway: 'base+overlay+audit 三层编辑保护与结构确定性推断（REQ-292~295 批次）' },
  { name: 'kubernetes-ontology', takeaway: '边级溯源（K1）、AI 消费契约（K2）、canonicalId（K3）设计' },
  { name: 'Microsoft Ontology-Playground', takeaway: '定位同构的平台对照，验证"本体 + 智能体"产品方向的合理性' },
  { name: 'ontology-on-snowflake', takeaway: '本体质量基准分档（50/60/70/78.2%）为 qualitygate 评分提供量标依据（REQ-313）' },
  { name: 'Graphify', takeaway: '确定性解析优先 + LLM 兜底、边级可信度、增量更新策略' },
  { name: 'HelixDB', takeaway: 'pre-filter → ranking 检索管线借鉴' },
  { name: 'Kartograph', takeaway: '架构边界可执行化（G1）与统一写路径（G2）' },
  { name: 'LLM Wiki 生态', takeaway: '三问裁决不引入代码，采纳"写时积累"范式另立知识库类型' },
]

const GUIDES: { title: string; desc: string; hash: string }[] = [
  { title: '整体架构', desc: '三平面 + 双代理拓扑总图', hash: '#/architecture' },
  { title: '关键实现', desc: 'SSE 时序 / 质量门禁 / 伴生管线三张图', hash: '#/implementation' },
  { title: '平台知识体系', desc: '模块导读与领域事实源', hash: '#/module/design' },
]

function SectionTitle({ id, eyebrow, title, desc }: { id?: string; eyebrow: string; title: string; desc?: string }) {
  return (
    <div className="sec-head" id={id}>
      <span className="eyebrow">{eyebrow}</span>
      <h2>{title}</h2>
      {desc && <p>{desc}</p>}
    </div>
  )
}

export default function PrinciplesPage() {
  return (
    <main className="page">
      <div className="page-inner">
        {/* ---------- 页头 ---------- */}
        <div className="page-hero">
          <span className="eyebrow">Implementation Principles</span>
          <h1>实现原理</h1>
          <p className="lead">
            这一页回答三个问题：<strong>机制如何工作</strong>（五层视角与管线设计）、
            <strong>为什么这样设计</strong>（设计原则与取舍依据）、
            <strong>凭什么可信</strong>（标准、论文与开源项目的理论和实践依据）。
            完整推导过程见 <a href={`https://github.com/${REPO_SLUG}/tree/main/platform-knowledge`} target="_blank" rel="noreferrer">platform-knowledge/</a>
            ，开源与论文台账见 <a href={`https://github.com/${REPO_SLUG}/blob/main/docs/15_开源项目及论文登记簿.md`} target="_blank" rel="noreferrer">docs/15 登记簿</a>。
          </p>
        </div>

        {/* ---------- 一、智能体执行模型：五层视角 ---------- */}
        <section className="sec">
          <SectionTitle
            eyebrow="01 · Agent Execution"
            title="智能体执行模型：五层视角"
            desc="把「AgentLab 如何运行一个智能体」沿五个正交切面拆开看——每层有独立配置、独立事件契约与代码落点，单层可替换、可观测。"
          />
          <div className="layer-grid">
            {LAYERS.map((l) => (
              <article key={l.key} className="layer-card">
                <header>
                  <span className="layer-key">{l.key}</span>
                  <h3>{l.title}</h3>
                  <span className="layer-scope">{l.scope}</span>
                </header>
                <ul>
                  {l.points.map((p, i) => (
                    <li key={i}>{p}</li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
          <p className="sec-note">
            五层的运行时序统一由 SSE 事件流暴露（<code>run.started → agent.started → message.delta* → tool.call/tool.result → run.finished</code>），
            事件协议与代码地图见 <a href="#/implementation">关键实现</a> 页。
          </p>
        </section>

        {/* ---------- 二、本体工程机制 ---------- */}
        <section className="sec">
          <SectionTitle
            eyebrow="02 · Ontology Engineering"
            title="本体工程机制"
            desc="六条构建路径殊途同归：入口可以是对话、导入或向导，出口必然是同一份 spec_json 资产与同一套质量门禁。"
          />
          <div className="onto-grid">
            {ONTOLOGY.map((o) => (
              <article key={o.title} className="onto-card">
                <h3>{o.title}</h3>
                <p>{o.desc}</p>
                {o.points.length > 0 && (
                  <ul>
                    {o.points.map((p, i) => (
                      <li key={i}>{p}</li>
                    ))}
                  </ul>
                )}
              </article>
            ))}
          </div>
        </section>

        {/* ---------- 三、知识增强机制 ---------- */}
        <section className="sec">
          <SectionTitle
            eyebrow="03 · Knowledge Enhancement"
            title="知识增强机制"
            desc="三类知识库按语料形态并存互补：块级召回（RAG）、图结构多跳（GraphRAG）、页面级合成（LLM Wiki）。"
          />
          <div className="kb-grid">
            {KNOWLEDGE.map((k) => (
              <article key={k.title} className="kb-card">
                <h3>{k.title}</h3>
                <p>{k.desc}</p>
              </article>
            ))}
          </div>
        </section>

        {/* ---------- 四、设计原则 ---------- */}
        <section className="sec">
          <SectionTitle eyebrow="04 · Design Principles" title="设计原则" desc="贯穿全部模块取舍的四条铁律。" />
          <div className="principle-grid">
            {PRINCIPLES.map((p, i) => (
              <article key={p.title} className="principle-card">
                <span className="principle-num">{String(i + 1).padStart(2, '0')}</span>
                <h3>{p.title}</h3>
                <p>{p.desc}</p>
              </article>
            ))}
          </div>
        </section>

        {/* ---------- 五、理论与实践依据 ---------- */}
        <section className="sec">
          <SectionTitle
            eyebrow="05 · Evidence & References"
            title="理论与实践依据"
            desc="平台机制不是凭空发明：标准与协议给数据形态，论文给方法论，开源项目给工程实践。以下为代表性条目，完整台账（含七类分级、全部 25 篇论文）见 docs/15 登记簿。"
          />

          <h3 className="ref-sub">标准与协议</h3>
          <div className="ref-table">
            {STANDARDS.map((s) => (
              <a key={s.name} className="ref-row" href={s.url} target="_blank" rel="noreferrer">
                <span className="ref-name">{s.name}</span>
                <span className="ref-meta">{s.by}</span>
                <span className="ref-use">{s.use}</span>
              </a>
            ))}
          </div>

          <h3 className="ref-sub">论文与方法论</h3>
          <div className="ref-table">
            {PAPERS.map((p) => (
              <a key={p.name} className="ref-row" href={p.url} target="_blank" rel="noreferrer">
                <span className="ref-name">{p.name}</span>
                <span className="ref-meta">{p.meta}</span>
                <span className="ref-use">{p.use}</span>
              </a>
            ))}
          </div>

          <h3 className="ref-sub">
            已集成开源项目 <span className="badge badge-integrated">✅ 生产在用</span>
          </h3>
          <div className="ref-table">
            {INTEGRATED.map((o) => (
              <div key={o.name} className="ref-row">
                <span className="ref-name">{o.name}</span>
                <span className="ref-meta">{o.license}</span>
                <span className="ref-use">{o.use}</span>
              </div>
            ))}
          </div>

          <h3 className="ref-sub">
            借鉴思想的项目 <span className="badge badge-inspired">🔭 思想采纳</span>
          </h3>
          <div className="inspired-grid">
            {INSPIRED.map((o) => (
              <article key={o.name} className="inspired-card">
                <h4>{o.name}</h4>
                <p>{o.takeaway}</p>
              </article>
            ))}
          </div>

          <p className="sec-note">
            台账维护约定（<a href={`https://github.com/${REPO_SLUG}/blob/main/docs/15_开源项目及论文登记簿.md`} target="_blank" rel="noreferrer">docs/15</a>）：
            新项目 / 新论文一经调研即登记，七类分级 + 六状态流转；本页仅展示进入生产或影响设计的条目。
          </p>
        </section>

        {/* ---------- 延伸阅读 ---------- */}
        <section className="sec">
          <SectionTitle eyebrow="Next" title="延伸阅读" />
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
