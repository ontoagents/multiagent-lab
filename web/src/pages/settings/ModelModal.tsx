/**
 * 模型表单（REQ-172 自 SettingsPage 抽取；REQ-177 协议重组 + 紧凑化）：
 * - 所属提供商（分组下拉，仅显示供应商名）；协议与 Base URL 为连接自己的属性（REQ-177①），
 *   新建时预填所选提供商锚点值、可改——同一供应商组内可混合 openai_compat 与 anthropic；
 * - 切换提供商时预填目标组锚点的协议/接入点（可再手改）；移动组 = copy_key_from 目标锚点；
 * - API Key 归属提供商（模型表单不出现 Key 输入）；提供商未变不发送 Key 字段（后端保留）；
 * - 协议联动（REQ-172）：anthropic 仅 chat（Anthropic 无官方向量接口）。
 */
import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Form, Input, Modal, Select, Space, Switch } from 'antd'
import { api } from '../../api/client'
import type { ModelConnection } from '../../api/types'
import { useUI } from '../../store/ui'
import { groupOf, isAnthropicProtocol, providerOfName, uniqueConnName, type ProviderGroup } from './grouping'
import { PROVIDER_PRESETS } from '../../api/providerPresets'
import HintIcon from './HintIcon'

const PROTOCOL_OPTIONS = [
  { value: 'openai_compat', label: 'openai_compat · OpenAI 兼容' },
  { value: 'anthropic', label: 'anthropic · Anthropic Messages' },
]

export function ModelModal({ conn, groups, conns, initialProvider, onClose, onSaved }: {
  conn: ModelConnection | 'new'
  groups: ProviderGroup[]
  conns: ModelConnection[]
  initialProvider?: string
  onClose: () => void
  onSaved: () => void
}) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [busy, setBusy] = useState(false)
  const editConn = conn === 'new' ? null : conn
  // 提供商切换时才预填目标组锚点协议/接入点；初始加载不覆盖连接自身值
  const loadedProvider = useRef<string | null>(null)

  useEffect(() => {
    if (editConn) {
      const key = groupOf(editConn)
      form.setFieldsValue({
        provider: key,
        protocol: editConn.protocol,
        base_url: editConn.base_url,
        model_name: editConn.model_name,
        conn_type: editConn.conn_type,
        is_default: editConn.is_default,
      })
      loadedProvider.current = key
    } else {
      const g0 = initialProvider ? groups.find((g) => g.key === initialProvider) : groups[0]
      form.setFieldsValue({
        provider: g0?.key,
        protocol: g0?.protocol ?? 'openai_compat',
        base_url: g0?.baseUrl ?? '',
        model_name: '',
        conn_type: 'chat',
        is_default: false,
      })
      loadedProvider.current = g0?.key ?? null
    }
  }, [conn, form, groups, initialProvider])

  const providerKey = Form.useWatch('provider', form)
  const providerGroup = groups.find((g) => g.key === providerKey)
  const protocol = Form.useWatch('protocol', form)
  const anthropic = isAnthropicProtocol(protocol)

  // 切换提供商（界面优化批）：保留当前协议，接入地址按「新供应商+当前协议」联动同步
  //（新商无该协议连接则顺带切到其锚点协议并取锚点地址——保证协议/地址永远配对）
  useEffect(() => {
    if (!providerKey || providerKey === loadedProvider.current) return
    const g = groups.find((x) => x.key === providerKey)
    if (g) {
      const cur = form.getFieldValue('protocol') ?? 'openai_compat'
      const sibling = g.members.find((m) => m.protocol === cur && m.base_url)
      if (sibling) {
        form.setFieldsValue({ base_url: sibling.base_url })
      } else if (!g.members.some((m) => m.protocol === cur)) {
        const anchor = g.members.find((m) => m.protocol === g.protocol) ?? g.anchor
        form.setFieldsValue({ protocol: anchor.protocol, base_url: anchor.base_url })
      } else {
        syncAddress(g.name, cur)
      }
    }
    loadedProvider.current = providerKey
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerKey, groups, form])

  // 切到 anthropic 时类型锁定 chat；切回 openai_compat 不代选（保持用户已选值）
  useEffect(() => {
    if (anthropic && form.getFieldValue('conn_type') === 'embedding') {
      form.setFieldsValue({ conn_type: 'chat' })
    }
  }, [anthropic, form])

  // 界面优化批：供应商/协议任一切换 → 接入地址联动同步（统一 syncAddress）——
  // 同商组内已有该协议连接取其地址（最准），否则厂商预设（按厂商名模糊匹配 + 协议一致）；
  // 初始加载不触发（换供应商/换协议均为用户显式切换）。
  const syncAddress = (vendorName: string, p: string) => {
    const g = groups.find((x) => x.name === vendorName)
    const sibling = g?.members.find((m) => m.protocol === p && m.base_url)
    if (sibling) {
      form.setFieldsValue({ base_url: sibling.base_url })
      return
    }
    const head = vendorName.split(' ')[0]
    const preset = PROVIDER_PRESETS.find(
      (pr) => (pr.protocol ?? 'openai_compat') === p &&
        ((head && pr.name.includes(head)) || (head && head.length >= 2 && pr.name.toUpperCase().includes(head.toUpperCase())) || vendorName.includes(pr.key)),
    )
    if (preset) form.setFieldsValue({ base_url: preset.baseUrl })
  }
  useEffect(() => {
    if (protocol == null || !providerGroup) return
    syncAddress(providerGroup.name, protocol)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [protocol])

  const save = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setBusy(true)
    try {
      if (editConn) {
        const target = providerGroup && providerGroup.key !== groupOf(editConn) ? providerGroup : null
        // 名称按 `{提供商}·{模型}` 维护：移动或改模型名时重生成（排除自身，重名追加 (n)）
        const takenOthers = new Set(conns.filter((c) => c.id !== editConn.id).map((c) => c.name))
        const name = target
          ? uniqueConnName(target.name, v.model_name, takenOthers)
          : v.model_name !== editConn.model_name
            ? uniqueConnName(providerGroup?.name ?? providerOfName(editConn.name), v.model_name, takenOthers)
            : editConn.name
        await api.updateConnection(editConn.id, {
          ...editConn,
          // REQ-177①：移动组只改归属与 Key 来源；协议/Base URL 为连接属性随表单值
          ...(target ? { provider_group_id: target.id, copy_key_from: target.anchor.id } : {}),
          name,
          protocol: v.protocol,
          base_url: v.base_url,
          model_name: v.model_name,
          conn_type: anthropic ? 'chat' : v.conn_type,
          is_default: v.is_default,
        })
        if (v.is_default) await api.setDefaultConnection(editConn.id)
      } else {
        if (!providerGroup) throw new Error('请先选择提供商；新提供商请先添加')
        const created = await api.createConnection({
          name: uniqueConnName(providerGroup.name, v.model_name, new Set(conns.map((c) => c.name))),
          protocol: v.protocol,
          base_url: v.base_url,
          model_name: v.model_name,
          conn_type: anthropic ? 'chat' : v.conn_type, // anthropic 无向量接口，强制 chat
          provider_group_id: providerGroup.id, // REQ-148：归属所选实例分组
          copy_key_from: providerGroup.anchor.id, // Key 归属提供商：与组锚点共享同一份密文
          enabled: true,
          is_default: false,
        })
        if (v.is_default) await api.setDefaultConnection(created.id)
      }
      showToast('已保存')
      onSaved()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      centered
      title={editConn ? `编辑模型 · ${editConn.model_name}` : '添加模型'}
      onCancel={onClose}
      width={620}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={busy} onClick={save}>保存</Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false} disabled={busy}>
        {!editConn && groups.length === 0 && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            title="尚无提供商"
            description="请先关闭本弹窗，用「＋ 添加提供商」从厂商预设快速填充访问配置（仅需补 API Key）。"
          />
        )}
        <Form.Item
          name="provider"
          label={<span>所属提供商 <HintIcon text="API Key 由提供商统一管理（移动提供商时自动沿用目标提供商的 Key）。切协议/切供应商时接入地址自动联动。" /></span>}
          rules={[{ required: true, message: '请选择提供商；新提供商请先添加' }]}
         
          style={{ marginBottom: 12 }}
        >
          <Select
            placeholder="选择提供商"
            options={groups.map((g) => ({ value: g.key, label: g.name }))}
          />
        </Form.Item>
        <div style={{ display: 'flex', gap: 12 }}>
          <Form.Item name="protocol" label={<span>协议 <HintIcon text="同一供应商下可与其他连接不同协议；切换协议自动联动匹配的接入地址。" /></span>} style={{ width: 250, marginBottom: 12 }}>
            <Select options={PROTOCOL_OPTIONS} />
          </Form.Item>
          <Form.Item name="conn_type" label={<span>类型 {anthropic && <HintIcon text="Anthropic 无向量接口，仅支持对话模型" />}</span>} style={{ width: 170, marginBottom: 12 }}>
            <Select
              disabled={anthropic}
              options={[
                { value: 'chat', label: 'chat（对话）' },
                { value: 'embedding', label: 'embedding（向量）', disabled: anthropic },
              ]}
            />
          </Form.Item>
        </div>
        <Form.Item
          name="base_url"
          label={<span>Base URL <HintIcon text={anthropic ? '填 Anthropic 网关根地址，平台自动拼接 /v1/messages' : 'OpenAI 兼容接入点；切协议/切供应商时自动联动匹配地址'} /></span>}
          rules={[{ required: true, message: 'Base URL 必填' }]}
         
          style={{ marginBottom: 12 }}
        >
          <Input placeholder={anthropic ? 'https://api.anthropic.com' : 'https://api.deepseek.com/v1'} />
        </Form.Item>
        <div style={{ display: 'flex', gap: 12 }}>
          <Form.Item name="model_name" label="模型名" rules={[{ required: true, message: '模型名必填' }]} style={{ flex: 1, marginBottom: 12 }}>
            <Input placeholder={anthropic ? 'claude-sonnet-4-5' : 'deepseek-chat'} />
          </Form.Item>
          <Form.Item name="is_default" label={<span>类型默认 <HintIcon text="chat / embedding 各至多一条默认连接，供智能体「跟随全局默认」引用" /></span>} valuePropName="checked" style={{ width: 190, marginBottom: 12 }}>
            <Switch checkedChildren="默认" unCheckedChildren="否" />
          </Form.Item>
        </div>
      </Form>
    </Modal>
  )
}
