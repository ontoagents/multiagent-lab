import DoneCTA from './components/DoneCTA'
import { useEffect, useState } from 'react'
import { Alert, Button, Card, Checkbox, Empty, Input, Segmented, Space, Steps, Table, Tabs, Tag, Typography, Upload } from 'antd'
import {
  ApiOutlined,
  CheckCircleOutlined,
  DatabaseOutlined,
  InboxOutlined,
  RightOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { api, ApiError } from '../../api/client'
import type { AiDraftResult, ExtractedCQ, ImportReport, Ontology, QualityReport, Spec, ValidationError } from '../../api/types'
import { useUI } from '../../store/ui'
import { ERR_COLUMNS } from './shared'
import SpecGraph from './components/SpecGraph'
import OntoChatFlow from './OntoChatFlow'
import KbBuildFlow, { StructuredFlow } from './BuildFromKBFlow'
import ReferenceMaterialsPanel from './components/ReferenceMaterialsPanel'
import OntoExtendFlow from './components/OntoExtendFlow'
import QualityReportCard, { qualityFindingsText } from './components/QualityReportCard'
import CatalogPane from './components/CatalogPane'

// ---------------------------------------------------------------------------
// 本体构建（BuildPage，REQ-104 ②）：按构建路径分二级模块（六路径分层标注状态）
//   自定义构建（可用，现有页面主体 S1~S4）| OntoChat 流程（可用，REQ-103 模式 A 载体）
//   | 由知识库构建（部分可用，O13/D-O14 REQ-108 独立流程页）| KG 消费流程（入口卡，D-O15 改造）
//   | OntoExtend 流程（部分可用，M-O14 P2②）
//   未工程化路径显示引导卡、不做空壳交互（D-O11）；Open Ontologies 双轨已退役（66 号拍板，方案 B）
// ---------------------------------------------------------------------------

type BuildPath = 'custom' | 'ontochat' | 'kg' | 'ontoextend' | 'kb' | 'catalog'

const PATHS: { key: BuildPath; label: string; state: 'ok' | 'partial' | 'guide'; desc: string }[] = [
  { key: 'custom', label: '自定义构建', state: 'ok', desc: 'S1 来源 → S2 编辑 → S3 校验 → S4 可视化（七阶段前 4 步）' },
  { key: 'ontochat', label: 'OntoChat 流程', state: 'ok', desc: '对话式 CQ 引导 → 逐轮补全 → 草稿入库（REQ-103 模式 A）' },
  { key: 'kb', label: '由知识库构建', state: 'partial', desc: 'KB chunk→LLM / KG→直转 / 混合三策略独立流程页（O13，D-O14/REQ-108）' },
  { key: 'kg', label: 'KG 消费流程', state: 'guide', desc: '入口卡跳转「消费与审计」栏（D-O15 自研 KG，原 semantica 流程改造）' },
  { key: 'ontoextend', label: 'OntoExtend 流程', state: 'partial', desc: 'ODP 模式推荐 + LOV 词表扩展 → 审查入库（M-O14 P2②）' },
  { key: 'catalog', label: '权威本体库', state: 'ok', desc: '查询并引入业界权威本体（W3C/ETSI/DCMI），运行与拓展零门槛（REQ-316）' },
]

const STATE_TAG: Record<BuildPath, { color: string; text: string }> = {
  custom: { color: 'green', text: '可用' },
  ontochat: { color: 'green', text: '可用' },
  kb: { color: 'orange', text: '部分可用' },
  kg: { color: 'cyan', text: '引导' },
  ontoextend: { color: 'gold', text: '部分可用' },
  catalog: { color: 'green', text: '可用' },
}

const ONTO_BUILD_PATH_KEY = 'eino.onto.buildPath'

function readBuildPath(): BuildPath {
  const v = localStorage.getItem(ONTO_BUILD_PATH_KEY)
  return v === 'ontochat' || v === 'kg' || v === 'semantica' /* 旧值兼容 */ || v === 'ontoextend' || v === 'kb' || v === 'catalog'
    ? (v as BuildPath)
    : 'custom'
}

export default function BuildPage() {
  const [buildPath, setBuildPath] = useState<BuildPath>(readBuildPath)

  const select = (key: BuildPath) => {
    localStorage.setItem(ONTO_BUILD_PATH_KEY, key)
    setBuildPath(key)
  }

  return (
    <div className="work-main">
      <div className="work-head">
        <div className="work-head-text">
          <div className="work-head-title">
            <Typography.Title level={4} style={{ margin: 0 }}>
              本体构建
            </Typography.Title>
            <Tag color="blue" style={{ margin: 0 }}>六条构建路径</Tag>
          </div>
          <p className="work-head-desc">
            同一个本体可以由不同路径建成（学习要点各不相同）；产物统一进入「本体资产」栏管理。
          </p>
        </div>
      </div>

      {/* REQ-183：左路径导航 + 右路径工作区两栏（状态徽标与差异说明常显） */}
      <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
        <nav
          aria-label="构建路径导航"
          style={{ width: 216, flexShrink: 0, position: 'sticky', top: 8, display: 'flex', flexDirection: 'column', gap: 6 }}
        >
          {PATHS.map((p) => (
            <button
              key={p.key}
              type="button"
              aria-current={buildPath === p.key || undefined}
              className={`onto-engine-item${buildPath === p.key ? ' active' : ''}`}
              style={{ textAlign: 'left', width: '100%' }}
              onClick={() => select(p.key)}
            >
              <span className="onto-engine-top">
                <span className="onto-engine-label">{p.label}</span>
                <Tag color={STATE_TAG[p.key].color} style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
                  {STATE_TAG[p.key].text}
                </Tag>
              </span>
              <span className="onto-engine-desc">{p.desc}</span>
            </button>
          ))}
        </nav>

        <div style={{ flex: 1, minWidth: 0 }}>
          {buildPath === 'custom' && (
            <CustomFlow
              onGoKbPath={() => select('kb')}
              // REQ-306 A2：AI 创建分流——描述携带进 OntoChat（localStorage 一次性预填）
              onGoOntoChat={(desc) => {
                if (desc) localStorage.setItem('eino.ontochat.preset.desc', desc)
                select('ontochat')
              }}
            />
          )}
          {buildPath === 'ontochat' && <OntoChatFlow onSaved={() => { /* 入库后产物进资产栏；此处留在会话页展示 done 态 */ }} />}
          {buildPath === 'kb' && (<><KbBuildFlow /><StructuredFlow /></>)}
          {buildPath === 'kg' && <KgGuide />}
          {buildPath === 'ontoextend' && <OntoExtendFlow />}
          {buildPath === 'catalog' && <CatalogPane />}
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// 自定义构建（现有页面主体：S1~S4 四步，S5~S7 卡片移除——运行看运行栏、facade/挂载看方案详情）
// ---------------------------------------------------------------------------

const CUSTOM_STAGES = [
  { key: 's1', title: 'S1 本体来源' },
  { key: 's2', title: 'S2 编辑' },
  { key: 's3', title: 'S3 校验' },
  { key: 's4', title: 'S4 可视化' },
]

function CustomFlow({ onGoKbPath, onGoOntoChat }: { onGoKbPath?: () => void; onGoOntoChat?: (desc: string) => void }) {
  const [step, setStep] = useState(0)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [spec, setSpec] = useState<Spec | null>(null)
  const [specTick, setSpecTick] = useState(0)

  // 选中本体 → 拉取 Spec（404 视为尚未保存）
  useEffect(() => {
    if (!activeId) {
      setSpec(null)
      return
    }
    let alive = true
    api
      .getSpec(activeId)
      .then((s) => {
        if (alive) setSpec(s)
      })
      .catch(() => {
        if (alive) setSpec(null)
      })
    return () => {
      alive = false
    }
  }, [activeId, specTick])

  const handleCreated = (selectId?: string) => {
    if (selectId) setActiveId(selectId)
    setSpecTick((t) => t + 1)
    if (selectId) setStep(1)
  }

  return (
    <Card className="work-card onto-stage-card" size="small">
      <Steps
        size="small"
        current={step}
        onChange={setStep}
        items={CUSTOM_STAGES.map((s, i) => ({
          key: s.key,
          title: s.title,
          status: i === step ? 'process' : 'wait',
          className: i === step ? 'onto-step-selected' : undefined,
        }))}
        style={{ marginBottom: 14 }}
      />
      {step === 0 && <S1Source onCreated={handleCreated} onGoKbPath={onGoKbPath} onGoOntoChat={onGoOntoChat} />}
      {step === 1 && activeId && (
        <S2EditPane
          ontologyId={activeId}
          spec={spec}
          onSaved={() => setSpecTick((t) => t + 1)}
          onNext={() => setStep(2)}
        />
      )}
      {step === 1 && !activeId && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请先在 S1 创建 / 导入本体" />}
      {step === 2 && activeId && <S3ValidatePane ontologyId={activeId} onNext={() => setStep(3)} />}
      {step === 2 && !activeId && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请先在 S1 创建 / 导入本体" />}
      {step === 3 && (
        <>
          <SpecGraph spec={spec} />
          {activeId && (
            <div style={{ marginTop: 10 }}>
              <DoneCTA ontologyId={activeId} detail="S1~S4 构建段完成" />
            </div>
          )}
        </>
      )}
      {activeId && step >= 1 && (
        <Alert
          type="success"
          showIcon
          style={{ marginTop: 12 }}
          title={`当前工作本体：${activeId}`}
          description="S1~S4 完成后：到「本体资产」栏查看产物与版本，到「本体运行」栏部署为运行方案（S5~S7 已移交）。"
          action={
            <Button
              size="small"
              icon={<RightOutlined />}
              onClick={() => {
                localStorage.setItem('eino.onto.sidebar', 'runtime')
                window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
              }}
            >
              前往本体运行
            </Button>
          }
        />
      )}
    </Card>
  )
}

// ---------------------------------------------------------------------------
// S1 本体来源（导入 / AI 创建 / 内置示例 / 空白 / CSV 灌装）
// ---------------------------------------------------------------------------

const S1_TAB_KEY = 'eino.onto.s1tab'

function S1Source({ onCreated, onGoKbPath, onGoOntoChat }: { onCreated: (selectId?: string) => void; onGoKbPath?: () => void; onGoOntoChat?: (desc: string) => void }) {
  // REQ-248/M71：S1 来源 tab 记忆（方法论卡「援引到构建」直达 AI 创建 CQ 输入点的落点）
  const [s1Tab, setS1Tab] = useState(() => localStorage.getItem(S1_TAB_KEY) ?? 'import')
  const { showToast } = useUI()
  const [importMode, setImportMode] = useState<'file' | 'paste'>('file')
  const [pasteFilename, setPasteFilename] = useState('')
  const [pasteContent, setPasteContent] = useState('')
  const [report, setReport] = useState<ImportReport | null>(null)
  const [importBusy, setImportBusy] = useState(false)

  const [aiDesc, setAiDesc] = useState('')
  const [aiHint, setAiHint] = useState('')
  const [aiCq, setAiCq] = useState('')
  const [aiBusy, setAiBusy] = useState(false)
  const [aiProgress, setAiProgress] = useState<string>('') // REQ-295③：服务端实测轮次进度（永不虚构百分比）
  const [aiResult, setAiResult] = useState<AiDraftResult | null>(null)
  const [aiName, setAiName] = useState('')
  // REQ-306 A1/M110：AI 创建澄清轮——生成前 LLM 缺口清单+逐轮补充（可随时「直接生成」跳过）
  const [clarifyReply, setClarifyReply] = useState<string | null>(null)
  const [clarifyBusy, setClarifyBusy] = useState(false)
  const [clarifyHints, setClarifyHints] = useState<string[]>([])
  const [clarifyInput, setClarifyInput] = useState('')
  // REQ-308/M112：CQ 候选生成确认卡——ExtractCQs（描述+澄清轮材料）→人工勾选/编辑→回填 aiCq
  const [cqJobBusy, setCqJobBusy] = useState(false)
  const [cqCandidates, setCqCandidates] = useState<ExtractedCQ[] | null>(null)
  const [cqPicks, setCqPicks] = useState<boolean[]>([])
  const [cqEdits, setCqEdits] = useState<Record<number, string>>({})
  // REQ-312/M116：质量报告卡——完整 Report 结构化渲染+新旧对照+报告驱动优化；REQ-314 渐进披露状态条
  const [aiPrevQuality, setAiPrevQuality] = useState<QualityReport | null>(null)
  const [aiShowResult, setAiShowResult] = useState(false)

  const [sampleBusy, setSampleBusy] = useState(false)

  const [blankName, setBlankName] = useState('')
  const [blankDesc, setBlankDesc] = useState('')
  const [blankBusy, setBlankBusy] = useState(false)

  // REQ-305 导入体积预检：与后端 IMPORT_MAX_BODY_MB 默认值一致（后端为权威，env 调小时仍兜底拒绝）
  const IMPORT_MAX_BYTES = 64 * 1024 * 1024

  const doImportFile = async (file: File) => {
    if (file.size > IMPORT_MAX_BYTES) {
      showToast(`导入文件超过上限（${(file.size / 1024 / 1024).toFixed(0)} MB > 64 MB）：请拆分或清洗后重试`, 'err')
      return
    }
    setImportBusy(true)
    setReport(null)
    try {
      const r = await api.importOntologyFile(file)
      setReport(r.report)
      showToast(`已导入「${r.ontology.name}」`)
      onCreated(r.ontology.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setImportBusy(false)
    }
  }

  const doImportPaste = async () => {
    if (!pasteContent.trim()) {
      showToast('请粘贴文件内容', 'err')
      return
    }
    if (pasteContent.length > IMPORT_MAX_BYTES) {
      showToast('粘贴内容超过上限（64 MB）：请拆分或清洗后重试', 'err')
      return
    }
    setImportBusy(true)
    setReport(null)
    try {
      const r = await api.importOntologyContent(pasteFilename.trim() || 'pasted.ttl', pasteContent)
      setReport(r.report)
      showToast(`已导入「${r.ontology.name}」`)
      onCreated(r.ontology.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setImportBusy(false)
    }
  }

  // REQ-308/M112：AI 生成 CQ——描述+澄清轮材料为原料调 ExtractCQs（P5 两净化），候选经确认卡人工勾选
  const doCQExtract = async () => {
    if (!aiDesc.trim()) {
      showToast('请先填写领域描述，再生成 CQ', 'err')
      return
    }
    setCqJobBusy(true)
    try {
      const { job_id } = await api.aiCQExtract(aiDesc.trim(), clarifyHints)
      let done = false
      for (let i = 0; i < 150; i++) {
        await new Promise((res) => setTimeout(res, 2000))
        const j = await api.aiDraftJob(job_id)
        if (j.status === 'done') {
          const cqs = j.cqs ?? []
          if (!cqs.length) throw new Error('未抽取到候选，请补充更多领域信息')
          setCqCandidates(cqs)
          setCqPicks(cqs.map(() => true))
          setCqEdits({})
          showToast(`已生成 ${cqs.length} 条 CQ 候选，请勾选确认`)
          done = true
          break
        }
        if (j.status === 'failed') throw new Error(j.error || 'CQ 生成失败')
      }
      if (!done) throw new Error('生成超时（>5 分钟）——可重试')
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setCqJobBusy(false)
    }
  }

  const applyCQCandidates = () => {
    if (!cqCandidates) return
    const picked = cqCandidates.map((c, i) => (cqPicks[i] ? (cqEdits[i] ?? c.cq).trim() : '')).filter(Boolean)
    if (!picked.length) {
      showToast('请至少勾选一条候选', 'err')
      return
    }
    const existing = aiCq.split('\n').map((x) => x.trim()).filter(Boolean)
    const merged = [...existing, ...picked.filter((x) => !existing.includes(x))]
    setAiCq(merged.join('\n'))
    setCqCandidates(null)
    showToast(`已采用 ${picked.length} 条 CQ 写入输入框（可继续手写补充后生成草案）`)
  }

  // REQ-306 A1：澄清轮——基于描述+已补材料调 LLM 输出范围复述+五维缺口清单（同步 RawChat，秒~十秒级）
  const doClarify = async () => {
    if (!aiDesc.trim()) {
      showToast('请先填写领域描述', 'err')
      return
    }
    const hints = [...clarifyHints]
    if (clarifyInput.trim()) {
      hints.push(clarifyInput.trim())
      setClarifyHints(hints)
      setClarifyInput('')
    }
    setClarifyBusy(true)
    try {
      const r = await api.aiClarify(aiDesc.trim(), hints)
      setClarifyReply(r.reply)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setClarifyBusy(false)
    }
  }

  const doAiDraft = async (force = false, extraHintOverride?: string) => {
    if (!aiDesc.trim()) {
      showToast('请描述本体用途', 'err')
      return
    }
    const cqs = aiCq
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    // REQ-306 A2：材料单薄（短描述且无 CQ/提示/已澄清）先走澄清轮——不拦截，点「直接生成草案」可随时跳过
    if (!force && !extraHintOverride && clarifyReply === null && aiDesc.trim().length < 50 && cqs.length === 0 && !aiHint.trim()) {
      await doClarify()
      return
    }
    // REQ-312 A2/M116：按报告优化——旧报告存 prev 供对照；REQ-314 收口：生成时收起详情
    setAiPrevQuality(aiResult?.quality ?? null)
    setAiShowResult(false)
    setAiBusy(true)
    setAiResult(null)
    try {
      // REQ-248/G2：CQ 显式入参（后端 DraftWithCQ 回写 spec.cq 入资产）
      // REQ-306 A1：澄清轮补充的材料并入生成上下文
      // REQ-267/M76：异步提交+轮询（202+job），去 120s 同步阻塞窗口；失败/超时回显可懂错误
      // REQ-312 A2：extraHintOverride=报告 findings 文本（按报告优化时由 QualityReportCard 回调传入）
      const mergedHint = [[aiHint.trim(), ...clarifyHints].filter(Boolean).join('\n\n'), extraHintOverride].filter(Boolean).join('\n\n')
      const { job_id } = await api.aiDraftOntologyAsync(aiDesc.trim(), mergedHint || undefined, cqs.length ? cqs : undefined)
      showToast('已提交生成任务，生成中…')
      let r: AiDraftResult | undefined
      for (let i = 0; i < 160; i++) {
        await new Promise((res) => setTimeout(res, 1500))
        const j = await api.aiDraftJob(job_id)
        if (j.progress) setAiProgress(j.progress) // REQ-295③：实测轮次进度透出
        if (j.status === 'done') {
          r = j.result
          break
        }
        if (j.status === 'failed') throw new Error(j.error || 'AI 草案生成失败')
      }
      if (!r) throw new Error('生成超时（>240s）——任务仍在后台执行，可稍后重试')
      setAiResult(r)
      // REQ-247/G4：草案质量分展示（此前 REST 丢弃 quality——用户看不到生成质量）
      if (r.quality?.score) {
        showToast(`草案质量分 ${Math.round(r.quality.score.overall ?? 0)}（错误 ${r.quality.error_count} / 告警 ${r.quality.warning_count}）`)
      }
      setAiName(r.spec?.name ?? '')
      showToast('草案已生成，请确认后创建')
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 503) showToast('AI 草案不可用：LLM 未配置（503）', 'err')
      else showToast(e.message, 'err')
    } finally {
      setAiBusy(false)
      setAiProgress('')
    }
  }

  const saveAiDraft = async () => {
    if (!aiResult) return
    const nm = aiName.trim() || aiResult.spec?.name || ''
    if (!nm) {
      showToast('请输入本体名称', 'err')
      return
    }
    setAiBusy(true)
    try {
      const created = await api.createOntology({ name: nm, description: aiResult.spec?.description ?? aiDesc })
      await api.saveSpec(created.id, { ...aiResult.spec, name: nm })
      showToast('AI 草案已创建并保存')
      setAiResult(null)
      setAiDesc('')
      setAiHint('')
      setAiCq('')
      onCreated(created.id)
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) showToast(`草案校验未通过：${e.validationErrors[0].message}`, 'err')
      else showToast(e.message, 'err')
    } finally {
      setAiBusy(false)
    }
  }

  const doSeed = async () => {
    setSampleBusy(true)
    try {
      const r = await api.seedSampleOntology()
      if (r.seeded === false) showToast(r.note || '内置示例已存在')
      else showToast('内置示例已创建')
      onCreated(r.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSampleBusy(false)
    }
  }

  // REQ-246/G1：从种子起步（灌装指定种子→跳编辑；建模说明在参考素材面板可见）
  const doSeedKey = async (key: string) => {
    setSampleBusy(true)
    try {
      const r = await api.seedLearningExample(key)
      showToast(r.seeded === false ? '种子本体已存在，直接打开' : '种子本体已灌装，可对照建模说明编辑')
      onCreated(r.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSampleBusy(false)
    }
  }

  // REQ-246/G1⑤：新建同名即时提示（保留同名，提示已有清单）
  const [dupName, setDupName] = useState<Ontology | null>(null)
  useEffect(() => {
    const n = blankName.trim()
    if (!n) {
      setDupName(null)
      return
    }
    let alive = true
    api
      .listOntologies()
      .then((ls) => {
        if (alive) setDupName(ls.find((o) => o.name === n) ?? null)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [blankName])

  const doBlank = async () => {
    if (!blankName.trim()) {
      showToast('请输入名称', 'err')
      return
    }
    setBlankBusy(true)
    try {
      const o = await api.createOntology({ name: blankName.trim(), description: blankDesc })
      showToast('已创建空白本体')
      setBlankName('')
      setBlankDesc('')
      onCreated(o.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBlankBusy(false)
    }
  }

  const reportAlert = report ? (
    <>
      <Alert
        type={report.lossy ? 'warning' : 'success'}
        showIcon
        style={{ marginTop: 12 }}
        title={`导入报告：格式 ${report.format}${report.lossy ? '（有损）' : '（无损）'}`}
        description={
          report.warnings?.length || report.lossy_note ? (
            <ul className="onto-report-list">
              {(report.warnings ?? []).map((w, i) => (
                <li key={i}>{w}</li>
              ))}
              {report.lossy_note && <li>{report.lossy_note}</li>}
            </ul>
          ) : undefined
        }
      />
      {/* REQ-318/M120：双形态口径——original 完整语义无损；spec_json 为建模子集，未投影公理已透传存档 */}
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
        口径：original 源文件完整语义无损（运行/查询/推理直挂）；归一化 spec_json 为建模子集（未投影公理已透传存档，详情页「透传公理」可查）。
      </Typography.Text>
    </>
  ) : null

  return (
    <Tabs
      activeKey={s1Tab}
      onChange={(k) => { setS1Tab(k); localStorage.setItem(S1_TAB_KEY, k) }}
      items={[
        {
          key: 'import',
          label: '导入文件',
          children: (
            <>
              <Segmented
                value={importMode}
                onChange={(v) => setImportMode(v as 'file' | 'paste')}
                options={[
                  { value: 'file', label: '上传文件' },
                  { value: 'paste', label: '粘贴内容' },
                ]}
                style={{ marginBottom: 12 }}
              />
              {importMode === 'file' ? (
                <Upload.Dragger
                  accept=".ttl,.owl,.graphml,.csv,.json,.txt"
                  multiple={false}
                  showUploadList={false}
                  disabled={importBusy}
                  beforeUpload={(file) => {
                    doImportFile(file)
                    return false
                  }}
                >
                  <p className="ant-upload-drag-icon">
                    <InboxOutlined />
                  </p>
                  <p className="ant-upload-text">点击或拖拽文件到此区域导入</p>
                  <p className="ant-upload-hint">支持 TTL / OWL / GraphML / CSV / spec_json，自动嗅探格式</p>
                </Upload.Dragger>
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size={8}>
                  <Input
                    value={pasteFilename}
                    onChange={(e) => setPasteFilename(e.target.value)}
                    placeholder="文件名（如 sample.ttl，用于格式嗅探）"
                  />
                  <Input.TextArea
                    className="onto-spec-editor"
                    value={pasteContent}
                    onChange={(e) => setPasteContent(e.target.value)}
                    autoSize={{ minRows: 8, maxRows: 18 }}
                    placeholder="粘贴 TTL / OWL / GraphML / CSV / spec_json 内容…"
                    spellCheck={false}
                  />
                  <Button type="primary" loading={importBusy} onClick={doImportPaste}>
                    导入内容
                  </Button>
                </Space>
              )}
              {reportAlert}
            </>
          ),
        },
        {
          key: 'ai',
          label: 'AI 创建',
          children: (
            // REQ-314/M117：两栏化——左列建模输入 / 右列反馈与产出（澄清卡/CQ 候选卡/质量报告卡/预览）；
            // 渐进披露：生成结果默认收起为状态条（首屏只留输入面）。窄屏 flexWrap 降级单列。
            <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start', flexWrap: 'wrap' }}>
              <div style={{ flex: '1 1 360px', minWidth: 320, maxWidth: 620 }}>
                <div className="onto-sec" style={{ marginTop: 0 }}>
                  <span className="onto-sec-title">领域描述（必填）</span>
                </div>
                <Input.TextArea
                  value={aiDesc}
                  onChange={(e) => setAiDesc(e.target.value)}
                  autoSize={{ minRows: 4, maxRows: 9 }}
                  placeholder="用自然语言描述要建模的领域（如：K8s 运维平台的概念、关系与实例）…"
                />
                <Input
                  style={{ marginTop: 8 }}
                  value={aiHint}
                  onChange={(e) => setAiHint(e.target.value)}
                  placeholder="额外提示（可选，如：聚焦 Deployment / Service / Pod 三类）"
                />
                <div className="onto-sec" style={{ marginTop: 12 }}>
                  <span className="onto-sec-title">能力问题（CQ）引导（可选，每行一个）</span>
                  <span className="hit-spacer" />
                  {/* REQ-308/M112：AI 生成 CQ——候选经确认卡人工勾选后回填，手写与生成共存 */}
                  <Button size="small" loading={cqJobBusy} disabled={!aiDesc.trim()} onClick={doCQExtract} data-testid="cq-generate-btn">
                    AI 生成 CQ
                  </Button>
                </div>
                <Input.TextArea
                  value={aiCq}
                  onChange={(e) => setAiCq(e.target.value)}
                  autoSize={{ minRows: 2, maxRows: 5 }}
                  placeholder={'本体应能回答的关键问题，每行一个：\n某缺陷源于哪个需求？\n某故障应采取什么维护措施？'}
                />
                <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
                  先列 3~5 个本体要回答的问题，会并入生成上下文（REQ-82 / REQ-90）；也可由 AI 依据领域描述生成候选
                </Typography.Text>
                <Button
                  type="primary"
                  icon={<ThunderboltOutlined />}
                  loading={aiBusy}
                  style={{ marginTop: 12 }}
                  onClick={() => doAiDraft()}
                  data-testid="ai-draft-btn"
                >
                  生成草案
                </Button>
                {/* REQ-295③（67 号 A4）：诚实进度——只显示服务端实测轮次计数，永不虚构百分比与 ETA */}
                {aiBusy && aiProgress && (
                  <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0, fontSize: 12 }} data-testid="ai-draft-progress">
                    {aiProgress}
                  </Typography.Paragraph>
                )}
              </div>
              <div style={{ flex: '1 1 360px', minWidth: 320 }}>
                {/* REQ-306 A1/M110：澄清轮卡——生成前 LLM 范围复述+五维缺口清单；逐轮补充或直接生成；A2 一键切 OntoChat */}
                {clarifyReply && (
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 10 }}
                    title="生成前澄清——材料缺口清单"
                    description={
                      <>
                        <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 8, fontSize: 13 }} data-testid="clarify-reply">
                          {clarifyReply}
                        </Typography.Paragraph>
                        <Input.TextArea
                          value={clarifyInput}
                          onChange={(e) => setClarifyInput(e.target.value)}
                          autoSize={{ minRows: 2, maxRows: 5 }}
                          placeholder="按缺口清单补充材料（可多轮；也可直接生成）…"
                          data-testid="clarify-input"
                        />
                        <Space style={{ marginTop: 8 }} size={8} wrap>
                          <Button size="small" loading={clarifyBusy} onClick={doClarify} data-testid="clarify-more">
                            补充并继续澄清
                          </Button>
                          <Button size="small" type="primary" loading={aiBusy} onClick={() => doAiDraft(true)} data-testid="clarify-generate">
                            直接生成草案
                          </Button>
                          {onGoOntoChat && (
                            <Button
                              size="small"
                              onClick={() => onGoOntoChat([aiDesc.trim(), ...clarifyHints].filter(Boolean).join('\n'))}
                              data-testid="clarify-goto-ontochat"
                            >
                              改用 OntoChat 对话式构建
                            </Button>
                          )}
                        </Space>
                        {clarifyHints.length > 0 && (
                          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 6 }}>
                            已补充 {clarifyHints.length} 轮材料（将并入生成上下文）
                          </Typography.Text>
                        )}
                        {clarifyHints.length >= 3 && (
                          <Typography.Text type="warning" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                            已澄清 {clarifyHints.length} 轮——建议直接生成，或切换 OntoChat 做深度多轮对话
                          </Typography.Text>
                        )}
                      </>
                    }
                  />
                )}
                {cqCandidates && (
                  <Card size="small" style={{ marginBottom: 10 }} data-testid="cq-candidate-card" title={`CQ 候选（${cqPicks.filter(Boolean).length}/${cqCandidates.length} 已勾选）——勾选/编辑后采用，将并入左列输入框`}>
                    {cqCandidates.map((c, i) => (
                      <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 4 }}>
                        <Checkbox checked={!!cqPicks[i]} onChange={(e) => setCqPicks((p) => p.map((v, j) => (j === i ? e.target.checked : v)))} />
                        <Input
                          size="small"
                          value={cqEdits[i] ?? c.cq}
                          onChange={(e) => setCqEdits((m) => ({ ...m, [i]: e.target.value }))}
                        />
                        {c.origin && <Tag style={{ margin: 0, flexShrink: 0 }}>{c.origin}</Tag>}
                        <Button
                          size="small"
                          type="text"
                          onClick={() => {
                            setCqCandidates((list) => (list ?? []).filter((_, j) => j !== i))
                            setCqPicks((p) => p.filter((_, j) => j !== i))
                          }}
                        >
                          删
                        </Button>
                      </div>
                    ))}
                    <Space style={{ marginTop: 6 }}>
                      <Button
                        size="small"
                        onClick={() => {
                          setCqCandidates((list) => [...(list ?? []), { cq: '' }])
                          setCqPicks((p) => [...p, true])
                        }}
                      >
                        手动添加
                      </Button>
                      <Button size="small" type="primary" onClick={applyCQCandidates} data-testid="cq-apply-btn">
                        采用 {cqPicks.filter(Boolean).length} 条
                      </Button>
                      <Button size="small" onClick={() => setCqCandidates(null)}>
                        放弃
                      </Button>
                    </Space>
                  </Card>
                )}
                {aiResult && !aiShowResult && (
                  <Alert
                    type="success"
                    showIcon
                    title="草案已生成"
                    description={
                      <Space size={6} wrap>
                        <Tag color="blue" style={{ margin: 0 }}>概念 {aiResult.spec?.concepts?.length ?? 0}</Tag>
                        <Tag color="geekblue" style={{ margin: 0 }}>关系 {aiResult.spec?.relations?.length ?? 0}</Tag>
                        <Tag color="purple" style={{ margin: 0 }}>实例 {aiResult.spec?.instances?.length ?? 0}</Tag>
                        {aiResult.quality?.score && <Tag color="gold" style={{ margin: 0 }}>质量分 {Number(aiResult.quality.score.overall ?? 0).toFixed(1)}</Tag>}
                        <Button size="small" type="link" onClick={() => setAiShowResult(true)} data-testid="ai-show-result">
                          展开质量报告与预览
                        </Button>
                      </Space>
                    }
                  />
                )}
                {aiResult && aiShowResult && (
                  <div className="onto-ai-preview" data-testid="ai-result-detail">
                    <Space size={6} wrap style={{ marginBottom: 8 }}>
                      <Tag color="blue" style={{ margin: 0 }}>概念 {aiResult.spec?.concepts?.length ?? 0}</Tag>
                      <Tag color="geekblue" style={{ margin: 0 }}>关系 {aiResult.spec?.relations?.length ?? 0}</Tag>
                      <Tag color="purple" style={{ margin: 0 }}>实例 {aiResult.spec?.instances?.length ?? 0}</Tag>
                      <Tag style={{ margin: 0 }}>rounds {aiResult.rounds ?? '—'}</Tag>
                      <Button size="small" type="link" onClick={() => setAiShowResult(false)}>
                        收起
                      </Button>
                    </Space>
                    {aiResult.warning && <Alert type="warning" showIcon style={{ marginBottom: 8 }} title={aiResult.warning} />}
                    {/* REQ-312/M116：完整质量报告卡（三维分+Findings 分组+新旧对照+按报告优化） */}
                    {aiResult.quality ? (
                      <QualityReportCard
                        report={aiResult.quality}
                        prev={aiPrevQuality}
                        optimizing={aiBusy}
                        onOptimize={() => {
                          const ftext = qualityFindingsText(aiResult.quality)
                          if (!ftext) {
                            showToast('当前报告无待修复问题', 'err')
                            return
                          }
                          void doAiDraft(true, '请重点修复上次生成发现的质量问题：\n' + ftext)
                        }}
                      />
                    ) : (
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        质量报告不可用（构建平面不可达时仅本地结构校验兜底）
                      </Typography.Text>
                    )}
                    <Space style={{ marginTop: 10 }} size={8}>
                      <Input
                        value={aiName}
                        onChange={(e) => setAiName(e.target.value)}
                        placeholder="本体名称"
                        style={{ width: 260 }}
                      />
                      <Button type="primary" loading={aiBusy} onClick={saveAiDraft}>
                        创建并保存
                      </Button>
                    </Space>
                  </div>
                )}
                {!aiResult && !clarifyReply && !cqCandidates && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    填写左列材料后点「生成草案」——生成结果、质量报告与预览将在此处呈现（REQ-312/314）。
                  </Typography.Text>
                )}
              </div>
            </div>
          ),
        },
        {
          key: 'sample',
          label: '内置示例',
          children: (
            <>
              <p className="onto-detail-empty">内置「K8s 运维平台」示例本体（id=onto_k8s_ops），一键创建即可体验完整流程。</p>
              <Button type="primary" loading={sampleBusy} onClick={doSeed}>
                创建内置示例
              </Button>
            </>
          ),
        },
        {
          key: 'blank',
          label: '空白新建',
          children: (
            <Space direction="vertical" style={{ width: '100%' }} size={8}>
              <Input
                value={blankName}
                onChange={(e) => setBlankName(e.target.value)}
                placeholder="本体名称（必填；建议领域名+用途，如「设备故障运维」）"
                status={dupName ? 'warning' : undefined}
              />
              {dupName && (
                <Typography.Text type="warning" style={{ fontSize: 12 }}>
                  已存在同名本体「{dupName.name}」（{dupName.id}）——仍可创建（同名不被禁止），建议换名或 Fork 那份本体。
                </Typography.Text>
              )}
              <Input value={blankDesc} onChange={(e) => setBlankDesc(e.target.value)} placeholder="描述（可选）" />
              <Button type="primary" loading={blankBusy} onClick={doBlank}>
                创建空白本体
              </Button>
              <ReferenceMaterialsPanel onStartFromSeed={doSeedKey} />
            </Space>
          ),
        },
        {
          key: 'fromkb',
          label: '由知识库构建',
          children: (
            <>
              <p className="onto-detail-empty">
                以已有知识库（RAG chunk 语料 / GraphRAG KG）为数据源生成本体草稿——数据来自 KB 而非自然语言，与「AI 创建」互补（D-O14/REQ-108）。
              </p>
              <Button type="primary" icon={<DatabaseOutlined />} onClick={() => onGoKbPath?.()}>
                进入「由知识库构建」流程
              </Button>
            </>
          ),
        },
      ]}
    />
  )
}

// ---------------------------------------------------------------------------
// S2 编辑（自定义构建内简化版：JSON 编辑 + 保存；版本/产物在资产栏）
// ---------------------------------------------------------------------------

function S2EditPane({ ontologyId, spec, onSaved, onNext }: { ontologyId: string; spec: Spec | null; onSaved: () => void; onNext: () => void }) {
  const { showToast } = useUI()
  const [specText, setSpecText] = useState('')
  const [saving, setSaving] = useState(false)
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([])
  const [ontoName, setOntoName] = useState('')

  useEffect(() => {
    setSpecText(spec ? JSON.stringify(spec, null, 2) : '')
    setValidationErrors([])
  }, [spec, ontologyId])

  useEffect(() => {
    api
      .getOntology(ontologyId)
      .then((o) => setOntoName(o.name))
      .catch(() => setOntoName(ontologyId))
  }, [ontologyId])

  const save = async () => {
    let parsed: Spec
    try {
      parsed = JSON.parse(specText)
    } catch (e: any) {
      showToast(`JSON 解析失败：${e.message}`, 'err')
      return
    }
    setSaving(true)
    setValidationErrors([])
    try {
      const r = await api.saveSpec(ontologyId, parsed)
      showToast(`Spec 已保存（version ${r.version}）`)
      onSaved()
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) {
        setValidationErrors(e.validationErrors)
        showToast('校验未通过，请修正后重试', 'err')
      } else {
        showToast(e.message, 'err')
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="onto-sec" style={{ marginTop: 0 }}>
        <span className="onto-sec-title">Spec JSON · {ontoName}</span>
        <span className="hit-spacer" />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {spec ? `概念 ${spec.concepts?.length ?? 0} · 关系 ${spec.relations?.length ?? 0} · 实例 ${spec.instances?.length ?? 0}` : '尚未保存过 Spec'}
        </Typography.Text>
        <Button size="small" type="primary" loading={saving} disabled={!specText} onClick={save}>
          保存 Spec
        </Button>
        <Button size="small" onClick={onNext}>
          下一步：校验
        </Button>
      </div>
      <Input.TextArea
        className="onto-spec-editor"
        value={specText}
        onChange={(e) => setSpecText(e.target.value)}
        autoSize={{ minRows: 16, maxRows: 40 }}
        spellCheck={false}
        placeholder='{ "name": "…", "concepts": [], "relations": [], "instances": [] }'
      />
      {validationErrors.length > 0 && (
        <>
          <Alert type="error" showIcon style={{ marginTop: 12 }} title={`校验未通过（${validationErrors.length} 项）`} />
          <Table<ValidationError>
            rowKey={(r) => `${r.path}::${r.message}`}
            columns={ERR_COLUMNS} scroll={{ x: 'max-content' }}
            dataSource={validationErrors}
            pagination={false}
            size="small"
            style={{ marginTop: 8 }}
          />
        </>
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// S3 校验（自定义构建内简化版）
// ---------------------------------------------------------------------------

function S3ValidatePane({ ontologyId, onNext }: { ontologyId: string; onNext: () => void }) {
  const { showToast } = useUI()
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; errors: ValidationError[] } | null>(null)

  const run = async () => {
    setBusy(true)
    try {
      const r = await api.validateOntology(ontologyId)
      const res = { ok: !!r.ok, errors: r.validation_errors ?? [] }
      setResult(res)
      showToast(res.ok ? '校验通过' : `校验发现 ${res.errors.length} 个问题`, res.ok ? 'ok' : 'err')
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Space>
        <Button type="primary" icon={<CheckCircleOutlined />} loading={busy} onClick={run}>
          运行校验
        </Button>
        {result && (result.ok ? <Tag color="success" style={{ margin: 0 }}>通过</Tag> : <Tag color="error" style={{ margin: 0 }}>未通过 · {result.errors.length}</Tag>)}
        <Button onClick={onNext}>
          下一步：可视化
        </Button>
      </Space>
      <p className="onto-detail-empty">JSON Schema + 引用完整性校验（保存时同样自动执行；此处可手动复跑）。</p>
      {result?.ok && (
        <Alert type="success" showIcon style={{ marginTop: 4 }} title="校验通过" description="可进入 S4 可视化；运行部署到「本体运行」栏。" />
      )}
      {result && result.errors.length > 0 && (
        <Table<ValidationError>
          rowKey={(r) => `${r.path}::${r.message}`}
          columns={ERR_COLUMNS} scroll={{ x: 'max-content' }}
          dataSource={result.errors}
          pagination={false}
          size="small"
          style={{ marginTop: 8 }}
        />
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// OntoChat 流程：完整交互见 OntoChatFlow.tsx（REQ-103 模式 A 已交付）
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 由知识库构建（第六路径独立流程页 KbBuildFlow.tsx，O13 已交付；S1 来源第 5 项跳转至此）
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// KG 消费流程（入口卡，D-O15 改造：原 semantica 流程入口，改跳「消费与审计」第五栏）
// ---------------------------------------------------------------------------

function KgGuide() {
  return (
    <Card className="work-card" size="small">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="KG 消费流程——自研 KG 图谱 / GraphRAG 试查 / 决策溯源（「消费与审计」栏承载）"
        description="知识库 chunk 语料经 REQ-98 LLM 能力代理抽取为自存 KG（SQLite 实体/关系/claim，D-O15），再消费为 GraphRAG 检索上下文或由 KG 直转本体（策略 B/C）；抽取/构建决策全程留痕可溯源。原 semantica worker 依赖已归档休眠。"
      />
      <div className="onto-sec" style={{ marginTop: 0 }}>
        <span className="onto-sec-title">与主线边界</span>
      </div>
      <ul className="onto-report-list">
        <li>本体建模与校验：归主线构建平面（本栏）</li>
        <li>KG 图谱浏览 / GraphRAG 试查 / 决策审计：归「消费与审计」第五栏</li>
        <li>KG→本体：本栏「由知识库构建」策略 B（kg-direct）/ C（hybrid）直读自存 KG</li>
      </ul>
      <Space style={{ marginTop: 12 }}>
        <Button
          type="primary"
          icon={<ApiOutlined />}
          onClick={() => {
            // 栏内切栏：sidebarKey 状态机经 localStorage + 事件同步（OntologyModule）
            localStorage.setItem('eino.onto.sidebar', 'audit')
            window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
          }}
        >
          前往消费与审计栏
        </Button>
      </Space>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// OntoExtend 流程（引导卡先行，工程化登记需求池）
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CSV 灌装（REQ-96 P2a 同名映射 + P2b 映射向导）在资产栏「CSV 灌装」页签（CsvIngestPane）
// ---------------------------------------------------------------------------
