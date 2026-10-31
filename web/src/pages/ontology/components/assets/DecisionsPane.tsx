import { Empty, Tag, Typography } from 'antd'
// REQ-544/M351 决策只读分区：decisions[] spec 层 v1——「判断」作为可检索可审计资产的三要素呈现
// （判断问题/必答证据槽/拒绝路径）；无可执行语义不做解释器（105 号明确不做清单）；
// 编辑走 Spec JSON 高级态（spec REST 自动带出零 API 改动——本分区纯只读教学/审计视图）。
import type { Spec } from '../../../../api/types'

export default function DecisionsPane({ spec }: { spec: Spec | null }) {
  const decisions = spec?.decisions ?? []
  if (decisions.length === 0) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={
          <span style={{ fontSize: 12 }}>
            本体尚未登记决策（decisions[]）
            <br />
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              决策=「判断」的结构化资产：判断问题+必答证据槽+拒绝路径。可在 Spec JSON 高级态编辑（参考内置示例「本体工程域（dogfooding）」）。
            </Typography.Text>
          </span>
        }
      />
    )
  }
  return (
    <div data-testid="decisions-pane">
      <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
        决策（decisions[]，spec 层 v1——不入 TTL 导出）：把「什么时候做什么/不做什么」的判断结构化为可审计资产；无执行语义，不做解释器。
      </Typography.Paragraph>
      {decisions.map((d) => (
        <div key={d.id} style={{ border: '1px solid var(--c-line, #e3e6f0)', borderRadius: 8, padding: '10px 12px', marginBottom: 10 }}>
          <div style={{ marginBottom: 6 }}>
            <Typography.Text code style={{ fontSize: 12, marginInlineEnd: 8 }}>{d.id}</Typography.Text>
            <Typography.Text strong style={{ fontSize: 13 }}>{d.decision_question}</Typography.Text>
            {d.proof_scope && (
              <Typography.Text type="secondary" style={{ fontSize: 11, marginInlineStart: 8 }}>生效范围：{d.proof_scope}</Typography.Text>
            )}
          </div>
          {(d.required_slots?.length ?? 0) > 0 && (
            <div style={{ marginBottom: 4 }}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>必答证据槽：</Typography.Text>
              {d.required_slots!.map((s, i) => (
                <div key={i} style={{ fontSize: 12, padding: '1px 0 1px 10px' }}>
                  <Tag style={{ margin: 0, fontSize: 11 }}>{s.slot}</Tag>
                  <Typography.Text code style={{ fontSize: 11, marginInline: 4 }}>{s.source}</Typography.Text>
                  {s.missing_action && (
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>缺槽处置：{s.missing_action}</Typography.Text>
                  )}
                </div>
              ))}
            </div>
          )}
          {(d.reject_routes?.length ?? 0) > 0 && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>拒绝路径：</Typography.Text>
              {d.reject_routes!.map((r, i) => (
                <div key={i} style={{ fontSize: 12, padding: '1px 0 1px 10px' }}>
                  <Tag style={{ margin: 0, fontSize: 11 }} color="default">{r.route}</Tag>
                  <Typography.Text type="secondary" style={{ fontSize: 11, marginInlineStart: 4 }}>{r.reason}</Typography.Text>
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
