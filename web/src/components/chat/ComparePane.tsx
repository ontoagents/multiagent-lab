// REQ-401③/M197 收口：对比窗格 section 抽件（Splitter.Panel 包裹留主件 map 回调——零行为变更 prop 化）。
// 窗格头（状态/智能体/采纳/四徽标覆盖选择）+流区（Bubble.List）+配置区（Agent 选择/不携带历史/更多配置/剖面）；
// 状态与 setPaneSel/applyProfile 等回调留 ChatWindow 主件，本件纯渲染+回调上抛。
import { Bubble } from '@ant-design/x'
import type { BubbleListProps } from '@ant-design/x'
import { Button, Checkbox, Collapse, Input, InputNumber, Select, Space, Tooltip, Typography } from 'antd'
import { AgentLogo } from '../AgentLogo'
import { connDisplayName } from '../../api/client'
import { paneBadgeSelect, type PaneSel } from './bits'
import type { Agent, KnowledgeBase, ModelConnection } from '../../api/types'

export type PaneStatus = 'idle' | 'streaming' | 'error' | 'stopped' | 'done'

export default function ComparePane({
  index,
  sel,
  paneAgent,
  paneInherit,
  status,
  hasItems,
  conns,
  kbs,
  profiles,
  agentOptions,
  cmpProfiles,
  running,
  canAdopt,
  bubbleRoles,
  paneListItems,
  onAdopt,
  onPaneSel,
  onApplyProfile,
  onSaveProfile,
  onCopyPrev,
}: {
  index: number
  sel: PaneSel
  paneAgent: Agent | null
  paneInherit: { model: string; kb: string; profile: string; skills: string }
  status: PaneStatus
  hasItems: boolean
  conns: ModelConnection[]
  kbs: KnowledgeBase[]
  profiles: { id: string; name: string; status: string }[]
  agentOptions: { value: string; label: string }[]
  cmpProfiles: { name: string; cfg: Partial<PaneSel> }[]
  running: boolean
  canAdopt: boolean
  bubbleRoles: BubbleListProps['role']
  paneListItems: (i: number) => any[]
  onAdopt: (sel: PaneSel) => void
  onPaneSel: (i: number, field: keyof PaneSel, value: string | number | boolean | null) => void
  onApplyProfile: (i: number, cfg: Partial<PaneSel>) => void
  onSaveProfile: (i: number) => void
  onCopyPrev: (i: number) => void
}) {
  const statusLabel = { idle: '待提问', streaming: '生成中', error: '出错', stopped: '已停止', done: '完成' }[status]
  return (
    <section className="cmp-pane" aria-label={`对比窗格 ${index + 1}`}>
      <header className="cmp-pane-head">
        <span className="cmp-pane-title">窗格 {index + 1}</span>
        <span className={`cmp-pane-status st-${status}`} aria-label={`窗格 ${index + 1} 状态`}>
          <i className="dot" aria-hidden="true" />
          {statusLabel}
        </span>
        <span className="cmp-pane-agent" title={sel.agent ? '窗格级智能体（REQ-143）' : '继承对话智能体'}>
          <AgentLogo agent={paneAgent} size={16} context="runtime" />
          <Typography.Text strong style={{ fontSize: 12 }}>{paneAgent?.name ?? '—'}</Typography.Text>
        </span>
        {/* REQ-144 可选增强：采纳该窗格配置写回对话（仅 agent 直聊；运行中禁用） */}
        {canAdopt && (
          <Tooltip title="采纳：把该窗格的智能体/模型/知识库/方案/技能开关写回对话配置，之后单流对话沿用">
            <Button
              type="link" size="small" style={{ padding: '0 4px', fontSize: 11 }}
              onClick={() => onAdopt(sel)}
            >
              采纳
            </Button>
          </Tooltip>
        )}
        <span className="cmp-pane-badges">
          {paneBadgeSelect({
            label: '模型', value: sel.model,
            valueLabel: conns.find((c) => c.id === sel.model) ? connDisplayName(conns.find((c) => c.id === sel.model)!) : (sel.model || ''),
            inheritText: paneInherit.model,
            options: conns.map((c) => ({ value: c.id, label: connDisplayName(c) })),
            onChange: (v) => onPaneSel(index, 'model', v),
          })}
          {paneBadgeSelect({
            label: '库', value: sel.kb,
            valueLabel: kbs.find((k) => k.id === sel.kb)?.name ?? sel.kb,
            inheritText: paneInherit.kb,
            options: kbs.map((k) => ({ value: k.id, label: k.name })),
            onChange: (v) => onPaneSel(index, 'kb', v),
          })}
          {paneBadgeSelect({
            label: '方案', value: sel.profile,
            valueLabel: profiles.find((p) => p.id === sel.profile)?.name ?? sel.profile,
            inheritText: paneInherit.profile,
            options: profiles.filter((p) => p.status === 'running').map((p) => ({ value: p.id, label: p.name })),
            onChange: (v) => onPaneSel(index, 'profile', v),
          })}
          {paneBadgeSelect({
            label: '技能', value: sel.skills,
            valueLabel: sel.skills === 'on' ? '开' : '关',
            inheritText: paneInherit.skills,
            options: [{ value: 'on', label: '开启' }, { value: 'off', label: '关闭' }],
            onChange: (v) => onPaneSel(index, 'skills', v),
          })}
        </span>
      </header>
      <div className="cmp-pane-stream">
        {hasItems ? (
          <Bubble.List items={paneListItems(index)} role={bubbleRoles} />
        ) : (
          <div className="cmp-pane-empty">独立生成 · 等待提问</div>
        )}
      </div>
      {/* REQ-19f 窗格独立配置区：未设置项继承对话当前配置（Q-6 不追溯，仅影响该窗格后续消息） */}
      {/* REQ-150：模型/库/方案/技能 承载已迁窗格头徽标（继承/覆盖显性化）；此处留 Agent 选择与历史口径 */}
      <footer className="cmp-pane-cfg">
        <Select
          size="small" allowClear disabled={running} showSearch
          optionFilterProp="label"
          placeholder="智能体 · 继承" value={sel.agent || undefined}
          onChange={(v) => onPaneSel(index, 'agent', v ?? '')}
          options={agentOptions}
        />
        <Checkbox
          checked={sel.noHistory}
          disabled={running}
          onChange={(e) => onPaneSel(index, 'noHistory', e.target.checked)}
          style={{ fontSize: 11 }}
        >
          不携带历史（干净对照，REQ-143）
        </Checkbox>
        {/* REQ-144：更多配置（推理参数/提示词改写/技能开关）+ 剖面与窗格间复制 */}
        <Collapse
          ghost
          size="small"
          className="cmp-more-cfg"
          items={[{
            key: 'more',
            label: <span style={{ fontSize: 11, color: 'var(--ant-color-text-tertiary, #999)' }}>更多配置（温度 / 提示词改写 / 技能 · 剖面）</span>,
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ display: 'flex', gap: 6 }}>
                  <InputNumber
                    size="small" style={{ flex: 1 }} min={0} max={2} step={0.1} disabled={running}
                    placeholder="温度 · 继承" value={sel.temperature ?? undefined}
                    onChange={(v) => onPaneSel(index, 'temperature', typeof v === 'number' ? v : null)}
                  />
                  <Typography.Text type="secondary" style={{ fontSize: 11, alignSelf: 'center' }}>
                    技能承载已迁窗格头徽标（REQ-150）
                  </Typography.Text>
                </div>
                <Input.TextArea
                  size="small" rows={2} disabled={running} maxLength={2000}
                  placeholder="系统提示词临时改写 · 继承（仅本窗格生效）"
                  value={sel.instruction || undefined}
                  onChange={(e) => onPaneSel(index, 'instruction', e.target.value)}
                />
                <Space size={4} wrap>
                  <Select
                    size="small" style={{ minWidth: 130 }} allowClear disabled={running}
                    placeholder="应用剖面…" value={undefined}
                    onChange={(name) => { const pr = cmpProfiles.find((x) => x.name === name); if (pr) onApplyProfile(index, pr.cfg) }}
                    options={cmpProfiles.map((x) => ({ value: x.name, label: x.name }))}
                  />
                  <Button size="small" disabled={running} onClick={() => onSaveProfile(index)}>存为剖面</Button>
                  {index > 0 && (
                    <Button size="small" disabled={running} onClick={() => onCopyPrev(index)}>
                      复制上一窗格
                    </Button>
                  )}
                </Space>
              </div>
            ),
          }]}
        />
      </footer>
    </section>
  )
}
