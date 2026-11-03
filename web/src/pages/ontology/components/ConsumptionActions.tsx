import { useEffect, useState } from 'react'
import { Alert, Button, Select, Space, Typography } from 'antd'
import { api } from '../../../api/client'
import type { Agent, RuntimeProfile } from '../../../api/types'
import { useUI } from '../../../store/ui'
import { deepLinkOnto } from '../shared/ontoRoute'
import { useOntologies } from '../shared/useOntoList'

/** REQ-606-bdf6：只建立消费上下文，不自动启动方案或发送消息。 */
export default function ConsumptionActions({ profile }: { profile: RuntimeProfile }) {
  const { ontos, err: ontosError, reload: reloadOntos } = useOntologies()
  const [agents, setAgents] = useState<Agent[]>([])
  const [agentId, setAgentId] = useState<string | null>(null)
  const [ontologyId, setOntologyId] = useState<string | null>(() => profile.ontology_ids?.length === 1 ? profile.ontology_ids[0] : null)
  const [error, setError] = useState<string | null>(null)
  const [agentsError, setAgentsError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const loadAgents = () => api.listAgents().then((as) => { setAgents(as); setAgentsError(null) }).catch((e) => setAgentsError(e.message))
  useEffect(() => { void loadAgents() }, [])
  const createChat = async () => {
    if (!agentId || profile.status !== 'running') return
    setBusy(true)
    setError(null)
    try {
      // 重新确认服务端状态，避免轮询间隙用已停止方案建立挂载。
      const latest = (await api.listRuntimeProfiles()).find((p) => p.id === profile.id)
      if (latest?.status !== 'running') throw new Error('该方案已停止或不可用，请返回运行页检查状态。')
      const conv = await api.createConversation({ scope: 'agent', agent_id: agentId, title: `本体查询 · ${profile.name}`, runtime_profile_id: profile.id, ontology_enabled: true })
      const ui = useUI.getState()
      // 此次明确会话跳转替代尚未消费的旧智能体定位。
      localStorage.removeItem('eino.agents.focus')
      localStorage.removeItem('eino.chat.preset')
      sessionStorage.setItem('eino.agent.active', agentId)
      sessionStorage.setItem('eino.focus.conv', conv.id)
      sessionStorage.removeItem('eino.focus.conv.pending')
      ui.setCurrentConv(conv.id)
      ui.bumpData()
      ui.setPage('agents')
    } catch (e: any) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Space direction="vertical" style={{ display: 'flex', marginBottom: 16 }}>
      <Typography.Text strong>使用此方案</Typography.Text>
      <Space wrap>
        <Select aria-label="消费智能体" placeholder="选择智能体" style={{ minWidth: 180 }} value={agentId} onChange={setAgentId} options={agents.map((a) => ({ value: a.id, label: a.name }))} />
        <Button type="primary" size="small" disabled={profile.status !== 'running' || !agents.some((a) => a.id === agentId) || !!agentsError} loading={busy} onClick={createChat}>新建挂载对话</Button>
        <Select aria-label="出题本体" placeholder="选择出题本体" style={{ minWidth: 180 }} value={ontologyId} onChange={setOntologyId} options={ontos.filter((o) => profile.ontology_ids?.includes(o.id)).map((o) => ({ value: o.id, label: o.name }))} />
        <Button size="small" disabled={!!ontosError || !ontos.some((o) => o.id === ontologyId && profile.ontology_ids?.includes(o.id))} onClick={() => deepLinkOnto({ col: 'learn', learn: 'quiz', ontologyId: ontologyId! })}>出题自测</Button>
      </Space>
      <Typography.Text type="secondary">对话绑定此运行方案；出题基于所选本体的当前资产内容。</Typography.Text>
      {profile.status !== 'running' && <Typography.Text type="secondary">启动方案后可新建挂载对话。</Typography.Text>}
      {agentsError && <Alert type="warning" title="智能体清单加载失败" description={agentsError} action={<Button size="small" onClick={loadAgents}>重试</Button>} />}
      {ontosError && <Alert type="warning" title="本体清单加载失败" description={ontosError} action={<Button size="small" onClick={reloadOntos}>重试</Button>} />}
      {error && <Alert type="error" title="新建挂载对话失败" description={error} />}
    </Space>
  )
}
