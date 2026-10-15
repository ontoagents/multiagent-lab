import { CompanionPipeline, QualityGateFlow, SseTimeline } from '../components/diagrams'
import { REPO_URL } from '../content'

// ---------------------------------------------------------------------------
// 关键实现页（REQ-238 扩展）：配置驱动装配 / SSE 可观测 / 质量门禁 / 伴生本体 /
// 可插拔抽象 / MCP 双向。图示均为内联 SVG（components/diagrams），
// 事实口径 = docs/02 §6/§7/§8 + docs/03/04 + 00_整体设计.md §4-§7。
// ---------------------------------------------------------------------------

const POINTS = [
  {
    title: '配置驱动装配',
    desc: '模型连接 → 智能体 → 项目三层配置全部存库（SQLite），每次运行按当前配置动态装配 Eino Agent——改配置下一次运行即生效，无需改代码重启。模型连接支持 openai_compat / anthropic 双协议（协议是连接属性，REQ-172/177）。',
  },
  {
    title: 'SSE 过程完全可观测',
    desc: '对话执行中的工具调用、深度思考、子智能体委派、知识召回、本体查询均以来源着色的事件卡呈现；三档观测级别 + 模型级调用链 + 事件流重放（REQ-117），过程不再黑盒。',
  },
  {
    title: 'LLM 生成必过质量门禁',
    desc: '本体六条构建路径中所有 LLM 参与的生成产物，必须通过 qualitygate（11 检查项 + 三维评分）并支持自动修复；导入侧统一收口导入审查合并——收敛于资产段，杜绝低质量产物入库（REQ-171）。',
  },
  {
    title: '伴生本体（动态薄本体）',
    desc: '对话收尾旁路抽取知识候选，人工确认后入会话图并自动并入检索源；开关默认关、产物可摘除、失败静默降级——低侵入三原则的实例化（REQ-170）。',
  },
  {
    title: '可插拔抽象',
    desc: '执行后端（inprocess / docker / k8s）、推理后端（eino-adk / claude-code / opencode / aider / deepseek-harness，D-O13）、向量存储、图引擎均接口先行、实现可替换；开源优先，自研只保留装配层与路由薄层。',
  },
  {
    title: 'MCP 双向 + 诚实降级',
    desc: '消费侧：本体 facade 五工具挂给 Agent；供给侧：任意智能体一键 MCP 服务化（/mcp）被外部客户端直连。方案未运行、向量库不可用等场景明确提示并降级，主链路不受影响。',
  },
]

export default function ImplementationPage() {
  return (
    <main className="page">
      <div className="page-inner">
        <div className="crumbs">
          <a href="#/">首页</a> / 关键实现
        </div>
        <h1>关键实现</h1>
        <p className="module-tagline">
          平台最值得了解的六组实现要点，配三张核心机制图：SSE 事件流、质量门禁与构建路径、伴生本体旁路管线。
        </p>

        {/* 要点卡 */}
        <section className="sec">
          <div className="impl-grid">
            {POINTS.map((p, i) => (
              <div className="impl-card" key={p.title}>
                <div className="impl-index">{String(i + 1).padStart(2, '0')}</div>
                <div>
                  <div className="impl-title">{p.title}</div>
                  <p className="impl-desc">{p.desc}</p>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* SSE 时序 */}
        <section className="sec">
          <h2>SSE 事件流：对话过程完全可观测</h2>
          <div className="diag-card">
            <SseTimeline />
          </div>
          <p className="diag-note">
            事件顺序：<code>run.started → (skill.loaded) → agent.started → (retrieval) → message.delta* → (tool.call / tool.result | subagent.enter/exit | ontology.query)* → run.finished | run.error</code>。
            前端按事件来源着色渲染为可展开的事件卡，支持调试原始事件与导出重放。协议细节见{' '}
            <a href={`${REPO_URL}/blob/main/docs/02_智能体_技术方案设计.md`} target="_blank" rel="noreferrer">docs/02 §8</a>。
          </p>
        </section>

        {/* 质量门禁 */}
        <section className="sec">
          <h2>六条构建路径收敛于质量门禁与资产段</h2>
          <div className="diag-card">
            <QualityGateFlow />
          </div>
          <p className="diag-note">
            设计要义：<b>差异只在生成段，收敛在资产段</b>——无论手工向导、AI 对话、OO 双轨导入、由知识库构建还是 CSV 灌装，
            产物一律经 qualitygate（LLM 生成必过 + 自修复）进入统一的多形态资产仓库；版本、diff、审查合并、进化候选均作用于资产层。
          </p>
        </section>

        {/* 伴生本体 */}
        <section className="sec">
          <h2>伴生本体：对话驱动的动态薄本体</h2>
          <div className="diag-card">
            <CompanionPipeline />
          </div>
          <p className="diag-note">
            与「先建本体再对话」的传统路径互补：在对话收尾时由旁路抽取候选，人工确认后进入会话图并自动并入 KG 检索源——
            本体随对话生长，而主链路零感知。
          </p>
        </section>

        {/* 源码指路 */}
        <section className="sec">
          <h2>源码入口</h2>
          <div className="md-body">
            <ul>
              <li><a href={`${REPO_URL}/tree/main/backend/internal/chat`} target="_blank" rel="noreferrer">backend/internal/chat</a> —— Eino ADK 装配（inprocess + docker 分发）与 SSE 运行器</li>
              <li><a href={`${REPO_URL}/tree/main/ontology-service`} target="_blank" rel="noreferrer">ontology-service</a> —— 六路径 / qualitygate / spec_json 资产仓库</li>
              <li><a href={`${REPO_URL}/tree/main/runtime-manager`} target="_blank" rel="noreferrer">runtime-manager</a> —— Oxigraph / Fuseki 方案编排与 MCP facade</li>
              <li><a href={`${REPO_URL}/tree/main/backend/internal/companion`} target="_blank" rel="noreferrer">backend/internal/companion</a> —— 伴生本体旁路管线</li>
              <li><a href={`${REPO_URL}/tree/main/web/src/pages/ontology`} target="_blank" rel="noreferrer">web/src/pages/ontology</a> —— 本体五栏前端（构建 / 资产 / 运行 / 消费审计）</li>
            </ul>
          </div>
        </section>
      </div>
    </main>
  )
}
