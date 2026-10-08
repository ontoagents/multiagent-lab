// OntoChatFlow 对话式多轮本体引导（REQ-103 模式 A）。
// 交互流：cq（领域描述+CQ）→ domain（逐轮补全，模型归纳+引导）→ 生成草稿（校验循环后端内聚）
// → 预览确认入库（REQ-82 门控）或回复修改意见进入 refine。会话留痕可切换/删除。
import { useCallback, useEffect, useRef, useState } from 'react'
import DoneCTA from './components/DoneCTA'
import { Alert, Button, Card, Checkbox, Collapse, Drawer, Dropdown, Empty, Input, InputNumber, List, Modal, Popconfirm, Skeleton, Space, Spin, Tag, Typography } from 'antd'
import { ApartmentOutlined, CommentOutlined, DeleteOutlined, DownOutlined, FileTextOutlined, PlusOutlined, SendOutlined, SolutionOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../api/client'
import type { OntoChatCluster, OntoChatExtractedCQ, OntoChatJob, OntoChatPrompt, OntoChatSession, Spec } from '../../api/types'
import LoadErrorAlert from '../../components/LoadErrorAlert'
import { DRAWER_SIZES, drawerSizeProps } from '../../lib/layout'
import { useUI } from '../../store/ui'

const STAGE_TAG: Record<string, { color: string; text: string }> = {
  story: { color: 'geekblue', text: '① 用户故事访谈' },
  cq: { color: 'default', text: '② 能力问题' },
  domain: { color: 'processing', text: '③ 领域补全' },
  draft: { color: 'blue', text: '④ 草稿就绪' },
  refine: { color: 'warning', text: '④ 草稿待修正' },
  done: { color: 'green', text: '⑤ 已入库' },
}

/** 首轮输入模板（cq 阶段占位提示） */
const CQ_TEMPLATE = '软件缺陷管理系统\n缺陷源于哪个需求？\n缺陷影响哪些模块？'

export default function OntoChatFlow({ onSaved }: { onSaved: (ontologyId: string) => void }) {
  const { showToast } = useUI()
  const [sessions, setSessions] = useState<OntoChatSession[]>([])
  const [sessionsErr, setSessionsErr] = useState<string | null>(null)
  const [active, setActive] = useState<OntoChatSession | null>(null)
  const [loading, setLoading] = useState(true)
  const [turning, setTurning] = useState(false)
  const [input, setInput] = useState('')
  const [draft, setDraft] = useState<Spec | null>(null)
  const [draftWarning, setDraftWarning] = useState('')
  const [saveName, setSaveName] = useState('')
  const [restored, setRestored] = useState<{ round: number; draft: boolean } | null>(null)
  const [job, setJob] = useState<OntoChatJob | null>(null)
  const [promptsOpen, setPromptsOpen] = useState(false)
  const [prompts, setPrompts] = useState<OntoChatPrompt[] | null>(null)
  const [cqDraft, setCqDraft] = useState<{ text: string; origin?: string; checked: boolean }[] | null>(null)
  const [clusterDraft, setClusterDraft] = useState<{ label: string; rows: { text: string; checked: boolean }[] }[] | null>(null)
  const [analyzeOpen, setAnalyzeOpen] = useState(false)
  const [analyzeMax, setAnalyzeMax] = useState<number | null>(null)
  const [storyTpl, setStoryTpl] = useState<{ label: string; text: string }[] | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const jobRef = useRef<string | null>(null)
  const listEndRef = useRef<HTMLDivElement>(null)

  const refreshList = useCallback(async () => {
    try {
      const list = await api.listOntoChatSessions()
      setSessions(list)
      setSessionsErr(null)
      return list
    } catch (e: any) {
      setSessionsErr(e?.message ?? '加载失败')
      return []
    }
  }, [])

  const stopJobPoll = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  // REQ-271：生成 job 轮询（1.5s）——终态拉会话最新态（消息含回复/错误留痕），done 附带草稿
  const startJobPoll = useCallback((jobId: string) => {
    stopJobPoll()
    jobRef.current = jobId
    setJob({ id: jobId, session_id: '', status: 'running', created_at: '', updated_at: '' })
    pollRef.current = setInterval(async () => {
      try {
        const j = await api.getOntoChatJob(jobId)
        setJob(j)
        if (j.status === 'done' || j.status === 'error' || j.status === 'cancelled') {
          stopJobPoll()
          jobRef.current = null
          setJob(null)
          setTurning(false)
          const s = await api.getOntoChatSession(j.session_id)
          setActive(s)
          await refreshList()
          if (j.status === 'done' && j.result?.clusters?.length) {
            // REQ-273：簇结果 → Collapse 勾选卡（人工确认步）
            setClusterDraft(
              (j.result.clusters as OntoChatCluster[]).map((cl) => ({
                label: cl.label,
                rows: (cl.cqs ?? []).map((q) => ({ text: q, checked: true })),
              })),
            )
            setAnalyzeOpen(false)
          } else if (j.status === 'done' && j.result?.cqs?.length) {
            // REQ-272：CQ 候选 → 可编辑确认卡
            setCqDraft(j.result.cqs.map((c: OntoChatExtractedCQ) => ({ text: c.cq, origin: c.origin, checked: true })))
          } else if (j.status === 'done' && j.result?.draft) {
            setDraft(j.result.draft)
            setSaveName(j.result.draft.name || '')
            setDraftWarning(j.result.warning || '')
          } else if (j.status === 'error') {
            showToast(`生成失败：${j.error ?? '未知错误'}`, 'err')
          } else if (j.status === 'cancelled') {
            showToast('已取消生成')
          }
        }
      } catch {
        /* 单次轮询失败下次重试 */
      }
    }, 1500)
  }, [refreshList, showToast, stopJobPoll])

  const cancelJob = async () => {
    if (!job) return
    try {
      await api.cancelOntoChatJob(job.id)
      showToast('正在取消…')
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  // REQ-272：抽取 CQ 候选（异步 job，复用轮询；done 时经 startJobPoll 填充确认卡）
  const extractCQs = async () => {
    if (!active) return
    setTurning(true)
    try {
      const r = await api.extractOntoChatCQs(active.id)
      if (r.job_id) {
        jobRef.current = r.job_id
        setJob({ id: r.job_id, session_id: active.id, status: 'running', created_at: '', updated_at: '' })
        startJobPoll(r.job_id)
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      if (!jobRef.current) setTurning(false)
    }
  }

  // REQ-272：人工确认写入会话（analyze 确认步定案保留；生成草稿时回写 spec.CQ）
  const confirmCQs = async () => {
    if (!active || !cqDraft) return
    const list = cqDraft.filter((c) => c.checked && c.text.trim()).map((c) => c.text.trim())
    if (list.length === 0) {
      showToast('请至少勾选并填写一条能力问题', 'err')
      return
    }
    setTurning(true)
    try {
      const r = await api.setOntoChatCQs(active.id, list)
      setActive(r.session)
      setCqDraft(null)
      await refreshList()
      showToast(`已确认能力问题 ${list.length} 条`)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setTurning(false)
    }
  }

  // REQ-273：CQ 去重与主题聚类（异步 job；可选簇数；结果经人工确认应用写回）
  const runAnalyze = async () => {
    if (!active) return
    setTurning(true)
    try {
      const r = await api.analyzeOntoChatCQs(active.id, analyzeMax ?? 0)
      setAnalyzeOpen(false)
      if (r.job_id) {
        jobRef.current = r.job_id
        setJob({ id: r.job_id, session_id: active.id, status: 'running', created_at: '', updated_at: '' })
        startJobPoll(r.job_id)
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      if (!jobRef.current) setTurning(false)
    }
  }

  const applyClusters = async () => {
    if (!active || !clusterDraft) return
    const list = clusterDraft.flatMap((cl) => cl.rows.filter((r) => r.checked && r.text.trim()).map((r) => r.text.trim()))
    if (list.length === 0) {
      showToast('请至少勾选一条能力问题', 'err')
      return
    }
    setTurning(true)
    try {
      const r = await api.setOntoChatCQs(active.id, list)
      setActive(r.session)
      setClusterDraft(null)
      await refreshList()
      showToast(`已应用去重结果：写入能力问题 ${list.length} 条`)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setTurning(false)
    }
  }

  // REQ-275：访谈动作
  const backStory = async () => {
    if (!active) return
    try {
      const r = await api.storyBack(active.id)
      setActive(r.session)
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }
  const finishStory = async () => {
    if (!active) return
    setTurning(true)
    try {
      const r = await api.storyFinish(active.id)
      if (r.job_id) {
        jobRef.current = r.job_id
        setJob({ id: r.job_id, session_id: active.id, status: 'running', created_at: '', updated_at: '' })
        startJobPoll(r.job_id)
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      if (!jobRef.current) setTurning(false)
    }
  }
  const loadTemplates = async () => {
    if (storyTpl) return
    try {
      setStoryTpl(await api.storyTemplates())
    } catch {
      /* 引导卡失败静默（辅助功能） */
    }
  }

  // REQ-271⑥：提示词只读清单（懒加载；页面显示=运行时注入同一份数据）
  const openPrompts = async () => {
    setPromptsOpen(true)
    if (!prompts) {
      try {
        setPrompts(await api.listOntoChatPrompts())
      } catch (e: any) {
        showToast(e.message, 'err')
      }
    }
  }

  const openSession = useCallback(async (id: string) => {
    setLoading(true)
    try {
      const s = await api.getOntoChatSession(id)
      setActive(s)
      // 从上下文恢复草稿预览（refine/done 阶段重进不丢）
      if (s.context?.draft_spec) {
        try {
          setDraft(s.context.draft_spec as Spec)
        } catch {
          setDraft(null)
        }
      } else {
        setDraft(null)
      }
      setDraftWarning('')
      setSaveName(s.title || '')
      // REQ-271：重进会话/刷新后恢复进行中生成任务的轮询
      if (jobRef.current == null) {
        try {
          const { job: aj } = await api.getOntoChatSessionActiveJob(id)
          if (aj) {
            setTurning(true)
            startJobPoll(aj.id)
          }
        } catch {
          /* 忽略 */
        }
      }
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setLoading(false)
    }
  }, [showToast, startJobPoll])

  useEffect(() => {
    ;(async () => {
      const list = await refreshList()
      if (list.length > 0) await openSession(list[0].id)
      else setLoading(false)
    })()
  }, [refreshList, openSession])

  useEffect(() => {
    listEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [active?.messages?.length, turning])

  useEffect(() => () => stopJobPoll(), [stopJobPoll])

  const newSession = async (mode?: 'guided') => {
    try {
      setRestored(null)
      const s = await api.createOntoChatSession(`引导 ${new Date().toLocaleString()}`, mode)
      await refreshList()
      setActive(s)
      setDraft(null)
      setDraftWarning('')
      setSaveName('')
      setInput('')
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const removeSession = async (id: string) => {
    try {
      await api.deleteOntoChatSession(id)
      const list = await refreshList()
      if (active?.id === id) {
        setActive(null)
        setDraft(null)
        if (list.length > 0) await openSession(list[0].id)
      }
      showToast('会话已删除')
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const send = async (text: string, feedback?: string) => {
    if (!active) return
    if (!text.trim() && !feedback?.trim()) {
      showToast('请输入内容', 'err')
      return
    }
    setTurning(true)
    try {
      const r = await api.ontoChatTurn(active.id, text, feedback)
      setInput('')
      setActive(r.session)
      if (r.job_id) {
        // REQ-271 生成轮异步：202 → 轮询 job 终态（进度可看/可取消）
        startJobPoll(r.job_id)
        return
      }
      if (r.draft) {
        setDraft(r.draft)
        setSaveName(r.draft.name || '')
      }
      if (r.warning) setDraftWarning(r.warning)
      else if (r.draft) setDraftWarning('')
      await refreshList()
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 503) showToast('LLM 未配置：请在「设置-模型连接」配置默认 chat 连接', 'err')
      else showToast(e.message, 'err')
      // REQ-271 错误留痕：服务端已落 assistant 错误消息，刷新会话让错误气泡可见（不再只有转瞬 toast）
      try {
        const s = await api.getOntoChatSession(active.id)
        setActive(s)
      } catch {
        /* 忽略 */
      }
    } finally {
      if (!jobRef.current) setTurning(false)
    }
  }

  const saveDraft = async () => {
    if (!active || !draft) return
    const nm = saveName.trim() || draft.name || ''
    if (!nm) {
      showToast('请输入本体名称', 'err')
      return
    }
    setTurning(true)
    try {
      const r = await api.ontoChatSave(active.id, nm)
      showToast(`已创建本体「${r.ontology.name}」`)
      await refreshList()
      setActive(r.session)
      onSaved(r.ontology.id)
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) showToast(`草稿校验未通过：${e.validationErrors[0].message}`, 'err')
      else showToast(e.message, 'err')
    } finally {
      setTurning(false)
    }
  }

  const stage = active?.stage ?? 'cq'
  const storyActive = stage === 'story'
  const draftStory = active?.context?.draft_story ?? ''
  const storyStep = active?.context?.story_step ?? 0
  useEffect(() => {
    if (storyActive) loadTemplates()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storyActive])
  const stageTag = STAGE_TAG[stage] ?? STAGE_TAG.cq
  const isDone = stage === 'done'
  const placeholder =
    stage === 'story'
      ? draftStory
        ? '回复修改意见精修用户故事…或点「完成并抽取 CQ」'
        : '回答当前问题（一问一轮）…可点下方模板快速填写'
      : stage === 'cq'
        ? `第一行领域描述，其后每行一条能力问题（CQ），如：\n${CQ_TEMPLATE}`
        : stage === 'domain'
        ? '补充领域信息（概念/层级/关系/实例来源）…或点击「生成草稿」直接产出'
        : '回复修改意见进入修正轮（如：给 Deployment 增加副本数属性）…'

  const msgs = active?.messages ?? []
  const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant')
  const lastUserMsg = [...msgs].reverse().find((m) => m.role === 'user')
  const lastUserText = lastUserMsg?.content
  // REQ-271：末条 assistant 为错误留痕时给一键重试（重发上一条用户输入）
  const canRetry =
    !turning && !job && !!lastAssistant && !!lastUserText &&
    (lastAssistant.content.startsWith('生成失败：') || lastAssistant.content.startsWith('本轮处理失败：'))

  return (
    <Card className="work-card" size="small">
      <Alert
        type="success"
        showIcon
        style={{ marginBottom: 12 }}
        title="对话式本体构建（OntoChat 流程，REQ-103 模式 A；REQ-271 生成异步化）"
        description="对话式 CQ 引导 → 逐轮补全领域信息 → 「生成草稿」异步产出 spec_json（生成-校验循环后端内聚；进度可看、可取消、失败留痕可重试）→ 预览确认入库。右上「提示词」可查看实际注入模型的提示词（只读）。"
      />
      <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
        {/* 会话列表 */}
        <div style={{ width: 220, flexShrink: 0 }}>
          <Space style={{ marginBottom: 8, width: '100%', justifyContent: 'space-between' }}>
            <Typography.Text strong>会话</Typography.Text>
            <Dropdown
              menu={{
                items: [
                  { key: 'quick', label: '快速模式（直接描述领域）' },
                  { key: 'guided', label: '访谈模式（用户故事共创，REQ-275）' },
                ],
                onClick: ({ key }) => newSession(key === 'guided' ? 'guided' : undefined),
              }}
              trigger={['click']}
            >
              <Button size="small" icon={<PlusOutlined />}>
                新建 <DownOutlined style={{ fontSize: 10 }} />
              </Button>
            </Dropdown>
          </Space>
          {sessionsErr && (
            <LoadErrorAlert
              title="会话列表加载失败"
              message={sessionsErr}
              onRetry={() => {
                refreshList()
              }}
              style={{ marginBottom: 8 }}
            />
          )}
          <List
            size="small"
            dataSource={sessions}
            locale={{ emptyText: '暂无会话' }}
            renderItem={(s) => (
              <List.Item
                style={{
                  cursor: 'pointer',
                  padding: '6px 8px',
                  background: active?.id === s.id ? 'var(--ant-color-primary-bg, #e6f4ff)' : undefined,
                  borderRadius: 6,
                }}
                onClick={() => openSession(s.id)}
                actions={[
                  <Popconfirm key="del" title="删除该会话？" onConfirm={(e) => { e?.stopPropagation(); removeSession(s.id) }}>
                    <Button type="text" size="small" icon={<DeleteOutlined />} aria-label="删除会话" onClick={(e) => e.stopPropagation()} />
                  </Popconfirm>,
                ]}
              >
                <div style={{ minWidth: 0 }}>
                  <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 13 }}>{s.title || s.id}</div>
                  <Tag {...(STAGE_TAG[s.stage] ?? {})} style={{ margin: 0, fontSize: 11 }}>
                    {STAGE_TAG[s.stage]?.text ?? s.stage}
                  </Tag>
                </div>
              </List.Item>
            )}
          />
        </div>

        {/* 对话区 */}
        <div style={{ flex: 1, minWidth: 0 }}>
          {loading ? (
            <Skeleton active paragraph={{ rows: 6 }} style={{ padding: 24, marginTop: 16 }} />
          ) : !active ? (
            <Empty description="新建或选择一个会话开始" style={{ padding: 40 }} />
          ) : (
            <>
              <Space size={8} style={{ marginBottom: 8 }} wrap>
                <Tag {...stageTag} style={{ margin: 0 }}>{stageTag.text}</Tag>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>轮数 {active.round}</Typography.Text>
                {!!active.context?.cqs?.length && (
                  <Tag color="purple" style={{ margin: 0 }}>CQ {active.context.cqs.length}</Tag>
                )}
                {storyActive && !draftStory && (
                  <Tag color="geekblue" style={{ margin: 0 }}>访谈 {Math.min(storyStep + 1, 5)}/5</Tag>
                )}
                <Button size="small" icon={<FileTextOutlined />} onClick={openPrompts}>
                  提示词
                </Button>
                {active.ontology_id && (
                  <Tag color="green" style={{ margin: 0 }}>产物 {active.ontology_id}</Tag>
                )}
                {restored && restored.round > 0 && (
                  <Tag color="blue" style={{ margin: 0 }}>中断恢复：已还原 {restored.round} 轮{restored.draft ? '与草稿' : ''}（服务端持久化）</Tag>
                )}
              </Space>
              <div
                className="onto-chat-msgs"
                style={{ maxHeight: 380, overflowY: 'auto', padding: '4px 2px', display: 'flex', flexDirection: 'column', gap: 8 }}
              >
                {(active.messages ?? []).map((m, i) => {
                  const isErr = m.role === 'assistant' && (m.content.startsWith('生成失败：') || m.content.startsWith('本轮处理失败：') || m.content.startsWith('CQ 抽取失败：'))
                  const isCancel = m.role === 'assistant' && m.content.startsWith('已取消生成')
                  return (
                    <div
                      key={i}
                      style={{
                        alignSelf: m.role === 'user' ? 'flex-end' : 'flex-start',
                        maxWidth: '86%',
                        background: m.role === 'user'
                          ? 'var(--ant-color-primary-bg, #e6f4ff)'
                          : isErr
                            ? 'var(--ant-color-error-bg, #fff2f0)'
                            : 'var(--ant-color-bg-layout, #f5f5f5)',
                        border: isErr ? '1px solid var(--ant-color-error-border, #ffccc7)' : undefined,
                        borderRadius: 8,
                        padding: '6px 10px',
                        whiteSpace: 'pre-wrap',
                        fontSize: 13,
                        lineHeight: 1.6,
                        opacity: isCancel ? 0.75 : 1,
                      }}
                    >
                      {m.content}
                    </div>
                  )
                })}
                {turning && !job && (
                  <div style={{ alignSelf: 'flex-start' }}>
                    <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>思考中…</Typography.Text>
                  </div>
                )}
                {job && (
                  <div className="onto-chat-job-bubble" style={{ alignSelf: 'flex-start', display: 'flex', alignItems: 'center', gap: 8, background: 'var(--ant-color-bg-layout, #f5f5f5)', borderRadius: 8, padding: '6px 10px' }}>
                    <Spin size="small" />
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>{job.progress || '任务排队中…'}</Typography.Text>
                    <Button size="small" onClick={cancelJob}>取消</Button>
                  </div>
                )}
                <div ref={listEndRef} />
              </div>

              {/* 用户故事卡（REQ-275：访谈制品 + 精修 + 完成入口） */}
              {storyActive && draftStory && (
                <Card size="small" style={{ marginTop: 10 }} title="用户故事（访谈制品，可精修）" data-testid="story-card">
                  <pre
                    style={{
                      margin: 0,
                      padding: 10,
                      background: 'var(--ant-color-bg-layout, #f5f5f5)',
                      borderRadius: 6,
                      fontSize: 12,
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-word',
                      maxHeight: 260,
                      overflowY: 'auto',
                    }}
                  >
                    {draftStory}
                  </pre>
                  <Space style={{ marginTop: 8 }}>
                    <Button size="small" onClick={backStory}>
                      上一步（重答末问）
                    </Button>
                    <Button size="small" type="primary" icon={<SolutionOutlined />} loading={turning} onClick={finishStory} data-testid="story-finish-btn">
                      完成并抽取 CQ
                    </Button>
                  </Space>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
                    直接在下方输入框回复修改意见即可精修；完成后进入能力问题抽取（候选经确认卡写回，生成草稿回写 spec.CQ）。
                  </Typography.Paragraph>
                </Card>
              )}

              {/* CQ 聚类结果卡（REQ-273：人工确认步——聚类仅供参考分组） */}
              {clusterDraft && !isDone && (
                <Card size="small" style={{ marginTop: 10 }} title="CQ 主题聚类（人工确认后应用）">
                  <Collapse
                    size="small"
                    defaultActiveKey={clusterDraft.map((_, i) => String(i))}
                    items={clusterDraft.map((cl, i) => ({
                      key: String(i),
                      label: `${cl.label}（${cl.rows.filter((r) => r.checked).length}/${cl.rows.length}）`,
                      children: cl.rows.map((row, j) => (
                        <div key={j} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 4 }}>
                          <Checkbox
                            checked={row.checked}
                            onChange={(e) =>
                              setClusterDraft(
                                clusterDraft.map((c2, k) =>
                                  k === i ? { ...c2, rows: c2.rows.map((r2, m) => (m === j ? { ...r2, checked: e.target.checked } : r2)) } : c2,
                                ),
                              )
                            }
                          />
                          <Typography.Text style={{ fontSize: 13, flex: 1 }}>{row.text}</Typography.Text>
                        </div>
                      )),
                    }))}
                  />
                  <Space style={{ marginTop: 8 }}>
                    <Button size="small" type="primary" loading={turning} onClick={applyClusters}>
                      应用去重结果（写回 {clusterDraft.reduce((n, cl) => n + cl.rows.filter((r) => r.checked && r.text.trim()).length, 0)} 条）
                    </Button>
                    <Button size="small" onClick={() => setClusterDraft(null)}>
                      放弃
                    </Button>
                  </Space>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
                    诚实边界：聚类仅供参考分组、不能单独支撑完整分析——写回后生成草稿仍以全部勾选问题为能力问题依据（spec.CQ 可追溯）。
                  </Typography.Paragraph>
                </Card>
              )}

              {/* CQ 候选确认卡（REQ-272：analyze 人工确认步） */}
              {cqDraft && !isDone && (
                <Card
                  size="small"
                  style={{ marginTop: 10 }}
                  title={`能力问题候选确认（勾选 ${cqDraft.filter((c) => c.checked && c.text.trim()).length}/${cqDraft.length}）`}
                >
                  {cqDraft.map((row, i) => (
                    <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6 }}>
                      <Checkbox
                        checked={row.checked}
                        onChange={(e) => setCqDraft(cqDraft.map((r2, j2) => (j2 === i ? { ...r2, checked: e.target.checked } : r2)))}
                      />
                      <Input
                        size="small"
                        value={row.text}
                        onChange={(e) => setCqDraft(cqDraft.map((r2, j2) => (j2 === i ? { ...r2, text: e.target.value } : r2)))}
                        style={{ flex: 1 }}
                      />
                      {row.origin && <Tag style={{ margin: 0, fontSize: 11 }}>{row.origin}</Tag>}
                      <Button size="small" type="text" icon={<DeleteOutlined />} aria-label="删除候选" onClick={() => setCqDraft(cqDraft.filter((_, j2) => j2 !== i))} />
                    </div>
                  ))}
                  <Space style={{ marginTop: 4 }} size={8}>
                    <Button size="small" onClick={() => setCqDraft([...cqDraft, { text: '', checked: true }])}>
                      手动添加
                    </Button>
                    <Button
                      size="small"
                      type="primary"
                      loading={turning}
                      disabled={cqDraft.filter((c) => c.checked && c.text.trim()).length === 0}
                      onClick={confirmCQs}
                    >
                      确认写入 {cqDraft.filter((c) => c.checked && c.text.trim()).length} 条
                    </Button>
                  </Space>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
                    确认后生成草稿将据此补充建模，并回写 spec.CQ 入本体资产（可追溯）。
                  </Typography.Paragraph>
                </Card>
              )}

              {/* 草稿预览 + 入库（REQ-82 门控：必须经用户确认） */}
              {draft && !isDone && (
                <Card size="small" style={{ marginTop: 10 }} title="草稿预览（确认后入库）">
                  <Space size={6} wrap>
                    <Tag color="blue" style={{ margin: 0 }}>概念 {draft.concepts?.length ?? 0}</Tag>
                    <Tag color="geekblue" style={{ margin: 0 }}>关系 {draft.relations?.length ?? 0}</Tag>
                    <Tag color="purple" style={{ margin: 0 }}>实例 {draft.instances?.length ?? 0}</Tag>
                  </Space>
                  {draftWarning && <Alert type="warning" showIcon style={{ marginTop: 8 }} title={draftWarning} />}
                  <Space style={{ marginTop: 10 }} size={8}>
                    <Input value={saveName} onChange={(e) => setSaveName(e.target.value)} placeholder="本体名称" style={{ width: 240 }} />
                    <Button type="primary" icon={<ThunderboltOutlined />} loading={turning} onClick={saveDraft}>
                      创建并保存
                    </Button>
                  </Space>
                </Card>
              )}
              {isDone && active.ontology_id && (
                <Alert
                  type="success"
                  showIcon
                  style={{ marginTop: 10 }}
                  title={`草稿已入库为「${active.title}」关联的本体 ${active.ontology_id}`}
                  description={<DoneCTA ontologyId={active.ontology_id} detail="对话式建模完成" />}
                />
              )}

              {/* 引导卡 chips（REQ-275：访谈阶段 P3 模板点击填入；模板是辅助非必填） */}
              {storyActive && storyTpl && !draftStory && (
                <div style={{ marginTop: 8, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  {storyTpl.map((t) => (
                    <Button
                      key={t.label}
                      size="small"
                      icon={<CommentOutlined />}
                      onClick={() => setInput(t.text)}
                      title={t.text.replace(/\*\*/g, '').replace(/\*/g, '')}
                    >
                      {t.label}
                    </Button>
                  ))}
                </div>
              )}

              {/* 输入区 */}
              {!isDone && (
                <div style={{ marginTop: 10, display: 'flex', gap: 8, alignItems: 'flex-start' }}>
                  {canRetry && (
                    <Button danger onClick={() => lastUserText && send(lastUserText)}>
                      重试上一轮
                    </Button>
                  )}
                  <Input.TextArea
                    style={{ flex: 1 }}
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    placeholder={placeholder}
                    autoSize={{ minRows: 2, maxRows: 5 }}
                    disabled={turning}
                    onPressEnter={(e) => {
                      if (!e.shiftKey) {
                        e.preventDefault()
                        send(input)
                      }
                    }}
                  />
                  {storyActive && !draftStory && (
                    <>
                      <Button disabled={turning || storyStep === 0} onClick={backStory}>
                        上一步
                      </Button>
                      <Button disabled={turning} onClick={() => send('（跳过）')}>
                        跳过
                      </Button>
                    </>
                  )}
                  <Button icon={<SolutionOutlined />} disabled={turning} onClick={extractCQs}>
                    抽取 CQ
                  </Button>
                  <Button
                    icon={<ApartmentOutlined />}
                    disabled={turning || !active?.context?.cqs?.length}
                    title={active?.context?.cqs?.length ? '对已确认的能力问题做去重与主题聚类' : '先抽取或补充能力问题后再分析'}
                    onClick={() => setAnalyzeOpen(true)}
                  >
                    CQ 分析
                  </Button>
                  {(stage === 'domain' || stage === 'draft' || stage === 'refine') && (
                    <Button icon={<ThunderboltOutlined />} disabled={turning} onClick={() => send('生成草稿')}>
                      生成草稿
                    </Button>
                  )}
                  <Button type="primary" icon={<SendOutlined />} loading={turning} onClick={() => send(input)}>
                    发送
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
      <Modal title="CQ 分析：去重与主题聚类" open={analyzeOpen} onCancel={() => setAnalyzeOpen(false)} footer={null}>
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          对当前会话已确认的 {active?.context?.cqs?.length ?? 0} 条能力问题做语义去重（等价合并保留最清晰表述）与主题聚类。
          诚实边界：聚类仅供参考分组、不能单独支撑完整分析——结果须人工确认后才应用写回。
        </Typography.Paragraph>
        <Space>
          <span style={{ fontSize: 13 }}>簇数（留空自动）</span>
          <InputNumber min={2} max={8} value={analyzeMax} onChange={(v) => setAnalyzeMax(v ?? null)} />
          <Button type="primary" icon={<ApartmentOutlined />} loading={turning} onClick={runAnalyze}>
            开始分析
          </Button>
        </Space>
      </Modal>
      <Drawer
        title="OntoChat 提示词（只读）"
        open={promptsOpen}
        onClose={() => setPromptsOpen(false)}
        {...drawerSizeProps('ontoChatPrompts', DRAWER_SIZES.medium)}
      >
        {(prompts ?? []).map((p) => (
          <Card key={p.id} size="small" title={p.label} style={{ marginBottom: 12 }}>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 4 }}>{p.purpose}</Typography.Paragraph>
            <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 8 }}>来源：{p.source}</Typography.Paragraph>
            <pre
              style={{
                margin: 0,
                padding: 10,
                background: 'var(--ant-color-bg-layout, #f5f5f5)',
                borderRadius: 6,
                fontSize: 12,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                maxHeight: 360,
                overflowY: 'auto',
              }}
            >
              {p.text}
            </pre>
          </Card>
        ))}
        {!prompts && <Skeleton active />}
      </Drawer>
    </Card>
  )
}
