// REQ-401③/M197 收口：输入区（composer 整块抽件——零行为变更 prop 化）。
// 附件回显条 +「/」快速配置面板 + Sender（prefix 附加/footer chips）+ 隐藏文件 input；
// 状态与 send/stop/uploadAttachments/patchConv/chips 元素构造留 ChatWindow 主件，本件纯渲染+回调上抛。
// REQ-619/M427：本体选择改多选弹窗（ontoPicker 由 ChatWindow 构造传入；单选 pickerList 保留知识库用）
// REQ-623/M433：@ 点名菜单——词首「@」唤出项目成员清单，点选回填 @名（mention 分散模式常驻提示）
import { Button, Popover, Tag, Tooltip, Typography } from 'antd'
import { PaperClipOutlined } from '@ant-design/icons'
import { Sender } from '@ant-design/x'
import { useState } from 'react'
import type { Dispatch, ReactNode, SetStateAction } from 'react'

export default function Composer({
  input,
  setInput,
  slashOpen,
  setSlashOpen,
  attachFiles,
  setAttachFiles,
  attaching,
  convAgentId,
  running,
  canSend,
  placeholder,
  mentionCandidates,
  mentionMode,
  runningProfiles,
  kbs,
  agentSkills,
  ontoDisabled,
  kbDisabled,
  skillsDisabled,
  picker,
  setPicker,
  pickerList,
  kbChip,
  ontoChip,
  skillsChip,
  kbHint,
  ontoHint,
  skillsHint,
  cmpOn,
  cmpPaneCount,
  conversationKbId,
  ontoPicker,
  patchConv,
  onSend,
  onSteer,
  onStop,
  onUploadAttachments,
}: {
  input: string
  setInput: Dispatch<SetStateAction<string>>
  slashOpen: boolean
  setSlashOpen: (v: boolean) => void
  attachFiles: { name: string; size: number }[]
  setAttachFiles: Dispatch<SetStateAction<{ name: string; size: number }[]>>
  attaching: boolean
  convAgentId?: string
  running: boolean
  canSend: boolean
  placeholder: string
  /** REQ-623/M433：@ 点名候选（项目成员清单；空=非项目会话不出菜单） */
  mentionCandidates?: { id: string; name: string }[]
  /** REQ-623/M433：mention 分散模式（@ 为唯一触发——菜单标题常驻语义提示） */
  mentionMode?: boolean
  runningProfiles: { id: string; name: string; engine?: string; status?: string }[]
  kbs: { id: string; name: string; official?: boolean; doc_count?: number }[]
  agentSkills: string[]
  ontoDisabled: boolean
  kbDisabled: boolean
  skillsDisabled: boolean
  picker: 'kb' | 'onto' | null
  setPicker: (v: 'kb' | 'onto' | null) => void
  pickerList: (title: string, items: { id: string; name: string; meta?: string }[], current: string | null, onPick: (id: string) => void, extra?: { icon: ReactNode; label: string; onClick: () => void }, extra2?: ReactNode) => ReactNode
  kbChip: ReactNode
  ontoChip: ReactNode
  skillsChip: ReactNode
  kbHint: string
  ontoHint: string
  skillsHint: string
  cmpOn: boolean
  cmpPaneCount: number
  conversationKbId?: string | null
  /** REQ-619/M427：本体多选弹窗内容（ChatWindow 构造——勾选亮暗态+完成落库） */
  ontoPicker?: ReactNode
  patchConv: (patch: Record<string, unknown>) => void
  onSend: () => void
  /** REQ-525/M332：运行中插话回调（onSteer 在位且 running 时 Sender 提交走插话而非新运行） */
  onSteer?: (message: string) => void
  onStop: () => void
  onUploadAttachments: (files: FileList | null) => void
}) {
  // REQ-623/M433：@ 点名菜单开合——词首 @（含 @后打字中途）唤出；文本不再以 @段结尾即收起
  const [mentionOpen, setMentionOpen] = useState(false)
  const cands = mentionCandidates ?? []
  return (
    <div className="composer">
      <div className="composer-inner">
        {/* REQ-336：附件回显条（发送前可移除；文件已上传入智能体工作目录） */}
        {attachFiles.length > 0 && (
          <div className="attach-tags" data-testid="attach-tags">
            {attachFiles.map((f, i) => (
              <Tag
                key={`${f.name}-${i}`}
                closable
                onClose={() => setAttachFiles((prev) => prev.filter((_, j) => j !== i))}
                style={{ fontSize: 11 }}
              >
                📎 {f.name}
              </Tag>
            ))}
          </div>
        )}
        {/* REQ-336：「/」触发快速配置面板（本体方案/知识库=会话级挂载即生效；技能=插入 / 引用） */}
        {slashOpen && (
          <div className="slash-picker" data-testid="slash-picker">
            <div className="chip-picker-title">快速配置（选择即对会话生效；Esc 关闭）</div>
            <div className="slash-section">本体运行方案{ontoDisabled ? '（暂无 running 方案）' : ''}</div>
            {runningProfiles.slice(0, 6).map((p) => (
              <button
                key={p.id}
                type="button"
                className="chip-picker-item"
                onClick={() => {
                  patchConv({ runtime_profile_id: p.id, ontology_enabled: true })
                  setInput((v) => v.replace(/\/$/, ''))
                  setSlashOpen(false)
                }}
              >
                <span className="chip-picker-name">🗂 {p.name}</span>
                <span className="chip-picker-meta">{p.engine}</span>
              </button>
            ))}
            <div className="slash-section">知识库{kbDisabled ? '（暂无知识库）' : ''}</div>
            {kbs.slice(0, 6).map((k) => (
              <button
                key={k.id}
                type="button"
                className="chip-picker-item"
                onClick={() => {
                  patchConv({ kb_id: k.id, enable_kb: true })
                  setInput((v) => v.replace(/\/$/, ''))
                  setSlashOpen(false)
                }}
              >
                <span className="chip-picker-name">
                  📚 {k.name}
                  {k.official ? <span className="kb-official-badge">官方·平台知识</span> : null}
                </span>
                {typeof k.doc_count === 'number' ? <span className="chip-picker-meta">{k.doc_count} 文档</span> : null}
              </button>
            ))}
            <div className="slash-section">已挂载技能{skillsDisabled ? '（该智能体未挂载技能）' : '（选择即引用，挂载在智能体配置）'}</div>
            {(agentSkills ?? []).slice(0, 6).map((s) => (
              <button
                key={s}
                type="button"
                className="chip-picker-item"
                onClick={() => {
                  setInput((v) => v.replace(/\/$/, `/${s} `))
                  setSlashOpen(false)
                }}
              >
                <span className="chip-picker-name">⚡ {s}</span>
                <span className="chip-picker-meta">插入引用</span>
              </button>
            ))}
          </div>
        )}
        {/* REQ-623/M433：@ 点名成员菜单（词首 @ 唤出；点选回填 @名——结构化 mention_agent_ids 由 ChatWindow 发送时检测） */}
        {mentionOpen && cands.length > 0 && (
          <div className="slash-picker" data-testid="mention-picker">
            <div className="chip-picker-title">
              @ 点名成员{mentionMode ? '（分散模式：@ 是唯一触发方式，可多选按序回答）' : '（点名成员直接回答，跳过协调者路由）'}
            </div>
            {cands.slice(0, 8).map((m) => (
              <button
                key={m.id}
                type="button"
                className="chip-picker-item"
                onClick={() => {
                  setInput((v) => v.replace(/@[^\s@]*$/, `@${m.name} `))
                  setMentionOpen(false)
                }}
              >
                <span className="chip-picker-name">🤖 {m.name}</span>
                <span className="chip-picker-meta">点名回答</span>
              </button>
            ))}
          </div>
        )}
        {/* REQ-322：过程显示分级已迁头部 ops 行（对话历史上方、观测旁），输入区回归「输入卡+会话配置行」两件 */}
        {/* 会话配置 chips 渲染在输入卡内底部（X Sender footer）：知识库 / 本体 / 技能 开关；模型归智能体配置 */}
        <Sender
          value={input}
          onChange={(v) => {
            setInput(() => v)
            // REQ-336：输入「/」触发快速配置（空输入或空白后 freshly typed 的斜杠）
            if (/(^|\s)\/$/.test(v)) setSlashOpen(true)
            else if (!v.endsWith('/') && slashOpen) setSlashOpen(false)
            // REQ-623/M433：词首「@」（含 @后打字中途）唤出点名菜单；@段结束后收起
            if (/(^|\s)@[^\s@]*$/.test(v)) setMentionOpen(true)
            else if (mentionOpen) setMentionOpen(false)
          }}
          prefix={
            <Tooltip title={convAgentId ? '附加本地文件（存入工作目录）' : '会话无智能体，无法附加文件'}>
              <Button
                type="text"
                size="small"
                icon={<PaperClipOutlined />}
                aria-label="附加文件"
                data-testid="attach-btn"
                disabled={!convAgentId || running}
                loading={attaching}
                onClick={() => document.getElementById('attach-file-input')?.click()}
              />
            </Tooltip>
          }
          onSubmit={() => onSend()}
          onCancel={onStop}
          loading={running}
          onKeyDown={(e) => {
            // REQ-525/M332：运行中 Enter=插话——Sender loading 态 triggerSend 被 !loading 短路
            //（源码 134 行），此处拦截转发 onSteer；返回 false 阻止 Sender 内部 Enter 处理
            if (running && onSteer && e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
              e.preventDefault()
              onSteer(input)
              return false
            }
          }}
          placeholder={placeholder}
          disabled={!canSend}
          footer={
            <div className="chat-chips">
              {cmpOn ? (
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                  对比模式 · {cmpPaneCount} 窗格独立配置（知识库/本体/技能承载于各窗格头，本区不参与配置）
                </Typography.Text>
              ) : (<>
              {kbDisabled ? (
                <Tooltip title={kbHint}><span className="chip-slot">{kbChip}</span></Tooltip>
              ) : (
                <Popover
                  open={picker === 'kb'}
                  onOpenChange={(o) => { if (!o) setPicker(null) }}
                  trigger="click"
                  placement="topLeft"
                  arrow={false}
                  content={pickerList(
                    '选择知识库',
                    kbs.map((k) => ({ id: k.id, name: k.name, meta: typeof k.doc_count === 'number' ? `${k.doc_count} 文档` : undefined })),
                    conversationKbId ?? null,
                    (id) => patchConv({ kb_id: id, enable_kb: true }),
                  )}
                >
                  <span className="chip-slot">{kbChip}</span>
                </Popover>
              )}
              {ontoDisabled ? (
                <Tooltip title={ontoHint}><span className="chip-slot">{ontoChip}</span></Tooltip>
              ) : (
                <Popover
                  open={picker === 'onto'}
                  onOpenChange={(o) => { if (!o) setPicker(null) }}
                  trigger="click"
                  placement="topLeft"
                  arrow={false}
                  content={ontoPicker}
                >
                  <span className="chip-slot">{ontoChip}</span>
                </Popover>
              )}
              {skillsDisabled ? <Tooltip title={skillsHint}><span className="chip-slot">{skillsChip}</span></Tooltip> : skillsChip}
              {/* REQ-311 B3 检索参数迁头部⋮会话设置；REQ-336：模型 chip 退役（模型归智能体）、伴生 chip 迁⋮ */}
              <span className="chat-chips-hint">输入 / 快速配置 · Enter 发送 · Shift+Enter 换行</span>
              </>)}
            </div>
          }
        />
        {/* REQ-336：附加文件选择（隐藏 input；回形针触发；上传入智能体工作目录） */}
        <input
          id="attach-file-input"
          type="file"
          multiple
          style={{ display: 'none' }}
          onChange={(e) => {
            onUploadAttachments(e.target.files)
            e.target.value = ''
          }}
        />
      </div>
    </div>
  )
}
