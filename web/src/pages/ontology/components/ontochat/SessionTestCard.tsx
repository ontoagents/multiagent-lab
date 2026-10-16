import { Alert, Card, Table, Tag, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined, SafetyOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-342/OC-2：会话草稿覆盖测试报告卡（test 站）——OntoChat 会话内 draft 后即可
// 逐条判定能力问题覆盖（无需先入库），结果存会话 context.test_report 刷新可回看。
// 快筛段=口语化逐条独立判定（防泄漏）；「No」项指出草稿建模缺口，入库后可在资产
// 质量卡做 SPARQL 精判并轨（双轨完整报告 artifact）。诚实边界沿 REQ-274：口语化
// 判定对「可推断不显式」的需求有系统性乐观偏误（论文 87.5% 系 56 条小样本二分类）。
// ---------------------------------------------------------------------------

export interface SessionTestReport {
  verdicts: { cq: string; verdict: string; explanation?: string }[]
  passed: number
  total: number
  pass_rate: number
}

export default function SessionTestCard({ report }: { report: SessionTestReport }) {
  return (
    <Card
      size="small"
      style={{ marginTop: 10 }}
      title={
        <span>
          <SafetyOutlined style={{ color: 'var(--c-brand, #4f46e5)', marginInlineEnd: 6 }} />
          覆盖测试报告（test 站 · 口语化快筛）
        </span>
      }
      data-testid="session-test-card"
    >
      <Alert
        type={report.passed === report.total ? 'success' : 'info'}
        showIcon
        style={{ marginBottom: 8 }}
        title={`通过 ${report.passed}/${report.total}（快筛通过率 ${Math.round(report.pass_rate * 100)}%）`}
        description="判定「No」的条目=草稿尚缺建模——可据此回复修改意见进入修正轮；入库后可在资产质量卡「CQ 验收（SPARQL）」做双轨精判并轨。快筛对「可推断不显式」需求偏乐观（论文口径），精判以 SPARQL 为准。"
      />
      <Table
        size="small"
        rowKey={(r) => r.cq}
        dataSource={report.verdicts}
        pagination={{ pageSize: 5 }}
        columns={[
          { title: '能力问题', dataIndex: 'cq', ellipsis: true },
          {
            title: '判定',
            dataIndex: 'verdict',
            width: 90,
            render: (v: string) =>
              v === 'Yes' ? (
                <Tag color="green" icon={<CheckCircleOutlined />}>Yes</Tag>
              ) : v === 'No' ? (
                <Tag color="red" icon={<CloseCircleOutlined />}>No</Tag>
              ) : (
                <Tag>Unknown</Tag>
              ),
          },
          { title: '解释', dataIndex: 'explanation', ellipsis: true, render: (v: string) => <Typography.Text type="secondary" style={{ fontSize: 12 }}>{v}</Typography.Text> },
        ]}
      />
    </Card>
  )
}
