// REQ-401③/M197 收口：会话设置面板（头部 ⋮ Popover content 抽件——零行为变更 prop 化）。
// 重命名/审批覆盖/模型覆盖/生效配置总览/上下文预算/检索参数/伴生覆盖/侧板入口；
// 状态（renameVal）与 patchConv/toast 语义留 ChatWindow 主件，本件纯渲染+回调上抛。
import { Button, Input, InputNumber, Segmented, Select, Typography } from 'antd'
import type { Agent, Conversation } from '../../api/types'

export default function ConvSettingsPanel({
  conversation,
  patchConv,
  renameVal,
  setRenameVal,
  modelConns,
  agentApprovalLabel,
  agent,
  companionOntos,
  isProjectScope,
  project,
  subjectName,
  onToggleSidePanel,
  onClose,
  onCompanionChange,
}: {
  conversation: Conversation
  patchConv: (patch: Partial<Conversation>) => void
  renameVal: string
  setRenameVal: (v: string) => void
  modelConns: { id: string; name: string }[]
  agentApprovalLabel: string
  agent: Agent | null
  companionOntos: { id: string; name: string }[]
  isProjectScope: boolean
  project: { name: string; workspace_id?: string | null } | null
  subjectName?: string
  onToggleSidePanel?: () => void
  onClose: () => void
  onCompanionChange: (id: string) => void
}) {
  return (
    <div className="conv-settings" data-testid="conv-settings">
      <div className="proc-pop-title">会话设置</div>
      <div className="cs-row">
        <Input
          size="small"
          value={renameVal}
          onChange={(e) => setRenameVal(e.target.value)}
          placeholder="会话名称"
          onPressEnter={() => { if (renameVal && renameVal !== conversation.title) patchConv({ title: renameVal }) }}
        />
        <Button size="small" data-testid="conv-rename-save" onClick={() => { if (renameVal && renameVal !== conversation.title) patchConv({ title: renameVal }) }}>保存</Button>
      </div>
      <div className="chip-picker-divider" />
      <div className="proc-pop-title">工具调用审批（会话级覆盖）</div>
      <Segmented
        size="small"
        value={conversation.tool_approval ?? ''}
        onChange={(v) => patchConv({ tool_approval: v as string })}
        options={[
          { value: '', label: '跟随' },
          { value: 'on', label: '开启' },
          { value: 'off', label: '关闭' },
        ]}
      />
      {/* REQ-403/M199：会话生效配置总览（合并序一屏可见）+ 会话级模型覆盖入口恢复（REQ-291 数据层在、REQ-336 退役 UI 入口） */}
      <div className="proc-pop-title">会话级模型覆盖（REQ-291）</div>
      <Select
        size="small"
        allowClear
        value={conversation.model_conn_id || undefined}
        placeholder="跟随智能体默认"
        onChange={(v) => patchConv({ model_conn_id: v ?? '' })}
        options={(modelConns ?? []).map((c) => ({ value: c.id, label: c.name }))}
        data-testid="conv-model-override"
        style={{ width: '100%' }}
      />
      <div className="proc-pop-title">生效配置总览（合并序：会话 &gt; 智能体 &gt; 全局）</div>
      {/* REQ-488：总览彩 Tag 灰字化（覆盖/默认为信息标注非状态色）；说明字 12px */}
      <div style={{ fontSize: 12, lineHeight: 1.9 }}>
        <div>
          模型：{conversation.model_conn_id && <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>会话覆盖 · </span>}
          {(modelConns ?? []).find((c) => c.id === conversation.model_conn_id)?.name ?? subjectName ?? '跟随'}
          {!conversation.model_conn_id && <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>（智能体默认）</span>}
        </div>
        <div>
          审批：{conversation.tool_approval === 'off' ? '会话关闭' : conversation.tool_approval === 'on' ? '会话开启' : '跟随智能体'}
          {conversation.tool_approval !== 'off' && conversation.tool_approval !== 'on' && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>（{agentApprovalLabel}）</Typography.Text>
          )}
        </div>
        <div>
          上下文预算：{conversation.context_mode && <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>会话覆盖 · </span>}
          {conversation.context_mode || '跟随'}
          {!conversation.context_mode && <span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>（智能体档）</span>}
        </div>
        {/* REQ-550/M357：项目归属工作区时透出默认值来源（写时复制——行内值已固化，与手动新建无异） */}
        {isProjectScope && project?.workspace_id && (
          <div>
            知识库/本体默认值：<span style={{ color: 'var(--c-ink-3)', fontSize: 11 }}>创建时继承自工作区（写时复制）</span>
          </div>
        )}
      </div>
      <div className="proc-pop-title">上下文预算档（会话级覆盖，REQ-393）</div>
      <Segmented
        size="small"
        value={conversation.context_mode ?? ''}
        onChange={(v) => patchConv({ context_mode: v as string })}
        options={[
          { value: '', label: '跟随' },
          { value: 'compact', label: '紧凑' },
          { value: 'standard', label: '标准' },
          { value: 'full', label: '完整' },
        ]}
        data-testid="conv-context-mode"
      />
      <div className="proc-pop-title">会话检索参数（0=跟随知识库默认）</div>
      <div className="cs-row">
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>top_k</Typography.Text>
        <InputNumber
          size="small" min={0} max={50} precision={0} style={{ width: 72 }}
          value={conversation.top_k ?? 0}
          onChange={(v) => patchConv({ top_k: typeof v === 'number' ? v : 0 })}
          data-testid="conv-topk"
        />
        <Typography.Text type="secondary" style={{ fontSize: 11.5 }}>min_score</Typography.Text>
        <InputNumber
          size="small" min={0} max={1} step={0.05} style={{ width: 84 }}
          value={conversation.min_score ?? 0}
          onChange={(v) => patchConv({ min_score: typeof v === 'number' ? v : 0 })}
          data-testid="conv-minscore"
        />
      </div>
      {!isProjectScope && agent?.companion_ontology_id && (
        <>
          <div className="chip-picker-divider" />
          <div className="proc-pop-title">会话伴生本体（覆盖仅本会话生效）</div>
          <Select
            size="small"
            style={{ width: '100%' }}
            value={conversation.companion_ontology_id || ''}
            onChange={(id) => onCompanionChange(id)}
            options={[
              { value: '', label: `跟随智能体默认${agent.companion_ontology_id ? `（${companionOntos.find((o) => o.id === agent.companion_ontology_id)?.name ?? '已绑定'}）` : ''}` },
              ...companionOntos
                .filter((o) => o.id !== agent.companion_ontology_id)
                .map((o) => ({ value: o.id, label: o.name })),
            ]}
          />
        </>
      )}
      {onToggleSidePanel && ((isProjectScope && project) || (!isProjectScope && agent)) && (
        <>
          <div className="chip-picker-divider" />
          <Button
            size="small"
            block
            data-testid="conv-open-panel"
            onClick={() => { onClose(); onToggleSidePanel() }}
          >
            智能体配置（打开侧边栏）
          </Button>
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
            模型 / 技能挂载 / 提示词等智能体级配置在侧边栏维护
          </Typography.Text>
        </>
      )}
    </div>
  )
}
