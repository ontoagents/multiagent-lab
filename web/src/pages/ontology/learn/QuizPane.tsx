import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Empty, Input, Progress, Radio, Select, Space, Tag, Typography } from 'antd'
import { HistoryOutlined, ReloadOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import type { Ontology } from '../../../api/types'
import MasteryGraph from './MasteryGraph'
import { parseOntoHash, writeOntoHash } from '../shared/ontoRoute'

/**
 * 出题自测（REQ-524/M331——Q-23 衍生③ 转正交付；REQ-572/M380 闭环化）：
 * 选本体 → LLM 概念子图出题（定向=掌握度薄弱/从未考察概念优先+历史禁重复+题型混合：
 * choice 单选/judge 判断=客观确定性判定，short_answer 简答=主观 LLM 批改）→ 作答 → 批改
 * （客观=选项原文精确比对不走 LLM；主观=对照参考答案 rubric 评分+点评，作答后才展示参考答案）
 * → 批改结果落档（批改记录审计面）→ 概念级掌握度落表（0.7 旧+0.3 新加权）→ 掌握度子图
 * （MasteryGraph：概念节点按掌握度着色，下一轮出题定向即其消费面）。
 */

interface QuizItem {
  id: string
  ontology_id: string
  concept_names: string[]
  question: string
  answer: string
  difficulty: string
  /** REQ-545/M352 出题预检裁决（''=未预检 | answerable | not_answerable——check_answerable 确定性预检） */
  coverage_verdict?: string
  /** REQ-572/M380 题型（缺省=short_answer 兼容存量行） */
  question_type?: string
  /** REQ-572/M380 客观题选项 */
  options?: string[]
}

interface MasteryLevel {
  concept_name: string
  asked_count: number
  correct_count: number
  score: number
}

interface GradeResult {
  score: number
  verdict: 'correct' | 'partial' | 'wrong'
  feedback: string
  concepts: string[]
}

interface QuizAttempt {
  id: string
  quiz_item_id: string
  question: string
  question_type: string
  answer: string
  score: number
  verdict: string
  feedback: string
  concept_names: string[]
  created_at: string
}

const VERDICT_COLOR: Record<string, string> = { correct: 'green', partial: 'gold', wrong: 'red' }
const VERDICT_TEXT: Record<string, string> = { correct: '答对', partial: '部分正确', wrong: '未答对' }
const TYPE_TEXT: Record<string, string> = { short_answer: '简答', choice: '单选', judge: '判断' }

export default function QuizPane({ ontologies }: { ontologies: Ontology[] }) {
  const [ontoId, setOntoId] = useState<string | null>(() => {
    const r = parseOntoHash()
    return r?.col === 'learn' && r.learn === 'quiz' ? r.ontologyId ?? null : null
  })
  const selectedRef = useRef(ontoId)
  const epoch = useRef(0)
  const masteryEpoch = useRef(0)
  const attemptsEpoch = useRef(0)
  const [difficulty, setDifficulty] = useState<'basic' | 'applied'>('basic')
  const [items, setItems] = useState<QuizItem[] | null>(null)
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [grades, setGrades] = useState<Record<string, GradeResult>>({})
  const [revealed, setRevealed] = useState<Record<string, boolean>>({})
  const [generating, setGenerating] = useState(false)
  const [gradingId, setGradingId] = useState<string | null>(null)
  const [mastery, setMastery] = useState<MasteryLevel[] | null>(null)
  const [genErr, setGenErr] = useState<string | null>(null)
  const [scopeNote, setScopeNote] = useState<string | null>(null) // REQ-608-f4bb 采样概念面如实提示
  const [graphKey, setGraphKey] = useState(0) // 批改后子图刷新
  const [attempts, setAttempts] = useState<QuizAttempt[] | null>(null)
  const [showAttempts, setShowAttempts] = useState(false)

  const selectOntology = useCallback((id: string | null, write = true) => {
    if (selectedRef.current === id) return
    selectedRef.current = id
    epoch.current++
    masteryEpoch.current++
    attemptsEpoch.current++
    setAttempts(null)
    setShowAttempts(false)
    setOntoId(id)
    setItems(null)
    setMastery(null)
    setAnswers({})
    setGrades({})
    setRevealed({})
    setGenErr(null)
    setGenerating(false)
    setGradingId(null)
    if (write) writeOntoHash({ col: 'learn', learn: 'quiz', ontologyId: id ?? undefined }, true)
  }, [])

  useEffect(() => {
    const sync = () => {
      const r = parseOntoHash()
      if (r?.col === 'learn' && r.learn === 'quiz') selectOntology(r.ontologyId ?? null, false)
    }
    window.addEventListener('popstate', sync)
    window.addEventListener('hashchange', sync)
    return () => {
      epoch.current++
      masteryEpoch.current++
      attemptsEpoch.current++
      window.removeEventListener('popstate', sync)
      window.removeEventListener('hashchange', sync)
    }
  }, [selectOntology])

  const refreshMastery = (oid: string) => {
    const expected = ++masteryEpoch.current
    api.quizMastery(oid).then((r) => {
      if (masteryEpoch.current === expected) setMastery(r.mastery)
    }).catch(() => { if (masteryEpoch.current === expected) setMastery([]) })
  }

  useEffect(() => {
    if (ontoId) refreshMastery(ontoId)
  }, [ontoId])

  const loadAttempts = (oid: string) => {
    const expected = ++attemptsEpoch.current
    api.quizAttempts(oid).then((r) => {
      if (attemptsEpoch.current === expected) setAttempts(r.attempts)
    }).catch(() => { if (attemptsEpoch.current === expected) setAttempts([]) })
  }

  const generate = async () => {
    if (!ontoId) return
    const expected = ++epoch.current
    setGenerating(true)
    setGenErr(null)
    setScopeNote(null)
    setItems(null)
    setGrades({})
    setRevealed({})
    setAnswers({})
    try {
      const r = await api.quizGenerate(ontoId, { difficulty, count: 3 })
      if (epoch.current !== expected) return
      setItems(r.items)
      // REQ-608-f4bb：超限本体采样出题如实提示（不冒充全量概念面）
      if (r.concept_used < r.concept_total) {
        setScopeNote(`该本体共 ${r.concept_total} 个概念，本轮出题基于 ${r.concept_used} 个概念子集（掌握度薄弱/从未考察优先，其余随机采样）——再出一批将覆盖不同概念`)
      }
    } catch (e: any) {
      if (epoch.current === expected) setGenErr(e?.message ?? '出题失败')
    } finally {
      if (epoch.current === expected) setGenerating(false)
    }
  }

  const submit = async (q: QuizItem) => {
    if (!ontoId || !answers[q.id]?.trim()) return
    const expected = epoch.current
    setGradingId(q.id)
    try {
      const g = await api.quizSubmit(ontoId, q.id, { answer: answers[q.id] })
      if (epoch.current !== expected) return
      setGrades((prev) => ({ ...prev, [q.id]: g }))
      setRevealed((prev) => ({ ...prev, [q.id]: true }))
      refreshMastery(ontoId)
      setGraphKey((k) => k + 1)
      if (showAttempts) loadAttempts(ontoId)
    } catch (e: any) {
      if (epoch.current === expected) setGenErr(e?.message ?? '批改失败')
    } finally {
      if (epoch.current === expected) setGradingId(null)
    }
  }

  const masterySummary = useMemo(() => {
    if (!mastery || mastery.length === 0) return null
    const avg = mastery.reduce((s, m) => s + m.score, 0) / mastery.length
    return { avg, weak: mastery.filter((m) => m.score < 0.5).length }
  }, [mastery])

  return (
    <Card
      className="work-card"
      size="small"
      title="出题自测（概念掌握度闭环）"
      extra={
        <Space size={8}>
          <Select
            showSearch
            optionFilterProp="label"
            placeholder="选择本体"
            style={{ minWidth: 200 }}
            value={ontoId}
            onChange={(v) => selectOntology(v)}
            options={ontologies.map((o) => ({ value: o.id, label: o.name }))}
          />
          <Select
            style={{ width: 116 }}
            value={difficulty}
            onChange={(v) => setDifficulty(v)}
            options={[
              { value: 'basic', label: '基础（定义）' },
              { value: 'applied', label: '应用（组合）' },
            ]}
          />
          <Button type="primary" disabled={!ontologies.some((o) => o.id === ontoId)} loading={generating} icon={<ReloadOutlined />} onClick={generate}>
            出题
          </Button>
        </Space>
      }
    >
      {!ontoId && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择一个本体开始出题自测（题型混合：单选/判断客观判定+简答 LLM 批改，批改落档累计概念掌握度并驱动下一轮定向出题）" />}
      {ontoId && !ontologies.some((o) => o.id === ontoId) && <Alert type="warning" showIcon title="当前清单中未找到所选本体" description="请确认本体服务可用，或重新选择本体。" />}

      {ontoId && masterySummary && (
        <div style={{ marginBottom: 12 }}>
          <Space size={14} align="center">
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>概念掌握度（均分）</Typography.Text>
            <Progress percent={Math.round(masterySummary.avg * 100)} style={{ width: 200, margin: 0 }} size="small" />
            <Tag color={masterySummary.weak > 0 ? 'gold' : 'green'} style={{ margin: 0 }}>
              {masterySummary.weak > 0 ? `${masterySummary.weak} 个薄弱概念（<0.5）` : '无薄弱概念'}
            </Tag>
          </Space>
          {mastery && mastery.length > 0 && (
            <div style={{ marginTop: 6, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {mastery.slice(0, 12).map((m) => (
                <Tag key={m.concept_name} color={m.score >= 0.8 ? 'green' : m.score >= 0.5 ? 'gold' : 'red'} style={{ margin: 0, fontSize: 11 }}>
                  {m.concept_name} {m.score.toFixed(2)}（{m.correct_count}/{m.asked_count}）
                </Tag>
              ))}
            </div>
          )}
        </div>
      )}

      {ontoId && (
        <div style={{ marginBottom: 12 }}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>掌握度子图（绿 ≥0.8 / 金 ≥0.5 / 红 &lt;0.5 / 灰=从未考察；薄弱与未考察概念下一轮出题优先）</Typography.Text>
          <div style={{ marginTop: 6 }}>
            <MasteryGraph key={ontoId} ontoId={ontoId} refreshKey={graphKey} />
          </div>
        </div>
      )}

      {ontoId && (
        <div style={{ marginBottom: 12 }}>
          <Button
            type="text"
            size="small"
            icon={<HistoryOutlined />}
            style={{ color: 'var(--c-text-secondary, #8a8fa3)', paddingInline: 4 }}
            onClick={() => {
              const next = !showAttempts
              setShowAttempts(next)
              if (next && ontoId) loadAttempts(ontoId)
            }}
          >
            批改记录{attempts ? `（${attempts.length}）` : ''}
          </Button>
          {showAttempts && (
            <div style={{ marginTop: 6 }}>
              {attempts && attempts.length === 0 && (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>暂无批改记录（提交作答后此处留档可回溯）</Typography.Text>
              )}
              {(attempts ?? []).map((a) => (
                <div key={a.id} style={{ display: 'flex', gap: 8, alignItems: 'baseline', padding: '3px 0', borderBottom: '1px dashed var(--c-border, #eef0f6)' }}>
                  <Typography.Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>{(a.created_at || '').replace('T', ' ').slice(0, 16)}</Typography.Text>
                  <Tag color={VERDICT_COLOR[a.verdict] ?? 'default'} style={{ margin: 0, fontSize: 11 }}>{VERDICT_TEXT[a.verdict] ?? a.verdict} · {(a.score * 100).toFixed(0)}</Tag>
                  <Typography.Text style={{ fontSize: 12, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {(a.question || a.quiz_item_id).slice(0, 40)} → {a.answer || '（空）'}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>{TYPE_TEXT[a.question_type] ?? a.question_type}</Typography.Text>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {genErr && <Alert type="error" showIcon style={{ marginBottom: 10 }} title="出题/批改失败" description={genErr} />}
      {scopeNote && <Alert type="info" showIcon style={{ marginBottom: 10 }} title="采样出题" description={scopeNote} />}

      {generating && <Typography.Text type="secondary">正在出题（LLM 生成中，约数秒）…</Typography.Text>}

      {(items ?? []).map((q) => {
        const g = grades[q.id]
        const objective = q.question_type === 'choice' || q.question_type === 'judge'
        return (
          <Card
            key={q.id}
            size="small"
            style={{ marginBottom: 10 }}
            title={
              <span>
                {/* REQ-545/M352：预检徽标（可答=绿/存疑=金/未预检不显——运行平面不可达时如实无标） */}
                {q.coverage_verdict === 'answerable' && <Tag color="green" style={{ marginInlineEnd: 6, fontSize: 11 }}>可答</Tag>}
                {q.coverage_verdict === 'not_answerable' && <Tag color="gold" style={{ marginInlineEnd: 6, fontSize: 11 }}>存疑</Tag>}
                {q.question}
              </span>
            }
          >
            <div style={{ marginBottom: 8 }}>
              {(q.concept_names ?? []).map((c) => (
                <Tag key={c} style={{ margin: '0 6px 0 0', fontSize: 11 }}>{c}</Tag>
              ))}
              <Tag style={{ margin: 0, fontSize: 11 }} color="default">{q.difficulty === 'applied' ? '应用' : '基础'} · {TYPE_TEXT[q.question_type ?? 'short_answer']}</Tag>
            </div>
            {objective ? (
              <Radio.Group
                disabled={!!g}
                value={answers[q.id] ?? ''}
                onChange={(e) => setAnswers((prev) => ({ ...prev, [q.id]: e.target.value }))}
                options={(q.options ?? []).map((o) => ({ value: o, label: o }))}
              />
            ) : (
              <Input.TextArea
                rows={3}
                placeholder="写下你的回答…"
                value={answers[q.id] ?? ''}
                disabled={!!g}
                onChange={(e) => setAnswers((prev) => ({ ...prev, [q.id]: e.target.value }))}
              />
            )}
            <Space size={8} style={{ marginTop: 8 }}>
              {!g && (
                <Button type="primary" size="small" disabled={!answers[q.id]?.trim()} loading={gradingId === q.id} onClick={() => submit(q)}>
                  提交批改
                </Button>
              )}
              {g && (
                <>
                  <Tag color={VERDICT_COLOR[g.verdict]} style={{ margin: 0 }}>{VERDICT_TEXT[g.verdict]} · {(g.score * 100).toFixed(0)} 分</Tag>
                  <Typography.Text style={{ fontSize: 13 }}>{g.feedback}</Typography.Text>
                </>
              )}
            </Space>
            {revealed[q.id] && (
              <div style={{ marginTop: 8, padding: '8px 10px', background: 'var(--c-bg-soft, #f6f7fb)', borderRadius: 6 }}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>参考答案</Typography.Text>
                <div className="chat-md" style={{ fontSize: 13, lineHeight: 1.75, whiteSpace: 'pre-wrap' }}>{q.answer}</div>
              </div>
            )}
          </Card>
        )
      })}

      {ontoId && items && items.length === 0 && !generating && (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本次未出出有效题目（均与近期题目重复或解析失败）——点「出题」重试" />
      )}
    </Card>
  )
}
