import { useEffect, useState } from 'react'
import { Alert, Button, Input, Modal, Select, Space, Tag, Typography, message } from 'antd'
import { api } from '../api/client'
import { PROVIDER_PRESETS } from '../api/providerPresets'

// REQ-408/M204：首启向导与零门槛跑通——无模型连接时全局引导（选预设→填 Key/本地端点→
// 测试连通→一键建示例 Agent→跑通第一轮，串联 EmptyGuide 语义）；本地模型预设
// （Ollama/LM Studio/vLLM）零 API key 跑通；示例智能体一键灌装。
// 触发： agents/model-connections 装载后判定——无任何 chat 连接时 Alert 常驻 + 向导 Modal。

const ONBOARDING_DONE_KEY = 'eino.onboarding.done'

export default function OnboardingGuide({ onChanged }: { onChanged?: () => void }) {
  const [hasConn, setHasConn] = useState<boolean | null>(null) // null=装载中
  const [open, setOpen] = useState(false)
  const [presetKey, setPresetKey] = useState<string>('ollama')
  const [modelName, setModelName] = useState('qwen2.5:7b')
  const [apiKey, setApiKey] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (localStorage.getItem(ONBOARDING_DONE_KEY) === '1') {
      setHasConn(true)
      return
    }
    api
      .listConnections()
      .then((cs) => {
        const has = (cs as any[]).some((c) => c.conn_type === 'chat')
        setHasConn(has)
        if (!has) setOpen(true)
      })
      .catch(() => setHasConn(null))
  }, [])

  if (hasConn !== false) return null

  const preset = PROVIDER_PRESETS.find((p) => p.key === presetKey)
  const isLocal = ['ollama', 'lmstudio', 'vllm'].includes(presetKey)

  const createConn = async () => {
    if (!preset) return
    setBusy(true)
    try {
      await api.createConnection({
        name: `${preset.name}·${modelName}`,
        conn_type: 'chat',
        protocol: preset.protocol ?? 'openai_compat',
        base_url: preset.baseUrl,
        model_name: modelName,
        api_key: apiKey || undefined,
        is_default: true,
        enabled: true,
      } as any)
      message.success('模型连接已创建并设为默认——可以开始对话了')
      localStorage.setItem(ONBOARDING_DONE_KEY, '1')
      setHasConn(true)
      setOpen(false)
      onChanged?.()
    } catch (e: any) {
      message.error(e?.message ?? '连接创建失败')
    } finally {
      setBusy(false)
    }
  }

  const seedAgent = async () => {
    setBusy(true)
    try {
      await api.createAgent({
        name: '平台助手示例',
        description: 'REQ-408 首启灌装的示例智能体——通用问答与工具试跑',
        instruction: '你是平台示例助手。用中文简洁回答；需要时调用可用工具辅助。',
      })
      message.success('示例智能体已创建——选中它开始第一轮对话')
    } catch (e: any) {
      message.error(e?.message ?? '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{ margin: '12px 16px 0' }}
        data-testid="onboarding-alert"
        message="尚未配置模型连接——3 步跑通第一轮对话"
        description="本地模型（Ollama/LM Studio/vLLM）零 API key 可用；云端供应商填 Key 即可。"
        action={
          <Button size="small" type="primary" onClick={() => setOpen(true)}>
            打开向导
          </Button>
        }
      />
      <Modal
        open={open}
        title="快速上手向导（REQ-408）"
        okText="创建连接"
        okButtonProps={{ loading: busy }}
        onCancel={() => setOpen(false)}
        onOk={createConn}
        width={560}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={10}>
          <Typography.Text type="secondary">① 选预设（本地模型零 API key；云端填 Key）</Typography.Text>
          <Select
            style={{ width: '100%' }}
            value={presetKey}
            onChange={(k) => {
              setPresetKey(k)
              const p = PROVIDER_PRESETS.find((x) => x.key === k)
              if (p) setModelName(p.defaultModel)
            }}
            options={[
              { label: '—— 本地（零 API key）——', disabled: true },
              ...PROVIDER_PRESETS.filter((p) => ['ollama', 'lmstudio', 'vllm'].includes(p.key)).map((p) => ({
                value: p.key,
                label: `${p.name} · ${p.baseUrl}`,
              })),
              { label: '—— 云端（需 API Key）——', disabled: true },
              ...PROVIDER_PRESETS.filter((p) => !['ollama', 'lmstudio', 'vllm'].includes(p.key)).map((p) => ({
                value: p.key,
                label: p.name,
              })),
            ]}
          />
          <Typography.Text type="secondary">② 模型名 + API Key（本地留空）</Typography.Text>
          <Space.Compact style={{ width: '100%' }}>
            <Select
              style={{ width: 260 }}
              value={modelName}
              showSearch
              onChange={setModelName}
              options={[preset?.defaultModel, ...(isLocal ? ['llama3.1:8b', 'qwen2.5:14b'] : [])]
                .filter(Boolean)
                .map((m) => ({ value: m as string, label: m as string }))}
            />
            {!isLocal && (
              <Input.Password
                style={{ flex: 1 }}
                placeholder="API Key"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
              />
            )}
          </Space.Compact>
          {isLocal && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              前置：本机已运行 <Tag style={{ margin: 0 }}>ollama serve</Tag> 等本地推理服务（OpenAI 兼容端点）
            </Typography.Text>
          )}
          <Typography.Text type="secondary">③ 一键建示例智能体（可选）</Typography.Text>
          <Button onClick={seedAgent} loading={busy}>
            创建示例智能体（平台助手示例）
          </Button>
        </Space>
      </Modal>
    </>
  )
}
