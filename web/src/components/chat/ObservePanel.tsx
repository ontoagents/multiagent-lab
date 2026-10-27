// REQ-401③/M197 收口：过程面板（头部「过程」Popover content 抽件——零行为变更 prop 化）。
// 显示档（粒度/深度思考/原始 JSON）与采集档（调试级别/入库开关）两段+关系说明；
// 状态与 patchConvCfg/changeDebugLevel 留 ChatWindow 主件，本件纯渲染+回调上抛。
import { Segmented, Switch } from 'antd'

export default function ObservePanel({
  granularity,
  patchConvCfg,
  showReasoning,
  showRaw,
  setShowRaw,
  debugLevel,
  changeDebugLevel,
  debugPersist,
}: {
  granularity: 'all' | 'key' | 'off'
  patchConvCfg: (patch: Record<string, unknown>) => void
  showReasoning: boolean
  showRaw: boolean
  setShowRaw: (v: boolean) => void
  debugLevel: number
  changeDebugLevel: (k: number) => void
  debugPersist: boolean
}) {
  return (
    <div className="proc-pop" data-testid="proc-pop">
      <div className="proc-pop-title">过程显示 · 聊天流展示多少（即时生效，会话级记忆）</div>
      <Segmented
        size="small"
        value={granularity}
        onChange={(v) => patchConvCfg({ granularity: v as 'all' | 'key' | 'off' })}
        options={[
          { value: 'all', label: '全部' },
          { value: 'key', label: '关键' },
          { value: 'off', label: '精简' },
        ]}
      />
      {/* REQ-336：自调用轨迹「显示设置」归并（轨迹页只留轨迹） */}
      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginTop: 10 }}>
        <span style={{ fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          深度思考过程 <Switch size="small" checked={showReasoning} onChange={(v) => patchConvCfg({ showReasoning: v })} />
        </span>
        <span style={{ fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          原始事件 JSON <Switch size="small" checked={showRaw} onChange={setShowRaw} />
        </span>
      </div>
      <div className="chip-picker-divider" />
      <div className="proc-pop-title">采集档 · 后端记录多少（仅影响之后的运行）</div>
      <div className="proc-pop-opts">
        {([
          { k: 0, label: '简洁（默认）', hint: '常规过程卡' },
          { k: 1, label: '详细', hint: '另记装配快照/分步用量' },
          { k: 2, label: '调试', hint: '另附模型输入全文' },
        ] as const).map((o) => (
          <button
            key={o.k}
            type="button"
            className={`chip-picker-item${debugLevel === o.k ? ' current' : ''}`}
            data-testid={`proc-lv${o.k}`}
            onClick={() => changeDebugLevel(o.k)}
          >
            <span className="chip-picker-name">{o.label}{debugLevel === o.k ? ' ✓' : ''}</span>
            <span className="chip-picker-meta">{o.hint}</span>
          </button>
        ))}
      </div>
      <button
        type="button"
        className={`chip-picker-item${debugPersist ? ' current' : ''}`}
        data-testid="proc-persist"
        onClick={() => patchConvCfg({ debugPersist: !debugPersist })}
      >
        <span className="chip-picker-name">{debugPersist ? '✓ ' : ''}调试事件入库</span>
        <span className="chip-picker-meta">轨迹/回放可回看</span>
      </button>
      <div className="proc-pop-hint">
        两档关系：采集档决定「记多少」，显示档决定「看多少」——「精简」显示下已记录的细节仍可在「调用轨迹」回看（入库后跨会话可查）。
      </div>
    </div>
  )
}
