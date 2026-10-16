/**
 * 设置页提示信息统一形态（界面优化批 2026-10-16）：
 * 说明文字收进悬停图标（QuestionCircle + Tooltip），替代整段常显 secondary 文案——
 * 开发者反馈「提示信息直接显示太杂乱」。
 */
import { QuestionCircleOutlined } from '@ant-design/icons'
import { Tooltip } from 'antd'

export default function HintIcon({ text, style }: { text: string; style?: React.CSSProperties }) {
  return (
    <Tooltip title={<span style={{ fontSize: 12 }}>{text}</span>} placement="right" arrow>
      <QuestionCircleOutlined
        aria-label={text}
        data-hint={text}
        style={{ color: 'var(--c-ink-3, #9ca3af)', fontSize: 13, marginLeft: 6, cursor: 'help', ...style }}
      />
    </Tooltip>
  )
}
