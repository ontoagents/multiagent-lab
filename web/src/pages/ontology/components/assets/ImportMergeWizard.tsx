import { useState } from 'react'
import { Alert, Button, Input, Modal, Segmented, Space, Steps, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import ReactDiffViewer from 'react-diff-viewer-continued'
import { api } from '../../../../api/client'
import type { MergePreview } from '../../../../api/client'

// ---------------------------------------------------------------------------
// REQ-157/M-O15 导入审查向导（OrionBelt 模式）：外部本体文件接入既有本体的受控流程——
//   ① 复用语义二选一（REQ-511/M319）：「并入」（吸收进本本体，走下方三策略）vs
//      「引用」（owl:imports/external_refs 登记，不改原文件——吃 REQ-479~482 装载闭包/
//      查询归因下游收益，登记态复用 ExternalRefsPane）；并入路径再选合并策略
//      （replace=导入版整体替换冲突实体 / merge-overwrite=字段级覆盖 /
//      merge=保留现行+导入实体前缀重命名并入【增量消歧】）→ 冲突预览；
//   ② 字段级冲突报告 + 新增清单 + 现行 vs 合并结果全文 diff（react-diff-viewer-continued 复用 M22 A3）；
//      引用路径跳过冲突审查直达登记结果；
//   ③ 应用（结构校验 + strict 门禁，M-O15）→ 版本快照 vN+1；引用路径零版本快照（spec 零变更）。
// 复杂语义合并明确不做（26 号方案 §8-3）；引用语义与「并入」互斥二选一（REQ-511）。
// ---------------------------------------------------------------------------

const STRATEGY_META: Record<string, { label: string; desc: string }> = {
  replace: { label: '替换', desc: '冲突实体以导入版整体替换，无冲突实体新增' },
  'merge-overwrite': { label: '字段合并', desc: '冲突实体按字段覆盖：导入版非空字段覆盖现行，保留现行独有字段' },
  merge: { label: '重命名并入', desc: '冲突实体保留现行，导入版加前缀重命名并入（增量消歧，引用随改写）' },
}

export default function ImportMergeWizard({
  open,
  onClose,
  ontologyId,
  targetSpecText,
  onApplied,
  onNeedEdit,
}: {
  open: boolean
  onClose: () => void
  ontologyId: string
  /** 现行 spec 全文（diff 左侧） */
  targetSpecText: string
  onApplied: (version: number) => void
  /** REQ-235/H5：有损导入后「前往图形编辑补录」回调（跳详情图形编辑分区） */
  onNeedEdit?: () => void
}) {
  const [step, setStep] = useState(0)
  const [filename, setFilename] = useState('')
  const [content, setContent] = useState('')
  // REQ-511/M319：复用语义（引用=登记 external_refs 不改原文件；并入=既有合并流程，默认维持存量行为）
  const [reuseSem, setReuseSem] = useState<'merge' | 'reference'>('merge')
  const [refResult, setRefResult] = useState<{ registered: number; skipped: number; iris: string[]; skipped_iris: string[]; note?: string } | null>(null)
  const [strategy, setStrategy] = useState<string>('merge-overwrite')
  const [prefix, setPrefix] = useState('ext')
  const [preview, setPreview] = useState<MergePreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [appliedVersion, setAppliedVersion] = useState<number | null>(null)

  const reset = () => {
    setStep(0)
    setFilename('')
    setContent('')
    setReuseSem('merge')
    setRefResult(null)
    setPreview(null)
    setErr(null)
    setAppliedVersion(null)
  }

  const close = () => {
    onClose()
    setTimeout(reset, 300)
  }

  const onFile = async (file: File) => {
    setFilename(file.name)
    setContent(await file.text())
  }

  const doPreview = () => {
    if (!content.trim()) {
      setErr('请先选择外部本体文件')
      return
    }
    setBusy(true)
    setErr(null)
    api
      .mergePreview(ontologyId, { filename, content, strategy })
      .then((pv) => {
        setPreview(pv)
        setStep(1)
      })
      .catch((e: any) => setErr(e?.message ?? '冲突预览失败'))
      .finally(() => setBusy(false))
  }

  // REQ-511/M319：引用语义登记——owl:imports 目标落 external_refs（幂等；零版本快照）
  const doRegisterRef = () => {
    if (!content.trim()) {
      setErr('请先选择外部本体文件')
      return
    }
    setBusy(true)
    setErr(null)
    api
      .registerExternalRefsFromImport(ontologyId, { filename, content })
      .then((r) => {
        setRefResult(r)
        setStep(2)
      })
      .catch((e: any) => setErr(e?.message ?? '引用登记失败'))
      .finally(() => setBusy(false))
  }

  const doApply = () => {
    setBusy(true)
    setErr(null)
    api
      .mergeApply(ontologyId, { filename, content, strategy, prefix, base_version: preview?.base_version })
      .then((r) => {
        setAppliedVersion(r.version)
        setStep(2)
        onApplied(r.version)
      })
      .catch((e: any) => setErr(e?.message ?? '合并应用失败'))
      .finally(() => setBusy(false))
  }

  const conflictColumns: ColumnsType<NonNullable<MergePreview['conflicts'][number]>> = [
    {
      title: '类型',
      dataIndex: 'kind',
      width: 76,
      render: (v: string) => <Tag style={{ margin: 0 }} color={{ concept: 'geekblue', relation: 'purple', instance: 'cyan' }[v] ?? 'default'}>{v}</Tag>,
    },
    { title: '实体', dataIndex: 'name', width: 140, ellipsis: true },
    {
      title: '冲突字段',
      dataIndex: 'fields',
      width: 150,
      render: (v: string[]) => (v ?? []).map((f) => <Tag key={f} style={{ margin: 2 }}>{f}</Tag>),
    },
    { title: '处置', dataIndex: 'resolution', width: 110, render: (v: string, r) => <Tag style={{ margin: 0 }} color="blue">{v}{r.resolved_as ? ` → ${r.resolved_as}` : ''}</Tag> },
    {
      title: '对照（导入 ← 现行）',
      ellipsis: true,
      render: (_, r) => (
        <Typography.Text type="secondary" style={{ fontSize: 11 }} copyable={false}>
          {r.incoming.slice(0, 90)}
          <br />
          {r.current.slice(0, 90)}
        </Typography.Text>
      ),
    },
  ]

  return (
    <Modal
      open={open}
      centered
      width={880}
      title="导入合并 · 审查向导（REQ-157）"
      onCancel={close}
      footer={null}
      destroyOnHidden
    >
      <Steps
        size="small"
        current={step}
        style={{ marginBottom: 14 }}
        items={reuseSem === 'reference' ? [{ title: '文件与引用' }, { title: '登记结果' }] : [{ title: '文件与策略' }, { title: '冲突审查' }, { title: '应用' }]}
      />
      {err && <Alert type="error" showIcon style={{ marginBottom: 10 }} title="操作失败" description={err} closable onClose={() => setErr(null)} />}

      {step === 0 && (
        <>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 10 }}
            title="将外部本体文件并入当前本体（增量消歧而非一次性重建）"
            description="支持构建平面可导入的格式（TTL/OWL/JSON-LD 走 sidecar、SKOS 词表自动识别、CSV、GraphML、spec_json）。先做冲突预览，确认策略后再应用；应用会生成新版本快照，可随时回退。"
          />
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 10 }}
            title="有损导入（REQ-235⑤）"
            description="本导入走 spec_json 轻量教学子集（类层次/对象属性/实例断言，datatype 断言落实例 attributes，公理与推理语义有损丢弃并在报告中明示）；丢失项可经「前往图形编辑补录」补齐。"
          />
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Space size={10} wrap>
              <input
                type="file"
                accept=".ttl,.owl,.rdf,.xml,.csv,.json,.jsonld,.md,.txt"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) void onFile(f)
                }}
              />
              {filename && <Tag color="blue" style={{ margin: 0 }}>{filename}</Tag>}
            </Space>
            {/* REQ-511/M319：复用语义二选一（先于合并策略——引用不触合并面） */}
            <div>
              <Typography.Text strong style={{ fontSize: 12 }}>复用语义</Typography.Text>
              <div style={{ marginTop: 6 }}>
                <Segmented
                  value={reuseSem}
                  onChange={(v) => setReuseSem(v as 'merge' | 'reference')}
                  options={[
                    { value: 'merge', label: '并入' },
                    { value: 'reference', label: '引用' },
                  ]}
                  data-testid="import-reuse-sem"
                />
                <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 6, marginBottom: 0 }}>
                  {reuseSem === 'merge'
                    ? '吸收进本本体：实体入 spec 并生成版本快照（可回退）'
                    : '不改原文件：提取文件中的 owl:imports 目标登记为外部引用（幂等，登记态见资产页「外部引用」分区；后续装载/查询吃引用闭包与归因收益）'}
                </Typography.Paragraph>
              </div>
            </div>
            {reuseSem === 'merge' && (
              <>
                <div>
                  <Typography.Text strong style={{ fontSize: 12 }}>合并策略</Typography.Text>
                  <div style={{ marginTop: 6 }}>
                    <Segmented
                      value={strategy}
                      onChange={(v) => setStrategy(v as string)}
                      options={Object.entries(STRATEGY_META).map(([k, m]) => ({ value: k, label: m.label }))}
                    />
                    <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 6, marginBottom: 0 }}>
                      {STRATEGY_META[strategy]?.desc}
                    </Typography.Paragraph>
                  </div>
                </div>
                {strategy === 'merge' && (
                  <Space size={6}>
                    <Typography.Text style={{ fontSize: 12 }}>冲突实体重命名前缀：</Typography.Text>
                    <Input size="small" style={{ width: 160 }} value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="ext" />
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>与现行实体撞名时自动追加序号避让</Typography.Text>
                  </Space>
                )}
              </>
            )}
            <Space>
              {reuseSem === 'merge' ? (
                <Button type="primary" loading={busy} onClick={doPreview}>
                  冲突预览
                </Button>
              ) : (
                <Button type="primary" loading={busy} onClick={doRegisterRef} data-testid="import-register-ref">
                  登记引用
                </Button>
              )}
              <Button onClick={close}>取消</Button>
            </Space>
          </Space>
        </>
      )}

      {step === 1 && preview && (
        <>
          <Space size={6} wrap style={{ marginBottom: 10 }}>
            <Tag color="blue" style={{ margin: 0 }}>新增 {preview.added.length}</Tag>
            <Tag color="orange" style={{ margin: 0 }}>冲突 {preview.conflicts.length}</Tag>
            <Tag color="purple" style={{ margin: 0 }}>重命名 {preview.renamed.length}</Tag>
            {(preview.semantic_conflicts?.length ?? 0) > 0 && (
              <Tag color="red" style={{ margin: 0 }}>语义分歧 {preview.semantic_conflicts!.length}</Tag>
            )}
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              概念 +{preview.stats.concepts_added}/改{preview.stats.concepts_updated} · 关系 +{preview.stats.relations_added}/改{preview.stats.relations_updated} · 实例 +{preview.stats.instances_added}/改{preview.stats.instances_updated}/更名{preview.stats.instances_renamed}
            </Typography.Text>
          </Space>
          <Table
            rowKey={(r) => r.kind + r.name}
            columns={conflictColumns}
            dataSource={preview.conflicts}
            size="small"
            pagination={preview.conflicts.length > 8 ? { pageSize: 10, hideOnSinglePage: true } : false}
            locale={{ emptyText: '无冲突——全部为新增实体' }}
            style={{ marginBottom: 12 }}
          />
          {preview.renamed.length > 0 && (
            <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
              重命名清单：{preview.renamed.join('；')}
            </Typography.Paragraph>
          )}
          {/* REQ-259/M258：语义级冲突预检呈现——conflict=红（父类分歧/定义漂移，人工裁决）hint=金（label 同义，可登记 external_ref alignment 对齐） */}
          {(preview.semantic_conflicts?.length ?? 0) > 0 && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message={`语义分歧预检 ${preview.semantic_conflicts!.length} 条（合并策略行为不受影响——人工裁决后应用）`}
              description={
                <ul style={{ margin: 0, paddingLeft: 18 }}>
                  {preview.semantic_conflicts!.map((c, i) => (
                    <li key={i} style={{ fontSize: 12 }}>
                      <Tag color={c.severity === 'conflict' ? 'red' : 'gold'} style={{ marginInlineEnd: 6 }}>
                        {c.kind === 'parent_divergence' ? '父类分歧' : c.kind === 'definition_drift' ? '定义漂移' : 'label 同义'}
                      </Tag>
                      <Typography.Text code style={{ fontSize: 12 }}>{c.entity}</Typography.Text> — {c.detail}
                    </li>
                  ))}
                </ul>
              }
            />
          )}
          {/* REQ-235/H5/M62：有损导入清单——条目可定位，应用后可跳图形编辑补录再重跑对账 */}
          {preview.import_report?.reconciliation && (
            <Alert
              type={preview.import_report.reconciliation.unaccounted === 0 ? 'success' : 'error'}
              showIcon
              style={{ marginBottom: 10 }}
              title={`导入对账（REQ-328）：入 ${preview.import_report.reconciliation.triples_in} 条 = 词汇 ${preview.import_report.reconciliation.vocabulary} + 投影 ${preview.import_report.reconciliation.projected} + 余量 ${preview.import_report.reconciliation.remainder}${preview.import_report.reconciliation.unaccounted === 0 ? '（unaccounted=0，零静默丢失）' : `（unaccounted=${preview.import_report.reconciliation.unaccounted}，对账异常请反馈）`}`}
              description={
                <>
                  {preview.import_report.reconciliation.capture_rate !== undefined && (
                    <div>语义捕获率 {Math.round(preview.import_report.reconciliation.capture_rate * 100)}%；余量账本将随导出与运行装载原样还原（未编辑本体运行 100% 导入源）。</div>
                  )}
                  {(preview.import_report.reconciliation.inferred_notes ?? []).map((n, i) => (
                    <div key={i} style={{ color: 'var(--c-ink-3)', marginTop: 2 }}>· {n}</div>
                  ))}
                </>
              }
              data-testid="import-reconciliation"
            />
          )}
          {preview.import_report && (preview.import_report.lossy || (preview.import_report.warnings?.length ?? 0) > 0) && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 10 }}
              title={`有损导入（${preview.import_report.format}）：${preview.import_report.warnings?.length ?? 0} 项丢弃/降级`}
              description={
                <div style={{ maxHeight: 120, overflowY: 'auto', fontSize: 12 }}>
                  {(preview.import_report.warnings ?? []).map((w, i) => (
                    <div key={i}>· {w}</div>
                  ))}
                  {preview.import_report.lossy_note && (
                    <div style={{ color: 'var(--c-ink-3)', marginTop: 4 }}>{preview.import_report.lossy_note}</div>
                  )}
                </div>
              }
            />
          )}
          <Typography.Paragraph strong style={{ fontSize: 12, marginBottom: 4 }}>
            现行 vs 合并结果（全文对照）
          </Typography.Paragraph>
          <div style={{ border: '1px solid var(--c-line, #e3e6f0)', borderRadius: 8, overflow: 'hidden', maxHeight: '50vh' }}>
            <ReactDiffViewer
              oldValue={targetSpecText}
              newValue={JSON.stringify(preview.merged_spec, null, 2)}
              splitView
              leftTitle="现行 spec"
              rightTitle="合并结果"
              styles={{ diffContainer: { fontSize: 11 } }}
            />
          </div>
          <Space style={{ marginTop: 12 }}>
            <Button onClick={() => setStep(0)}>上一步</Button>
            <Button type="primary" loading={busy} onClick={doApply}>
              应用合并（生成版本快照）
            </Button>
            <Button onClick={close}>取消</Button>
          </Space>
        </>
      )}

      {step === 2 && reuseSem === 'reference' && refResult && (
        <div style={{ padding: '16px 0' }}>
          <Alert
            type={refResult.registered > 0 ? 'success' : 'info'}
            showIcon
            title={refResult.registered > 0 ? `引用登记完成：新增 ${refResult.registered} 条外部引用` : '未探测到 owl:imports 目标'}
            description={
              <>
                {refResult.iris.length > 0 && (
                  <p style={{ margin: '4px 0' }}>
                    {refResult.iris.map((iri) => (
                      <Tag key={iri} style={{ margin: 2 }} color="blue">{iri}</Tag>
                    ))}
                  </p>
                )}
                {refResult.skipped_iris.length > 0 && (
                  <p style={{ fontSize: 12, color: 'var(--ant-color-text-tertiary, #888)' }}>
                    已登记跳过 {refResult.skipped_iris.length} 条（幂等不重复登记）
                  </p>
                )}
                {refResult.note && <p style={{ fontSize: 12, color: '#d46b08' }}>{refResult.note}</p>}
                <p style={{ fontSize: 12, color: 'var(--ant-color-text-tertiary, #888)' }}>
                  本体 spec 未变更（引用语义不改原文件）；登记态与悬空检测见资产页「外部引用」分区，运行装载按引用闭包取数。
                </p>
              </>
            }
            data-testid="import-ref-result"
          />
          <Space style={{ marginTop: 12 }}>
            <Button onClick={() => { setStep(0); setRefResult(null) }}>上一步</Button>
            <Button type="primary" onClick={close}>完成</Button>
          </Space>
        </div>
      )}

      {step === 2 && reuseSem === 'merge' && (
        <div style={{ textAlign: 'center', padding: '16px 0' }}>
          <Alert
            type="success"
            showIcon
            title={`合并已应用：新版本 v${appliedVersion}`}
            description={
              <>
                <p>
                  新增 {preview?.added.length ?? 0} · 冲突处置 {preview?.conflicts.length ?? 0} · 重命名 {preview?.renamed.length ?? 0}
                </p>
                <p style={{ fontSize: 12, color: 'var(--ant-color-text-tertiary, #888)' }}>
                  已生成版本快照（可在「版本与源码」页签回看与回退）；strict 门禁开启时错误级命中会被拦截。
                </p>
                {preview?.import_report?.lossy && (
                  <p style={{ fontSize: 12, color: '#d46b08' }}>
                    本次导入存在 {preview.import_report.warnings?.length ?? 0} 项有损丢弃/降级（公理、datatype 声明等）——可前往图形编辑补录，再重跑本向导对账。
                  </p>
                )}
              </>
            }
          />
          <Space style={{ marginTop: 12 }}>
            {preview?.import_report?.lossy && (
              <Button
                onClick={() => {
                  close()
                  onNeedEdit?.()
                }}
              >
                前往图形编辑补录
              </Button>
            )}
            <Button type="primary" onClick={close}>
              完成
            </Button>
          </Space>
        </div>
      )}
    </Modal>
  )
}
