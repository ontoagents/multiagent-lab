import { useCallback, useEffect, useMemo, useState } from 'react'
import { Alert, Badge, Breadcrumb, Button, Card, Checkbox, Collapse, Divider, Empty, Form, FormInstance, Input, InputNumber, Modal, Popconfirm, Segmented, Select, Space, Spin, Switch, Tabs, Tag, Tooltip, Typography } from 'antd'
import {
  ApiOutlined,
  ApartmentOutlined,
  CloudOutlined,
  DoubleLeftOutlined,
  DoubleRightOutlined,
  ClusterOutlined,
  CopyOutlined,
  ExportOutlined,
  FileOutlined,
  FolderOutlined,
  ReloadOutlined,
  SettingOutlined,
  ToolOutlined,
  ControlOutlined,
  InfoCircleOutlined,
DeleteOutlined, PlusOutlined, EyeOutlined} from '@ant-design/icons'
import { api, connDisplayName } from '../api/client'
import type { HarnessAuditEvent } from '../api/client'
import AIOptimizeButton from './AIOptimizeButton'
import AgentCompanionManage from './AgentCompanionManage'
import AssistantProposalBanner from './AssistantProposalBanner'
import type { SandboxStatus } from '../api/client'
import type { Agent, AgentConfigVersion, Connector, InferenceBackendStatus, McpServeInfo, ModelConnection, OntoChatSession, Ontology, Skill, ToolInfo } from '../api/types'
import { companionApi } from '../api/companion'
import { useUI } from '../store/ui'
import { inferenceBackendOptions } from './inferenceOptions'

/** 连接名已按 `{提供商}·{模型}` 约定时直接展示，否则补上模型名（兼容老数据） */
/** REQ-148：连接展示名（组别名优先替换提供商前缀，别名仅展示层） */
const connLabel = (c: ModelConnection) => connDisplayName(c)

/** 模型身份展示用：连接名按 `{提供商}·{模型}` 约定时取提供商前缀，否则取整名 */
const providerOfConn = (c: ModelConnection) => {
  const i = c.name.indexOf('·')
  return i > 0 ? c.name.slice(0, i) : c.name
}

/** REQ-189：侧板宽度三常量与 localStorage 键（拖拽 clamp 320~720，记忆 eino.agentpanel.width）
 *  REQ-326 迭代三：默认展开宽度 560→320（最窄档）——开发者反馈「新建智能体默认展开太宽
 *  挤占对话区」；已有宽度记忆的用户偏好不受影响，拖拽与双击复位（DEFAULT）照常 */
const PANEL_WIDTH_KEY = 'eino.agentpanel.width'
const MIN_PANEL_WIDTH = 320
const MAX_PANEL_WIDTH = 720
const DEFAULT_PANEL_WIDTH = MIN_PANEL_WIDTH
// REQ-21705/M48: activity bar shu-tiao width (collapsed rail)
const PANEL_BAR_WIDTH = 44

/** REQ-240 前端优化①（开发者指令「除基本之外的所有配置页迁移到与智能体配置同一级，不要重复」）：
 *  「智能体配置」视图只留「基本」页签；模型/连接器/对外服务已有独立入口（去重复），
 *  Context/Harness/Loop/能力自配置页签提级为同平级入口（REQ-219 分层视角保持）。
 *  REQ-264 二级配置层级（62 号分析拍板采纳 B+B′+A）：一级收敛七入口 =
 *  配置→模型→执行（Context/Harness/Loop 内层 Segmented 三层并列，默认 Harness）→能力→
 *  对外集成（连接器 client 侧+对外服务 server 侧内层 Segmented）→文件→伴生本体；
 *  簇间分隔线（身份与模型｜执行｜能力与集成｜资产）。 */
export type PanelView =
  | 'config'
  | 'model'
  | 'exec'
  | 'ability'
  | 'integrations'
  | 'files'
  | 'companion'

/** 直接以 AgentConfigForm 承载的一级视图（执行/对外集成经 SegmentedFormView 二级承载） */
const DIRECT_FORM_VIEWS: PanelView[] = ['config', 'model', 'ability']

/** 一级视图 → AgentConfigForm 页签映射 */
const FORM_TAB_OF: Partial<Record<PanelView, string[]>> = {
  config: ['basic'],
  model: ['model'],
  ability: ['ability'],
}

/** REQ-264：存量 localStorage 视图值迁移（context/harness/loop→执行段位记忆；connectors/serve→对外集成段位；graph→配置） */
const LEGACY_VIEW_OF: Record<string, { view: PanelView; seg?: string }> = {
  context: { view: 'exec', seg: 'context' },
  harness: { view: 'exec', seg: 'harness' },
  loop: { view: 'exec', seg: 'loop' },
  connectors: { view: 'integrations', seg: 'connectors' },
  serve: { view: 'integrations', seg: 'serve' },
  graph: { view: 'config' },
}

/** REQ-264：执行入口内层三段（五层视角保留为内层并列；默认 Harness=字段最重的一层） */
const EXEC_SEGMENTS = [
  { key: 'context', tab: 'context', label: 'Context', desc: '上下文层——历史预算、压缩与工具结果剪枝，决定模型每轮「真正看到什么」' },
  { key: 'harness', tab: 'harness', label: 'Harness', desc: '执行面——运行后端、文件安全根、验证背压与工具审批' },
  { key: 'loop', tab: 'loop', label: 'Loop', desc: '循环层——ReAct 迭代上限、挂起恢复与长任务推进' },
]

/** REQ-264：对外集成入口内层两段（client/server 双向闭环，REQ-131/214） */
const INTEGRATION_SEGMENTS = [
  { key: 'connectors', tab: 'connectors', label: '连接器', desc: '作为客户端调用外部能力（REQ-214）；与对外服务构成双向闭环' },
  { key: 'serve', tab: 'serve', label: '对外服务', desc: '作为 MCP 工具暴露给外部客户端（REQ-131，server 侧）' },
]

/** REQ-214/M46：连接器类型徽标（产品层只呈现「连接器」，MCP 为交付驱动之一） */
const CONNECTOR_KINDS: { value: Connector['kind']; label: string; color: string }[] = [
  { value: 'mcp', label: '自定义 MCP', color: 'blue' },
  { value: 'kubernetes', label: 'Kubernetes', color: 'purple' },
  { value: 'ssh', label: 'SSH', color: 'cyan' },
]
const kindMeta = (k: string) => CONNECTOR_KINDS.find((x) => x.value === k) ?? CONNECTOR_KINDS[0]

/**
 * 智能体右侧侧边栏（REQ-103 统一范式 + REQ-132 四分类改版 / M18；REQ-193/M33 双入口）：
 * activity bar（~44px）七入口（REQ-264 二级层级定版）——「配置（基本身份）」「模型」
 * 「执行」（Context/Harness/Loop 内层 Segmented 三层并列，默认 Harness）「能力」「对外集成」
 * （连接器+对外服务内层 Segmented）「文件」（REQ-218④ work_dir 浏览）「伴生本体」（REQ-193 两页）；
 * 簇间分隔线=身份与模型｜执行｜能力与集成｜资产。存量 localStorage 旧视图值自动迁移
 * （context/harness/loop/connectors/serve/graph，见 LEGACY_VIEW_OF）。
 * 五层视角保留：Model=「模型」入口，Context/Harness/Loop=「执行」入口内层并列（REQ-219 口径：
 * 并列性自 rail 降为入口内，层职责边界不变，02 §6 五层总纲仍为权威）。
 * 页签面板 forceRender（跨页签字段同表单提交）；字段/校验/提交 API 不变，仅承载重组。
 */
const PANEL_VIEW_KEY = 'eino.agentpanel.view'

// REQ-213：内置助手基座工具（L0 五只读 + L1 平台知识/提案三件）——内置行不可摘除（禁用勾选态）。
const ASSISTANT_BASE_TOOLS = ['doc_read', 'list_model_connections', 'list_agents', 'list_kbs',
  'get_assistant_config', 'sync_platform_kb', 'search_platform_kb', 'propose_assistant_config']

export default function AgentSidePanel({
  agent,
  open,
  onOpenChange,
  onChanged,
}: {
  agent: Agent
  /** 展开态（true=内容面板，false=竖条）；REQ-237：面板常驻挂载（竖条对齐 REQ-217⑤「常驻右侧」定案），open 仅控制展开/收缩 */
  open: boolean
  /** 展开态变化（竖条点入口展开=true、再点收起=false）；与 ChatWindow 头部收放按钮同源 */
  onOpenChange?: (open: boolean) => void
  onChanged?: () => void
}) {
  // REQ-218①/M49 + REQ-264/M74：activity bar 七入口二级层级——智能体配置 / 模型 / 执行（三层内嵌） /
  // 能力 / 对外集成（连接器+对外服务内嵌）/ 文件 / 伴生本体；簇间分隔线，旧视图值自动迁移。
  // REQ-217⑤/M48：侧板默认收缩为一竖行按钮常驻右侧——点击按钮展开内容、再点同一按钮收起；
  // 宽度记忆保留（展开时生效）。
  const [view, setView] = useState<PanelView>(() => {
    const saved = localStorage.getItem(PANEL_VIEW_KEY)
    // REQ-213/251：内置行无伴生入口；REQ-264：旧视图值（context/harness/loop/connectors/serve/graph）迁移
    if (saved === 'companion' && agent.is_builtin) return 'config'
    const legacy = saved ? LEGACY_VIEW_OF[saved] : undefined
    if (legacy) return legacy.view
    return (saved as PanelView) ?? 'config'
  })
  // REQ-264：二级段位状态上提（跨视图切换保活；存量旧视图值映射为初始段位）
  const [execSeg, setExecSeg] = useState<string>(() => {
    const saved = localStorage.getItem(PANEL_VIEW_KEY)
    return saved === 'context' || saved === 'harness' || saved === 'loop' ? saved : 'harness'
  })
  const [integSeg, setIntegSeg] = useState<string>(() => {
    const saved = localStorage.getItem(PANEL_VIEW_KEY)
    return saved === 'serve' ? 'serve' : 'connectors'
  })
  const [collapsed, setCollapsed] = useState(true) // 默认竖条态（REQ-217⑤「默认收缩」）
  // REQ-237：常驻挂载后 open=展开态权威（头部收放按钮/竖条入口双向同步）
  useEffect(() => {
    setCollapsed(!open)
  }, [open])
  // REQ-230①：候选触达——伴生入口红点（pending>0 亮起；选中 agent 即取数+对话运行后 dataVersion 刷新）
  const { dataVersion } = useUI()
  const [companionPending, setCompanionPending] = useState(0)
  useEffect(() => {
    if (agent.is_builtin || !agent.companion_ontology_id) {
      setCompanionPending(0)
      return
    }
    let alive = true
    companionApi
      .status(agent.id)
      .then((st) => {
        if (alive) setCompanionPending(st.pending_count ?? 0)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [agent.id, agent.companion_ontology_id, dataVersion])
  const switchView = (v: PanelView) => {
    if (v === view && !collapsed) {
      setCollapsed(true) // 再点同一按钮 = 收起回竖条态
      onOpenChange?.(false)
      return
    }
    setView(v)
    if (collapsed) onOpenChange?.(true)
    setCollapsed(false)
    localStorage.setItem(PANEL_VIEW_KEY, v)
  }
  // REQ-189：侧板宽度可调（默认上调 364→560；拖拽 320~720；localStorage 记忆；双击复位）
  const [panelWidth, setPanelWidth] = useState(() => {
    const saved = Number(localStorage.getItem(PANEL_WIDTH_KEY))
    return saved >= MIN_PANEL_WIDTH && saved <= MAX_PANEL_WIDTH ? saved : DEFAULT_PANEL_WIDTH
  })
  const startResize = (e: React.MouseEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startW = panelWidth
    const onMove = (ev: MouseEvent) => {
      const w = Math.min(MAX_PANEL_WIDTH, Math.max(MIN_PANEL_WIDTH, startW + (startX - ev.clientX)))
      setPanelWidth(w)
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      setPanelWidth((w) => {
        localStorage.setItem(PANEL_WIDTH_KEY, String(w))
        return w
      })
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
  return (
    <aside className="proj-panel" style={{ width: collapsed ? PANEL_BAR_WIDTH : panelWidth }}>
      {!collapsed && (
        <div
          className="proj-panel-resizer"
          role="separator"
          aria-label="拖拽调整侧边栏宽度"
          aria-orientation="vertical"
          onMouseDown={(e) => startResize(e)}
          onDoubleClick={() => { setPanelWidth(DEFAULT_PANEL_WIDTH); localStorage.setItem(PANEL_WIDTH_KEY, String(DEFAULT_PANEL_WIDTH)) }}
        />
      )}
      <div className="proj-panel-bar" role="tablist" aria-label="智能体侧边栏视图" style={{ overflowY: 'auto' }}>
        {/* REQ-218①/M49 六入口平级 + REQ-217⑤/M48 再点收起（竖条态常驻默认） */}
        <Tooltip title="智能体配置" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'config' && !collapsed ? ' active' : ''}`}
            aria-label="智能体配置"
            aria-selected={view === 'config' && !collapsed}
            role="tab"
            onClick={() => switchView('config')}
          >
            <SettingOutlined />
          </button>
        </Tooltip>
        <Tooltip title="模型" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'model' && !collapsed ? ' active' : ''}`}
            aria-label="模型"
            aria-selected={view === 'model' && !collapsed}
            role="tab"
            onClick={() => switchView('model')}
          >
            <CloudOutlined />
          </button>
        </Tooltip>
        {/* REQ-264：簇间分隔线（身份与模型｜执行｜能力与集成｜资产） */}
        <span className="proj-bar-sep" />
        <Tooltip title="执行（Context/Harness/Loop）" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'exec' && !collapsed ? ' active' : ''}`}
            aria-label="执行"
            aria-selected={view === 'exec' && !collapsed}
            role="tab"
            onClick={() => switchView('exec')}
          >
            {/* REQ-265：ControlOutlined——原 Thunderbolt 与顶栏「技能」图标撞用 */}
            <ControlOutlined />
          </button>
        </Tooltip>
        <span className="proj-bar-sep" />
        <Tooltip title="能力（技能/工具）" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'ability' && !collapsed ? ' active' : ''}`}
            aria-label="能力"
            aria-selected={view === 'ability' && !collapsed}
            role="tab"
            onClick={() => switchView('ability')}
          >
            {/* REQ-265：ToolOutlined——原 Appstore 与对话右上角侧栏开关图标撞用 */}
            <ToolOutlined />
          </button>
        </Tooltip>
        <Tooltip title="对外集成（连接器/对外服务）" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'integrations' && !collapsed ? ' active' : ''}`}
            aria-label="对外集成"
            aria-selected={view === 'integrations' && !collapsed}
            role="tab"
            onClick={() => switchView('integrations')}
          >
            <ApiOutlined />
          </button>
        </Tooltip>
        <span className="proj-bar-sep" />
        <Tooltip title="文件" placement="left">
          <button
            type="button"
            className={`proj-bar-btn${view === 'files' && !collapsed ? ' active' : ''}`}
            aria-label="文件"
            aria-selected={view === 'files' && !collapsed}
            role="tab"
            onClick={() => switchView('files')}
          >
            <FolderOutlined />
          </button>
        </Tooltip>
        <Tooltip title={companionPending > 0 ? `伴生本体（${companionPending} 条候选待确认）` : '伴生本体'} placement="left">
          <Badge count={companionPending} size="small" offset={[-2, 2]}>
            <button
              type="button"
              className={`proj-bar-btn${view === 'companion' && !collapsed ? ' active' : ''}`}
              aria-label="伴生本体"
              aria-selected={view === 'companion' && !collapsed}
              role="tab"
              // REQ-213：内置助手（平台辅助角色）无伴生诉求，伴生视图入口隐藏
              hidden={!!agent.is_builtin}
              onClick={() => switchView('companion')}
            >
              <ClusterOutlined />
            </button>
          </Badge>
        </Tooltip>
        {/* REQ-251：Git 禁用占位退役——与项目侧板 Git 视图重复，智能体级 Git 未立项（REQ-218「占位保留」口径随之变更） */}
        <span className="proj-bar-spacer" />
        {/* REQ-237：竖条常驻后无「关闭」态——展开/收起即开合（原「关闭侧边栏」按钮退役，与收起语义重复） */}
        <Tooltip title={collapsed ? '展开侧边栏' : '收起为竖条'} placement="left">
          <button
            type="button"
            className="proj-bar-btn"
            aria-label={collapsed ? '展开侧边栏' : '收起为竖条'}
            onClick={() => {
              if (collapsed) {
                onOpenChange?.(true)
                setCollapsed(false)
              } else {
                switchView(view)
              }
            }}
          >
            {/* REQ-326 迭代三：图标方向修复——侧板居右，展开=内容向左铺开（«）、收起=向右收拢（»），
                原两态图标恰好反向（开发者反馈「展开、收缩按钮是反的」） */}
            {collapsed ? <DoubleLeftOutlined /> : <DoubleRightOutlined />}
          </button>
        </Tooltip>
      </div>

      {!collapsed && (
        <div className="proj-panel-view">
          {view === 'companion' && <AgentCompanionView agent={agent} onChanged={onChanged} />}
          {view === 'files' && <AgentFilesView agent={agent} onChanged={onChanged} />}
          {view === 'exec' && (
            <SegmentedFormView
              agent={agent}
              onChanged={onChanged}
              segments={EXEC_SEGMENTS}
              value={execSeg}
              onChange={setExecSeg}
            />
          )}
          {view === 'integrations' && (
            <SegmentedFormView
              agent={agent}
              onChanged={onChanged}
              // REQ-213：内置行不开放对外服务（mcp_serve 锁死）——段位收敛为仅连接器
              segments={agent.is_builtin ? INTEGRATION_SEGMENTS.filter((s) => s.key !== 'serve') : INTEGRATION_SEGMENTS}
              value={integSeg}
              onChange={setIntegSeg}
            />
          )}
          {DIRECT_FORM_VIEWS.includes(view) && (
            <AgentConfigForm agent={agent} onChanged={onChanged} visibleTabs={FORM_TAB_OF[view]} />
          )}
        </div>
      )}
    </aside>
  )
}

// ---------------------------------------------------------------------------
// REQ-264 二级配置承载（62 号分析拍板 B/B′）：Segmented 切换标签置顶 + 表单内容；
// 一句话说明降为内容区顶部小字（开发者指令「最上面是切换标签，下面是标签内内容」——
// 原「标题行+页签行」双标题形态退役）。AgentConfigForm embedded 单实例跨段保活
// （visibleTabs 机制复用，零后端变更）。
// ---------------------------------------------------------------------------

type FormSegment = { key: string; tab: string; label: string; desc: string }

function SegmentedFormView({
  agent,
  onChanged,
  segments,
  value,
  onChange,
}: {
  agent: Agent
  onChanged?: () => void
  segments: FormSegment[]
  value: string
  onChange: (k: string) => void
}) {
  const cur = segments.find((s) => s.key === value) ?? segments[0]
  return (
    <div className="proj-view-body">
      {segments.length > 1 && (
        <Segmented
          block
          size="small"
          style={{ marginBottom: 8 }}
          value={cur.key}
          onChange={(v) => onChange(v as string)}
          options={segments.map((s) => ({ value: s.key, label: s.label }))}
        />
      )}
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 8 }}>{cur.desc}</Typography.Text>
      <AgentConfigForm agent={agent} onChanged={onChanged} visibleTabs={[cur.tab]} embedded />
    </div>
  )
}

// ---------------------------------------------------------------------------
// 伴生本体视图（REQ-193/M33：伴生自「配置」页签上提一级，与「智能体配置」同级；
// 视图内两页——「伴生配置」（开关+REQ-187 三字段，独立表单全量保存）与
// 「伴生管理」（AgentCompanionManage 铺平视图）。「左侧对话、右侧伴生管理」并行
// 几何已具备（ChatWindow 与本侧板 flex 兄弟节点）。整体摘除/成长图 3D 留本体模块
// 伴生子模块全量管理面（D-O19/D-O21 展示位不动）。
// ---------------------------------------------------------------------------

function AgentCompanionView({ agent, onChanged }: { agent: Agent; onChanged?: () => void }) {
  const { showToast, bumpData, setPage: gotoPage } = useUI()
  const [page, setPage] = useState<'config' | 'manage'>('config')
  const [form] = Form.useForm()
  const [allConns, setAllConns] = useState<ModelConnection[]>([])
  const [ontos, setOntos] = useState<Ontology[]>([])
  const [saving, setSaving] = useState(false)
  const [promptsOpen, setPromptsOpen] = useState(false)
  const [promptList, setPromptList] = useState<{ id: string; label: string; purpose: string; text: string }[]>([])
  const [creating, setCreating] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [createName, setCreateName] = useState('')

  // REQ-303 C1：抽取高级配置回填（JSON 列解析四字段）
  useEffect(() => {
    let cfg: { win_msgs?: number; win_chars?: number; max_windows?: number; temperature?: number } = {}
    try {
      cfg = JSON.parse(agent.companion_extract_cfg ?? '{}') ?? {}
    } catch { /* 旧数据无 cfg */ }
    form.setFieldsValue({
      companion_ontology_id: agent.companion_ontology_id ?? '',
      companion_extract_hint: agent.companion_extract_hint ?? '',
      companion_extract_conn_id: agent.companion_extract_conn_id ?? '',
      companion_auto_threshold: agent.companion_auto_threshold ?? 0,
      cfg_win_msgs: cfg.win_msgs,
      cfg_win_chars: cfg.win_chars,
      cfg_max_windows: cfg.max_windows,
      cfg_temperature: cfg.temperature,
    })
  }, [agent.id, agent.companion_ontology_id, agent.companion_extract_cfg, form])
  useEffect(() => {
    api.listConnections().then(setAllConns).catch(() => {})
    // REQ-216：绑定候选 = 全部本体资产（对话生长产物与其他本体同仓，可复用既有本体沉淀）
    api.listOntologies().then(setOntos).catch(() => {})
  }, [])

  const conns = useMemo(() => allConns.filter((c) => c.conn_type === 'chat' && c.enabled), [allConns])

  // REQ-216②：一键创建空本体（建议名「{agent 名}的伴生本体」）→ 立即绑定
  const doCreate = async () => {
    setCreating(true)
    try {
      const r = await companionApi.bindAgent(agent.id, { create: { name: createName.trim() || undefined } })
      showToast(`已创建并绑定「${r.ontology_id}」的伴生本体` + (r.plan_error ? `（宿主方案暂未就绪：${r.plan_error}）` : '，宿主方案已就绪'))
      setCreateOpen(false)
      form.setFieldsValue({ companion_ontology_id: r.ontology_id })
      bumpData()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setCreating(false)
    }
  }

  // 保存走全量载荷（普通 agent PUT 为 full-replace）：非伴生字段以 props agent 现值透传，
  // 仅伴生字段来自本表单——与其他视图保存互不踩踏（REQ-189 全字段替换坑同源规避）。
  // REQ-216：绑定变更走 bind/unbind 专端点（绑定即确保宿主方案；关闭=解绑），
  // companion_ontology_id 随载荷落库（派生开关 companion_ontology 由后端回填）。
  const save = async () => {
    try {
      const v = await form.validateFields()
      const targetOnt: string = v.companion_ontology_id ?? ''
      if (targetOnt === '' && (agent.companion_ontology_id ?? '') !== '') {
        // REQ-230④：解绑不可逆（清候选+游标）——保存路径补确认弹窗（管理页解绑按钮已有 Popconfirm）
        Modal.confirm({
          title: '解绑伴生本体？',
          content: '将清空该智能体全部待确认候选与抽取游标并断开绑定；本体伴生子图数据保留。确认继续保存？',
          okText: '解绑并保存',
          okButtonProps: { danger: true },
          cancelText: '取消',
          onOk: () => doSave(),
        })
        return
      }
      await doSave()
    } catch (e: any) {
      if (e?.errorFields) return
      showToast(e.message, 'err')
      setSaving(false)
    }
  }

  const doSave = async () => {
    try {
      const v = await form.getFieldsValue()
      const targetOnt: string = v.companion_ontology_id ?? ''
      setSaving(true)
      if (targetOnt !== (agent.companion_ontology_id ?? '')) {
        if (targetOnt === '') {
          await companionApi.resetAgent(agent.id) // 解绑：清该 agent 候选与游标（本体伴生子图数据保留）
        } else {
          const r = await companionApi.bindAgent(agent.id, { ontology_id: targetOnt })
          if (r.plan_error) showToast(`绑定成功，但宿主方案暂未就绪（读路径会自动拉起）: ${r.plan_error}`, 'err')
        }
      }
      await api.updateAgent(agent.id, {
        name: agent.name,
        description: agent.description,
        instruction: agent.instruction,
        model_conn_id: agent.model_conn_id || null,
        temperature: agent.temperature ?? null,
        max_tokens: agent.max_tokens ?? null,
        max_iteration: agent.max_iteration ?? 25,
        delegate_timeout_seconds: agent.delegate_timeout_seconds ?? 0, // REQ-319 A2
        runtime_backend: agent.runtime_backend ?? 'inprocess',
        sandbox_memory: agent.sandbox_memory ?? '',
        sandbox_cpus: agent.sandbox_cpus ?? 0,
        inference_backend: agent.inference_backend ?? 'eino-adk',
        logo_url: (agent.logo_url ?? '').trim(),
        context_mode: agent.context_mode ?? '', // REQ-201：full-replace 透传防清零（伴生表单不含此字段）
        work_dir: agent.work_dir ?? '', // REQ-202：透传防清零
        verify_command: agent.verify_command ?? '', // REQ-202：透传防清零
        tools: agent.tools ?? [],
        skills: agent.skills ?? [],
        mcp_servers: agent.mcp_servers ?? [],
        mcp_serve: { enabled: !!agent.mcp_serve?.enabled, tool_name: agent.mcp_serve?.tool_name ?? '', token: agent.mcp_serve?.token ?? '' },
        companion_ontology_id: targetOnt,
        companion_extract_hint: v.companion_extract_hint ?? '',
        companion_extract_conn_id: v.companion_extract_conn_id ?? '',
        companion_auto_threshold: v.companion_auto_threshold ?? 0,
        // REQ-303 C1：抽取高级设置（JSON 合列；空值字段剔除=env 默认兜底）
        companion_extract_cfg: (() => {
          const raw: Record<string, number> = {}
          if (v.cfg_win_msgs != null) raw.win_msgs = v.cfg_win_msgs
          if (v.cfg_win_chars != null) raw.win_chars = v.cfg_win_chars
          if (v.cfg_max_windows != null) raw.max_windows = v.cfg_max_windows
          if (v.cfg_temperature != null) raw.temperature = v.cfg_temperature
          return Object.keys(raw).length ? JSON.stringify(raw) : ''
        })(),
      })
      showToast('伴生配置已保存，下次运行生效')
      bumpData()
      onChanged?.()
    } catch (e: any) {
      if (e?.errorFields) return
      showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="proj-view-body">
      {/* REQ-195：两页各带标题+一句话说明（页切换右置），替代裸 Segmented 孤行 */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8, marginBottom: 10 }}>
        <div style={{ minWidth: 0 }}>
          <Typography.Text strong style={{ fontSize: 13 }}>{page === 'config' ? '伴生配置' : '伴生管理'}</Typography.Text>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 11 }}>
              {page === 'config'
                ? '绑定伴生本体与抽取字段（REQ-216）；保存后下次运行生效'
                : '本智能体跨会话候选确认流（铺平视图）；入图写入绑定本体伴生子图'}
            </Typography.Text>
          </div>
        </div>
        <Segmented
          size="small"
          value={page}
          onChange={(v) => setPage(v as typeof page)}
          options={[
            { value: 'config', label: '伴生配置' },
            { value: 'manage', label: '伴生管理' },
          ]}
        />
      </div>
      {page === 'config' ? (
        <Form form={form} layout="vertical" requiredMark={false} size="small">
          <Form.Item
            name="companion_ontology_id"
            label="伴生本体"
            className="item-inline"
            tooltip="对话收尾抽取的知识写入该本体的伴生子图——多智能体绑同一本体即共享沉淀；绑定即自动创建并运行宿主方案；清空=解绑（候选与游标清除，伴生子图数据保留）"
          >
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="选择本体开启伴生（不选=关闭）"
              options={[
                ...ontos.map((o) => ({ value: o.id, label: `${o.name}（${o.id.slice(0, 14)}…）` })),
                { value: '__create__', label: '✚ 创建新伴生本体（空本体）' },
              ]}
              onChange={(v) => {
                if (v === '__create__') {
                  setCreateName(companionOntologyNameOf(agent))
                  setCreateOpen(true)
                  form.setFieldsValue({ companion_ontology_id: agent.companion_ontology_id ?? '' })
                }
              }}
            />
          </Form.Item>
          <Form.Item
            name="companion_extract_hint"
            label="领域聚焦提示（可选，REQ-187）"
            extra="追加到抽取提示词：定义本 agent 领域内「什么值得沉淀」（例：重点关注 Kubernetes 部署与回滚术语；忽略寒暄与操作细节）"
          >
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="留空 = 通用抽取标准" />
          </Form.Item>
          <Form.Item
            name="companion_extract_conn_id"
            label="抽取模型连接"
            className="item-inline"
            tooltip="留空=跟随智能体模型连接（外部 CLI 后端 agent 无生效连接时须指定真实 chat 连接）"
          >
            <Select allowClear showSearch optionFilterProp="label" placeholder="跟随智能体模型连接" options={conns.map((c) => ({ value: c.id, label: connLabel(c) }))} />
          </Form.Item>
          <Form.Item
            name="companion_auto_threshold"
            label="自动入图置信阈值"
            className="item-inline"
            tooltip="0=全部候选人工确认（默认）；>0 时置信达标候选自动入图（带 autoConfirmed 标记），其余仍待人工审"
          >
            <InputNumber min={0} max={1} step={0.05} style={{ width: '100%' }} placeholder="0（全人工确认）" />
          </Form.Item>
          {/* REQ-303 C1：抽取高级设置（每 agent 覆盖；空=env 默认 16/8000/3/0.2） */}
          <Collapse
            size="small"
            ghost
            items={[{
              key: 'extract-adv',
              label: <span style={{ fontSize: 12 }}>抽取高级设置（可选）</span>,
              children: (
                <>
                  <Form.Item name="cfg_win_msgs" label="每窗消息数" className="item-inline" tooltip="单窗携带的消息条数上限（越小越细粒度，token 越省）">
                    <InputNumber min={2} max={100} style={{ width: '100%' }} placeholder="16" />
                  </Form.Item>
                  <Form.Item name="cfg_win_chars" label="每窗字符数" className="item-inline" tooltip="单窗语料字符上限">
                    <InputNumber min={1000} max={100000} step={1000} style={{ width: '100%' }} placeholder="8000" />
                  </Form.Item>
                  <Form.Item name="cfg_max_windows" label="单轮最大窗数" className="item-inline" tooltip="每轮对话收尾最多抽取的窗数；积压窗自动在下轮续抽">
                    <InputNumber min={1} max={20} style={{ width: '100%' }} placeholder="3" />
                  </Form.Item>
                  <Form.Item name="cfg_temperature" label="抽取温度" className="item-inline" tooltip="低温（0~0.3）输出更稳定；留空=0.2">
                    <InputNumber min={0} max={2} step={0.1} style={{ width: '100%' }} placeholder="0.2" />
                  </Form.Item>
                  <Button
                    size="small"
                    block
                    icon={<EyeOutlined />}
                    onClick={async () => {
                      if (promptList.length === 0) {
                        try {
                          const r = await companionApi.prompts()
                          setPromptList(r.prompts ?? [])
                        } catch { /* 静默：抽屉仍开，提示加载失败 */ }
                      }
                      setPromptsOpen(true)
                    }}>
                    查看抽取提示词
                  </Button>
                </>
              ),
            }]}
            style={{ marginBottom: 8 }}
          />
          <div className="proj-view-actions">
            {/* REQ-326③：跨模块深链带详情语义——已绑定时直达绑定本体的资产详情 */}
            {agent.companion_ontology_id && (
              <Button
                size="small"
                icon={<ApartmentOutlined />}
                onClick={() => {
                  localStorage.setItem('eino.onto.sidebar', 'assets')
                  localStorage.setItem('eino.onto.asset.focus', agent.companion_ontology_id!)
                  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
                  gotoPage('ontology')
                }}
              >
                打开本体资产
              </Button>
            )}
            <Button type="primary" size="small" loading={saving} onClick={save}>
              保存伴生配置
            </Button>
          </div>
        </Form>
      ) : agent.companion_ontology ? (
        <AgentCompanionManage agent={agent} />
      ) : (
        // REQ-195：开关未开启时管理页不再照常呈现（此前与开关状态脱节）——引导先开启
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          style={{ padding: '24px 0' }}
          description={<span style={{ fontSize: 12 }}>伴生未开启——绑定伴生本体并对话一轮后，抽取候选在此跨会话确认入图</span>}
        >
          <Button type="primary" size="small" onClick={() => setPage('config')}>
            去绑定伴生本体
          </Button>
        </Empty>
      )}
      <Modal
        title="伴生抽取提示词（只读）"
        open={promptsOpen}
        onCancel={() => setPromptsOpen(false)}
        footer={null}
        width={720}
      >
        {promptList.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载失败——请重试</Typography.Text>
        ) : (
          promptList.map((p) => (
            <div key={p.id} style={{ marginBottom: 14 }}>
              <Typography.Text strong style={{ fontSize: 12 }}>{p.label}</Typography.Text>
              <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 4 }}>{p.purpose}</Typography.Paragraph>
              <Input.TextArea readOnly value={p.text} autoSize={{ minRows: 4, maxRows: 12 }} style={{ fontSize: 11, fontFamily: 'monospace' }} />
            </div>
          ))
        )}
      </Modal>

      {/* REQ-216②：创建空本体弹窗（建议名默认「{agent 名}的伴生本体」） */}
      <Modal
        title="创建伴生本体"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={doCreate}
        okText="创建并绑定"
        okButtonProps={{ loading: creating }}
        cancelText="取消"
        width={420}
      >
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          创建一个空本体作为本智能体的伴生归属（REQ-216 资产化：对话生长产物入其伴生子图，
          在「本体资产」列表可见并随「对话生长」徽标呈现）。
        </Typography.Paragraph>
        <Input value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="伴生本体名称" />
      </Modal>
    </div>
  )
}

/** REQ-216②：伴生本体建议名（与后端 CompanionOntologyName 同口径）。 */
function companionOntologyNameOf(agent: Agent): string {
  return `${agent.name || '未命名智能体'}的伴生本体`
}

/** 文件大小展示（REQ-251 与项目文件视图 .proj-entry-size 同口径） */
const fmtAgentSize = (n: number) => (n > 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)}MB` : `${(n / 1024).toFixed(1)}KB`)

/**
 * REQ-219 顺修：普通 agent PUT 为 full-replace（store.UpdateAgent 全字段写入，空值也落库），
 * 载荷必须全字段构造——表单值 + 非本表单字段按 agent 现值透传防清零（REQ-189/213 模式）。
 * 保存与 SandboxPanel 启动共用（此前启动沙箱仅 PUT 两字段，会把其余配置清零）。
 * REQ-264 顺修：validateFields() 仅返回当前已注册字段的值（二级段位/提级单页签下其余字段
 * 未挂载），此前 `v.x ?? 默认值` 会把未挂载字段落默认值（如从执行视图保存清空描述/指令/
 * 工具白名单）——全字段改为 agent 现值兜底；model_conn_id/temperature/max_tokens 等以
 * null 为合法值的字段用 `!== undefined` 判定（保住「用户清空选择」语义，只在未挂载时取现值）。
 */
function agentFullPayload(agent: Agent, v: Record<string, any>) {
  return {
    name: v.name ?? agent.name,
    description: v.description ?? agent.description ?? '',
    instruction: v.instruction ?? agent.instruction ?? '',
    model_conn_id: v.model_conn_id !== undefined ? v.model_conn_id : (agent.model_conn_id || null),
    temperature: v.temperature !== undefined ? v.temperature : (agent.temperature ?? null),
    max_tokens: v.max_tokens !== undefined ? v.max_tokens : (agent.max_tokens ?? null),
    max_iteration: v.max_iteration ?? agent.max_iteration ?? 25,
    delegate_timeout_seconds: typeof v.delegate_timeout_seconds === 'number' ? v.delegate_timeout_seconds : (agent.delegate_timeout_seconds ?? 0), // REQ-319 A2：成员委派超时（Graph 层）
    runtime_backend: v.runtime_backend ?? agent.runtime_backend ?? 'inprocess',
    sandbox_memory: v.sandbox_memory ?? agent.sandbox_memory ?? '',
    sandbox_cpus: v.sandbox_cpus ?? agent.sandbox_cpus ?? 0,
    inference_backend: v.inference_backend ?? agent.inference_backend ?? 'eino-adk',
    context_mode: v.context_mode ?? agent.context_mode ?? '', // REQ-201/M37：上下文预算档位（Context 层）
    work_dir: (v.work_dir ?? agent.work_dir ?? '').trim(), // REQ-202/M38：文件原语安全根（Harness 层）
    verify_command: (v.verify_command ?? agent.verify_command ?? '').trim(), // REQ-202/M38：verify_on_stop 背压（Harness 层）
    tool_approval: v.tool_approval ?? agent.tool_approval ?? '', // REQ-219 顺修：审批开关（'' = 关闭为合法值）
    approval_exempt: v.approval_exempt ?? agent.approval_exempt ?? [], // REQ-231②：审批豁免清单
    approval_timeout_hours: v.approval_timeout_hours ?? agent.approval_timeout_hours ?? 0, // REQ-231③：挂起超时
    logo_url: (v.logo_url ?? agent.logo_url ?? '').trim(), // REQ-137
    tools: v.tools ?? agent.tools ?? [],
    skills: v.skills ?? agent.skills ?? [],
    connectors: (v.connectors ?? agent.connectors ?? []) as string[], // REQ-214/M46：连接器授权白名单
    // REQ-360/M161：对话建本体能力开关（''=关 | guided | quick）——full-replace 透传防清零
    onchat_mode: v.onchat_mode ?? agent.onchat_mode ?? '',
    // REQ-366/M164：本体操作工具能力开关——full-replace 透传防清零
    onto_ops: v.onto_ops !== undefined ? !!v.onto_ops : !!agent.onto_ops,
    mcp_servers: agent.mcp_servers ?? [], // REQ-214：兼容残留透传（迁移后恒空）
    // REQ-131/M18：对外服务（token 原样保留——重置走专用端点）
    mcp_serve: {
      enabled: v.mcp_serve_enabled !== undefined ? !!v.mcp_serve_enabled : !!agent.mcp_serve?.enabled,
      tool_name: (v.mcp_serve_tool_name !== undefined ? v.mcp_serve_tool_name : agent.mcp_serve?.tool_name ?? '').trim(),
      token: agent.mcp_serve?.token ?? '',
    },
    // REQ-170/187 + REQ-193：伴生字段自「伴生本体」视图维护——按 agent 现值透传
    companion_ontology_id: agent.companion_ontology_id ?? '',
    companion_extract_hint: agent.companion_extract_hint ?? '',
    companion_extract_conn_id: agent.companion_extract_conn_id ?? '',
    companion_auto_threshold: agent.companion_auto_threshold ?? 0,
    // REQ-232/M59：agent 级 hooks 配置（fetchGuard 启停 + 自定义 deny 规则）
    hooks: {
      fetch_guard_disabled: v.hooks_fetch_guard_disabled ?? agent.hooks?.fetch_guard_disabled ?? false,
      deny_rules: (v.hooks_deny_rules ?? agent.hooks?.deny_rules ?? []).map((r: any) => ({
        name: (r.name ?? '').trim() || '规则',
        tool_pattern: (r.tool_pattern ?? '').trim(),
        args_pattern: (r.args_pattern ?? '').trim(),
        reason: (r.reason ?? '').trim(),
      })).filter((r: any) => r.tool_pattern),
    },
  }
}

// ---------------------------------------------------------------------------
// 配置视图（REQ-132：四分类页签；字段/校验/提交逻辑不变）
// ---------------------------------------------------------------------------

/** REQ-360/M161：「对话建本体」开关+默认模式组合控件（Form.Item 受控——value=onchat_mode 字符串）。
 *  开关语义在后端装配层：onchat_mode 非空 → 自动并入 ontochat_* 四工具+注入内置纪律段（03 §2.54 A），
 *  前端只写字段不复制提示词——引导段从「用户手工粘贴」退役为系统内置。 */
function OnchatModeControl({ value, onChange }: { value?: string; onChange?: (v: string) => void }) {
  const on = !!value
  return (
    <Space size={8} wrap>
      <Switch size="small" checked={on} onChange={(c) => onChange?.(c ? 'guided' : '')} data-testid="onchat-switch" aria-label="对话建本体开关" />
      {on && (
        <Segmented
          size="small"
          value={value}
          onChange={(v) => onChange?.(v as string)}
          options={[
            { value: 'guided', label: '访谈模式' },
            { value: 'quick', label: '快速模式' },
          ]}
        />
      )}
      <Typography.Text type="secondary" style={{ fontSize: 11 }}>
        {on ? `已开启（默认${value === 'quick' ? '快速' : '访谈'}模式；运行时自动并入 ontochat_* 工具+注入内置访谈纪律段）` : '关闭——开启后自动并入工具与纪律段，无需手勾/粘贴'}
      </Typography.Text>
    </Space>
  )
}

/** REQ-366/M164：本体操作工具能力开关（OntoOps）——开启时挂载本体的会话装配自动并入
 *  facade 13 件扩展工具（spec CRUD/校验/CQ 基准）；写类 upsert/delete 触发 danger 档审批。
 *  开关语义在后端装配层（agent.onto_ops），前端只写字段；默认关=存量智能体装配零变化。 */
function OntoOpsControl({ value, onChange }: { value?: boolean; onChange?: (v: boolean) => void }) {
  const on = !!value
  return (
    <Space size={8} wrap>
      <Switch size="small" checked={on} onChange={(c) => onChange?.(c)} data-testid="ontoops-switch" aria-label="本体操作工具开关" />
      <Typography.Text type="secondary" style={{ fontSize: 11 }}>
        {on ? '已开启——挂载本体的会话自动并入 spec CRUD/校验/CQ 基准 13 件工具（写类触发审批）' : '关闭——开启后挂载本体的会话可由智能体直接增删改本体资产（写类触发审批）'}
      </Typography.Text>
    </Space>
  )
}

/** REQ-360/M161 D：产出清单——本智能体的 OntoChat 构建会话（进行中/已入库计数+「继续」深链）。
 *  数据源=构建平面会话列表 agent_id 过滤（REQ-354/P2·G4 审计两列的消费闭环）；
 *  生命周期口径：搁置为合法终态（半途会话在构建平面列表可删可续作，此处不暴露删除防误删）。 */
function OnchatCapabilitySection({ agent, form }: { agent: Agent; form: FormInstance }) {  const { setPage } = useUI()
  const mode = Form.useWatch('onchat_mode', form) ?? agent.onchat_mode ?? ''
  const on = mode !== ''
  const [sessions, setSessions] = useState<OntoChatSession[] | null>(null)

  useEffect(() => {
    let alive = true
    api.listOntoChatSessions(agent.id)
      .then((l) => { if (alive) setSessions(Array.isArray(l) ? l : []) })
      .catch(() => { if (alive) setSessions([]) })
    return () => { alive = false }
  }, [agent.id])

  // 继续深链：写跨栏记忆 + onto-sidebar-change + 切本体模块（REQ-354/P2 同款四行动线）。
  // 深链=显式路径选择，连带置 MIGRATED 标志——防 REQ-337 buildPath 一次性迁移把显式值吃回
  // home（全新浏览器首次跳转必中；沿显式选择仍记忆的迁移语义补齐写入侧）。
  const continueInBuild = (sid: string) => {
    localStorage.setItem('eino.onto.buildPath', 'ontochat')
    localStorage.setItem('eino.onto.buildPath.homeMigrated', '1')
    localStorage.setItem('eino.ontochat.session.focus', sid)
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
    setPage('ontology')
  }

  const ongoing = (sessions ?? []).filter((s) => s.stage !== 'done').length
  const saved = (sessions ?? []).filter((s) => s.stage === 'done' || !!s.ontology_id).length

  return (
    <>
      <Divider titlePlacement="left" plain style={{ margin: '4px 0 12px' }}>对话建本体（OntoChat 能力开关，REQ-360）</Divider>
      <div data-testid="onchat-section">
        <Form.Item name="onchat_mode" style={{ marginBottom: 6 }}>
          <OnchatModeControl />
        </Form.Item>
      </div>
      {on && (
        <div data-testid="onchat-sessions" style={{ marginTop: 8 }}>
          <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
            产出清单：进行中 {ongoing} · 已入库 {saved}
            {sessions && sessions.length === 0 ? '（暂无构建会话——对话中说「帮我构建一个本体」即可开始）' : ''}
            {!sessions ? '（清单加载失败，稍后重试）' : ''}
          </Typography.Text>
          {(sessions ?? []).slice(0, 5).map((s) => (
            <div key={s.id} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '2px 0', minWidth: 0 }}>
              <Tag {...(s.stage === 'done' ? { color: 'green' } : { color: 'processing' })} style={{ margin: 0, fontSize: 11 }}>
                {s.stage === 'done' ? '已入库' : '进行中'}
              </Tag>
              <Typography.Text ellipsis style={{ flex: 1, fontSize: 12 }} title={s.title}>{s.title || s.id}</Typography.Text>
              {s.source === 'agent_chat' && <Tag style={{ margin: 0, fontSize: 10 }}>对话渠道</Tag>}
              <Button size="small" type="link" style={{ padding: 0, fontSize: 12 }} onClick={() => continueInBuild(s.id)}>
                继续
              </Button>
            </div>
          ))}
          {(sessions?.length ?? 0) > 5 && (
            <Button size="small" type="link" style={{ padding: 0, fontSize: 11 }} onClick={() => continueInBuild('')}>
              在构建平面查看全部 {sessions!.length} 条…
            </Button>
          )}
        </div>
      )}
    </>
  )
}

function AgentConfigForm({
  agent,
  onChanged,
  visibleTabs,
  embedded,
}: {
  agent: Agent
  onChanged?: () => void
  /** REQ-218①/M49：activity bar 提级视图复用本表单（单实例全字段 forceRender，保存载荷不受限）——
   *  传定时仅渲染指定页签（如 ['model'] / ['ability']）；空 = 全部页签（配置视图）。 */
  visibleTabs?: string[]
  /** REQ-264：二级承载模式——外层 proj-view-body 与标题/切换由 SegmentedFormView 负责，本表单只出表单体 */
  embedded?: boolean
}) {
  const { showToast, bumpData, setPage } = useUI()
  const [form] = Form.useForm()
  // REQ-213：内置行（平台助手）适配——白名单 8 字段可编辑（instruction/模型/温度/tools/skills/
  // mcp/max_tokens/max_iteration），身份与角色字段禁用；基座工具不可摘除；提案横幅置顶。
  const isBuiltin = !!agent.is_builtin
  const instructionValue = Form.useWatch('instruction', form) ?? ''
  const [allConns, setAllConns] = useState<ModelConnection[]>([])
  const [tools, setTools] = useState<ToolInfo[]>([])
  const [toolsErr, setToolsErr] = useState(false)
  const [skills, setSkills] = useState<Skill[]>([]) // REQ-164：技能勾选候选（注册表来源，仅启用项）
  const [backends, setBackends] = useState<InferenceBackendStatus[]>([]) // M13：推理后端探测清单
  // REQ-231⑤⑥：运行时工具预览 + hooks 注册真相（后端读取——前端静态文案退役）
  const [previewTools, setPreviewTools] = useState<{ name: string; source: string }[]>([])
  const [previewNotes, setPreviewNotes] = useState<string[]>([])
  const [previewMasked, setPreviewMasked] = useState<{ name: string; source: string }[]>([]) // REQ-311 A3：被遮蔽工具（后端已返回，此前未渲染）
  const [hooksInfo, setHooksInfo] = useState<{ name: string; active: boolean; description: string }[]>([])
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  // REQ-214/M46：连接器授权候选（设置页「连接器」分区为管理面，侧板只做勾选授权）
  const [connectors, setConnectors] = useState<Connector[]>([])

  useEffect(() => {
    form.setFieldsValue({
      ...agent,
      approval_exempt: agent.approval_exempt ?? [], // REQ-231②③：审批面新字段初始化
      approval_timeout_hours: agent.approval_timeout_hours ?? 0,
      hooks_fetch_guard_disabled: agent.hooks?.fetch_guard_disabled ?? false, // REQ-232①
      hooks_deny_rules: agent.hooks?.deny_rules ?? [], // REQ-232②
      // REQ-131：对外服务开关/工具名平铺为表单字段（token 不进表单，走专用端点管理）
      mcp_serve_enabled: agent.mcp_serve?.enabled ?? false,
      mcp_serve_tool_name: agent.mcp_serve?.tool_name ?? '',
    })
    api.listConnections().then(setAllConns).catch(() => {})
    // REQ-231⑤⑥：工具预览与 hooks 清单（agent 变更即取；后端读取——前端静态文案退役）
    api.agentToolPreview(agent.id).then((r) => {
      setPreviewTools(r.tools ?? [])
      setPreviewNotes(r.notes ?? [])
      setPreviewMasked(r.masked ?? []) // REQ-311 A3：遮蔽清单接线（重名先到先得被跳过的工具）
    }).catch(() => {})
    api.listHooks(agent.id).then((r) => setHooksInfo(r.hooks ?? [])).catch(() => {})
    // M5：工具注册表（失败降级为空 + 提示，不阻塞保存）
    api
      .listTools()
      .then((ts) => {
        setTools(ts)
        setToolsErr(false)
      })
      .catch(() => setToolsErr(true))
    // M13：推理后端探测清单（失败降级为仅 eino-adk 默认项）
    api.listInferenceBackends().then((r) => setBackends(r.backends ?? [])).catch(() => {})
  }, [agent.id, form])

  // REQ-164：技能注册表加载（勾选候选；失败静默，能力页签仍可用工具/MCP）
  // REQ-373：集合入候选——label 带「集合」标注、title 附成员清单（挂载集合=按启用成员展开注入）
  useEffect(() => {
    api
      .listSkills()
      .then((ls) => setSkills(ls.filter((x) => x.enabled)))
      .catch(() => setSkills([]))
  }, [])

  // REQ-214/M46：连接器候选（失败静默——勾选区空态引导去设置页）
  useEffect(() => {
    api
      .listConnectors()
      .then((r) => setConnectors(r.connectors ?? []))
      .catch(() => setConnectors([]))
  }, [])

  // REQ-214 P2⑥：已授权但当前不可达的连接器（配置时点预警——运行时将降级告警）
  const watchedConnectors = Form.useWatch('connectors', form) as string[] | undefined
  const unreachableAuthorized = useMemo(
    () =>
      connectors.filter(
        (c) => c.status === 'error' && (watchedConnectors ?? agent.connectors ?? []).includes(c.id),
      ),
    [connectors, watchedConnectors, agent.connectors],
  )

  const goConnectorSettings = () => {
    localStorage.setItem('eino.settings.section', 'connectors')
    setPage('settings')
  }

  // 可选 chat 连接（启用中）与生效的全局默认（默认连接须启用，与后端 GetDefaultConnection 语义一致）
  const conns = useMemo(() => allConns.filter((c) => c.conn_type === 'chat' && c.enabled), [allConns])
  const defaultConn = useMemo(
    () => allConns.find((c) => c.conn_type === 'chat' && c.is_default && c.enabled) ?? null,
    [allConns],
  )
  // 当前选中连接（含已停用的历史绑定，便于如实展示身份）
  const modelConnId = Form.useWatch('model_conn_id', form)
  const runtimeBackend = (Form.useWatch('runtime_backend', form) as string | undefined) ?? agent.runtime_backend
  // 51 号 W5 定案④（REQ-206/M41）：外部推理后端的 harness 治理面诚实降维——
  // 运行后端/沙箱/验证命令由外部 harness 自治（UI 禁用+后端保存校验双防线）；
  // tool_approval 仅 deepseek-harness-acp 生效（request_permission 转平台审批面）。
  const inferenceBackend = (Form.useWatch('inference_backend', form) as string | undefined) ?? agent.inference_backend ?? 'eino-adk'
  const isExternalBackend = inferenceBackend !== '' && inferenceBackend !== 'eino-adk'
  const isACPBackend = inferenceBackend === 'deepseek-harness-acp'
  const selectedConn = modelConnId ? allConns.find((c) => c.id === modelConnId) ?? null : null

  const save = async () => {
    try {
      const v = await form.validateFields()
      setSaving(true)
      if (isBuiltin) {
        // REQ-213：内置行走白名单 8 字段（后端合并保护身份/角色字段；tools 基座并集兜底）
        await api.updateAgent(agent.id, {
          instruction: v.instruction ?? '',
          model_conn_id: v.model_conn_id || null,
          temperature: v.temperature ?? null,
          max_tokens: v.max_tokens ?? null,
          max_iteration: v.max_iteration ?? 15,
          tools: v.tools ?? [],
          skills: v.skills ?? [],
          mcp_servers: agent.mcp_servers ?? [], // REQ-214：兼容残留透传（迁移后恒空）
        })
        showToast('平台助手配置已保存，下次对话生效')
        bumpData()
        onChanged?.()
        return
      }
      // REQ-219：载荷统一走 agentFullPayload（表单值+非表单字段透传防清零；含 tool_approval 顺修）
      await api.updateAgent(agent.id, agentFullPayload(agent, v))
      showToast('已保存，下次运行生效')
      bumpData()
      onChanged?.()
    } catch (e: any) {
      if (e?.errorFields) return // 表单校验错误，antd 已提示
      showToast(e.message, 'err')
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    try {
      await api.deleteAgent(agent.id)
      showToast('已删除')
      bumpData()
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setDeleting(false)
    }
  }

  /** 小节分隔（页签内二级标题） */
  const sec = (label: string) => (
    <Divider titlePlacement="left" plain style={{ margin: '4px 0 12px' }}>
      {label}
    </Divider>
  )

  const body = (
    <>
      {isBuiltin && <AssistantProposalBanner onChanged={onChanged} />}
      <Form form={form} layout="vertical" initialValues={agent} requiredMark={false} size="small">
        <Tabs
          defaultActiveKey={visibleTabs?.[0] ?? 'basic'}
          size="small"
          // REQ-264 顺调：embedded（执行/对外集成二级段位承载）下隐藏页签行——单段无需页签标题，
          // items forceRender 保留（全字段值进 form store，保存载荷不受限）
          className={embedded ? 'proj-embedded-tabs' : undefined}
          items={[
            {
              key: 'basic',
              label: '基本',
              forceRender: true,
              children: (
                <>
                  <Form.Item name="name" label="名称" className="item-inline" rules={isBuiltin ? [] : [{ required: true, message: '名称必填' }]} tooltip={isBuiltin ? '内置助手名称不可修改' : undefined}>
                    <Input placeholder="智能体名称" disabled={isBuiltin} />
                  </Form.Item>
                  <Form.Item name="description" label="描述（多智能体协作时互相理解）" tooltip={isBuiltin ? '内置助手描述不可修改' : undefined}>
                    <Input.TextArea autoSize={{ minRows: 2, maxRows: 5 }} disabled={isBuiltin} />
                  </Form.Item>
                  <Form.Item name="instruction" label={<Space size={6}>系统提示词（Instruction）<AIOptimizeButton kind="agent_instruction" value={instructionValue} onApply={(v) => form.setFieldValue('instruction', v)} /></Space>}>
                    <Input.TextArea autoSize={{ minRows: 6, maxRows: 14 }} placeholder="定义角色、能力边界、回答风格…" />
                  </Form.Item>
                  {/* REQ-251：推理后端与 Logo 迁「模型」视图（五层 Model=inference 归属；「在哪儿跑」在 Harness） */}
                </>
              ),
            },
            {
              key: 'model',
              label: '模型与参数',
              forceRender: true,
              children: (
                <>
                  <Form.Item
                    name="model_conn_id"
                    label="模型连接"
                    className="item-inline"
                    extra={
                      selectedConn ? (
                        <span className="model-meta" title={selectedConn.base_url}>
                          当前模型：{providerOfConn(selectedConn)} · <span className="model-meta-name">{selectedConn.model_name}</span>
                        </span>
                      ) : defaultConn ? (
                        <span className="model-meta" title={defaultConn.base_url}>
                          留空 = 跟随全局默认：{providerOfConn(defaultConn)} · <span className="model-meta-name">{defaultConn.model_name}</span>
                        </span>
                      ) : (
                        <span className="model-meta warn">
                          {conns.length === 0
                            ? '留空 = 跟随全局默认；当前无可用 chat 连接，可到「设置-模型管理」新增。'
                            : '留空 = 跟随全局默认；当前无启用的 chat 默认连接，可到「设置-模型管理」设置默认。'}
                        </span>
                      )
                    }
                  >
                    <Select allowClear showSearch optionFilterProp="label" placeholder="跟随全局默认" options={conns.map((c) => ({ value: c.id, label: connLabel(c) }))} />
                  </Form.Item>
                  {sec('采样参数')}
                  <Form.Item name="temperature" label="温度" className="item-inline" tooltip="0~2，留空默认">
                    <InputNumber min={0} max={2} step={0.1} style={{ width: '100%' }} placeholder="默认" />
                  </Form.Item>
                  <Form.Item name="max_tokens" label="最大回复 tokens" className="item-inline">
                    <InputNumber min={1} style={{ width: '100%' }} placeholder="默认" />
                  </Form.Item>
                  {/* REQ-219：context_mode 迁 Context 层页签（配置聚合呈现，层职责仍归 Context） */}
                  {/* REQ-251：推理后端自「基本」迁入（五层 Model=inference 归属；「在哪儿跑」在 Harness） */}
                  {sec('推理后端（谁来推理）')}
                  <Form.Item
                    name="inference_backend"
                    label="推理后端"
                    className="item-inline"
                    tooltip={isBuiltin ? '内置助手固定 eino-adk 自研后端' : 'eino-adk 为平台自研（完整能力）；外部 CLI 后端模型由其自身配置决定（Agent 模型连接不生效），技能/MCP 降级为提示注入，不支持多 Agent 编排'}
                  >
                    <Select
                      disabled={isBuiltin}
                      options={inferenceBackendOptions(backends)}
                      showSearch
                      optionFilterProp="label"
                      placeholder="eino-adk（自研默认）"
                    />
                  </Form.Item>
                  <Form.Item
                    name="logo_url"
                    label="自定义后端 Logo URL"
                    className="item-inline"
                    tooltip={isBuiltin ? '内置助手不可自定义' : '推理后端为自定义/外部部署时，会话列表与对话界面展示此图标；未配置回退默认图标'}
                  >
                    <Input placeholder="https://…/logo.png" allowClear disabled={isBuiltin} />
                  </Form.Item>
                </>
              ),
            },
            // REQ-219/M50 五层视角落侧板：Context/Harness/Loop/Graph 与「模型与参数」（Model 层）
            // 并列成栏（开发者定案：context 不并入 Harness）——配置聚合呈现≠层职责混淆
            {
              key: 'context',
              label: 'Context',
              forceRender: true,
              children: (
                <>
                  {/* REQ-264：层级介绍自页签内迁「执行」视图头（SegmentedFormView），此处保留可配置项 */}
                  <Form.Item
                    name="context_mode"
                    label="上下文预算"
                    className="item-inline"
                    tooltip={isBuiltin ? '内置助手固定标准档' : 'REQ-201：历史 token 预算档位——超限时先压缩（LLM 摘要持久化）再裁剪，压缩/裁剪均以运行警告诚实标注。「完整」不限量（存量行为）；「紧凑」约 6k tokens 适合长对话省成本；默认标准约 24k。'}
                  >
                    <Select
                      allowClear
                      disabled={isBuiltin}
                      placeholder="标准（约 24k tokens）"
                      options={[
                        { value: 'compact', label: '紧凑（约 6k tokens）' },
                        { value: 'standard', label: '标准（约 24k tokens）' },
                        { value: 'full', label: '完整（不限量，存量行为）' },
                      ]}
                    />
                  </Form.Item>
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
                    压缩/召回/剪枝策略进程内固定（观察项）：超预算先 LLM 摘要再尾半裁剪；知识库与伴生召回按序注入；工具结果超 4k 截断。
                  </Typography.Text>
                  {/* REQ-311 A6：生效口径显性化——档位即生效值（无运行时改写）；每轮实际账单见对话流「🧾 上下文账单」事件卡 */}
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block' }}>
                    生效口径：未设置=标准档（约 24k tokens）；紧凑=约 6k；完整=不限量。每轮运行的实际账单（历史量/裁剪/注入明细）见对话流「上下文账单」事件卡与调用轨迹「运行始末」。
                  </Typography.Text>
                </>
              ),
            },
            {
              key: 'harness',
              label: 'Harness',
              forceRender: true,
              children: (
                <>
                  {/* REQ-264：层级介绍自页签内迁「执行」视图头 */}
                  {isExternalBackend && (
                    <Alert
                      type="info"
                      showIcon
                      style={{ marginBottom: 8 }}
                      message={`外部推理后端（${inferenceBackend}）：harness 治理由外部自治`}
                      description="运行后端/沙箱资源/验证命令不生效（已禁用）；工作目录仍为平台文件视图根；工具审批仅 DeepSeek Harness·ACP 生效（权限请求转平台审批）。"
                    />
                  )}
                  {sec('运行后端（在哪儿跑）')}
                  <Form.Item name="runtime_backend" label="运行后端" className="item-inline" initialValue="inprocess" tooltip={isBuiltin ? '内置助手固定进程内执行' : isExternalBackend ? '由外部 harness 自治，平台侧配置不生效' : 'inprocess=进程内；docker=容器沙箱；k8s=Pod 沙箱；auto=自动检测（k8s 优先→docker→进程内兜底，需配置 SANDBOX_IMAGE）'}>
                    <Select
                      disabled={isBuiltin || isExternalBackend}
                      options={[
                        { value: 'inprocess', label: 'inprocess（进程内）' },
                        { value: 'docker', label: 'docker（沙箱容器）' },
                        { value: 'k8s', label: 'k8s（Pod 沙箱）' },
                        { value: 'auto', label: 'auto（自动检测：k8s 优先）' },
                      ]}
                    />
                  </Form.Item>
                  {(runtimeBackend === 'docker' || runtimeBackend === 'k8s' || runtimeBackend === 'auto') && (
                    <>
                      <Form.Item name="sandbox_memory" label="沙箱内存上限" className="item-inline" tooltip="留空默认 512m">
                        <Select
                          disabled={isExternalBackend}
                          allowClear
                          placeholder="512m（默认）"
                          options={[{ value: '256m', label: '256m' }, { value: '512m', label: '512m' }, { value: '1g', label: '1g' }, { value: '2g', label: '2g' }]}
                        />
                      </Form.Item>
                      <Form.Item name="sandbox_cpus" label="沙箱 CPU 核数" className="item-inline" tooltip="留空默认 1 CPU">
                        <InputNumber min={0.5} max={8} step={0.5} style={{ width: '100%' }} placeholder="1（默认）" disabled={isExternalBackend} />
                      </Form.Item>
                      <SandboxPanel agent={agent} form={form} />
                    </>
                  )}
                  {sec('文件原语安全根与验证背压（REQ-202，自「能力」迁入）')}
                  <Form.Item
                    name="work_dir"
                    label="工作目录"
                    className="item-inline"
                    tooltip={isBuiltin ? '内置助手不开放文件原语安全根配置' : '文件原语工具（grep/glob/read_file/write_file）的安全根：绝对路径，越界由 SafeJoin 强制拒绝；空=仅项目会话具备文件能力。'}
                  >
                    <Input allowClear disabled={isBuiltin} placeholder="如 /home/user/project（绝对路径，空=不装配文件原语）" />
                  </Form.Item>
                  <Form.Item
                    name="verify_command"
                    label="验证命令"
                    className="item-inline"
                    tooltip={isBuiltin ? '内置助手不开放 verify_on_stop 配置' : isExternalBackend ? '由外部 harness 自治，平台侧 verify 背压不生效' : '运行标记完成前在安全根执行（sh -c，10s 超时）：退出码非 0 即背压——本次运行标记为 verify_failed 并在过程时间线透出输出。示例：go build ./...。'}
                  >
                    <Input allowClear disabled={isBuiltin || isExternalBackend} placeholder="如 go build ./...（空=不验证）" />
                  </Form.Item>
                  {sec('工具调用人工审批（REQ-231/M58 三档精细化）')}
                  <Form.Item
                    name="tool_approval"
                    label="审批策略"
                    className="item-inline"
                    initialValue=""
                    tooltip={isBuiltin ? '内置助手不开放审批策略配置' : isExternalBackend && !isACPBackend ? '当前外部后端不承接平台审批（由外部 harness 自治）' : isACPBackend ? 'deepseek-harness-acp：权限请求（session/request_permission）转平台审批挂起，danger/all 档逐请求询问、关闭档自动放行' : 'danger=仅危险工具（写类内置/http_fetch/连接器写；read-only 免审）；all=全部工具。开启后对外 MCP server 模式调用默认拒绝；对话级开关可覆盖'}
                  >
                    <Select
                      disabled={isBuiltin || (isExternalBackend && !isACPBackend)}
                      options={[
                        { value: '', label: '关闭（直接执行）' },
                        { value: 'danger', label: '危险工具审批（写类/出网/连接器写）' },
                        { value: 'all', label: '全部工具调用前审批' },
                      ]}
                    />
                  </Form.Item>
                  <Form.Item
                    name="approval_exempt"
                    label="审批豁免清单"
                    className="item-inline"
                    tooltip="danger/all 档下勾选的工具直接放行不挂起"
                  >
                    <Select
                      mode="multiple"
                      allowClear
                      disabled={isBuiltin}
                      placeholder="从运行时工具清单选择豁免项"
                      options={previewTools.map((t) => ({ value: t.name, label: `${t.name}（${t.source}）` }))}
                    />
                  </Form.Item>
                  <Form.Item
                    name="approval_timeout_hours"
                    label="审批挂起超时（小时）"
                    className="item-inline"
                    tooltip="0=不限（默认）；挂起超过该时长自动拒绝并告知模型超时语义"
                  >
                    <InputNumber min={0} max={168} step={1} style={{ width: '100%' }} disabled={isBuiltin} placeholder="0（不限）" />
                  </Form.Item>
                  <ToolPreviewCard approval={Form.useWatch('tool_approval', form) ?? ''} tools={previewTools} notes={previewNotes} masked={previewMasked} />
                  {sec('防护 hooks（进程内确定性层，REQ-232 可配置）')}
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
                    {isExternalBackend
                      ? '外部推理后端不走平台装配链，防护 hooks 不生效（由外部 harness 自治）。'
                      : hooksInfo.length > 0
                        ? hooksInfo.map((h) => `${h.name}（${h.active ? '启用' : '停用'}）`).join(' · ')
                        : 'hook 清单加载中…'}
                    <Tooltip title="hook 只放行/拒绝、不改写参数与结果——确定性检查不进模型上下文决策；拒绝走 hook.denied 结构化事件">
                      <InfoCircleOutlined style={{ marginLeft: 4, color: 'var(--c-ink-3)' }} />
                    </Tooltip>
                  </Typography.Text>
                  {!isExternalBackend && (
                    <>
                      {/* REQ-232①：agent 级 fetchGuard 开关 */}
                      <Form.Item name="hooks_fetch_guard_disabled" label="fetchGuard 内网预检" className="item-inline" valuePropName="checked" tooltip="开启=http_fetch 执行前做内网/环回地址预检（SSRF 粗防，默认）；关闭=跳过该预检（自担风险）">
                        <Switch checkedChildren="启用" unCheckedChildren="停用" disabled={isBuiltin} />
                      </Form.Item>
                      {/* REQ-232②：自定义 deny 规则（工具名 glob + 参数子串） */}
                      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
                        自定义 deny 规则（工具名 glob 如 mcp__* · web_*；参数子串大小写不敏感；命中即拒绝并回喂原因）
                      </Typography.Text>
                      <Form.List name="hooks_deny_rules">
                        {(fields, { add, remove }) => (
                          <>
                            {fields.map((field) => (
                              <Space.Compact key={field.key} style={{ display: 'flex', marginBottom: 4, gap: 4 }}>
                                <Form.Item name={[field.name, 'name']} noStyle>
                                  <Input placeholder="规则名" style={{ width: 90 }} disabled={isBuiltin} />
                                </Form.Item>
                                <Form.Item name={[field.name, 'tool_pattern']} noStyle rules={[{ required: true, message: '工具名模式必填' }]}>
                                  <Input placeholder="工具名 glob，如 mcp__*" style={{ width: 150 }} disabled={isBuiltin} />
                                </Form.Item>
                                <Form.Item name={[field.name, 'args_pattern']} noStyle>
                                  <Input placeholder="参数子串（可选）" style={{ width: 130 }} disabled={isBuiltin} />
                                </Form.Item>
                                <Form.Item name={[field.name, 'reason']} noStyle>
                                  <Input placeholder="拒绝原因（可选）" style={{ flex: 1 }} disabled={isBuiltin} />
                                </Form.Item>
                                <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(field.name)} disabled={isBuiltin} />
                              </Space.Compact>
                            ))}
                            <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add({ name: `规则${fields.length + 1}` })} disabled={isBuiltin}>
                              添加 deny 规则
                            </Button>
                          </>
                        )}
                      </Form.List>
                    </>
                  )}
                  {/* REQ-311 B2/M115：Harness 审计跨会话聚合——审批挂起/决议、hook 拦截、verify 背压 */}
                  {sec('审计历史（跨会话聚合，REQ-311 B2）')}
                  <HarnessAuditSection agentId={agent.id} />
                </>
              ),
            },
            {
              key: 'loop',
              label: 'Loop',
              forceRender: true,
              children: (
                <>
                  {/* REQ-264：层级介绍自页签内迁「执行」视图头 */}
                  <Form.Item name="max_iteration" label="最大迭代次数" className="item-inline" tooltip="ReAct 循环单次运行的最大步数上限" initialValue={25}>
                    <InputNumber min={1} max={100} style={{ width: '100%' }} />
                  </Form.Item>
                  {/* REQ-319 A2：成员委派超时（作为成员被项目协调者委派时生效；超时降级回喂不炸 run） */}
                  <Form.Item name="delegate_timeout_seconds" label="委派超时（秒）" className="item-inline" tooltip="REQ-319：本智能体作为项目成员被协调者委派调用时，单次执行的超时秒数；0=不限（默认，存量行为）。超时以结构化错误回喂协调者（可改道/重试），不会终止整个运行">
                    <InputNumber min={0} max={86400} style={{ width: '100%' }} placeholder="0 = 不限" />
                  </Form.Item>
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 4 }}>
                    挂起恢复与定时续跑为会话级（对话窗口配置，checkpoint 持久化重启可恢复）；进度产物 todo.md，usage 随运行事件落库。
                  </Typography.Text>
                </>
              ),
            },
            // REQ-251：Graph 页签退役（纯说明占位零配置项）——编排配置在项目侧板「智能体协作」视图，
            // 委派机制说明随迁；REQ-205 工作流编排落地时再立 Graph 配置入口（五层视角文档口径不变）
            {
              key: 'ability',
              label: '能力',
              forceRender: true,
              children: (
                <>
                  <Form.Item
                    name="skills"
                    label="技能"
                    className="item-inline"
                    tooltip="技能 = 指令 + 工具集 + 资源的打包能力单元；勾选后按对话级开关挂载注入，外部 CLI 后端降级为提示注入"
                  >
                    <Select
                      mode="multiple"
                      allowClear
                      showSearch
                      optionFilterProp="label"
                      placeholder="不挂载技能"
                      options={skills.map((k) => ({
                        value: k.id,
                        label: k.kind === 'collection' ? `${k.name}（集合）` : k.name,
                        title:
                          k.kind === 'collection'
                            ? `${k.description || '技能集合'} · 成员 ${k.children?.length ?? 0} 件（挂载后按启用成员展开注入）`
                            : k.description,
                      }))}
                      notFoundContent="技能注册表为空（到「技能」模块创建）"
                      optionRender={(opt) => (
                        <div className="tool-option">
                          <div className="tool-option-name">
                            <span>{opt.data?.label}</span>
                          </div>
                          {opt.data?.title ? <div className="tool-option-desc">{opt.data.title}</div> : null}
                        </div>
                      )}
                    />
                  </Form.Item>

                  <Form.Item
                    name="tools"
                    label="工具白名单"
                    className="item-inline"
                    tooltip={toolsErr ? '工具注册表暂不可用，可稍后重试' : isBuiltin ? '内置基座工具（L0/L1）不可摘除（禁用项）；可另行勾选通用工具' : '来自工具注册表（内置 / 本体 / MCP 动态工具）；「propose_assistant_config」为平台助手专属'}
                  >
                    <Select
                      mode="multiple"
                      allowClear
                      virtual={false}
                      placeholder={toolsErr ? '工具注册表暂不可用' : '选择可用工具'}
                      options={tools.map((t) => ({
                        value: t.id,
                        label: t.name,
                        title: t.description,
                        source: t.source,
                        // REQ-213：内置行基座八工具禁选（不可摘除）；普通行提案工具禁选（内置专属）
                        disabled: isBuiltin ? ASSISTANT_BASE_TOOLS.includes(t.id) : t.id === 'propose_assistant_config',
                      }))}
                      notFoundContent={toolsErr ? '工具注册表暂不可用' : '暂无工具'}
                      classNames={{ popup: { root: 'tool-select-popup' } }}
                      optionRender={(opt) => (
                        <div className="tool-option">
                          <div className="tool-option-name">
                            <span>{opt.data?.label}</span>
                            {opt.data?.source ? <span className="tool-option-src">{opt.data.source}</span> : null}
                          </div>
                          {opt.data?.title ? <div className="tool-option-desc">{opt.data.title}</div> : null}
                        </div>
                      )}
                    />
                  </Form.Item>

                  {/* REQ-219：work_dir/verify_command/tool_approval 迁 Harness 层页签 */}

                  {/* REQ-360/M161：对话建本体能力开关分区（开关+默认模式+产出清单，03 §2.54 A/D） */}
                  <OnchatCapabilitySection agent={agent} form={form} />

                  {/* REQ-366/M164：本体操作工具能力开关分区（默认关；写类触发审批） */}
                  <Divider titlePlacement="left" plain style={{ margin: '4px 0 12px' }}>本体操作工具（OntoOps 能力开关，REQ-366）</Divider>
                  <div data-testid="ontoops-section">
                    <Form.Item name="onto_ops" style={{ marginBottom: 6 }}>
                      <OntoOpsControl />
                    </Form.Item>
                  </div>

                  {/* REQ-218①/M49：外部连接器授权区块提级 activity bar「连接器」独立视图（connectors 页签） */}
                </>
              ),
            },
            {
              key: 'connectors',
              label: '连接器',
              forceRender: true,
              children: (
                <>
                  {sec('外部连接器（勾选 = 授权本智能体使用）')}
                  <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 8 }}>
                    自定义 MCP / Kubernetes / SSH；凭据服务端绑定不进模型上下文，连接失败降级不阻断运行。
                  </Typography.Text>
                  {unreachableAuthorized.length > 0 && (
                    <Alert
                      style={{ marginBottom: 8 }}
                      type="warning"
                      showIcon
                      title={`已授权连接器当前不可达：${unreachableAuthorized.map((c) => c.name).join('、')}——运行时将降级告警（不加载其工具），可到设置页重新测试`}
                    />
                  )}
                  <Form.Item name="connectors" style={{ marginBottom: 8 }}>
                    <Checkbox.Group style={{ display: 'flex', flexDirection: 'column', gap: 4 }} disabled={isBuiltin}>
                      {connectors.map((c) => {
                        const meta = kindMeta(c.kind)
                        // REQ-214 P2②：工具集提示——授权前知道模型将得到什么工具
                        const tip =
                          (c.tools?.length ?? 0) > 0
                            ? `工具集：${c.tools.join('、')}${c.tested_at ? `（${new Date(c.tested_at).toLocaleString()} 测试）` : ''}`
                            : '尚未测试——设置页「测试」后显示工具集'
                        return (
                          <Checkbox key={c.id} value={c.id}>
                            <Tooltip title={tip}>
                              <span style={{ fontSize: 12 }}>
                                <Tag color={meta.color} style={{ marginInlineEnd: 4 }}>{meta.label}</Tag>
                                {c.name}
                                {c.is_builtin && <Tag style={{ marginInlineEnd: 0 }}>内置</Tag>}
                                {c.status === 'error' && <Tag color="red" style={{ marginInlineEnd: 0 }}>不可达</Tag>}
                                {c.status === 'ok' && <Tag color="green" style={{ marginInlineEnd: 0 }}>可达</Tag>}
                              </span>
                            </Tooltip>
                          </Checkbox>
                        )
                      })}
                    </Checkbox.Group>
                  </Form.Item>
                  {connectors.length === 0 && (
                    <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 6 }}>
                      暂无连接器——在设置页「连接器」分区创建（MCP / Kubernetes / SSH）。
                    </Typography.Text>
                  )}
                  <Button size="small" icon={<SettingOutlined />} onClick={goConnectorSettings}>
                    管理连接器
                  </Button>
                </>
              ),
            },
            // REQ-193：伴生本体页签摘除——上提为侧板一级视图（activity bar「伴生本体」按钮），
            // 开关+REQ-187 三字段迁「伴生配置」页、候选管理迁「伴生管理」页（铺平）
            // REQ-213：内置行不开放对外服务页（mcp_serve 锁死，无从编辑）
            ...(isBuiltin ? [] : [{
              key: 'serve',
              label: '对外服务',
              forceRender: true,
              children: <McpServeTab agent={agent} />,
            }]),
          ].filter((t: { key: string }) => !visibleTabs || visibleTabs.includes(t.key))}
        />
      </Form>

      {/* REQ-226/M54：配置版本与一键回滚（保存即版本；回滚动作自身先快照可再滚回）——
          REQ-264：收敛到「配置（基本）」视图呈现（版本/回滚是全量配置级动作，不在各二级视图重复） */}
      {!isBuiltin && (!visibleTabs || visibleTabs.includes('basic')) && (
        <ConfigVersionCard agentId={agent.id} onRolled={onChanged} />
      )}

      <div className="proj-view-actions">
        <Button type="primary" size="small" loading={saving} onClick={save}>
          保存
        </Button>
        {!isBuiltin && (
          <Popconfirm
            title={`删除智能体「${agent.name}」？`}
            description="其历史对话将保留。"
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={remove}
          >
            <Button danger size="small" loading={deleting}>
              删除智能体
            </Button>
          </Popconfirm>
        )}
      </div>
    </>
  )
  return embedded ? body : <div className="proj-view-body">{body}</div>
}

// ---------------------------------------------------------------------------
// 对外服务（REQ-131/M18）：开关/工具名随本表单保存提交；Token 经专用端点管理
// ---------------------------------------------------------------------------

/**
 * M10/10b 沙箱面板：容器状态可见 + 启动/停止（docker 运行后端的 Agent）。
 * 状态经 /api/agents/{id}/sandbox 轮询（10s），启停后即时刷新；未启用 SANDBOX_IMAGE 时降级提示。
 */
function SandboxPanel({ agent, form }: { agent: Agent; form: FormInstance }) {
  const agentId = agent.id
  const { showToast } = useUI()
  const [st, setSt] = useState<SandboxStatus | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api
      .sandboxStatus(agentId)
      .then((r) => setSt(r))
      .catch(() => setSt(null))
  }, [agentId])
  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load])

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    try {
      await fn()
      showToast(ok)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
      load()
    }
  }

  if (!st?.enabled) {
    return (
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="沙箱后端未启用"
        description="平台未配置 SANDBOX_IMAGE——配置后此处可管理该智能体的 agentd 容器。"
      />
    )
  }
  const running = st.state === 'running'
  return (
    <Card size="small" style={{ marginBottom: 12 }}>
      <Space size={8} wrap style={{ marginBottom: 6 }}>
        <Badge status={running ? 'success' : 'default'} text={running ? '容器运行中' : st.state === 'error' ? `异常：${st.detail ?? ''}` : '容器未运行'} />
        {st.memory || st.cpus ? (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            限制 {st.memory || '512m'} / {st.cpus || 1} CPU
          </Typography.Text>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>限制 512m / 1 CPU（默认）</Typography.Text>
        )}
      </Space>
      <div>
        {running ? (
          <Button size="small" loading={busy} onClick={() => act(() => api.sandboxStop(agentId), '沙箱容器已停止并移除')}>
            停止沙箱
          </Button>
        ) : (
          <Button
            size="small"
            type="primary"
            loading={busy}
            onClick={async () => {
              // REQ-219 顺修：启动前落库资源限制须走全量载荷——后端 PUT 为 full-replace，
              // 此前仅传沙箱两字段会把其余配置清零（agentFullPayload 与保存共用）
              await act(async () => {
                await api.updateAgent(agentId, agentFullPayload(agent, form.getFieldsValue()))
                await api.sandboxStart(agentId)
              }, '沙箱容器已启动（per-Agent agentd）')
            }}
          >
            启动沙箱
          </Button>
        )}
        <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 8 }}>
          每 Agent 一个 agentd 容器（agt-{agentId.slice(0, 8)}…）；对话时自动拉起，此处可手动管理
        </Typography.Text>
      </div>
    </Card>
  )
}

function McpServeTab({ agent }: { agent: Agent }) {
  const { showToast, bumpData } = useUI()
  const form = Form.useFormInstance()
  const enabled = Form.useWatch('mcp_serve_enabled', form) ?? false
  const toolName = Form.useWatch('mcp_serve_tool_name', form) ?? ''
  const [info, setInfo] = useState<McpServeInfo | null>(null)
  const [resetting, setResetting] = useState(false)

  const loadInfo = () => {
    api.getAgentMcpServe(agent.id).then(setInfo).catch(() => setInfo(null))
  }
  useEffect(loadInfo, [agent.id])

  const resetToken = async () => {
    setResetting(true)
    try {
      await api.resetAgentMcpToken(agent.id)
      showToast('Token 已重置（旧 Token 立即失效）')
      loadInfo()
      bumpData()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setResetting(false)
    }
  }

  const effToolName = (toolName || '').trim() || `agent_${agent.id}`
  const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''
  const curl = `curl -X POST "${baseUrl}/mcp" \\\n  -H "Authorization: Bearer $TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"${effToolName}","arguments":{"input":"你好"}},"id":1}'`

  const hasToken = !!(info?.configured || agent.mcp_serve?.token)

  return (
    <>
      <Form.Item name="mcp_serve_enabled" label="开启对外服务" className="item-inline" valuePropName="checked" tooltip="开启后本智能体作为 MCP 工具经平台 /mcp 端点暴露；服务默认仅回环监听，跨机访问需经反代按需暴露">
        <Switch />
      </Form.Item>
      <Form.Item
        name="mcp_serve_tool_name"
        label="工具名"
        className="item-inline"
        tooltip={`缺省为 agent_${agent.id}；须全局唯一，冲突时后注册者跳过`}
      >
        <Input placeholder={`agent_${agent.id}`} allowClear disabled={!enabled} />
      </Form.Item>

      {enabled && (
        <Collapse
          ghost
          size="small"
          items={[
            {
              key: 'access',
              label: <span className="event-link">接入信息（端点 / Token / 调用示例）</span>,
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div className="proj-kv">
                    <span className="proj-k">端点</span>
                    <span className="proj-mono">{baseUrl}/mcp</span>
                    <Button
                      size="small"
                      type="text"
                      icon={<CopyOutlined />}
                      aria-label="复制端点"
                      onClick={() => {
                        navigator.clipboard?.writeText(`${baseUrl}/mcp`).then(
                          () => showToast('端点已复制'),
                          () => showToast('复制失败', 'err'),
                        )
                      }}
                    />
                  </div>
                  <div className="proj-kv">
                    <span className="proj-k">工具名</span>
                    <span className="proj-mono">{effToolName}</span>
                  </div>
                  <div className="proj-kv">
                    <span className="proj-k">Token</span>
                    <span className="proj-mono">{hasToken ? info?.token_mask || '已配置' : '未生成'}</span>
                    <Tooltip title={hasToken ? '重置后旧 Token 立即失效' : '生成 Agent 级 Bearer Token'}>
                      <Button size="small" icon={hasToken ? <ReloadOutlined /> : <ExportOutlined />} loading={resetting} onClick={resetToken} aria-label={hasToken ? '重置 Token' : '生成 Token'} />
                    </Tooltip>
                  </div>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 4 }}>
                    调用示例（$TOKEN 为生成/重置后的 Token 值，仅服务端保存，请妥善保管）：
                  </Typography.Paragraph>
                  <pre className="raw-json" style={{ maxHeight: 170, overflow: 'auto' }}>{curl}</pre>
                  <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 0 }}>
                    安全边界：开启「工具审批」的智能体在 server 模式下调用将被默认拒绝；本平台 /mcp 端点不可配置进其他智能体的 MCP servers（自引用拦截，装配期告警）。
                  </Typography.Paragraph>
                </div>
              ),
            },
          ]}
        />
      )}
      {!enabled && (
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block' }}>
          开启并保存后可见接入信息（端点 / Token / 调用示例）。
        </Typography.Text>
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// 智能体文件视图（REQ-218④/M49）：以 agent.work_dir 为安全根浏览——面包屑 + 单层列表 +
// 1MB 文本预览（沿项目 FilesView 交互模型）；「设为工作目录」把当前浏览目录持久化为
// agent.work_dir（全量载荷防清零），装配期 resolveWorkRoot 随新值生效 = 新 Run 文件工具安全根切换。
// 诚实边界：沙箱容器执行世界与 work_dir 零 volume 绑定（REQ-202 口径，文件原语平台进程内执行）。
// ---------------------------------------------------------------------------

function AgentFilesView({ agent, onChanged }: { agent: Agent; onChanged?: () => void }) {
  const { showToast } = useUI()
  const [path, setPath] = useState('') // 相对 work_dir 的目录（'' = 根）
  const [listing, setListing] = useState<{ path: string; root: string; entries: { name: string; is_dir: boolean; size: number; mod_time: string }[] } | null>(null)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [preview, setPreview] = useState<{ path: string; content: string } | null>(null)
  const [previewErr, setPreviewErr] = useState<string | null>(null)
  const [manualDir, setManualDir] = useState('') // 空态手工输入
  const [setting, setSetting] = useState(false)

  const load = useCallback((p = path) => {
    setLoading(true)
    setErr(null)
    api
      .listAgentDirFiles(agent.id, p)
      .then((r) => setListing(r))
      .catch((e) => {
        setListing(null)
        setErr(e.message)
      })
      .finally(() => setLoading(false))
  }, [agent.id, path])
  useEffect(() => {
    setPreview(null)
    setPreviewErr(null)
    load(path)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent.id, path])

  // 「设为工作目录」：当前浏览目录（绝对路径；空=work_dir 本身则无意义，按钮置灰）持久化（全量载荷防清零）
  const root: string = listing?.root || agent.work_dir || ''
  const absOf = (rel: string) => (rel ? `${root.replace(/\/+$/, '')}/${rel}` : root)
  const setWorkDir = async (dir: string) => {
    setSetting(true)
    try {
      await api.updateAgent(agent.id, { ...agentFullPayload(agent, { ...agent }), work_dir: dir })
      showToast(`工作目录已设为：${dir || agent.work_dir}（新 Run 的文件工具安全根即此目录）`)
      onChanged?.()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSetting(false)
    }
  }

  const openFile = async (name: string) => {
    const rel = path ? `${path}/${name}` : name
    setPreviewErr(null)
    try {
      const content = await api.getAgentDirFile(agent.id, rel)
      setPreview({ path: rel, content })
    } catch (e: any) {
      setPreview(null)
      setPreviewErr(`${rel}: ${e.message}`)
    }
  }

  if (!agent.work_dir) {
    // 空 work_dir 引导态：手工输入路径设为工作目录
    return (
      <div className="proj-view-body">
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="未设置工作目录——设置后文件工具（grep/glob/read/write）以此为安全根"
        />
        <Space.Compact style={{ width: '100%', marginTop: 8 }}>
          <Input placeholder="输入绝对路径，如 /home/me/project" value={manualDir} onChange={(e) => setManualDir(e.target.value)} />
          <Button
            type="primary"
            loading={setting}
            onClick={async () => {
              const d = manualDir.trim()
              if (!d) return
              await setWorkDir(d)
              setPath('')
            }}
          >
            设为工作目录
          </Button>
        </Space.Compact>
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
          也可在 Harness 视图填写工作目录；后端以 fsutil.SafeJoin 限制浏览与文件工具不越出该目录。
        </Typography.Text>
      </div>
    )
  }

  const crumbs = path ? path.split('/') : []
  return (
    <div className="proj-view-body">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', marginBottom: 8 }}>
        <FolderOutlined />
        <Breadcrumb
          items={[
            { title: <a onClick={() => setPath('')}>{agent.work_dir}</a> },
            ...crumbs.map((c, i) => ({
              title: <a onClick={() => setPath(crumbs.slice(0, i + 1).join('/'))}>{c}</a>,
            })),
          ]}
        />
        <span style={{ flex: 1 }} />
        <Tooltip title="把当前浏览目录持久化为工作目录（新 Run 文件工具安全根切换）">
          <Button size="small" disabled={!path} loading={setting} onClick={() => setWorkDir(absOf(path))}>
            设为工作目录
          </Button>
        </Tooltip>
      </div>
      {err && <Alert type="error" showIcon title={err} style={{ marginBottom: 8 }} />}
      {loading ? (
        <Spin size="small" />
      ) : listing ? (
        listing.entries.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="目录为空" />
        ) : (
          // REQ-251：列表样式与项目文件视图归一（.proj-entries 体系，目录优先排序同语义），裸 inline 样式退役
          <ul className="proj-entries">
            {[...listing.entries]
              .sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1))
              .map((e) => (
                <li
                  key={e.name}
                  className="proj-entry"
                  onClick={() => (e.is_dir ? setPath(path ? `${path}/${e.name}` : e.name) : openFile(e.name))}
                >
                  <span className="proj-entry-icon">{e.is_dir ? <FolderOutlined /> : <FileOutlined />}</span>
                  <span className="proj-entry-name" title={e.name}>
                    {e.name}
                  </span>
                  {!e.is_dir && <span className="proj-entry-size">{fmtAgentSize(e.size)}</span>}
                </li>
              ))}
          </ul>
        )
      ) : null}
      {previewErr && <Alert type="warning" showIcon title={previewErr} style={{ marginTop: 8 }} />}
      {preview && (
        <Card size="small" title={preview.path} style={{ marginTop: 10 }}
          extra={<Button size="small" type="text" onClick={() => setPreview(null)}>关闭</Button>}>
          <pre style={{ margin: 0, maxHeight: 320, overflow: 'auto', fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{preview.content}</pre>
        </Card>
      )}
    </div>
  )
}


// ---------------------------------------------------------------------------
// REQ-231⑤（51 号 W3）：运行时工具预览卡——「这个智能体运行时实际会拿到哪些工具」
// 的确定性呈现（后端四源合并结果+遮蔽告警；治 P-H4 仅 debug 快照可见）。危险工具
// 高亮 + 审批策略摘要（与 /api/hooks 卡同面板）。
// REQ-251：不再自行拉取 agentToolPreview（表单 useEffect 已拉，透传防双请求）。
// ---------------------------------------------------------------------------
function ToolPreviewCard({ approval, tools, notes, masked }: { approval: string; tools: { name: string; source: string }[]; notes?: string[]; masked?: { name: string; source: string }[] }) {
  // danger 清单由后端审批策略口径推导（同 IsDangerousTool）——前端按 source 近似标注：
  // builtin 写类 + 非 ontology/oo 连接器前缀
  const danger = useMemo(() => {
    const builtinDanger = ['write_file', 'save_file', 'todo_write', 'http_fetch']
    return tools
      .filter((t) => builtinDanger.includes(t.name) || (t.name.includes('__') && !t.name.startsWith('ontology__') && !t.name.startsWith('oo__')))
      .map((t) => t.name)
  }, [tools])
  return (
    <div style={{ border: '1px solid var(--ant-color-border, #ddd)', borderRadius: 6, padding: '8px 10px', marginBottom: 10 }}>
      <Typography.Text strong style={{ fontSize: 12 }}>运行时工具预览（{tools.length}）</Typography.Text>
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', margin: '2px 0 6px' }}>
        审批策略生效：{approval === '' ? '关闭' : approval === 'danger' ? '危险工具档' : approval === 'all' ? '全部工具' : approval || '跟随 agent'}{danger.length > 0 && approval !== '' ? ` · 危险清单 ${danger.length} 项将挂起审批` : ''}
      </Typography.Text>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
        {tools.map((t) => (
          <Tooltip key={t.name} title={`来源：${t.source}`}>
            <Tag color={danger.includes(t.name) ? 'orange' : 'default'} style={{ margin: 0, fontSize: 11 }}>{t.name}</Tag>
          </Tooltip>
        ))}
      </div>
      {(notes ?? []).length > 0 && (
        <Typography.Text type="secondary" style={{ fontSize: 10.5, display: 'block', marginTop: 6 }}>
          {notes!.join('；')}
        </Typography.Text>
      )}
      {/* REQ-311 A3：被遮蔽工具清单（function name 冲突先到先得被跳过的工具——生效链的另一半） */}
      {(masked ?? []).length > 0 && (
        <div style={{ marginTop: 6 }}>
          <Typography.Text type="warning" style={{ fontSize: 11, display: 'block', marginBottom: 2 }}>
            被遮蔽 {masked!.length} 项（与其他工具重名，本次运行不加载）：
          </Typography.Text>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {masked!.map((t) => (
              <Tooltip key={t.name} title={`来源：${t.source}`}>
                <Tag color="orange" style={{ margin: 0, fontSize: 11, borderStyle: 'dashed' }}>{t.name}</Tag>
              </Tooltip>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}


// ---------------------------------------------------------------------------
// REQ-226/M54：配置版本与一键回滚——「保存即版本」（PUT 成功后端自动快照「更新前」
// 整包配置）；版本列表新→旧 + 选中版本与当前的 diff + 一键回滚（回滚动作自身先快照
// 「回滚前」状态，可再滚回）；保留最近 50 版（后端惰性裁剪）。治 P-2「改坏配置无法还原」。
// ---------------------------------------------------------------------------
function ConfigVersionCard({ agentId, onRolled }: { agentId: string; onRolled?: () => void }) {
  const { showToast, bumpData } = useUI()
  const [versions, setVersions] = useState<AgentConfigVersion[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<number | null>(null)
  const [diffs, setDiffs] = useState<{ field: string; old: string; new: string }[] | null>(null)
  const [rolling, setRolling] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    api
      .listConfigVersions(agentId)
      .then((ls) => {
        setVersions(ls ?? [])
        setSelected(null)
        setDiffs(null)
      })
      .catch(() => setVersions([]))
      .finally(() => setLoading(false))
  }, [agentId])

  useEffect(() => {
    load()
  }, [load])

  const showDiff = async (version: number) => {
    setSelected(version)
    setDiffs(null)
    try {
      const res = await fetch(`/api/agents/${agentId}/config-versions/${version}/diff`)
      const body = await res.json()
      setDiffs(body.diffs ?? [])
    } catch {
      setDiffs([])
    }
  }

  const rollback = async (version: number) => {
    setRolling(true)
    try {
      await api.rollbackConfig(agentId, version)
      showToast(`已回滚到版本 v${version}（回滚前状态已自动快照，可再滚回）`)
      bumpData()
      onRolled?.()
      load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setRolling(false)
    }
  }

  return (
    <Card
      size="small"
      style={{ marginTop: 10 }}
      title={
        <span style={{ fontSize: 12 }}>
          配置版本（保存即版本，REQ-226）
          <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 8 }}>保留最近 50 版</Typography.Text>
        </span>
      }
      extra={
        <Button size="small" icon={<ReloadOutlined />} onClick={load} aria-label="刷新配置版本" />
      }
    >
      {loading ? (
        <Spin size="small" />
      ) : versions.length === 0 ? (
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>暂无历史版本——下次保存配置后自动生成（保存前状态入版本）</Typography.Text>
      ) : (
        <>
          <Select
            size="small"
            style={{ width: '100%' }}
            placeholder="选择历史版本查看与当前配置的差异"
            value={selected}
            onChange={(v) => showDiff(v as number)}
            options={versions.map((v) => ({
              value: v.version,
              label: `v${v.version} · ${v.created_at?.slice(5, 16).replace('T', ' ')} · ${v.note}`,
            }))}
          />
          {selected !== null && (
            <div style={{ marginTop: 8 }}>
              {diffs === null ? (
                <Spin size="small" />
              ) : diffs.length === 0 ? (
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>该版本与当前配置无差异</Typography.Text>
              ) : (
                <div style={{ maxHeight: 160, overflowY: 'auto', fontSize: 11 }}>
                  {diffs.map((d) => (
                    <div key={d.field} style={{ padding: '2px 0', borderBottom: '1px dashed var(--ant-color-border, #eee)' }}>
                      <Tag style={{ margin: 0 }}>{d.field}</Tag>
                      <Typography.Text type="secondary" delete style={{ fontSize: 11 }}>{(d.old || '∅').slice(0, 60)}</Typography.Text>
                      {' → '}
                      <Typography.Text style={{ fontSize: 11 }}>{(d.new || '∅').slice(0, 60)}</Typography.Text>
                    </div>
                  ))}
                </div>
              )}
              <Popconfirm
                title={`回滚到 v${selected}？`}
                description="整包写回该版本配置；回滚前状态会自动快照（可再滚回）。"
                okText="回滚"
                okButtonProps={{ danger: true, loading: rolling }}
                cancelText="取消"
                onConfirm={() => rollback(selected)}
              >
                <Button size="small" danger style={{ marginTop: 8 }} disabled={diffs !== null && diffs.length === 0}>
                  一键回滚到 v{selected}
                </Button>
              </Popconfirm>
            </div>
          )}
        </>
      )}
    </Card>
  )
}

// ---------------------------------------------------------------------------
// REQ-311 B2/M115：Harness 审计跨会话聚合段——该智能体全部会话的审批挂起/决议、hook 拦截、
// verify 背压倒序列表（GET /api/agents/{id}/harness-audit；展开时拉取）。55 号开放问题③
// 「挂起触达」顺带覆盖：挂起中的 approval.request 在列表即时可见。
// v1 范围=agent 直聊会话（项目会话成员执行审计在各会话轨迹内——后端 ListHarnessAudit 边界注）。
// ---------------------------------------------------------------------------
const HARNESS_AUDIT_META: Record<string, { color: string; label: string }> = {
  'approval.request': { color: 'gold', label: '审批挂起' },
  'approval.granted': { color: 'green', label: '批准' },
  'approval.denied': { color: 'red', label: '拒绝' },
  'hook.denied': { color: 'orange', label: '拦截' },
  'verify.completed': { color: 'blue', label: '验证通过' },
  'verify.failed': { color: 'red', label: '验证未通过' },
}

function describeHarnessAudit(type: string, dataJson?: string): string {
  let d: any = {}
  try { d = dataJson ? JSON.parse(dataJson) : {} } catch { /* 原始摘要兜底 */ }
  switch (type) {
    case 'approval.request':
      return `工具 ${d.tool_name ?? '?'} 请求审批（等待处理${Array.isArray(d.options) ? `，${d.options.length} 个选项` : ''}）`
    case 'approval.granted':
    case 'approval.denied':
      return `工具 ${d.tool_name ?? '?'} ${type === 'approval.granted' ? '批准' : '拒绝'}（${d.decision_source === 'timeout' ? '挂起超时自动' : '人工'}决议）`
    case 'hook.denied':
      return `guard=${d.guard ?? '?'} 拦截 ${d.tool_name ?? '?'}：${d.reason ?? ''}`
    case 'verify.completed':
      return `验证命令通过：${d.command ?? ''}`
    case 'verify.failed':
      return `验证命令未通过：${d.command ?? ''}${d.error ? `（${d.error}）` : ''}`
    default:
      return (dataJson ?? '').slice(0, 120)
  }
}

function HarnessAuditSection({ agentId }: { agentId: string }) {
  const [open, setOpen] = useState(false)
  const [rows, setRows] = useState<HarnessAuditEvent[] | null>(null)
  const [total, setTotal] = useState(0)
  const [err, setErr] = useState('')
  const load = useCallback(() => {
    setErr('')
    api.harnessAudit(agentId, 50)
      .then((r) => { setRows(r.events ?? []); setTotal(r.total ?? 0) })
      .catch((e: any) => setErr(e?.message ?? '加载失败'))
  }, [agentId])
  useEffect(() => {
    setRows(null)
    if (open) load()
  }, [open, agentId, load])
  return (
    <Collapse
      size="small"
      ghost
      activeKey={open ? ['a'] : []}
      onChange={(k) => setOpen(Array.isArray(k) && k.includes('a'))}
      items={[{
        key: 'a',
        label: (
          <span style={{ fontSize: 12 }}>
            审批 / 拦截 / 验证事件（跨会话倒序{total > 0 ? `，共 ${total} 条` : ''}；点开展开）
          </span>
        ),
        children: (
          <div data-testid="harness-audit-list">
            {err && <Alert type="warning" showIcon message={err} />}
            {rows === null && !err && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载中…</Typography.Text>
            )}
            {rows !== null && rows.length === 0 && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                暂无记录——开启「危险工具审批」并运行后，审批/拦截/验证事件将在此聚合可见。
              </Typography.Text>
            )}
            {(rows ?? []).map((r) => {
              const meta = HARNESS_AUDIT_META[r.type] ?? { color: 'default', label: r.type }
              return (
                <div key={r.id} className="harness-audit-row" style={{ display: 'flex', gap: 6, alignItems: 'baseline', padding: '3px 0', borderBottom: '1px dashed var(--c-line)', fontSize: 12 }}>
                  <Typography.Text type="secondary" style={{ fontSize: 11, flexShrink: 0 }}>
                    {new Date(r.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                  </Typography.Text>
                  <Tag color={meta.color} style={{ margin: 0, flexShrink: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>{meta.label}</Tag>
                  <span style={{ flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{describeHarnessAudit(r.type, r.data)}</span>
                  <Typography.Text type="secondary" style={{ fontSize: 10.5, flexShrink: 0, maxWidth: 120 }} ellipsis={{ tooltip: r.conversation_title }}>
                    {r.conversation_title || r.conversation_id}
                  </Typography.Text>
                </div>
              )
            })}
          </div>
        ),
      }]}
    />
  )
}
