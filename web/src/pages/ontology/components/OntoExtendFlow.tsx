import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Checkbox, Empty, Input, InputNumber, Segmented, Select, Space, Spin, Steps, Table, Tag, Tooltip, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined, LeftOutlined, RightOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import DoneCTA from './DoneCTA'
import LoadErrorAlert from '../../../components/LoadErrorAlert'
import PathIntro from '../build/PathIntro'
import { loadDraft, saveDraft, clearDraft } from '../../../lib/draftStore'
import { useOntologies } from '../shared/useOntoList'

// ---------------------------------------------------------------------------
// M-O14 P2②（REQ-171 P2/26 号方案 §9）+ REQ-343/OE-2 双模式演化动线：
//   模式一 ODP 模式扩展（既有）：人工精选 ODP + LOV 词表 → 静态片段 → merge 审查。
//   模式二 CQ 驱动扩展（REQ-343/OE-1 论文三段式 RAG）：spec.CQ → 元素级向量检索 top-k →
//     LLM 生成 spec 片段（双 profile+两阶段校验，失败标记不静默入库）→ 去重 → merge 审查。
// 两模式共用目标本体选择与 merge/preview|apply 审查入库底座（REQ-157）。
// ---------------------------------------------------------------------------

interface OdpItem {
  id: string
  name: string
  description: string
  source: string
  spec: { concepts: { name: string; label?: string; definition?: string }[]; relations: { name: string; from: string; to: string }[] }
}
interface MergePreviewData {
  strategy: string
  /** 实测形态：扁平标记数组（'concept:组织' / 'relation:隶属于'）；conflicts/renamed 为对象数组 */
  added?: string[]
  renamed?: unknown[]
  conflicts?: unknown[]
}
/** REQ-343：CQ 驱动扩展片段结果（job.rag.fragments[]） */
interface RagFragment {
  cq: string
  status: 'ok' | 'failed'
  fragment?: { concepts?: unknown[]; relations?: unknown[]; data_properties?: unknown[]; axioms?: unknown[] }
  error?: string
  rounds?: number
  used?: { element: { local_name: string; kind: string; label?: string }; score: number }[]
}
interface RagJobData {
  fragments: RagFragment[]
  draft_spec?: { name?: string; concepts?: unknown[]; relations?: unknown[]; data_properties?: unknown[]; axioms?: unknown[] }
  removed?: string[]
  retrieval: { elements_indexed: number; top_k: number; profile: string; cq_total: number; cq_ok: number; cq_failed: number; elements_used: number }
  quality?: { score?: { overall: number }; error_count: number; warning_count: number }
  /** REQ-344/OE-5：CQ 质量预检信号（可答性/原子性/具体性；信号非闸门） */
  precheck?: {
    items: { cq: string; answerable: boolean; atomic: boolean; specific: boolean; note?: string }[]
    not_answerable: number
    not_atomic: number
    not_specific: number
    note?: string
  }
}

/** REQ-339：资产详情「演化」动作 handoff 键（携本体 id 跳转本页，消费后置空）。 */
export const ASSET_EXTEND_KEY = 'eino.onto.asset.extend'

type ExtendMode = 'odp' | 'cqrag'

/** REQ-466/M266：演化流进度持久化（拉齐 custom 口径——step/targetId/mode 随 draftStore，刷新回原步骤） */
type ExtendProgress = { step: number; targetId?: string; mode?: ExtendMode }

export default function OntoExtendFlow() {
  const [mode, setMode] = useState<ExtendMode>('odp')
  // REQ-467/M267：本体清单单源 hook（原自有拉取+ontosErr 收敛；重试走 reload）
  const { ontos, err: ontosErr, reload: reloadOntos } = useOntologies()
  const [targetId, setTargetId] = useState<string | undefined>(() => loadDraft<ExtendProgress>('extend-progress')?.targetId)

  // ---- ODP 模式状态（REQ-171 既有）----
  const [odps, setOdps] = useState<OdpItem[]>([])
  const [odpsLoading, setOdpsLoading] = useState(false)
  const [odpsErr, setOdpsErr] = useState<string | null>(null)
  const [odpId, setOdpId] = useState<string | undefined>(undefined)
  const [lovQuery, setLovQuery] = useState('')
  const [lovHits, setLovHits] = useState<{ label: string; uri: string; desc?: string }[]>([])
  const [lovLoading, setLovLoading] = useState(false)
  const [lovErr, setLovErr] = useState<string | null>(null)
  const [lovPicked, setLovPicked] = useState<Record<string, { label: string; uri: string; desc?: string }>>({})

  // ---- CQ 驱动模式状态（REQ-343/OE-1）----
  const [targetCQs, setTargetCQs] = useState<string>('')
  const [profile, setProfile] = useState<'structure' | 'restriction'>('structure')
  const [topK, setTopK] = useState<number>(20)
  const [ragBusy, setRagBusy] = useState(false)
  const [ragProgress, setRagProgress] = useState('')
  const [ragErr, setRagErr] = useState<string | null>(null)
  const [ragData, setRagDataState] = useState<RagJobData | null>(() => loadDraft<RagJobData>('extend-rag-data')) // REQ-402：草稿持久化
  const setRagData = (v: RagJobData | null) => {
    setRagDataState(v)
    if (v === null) clearDraft('extend-rag-data')
    else saveDraft('extend-rag-data', v)
  }

  const [preview, setPreviewState] = useState<MergePreviewData | null>(() => loadDraft<MergePreviewData>('extend-preview'))
  const setPreview = (v: MergePreviewData | null) => {
    setPreviewState(v)
    if (v === null) clearDraft('extend-preview')
    else saveDraft('extend-preview', v)
  }
  const previewBaseVersionRef = useRef<number>(0) // REQ-293②：预览时点版本（apply 回传防陈旧）
  const [previewLoading, setPreviewLoading] = useState(false)
  const [applyErr, setApplyErr] = useState<string | null>(null)
  const [applying, setApplying] = useState(false)
  const [appliedVersion, setAppliedVersion] = useState<number | null>(null)
  // REQ-466：恢复钳位防孤儿步骤——审查步需预览/生成草稿在场（apply 成功后 REQ-459 已清材料，
  // 刷新回落材料步），材料步需目标本体在场
  const [step, setStep] = useState<number>(() => {
    const p = loadDraft<ExtendProgress>('extend-progress')
    const s = p?.step ?? 0
    if (s >= 1 && !p?.targetId) return 0
    if (s >= 2 && !loadDraft('extend-preview') && !loadDraft('extend-rag-data')) return 1
    return Math.max(0, Math.min(2, s))
  })

  // REQ-466：进度持久化（targetId/mode 随存——刷新回原步骤；custom 口径拉齐）
  useEffect(() => {
    if (step === 0 && !targetId) clearDraft('extend-progress')
    else saveDraft('extend-progress', { step, targetId, mode } satisfies ExtendProgress)
  }, [step, targetId, mode])

  // （清单加载收敛至 useOntologies）

  // REQ-339：资产详情「演化」动作携 id 直达——预选目标本体并进 ODP 步（消费后置空）
  useEffect(() => {
    const id = localStorage.getItem(ASSET_EXTEND_KEY)
    if (id) {
      localStorage.removeItem(ASSET_EXTEND_KEY)
      setTargetId(id)
      setPreview(null)
      setAppliedVersion(null)
      setStep(1)
    }
  }, [])

  // CQ 模式：目标本体就绪后拉 spec.CQ 预填（REQ-248 CQ 一等公民直接作需求输入）
  useEffect(() => {
    if (mode !== 'cqrag' || !targetId) return
    api
      .getSpec(targetId)
      .then((sp: any) => setTargetCQs(Array.isArray(sp?.cq) && sp.cq.length ? sp.cq.map((c: string) => String(c)).join('\n') : ''))
      .catch(() => setTargetCQs(''))
    setRagData(null)
    setRagErr(null)
  }, [mode, targetId])

  const loadOdps = useCallback(() => {
    setOdpsLoading(true)
    api
      .ontoExtendODPs()
      .then((d) => {
        setOdps((d.odps ?? []) as OdpItem[])
        setOdpsErr(null)
      })
      .catch((e: any) => setOdpsErr(e?.message ?? 'ODP 加载失败'))
      .finally(() => setOdpsLoading(false))
  }, [])
  useEffect(() => {
    loadOdps()
  }, [loadOdps])

  const odp = useMemo(() => odps.find((o) => o.id === odpId) ?? null, [odps, odpId])

  const searchLov = useCallback(() => {
    const q = lovQuery.trim()
    if (!q) return
    setLovLoading(true)
    setLovErr(null)
    api
      .vocabSearch(q)
      .then((d) => {
        const results = (d.results ?? d.items ?? []) as never[]
        setLovHits(results.slice(0, 12))
      })
      .catch((e: any) => setLovErr(e?.message ?? 'LOV 检索失败（上游不可达时为预期降级）'))
      .finally(() => setLovLoading(false))
  }, [lovQuery])

  /** ODP 模式草稿 = ODP 片段 + LOV 勾选术语（作为附加概念并入 spec 后送 merge 审查） */
  const buildDraftSpec = useCallback(() => {
    if (!odp) return null
    const spec: any = {
      name: odp.spec.concepts?.length ? `${odp.name}扩展片段` : odp.name,
      concepts: [...(odp.spec.concepts ?? [])],
      relations: [...(odp.spec.relations ?? [])],
      instances: [],
    }
    for (const term of Object.values(lovPicked)) {
      spec.concepts.push({ name: term.label, label: term.label, definition: term.desc || `LOV 词表术语（${term.uri}）` })
    }
    return spec
  }, [odp, lovPicked])

  /** CQ 模式草稿 = 三段式生成产物（去重合并片段；两阶段校验在生成段已完成） */
  const buildRagDraftSpec = useCallback(() => (ragData?.draft_spec ?? null), [ragData])

  /** 发起 CQ 驱动生成（202+job 轮询，复用 aiDraftJob 轮询通道 job.rag） */
  const runRag = async () => {
    if (!targetId) return
    const cqs = targetCQs.split('\n').map((s) => s.trim()).filter(Boolean)
    if (!cqs.length) {
      setRagErr('请至少填写一条能力问题（CQ）——CQ 驱动扩展以能力问题为需求输入')
      return
    }
    setRagBusy(true)
    setRagErr(null)
    setRagData(null)
    setRagProgress('任务提交中…')
    try {
      const j = await api.cqragDraft(targetId, cqs, profile, topK)
      const timer = setInterval(async () => {
        try {
          const job: any = await api.aiDraftJob(j.job_id)
          if (job.progress) setRagProgress(job.progress)
          if (job.status === 'done' || job.status === 'failed') {
            clearInterval(timer)
            setRagBusy(false)
            setRagProgress('')
            if (job.status === 'failed') setRagErr(job.error ?? '生成失败')
            else if (job.rag) setRagData(job.rag as RagJobData)
          }
        } catch {
          /* 单次轮询失败下次重试 */
        }
      }, 1500)
    } catch (e: any) {
      setRagBusy(false)
      setRagProgress('')
      setRagErr(e?.message ?? '提交失败')
    }
  }

  const doPreview = async () => {
    if (!targetId) return
    const spec = mode === 'cqrag' ? buildRagDraftSpec() : buildDraftSpec()
    if (!spec) return
    const filename = mode === 'cqrag' ? 'cqrag-fragments.json' : `ontoextend-${odp?.id ?? 'draft'}.json`
    setPreviewLoading(true)
    setApplyErr(null)
    setAppliedVersion(null)
    try {
      const pv = await api.mergePreview(targetId, { filename, spec, strategy: 'merge' })
      previewBaseVersionRef.current = pv.base_version ?? 0 // REQ-293②：apply 回传防陈旧（0=跳过校验）
      setPreview(pv as unknown as MergePreviewData)
      setStep(2)
    } catch (e: any) {
      setApplyErr(e?.message ?? '审查预览失败')
    } finally {
      setPreviewLoading(false)
    }
  }

  const doApply = async () => {
    if (!targetId) return
    const spec = mode === 'cqrag' ? buildRagDraftSpec() : buildDraftSpec()
    if (!spec) return
    const filename = mode === 'cqrag' ? 'cqrag-fragments.json' : `ontoextend-${odp?.id ?? 'draft'}.json`
    setApplying(true)
    setApplyErr(null)
    try {
      const res = await api.mergeApply(targetId, { filename, spec, strategy: 'merge', base_version: previewBaseVersionRef.current })
      setAppliedVersion(res.version ?? null)
      setPreview(null)
      // REQ-459：入库即新会话——本轮材料态清空（extend-rag-data 草稿同清），回看旧会话不残留；
      // 目标本体/模式选择保留（工作对象连续性，与 S1 来源 tab 记忆同口径）
      setRagData(null)
      setLovPicked({})
      setOdpId(undefined)
      setTargetCQs('')
    } catch (e: any) {
      setApplyErr(e?.message ?? '入库失败（strict 质量门禁可能拦截：见错误详情）')
    } finally {
      setApplying(false)
    }
  }

  const addedList = useMemo(() => (Array.isArray(preview?.added) ? (preview!.added as string[]) : []), [preview])
  const addedConcepts = addedList.filter((s) => s.startsWith('concept:')).length
  const addedRelations = addedList.filter((s) => s.startsWith('relation:')).length
  const conflictCount = (preview?.conflicts?.length ?? 0) + (preview?.renamed?.length ?? 0)

  const stepItems =
    mode === 'cqrag'
      ? [{ title: '① 选目标本体' }, { title: '② CQ 与生成' }, { title: '③ 审查入库' }] // REQ-405②：流程语言统一
      : [{ title: '① 选目标本体' }, { title: '② ODP + 词表' }, { title: '③ 审查入库' }]

  return (
    <Card className="work-card" size="small">
      {/* REQ-485/M284：开场 Alert 降格一句目的 */}
      <PathIntro
        purpose="对已交付本体做增量扩展，不新建本体；扩展片段经审查与质量门禁后并入新版本。"
        detail="ODP 模式扩展=人工精选设计模式与 LOV 词表补全；CQ 驱动扩展=以能力问题为需求输入，检索目标本体相关元素（RAG）后由模型生成扩展片段（两阶段校验，失败不静默入库）。也可从「本体资产」详情页点「演化」携本体直达。"
      />
      <div style={{ marginBottom: 12 }} data-testid="extend-mode-seg">
        <Segmented
          value={mode}
          onChange={(v) => {
            setMode(v as ExtendMode)
            setPreview(null)
            setAppliedVersion(null)
            setStep((s) => (s === 0 ? 0 : 1))
          }}
          options={[
            { value: 'odp', label: 'ODP 模式扩展' },
            { value: 'cqrag', label: 'CQ 驱动扩展（RAG）' },
          ]}
        />
      </div>
      {/* REQ-337/M137 三步页化 + REQ-343 模式化 Steps——可点跳步，每步独立子页 */}
      <Steps size="small" current={step} onChange={setStep} items={stepItems} style={{ marginBottom: 12 }} />

      {step === 0 && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">① 目标本体（扩展并入对象）</span>
          </div>
          {ontosErr ? (
            <LoadErrorAlert title="本体列表加载失败" message={ontosErr} onRetry={reloadOntos} style={{ marginBottom: 8 }} />
          ) : (
            <Select
              showSearch
              optionFilterProp="label"
              style={{ width: 320 }}
              placeholder="选择要扩展的本体"
              value={targetId}
              onChange={(v) => {
                setTargetId(v)
                setPreview(null)
                setAppliedVersion(null)
                if (v && step === 0) setStep(1)
              }}
              options={ontos.map((o) => ({ value: o.id, label: o.name }))}
              notFoundContent={ontos.length === 0 ? '暂无本体' : undefined}
            />
          )}
          {!targetId && (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="先选择目标本体——演化对既有本体做增量扩展，不新建" />
          )}
          <Space style={{ marginTop: 12 }}>
            <Button type="primary" disabled={!targetId} onClick={() => setStep(1)}>
              下一步：{mode === 'cqrag' ? '配置 CQ 与生成' : '选择扩展模式'} <RightOutlined />
            </Button>
          </Space>
        </>
      )}

      {step === 1 && mode === 'odp' && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">
              ② ODP 精选推荐（人工精选 12 模式）
              <Button size="small" type="text" onClick={loadOdps} style={{ marginLeft: 6 }}>刷新</Button>
            </span>
          </div>
          {odpsLoading ? (
            <Spin size="small" />
          ) : odpsErr ? (
            <LoadErrorAlert title="ODP 加载失败" message={odpsErr} onRetry={loadOdps} />
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: 8 }}>
              {odps.map((o) => (
                <Card
                  key={o.id}
                  size="small"
                  hoverable
                  onClick={() => {
                    setOdpId(o.id)
                    setPreview(null)
                  }}
                  style={{ borderColor: odpId === o.id ? 'var(--ant-color-primary, #4f46e5)' : undefined }}
                >
                  <div style={{ fontWeight: 600, fontSize: 13 }}>
                    {o.name} {odpId === o.id && <Tag color="processing" style={{ marginInlineStart: 4 }}>已选</Tag>}
                  </div>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 4 }}>{o.description}</Typography.Paragraph>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    {o.spec.concepts?.length ?? 0} 概念 · {o.spec.relations?.length ?? 0} 关系 ｜ 出处：{o.source}
                  </Typography.Text>
                </Card>
              ))}
            </div>
          )}

          <div className="onto-sec">
            <span className="onto-sec-title">③ LOV 词表卡片（可选补充术语）</span>
          </div>
          <Space.Compact style={{ width: 'min(520px, 100%)', marginBottom: 6 }}>
            <Input
              placeholder="检索 LOV 词表术语（如 prov、time）——上游不可达时优雅降级"
              value={lovQuery}
              onChange={(e) => setLovQuery(e.target.value)}
              onPressEnter={searchLov}
              allowClear
            />
            <Button onClick={searchLov} loading={lovLoading}>检索</Button>
          </Space.Compact>
          {lovErr && <LoadErrorAlert title="LOV 检索失败" message={lovErr} onRetry={searchLov} style={{ marginBottom: 6 }} />}
          {lovHits.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 8, maxHeight: '30vh', overflowY: 'auto' }}>
              {lovHits.map((h) => (
                <Checkbox
                  key={h.uri}
                  checked={!!lovPicked[h.uri]}
                  onChange={(e) =>
                    setLovPicked((cur) => {
                      const next = { ...cur }
                      if (e.target.checked) next[h.uri] = h
                      else delete next[h.uri]
                      return next
                    })
                  }
                >
                  <span style={{ fontSize: 12 }}>{h.label}</span>
                  <Typography.Text type="secondary" style={{ fontSize: 11, marginInlineStart: 6 }}>{h.uri}</Typography.Text>
                </Checkbox>
              ))}
            </div>
          )}
          {Object.keys(lovPicked).length > 0 && (
            <div style={{ marginBottom: 8 }}>
              已选术语：{Object.values(lovPicked).map((t) => <Tag key={t.uri} color="geekblue">{t.label}</Tag>)}
            </div>
          )}

          <Space wrap style={{ marginTop: 12 }}>
            <Button onClick={() => setStep(0)} icon={<LeftOutlined />}>
              上一步
            </Button>
            <Button type="primary" disabled={!targetId || !odp} loading={previewLoading} onClick={doPreview}>
              生成扩展草稿并审查 <RightOutlined />
            </Button>
            {!odp && <Typography.Text type="secondary" style={{ fontSize: 12 }}>请先选择一个 ODP 模式</Typography.Text>}
          </Space>
        </>
      )}

      {step === 1 && mode === 'cqrag' && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">② 能力问题与生成参数（REQ-343 论文三段式）</span>
          </div>
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 8 }}>
            <div style={{ flex: 1, minWidth: 280 }}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                CQ 清单（每行一条；已预填目标本体 spec.CQ——可增删改，只影响本次扩展不改回本体）
              </Typography.Text>
              <Input.TextArea
                rows={4}
                value={targetCQs}
                onChange={(e) => setTargetCQs(e.target.value)}
                placeholder={'如：药物可治疗哪些疾病？\n哪种症状提示重症？'}
                style={{ marginTop: 4, fontFamily: 'inherit' }}
                data-testid="cqrag-cqs-input"
              />
            </div>
            <div style={{ width: 220 }}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>建模 profile</Typography.Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={profile}
                onChange={setProfile}
                options={[
                  { value: 'structure', label: '结构型（声明齐备）' },
                  { value: 'restriction', label: '限制型（层级+公理）' },
                ]}
                data-testid="cqrag-profile"
              />
              <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>检索 top-k</Typography.Text>
              <InputNumber min={5} max={50} value={topK} onChange={(v) => setTopK(v ?? 20)} style={{ width: '100%', marginTop: 4 }} />
              <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
                论文口径：检索目标本体元素级片段作只读上下文（默认 20）；生成片段经两阶段校验（结构校验+治理约束），失败标记不静默入库。
              </Typography.Text>
            </div>
          </div>
          <Space wrap style={{ marginBottom: 8 }}>
            <Button onClick={() => setStep(0)} icon={<LeftOutlined />}>上一步</Button>
            <Button type="primary" loading={ragBusy} disabled={ragBusy || !targetId} onClick={runRag} data-testid="cqrag-run-btn">
              生成扩展片段
            </Button>
            {ragData && (
              <Button type="primary" ghost loading={previewLoading} onClick={doPreview} data-testid="cqrag-review-btn">
                送审查入库 <RightOutlined />
              </Button>
            )}
          </Space>
          {ragProgress && (
            <div style={{ marginBottom: 8 }}>
              <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>{ragProgress}</Typography.Text>
            </div>
          )}
          {ragErr && <LoadErrorAlert title="CQ 驱动扩展失败" message={ragErr} onRetry={runRag} style={{ marginBottom: 8 }} />}
          {ragData && (
            <>
              <Card size="small" style={{ marginBottom: 8 }} data-testid="cqrag-result-summary">
                <Space size={14} wrap>
                  <Typography.Text strong>生成结果</Typography.Text>
                  <Tag color="blue">元素索引 {ragData.retrieval.elements_indexed}</Tag>
                  <Tag color="geekblue">top-k {ragData.retrieval.top_k}</Tag>
                  <Tag color={ragData.retrieval.cq_failed > 0 ? 'orange' : 'green'}>
                    CQ 成功 {ragData.retrieval.cq_ok}/{ragData.retrieval.cq_total}
                    {ragData.retrieval.cq_failed > 0 ? `（失败 ${ragData.retrieval.cq_failed} 已标记）` : ''}
                  </Tag>
                  {ragData.removed?.length ? <Tag color="default">去重剔除 {ragData.removed.length}</Tag> : null}
                  {ragData.precheck && (
                    <Tooltip title={ragData.precheck.items.filter((i) => !i.answerable || !i.atomic || !i.specific).map((i) => `${i.cq}：${i.note ?? '质量信号未过'}`).join('；') || '全部 CQ 预检通过'}>
                      <Tag color={ragData.precheck.not_answerable + ragData.precheck.not_atomic + ragData.precheck.not_specific > 0 ? 'orange' : 'green'} data-testid="cqrag-precheck-tag">
                        预检 不可答 {ragData.precheck.not_answerable} / 非原子 {ragData.precheck.not_atomic} / 不具体 {ragData.precheck.not_specific}
                      </Tag>
                    </Tooltip>
                  )}
                  {ragData.quality && (
                    <Tag color={ragData.quality.error_count > 0 ? 'red' : 'green'}>质量 err {ragData.quality.error_count} / warn {ragData.quality.warning_count}</Tag>
                  )}
                </Space>
                <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
                  失败片段不静默入库（论文不变式：失败要么重试要么标记）；合并片段已逐条去重，审查入库走导入合并底座（strict 门禁+版本快照）。
                </Typography.Paragraph>
              </Card>
              <Table
                size="small"
                rowKey={(r) => r.cq}
                dataSource={ragData.fragments}
                pagination={{ pageSize: 10, hideOnSinglePage: true }}
                data-testid="cqrag-fragments-table"
                columns={[
                  { title: '能力问题', dataIndex: 'cq', ellipsis: true },
                  {
                    title: '状态',
                    dataIndex: 'status',
                    width: 90,
                    render: (v: string) =>
                      v === 'ok' ? <Tag color="green" icon={<CheckCircleOutlined />}>ok</Tag> : <Tag color="red" icon={<CloseCircleOutlined />}>failed</Tag>,
                  },
                  {
                    title: '产出',
                    width: 190,
                    render: (_: unknown, r: RagFragment) =>
                      r.fragment ? (
                        <Typography.Text style={{ fontSize: 12 }}>
                          概念 {r.fragment.concepts?.length ?? 0} · 关系 {r.fragment.relations?.length ?? 0}
                          {r.fragment.data_properties?.length ? ` · 数据属性 ${r.fragment.data_properties.length}` : ''}
                          {r.fragment.axioms?.length ? ` · 公理 ${r.fragment.axioms.length}` : ''}
                        </Typography.Text>
                      ) : (
                        <Typography.Text type="secondary" style={{ fontSize: 11 }}>{r.error?.slice(0, 120)}</Typography.Text>
                      ),
                  },
                  {
                    title: '检索命中',
                    dataIndex: 'used',
                    ellipsis: true,
                    render: (used: RagFragment['used']) =>
                      used?.length ? (
                        <span>
                          {used.slice(0, 4).map((u) => (
                            <Tag key={u.element.local_name} style={{ marginInlineEnd: 4, fontSize: 11 }}>
                              {u.element.local_name} {u.score.toFixed(2)}
                            </Tag>
                          ))}
                          {used.length > 4 ? <Typography.Text type="secondary" style={{ fontSize: 11 }}>+{used.length - 4}</Typography.Text> : null}
                        </span>
                      ) : (
                        <Typography.Text type="secondary" style={{ fontSize: 11 }}>无命中（全新建模）</Typography.Text>
                      ),
                  },
                ]}
              />
            </>
          )}
        </>
      )}

      {step === 2 && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">③ 审查入库（strategy=merge，REQ-157 底座）</span>
          </div>
          {applyErr && <LoadErrorAlert title="演化操作失败" message={applyErr} onRetry={() => setApplyErr(null)} style={{ marginBottom: 8 }} />}
          {preview ? (
            <>
              <Card size="small" style={{ marginBottom: 8 }}>
                <div style={{ fontWeight: 600, marginBottom: 4 }}>审查报告</div>
                <Space size={16} wrap>
                  <Tag color="green">新增概念 {addedConcepts}</Tag>
                  <Tag color="geekblue">新增关系 {addedRelations}</Tag>
                  <Tag color={conflictCount > 0 ? 'orange' : 'default'}>冲突/改名 {conflictCount}</Tag>
                </Space>
                {conflictCount > 0 && (
                  <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 6 }}>
                    存在撞名冲突/改名项——应用后按 merge 策略自动改名并入（前缀避让），详情见资产栏版本 diff。
                  </Typography.Paragraph>
                )}
              </Card>
              <Space wrap style={{ marginBottom: 8 }}>
                <Button onClick={() => setStep(1)} icon={<LeftOutlined />}>
                  上一步{mode === 'cqrag' ? '（调整 CQ/参数）' : '（调整模式/词表）'}
                </Button>
                <Button type="primary" ghost loading={applying} onClick={doApply}>
                  应用入库（新版本）
                </Button>
              </Space>
            </>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未生成审查预览——回上一步发起" />
          )}
          {appliedVersion !== null && (
            <Alert
              type="success"
              showIcon
              title={`已并入目标本体新版本（v${appliedVersion}）——strict 质量门禁通过`}
              description={<DoneCTA ontologyId={targetId ?? ''} detail="扩展完成" />}
            />
          )}
        </>
      )}
    </Card>
  )
}
