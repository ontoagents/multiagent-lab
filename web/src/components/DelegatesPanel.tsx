import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Input, Space, Tag, Tooltip, Typography } from 'antd'
import { CaretRightOutlined, PauseOutlined, SendOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { ConversationDelegate } from '../api/types'

// ---------------------------------------------------------------------------
// REQ-208/M42 余项：可继续子 Agent（continuation）——会话级「后台委派」面板
// （挂在调用轨迹面板顶部；委派由协调者 delegate_background 工具或既有会话产生）。
// 状态机 running|completed|failed|stopped；人工动作=插话（steer）/打断（interrupt）/
// 恢复（continue，冷恢复同路径）；结果全文经详情拉取在行下展开。
// 轮询纪律：仅在存在 running 委派时 3s 轮询，否则静默（无委派零请求）。
// ---------------------------------------------------------------------------

const STATUS_META: Record<string, { color: string; text: string }> = {
  running: { color: 'processing', text: '运行中' },
  completed: { color: 'success', text: '已完成' },
  failed: { color: 'error', text: '失败' },
  stopped: { color: 'default', text: '已停止' },
}

export default function DelegatesPanel({ conversationId }: { conversationId: string }) {
  const [rows, setRows] = useState<ConversationDelegate[]>([])
  const [steerFor, setSteerFor] = useState<string | null>(null)
  const [steerText, setSteerText] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [openResult, setOpenResult] = useState<string | null>(null)
  const [resultText, setResultText] = useState('')
  const timer = useRef<number | null>(null)

  const load = useCallback(() => {
    api
      .listDelegates(conversationId)
      .then(setRows)
      .catch(() => setRows([]))
  }, [conversationId])

  useEffect(() => {
    if (!conversationId) return
    load()
  }, [conversationId, load])

  // 存在 running 委派时 3s 轮询（无委派/全终态零请求）
  useEffect(() => {
    if (timer.current) {
      window.clearInterval(timer.current)
      timer.current = null
    }
    if (rows.some((r) => r.status === 'running')) {
      timer.current = window.setInterval(load, 3000)
    }
    return () => {
      if (timer.current) {
        window.clearInterval(timer.current)
        timer.current = null
      }
    }
  }, [rows, load])

  const doSteer = async (id: string) => {
    if (!steerText.trim()) return
    setBusy(id)
    try {
      await api.steerDelegate(id, steerText.trim())
      setSteerText('')
      setSteerFor(null)
      load()
    } finally {
      setBusy(null)
    }
  }

  const doAction = async (id: string, kind: 'interrupt' | 'continue') => {
    setBusy(id)
    try {
      if (kind === 'interrupt') await api.interruptDelegate(id)
      else await api.continueDelegate(id)
      load()
    } finally {
      setBusy(null)
    }
  }

  const showResult = async (id: string) => {
    if (openResult === id) {
      setOpenResult(null)
      return
    }
    const r = await api.getDelegate(id)
    setResultText(r.result || r.delegate?.result_digest || r.delegate?.error || '（暂无结果）')
    setOpenResult(id)
  }

  if (rows.length === 0) return null

  return (
    <div style={{ marginBottom: 10 }} data-testid="delegates-panel">
      <Typography.Text strong style={{ fontSize: 12 }}>
        后台委派（{rows.length}）
      </Typography.Text>
      <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 8 }}>
        可继续子 Agent（REQ-208）：插话引导 / 打断 / 冷恢复；子会话历史完整可随时接续
      </Typography.Text>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 6 }}>
        {rows.map((d) => {
          const meta = STATUS_META[d.status] ?? { color: 'default', text: d.status }
          return (
            <div
              key={d.id}
              style={{ border: '1px solid var(--ant-color-border, #eee)', borderRadius: 6, padding: '6px 8px', fontSize: 12 }}
            >
              <Space size={6} wrap>
                <Tag color={meta.color} style={{ margin: 0 }} data-testid={`delegate-status-${d.status}`}>
                  {meta.text}
                </Tag>
                <Typography.Text strong>{d.member_name || d.member_agent_id.slice(0, 8)}</Typography.Text>
                <Typography.Text type="secondary" ellipsis style={{ maxWidth: 420 }}>
                  {d.task}
                </Typography.Text>
                {d.steer_note && (
                  <Tooltip title={d.steer_note}>
                    <Tag color="orange" style={{ margin: 0 }}>
                      已插话待续跑
                    </Tag>
                  </Tooltip>
                )}
              </Space>
              <Space size={4} style={{ marginTop: 4 }} wrap>
                {(d.status === 'completed' || d.status === 'failed' || d.steer_note) && (
                  <Button size="small" type="link" style={{ padding: 0 }} onClick={() => showResult(d.id)}>
                    {openResult === d.id ? '收起结果' : '查看结果'}
                  </Button>
                )}
                {d.status === 'running' && (
                  <Button size="small" type="link" style={{ padding: 0 }} icon={<SendOutlined />} onClick={() => setSteerFor(steerFor === d.id ? null : d.id)}>
                    插话引导
                  </Button>
                )}
                {d.status === 'running' && (
                  <Button size="small" type="link" style={{ padding: 0 }} icon={<PauseOutlined />} loading={busy === d.id} onClick={() => doAction(d.id, 'interrupt')}>
                    打断
                  </Button>
                )}
                {d.status !== 'running' && (
                  <Button size="small" type="link" style={{ padding: 0 }} icon={<CaretRightOutlined />} loading={busy === d.id} onClick={() => doAction(d.id, 'continue')}>
                    恢复/续跑
                  </Button>
                )}
                <Typography.Text type="secondary" style={{ fontSize: 11 }} copyable={{ text: d.child_conversation_id }}>
                  子会话 {d.child_conversation_id.slice(0, 8)}
                </Typography.Text>
              </Space>
              {steerFor === d.id && (
                <Space.Compact style={{ width: '100%', marginTop: 6 }}>
                  <Input
                    size="small"
                    placeholder="追加给子智能体的指令（本轮结束后自动续跑消费）"
                    value={steerText}
                    onChange={(e) => setSteerText(e.target.value)}
                    onPressEnter={() => doSteer(d.id)}
                    data-testid="delegate-steer-input"
                  />
                  <Button size="small" type="primary" loading={busy === d.id} onClick={() => doSteer(d.id)}>
                    发送
                  </Button>
                </Space.Compact>
              )}
              {openResult === d.id && (
                <Typography.Paragraph
                  style={{ fontSize: 12, whiteSpace: 'pre-wrap', background: 'var(--ant-color-fill-quaternary, #fafafa)', padding: 6, borderRadius: 4, marginTop: 6, marginBottom: 0 }}
                  data-testid="delegate-result"
                  ellipsis={{ rows: 12, expandable: true, symbol: '展开全文' }}
                >
                  {resultText}
                </Typography.Paragraph>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
