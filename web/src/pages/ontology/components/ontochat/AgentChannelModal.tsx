// REQ-360/M161 B：页面渠道流程弹窗——「在智能体对话中构建」（B1）/「转到智能体对话继续」（B2）共用。
// 渠道列表兜底（B3）：已具能力智能体置顶 + 平台助手兜底 + 一键新建「本体工程师」预设智能体。
// 跳转=写 eino.chat.preset 载荷 + 切智能体模块；新会话由 AgentsPage 消费创建，启动消息**预填不代发**
// （最后一脚留给用户）；channel-first：构建会话由智能体 ontochat_start 创建，审计归因 source=agent_chat。
// 未开能力时确认弹窗翻开关（明示「并入四工具+注入纪律段」，不静默改配置）。
import { useEffect, useMemo, useState } from 'react'
import { Alert, Modal, Select, Space, Tag, Typography, Input } from 'antd'
import { RobotOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { Agent } from '../../../../api/types'
import { useUI } from '../../../../store/ui'

/** 「本体工程师」预设智能体（03 §2.54 C2：预置能力开关 on，纪律段由装配层内置注入——
 *  instruction 只写角色定位不复制纪律；演进位=内置本体专业能力，触发驱动另行立项） */
export const ONTO_ENGINEER_PRESET = {
  name: '本体工程师',
  description: '对话式本体构建专职智能体（OntoChat 访谈式建模：用户故事→CQ→草稿→质量门禁入库）',
  instruction:
    '你是「本体工程师」——本体构建专职智能体。通过对话与用户共创本体：先确认领域主题与构建意图，再用 ontochat_start 开启构建会话，按访谈纪律逐步推进（用户故事→能力问题→草稿→质量门禁入库）；产物入库后告知用户可到「本体-资产」继续精修。',
  onchat_mode: 'guided',
  tools: [] as string[],
  max_iteration: 25,
}

const ENGINEER_CREATE_KEY = '__create_engineer__'

export default function AgentChannelModal({
  open,
  mode,
  session,
  initialTopic,
  onClose,
}: {
  open: boolean
  mode: 'build' | 'handoff'
  /** handoff 模式的接续会话（携 session_id 预填接手指令，快照链续作零工具改动） */
  session?: { id: string; title?: string; round?: number } | null
  /** build 模式的领域主题（取当前输入非空时拼进启动消息） */
  initialTopic?: string
  onClose: () => void
}) {
  const { showToast, setPage } = useUI()
  const [agents, setAgents] = useState<Agent[]>([])
  const [agentId, setAgentId] = useState<string>()
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    let alive = true
    api.listAgents().then((l) => {
      if (!alive) return
      const list = Array.isArray(l) ? l : []
      setAgents(list)
      // 缺省选首个已具能力者；无则平台助手（迁移后默认已开能力）
      const capable = list.find((a) => a.onchat_mode)
      setAgentId(capable?.id ?? list.find((a) => a.is_builtin)?.id ?? list[0]?.id)
    }).catch((e) => showToast(e?.message ?? '智能体列表加载失败', 'err'))
    return () => { alive = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  // 预填启动消息（open 时按模式初始化，可编辑）
  useEffect(() => {
    if (!open) return
    if (mode === 'handoff' && session) {
      setMsg(`请接续我的本体构建会话「${session.title || session.id}」（已进行 ${session.round ?? 0} 轮，session_id=${session.id}）：用 ontochat_turn 查看当前进度并继续推进。`)
    } else {
      setMsg(initialTopic?.trim() ? `我想用对话方式构建一个本体。领域主题/描述：${initialTopic.trim()}` : '我想用对话方式构建一个本体，请开启构建会话。')
    }
  }, [open, mode, session, initialTopic])

  const capableFirst = useMemo(() => {
    const cap = agents.filter((a) => a.onchat_mode)
    const rest = agents.filter((a) => !a.onchat_mode)
    return { cap, rest }
  }, [agents])

  const selected = agents.find((a) => a.id === agentId) ?? null

  const ensureCapability = async (ag: Agent): Promise<Agent> => {
    if (ag.onchat_mode) return ag
    return new Promise<Agent>((resolve, reject) => {
      Modal.confirm({
        title: `为「${ag.name}」开启对话建本体能力？`,
        icon: <RobotOutlined />,
        content: '开启后运行时自动并入 ontochat_* 四工具（start/turn/save/export_ttl）并注入内置访谈纪律段；配置将保存（onchat_mode=guided），不静默改配置。',
        okText: '开启并继续',
        cancelText: '取消',
        onOk: async () => {
          try {
            const updated = await api.updateAgent(ag.id, { ...ag, onchat_mode: ag.onchat_mode || 'guided' })
            showToast(`已为「${ag.name}」开启对话建本体能力`)
            resolve(updated)
          } catch (e: any) {
            showToast(e?.message ?? '能力开启失败', 'err')
            reject(e)
          }
        },
        onCancel: () => reject(new Error('cancelled')),
      })
    })
  }

  const createEngineer = async (): Promise<Agent> => {
    try {
      const created = await api.createAgent(ONTO_ENGINEER_PRESET)
      showToast('已创建「本体工程师」预设智能体')
      return created
    } catch (e: any) {
      // 同名冲突：回落既有同名智能体（预设幂等语义）
      const found = agents.find((a) => a.name === ONTO_ENGINEER_PRESET.name)
      if (found) {
        showToast('已存在同名「本体工程师」，直接选用')
        return found
      }
      throw e
    }
  }

  const jump = (ag: Agent, message: string) => {
    localStorage.setItem('eino.chat.preset', JSON.stringify({ agentId: ag.id, msg: message }))
    setPage('agents')
    onClose()
  }

  const onOk = async () => {
    if (!msg.trim()) {
      showToast('启动消息不能为空', 'err')
      return
    }
    setBusy(true)
    try {
      let ag = selected
      if (agentId === ENGINEER_CREATE_KEY || (!ag && agentId === ENGINEER_CREATE_KEY)) {
        ag = await createEngineer()
      }
      if (!ag) {
        showToast('请选择智能体', 'err')
        return
      }
      ag = await ensureCapability(ag)
      jump(ag, msg.trim())
    } catch {
      /* 取消/已提示 */
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      title={mode === 'handoff' ? '转到智能体对话继续（REQ-360 页面渠道）' : '在智能体对话中构建（REQ-360 页面渠道）'}
      open={open}
      onCancel={onClose}
      onOk={onOk}
      okText={mode === 'handoff' ? '转到对话' : '去对话构建'}
      confirmLoading={busy}
      width={520}
      destroyOnHidden
    >
      <Space direction="vertical" size={8} style={{ display: 'flex', marginTop: 4 }}>
        <Alert
          type="info"
          showIcon
          style={{ padding: '6px 12px' }}
          message={
            mode === 'handoff'
              ? '跳转到智能体新会话并预填接手指令（快照链续作）；构建会话由智能体工具接续，审计归因该智能体。'
              : '跳转到智能体新会话并预填启动消息（预填不代发，由你按发送开始）；构建会话由智能体创建，审计归因 agent_chat。'
          }
        />
        <div>
          <Typography.Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>选择智能体</Typography.Text>
          <Select
            style={{ width: '100%' }}
            value={agentId}
            onChange={setAgentId}
            placeholder="选择智能体"
            showSearch
            optionFilterProp="label"
            data-testid="onchat-channel-agent"
            options={[
              ...(capableFirst.cap.length
                ? [{
                    label: '已具对话建本体能力',
                    title: 'cap',
                    desc: undefined as string | undefined,
                    options: capableFirst.cap.map((a) => ({
                      value: a.id,
                      label: `${a.name}${a.is_builtin ? '（平台助手）' : ''}`,
                      desc: 'on',
                    })),
                  }]
                : []),
              {
                label: '其他智能体（确认时开启能力）',
                title: 'rest',
                desc: undefined as string | undefined,
                options: [
                  ...capableFirst.rest.map((a) => ({
                    value: a.id,
                    label: `${a.name}${a.is_builtin ? '（平台助手）' : ''}`,
                    desc: 'off',
                  })),
                  { value: ENGINEER_CREATE_KEY, label: '✚ 新建「本体工程师」预设智能体', desc: 'new' },
                ],
              },
            ]}
            optionRender={(opt) => (
              <Space size={6}>
                <span>{opt.data?.label}</span>
                {opt.data?.desc === 'on' && <Tag color="green" style={{ margin: 0, fontSize: 10 }}>已具能力</Tag>}
              </Space>
            )}
          />
        </div>
        <div>
          <Typography.Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
            {mode === 'handoff' ? '接手指令（预填，可在对话中修改）' : '启动消息（预填不代发，可在对话中修改后发送）'}
          </Typography.Text>
          <Input.TextArea
            value={msg}
            onChange={(e) => setMsg(e.target.value)}
            autoSize={{ minRows: 3, maxRows: 6 }}
            data-testid="onchat-channel-msg"
          />
        </div>
      </Space>
    </Modal>
  )
}
