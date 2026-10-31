import { Fragment, useEffect, useMemo, useState } from 'react'
import XMarkdown from '@ant-design/x-markdown'
import { Alert, Button, Card, Collapse, Menu, Progress, Skeleton, Space, Tag, Typography } from 'antd'
import {
  BookOutlined,
  CheckCircleOutlined,
  CloudServerOutlined,
  RightOutlined,
} from '@ant-design/icons'
// REQ-505/M313：方法论五卡全文单源（seeds/learning/methodology/——?raw 编译期内联，修 body/deep 内联副本与 md 双源漂移）
import MD_CQ from '../../../../seeds/learning/methodology/01-competency-questions.md?raw'
import MD_DOMAIN from '../../../../seeds/learning/methodology/02-domain-analysis.md?raw'
import MD_REUSE from '../../../../seeds/learning/methodology/03-reuse-vocabularies.md?raw'
import MD_NAMING from '../../../../seeds/learning/methodology/04-naming-and-uri.md?raw'
import MD_OWL from '../../../../seeds/learning/methodology/05-owl-patterns.md?raw'
import { api } from '../../api/client'
import { useUI } from '../../store/ui'
import { STAGE_DEFS } from './shared'
import { parseOntoHash, writeOntoHash } from './shared/ontoRoute'
import PipelinePane from './components/PipelinePane'
import QuizPane from './learn/QuizPane'
import type { Ontology } from '../../api/types'

// ---------------------------------------------------------------------------
// 学习中心（LearnPage，REQ-104 ①，默认页）：本体模块 = 学习各种本体构建、运行方式的模块
//   七阶段学习路径主视图（方法论卡片 + 任务卡打卡 + 工具入口跳转）
//   构建方式对照（五路径卡）| 运行方式对照（引擎差异卡）
//   打卡：checklist `task:` key 空间（REQ-76 机制复用；localStorage P1 尾实现）
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 方法论卡片（REQ-90 五模块，v0.2 深度版：body 精简骨架 + deep 深度增量；全文见 seeds/learning/methodology/）
// REQ-139：要点结构化渲染——按 ①②③/序号/句读自动分段为要点列表（排版降噪），全部卡片统一受益
// REQ-270 修复：序号标记的前导符扩为「。；;：:」（原文「三个技法：①」的 ① 前是「：」不命中），
// 且改用带捕获组的 split——marker 与文本天然逐段对齐，不再出现「；②文本」式的错位 marker。
// ---------------------------------------------------------------------------

const METHODOLOGY: { key: string; stage: string; title: string; tag: string; raw: string }[] = [
  { key: 'cq', stage: 's1', title: '能力问题法（Competency Questions）', tag: '问题驱动', raw: MD_CQ },
  { key: 'domain', stage: 's1', title: '领域分析与概念抽取', tag: '方法论', raw: MD_DOMAIN },
  { key: 'reuse', stage: 's2', title: '复用优先：词表与顶层本体', tag: '复用', raw: MD_REUSE },
  { key: 'naming', stage: 's2', title: '命名与 URI 规范', tag: '规范', raw: MD_NAMING },
  { key: 'owl', stage: 's3', title: '常用 OWL 模式五则', tag: '进阶', raw: MD_OWL },
]


// ---------------------------------------------------------------------------
// 任务卡（REQ-91 结构：阶段/标题/目标/前置/步骤/验收问题/难度；`task:` 打卡）
// ---------------------------------------------------------------------------

interface TaskCardDef {
  id: string
  stage: string
  title: string
  goal: string
  prereq: string[]
  steps: string[]
  acceptance: string
  link: { text: string; sidebar: 'build' | 'assets' | 'runtime' ; s1tab?: string }
  difficulty: number
}

const TASKS: TaskCardDef[] = [
  {
    id: 'task_s1_first_ontology',
    stage: 's1',
    title: '建立第一个本体（每条来源路径都走一遍）',
    goal: '体会 S1 每条来源路径的差异与产物形态',
    prereq: [],
    steps: ['创建内置示例（seed-sample）', '用「AI 创建」生成一个小领域草稿并确认入库', '粘贴一段 TTL 走「导入文件」（观察导入报告与有损警告）'],
    acceptance: '每条路径的产物分别是什么形态？哪条有损、为什么？',
    link: { text: '前往自定义构建 S1', sidebar: 'build' },
    difficulty: 1,
  },
  {
    id: 'task_s2_model_cq',
    stage: 's2',
    title: '从 CQ 到 Spec：为一个问题建模',
    goal: '体验"问题驱动建模"',
    prereq: ['task_s1_first_ontology'],
    steps: ['写下 3 条能力问题', '在 Spec 编辑器为每条 CQ 补概念与关系', '保存并观察版本号递增'],
    acceptance: '你的哪条 CQ 需要跨概念的关系才能回答？',
    link: { text: '前往录入 CQ 与建模（构建栏 AI 创建）', sidebar: 'build', s1tab: 'ai' }, // REQ-248/M71：直达 CQ 录入点
    difficulty: 1,
  },
  {
    id: 'task_s3_break_references',
    stage: 's3',
    title: '故意制造一次校验失败',
    goal: '理解引用完整性校验拦住了什么',
    prereq: ['task_s2_model_cq'],
    steps: ['在 Spec 里让某关系指向未定义的概念', '尝试保存（观察校验错误表）', '修正后重新保存'],
    acceptance: '校验在构建平面还是运行平面执行？为什么不允许保存坏本体？',
    link: { text: '前往本体资产校验', sidebar: 'assets' },
    difficulty: 1,
  },
  {
    id: 'task_s4_read_graph',
    stage: 's4',
    title: '读图：从可视化找一条继承链',
    goal: '用 S4 图谱理解层次结构',
    prereq: ['task_s2_model_cq'],
    steps: ['打开资产详情「可视化」', '找到最深的一条继承链', '点击节点查看实例数徽标'],
    acceptance: '虚线与实线分别代表什么？',
    link: { text: '前往本体资产可视化', sidebar: 'assets' },
    difficulty: 1,
  },
  {
    id: 'task_s5_run_profile',
    stage: 's5',
    title: '把本体部署为运行方案',
    goal: '理解"构建/运行解耦"与显式重载',
    prereq: ['task_s3_break_references'],
    steps: ['在运行栏新建方案（勾选本体）', '启动并确认 running', '回资产栏保存一个新版本，再回方案页执行「重载」'],
    acceptance: '为什么仓库更新后方案不会自动生效？（REQ-87 解耦语义）',
    link: { text: '前往本体运行', sidebar: 'runtime' },
    difficulty: 2,
  },
  {
    id: 'task_s6_facade',
    stage: 's6',
    title: '认识 facade：4 个固定工具',
    goal: '理解统一 MCP 契约对所有运行时类型一致',
    prereq: ['task_s5_run_profile'],
    steps: ['打开方案详情「facade 与工具」', '阅读 4 个 onto_* 工具签名', '展开「翻译透视」看一次调用的 SPARQL 原文'],
    acceptance: 'onto_neighbors 返回什么？翻译透视图里 SPARQL 是谁生成的？',
    link: { text: '前往方案详情', sidebar: 'runtime' },
    difficulty: 2,
  },
  {
    id: 'task_s7_agent_query',
    stage: 's7',
    title: '让 Agent 用本体回答跨概念问题',
    goal: '验证对话挂载与查询全链路',
    prereq: ['task_s6_facade'],
    steps: ['在对话输入框「本体增强」chip 选择 running 方案', '提问一个需要 neighbors 的问题', '在时间线观察工具调用卡片，回方案详情看透视记录'],
    acceptance: 'Agent 调用了哪个工具？方案的哪个端点承接了这次查询？',
    link: { text: '前往方案详情', sidebar: 'runtime' },
    difficulty: 2,
  },
  {
    id: 'task_s5_reload_semantics',
    stage: 's5',
    title: '多方案并存对照',
    goal: '体会同一本体的多套部署视图',
    prereq: ['task_s5_run_profile'],
    steps: ['用不同端口再建一套方案（加载同一本体）', '分别启动', '停止其中一套，确认另一套与普通对话不受影响（NFR-O-2 故障隔离）'],
    acceptance: '两套方案的查询结果一致吗？停止一套后挂载它的对话看到什么提示？',
    link: { text: '前往本体运行', sidebar: 'runtime' },
    difficulty: 2,
  },
]

/** `task:` 打卡空间（localStorage；REQ-75 配置页 P2 交付后迁服务端 pipeline_profile.checklist） */
const CHECKLIST_KEY = 'eino.onto.checklist.tasks'

function readChecklist(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(CHECKLIST_KEY) ?? '{}')
  } catch {
    return {}
  }
}

function writeChecklist(c: Record<string, string>) {
  localStorage.setItem(CHECKLIST_KEY, JSON.stringify(c))
}

// bugfix（开发者报障「图标淡几乎看不到」）：值必须是 **CSS 颜色**——原 s3: 'geekblue' 是 AntD
// 预设色名而非 CSS 颜色，inline background 无效 → S3 色块透明、白字落白底（不可见）。
// 统一改 hex（与 AntD preset 视觉近似），不再依赖「恰好是 CSS 关键字」的偶然。
// REQ-490：七阶段色块灰阶化（无框灰字语言——阶段差异由序号与完成态承载，不再用七彩区分）
const STAGE_COLORS: Record<string, string> = {
  s1: '#8b91a5', s2: '#8b91a5', s3: '#8b91a5', s4: '#8b91a5', s5: '#8b91a5', s6: '#8b91a5', s7: '#8b91a5',
}

// ---------------------------------------------------------------------------
// 构建方式对照（六路径卡，D-O11 分层 + D-O14 第六路径）
// ---------------------------------------------------------------------------

const BUILD_PATH_CARDS: { key: string; title: string; scene: string; points: string; state: { color: string; text: string }; example: string }[] = [
  {
    key: 'custom',
    title: '自定义构建',
    scene: '从零手写 / 导入 / AI 单轮生成',
    points: 'S1~S4 全程亲手操作，理解 spec_json 三要素',
    state: { color: 'green', text: '可用' },
    example: 'K8s 迷你运维（内置示例）',
  },
  {
    key: 'ontochat',
    title: 'OntoChat 流程',
    scene: '对话式多轮引导（CQ → 补全 → 草稿 → 校验 → 入库）',
    points: '对话式知识工程方法（OntoChat 论文三部曲）',
    state: { color: 'green', text: '可用' },
    example: '软件缺陷管理（AI 生成路径演示）',
  },
  {
    key: 'kb',
    title: '由知识库构建',
    scene: 'KB chunk→LLM 抽取 / KG→直转 / 混合（KB→本体构建方向）',
    points: '知识资产复用：语料/图谱 → 语义资产的直接通路（D-O14，O13 已工程化）',
    state: { color: 'orange', text: '部分可用' },
    example: '设备故障知识库 → 设备故障本体（第 5 示例，样例语料随 seeds/learning）',
  },
  {
    key: 'kg',
    title: 'KG 消费流程',
    scene: '自研 KG 图谱 / GraphRAG 试查 / 决策溯源（消费与审计栏承载）',
    points: '精确查询 vs 语义检索的消费面对照（D-O15 去-semantica 化）',
    state: { color: 'cyan', text: '引导' },
    example: '—（消费侧演示）',
  },
  {
    key: 'ontoextend',
    title: 'OntoExtend 流程',
    scene: 'ODP 模式推荐 + LOV 词表 → 扩展现有本体（审查入库）',
    points: '增量建模与本体演化',
    state: { color: 'gold', text: '部分可用' },
    example: 'ontoextend：组织与人员 ODP → 合并审查 → 新版本（M-O14 P2②）',
  },
]


// ---------------------------------------------------------------------------
// 运行方式对照（引擎差异卡；O6 已交付，推理对照已激活）
// ---------------------------------------------------------------------------

const RUNTIME_CARDS: { engine: string; tag: { color: string; text: string };推理: string; points: string; entry: string }[] = [
  {
    engine: 'Oxigraph',
    tag: { color: 'green', text: '可用' },
    推理: '无推理（精确匹配优先）',
    points: 'SPARQL 毫秒级查询；spec_json→TTL 导出后装载；显式重载语义',
    entry: '本体运行 → Oxigraph',
  },
  {
    engine: 'Fuseki',
    tag: { color: 'green', text: '可用' },
    推理: '带推理（RDFS/OWL 规则推理，方案级开关）',
    points: '推理对照（REQ-94）已激活：同本体建两套方案一开一关推理，同一 SPARQL 对照结果差异（如 subClassOf 实例类型传导）；对照实验建议见方案向导',
    entry: '本体运行 → Fuseki',
  },
]


export default function LearnPage() {
  const { showToast } = useUI()
  const [checklist, setChecklist] = useState<Record<string, string>>(readChecklist)
  const [learningExamples, setLearningExamples] = useState<{ key: string; name: string; description: string }[] | null>(null)
  const [ontologies, setOntologies] = useState<Ontology[]>([]) // REQ-524/M331：出题自测本体选择
  const [activeStage, setActiveStage] = useState<string>('s1') // REQ-162①：方块步骤条选中阶段
  // REQ-182：左导航子模块选中（学习路径默认首屏主轴；localStorage 记忆）
  // 开发者拍板合并（2026-09-29）：methods 页签删除——旧记忆 key=methods 兼容重定向到 path
  const [section, setSection] = useState<string>(() => {
    // REQ-470：hash 优先（#/onto/learn/{section}），无 hash 回落 localStorage 记忆
    const r = parseOntoHash()
    if (r?.col === 'learn' && r.learn) return r.learn
    const saved = localStorage.getItem('eino.onto.learn.section') ?? 'path'
    const valid = ['path', 'quiz', 'build-paths', 'runtime-paths', 'examples', 'pipeline']
    return valid.includes(saved) ? saved : 'path'
  })
  useEffect(() => {
    localStorage.setItem('eino.onto.learn.section', section)
  }, [section])
  // REQ-524/M331：出题自测本体清单（失败静默——出题页空态自明）
  useEffect(() => {
    api.listOntologies().then(setOntologies).catch(() => setOntologies([]))
  }, [])
  // REQ-470：后退/前进按 hash 恢复子模块
  useEffect(() => {
    const onPop = () => {
      const r = parseOntoHash()
      if (r?.col === 'learn' && r.learn) setSection(r.learn)
    }
    window.addEventListener('popstate', onPop)
    window.addEventListener('hashchange', onPop) // REQ-470：同文档 hash 导航同应用
    return () => {
      window.removeEventListener('popstate', onPop)
      window.removeEventListener('hashchange', onPop)
    }
  }, [])

  useEffect(() => {
    api
      .listLearningExamples()
      .then(setLearningExamples)
      .catch(() => setLearningExamples([]))
  }, [])

  const toggleTask = (id: string) => {
    const next = { ...checklist }
    if (next[id]) delete next[id]
    else next[id] = new Date().toISOString()
    setChecklist(next)
    writeChecklist(next)
  }

  /** 按阶段聚合任务完成度（REQ-91 ④） */
  const stageProgress = useMemo(() => {
    const m = new Map<string, { total: number; done: number }>()
    for (const t of TASKS) {
      const e = m.get(t.stage) ?? { total: 0, done: 0 }
      e.total += 1
      if (checklist[t.id]) e.done += 1
      m.set(t.stage, e)
    }
    return m
  }, [checklist])

  const totalDone = TASKS.filter((t) => checklist[t.id]).length
  const goSidebar = (key: string) => {
    localStorage.setItem('eino.onto.sidebar', key)
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  }

  const taskByStage = (stage: string) => TASKS.filter((t) => t.stage === stage)

  /** REQ-182：阶段详情（方法论卡片 + 任务卡）——「学习路径」与「方法论与任务卡」两视图复用 */
  const stageDetail = (stageKey: string) => {
    const i = STAGE_DEFS.findIndex((x) => x.key === stageKey)
    const s = STAGE_DEFS[i]
    const prog = stageProgress.get(s.key) ?? { total: 0, done: 0 }
    const methods = METHODOLOGY.filter((m) => m.stage === s.key)
    return (
      <div className="onto-learn-stage">
        {methods.map((m) => (
          <Collapse
            key={m.key}
            size="small"
            className="onto-learn-method"
            items={[{
              key: m.key,
              label: (
                <Space size={6} wrap>
                  <BookOutlined style={{ color: 'var(--c-brand)' }} />
                  <span style={{ fontWeight: 600 }}>{m.title}</span>
                  <Tag color="blue" style={{ margin: 0 }}>{m.tag}</Tag>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>方法论卡片 · REQ-90</Typography.Text>
                </Space>
              ),
              children: (
                <>
                  {/* REQ-505/M313：全文单源渲染（md 含核心思想+深度版全部节） */}
                  <div data-testid={`method-md-${m.key}`}>
                    <XMarkdown className="chat-md" content={m.raw} openLinksInNewTab />
                  </div>
                  {m.key === 'cq' && (
                    <div style={{ marginTop: 8 }}>
                      {/* REQ-248/M71：方法论卡「援引到构建」——直达 S1 AI 创建 CQ 输入点（此前纯展示零互链） */}
                      <Button
                        size="small"
                        type="primary"
                        ghost
                        onClick={() => {
                          localStorage.setItem('eino.onto.s1tab', 'ai')
                          localStorage.setItem('eino.onto.buildPath', 'custom')
                          localStorage.setItem('eino.onto.sidebar', 'build')
                          window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
                        }}
                      >
                        援引到构建：去 AI 创建录入 CQ →
                      </Button>
                    </div>
                  )}
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
                    来源：seeds/learning/methodology/ 深度全文 · 对应文档 03 §2（REQ-90）
                  </Typography.Text>
                  <Collapse
                    size="small"
                    ghost
                    items={[
                      {
                        key: 'deep',
                        label: <Typography.Text type="secondary" style={{ fontSize: 12 }}>深度版（REQ-90 P2 补齐）</Typography.Text>,
                        children: (
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            全文已随 REQ-505 单源渲染于上方（seeds/learning/methodology/ 该档 .md）。
                          </Typography.Text>
                        ),
                      },
                    ]}
                  />
                </>
              ),
            }]}
          />
        ))}
        {methods.length === 0 && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>本阶段方法论卡片随 P2 深度版补齐。</Typography.Text>
        )}
        {prog.total > 0 && (
          <div className="onto-learn-tasks">
            <div className="onto-learn-tasks-head">
              <span>任务卡（{prog.done}/{prog.total} 已完成）</span>
              <Progress percent={Math.round((prog.done / prog.total) * 100)} size="small" style={{ width: 120, margin: 0 }} showInfo={false} />
            </div>
            {taskByStage(s.key).map((t) => {
              const done = !!checklist[t.id]
              const prereqOk = t.prereq.every((pp) => checklist[pp])
              return (
                <div key={t.id} className={`onto-task-card${done ? ' done' : ''}`}>
                  <div className="onto-task-head">
                    <Button
                      size="small"
                      type={done ? 'primary' : 'default'}
                      icon={<CheckCircleOutlined />}
                      disabled={!prereqOk && !done}
                      title={!prereqOk && !done ? `前置任务未完成：${t.prereq.join(', ')}` : undefined}
                      onClick={() => toggleTask(t.id)}
                    >
                      {done ? '已完成' : '打卡'}
                    </Button>
                    <span className="onto-task-title">{t.title}</span>
                    <Tag style={{ margin: 0 }}>难度 {t.difficulty}</Tag>
                  </div>
                  <div className="onto-task-body">
                    <p><b>目标：</b>{t.goal}</p>
                    <ol>
                      {t.steps.map((st, j) => (
                        <li key={j}>{st}</li>
                      ))}
                    </ol>
                    <p className="onto-task-accept"><b>验收问题：</b>{t.acceptance}</p>
                    <Button
                      size="small"
                      type="link"
                      icon={<RightOutlined />}
                      onClick={() => {
                        if (t.link.s1tab) localStorage.setItem('eino.onto.s1tab', t.link.s1tab)
                        if (t.link.s1tab) localStorage.setItem('eino.onto.buildPath', 'custom')
                        goSidebar(t.link.sidebar)
                      }}
                    >
                      {t.link.text}
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
          阶段 {i + 1}/{STAGE_DEFS.length} · {s.short}
        </Typography.Text>
      </div>
    )
  }


  // REQ-182：左导航子模块定义（徽标带进度）。
  // 开发者拍板（2026-09-29）：「学习路径/方法论与任务卡两重复，保留一个」——methods 视图删除；
  // REQ-270：path 视图底部的「全部阶段方法论总览」收起区亦退役——步骤条已按阶段切换，全量重复无增量
  const menuItems = [
    { key: 'path', label: <Space size={6}>学习路径<Tag style={{ margin: 0 }} color="geekblue">{totalDone}/{TASKS.length}</Tag></Space> },
    { key: 'quiz', label: '出题自测' },
    { key: 'build-paths', label: '构建方式对照' },
    { key: 'runtime-paths', label: '运行方式对照' },
    { key: 'examples', label: '示例本体库' },
    { key: 'pipeline', label: '工具链配置' },
  ]

  return (
    <div className="work-main">
      <div className="work-head">
        <div className="work-head-text">
          <div className="work-head-title">
            <Typography.Title level={4} style={{ margin: 0 }}>
              学习中心
            </Typography.Title>
            <Tag color="blue" style={{ margin: 0 }}>默认页</Tag>
            <Tag color="geekblue" style={{ margin: 0 }}>任务进度 {totalDone}/{TASKS.length}</Tag>
          </div>
          <p className="work-head-desc">
            七阶段学习路径（REQ-74 理论骨架）：每阶段有方法论卡片（REQ-90）与动手任务卡（REQ-91，`task:` 打卡）；学完的路径产物进「本体构建」，部署进「本体运行」。
          </p>
        </div>
      </div>

      {/* REQ-182：左子模块导航 + 右内容区两栏（学习路径为默认首屏主轴） */}
      <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
        <Menu
          mode="vertical"
          selectedKeys={[section]}
          onClick={({ key }) => {
            setSection(key as string)
            // REQ-470：子模块切换 replace 写 hash（学习节页内细化不灌栈）
            writeOntoHash({ col: 'learn', learn: key as string }, true)
          }}
          style={{ width: 188, flexShrink: 0, position: 'sticky', top: 8, background: 'transparent', borderInlineEnd: 'none' }}
          items={menuItems}
        />
        <div style={{ flex: 1, minWidth: 0 }}>
          {section === 'path' && (
            <Card className="work-card work-card-flat" size="small" title="七阶段学习路径（S1 → S7）">
              <div className="onto-learn-steps" role="tablist" aria-label="七阶段学习路径步骤条">
                {STAGE_DEFS.map((st, i) => {
                  const prog = stageProgress.get(st.key) ?? { total: 0, done: 0 }
                  const allDone = prog.total > 0 && prog.done === prog.total
                  return (
                    <Fragment key={st.key}>
                      {i > 0 && <RightOutlined className="onto-learn-step-arrow" aria-hidden="true" />}
                      <button
                        type="button"
                        role="tab"
                        aria-selected={activeStage === st.key}
                        className={`onto-learn-step${activeStage === st.key ? ' active' : ''}${allDone ? ' done' : ''}`}
                        onClick={() => setActiveStage(st.key)}
                        title={i <= 3 ? `${st.short}（构建段）` : `${st.short}（运行段 · 见运行栏）`}
                      >
                        <span className="onto-learn-step-key" style={{ background: STAGE_COLORS[st.key] }}>{st.key.toUpperCase()}</span>
                        <span className="onto-learn-step-name">{st.short}</span>
                        {prog.total > 0 && (
                          <span className={`onto-learn-step-badge${allDone ? ' done' : ''}`}>{prog.done}/{prog.total}</span>
                        )}
                      </button>
                    </Fragment>
                  )
                })}
              </div>
              <div className="onto-learn-stage-detail" role="tabpanel">
                {stageDetail(activeStage)}
              </div>
            </Card>
          )}

          {section === 'build-paths' && (
            <Card className="work-card work-card-flat" size="small" title="构建方式对照（六路径）">
              <div className="onto-learn-cards">
                {BUILD_PATH_CARDS.map((c) => (
                  <div className="onto-learn-card" key={c.key}>
                    <div className="onto-learn-card-head">
                      <span className="onto-learn-card-title">{c.title}</span>
                      <Tag color={c.state.color} style={{ margin: 0 }}>{c.state.text}</Tag>
                    </div>
                    <p className="onto-learn-card-line"><b>适用场景：</b>{c.scene}</p>
                    <p className="onto-learn-card-line"><b>学习要点：</b>{c.points}</p>
                    <p className="onto-learn-card-line"><b>关联示例：</b>{c.example}</p>
                    <Button size="small" type="link" icon={<RightOutlined />} onClick={() => { localStorage.setItem('eino.onto.buildPath', c.key); goSidebar('build') }}>
                      进入该路径
                    </Button>
                  </div>
                ))}
              </div>
            </Card>
          )}

          {section === 'runtime-paths' && (
            <Card className="work-card work-card-flat" size="small" title="运行方式对照（引擎差异）">
              <Alert
                type="success"
                showIcon
                style={{ marginBottom: 10 }}
                title="同一份本体在不同引擎下行为不同——这是本体运行环节的核心学习点"
                description="推理对照已随 Fuseki（O6）激活：对同一本体建两套 Fuseki 方案（推理一开一关），同一 SPARQL 并行发往两套方案对照结果差异（推荐用 subClassOf 子类实例验证类型传导）。"
              />
              <div className="onto-learn-cards">
                {RUNTIME_CARDS.map((c) => (
                  <div className="onto-learn-card" key={c.engine}>
                    <div className="onto-learn-card-head">
                      <span className="onto-learn-card-title">
                        <CloudServerOutlined style={{ marginRight: 6, color: 'var(--c-brand)' }} />
                        {c.engine}
                      </span>
                      <Tag color={c.tag.color} style={{ margin: 0 }}>{c.tag.text}</Tag>
                    </div>
                    <p className="onto-learn-card-line"><b>推理能力：</b>{c.推理}</p>
                    <p className="onto-learn-card-line"><b>学习要点：</b>{c.points}</p>
                    <p className="onto-learn-card-line"><b>入口：</b>{c.entry}</p>
                    <Button size="small" type="link" icon={<RightOutlined />} onClick={() => goSidebar('runtime')}>
                      前往运行栏
                    </Button>
                  </div>
                ))}
              </div>
            </Card>
          )}

          {section === 'quiz' && <QuizPane ontologies={ontologies} />}
          {section === 'examples' && (
            <Card className="work-card work-card-flat" size="small" title="示例本体库">
              {learningExamples === null ? (
                <Skeleton active title={false} paragraph={{ rows: 3 }} />
              ) : learningExamples.length === 0 ? (
                <Typography.Text type="secondary">构建平面未返回学习示例（需 ontology-service 就绪）。</Typography.Text>
              ) : (
                <div className="onto-learn-cards">
                  {learningExamples.map((le) => (
                    <div className="onto-learn-card" key={le.key}>
                      <div className="onto-learn-card-head">
                        <span className="onto-learn-card-title">{le.name}</span>
                        <Tag style={{ margin: 0 }}>{le.key}</Tag>
                      </div>
                      <p className="onto-learn-card-line">{le.description}</p>
                      <Button
                        size="small"
                        type="primary"
                        ghost
                        onClick={() => {
                          api
                            .seedLearningExample(le.key)
                            .then((o) => {
                              showToast((o as unknown as { seeded?: boolean }).seeded === false ? '该学习示例已存在' : `学习示例「${o.name}」已创建`)
                            })
                            .catch((e: any) => showToast(e.message, 'err'))
                        }}
                      >
                        一键灌装到资产
                      </Button>
                    </div>
                  ))}
                  <Alert type="success" showIcon title="7 示例本体已齐（REQ-91 ③ + REQ-153 大型种子）" description="覆盖构建路径与规模梯度：K8s 迷你运维（手写，seed-sample）、软件缺陷管理（AI 生成）、组织与人员（复用对照 FOAF）、设备故障知识（分类+灌装路径，第 5 例随 O13 KB 构建路径）+ 医学常识（med_common，102 概念/170 实例，禁忌关系网）、基因与中心法则（gene_core，104 概念/94 实例，命名对齐 HGNC）两个百级大型种子（REQ-153，兼作 NFR-O-3 性能载体）。每份示例附建模说明（seeds/learning/examples/*.README.md：背景/CQ/决策记录/局限）。" />
                </div>
              )}
            </Card>
          )}

          {section === 'pipeline' && (
            <Card className="work-card work-card-flat" size="small" title="工具链配置（REQ-75/76）">
              <PipelinePane />
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
