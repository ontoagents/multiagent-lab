import { Alert, Button, Card, Input, Space, Tag } from 'antd'
import { ThunderboltOutlined } from '@ant-design/icons'
import QualityReportCard, { qualityFindingsText } from '../QualityReportCard'
import type { QualityReport, Spec } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-337/M137：OntoChat 五阶段视图分离——draft/refine（草稿确认）阶段视图。
// 草稿预览 + 质量报告卡 + 入库行自 OntoChatFlow 迁出（结构与交互零变更；
// REQ-338 在此视图上做双栏与入库前增量确认分组勾选）。
// ---------------------------------------------------------------------------

export default function DraftConfirmPanel({
  draft,
  draftWarning,
  draftQuality,
  saveName,
  turning,
  onSaveName,
  onSave,
  onOptimize,
}: {
  draft: Spec
  draftWarning: string
  draftQuality: QualityReport | null
  saveName: string
  turning: boolean
  onSaveName: (v: string) => void
  onSave: () => void
  onOptimize: (findingsText: string) => void
}) {
  return (
    <Card size="small" style={{ marginTop: 10 }} title="草稿预览（确认后入库）">
      <Space size={6} wrap>
        <Tag color="blue" style={{ margin: 0 }}>概念 {draft.concepts?.length ?? 0}</Tag>
        <Tag color="geekblue" style={{ margin: 0 }}>关系 {draft.relations?.length ?? 0}</Tag>
        <Tag color="purple" style={{ margin: 0 }}>实例 {draft.instances?.length ?? 0}</Tag>
      </Space>
      {draftWarning && <Alert type="warning" showIcon style={{ marginTop: 8 }} title={draftWarning} />}
      {/* REQ-312/M116：草稿质量报告卡（与 AI 创建共用）；「按报告优化」=报告 findings 作为修正意见走 refine 管道 */}
      {draftQuality && (
        <div style={{ marginTop: 8 }}>
          <QualityReportCard
            report={draftQuality}
            optimizing={turning}
            onOptimize={() => {
              const ftext = qualityFindingsText(draftQuality)
              if (!ftext) return
              onOptimize(ftext)
            }}
          />
        </div>
      )}
      <Space style={{ marginTop: 10 }} size={8}>
        <Input value={saveName} onChange={(e) => onSaveName(e.target.value)} placeholder="本体名称" style={{ width: 240 }} />
        <Button type="primary" icon={<ThunderboltOutlined />} loading={turning} onClick={onSave}>
          创建并保存
        </Button>
      </Space>
    </Card>
  )
}
