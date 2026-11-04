import { Typography } from 'antd'

/**
 * 「关系与后果」对照块（REQ-615-721e/M423-721e，121 号 §4.2-C6 / §4.4 文案单源）：
 * 两处删除确认（工作区侧板删除 Modal / 项目侧板删除 Popconfirm）顶部统一渲染同一块，
 * 把两种删除语义并置对照——删工作区＝容器脱钩（唯记忆 CASCADE）；删项目＝内容级联硬删。
 * kind 仅控制当前操作对象那一行加粗强调（对照完整透出，防删错对象）；底层 FK/级联策略零改动
 * （SET NULL 软脱钩 vs CASCADE 硬删维持现状，121 号 §5.1）。
 */
export default function RelationConsequenceBlock({ kind }: { kind: 'workspace' | 'project' }) {
  const strongStyle = { fontWeight: 600 } as const
  return (
    <div
      data-testid={`rel-consequence-${kind}`}
      style={{
        border: '1px solid var(--c-line, #e6e8f0)',
        borderLeft: '3px solid var(--c-brand, #4f46e5)',
        borderRadius: 6,
        padding: '8px 10px',
        marginBottom: 10,
        background: 'var(--c-panel, #fff)',
      }}
    >
      <Typography.Text strong style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
        关系与后果对照
      </Typography.Text>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', lineHeight: 1.7 }}>
        <span style={kind === 'workspace' ? strongStyle : undefined}>删工作区＝容器脱钩</span>
        ：归属项目解除归属保留、直挂对话回落智能体页（消息资产保留）；仅该工作区的跨会话记忆（含候选）随容器删除（CASCADE）。
      </Typography.Text>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', lineHeight: 1.7 }}>
        <span style={kind === 'project' ? strongStyle : undefined}>删项目＝内容级联硬删</span>
        ：项目及其全部对话、消息一并删除，不可恢复；归属工作区与工作区记忆不受影响（项目自动解除归属）。
      </Typography.Text>
    </div>
  )
}
