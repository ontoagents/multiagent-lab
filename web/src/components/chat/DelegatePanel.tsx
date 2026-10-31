import { useEffect, useState } from 'react'
import { Button, Drawer, Empty, Input, Modal, Popconfirm, Space, Spin, Tag, Tooltip, Typography, message } from 'antd'
import { InboxOutlined, MessageOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons'
import { api } from '../../api/client'
import type { ConversationDelegate } from '../../api/types'

// REQ-403②/M199 余项（随 REQ-401 拆分轮归位）：后台委派入口层级提升——会话头部一级入口
// 打开委派面板（此前委派仅模型经 delegate_* 工具可见，用户无管理面）。列表=本会话发起的
// 后台委派（conversation_delegate）；动作=steer 插话引导（轮界接续）/interrupt 打断/
// collect 收结果摘要（全文在子会话）。API 全部既有（REQ-208），零新后端。

// REQ-488：状态彩 Tag 灰字化——仅 failed 留色（REQ-483 视觉语言），其余中性灰字
const STATUS_META: Record<ConversationDelegate['status'], { color?: string; label: string }> = {
  running: { label: '运行中' },
  completed: { label: '已完成' },
  failed: { color: 'error', label: '失败' },
  stopped: { label: '已停止' },
}

export default function DelegatePanel({ conversationId, open, onClose }: {
  conversationId: string
  open: boolean
  onClose: () => void
}) {
  const [items, setItems] = useState<ConversationDelegate[] | null>(null)
  const [steer, setSteer] = useState<{ d: ConversationDelegate; note: string } | null>(null)
  const [result, setResult] = useState<{ d: ConversationDelegate; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const load = () => {
    api.listDelegates(conversationId)
      .then((ls) => setItems(ls ?? []))
      .catch((e) => { setItems([]); message.error(e?.message ?? '加载委派列表失败') })
  }
  useEffect(() => { if (open) load() }, [open, conversationId])

  const doSteer = async () => {
    if (!steer) return
    setBusy(true)
    try {
      await api.steerDelegate(steer.d.id, steer.note)
      message.success('插话已提交——子智能体当前轮结束后自动续跑消费')
      setSteer(null)
      load()
    } catch (e: any) {
      message.error(e?.message ?? '插话失败')
    } finally {
      setBusy(false)
    }
  }

  const doInterrupt = async (d: ConversationDelegate) => {
    setBusy(true)
    try {
      await api.interruptDelegate(d.id)
      message.success('已打断（子会话历史完整可恢复）')
      load()
    } catch (e: any) {
      message.error(e?.message ?? '打断失败')
    } finally {
      setBusy(false)
    }
  }

  const doCollect = async (d: ConversationDelegate) => {
    setBusy(true)
    try {
      const r = await api.getDelegate(d.id)
      setResult({ d, text: r.result || r.delegate.result_digest || '（尚无结果）' })
    } catch (e: any) {
      message.error(e?.message ?? '收取失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title={
        <Space>
          <span>后台委派</span>
          <Button size="small" type="text" icon={<ReloadOutlined />} onClick={load}>刷新</Button>
        </Space>
      }
      width={520}
      data-testid="delegate-panel"
    >
      {items === null ? (
        <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
      ) : items.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本会话暂无后台委派——协调者经 delegate_background 工具发起后在此可见" />
      ) : (
        <Space direction="vertical" style={{ width: '100%' }} size={10}>
          {items.map((d) => {
            const meta = STATUS_META[d.status] ?? STATUS_META.running
            return (
              // REQ-488：委派卡去框改分隔线；状态灰字（仅失败留色）；操作钮灰图标 hover；字号统一 13/12 两档
              <div key={d.id} data-testid="delegate-item" style={{ padding: '10px 2px', borderBottom: '1px solid var(--c-line, #e6e8f0)' }}>
                <Space size={6} wrap style={{ marginBottom: 4 }}>
                  {meta.color ? (
                    <Tag color={meta.color} style={{ margin: 0 }}>{meta.label}</Tag>
                  ) : (
                    <Typography.Text type="secondary" style={{ margin: 0, fontSize: 12 }}>{meta.label}</Typography.Text>
                  )}
                  <Typography.Text strong style={{ fontSize: 13 }}>{d.member_name || d.member_agent_id}</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{new Date(d.created_at).toLocaleString()}</Typography.Text>
                </Space>
                <Typography.Paragraph style={{ margin: 0, fontSize: 13 }} ellipsis={{ rows: 2, expandable: true, symbol: '展开' }}>
                  {d.task}
                </Typography.Paragraph>
                {d.steer_note && (
                  <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>插话：{d.steer_note}</Typography.Text>
                )}
                {d.status === 'failed' && d.error && (
                  <Typography.Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>{d.error}</Typography.Text>
                )}
                <Space size={2} style={{ marginTop: 6 }}>
                  {d.status === 'running' && (
                    <>
                      <Tooltip title="插话引导（轮界接续）">
                        <Button size="small" type="text" aria-label="插话引导" icon={<MessageOutlined />} disabled={busy} onClick={() => setSteer({ d, note: '' })} />
                      </Tooltip>
                      <Popconfirm title="打断该后台委派？" description="子会话历史完整保留，可冷恢复续跑" onConfirm={() => doInterrupt(d)}>
                        <Tooltip title="打断">
                          <Button size="small" type="text" aria-label="打断" danger icon={<StopOutlined />} disabled={busy} />
                        </Tooltip>
                      </Popconfirm>
                    </>
                  )}
                  {(d.status === 'completed' || d.status === 'failed') && (
                    <Tooltip title="收结果">
                      <Button size="small" type="text" aria-label="收结果" icon={<InboxOutlined />} disabled={busy} onClick={() => doCollect(d)} />
                    </Tooltip>
                  )}
                  {d.child_conversation_id && (
                    <Tooltip title={`子会话 ${d.child_conversation_id.slice(0, 8)}…（历史持久化，可冷恢复）`}>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>子会话在档</Typography.Text>
                    </Tooltip>
                  )}
                </Space>
              </div>
            )
          })}
        </Space>
      )}
      <Modal
        open={!!steer}
        title={`插话引导 · ${steer?.d.member_name ?? ''}`}
        onCancel={() => setSteer(null)}
        onOk={doSteer}
        okText="提交插话"
        okButtonProps={{ loading: busy }}
      >
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 8 }}>
          插话落入子会话并标记；子智能体当前轮结束后自动续跑一轮消费插话（轮界接续，非 token 级中断注入）。
        </Typography.Text>
        <Input.TextArea
          rows={3}
          value={steer?.note ?? ''}
          onChange={(e) => setSteer((s) => (s ? { ...s, note: e.target.value } : s))}
          placeholder="例如：优先覆盖边界情况，测试用例补齐中文输入"
        />
      </Modal>
      <Modal
        open={!!result}
        title={`委派结果 · ${result?.d.member_name ?? ''}`}
        onCancel={() => setResult(null)}
        footer={<Button type="primary" onClick={() => setResult(null)}>关闭</Button>}
        width={560}
      >
        <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12.5, maxHeight: 420, overflow: 'auto' }}>{result?.text}</pre>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>摘要 ≤2000 字符；全文在子会话（历史持久化）。</Typography.Text>
      </Modal>
    </Drawer>
  )
}
