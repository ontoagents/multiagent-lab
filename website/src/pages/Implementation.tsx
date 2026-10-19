import { CompanionPipeline, QualityGateFlow, SseTimeline } from '../components/diagrams'
import { REPO_URL } from '../content'
import { useCopy, useLocale } from '../i18n'
import type { L10n } from '../locale'

// ---------------------------------------------------------------------------
// 关键实现页（REQ-238 扩展）：配置驱动装配 / SSE 可观测 / 质量门禁 / 伴生本体 /
// 可插拔抽象 / MCP 双向。图示均为内联 SVG（components/diagrams），
// 事实口径 = docs/02 §6/§7/§8 + docs/03/04 + 00_整体设计.md §4-§7。
// REQ-360：全页文案中英双份（zh 为事实源）；机制图内文字随语言切换。
// ---------------------------------------------------------------------------

const POINTS: { i18n: L10n<{ title: string; desc: string }> }[] = [
  {
    i18n: {
      zh: {
        title: '配置驱动装配',
        desc: '模型连接 → 智能体 → 项目三层配置全部存库（SQLite），每次运行按当前配置动态装配 Eino Agent——改配置下一次运行即生效，无需改代码重启。模型连接支持 openai_compat / anthropic 双协议（协议是连接属性，REQ-172/177）。',
      },
      en: {
        title: 'Config-driven assembly',
        desc: 'All three config layers — model connections → agents → projects — live in the database (SQLite); every run dynamically assembles the Eino agent from current config, so config changes take effect on the next run with no code change or restart. Model connections support dual protocols, openai_compat / anthropic (protocol is a connection attribute, REQ-172/177).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'SSE 过程完全可观测',
        desc: '对话执行中的工具调用、深度思考、子智能体委派、知识召回、本体查询均以来源着色的事件卡呈现；三档观测级别 + 模型级调用链 + 事件流重放（REQ-117），过程不再黑盒。',
      },
      en: {
        title: 'Fully observable SSE process',
        desc: 'Tool calls, deep thinking, sub-agent delegation, knowledge retrieval and ontology queries all render as source-colored event cards during execution; three observation levels + model-level call chains + event-stream replay (REQ-117) — no more black box.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'LLM 生成必过质量门禁',
        desc: '本体六条构建路径中所有 LLM 参与的生成产物，必须通过 qualitygate（11 检查项 + 三维评分）并支持自动修复；导入侧统一收口导入审查合并——收敛于资产段，杜绝低质量产物入库（REQ-171）。',
      },
      en: {
        title: 'LLM generations must pass the quality gate',
        desc: 'Across all six ontology build paths, anything LLM-generated must pass qualitygate (11 checks + three-dimension scoring) with auto-repair support; imports funnel through import review/merge — converging on the asset stage to keep low-quality artifacts out of the store (REQ-171).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '伴生本体（动态薄本体）',
        desc: '对话收尾旁路抽取知识候选，人工确认后入会话图并自动并入检索源；开关默认关、产物可摘除、失败静默降级——低侵入三原则的实例化（REQ-170）。',
      },
      en: {
        title: 'Companion ontology (dynamic thin ontology)',
        desc: 'A bypass extracts knowledge candidates as conversations wrap up; after human confirmation they enter the session graph and automatically join retrieval sources. Off by default, removable, silent degradation on failure — the three low-intrusion principles instantiated (REQ-170).',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: '可插拔抽象',
        desc: '执行后端（inprocess / docker / k8s）、推理后端（eino-adk / claude-code / opencode / aider / deepseek-harness，D-O13）、向量存储、图引擎均接口先行、实现可替换；开源优先，自研只保留装配层与路由薄层。',
      },
      en: {
        title: 'Pluggable abstractions',
        desc: 'Execution backends (inprocess / docker / k8s), reasoning backends (eino-adk / claude-code / opencode / aider / deepseek-harness, D-O13), vector stores and graph engines are all interface-first with replaceable implementations; open source first, with in-house code limited to thin assembly and routing layers.',
      },
    },
  },
  {
    i18n: {
      zh: {
        title: 'MCP 双向 + 诚实降级',
        desc: '消费侧：本体 facade 五工具挂给 Agent；供给侧：任意智能体一键 MCP 服务化（/mcp）被外部客户端直连。方案未运行、向量库不可用等场景明确提示并降级，主链路不受影响。',
      },
      en: {
        title: 'Bidirectional MCP + honest degradation',
        desc: 'Consumer side: the five ontology facade tools attach to any agent. Provider side: any agent can be exposed via MCP (/mcp) for direct external-client connections. When a plan isn’t running or the vector store is unavailable, the platform says so clearly and degrades — the main path is unaffected.',
      },
    },
  },
]

const COPY = {
  zh: {
    crumbsHome: '首页',
    title: '关键实现',
    tagline: '平台最值得了解的六组实现要点，配三张核心机制图：SSE 事件流、质量门禁与构建路径、伴生本体旁路管线。',
    sseTitle: 'SSE 事件流：对话过程完全可观测',
    sseNotePre: '事件顺序：',
    sseNoteMid: '。前端按事件来源着色渲染为可展开的事件卡，支持调试原始事件与导出重放。协议细节见 ',
    sseNoteLink: 'docs/02 §8',
    sseNotePost: '。',
    qgTitle: '六条构建路径收敛于质量门禁与资产段',
    qgNotePre: '设计要义：',
    qgNoteB: '差异只在生成段，收敛在资产段',
    qgNoteMid: '——无论手工向导、AI 对话、OO 双轨导入、由知识库构建还是 CSV 灌装，产物一律经 qualitygate（LLM 生成必过 + 自修复）进入统一的多形态资产仓库；版本、diff、审查合并、进化候选均作用于资产层。',
    compTitle: '伴生本体：对话驱动的动态薄本体',
    compNote:
      '与「先建本体再对话」的传统路径互补：在对话收尾时由旁路抽取候选，人工确认后进入会话图并自动并入 KG 检索源——本体随对话生长，而主链路零感知。',
    srcTitle: '源码入口',
    srcItems: [
      { href: 'backend/internal/chat', desc: ' —— Eino ADK 装配（inprocess + docker 分发）与 SSE 运行器' },
      { href: 'ontology-service', desc: ' —— 六路径 / qualitygate / spec_json 资产仓库' },
      { href: 'runtime-manager', desc: ' —— Oxigraph / Fuseki 方案编排与 MCP facade' },
      { href: 'backend/internal/companion', desc: ' —— 伴生本体旁路管线' },
      { href: 'web/src/pages/ontology', desc: ' —— 本体五栏前端（构建 / 资产 / 运行 / 消费审计）' },
    ],
  },
  en: {
    crumbsHome: 'Home',
    title: 'Key Implementations',
    tagline: 'Six implementation highlights worth knowing, with three core mechanism diagrams: the SSE event stream, the quality gate & build paths, and the companion-ontology bypass pipeline.',
    sseTitle: 'SSE event stream: fully observable conversations',
    sseNotePre: 'Event order: ',
    sseNoteMid: '. The frontend renders them as expandable, source-colored event cards with raw-event debugging and export & replay. Protocol details: ',
    sseNoteLink: 'docs/02 §8',
    sseNotePost: '.',
    qgTitle: 'Six build paths converge on the quality gate and asset stage',
    qgNotePre: 'Design essence: ',
    qgNoteB: 'differences live only in generation; everything converges on the asset stage',
    qgNoteMid: ' — manual wizard, AI chat, OO dual-track import, from-knowledge-base or CSV fill: all artifacts enter the unified multi-form asset store through qualitygate (mandatory for LLM generations, with self-repair); versions, diff, review merge and evolution candidates all act on the asset layer.',
    compTitle: 'Companion ontology: a conversation-driven dynamic thin ontology',
    compNote:
      'Complementary to the traditional "build the ontology first, then chat" path: a bypass extracts candidates as the conversation wraps up; after human confirmation they enter the session graph and automatically join KG retrieval sources — the ontology grows with the conversation while the main path stays oblivious.',
    srcTitle: 'Source code entry points',
    srcItems: [
      { href: 'backend/internal/chat', desc: ' — Eino ADK assembly (inprocess + docker dispatch) and the SSE runner' },
      { href: 'ontology-service', desc: ' — six paths / qualitygate / the spec_json asset store' },
      { href: 'runtime-manager', desc: ' — Oxigraph / Fuseki plan orchestration and the MCP facade' },
      { href: 'backend/internal/companion', desc: ' — the companion-ontology bypass pipeline' },
      { href: 'web/src/pages/ontology', desc: ' — the ontology five-tab frontend (build / assets / run / consumption & audit)' },
    ],
  },
}

export default function ImplementationPage() {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">{c.crumbsHome}</a> / {c.title}
        </div>
        <h1>{c.title}</h1>
        <p className="module-tagline">{c.tagline}</p>

        {/* 要点卡 */}
        <section className="sec">
          <div className="impl-grid">
            {POINTS.map((p, i) => (
              <div className="impl-card" key={i}>
                <div className="impl-index">{String(i + 1).padStart(2, '0')}</div>
                <div>
                  <div className="impl-title">{p.i18n[locale].title}</div>
                  <p className="impl-desc">{p.i18n[locale].desc}</p>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* SSE 时序 */}
        <section className="sec">
          <h2>{c.sseTitle}</h2>
          <div className="diag-card">
            <SseTimeline />
          </div>
          <p className="diag-note">
            {c.sseNotePre}
            <code>run.started → (skill.loaded) → agent.started → (retrieval) → message.delta* → (tool.call / tool.result | subagent.enter/exit | ontology.query)* → run.finished | run.error</code>
            {c.sseNoteMid}
            <a href={`${REPO_URL}/blob/main/docs/02_智能体_技术方案设计.md`} target="_blank" rel="noreferrer">
              {c.sseNoteLink}
            </a>
            {c.sseNotePost}
          </p>
        </section>

        {/* 质量门禁 */}
        <section className="sec">
          <h2>{c.qgTitle}</h2>
          <div className="diag-card">
            <QualityGateFlow />
          </div>
          <p className="diag-note">
            {c.qgNotePre}
            <b>{c.qgNoteB}</b>
            {c.qgNoteMid}
          </p>
        </section>

        {/* 伴生本体 */}
        <section className="sec">
          <h2>{c.compTitle}</h2>
          <div className="diag-card">
            <CompanionPipeline />
          </div>
          <p className="diag-note">{c.compNote}</p>
        </section>

        {/* 源码指路 */}
        <section className="sec">
          <h2>{c.srcTitle}</h2>
          <div className="md-body">
            <ul>
              {c.srcItems.map((s) => (
                <li key={s.href}>
                  <a href={`${REPO_URL}/tree/main/${s.href}`} target="_blank" rel="noreferrer">
                    {s.href}
                  </a>
                  {s.desc}
                </li>
              ))}
            </ul>
          </div>
        </section>
      </div>
    </main>
  )
}
