import { useState } from 'react'
import { Button, Input, Space, Typography } from 'antd'

// REQ-510/M318：建模备注编辑字段（元素级决策记录——随 spec_json 版本留痕+skos:note 导出）。
// 显示+编辑双态；保存上抛调用方（elementUpsert note 字段——同一 Spec.Validate 管线）。

export default function NoteEditField({
  initialValue,
  onSave,
  testid,
}: {
  initialValue: string
  onSave: (text: string) => void
  testid?: string
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(initialValue)
  return (
    <div style={{ marginBottom: 8 }}>
      {editing ? (
        <>
          <Input.TextArea
            rows={2}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="建模决策记录（为何这样建、备选方案、待办…）"
            data-testid={testid ? `${testid}-input` : undefined}
          />
          <Space size={6} style={{ marginTop: 4 }}>
            <Button size="small" type="primary" onClick={() => { onSave(text.trim()); setEditing(false) }} data-testid={testid ? `${testid}-save` : undefined}>
              保存备注
            </Button>
            <Button size="small" onClick={() => { setText(initialValue); setEditing(false) }}>
              取消
            </Button>
          </Space>
        </>
      ) : (
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
          <Typography.Text type={initialValue ? undefined : 'secondary'} style={{ fontSize: 12 }} data-testid={testid}>
            {initialValue || '未填写建模备注'}
          </Typography.Text>
          <Button size="small" type="text" onClick={() => setEditing(true)} data-testid={testid ? `${testid}-edit` : undefined}>
            {initialValue ? '编辑' : '填写'}
          </Button>
        </div>
      )}
    </div>
  )
}
