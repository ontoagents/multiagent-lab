import { ConvergeCard } from './build/PathShell'
import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Card, Empty, Input, Radio, Space, Steps, Table, Tag, Tooltip, Typography } from 'antd'
import { CheckCircleOutlined, DatabaseOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../api/client'
import type { OntoBuildSelectableKB, OntoBuildResult, ValidationError } from '../../api/types'
import { useUI } from '../../store/ui'
import { ERR_COLUMNS } from './shared'
import { loadDraft, saveDraft, clearDraft } from '../../lib/draftStore'
import { invalidateOntos } from './shared/useOntoList'

// ---------------------------------------------------------------------------
// 由知识库构建本体——独立流程页（O13，D-O14/REQ-108，04 §3.7）：
//   KB 选择器（mode 徽标 + chunk/KG 规模）→ 抽取策略（A chunk→LLM / B KG→直转 / C 混合）
//   → CQ 引导（REQ-90：auto / custom / skip）→ 生成草稿（spec_json + 校验报告）→ 预览调优 → 入库
// 与自定义构建 S1~S4 互不共用页面；产物入库走构建平面（POST /api/ontologies + PUT spec）。
// ---------------------------------------------------------------------------

// REQ-405②：流程语言统一——步名带序号（①~④，四流程同款）
const STEPS = [
  { key: 'kb', title: '① 选择知识库' },
  { key: 'config', title: '② 策略与 CQ' },
  { key: 'preview', title: '③ 生成与预览' },
  { key: 'done', title: '④ 入库' },
]

const MODE_TAG: Record<string, { color: string; text: string }> = {
  rag: { color: 'blue', text: 'RAG' },
  graphrag: { color: 'purple', text: 'GraphRAG' },
  wiki: { color: 'geekblue', text: 'LLM Wiki' }, // REQ-241/M67 第三类型（顺修：缺失时 mt undefined 渲染崩）
}

const STRATEGY_HINT: Record<string, string> = {
  'chunk-llm': '策略 A：chunk 语料 → LLM 抽取 spec_json（复用 REQ-82，可即时上线；需模型连接）',
  'kg-direct': '策略 B：GraphRAG KG 直转（entity→Concept / relation→Relation 薄映射，不做抽取；保真度依赖抽取质量）',
  hybrid: '策略 C：混合——KG 作初稿 + LLM 校验补全 definition / 缺失关系',
}

const STRATEGY_NAME: Record<string, string> = {
  'chunk-llm': 'A · chunk→LLM',
  'kg-direct': 'B · KG→直转',
  hybrid: 'C · 混合',
}

function KbBuildFlow() {
  const { showToast, setPage } = useUI()
  // REQ-466/M266：进度持久化（{step, kbId}，custom 口径拉齐）+ 恢复钳位防孤儿步骤——
  // 生成预览步需 result 草稿在场（入库成功 REQ-459 已清草稿，刷新回落策略步），策略步需 kbId 在场
  const initialProgress = loadDraft<{ step: number; kbId?: string | null }>('kb-progress')
  const initialResult = loadDraft<OntoBuildResult>('kb-flow-result')
  const [step, setStep] = useState<number>(() => {
    const s = initialProgress?.step ?? 0
    if (s >= 1 && !initialProgress?.kbId) return 0
    if (s >= 2 && !initialResult) return 1
    return Math.max(0, Math.min(3, s))
  })
  const [kbs, setKbs] = useState<OntoBuildSelectableKB[]>([])
  const [kbLoading, setKbLoading] = useState(false)
  const [kbId, setKbId] = useState<string | null>(initialProgress?.kbId ?? null)
  const [buildingKG, setBuildingKG] = useState<string | null>(null)

  const [strategy, setStrategy] = useState<'chunk-llm' | 'kg-direct' | 'hybrid'>('chunk-llm')
  const [cqMode, setCqMode] = useState<'auto' | 'custom' | 'skip'>('auto')
  const [cqText, setCqText] = useState('')

  const [busy, setBusy] = useState(false)
  const [result, setResultState] = useState<OntoBuildResult | null>(initialResult) // REQ-402：草稿持久化
  const setResult = (v: OntoBuildResult | null) => {
    setResultState(v)
    if (v === null) clearDraft('kb-flow-result')
    else saveDraft('kb-flow-result', v)
  }
  const [specText, setSpecText] = useState('')
  const [ontoName, setOntoName] = useState('')
  const [saving, setSaving] = useState(false)
  const [savedId, setSavedId] = useState<string | null>(null)
  // REQ-459：入库成功后的展示名（表单态随新会话清空，展示不依赖残留表单）
  const [savedName, setSavedName] = useState('')

  // REQ-466：进度持久化；REQ-459 完成即新会话——入库成功（savedId 置位）后进度不再持久
  // （KB 选择保留在会话内存，刷新即全新会话回第①步）
  useEffect(() => {
    if (savedId) {
      clearDraft('kb-progress')
      return
    }
    if (step === 0 && !kbId) clearDraft('kb-progress')
    else saveDraft('kb-progress', { step, kbId })
  }, [step, kbId, savedId])

  const selected = useMemo(() => kbs.find((k) => k.id === kbId) ?? null, [kbs, kbId])

  const loadKBs = (keep?: string | null) => {
    setKbLoading(true)
    api
      .selectableKBsForBuild()
      .then((list) => {
        setKbs(list)
        if (keep && list.some((k) => k.id === keep)) setKbId(keep)
      })
      .catch((e) => showToast(e.message, 'err'))
      .finally(() => setKbLoading(false))
  }
  useEffect(() => loadKBs(), [])

  const buildKG = async (id: string) => {
    setBuildingKG(id)
    try {
      const r = await api.chunksToKG(id)
      if (r.graphrag?.degraded) showToast(`KG 构建降级：${r.graphrag.error ?? 'worker 不可达'}`, 'err')
      else showToast(`KG 已构建：${r.graphrag?.entities ?? 0} 实体 / ${r.graphrag?.relationships ?? 0} 关系`)
      loadKBs(id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBuildingKG(null)
    }
  }

  const generate = async () => {
    if (!kbId) {
      showToast('请先选择知识库', 'err')
      return
    }
    const cqs = cqText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    if (cqMode === 'custom' && cqs.length === 0) {
      showToast('自定义 CQ 模式需至少输入一个问题', 'err')
      return
    }
    setBusy(true)
    setResult(null)
    try {
      const r = await api.buildFromKB({
        kb_id: kbId,
        strategy,
        cq_mode: cqMode,
        custom_cqs: cqMode === 'custom' ? cqs : undefined,
      })
      setResult(r)
      setSpecText(JSON.stringify(r.spec_json, null, 2))
      setOntoName(r.spec_json?.name ?? '')
      setStep(2)
      // REQ-267/M76：统一质量快评随 toast 透出（degraded=构建平面离线仅本地结构校验）
      const q = r.quality
      showToast(
        r.validation_report?.ok
          ? `草稿已生成且校验通过${q && !q.degraded ? `，质量分 ${Math.round(q.overall)}` : ''}`
          : '草稿已生成，但校验存在问题，请检查或修改',
      )
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 503) showToast('生成失败：LLM 未配置（503），请到「设置-模型管理」配置', 'err')
      else showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  const save = async () => {
    let spec
    try {
      spec = JSON.parse(specText)
    } catch (e: any) {
      showToast(`JSON 解析失败：${e.message}`, 'err')
      return
    }
    const nm = ontoName.trim() || spec?.name || ''
    if (!nm) {
      showToast('请输入本体名称', 'err')
      return
    }
    setSaving(true)
    try {
      const created = await api.createOntology({ name: nm, description: spec?.description ?? `由知识库「${result?.kb_name}」构建（O13 策略 ${result?.strategy}）` })
      // 兼容旧草稿旁路cqs；显式cq（包括空数组）属于用户编辑，必须保留。
      const savedSpec = Object.prototype.hasOwnProperty.call(spec, 'cq')
        ? { ...spec, name: nm }
        : { ...spec, name: nm, cq: result?.cqs ?? [] }
      await api.saveSpec(created.id, savedSpec)
      invalidateOntos() // REQ-467：单源清单失效广播（资产栏/各构建流即时可见）
      showToast('已入库到本体资产')
      setSavedId(created.id)
      setSavedName(nm)
      // REQ-459：入库即新会话——生成草稿与编辑态清空（kb-flow-result 草稿同清），跳步回看不再残留旧会话；
      // KB/策略选择保留（与 S1 来源 tab 记忆同口径——同源重跑便捷）
      setResult(null)
      setSpecText('')
      setOntoName('')
      setStep(3)
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) showToast(`构建平面校验未通过：${e.validationErrors[0].message}`, 'err')
      else showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  const report = result?.validation_report
  const specTags = result && (
    <Space size={6} wrap>
      <Tag color="blue" style={{ margin: 0 }}>概念 {result.spec_json?.concepts?.length ?? 0}</Tag>
      <Tag color="geekblue" style={{ margin: 0 }}>关系 {result.spec_json?.relations?.length ?? 0}</Tag>
      <Tag color="purple" style={{ margin: 0 }}>实例 {result.spec_json?.instances?.length ?? 0}</Tag>
      <Tag style={{ margin: 0 }}>rounds {result.rounds}</Tag>
      {result.method && <Tag color="cyan" style={{ margin: 0 }}>KG method {result.method}</Tag>}
      {result.cqs?.length ? <Tag color="orange" style={{ margin: 0 }}>CQ ×{result.cqs.length}</Tag> : null}
      {result.truncated && <Tag color="volcano" style={{ margin: 0 }}>语料已截断</Tag>}
      {result.quality && !result.quality.degraded && (
        <Tooltip title="qualitygate 统一口径快评（构建平面 save=false 零副作用，REQ-267）——详细 findings 见入库后资产质量卡">
          <Tag color={result.quality.error_count > 0 ? 'red' : result.quality.overall >= 80 ? 'green' : 'orange'} style={{ margin: 0 }}>
            质量 {Math.round(result.quality.overall)}
          </Tag>
        </Tooltip>
      )}
      {result.quality?.degraded && <Tag style={{ margin: 0 }}>质量快评不可达（本地结构校验兜底）</Tag>}
    </Space>
  )

  return (
    <Card className="work-card onto-stage-card" size="small">
      <Steps
        size="small"
        current={step}
        onChange={setStep}
        items={STEPS.map((s, i) => ({ key: s.key, title: s.title, status: i === step ? 'process' : 'wait' }))}
        style={{ marginBottom: 14 }}
      />

      {step === 0 && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">选择作为数据源的知识库</span>
            <Button size="small" icon={<ReloadOutlined />} loading={kbLoading} onClick={() => loadKBs(kbId)}>
              刷新
            </Button>
          </div>
          {kbs.length === 0 && !kbLoading && (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无知识库；请先到「知识库」栏创建并导入文档" />
          )}
          <div className="onto-engine-nav" style={{ marginTop: 8 }}>
            {kbs.map((k) => {
              const mt = MODE_TAG[k.mode ?? 'rag']
              return (
                <button
                  key={k.id}
                  type="button"
                  className={`onto-engine-item${kbId === k.id ? ' active' : ''}`}
                  onClick={() => setKbId(k.id)}
                >
                  <span className="onto-engine-top">
                    <span className="onto-engine-label">{k.name}</span>
                    <Tag color={mt.color} style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
                      {mt.text}
                    </Tag>
                  </span>
                  <span className="onto-engine-desc">
                    文档 {k.doc_count} · chunk {k.chunk_count}
                    {k.mode === 'graphrag' || k.kg_ready ? ` · KG ${k.kg_entities} 实体 / ${k.kg_relationships} 关系` : ' · 无 KG'}
                  </span>
                  <span className="onto-engine-desc">
                    {(k.chunk_count ?? 0) === 0
                      ? '暂无语料（先到知识库栏导入文档）'
                      : !k.kg_ready
                        ? '策略 A 可用；B/C 需先构建 KG'
                        : '策略 A / B / C 均可用'}
                  </span>
                </button>
              )
            })}
          </div>
          {selected && !selected.kg_ready && (selected.chunk_count ?? 0) > 0 && (
            <Alert
              type="info"
              showIcon
              style={{ marginTop: 12 }}
              title={`「${selected.name}」尚无 KG`}
              description="策略 B / C 需要先抽取 KG。点击下方按钮用自研抽取器重建（REQ-98 LLM 主路径 + 规则回退，D-O15；graphrag 模式导入文档时也会自动构建）。"
              action={
                <Space size={8}>
                  <Button size="small" icon={<DatabaseOutlined />} loading={buildingKG === selected.id} onClick={() => buildKG(selected.id)}>
                    构建 KG
                  </Button>
                  {/* REQ-600：B/C 前置条件入口拦截补指路——也可到知识库模块用 GraphRAG 抽取（能力更强） */}
                  <Button size="small" type="link" style={{ padding: 0 }} onClick={() => setPage('knowledge')}>
                    前往知识库抽取
                  </Button>
                </Space>
              }
            />
          )}
          <Space style={{ marginTop: 12 }}>
            <Button type="primary" disabled={!kbId || (selected?.chunk_count ?? 0) === 0} onClick={() => setStep(1)}>
              下一步：策略与 CQ
            </Button>
          </Space>
        </>
      )}

      {step === 1 && selected && (
        <>
          <div className="onto-sec" style={{ marginTop: 0 }}>
            <span className="onto-sec-title">抽取策略（04 §3.7；REQ-337 三策略卡片化）</span>
          </div>
          {/* REQ-337/M137：三策略页化——A/B/C 卡片选择（含可用性与口径说明，替代 Segmented） */}
          <div className="build-path-grid" data-testid="kb-strategy-cards" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
            {(['chunk-llm', 'kg-direct', 'hybrid'] as const).map((sk) => {
              const disabled = sk !== 'chunk-llm' && !selected.kg_ready
              return (
                <Card
                  key={sk}
                  size="small"
                  hoverable={!disabled}
                  onClick={() => !disabled && setStrategy(sk)}
                  style={{
                    borderColor: strategy === sk ? 'var(--ant-color-primary, #4f46e5)' : undefined,
                    opacity: disabled ? 0.55 : 1,
                    cursor: disabled ? 'not-allowed' : 'pointer',
                  }}
                  data-testid={`kb-strategy-${sk}`}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                    <span style={{ fontWeight: 600, fontSize: 13 }}>{STRATEGY_NAME[sk]}</span>
                    {disabled && <Tag style={{ margin: 0, fontSize: 10 }}>需先构建 KG</Tag>}
                  </div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{STRATEGY_HINT[sk]}</Typography.Text>
                </Card>
              )
            })}
          </div>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 8, marginBottom: 0 }}>
            {STRATEGY_HINT[strategy]}
            {!selected.kg_ready && strategy !== 'chunk-llm' && '（当前 KB 无 KG，先回上一步构建）'}
          </Typography.Paragraph>

          <div className="onto-sec">
            <span className="onto-sec-title">CQ 引导模式（REQ-90）</span>
          </div>
          <Radio.Group value={cqMode} onChange={(e) => setCqMode(e.target.value)}>
            <Radio value="auto">自动生成 3~5 个能力问题</Radio>
            <Radio value="custom">用户自定义</Radio>
            <Radio value="skip">跳过</Radio>
          </Radio.Group>
          {cqMode === 'custom' && (
            <Input.TextArea
              style={{ marginTop: 8 }}
              value={cqText}
              onChange={(e) => setCqText(e.target.value)}
              autoSize={{ minRows: 2, maxRows: 5 }}
              placeholder={'本体应能回答的关键问题，每行一个：\n某故障源于什么原因？\n某原因对应什么维护措施？'}
            />
          )}
          <Space style={{ marginTop: 14 }}>
            <Button onClick={() => setStep(0)}>上一步</Button>
            <Button type="primary" icon={<ThunderboltOutlined />} loading={busy} onClick={generate}>
              生成草稿
            </Button>
          </Space>
        </>
      )}

      {step === 1 && !selected && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请先选择知识库" />}

      {step === 2 && result && (
        <>
          {specTags}
          {result.warnings?.map((w, i) => (
            <Alert key={i} type="warning" showIcon style={{ marginTop: 8 }} title={w} />
          ))}
          {report?.ok ? (
            <Alert type="success" showIcon style={{ marginTop: 8 }} title="本地结构校验通过（入库时构建平面仍会做权威校验）" />
          ) : (
            <>
              <Alert type="error" showIcon style={{ marginTop: 8 }} title={`校验未通过（${report?.errors.length ?? 0} 项）——可直接修改下方 JSON 后入库，或重新生成`} />
              <Table<ValidationError>
                rowKey={(r) => `${r.path}::${r.message}`}
                columns={ERR_COLUMNS} scroll={{ x: 'max-content' }}
                dataSource={report?.errors ?? []}
                pagination={false}
                size="small"
                style={{ marginTop: 8 }}
              />
            </>
          )}
          {result.cqs?.length ? (
            <Alert
              type="info"
              showIcon
              style={{ marginTop: 8 }}
              title={`能力问题（CQ，REQ-90 ${result.cq_mode === 'auto' ? '自动生成' : '自定义'}）`}
              description={
                <ul className="onto-report-list" style={{ margin: 0 }}>
                  {result.cqs.map((c, i) => (
                    <li key={i}>{c}</li>
                  ))}
                </ul>
              }
            />
          ) : null}
          <Input style={{ marginTop: 10 }} value={ontoName} onChange={(e) => setOntoName(e.target.value)} placeholder="本体名称" />
          <Input.TextArea
            className="onto-spec-editor"
            style={{ marginTop: 8 }}
            value={specText}
            onChange={(e) => setSpecText(e.target.value)}
            autoSize={{ minRows: 14, maxRows: 34 }}
            spellCheck={false}
          />
          <Space style={{ marginTop: 10 }}>
            <Button onClick={() => { setResult(null); setStep(1) }}>上一步（重新配置）</Button>
            <Button icon={<ThunderboltOutlined />} loading={busy} onClick={generate}>
              重新生成
            </Button>
            <Button type="primary" icon={<CheckCircleOutlined />} loading={saving} onClick={save}>
              创建并保存到本体资产
            </Button>
          </Space>
        </>
      )}

      {step === 2 && !result && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未生成草稿" />}

      {step === 3 && savedId && (
        <Alert
          type="success"
          showIcon
          title={`已入库：${savedName || savedId}`}
          /* REQ-600：收敛卡统一件（DoneCTA 退役） */
          description={
            <ConvergeCard
              ontologyId={savedId}
              detail="由知识库构建完成"
              onRebuild={() => {
                setSavedId(null)
                setResult(null)
                setStep(0)
              }}
            />
          }
        />
      )}
    </Card>
  )
}

export default KbBuildFlow

// ---------------------------------------------------------------------------
// M-O14 P2⑤（REQ-171 P2/23 号 §7.1 借鉴）：结构化数据映射（CSV/JSON → 本体骨架）。
// 独立于 KB 选择——规则推导（首列→实例名/主概念，余列→属性+类型推断），可选目标本体
// 概念命中标注；预览映射报告与骨架 → 一键创建为新本体（走既有 create+saveSpec+校验）。
// 关系推导不做规则臆断（后端诚实注记指向 REQ-82 LLM 加工）。
// ---------------------------------------------------------------------------
