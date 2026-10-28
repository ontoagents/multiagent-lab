// REQ-401/M197：对话窗口共用小件（自 ChatWindow.tsx 拆出——对比窗格覆盖徽标/配置 chip/
// Bubble 角色映射/提示词分段标签；行为零变更）。
import type { ReactNode } from 'react'
import { Popover, Select, Typography } from 'antd'
import XMarkdown from '@ant-design/x-markdown'
import type { BubbleListProps } from '@ant-design/x'

// REQ-19f/143 对比窗格单项覆盖（UI 本地形态：''= 继承对话当前配置；提交时映射 ComparePaneConfig）
export interface PaneSel {
  agent: string // ''= 继承对话所属/项目主智能体；否则窗格级独立装配（REQ-143）
  model: string
  kb: string
  profile: string
  noHistory: boolean // REQ-143③：不携带对话历史（干净对照）
  temperature: number | null // REQ-144：推理参数覆盖（null=继承）
  instruction: string // REQ-144：系统提示词临时改写（''=继承）
  skills: '' | 'on' | 'off' // REQ-144：技能开关（''=继承，REQ-19g 口径）
}
export const BLANK_PANE: PaneSel = { agent: '', model: '', kb: '', profile: '', noHistory: false, temperature: null, instruction: '', skills: '' }

// REQ-311 A2：提示词装配透镜——分段来源标签与配色（与后端 instructionSegments source 键对应）
export const SEG_LABELS: Record<string, string> = { base: '基座指令', skills: '技能目录', project_dir: '项目本地目录', ontology_guide: '本体方案指引' }
export const SEG_COLORS: Record<string, string> = { base: 'blue', skills: 'green', project_dir: 'gold', ontology_guide: 'purple' }

/** REQ-150 对比窗格覆盖徽标：覆盖态品牌填充显示值名，继承态浅色；点击经 Popover 选择（继承值显性化） */
export function paneBadgeSelect(opts: {
  label: string
  value: string
  valueLabel: string
  inheritText: string
  options: { value: string; label: string }[]
  onChange: (v: string) => void
}) {
  const { label, value, valueLabel, inheritText, options, onChange } = opts
  return (
    <Popover
      trigger="click"
      placement="bottomLeft"
      arrow={false}
      content={
        <div style={{ width: 230 }}>
          <Select
            size="small"
            style={{ width: '100%' }}
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder={`选择${label}（清空 = 继承）`}
            value={value || undefined}
            onChange={(v) => onChange(v ?? '')}
            options={options}
          />
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
            继承：{inheritText}
          </Typography.Text>
        </div>
      }
    >
      <button type="button" className={`cmp-badge clickable${value ? ' set' : ''}`} title={`${label}：${value ? valueLabel : `继承（${inheritText}）`}`}>
        {label}{value ? `·${valueLabel}` : '·继承'}
      </button>
    </Popover>
  )
}

/** 会话配置 chip（渲染在 Sender footer 内）：ON = 品牌填充，OFF = 浅色描边；禁用置灰 + Tooltip 说明 */
export function ChatChip({ on, disabled, icon, label, title, onClick }: {
  on: boolean
  disabled: boolean
  icon: ReactNode
  label: string
  title: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className={`chat-chip${on ? ' on' : ''}${disabled ? ' disabled' : ''}`}
      aria-pressed={on}
      aria-disabled={disabled || undefined}
      title={disabled ? undefined : title}
      onClick={onClick}
    >
      <span className="chat-chip-icon">{icon}</span>
      <span>{label}</span>
    </button>
  )
}

// Bubble 角色映射（X 2.x：role 单数；条目 role 必须命中此处定义的 key）
// REQ-483⑤：对话流去头像单列——智能体标识由窗口头部承担（REQ-165 运行上下文口径收窄注），过程行与正文全程左对齐同列
export const BUBBLE_ROLES: BubbleListProps['role'] = {
  user: {
    placement: 'end',
    styles: { content: { background: 'var(--c-brand)', color: '#fff', borderRadius: 12, borderBottomRightRadius: 4 } },
  },
  ai: {
    placement: 'start',
    // REQ-477④：助手回复去气泡框——正文直接落在对话底色（ZCode 会话流形态；用户气泡维持品牌填充）
    styles: { content: { background: 'transparent', border: 'none', boxShadow: 'none', padding: '2px 0 0' } },
    // 助手正文走 Markdown（XMarkdown）：流式期间尾部游标，hasNextChunk=false 时收尾刷新
    contentRender: (content, info) => (
      <XMarkdown
        className="chat-md"
        content={String(content ?? '')}
        openLinksInNewTab
        streaming={{ hasNextChunk: !!info.extraInfo?.streaming, tail: true }}
      />
    ),
  },
  event: {
    variant: 'borderless',
    styles: {
      root: { paddingBlock: 2 },
      content: { padding: 0, background: 'transparent', width: '100%', maxWidth: '100%' },
    },
  },
}