import { useState } from 'react'
import { Button, Input, Modal } from 'antd'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// REQ-475/M275（Protégé 编辑能力对齐批④）：实体重命名（refactor rename）共用 Modal——
// 四类实体（concept/relation/attribute/instance）经 elements/{kind}/{name}/rename，
// 服务端全引用面 rewire（关系端点/实例类型/断言/限制公理/数据属性域与键/逆属性）。
// ---------------------------------------------------------------------------

export function RenameButton({
  ontologyId,
  kind,
  name,
  label,
  onSaved,
  size = 'small',
  danger,
}: {
  ontologyId: string
  kind: 'concept' | 'relation' | 'attribute' | 'instance'
  name: string
  /** 实体类型中文名（弹窗文案用，如「概念」） */
  label: string
  onSaved: () => void
  size?: 'small' | 'middle'
  danger?: boolean
}) {
  const { showToast } = useUI()
  const [open, setOpen] = useState(false)
  const [to, setTo] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    const v = to.trim()
    if (!v || v === name) {
      showToast('请输入不同的新名', 'err')
      return
    }
    setBusy(true)
    try {
      const r = await fetch(`/api/ontologies/${encodeURIComponent(ontologyId)}/elements/${kind}/${encodeURIComponent(name)}/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ to: v }),
      })
      const out = await r.json().catch(() => ({}))
      if (!r.ok) {
        showToast(out?.error ?? `重命名失败（HTTP ${r.status}）`, 'err')
        return
      }
      showToast(out?.detail ?? '已重命名（引用面已改写）')
      setOpen(false)
      setTo('')
      onSaved()
    } catch (e: any) {
      showToast(e?.message ?? '重命名失败', 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button size={size} type="link" style={{ padding: 0 }} danger={danger} onClick={() => setOpen(true)} data-testid={`rename-${kind}-${name}`}>
        重命名
      </Button>
      <Modal
        open={open}
        centered
        keyboard={false}
        title={`重命名${label}「${name}」`}
        okText="重命名"
        okButtonProps={{ danger: true, loading: busy }}
        cancelText="取消"
        onCancel={() => setOpen(false)}
        onOk={submit}
      >
        <Input
          value={to}
          onChange={(e) => setTo(e.target.value)}
          placeholder={`新名称（现为 ${name}）`}
          autoFocus
          onPressEnter={submit}
          data-testid="rename-input"
        />
      </Modal>
    </>
  )
}
