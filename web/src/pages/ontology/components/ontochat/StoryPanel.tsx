import { Button, Card, Space, Typography } from 'antd'
import { CommentOutlined, SolutionOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-337/M137：OntoChat 五阶段视图分离——story（用户故事访谈）阶段视图。
// REQ-275 访谈制品卡 + 模板 chips 自 OntoChatFlow 迁出（结构与交互零变更）。
// ---------------------------------------------------------------------------

export function StoryPanel({
  draftStory,
  turning,
  onBack,
  onFinish,
}: {
  draftStory: string
  turning: boolean
  onBack: () => void
  onFinish: () => void
}) {
  return (
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
        <Button size="small" onClick={onBack}>
          上一步（重答末问）
        </Button>
        <Button size="small" type="primary" icon={<SolutionOutlined />} loading={turning} onClick={onFinish} data-testid="story-finish-btn">
          完成并抽取 CQ
        </Button>
      </Space>
      <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 6, marginBottom: 0 }}>
        直接在下方输入框回复修改意见即可精修；完成后进入能力问题抽取（候选经确认卡写回，生成草稿回写 spec.CQ）。
      </Typography.Paragraph>
    </Card>
  )
}

/** 访谈阶段 P3 模板点击填入 chips（模板是辅助非必填）。 */
export function StoryTemplateChips({
  templates,
  onPick,
}: {
  templates: { label: string; text: string }[]
  onPick: (text: string) => void
}) {
  return (
    <div style={{ marginTop: 8, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
      {templates.map((t) => (
        <Button
          key={t.label}
          size="small"
          icon={<CommentOutlined />}
          onClick={() => onPick(t.text)}
          title={t.text.replace(/\*\*/g, '').replace(/\*/g, '')}
        >
          {t.label}
        </Button>
      ))}
    </div>
  )
}
