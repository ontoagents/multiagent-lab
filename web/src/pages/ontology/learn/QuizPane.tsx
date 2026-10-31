import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Card, Empty, Input, Progress, Select, Space, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import type { Ontology } from '../../../api/types'

/**
 * 出题自测（REQ-524/M331——Q-23 衍生③ 转正交付，学习动线出题·批改·掌握度最小闭环）：
 * 选本体 → LLM 概念子图出题（难度/题数可选）→ 作答 → LLM 批改（对照参考答案评分+点评，
 * 作答后才展示参考答案）→ 概念级掌握度落表（0.7 旧+0.3 新加权，薄弱优先排序）。
 * 地基复用=平台模型代理（chat.GenerateText 直调）+构建平面 spec 概念面；与 REQ-512 打卡动作
 * 校验、REQ-508 冷启动建模统筹（姊妹件边界=构建面快筛 vs 学习面自测）。
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

const VERDICT_COLOR: Record<string, string> = { correct: 'green', partial: 'gold', wrong: 'red' }
const VERDICT_TEXT: Record<string, string> = { correct: '答对', partial: '部分正确', wrong: '未答对' }

export default function QuizPane({ ontologies }: { ontologies: Ontology[] }) {
  const [ontoId, setOntoId] = useState<string | null>(null)
  const [difficulty, setDifficulty] = useState<'basic' | 'applied'>('basic')
  const [items, setItems] = useState<QuizItem[] | null>(null)
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [grades, setGrades] = useState<Record<string, GradeResult>>({})
  const [revealed, setRevealed] = useState<Record<string, boolean>>({})
  const [generating, setGenerating] = useState(false)
  const [gradingId, setGradingId] = useState<string | null>(null)
  const [mastery, setMastery] = useState<MasteryLevel[] | null>(null)
  const [genErr, setGenErr] = useState<string | null>(null)

  const refreshMastery = (oid: string) => {
    api.quizMastery(oid).then((r) => setMastery(r.mastery)).catch(() => setMastery([]))
  }

  useEffect(() => {
    if (ontoId) refreshMastery(ontoId)
  }, [ontoId])

  const generate = async () => {
    if (!ontoId) return
    setGenerating(true)
    setGenErr(null)
    setItems(null)
    setGrades({})
    setRevealed({})
    setAnswers({})
    try {
      const r = await api.quizGenerate(ontoId, { difficulty, count: 3 })
      setItems(r.items)
    } catch (e: any) {
      setGenErr(e?.message ?? '出题失败')
    } finally {
      setGenerating(false)
    }
  }

  const submit = async (q: QuizItem) => {
    if (!ontoId || !answers[q.id]?.trim()) return
    setGradingId(q.id)
    try {
      const g = await api.quizSubmit(ontoId, q.id, { answer: answers[q.id] })
      setGrades((prev) => ({ ...prev, [q.id]: g }))
      setRevealed((prev) => ({ ...prev, [q.id]: true }))
      refreshMastery(ontoId)
    } catch (e: any) {
      setGenErr(e?.message ?? '批改失败')
    } finally {
      setGradingId(null)
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
            onChange={(v) => {
              setOntoId(v)
              setItems(null)
              setMastery(null)
            }}
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
          <Button type="primary" disabled={!ontoId} loading={generating} icon={<ReloadOutlined />} onClick={generate}>
            出题
          </Button>
        </Space>
      }
    >
      {!ontoId && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择一个本体开始出题自测（围绕概念定义与概念间关系出简答题，作答后 LLM 批改并累计概念掌握度）" />}

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

      {genErr && <Alert type="error" showIcon style={{ marginBottom: 10 }} title="出题/批改失败" description={genErr} />}

      {generating && <Typography.Text type="secondary">正在出题（LLM 生成中，约数秒）…</Typography.Text>}

      {(items ?? []).map((q) => {
        const g = grades[q.id]
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
              <Tag style={{ margin: 0, fontSize: 11 }} color="default">{q.difficulty === 'applied' ? '应用' : '基础'}</Tag>
            </div>
            <Input.TextArea
              rows={3}
              placeholder="写下你的回答…"
              value={answers[q.id] ?? ''}
              disabled={!!g}
              onChange={(e) => setAnswers((prev) => ({ ...prev, [q.id]: e.target.value }))}
            />
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
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本次未出出有效题目（模型输出解析失败）——点「出题」重试" />
      )}
    </Card>
  )
}
