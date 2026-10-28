import { Typography } from 'antd'
import { Tooltip } from 'antd'

// ---------------------------------------------------------------------------
// REQ-485/M284（视觉降噪批②）：路径页开场说明统一形态——一句目的 + 「详情」折叠
//（原四页各挂 3~5 行大号 Alert 当说明书用，语义与视觉双重嘈杂；机制描述收进
//   Tooltip，REQ/M 编号一并移出主文案——「详情」悬停可见）。
// ---------------------------------------------------------------------------

export default function PathIntro({ purpose, detail }: { purpose: string; detail?: string }) {
  return (
    <Typography.Paragraph type="secondary" style={{ fontSize: 12.5, marginBottom: 10 }} data-testid="path-intro">
      {purpose}
      {detail && (
        <Tooltip title={<span style={{ fontSize: 12 }}>{detail}</span>} placement="right">
          <Typography.Link style={{ fontSize: 12, marginLeft: 6 }} data-testid="path-intro-detail">
            详情
          </Typography.Link>
        </Tooltip>
      )}
    </Typography.Paragraph>
  )
}
