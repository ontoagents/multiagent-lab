/**
 * 官网界面示意图组件（REQ-364）：纯内联 SVG 手绘高保真 UI mockup，
 * 与品牌视觉同源（#4f46e5 系，对齐 diagrams.tsx 视觉语言）。
 * 目的：首页产品预览——让访客不跑平台即可直观看到界面形态与核心交互。
 * 诚实边界：为示意图（layout 按真实界面结构绘制：全局导航六项、本体五栏、
 * SSE 事件轨迹等均取自 web/src 真实 UI），非运行截图；后续可无缝替换实拍图。
 * 图内文字为产品界面原文（中文单源，REQ-360 同导读档口径）；图外说明双语。
 */

import { useLocale } from '../i18n'
import type { L10n } from '../locale'

const INK = '#1c2130'
const INK2 = '#575e75'
const INK3 = '#8a90ad'
const LINE = '#e3e6f0'
const BRAND = '#4f46e5'
const BRAND_BG = '#eef0ff'

/** 浏览器窗框 chrome：三色点 + 地址栏 */
function Chrome() {
  return (
    <g>
      <rect x="0" y="0" width="1200" height="740" rx="14" fill="#ffffff" stroke={LINE} />
      <path d="M0 14 A14 14 0 0 1 14 0 H1186 A14 14 0 0 1 1200 14 V40 H0 Z" fill="#f2f4fb" />
      <line x1="0" y1="40" x2="1200" y2="40" stroke={LINE} />
      <circle cx="24" cy="20" r="6" fill="#ff5f57" />
      <circle cx="44" cy="20" r="6" fill="#febc2e" />
      <circle cx="64" cy="20" r="6" fill="#28c840" />
      <rect x="96" y="9" width="330" height="22" rx="11" fill="#ffffff" stroke={LINE} />
      <text x="112" y="21" fontSize="11" fill={INK3}>localhost:8080</text>
    </g>
  )
}

/** 全局导航（web/src/components/GlobalNav 真实六项） */
function GlobalNavMock({ active }: { active: string }) {
  const items = ['智能体', '项目', '本体', '知识库', '技能', '平台知识']
  return (
    <g>
      <rect x="0" y="40" width="152" height="700" fill="#f9fafe" />
      <line x1="152" y1="40" x2="152" y2="740" stroke={LINE} />
      {/* 品牌 */}
      <g transform="translate(20, 62)">
        <circle cx="6" cy="0" r="4.5" fill={BRAND} />
        <circle cx="0" cy="10" r="4" fill={BRAND} opacity="0.86" />
        <circle cx="12" cy="10" r="4" fill={BRAND} opacity="0.86" />
        <path d="M6 2 L1 9 M6 2 L11 9 M2 10 H10" stroke={BRAND} strokeWidth="1.5" fill="none" />
        <text x="24" y="8" fontSize="13" fontWeight={800} fill={INK}>AgentLab</text>
      </g>
      {items.map((label, i) => {
        const y = 108 + i * 38
        const on = label === active
        return (
          <g key={label}>
            {on && <rect x="10" y={y - 15} width="132" height="30" rx="8" fill={BRAND_BG} />}
            <rect x="22" y={y - 7} width="14" height="14" rx="4" fill={on ? BRAND : '#c3c8dd'} />
            <text x="44" y={y + 1} fontSize="12.5" fontWeight={on ? 700 : 400} fill={on ? BRAND : INK2}>
              {label}
            </text>
          </g>
        )
      })}
      <rect x="22" y="676" width="14" height="14" rx="4" fill="#c3c8dd" />
      <text x="44" y="687" fontSize="12.5" fill={INK2}>设置</text>
    </g>
  )
}

// ---------------------------------------------------------------------------
// 1. 对话工作台：会话流 + 事件轨迹面板（结构取自 web/src/pages/agents 真实布局）
// ---------------------------------------------------------------------------

/** 消息区事件卡（来源着色左边条） */
function EventCard({ x, y, w, tint, icon, title, chips }: {
  x: number; y: number; w: number; tint: string; icon: string; title: string; chips: string[]
}) {
  return (
    <g>
      <rect x={x} y={y} width={w} height={30 + chips.length * 24 + 10} rx="9" fill="#fbfbfe" stroke={LINE} />
      <rect x={x} y={y} width="4" height={30 + chips.length * 24 + 10} rx="2" fill={tint} />
      <text x={x + 16} y={y + 19} fontSize="12" fontWeight={700} fill={INK}>
        {icon} {title}
      </text>
      {chips.map((chip, i) => (
        <g key={chip}>
          <rect x={x + 16} y={y + 30 + i * 24} width={chip.length * 6.2 + 22} height="18" rx="9" fill="#ffffff" stroke={LINE} />
          <circle cx={x + 27} cy={y + 39 + i * 24} r="3.5" fill={tint} />
          <text x={x + 35} y={y + 39 + i * 24} fontSize="10.5" fill={INK2}>{chip}</text>
        </g>
      ))}
    </g>
  )
}

export function ChatWorkbenchMock() {
  const { locale } = useLocale()
  const caption: L10n<string> = {
    zh: '对话工作台：左侧全局导航与会话列表，中间流式消息 + 来源着色事件卡，右侧调用轨迹面板全程可观测。',
    en: 'Chat workbench: global nav & sessions on the left, streaming messages with source-colored event cards in the middle, and a fully observable trace panel on the right.',
  }
  const trace: { color: string; label: string; time: string }[] = [
    { color: INK3, label: 'run.started', time: '00:00.0' },
    { color: '#16a34a', label: 'retrieval · 知识库命中 ×3', time: '00:01.2' },
    { color: '#2563eb', label: 'tool.call · defect_query', time: '00:02.8' },
    { color: '#7c3aed', label: 'subagent.enter · ontology-builder', time: '00:05.1' },
    { color: INK3, label: 'message.delta ×128', time: '00:06.0+' },
    { color: '#ea580c', label: 'approval · 等待确认', time: '00:09.4' },
  ]
  return (
    <figure className="mock-fig">
      <svg viewBox="0 0 1200 740" role="img" className="mock-svg">
        <Chrome />
        <GlobalNavMock active="智能体" />
        {/* 会话列表 */}
        <g>
          <rect x="152" y="40" width="208" height="700" fill="#ffffff" />
          <line x1="360" y1="40" x2="360" y2="740" stroke={LINE} />
          <rect x="166" y="56" width="180" height="30" rx="8" fill={BRAND} />
          <text x="256" y="75" fontSize="12" fontWeight={700} fill="#ffffff" textAnchor="middle">＋ 新建对话</text>
          {[
            { t: '缺陷本体构建', s: '12 概念 · 进行中', on: true },
            { t: '生产排产问答', s: '昨天', on: false },
            { t: '设备巡检周报总结', s: '周二', on: false },
            { t: 'orgs 本体查询', s: '周一', on: false },
            { t: '新会话', s: '10-12', on: false },
          ].map((it) => (
            <g key={it.t}>
              <rect x={it.on ? 164 : 166} y={it.on ? 100 : 102} width={it.on ? 192 : 188} height="44" rx="9" fill={it.on ? BRAND_BG : 'transparent'} />
              <text x="180" y={it.on ? 118 : 120} fontSize="12" fontWeight={it.on ? 700 : 500} fill={it.on ? BRAND : INK}>{it.t}</text>
              <text x="180" y={it.on ? 134 : 136} fontSize="10.5" fill={INK3}>{it.s}</text>
            </g>
          ))}
        </g>
        {/* 消息流 */}
        <g>
          <line x1="360" y1="40" x2="360" y2="740" stroke={LINE} />
          <text x="384" y="68" fontSize="13.5" fontWeight={700} fill={INK}>缺陷本体构建</text>
          <rect x="700" y="52" width="86" height="22" rx="11" fill="#f2f4fb" />
          <circle cx="714" cy="63" r="3.5" fill="#16a34a" />
          <text x="724" y="67" fontSize="10.5" fill={INK2}>观测 · standard</text>
          {/* 用户气泡 */}
          <rect x="560" y="92" width="286" height="40" rx="12" fill={BRAND} />
          <text x="576" y="109" fontSize="12" fill="#ffffff">帮我把本周的设备缺陷记录整理</text>
          <text x="576" y="125" fontSize="12" fill="#ffffff">成本体初稿</text>
          {/* AI 文本 */}
          <text x="384" y="158" fontSize="12" fill={INK2}>好的，我将从缺陷记录中抽取概念与关系，先检索知识库…</text>
          <EventCard x={384} y={176} w={300} tint="#16a34a" icon="🔍" title="知识库检索" chips={['设备缺陷周报.pdf ×3', '缺陷分类标准.docx ×1']} />
          <EventCard x={384} y={280} w={300} tint="#7c3aed" icon="🤖" title="委派 · ontology-builder" chips={['生成概念与关系候选', '隔离上下文运行']} />
          {/* 结果卡 */}
          <rect x="384" y="392" width="470" height="118" rx="9" fill="#fbfbfe" stroke={LINE} />
          <rect x="384" y="392" width="4" height="118" rx="2" fill={BRAND} />
          <text x="402" y="416" fontSize="12.5" fontWeight={700} fill={INK}>✅ 本体初稿已生成（草稿）</text>
          <text x="402" y="438" fontSize="11.5" fill={INK2}>12 个概念 · 18 条关系 · 来源 2 份文档</text>
          <rect x="402" y="452" width="380" height="8" rx="4" fill="#e9ebf5" />
          <rect x="402" y="452" width="332" height="8" rx="4" fill={BRAND} opacity="0.75" />
          <text x="402" y="480" fontSize="10.5" fill={INK3}>质量门禁评分 87 / 100 · 待人工确认后入资产库</text>
          <rect x="744" y="462" width="94" height="26" rx="13" fill={BRAND_BG} stroke={BRAND} />
          <text x="791" y="479" fontSize="11" fontWeight={700} fill={BRAND} textAnchor="middle">查看详情</text>
          {/* 输入框 */}
          <rect x="384" y="656" width="470" height="52" rx="13" fill="#ffffff" stroke={LINE} />
          <text x="404" y="678" fontSize="11.5" fill={INK3}>继续追问，或输入 / 唤起技能…</text>
          <circle cx="828" cy="682" r="15" fill={BRAND} />
          <path d="M822 682 h12 M830 676 l6 6 -6 6" stroke="#ffffff" strokeWidth="1.8" fill="none" strokeLinecap="round" />
        </g>
        {/* 调用轨迹面板 */}
        <g>
          <rect x="870" y="40" width="330" height="700" fill="#fbfbfe" />
          <line x1="870" y1="40" x2="870" y2="740" stroke={LINE} />
          <text x="894" y="72" fontSize="13" fontWeight={700} fill={INK}>调用轨迹</text>
          <rect x="1130" y="56" width="52" height="21" rx="10" fill="#ffffff" stroke={LINE} />
          <text x="1156" y="70" fontSize="10" fill={INK2} textAnchor="middle">导出</text>
          <line x1="902" y1="104" x2="902" y2="600" stroke="#d9dcea" strokeWidth="2" />
          {trace.map((t, i) => {
            const y = 118 + i * 82
            return (
              <g key={t.label}>
                <circle cx="902" cy={y} r="6" fill="#ffffff" stroke={t.color} strokeWidth="2.5" />
                <text x="920" y={y - 4} fontSize="11.5" fontWeight={600} fill={INK}>{t.label}</text>
                <text x="920" y={y + 12} fontSize="10" fill={INK3}>{t.time}</text>
              </g>
            )
          })}
          <circle cx="902" cy="618" r="6" fill="#16a34a" />
          <text x="920" y="614" fontSize="11.5" fontWeight={600} fill={INK}>run.finished</text>
          <text x="920" y="630" fontSize="10" fill={INK3}>00:12.3 · tokens 18.4k</text>
        </g>
      </svg>
      <figcaption className="mock-cap">{caption[locale]}</figcaption>
    </figure>
  )
}

// ---------------------------------------------------------------------------
// 2. 本体工作台：五栏页签 + 六路径构建卡 + 质量门禁面板（结构取自 ontology 五栏）
// ---------------------------------------------------------------------------

export function OntologyConsoleMock() {
  const { locale } = useLocale()
  const caption: L10n<string> = {
    zh: '本体工作台：五栏页签覆盖 构建 → 资产 → 运行 → 消费与审计 → 学习中心；构建栏六路径入口，右侧质量门禁实时评分。',
    en: 'Ontology workbench: five tabs cover build → assets → run → consume & audit → learning; six build-path entries with a live quality-gate panel on the right.',
  }
  const tabs = [
    { t: '本体构建', on: true },
    { t: '本体资产', on: false },
    { t: '本体运行', on: false },
    { t: '消费与审计', on: false },
    { t: '学习中心', on: false },
  ]
  const paths = [
    { icon: '📝', t: '手工向导', s: '表单式结构化建模' },
    { icon: '💬', t: 'AI 对话构建', s: 'OntoChat 四步流程' },
    { icon: '📚', t: '由知识库构建', s: '从文档语料抽取' },
    { icon: '📦', t: '导入外部本体', s: 'TTL / OWL 导入审查' },
    { icon: '🧬', t: 'OntoExtend 扩展', s: '需求驱动增量进化' },
    { icon: '🏛️', t: '权威本体库', s: '领域权威本体引入' },
  ]
  const gates = [
    { t: '结构完整性', v: 92 },
    { t: '语义一致性', v: 85 },
    { t: '来源可溯性', v: 88 },
  ]
  return (
    <figure className="mock-fig">
      <svg viewBox="0 0 1200 740" role="img" className="mock-svg">
        <Chrome />
        <GlobalNavMock active="本体" />
        {/* 页头 + 五栏页签 */}
        <g>
          <line x1="152" y1="40" x2="152" y2="740" stroke={LINE} />
          <text x="184" y="72" fontSize="14" fontWeight={800} fill={INK}>缺陷管理本体</text>
          <text x="300" y="72" fontSize="11.5" fill={INK3}>v3 · 草稿 → 审查中</text>
          <rect x="1010" y="52" width="86" height="26" rx="13" fill={BRAND_BG} stroke={BRAND} />
          <text x="1053" y="69" fontSize="11" fontWeight={700} fill={BRAND} textAnchor="middle">质量 87</text>
          <rect x="1104" y="52" width="60" height="26" rx="13" fill="#f2f4fb" />
          <text x="1134" y="69" fontSize="11" fill={INK2} textAnchor="middle">导出</text>
          {tabs.map((tab, i) => {
            const x = 168 + i * 130
            return (
              <g key={tab.t}>
                <text x={x + 8} y="112" fontSize="12.5" fontWeight={tab.on ? 700 : 400} fill={tab.on ? BRAND : INK2}>{tab.t}</text>
                {tab.on && <rect x={x} y="122" width="112" height="3" rx="1.5" fill={BRAND} />}
              </g>
            )
          })}
          <line x1="152" y1="130" x2="1200" y2="130" stroke={LINE} />
        </g>
        {/* 六路径卡 */}
        <g>
          <text x="184" y="166" fontSize="12.5" fontWeight={700} fill={INK}>选择构建路径</text>
          {paths.map((p, i) => {
            const col = i % 3
            const row = Math.floor(i / 3)
            const x = 184 + col * 220
            const y = 184 + row * 128
            return (
              <g key={p.t}>
                <rect x={x} y={y} width="204" height="112" rx="11" fill="#ffffff" stroke={LINE} />
                <circle cx={x + 34} cy={y + 34} r="17" fill={BRAND_BG} />
                <text x={x + 34} y={y + 39} fontSize="15" textAnchor="middle">{p.icon}</text>
                <text x={x + 62} y={y + 39} fontSize="13" fontWeight={700} fill={INK}>{p.t}</text>
                <text x={x + 18} y={y + 74} fontSize="11" fill={INK2}>{p.s}</text>
                <text x={x + 18} y={y + 94} fontSize="10.5" fill={BRAND}>进入 →</text>
              </g>
            )
          })}
        </g>
        {/* 质量门禁面板 */}
        <g>
          <rect x="856" y="184" width="308" height="400" rx="12" fill="#fbfbfe" stroke={LINE} />
          <text x="880" y="216" fontSize="13" fontWeight={700} fill={INK}>质量门禁</text>
          <rect x="1076" y="198" width="64" height="24" rx="12" fill="#e8f7ee" />
          <text x="1108" y="214" fontSize="10.5" fontWeight={700} fill="#16a34a" textAnchor="middle">11/11 通过</text>
          {gates.map((g, i) => (
            <g key={g.t}>
              <text x="880" y={256 + i * 64} fontSize="11.5" fill={INK2}>{g.t}</text>
              <rect x="880" y={264 + i * 64} width="240" height="9" rx="4.5" fill="#e9ebf5" />
              <rect x="880" y={264 + i * 64} width={240 * (g.v / 100)} height="9" rx="4.5" fill={BRAND} opacity="0.8" />
              <text x="1136" y={272 + i * 64} fontSize="11.5" fontWeight={700} fill={INK} textAnchor="end">{g.v}</text>
            </g>
          ))}
          <rect x="880" y="462" width="260" height="76" rx="9" fill="#f4f2ff" />
          <text x="896" y="488" fontSize="11" fontWeight={700} fill={BRAND}>🛡️ LLM 生成必过门禁</text>
          <text x="896" y="508" fontSize="10.5" fill={INK2}>自动修复 ≤3 轮 · 三维评分 · 不通过不入库</text>
          <text x="896" y="526" fontSize="10.5" fill={INK3}>低质量产物收敛于资产段之外</text>
        </g>
        {/* 底部管线 */}
        <g>
          <rect x="184" y="608" width="832" height="88" rx="12" fill="#ffffff" stroke={LINE} />
          <text x="208" y="640" fontSize="12" fontWeight={700} fill={INK}>资产管线</text>
          {['生成', '校验', '修复', '审查合并', '入库'].map((s, i) => (
            <g key={s}>
              <rect x={330 + i * 132} y={626} width="96" height="30" rx="15" fill={i === 4 ? '#e8f7ee' : BRAND_BG} />
              <text x={378 + i * 132} y={645} fontSize="11.5" fontWeight={600} fill={i === 4 ? '#16a34a' : BRAND} textAnchor="middle">{s}</text>
              {i < 4 && <path d={`M${432 + i * 132} 641 h22 m-6 -5 6 5 -6 5`} stroke={INK3} strokeWidth="1.5" fill="none" strokeLinecap="round" />}
            </g>
          ))}
          <text x="208" y="676" fontSize="10.5" fill={INK3}>六路径差异只在生成段，收敛在资产段：版本 · diff · 进化候选 · 被引用聚合</text>
        </g>
      </svg>
      <figcaption className="mock-cap">{caption[locale]}</figcaption>
    </figure>
  )
}
