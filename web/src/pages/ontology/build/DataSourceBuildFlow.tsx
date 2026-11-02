import { useEffect, useState } from 'react'
import { Alert, Button, Card, Checkbox, Collapse, Empty, Input, Space, Tag, Typography } from 'antd'
import { ApiError, api } from '../../../api/client'
import type { DataSourceFKSuggestion, DataSourceProposal, Spec } from '../../../api/types'
import { useUI } from '../../../store/ui'
import DraftTrimPanel from '../components/DraftTrimPanel'
import { clearDraft, loadDraft, saveDraft } from '../../../lib/draftStore'
import { invalidateOntos } from '../shared/useOntoList'

// ---------------------------------------------------------------------------
// REQ-595/M406 由数据源构建（第八构建路径）——连接 SQLite 只读内省 → 列画像采样 →
// 规则骨架 → LLM 窄调用补显示名/描述/隐含 FK 建议 → 提案人审（本组件）→ saveSpec 落资产。
// 人审形态：① LLM FK 建议逐条勾选采纳（默认不勾——必人审不静默落库，诚实边界）；
// ② DraftTrimPanel 分组裁剪（REQ-503 裁决形态复用）；③ 命名可改。
// 画像区只读透出采样证据（top 值/唯一性/枚举候选/日期格式），供人工核对 LLM 判断。
// ---------------------------------------------------------------------------

const DS_JOB_KEY = 'custom-ds-job'
const DS_PROPOSAL_KEY = 'custom-ds-proposal'

/** 采纳 FK 建议追加为对象关系（人审后 evidence=values——非声明约束，溯源如实） */
function assembleSpec(base: Spec, adopted: DataSourceFKSuggestion[]): Spec {
  if (adopted.length === 0) return base
  const next: Spec = { ...base, relations: [...(base.relations ?? [])] }
  const used = new Set((base.relations ?? []).map((r) => r.name))
  for (const s of adopted) {
    let name = s.column
    if (used.has(name)) name = `${s.table} ${s.column}`
    let i = 2
    while (used.has(name)) name = `${s.table} ${s.column} ${i++}`
    used.add(name)
    next.relations = [
      ...next.relations,
      { name, label: s.column, from: s.table, to: s.target_table, evidence: 'values', definition: s.reason ? `人工采纳的隐含引用关系（AI 建议：${s.reason}）` : '人工采纳的隐含引用关系（AI 建议）' },
    ]
  }
  return next
}

function profileLine(p: { is_unique?: boolean; top_values?: { value: string; count: number }[]; enum_candidates?: string[]; date_formats?: string[]; null_count?: number; sampled_rows?: number }): string {
  const bits: string[] = []
  if (p.is_unique) bits.push('采样内唯一')
  if (p.enum_candidates && p.enum_candidates.length > 0 && p.enum_candidates.length <= 8) bits.push(`枚举[${p.enum_candidates.join('|')}]`)
  else if (p.top_values && p.top_values.length > 0) bits.push(`高频[${p.top_values.slice(0, 3).map((v) => v.value).join('|')}]`)
  if (p.date_formats && p.date_formats.length > 0) bits.push(`日期 ${p.date_formats[0]}`)
  return bits.join(' · ')
}

export default function DataSourceBuildFlow({ onGoCustom }: { onGoCustom?: (ontoId: string) => void }) {
  const { showToast } = useUI()
  const [dsPath, setDsPath] = useState('')
  const [dsName, setDsName] = useState('')
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState('')
  const [timedOut, setTimedOut] = useState(false)
  const [jobId, setJobId] = useState<string | null>(() => loadDraft<string>(DS_JOB_KEY))
  const [proposal, setProposal] = useState<DataSourceProposal | null>(() => loadDraft<DataSourceProposal>(DS_PROPOSAL_KEY))
  const [trimmed, setTrimmed] = useState<Spec | null>(null)
  const [fkPicks, setFkPicks] = useState<boolean[]>([])
  const [nameDraft, setNameDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [createdId, setCreatedId] = useState<string | null>(null)

  // 提案到达/恢复时初始化工作副本与勾选态
  useEffect(() => {
    if (proposal) {
      setTrimmed((t) => t ?? proposal.spec)
      setNameDraft((n) => n || proposal.name)
      setFkPicks((p) => (p.length === (proposal.fk_suggestions ?? []).length ? p : (proposal.fk_suggestions ?? []).map(() => false)))
    }
  }, [proposal])

  const doAnalyze = async () => {
    if (!dsPath.trim()) {
      showToast('请输入 SQLite 数据库文件路径', 'err')
      return
    }
    setBusy(true)
    setProgress('')
    setTimedOut(false)
    setProposal(null)
    setTrimmed(null)
    setCreatedId(null)
    clearDraft(DS_PROPOSAL_KEY)
    try {
      const { job_id } = await api.datasourceAnalyze(dsPath.trim(), dsName.trim() || undefined)
      setJobId(job_id)
      saveDraft(DS_JOB_KEY, job_id)
      let done: DataSourceProposal | undefined
      for (let i = 0; i < 160; i++) {
        await new Promise((res) => setTimeout(res, 1500))
        try {
          const j = await api.aiDraftJob(job_id)
          if (j.progress) setProgress(j.progress)
          if (j.status === 'done') {
            done = j.datasource
            break
          }
          if (j.status === 'failed') throw new Error(j.error || '数据分析失败')
        } catch (e: any) {
          if (e?.message?.includes('失败') || e?.message?.includes('不存在')) throw e
        }
      }
      if (!done) {
        setTimedOut(true)
        throw new Error('分析超时——可点「取回上次任务」继续等待（大库采样耗时与表数成正比）')
      }
      setProposal(done)
      saveDraft(DS_PROPOSAL_KEY, done)
      showToast('分析完成——请人工确认提案后落库')
    } catch (e: any) {
      showToast(e instanceof ApiError ? e.message : e.message, 'err')
    } finally {
      setBusy(false)
      setProgress('')
    }
  }

  // 超时重入：凭 draftStore 的 job_id 重新轮询（沿 REQ-503③ ai 取回先例）
  const reclaimJob = async () => {
    if (!jobId) return
    setBusy(true)
    setTimedOut(false)
    try {
      let done: DataSourceProposal | undefined
      for (let i = 0; i < 160; i++) {
        await new Promise((res) => setTimeout(res, 1500))
        try {
          const j = await api.aiDraftJob(jobId)
          if (j.progress) setProgress(j.progress)
          if (j.status === 'done') {
            done = j.datasource
            break
          }
          if (j.status === 'failed') throw new Error(j.error || '数据分析失败')
        } catch (e: any) {
          if (e?.message?.includes('失败') || e?.message?.includes('不存在')) throw e
        }
      }
      if (!done) throw new Error('仍未完成——大库建议调小 DATASOURCE_SAMPLE_ROWS 后重试')
      setProposal(done)
      saveDraft(DS_PROPOSAL_KEY, done)
      showToast('已取回分析结果')
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  const doSave = async () => {
    if (!proposal) return
    const effective = trimmed ?? proposal.spec
    const nm = nameDraft.trim() || proposal.name
    if (!nm) {
      showToast('请输入本体名称', 'err')
      return
    }
    setSaving(true)
    try {
      const sugg = proposal.fk_suggestions ?? []
      const adopted = sugg.filter((_, i) => fkPicks[i])
      const finalSpec = assembleSpec(effective, adopted)
      const created = await api.createOntology({ name: nm, description: finalSpec.description ?? '' })
      await api.saveSpec(created.id, { ...finalSpec, name: nm })
      invalidateOntos()
      setCreatedId(created.id)
      clearDraft(DS_PROPOSAL_KEY)
      clearDraft(DS_JOB_KEY)
      showToast('数据源提案已创建并保存')
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) showToast(`提案校验未通过：${e.validationErrors[0].message}`, 'err')
      else showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  const goToAsset = (id: string) => {
    localStorage.setItem('eino.onto.sidebar', 'assets')
    localStorage.setItem('eino.onto.asset.focus', id)
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  }

  if (createdId) {
    return (
      <Card size="small" data-testid="ds-success">
        <Alert
          type="success"
          showIcon
          title="数据源提案已创建并保存"
          description={`本体「${nameDraft.trim() || proposal?.name || createdId}」已入库（保存即过结构校验与质量门禁）。可进入 S2 继续编辑，或前往资产详情查看质量评分。`}
          style={{ marginBottom: 12 }}
        />
        <Space>
          {onGoCustom && (
            <Button type="primary" size="small" data-testid="ds-go-custom" onClick={() => onGoCustom(createdId)}>
              进入 S2 编辑
            </Button>
          )}
          <Button size="small" data-testid="ds-go-asset" onClick={() => goToAsset(createdId)}>
            前往资产详情
          </Button>
          <Button
            size="small"
            onClick={() => {
              setCreatedId(null)
              setProposal(null)
              setTrimmed(null)
              setDsPath('')
              setDsName('')
              setNameDraft('')
            }}
          >
            再建一个
          </Button>
        </Space>
      </Card>
    )
  }

  return (
    <Card size="small" data-testid="datasource-build-flow">
      <Typography.Title level={5} style={{ marginTop: 0 }}>
        由数据源构建（第八构建路径）
      </Typography.Title>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12.5, marginBottom: 12 }}>
        连接本机 SQLite 数据库（只读），自动推导本体骨架：数据表→概念、非主键非外键列→数据属性、声明外键→对象关系、纯外键关联表→关系；AI 补充中文显示名/业务描述/隐含外键建议。全程只读不写库，提案需人工确认后才落资产。
      </Typography.Paragraph>
      <Space.Compact style={{ width: '100%', marginBottom: 8 }}>
        <Input
          data-testid="ds-path-input"
          placeholder="SQLite 数据库文件路径（服务端本地，如 D:/data/shop.db 或 /data/shop.db）"
          value={dsPath}
          onChange={(e) => setDsPath(e.target.value)}
          disabled={busy}
        />
        <Button type="primary" data-testid="ds-analyze-btn" loading={busy} onClick={doAnalyze}>
          连接并分析
        </Button>
      </Space.Compact>
      <Input
        data-testid="ds-name-input"
        placeholder="本体名称（可空，默认取文件名）"
        value={dsName}
        onChange={(e) => setDsName(e.target.value)}
        style={{ marginBottom: 8 }}
        disabled={busy}
      />
      {(progress || busy) && (
        <Typography.Paragraph data-testid="ds-progress" type="secondary" style={{ fontSize: 12.5 }}>
          {progress || '提交中…'}
        </Typography.Paragraph>
      )}
      {timedOut && (
        <Alert
          type="warning"
          showIcon
          title="分析超时"
          description="任务仍在后台执行，可稍后取回结果。"
          style={{ marginBottom: 8 }}
          action={
            <Button size="small" data-testid="ds-reclaim-job" onClick={reclaimJob}>
              取回上次任务
            </Button>
          }
        />
      )}
      {!proposal && !busy && (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入路径开始分析——MySQL/Postgres 等网络数据源留二期（触发驱动）" style={{ margin: '16px 0' }} />
      )}
      {proposal && (
        <div data-testid="ds-proposal">
          <Space wrap size={4} style={{ marginBottom: 8 }}>
            <Tag>概念 {proposal.spec.concepts?.length ?? 0}</Tag>
            <Tag>数据属性 {proposal.spec.data_properties?.length ?? 0}</Tag>
            <Tag>对象关系 {proposal.spec.relations?.length ?? 0}</Tag>
            <Tag color={proposal.llm_used ? 'blue' : 'default'}>{proposal.llm_used ? 'AI 已补名称/描述' : 'AI 未参与'}</Tag>
          </Space>
          {!proposal.llm_used && proposal.llm_skipped && (
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
              {proposal.llm_skipped}
            </Typography.Paragraph>
          )}
          {proposal.llm_fallback && (
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
              AI 窄调用部分批次失败，已回退确定性结果（骨架不受影响，如实注记见下方告警）。
            </Typography.Paragraph>
          )}
          {(proposal.warnings?.length ?? 0) > 0 && (
            <Alert
              type="warning"
              showIcon
              title={`${proposal.warnings!.length} 条如实注记`}
              description={
                <ul style={{ margin: 0, paddingLeft: 18, fontSize: 12 }}>
                  {proposal.warnings!.slice(0, 8).map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              }
              style={{ marginBottom: 8 }}
            />
          )}
          {(proposal.spec.concepts?.length ?? 0) > 0 && (
            <Card size="small" style={{ marginBottom: 8 }} title="骨架预览（概念与 AI 补充的显示名/描述——人审重点）" data-testid="ds-concept-preview">
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {proposal.spec.concepts.map((c) => (
                  <div key={c.name} style={{ fontSize: 12.5 }}>
                    {c.name}
                    {c.label && c.label !== c.name ? <b>（{c.label}）</b> : null}
                    {c.definition ? <Typography.Text type="secondary" style={{ fontSize: 12 }}>　{c.definition}</Typography.Text> : null}
                  </div>
                ))}
              </div>
            </Card>
          )}
          {(proposal.fk_suggestions?.length ?? 0) > 0 && (
            <Card size="small" style={{ marginBottom: 8 }} title="隐含外键建议（AI 结构性建议——默认不采纳，逐条勾选后才并入）" data-testid="ds-fk-suggestions">
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {proposal.fk_suggestions!.map((s, i) => (
                  <Checkbox
                    key={`${s.table}.${s.column}`}
                    data-testid={`ds-fk-check-${i}`}
                    checked={fkPicks[i] ?? false}
                    onChange={(e) => {
                      const next = [...fkPicks]
                      next[i] = e.target.checked
                      setFkPicks(next)
                    }}
                  >
                    <span style={{ fontSize: 12.5 }}>
                      {s.table}.{s.column} → <b>{s.target_table}</b>
                      {s.reason ? <Typography.Text type="secondary" style={{ fontSize: 12 }}>　{s.reason}</Typography.Text> : null}
                    </span>
                  </Checkbox>
                ))}
              </div>
            </Card>
          )}
          <Card size="small" style={{ marginBottom: 8 }} title="骨架裁剪（分组勾选剔除——落库前人工把关）">
            <DraftTrimPanel spec={trimmed ?? proposal.spec} onChange={(next) => setTrimmed(next)} />
          </Card>
          <Input data-testid="ds-final-name" addonBefore="本体名称" value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} style={{ marginBottom: 8 }} />
          <Collapse
            size="small"
            ghost
            items={[
              {
                key: 'profile',
                label: <span style={{ fontSize: 12.5 }}>列画像采样证据（只读透出，供核对 AI 判断）</span>,
                children: (
                  <div data-testid="ds-profile" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {proposal.tables.map((tp) => (
                      <div key={tp.table}>
                        <Typography.Text strong style={{ fontSize: 12.5 }}>
                          {tp.table}
                        </Typography.Text>
                        {tp.error ? (
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            　{tp.error}
                          </Typography.Text>
                        ) : (
                          <Typography.Paragraph type="secondary" style={{ fontSize: 12, margin: 0 }}>
                            （采样 {tp.sampled_rows} 行）
                          </Typography.Paragraph>
                        )}
                        {(tp.columns ?? []).map((cp) => (
                          <div key={cp.column} style={{ fontSize: 12, color: 'var(--c-text-secondary, #888)', paddingLeft: 12 }}>
                            {cp.column} <Tag style={{ fontSize: 11, lineHeight: '16px' }}>{cp.decl_type || '动态'}</Tag>
                            {profileLine(cp)}
                          </div>
                        ))}
                      </div>
                    ))}
                  </div>
                ),
              },
            ]}
          />
          <Space style={{ marginTop: 8 }}>
            <Button type="primary" data-testid="ds-save-btn" loading={saving} onClick={doSave}>
              创建资产并保存
            </Button>
            <Button
              data-testid="ds-discard-btn"
              onClick={() => {
                setProposal(null)
                setTrimmed(null)
                clearDraft(DS_PROPOSAL_KEY)
              }}
            >
              放弃提案
            </Button>
          </Space>
        </div>
      )}
    </Card>
  )
}
