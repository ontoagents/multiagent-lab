import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Input, Popconfirm, Result, Segmented, Space, Tag, Tooltip, Typography } from 'antd'
import {
  ArrowLeftOutlined,
  BranchesOutlined,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  DeleteOutlined,
  EditOutlined,
  ImportOutlined,
  LeftOutlined,
  RightOutlined,
  RocketOutlined,
} from '@ant-design/icons'
import { api, ApiError } from '../../api/client'
import { ONTO_SIDEBAR_KEY } from './OntologyModule'
import type { QualityReport } from '../../api/client'
import { companionApi } from '../../api/companion'
import type { Agent, Ontology, OntologyReferences, RuntimeProfile, Spec } from '../../api/types'
import { expressivityOf } from './shared/expressivity'
import { useUI } from '../../store/ui'
import { sourceTag, type ValidationState } from './shared'
import { clearLeaveGuard, guardLeave, setLeaveGuard } from './shared/leaveGuard'
import { parseOntoHash, writeOntoHash } from './shared/ontoRoute'
import { useOntologies } from './shared/useOntoList'
import CsvIngestPane from './components/CsvIngestPane'
import GraphEditor from './components/GraphEditor'
import SourceView from './components/SourceView'
import AxiomsRawPane from './components/assets/AxiomsRawPane'
import SpecEditorPane from './components/assets/SpecEditorPane'
import ImpactPane from './components/assets/ImpactPane'
import CommunityPane from './components/companion/CommunityPane'
import { ArtifactsPane, ExportPane, ValidatePane } from './components/assets/AssetPanes'
import { RenameModal, VizTabs } from './components/assets/AssetExtras'
import OntologyCompanionPane from './components/companion/OntologyCompanionPane'
import OntologyCompanionGraph from './components/companion/OntologyCompanionGraph'
import CompanionContentPane from './components/companion/CompanionContentPane'
import MineRelationsButton from './components/companion/MineRelationsButton'
import CompanionCheckupButton from './components/companion/CompanionCheckupButton'
import CompanionExportPane from './components/companion/CompanionExportPane'
import type { CompanionGraph } from '../../api/companion'
import EvolutionPane from './components/assets/EvolutionPane'
import RelationTypesPane from './components/assets/RelationTypesPane'
import AxiomsPane from './components/assets/AxiomsPane'
import DataPropertiesPane from './components/assets/DataPropertiesPane'
import AssetListPage, { AssetSearchBox, orderedGroups } from './components/assets/AssetList'
import QualityCardPane from './components/assets/QualityCardPane'
import ImportMergeWizard from './components/assets/ImportMergeWizard'
import ReferencesPane from './components/assets/ReferencesPane'
import ExternalRefsPane from './components/assets/ExternalRefsPane'
import HierarchyPane, { type OntoSelection } from './components/assets/HierarchyPane'

// ---------------------------------------------------------------------------
// 本体资产（AssetsPage，REQ-104 ③）：全部已构建本体统一管理
//   REQ-326③：列表/详情双态页级工作区——资产列表自左侧窄栏升格独占整页（搜索/来源
//   分组/校验徽标/质量雷达全宽承载），点击行进详情页级工作区（原 12 页签平铺改
//   「左锚点分区导航 + 右内容」，REQ-237，57 号 F1 整改）；返回列表链路不丢列表态
//   （搜索词提升）；跨模块深链 eino.onto.asset.focus（沿 eino.kb.focus 模式）带详情
//   语义；详情内上一条/下一条轻切换（分组序）；Maximizeable「最大化编辑」页内最大化
//   补丁随页级化退役（可视化 VizTabs 的保留——看图纯净全屏仍是有意语义）。
//   Spec 编辑常驻保活（切分区不丢未保存编辑），其余分区激活挂载（沿原 destroyOnHidden 行为）。
//   分区（REQ-458/M254 合并梳理：普通型 17 → 11——关系类型+数据属性 → 关系与属性〔分段〕/
//   公理+透传公理 → 公理〔分段〕/校验+质量卡 → 校验与质量/进化+影响分析 → 演化与影响/
//   被引用+产物+TTL 导出 → 引用与产物）：Spec 编辑 | 层级浏览 | 图形编辑 | 关系与属性 | 公理 |
//   CSV 灌装 ‖ 校验与质量 | 版本与源码 | 演化与影响 ‖ 可视化 | 引用与产物
//   正交红线：不出现任何引擎、端口、启停配置（运行看本体运行栏）
// B1（REQ-145/M22）：Spec 编辑/校验/产物/导出/选择条/重命名/可视化拆至 components/assets/。
// REQ-233/M60：资产列表搜索 + 空态 EmptyGuide 接入 + 详情「被引用」聚合区块 + 删除确认引用预检
// （running 方案引用服务端拦截 409、前端禁用；stopped/KB 引用警示放行——聚合源 GET /{id}/references）。
// ---------------------------------------------------------------------------

/** 跨模块深链 handoff（伴生卡 / 构建完成跳转等）：非空时进详情态并选中该本体（消费后置空）。 */
export const ASSET_FOCUS_KEY = 'eino.onto.asset.focus'

/** 详情工作区分区（三簇；57 号 F1 分组口径）。REQ-240 优化④：伴生分区仅伴生型本体注入。 */
function sectionGroups(isCompanion: boolean): { title: string; items: { key: string; label: string }[] }[] {
  // REQ-285①：详情模板分型——伴生型（对话生长容器）切换伴生语义分组，spec 系分区全部不出现
  // （空 spec 的 Spec 编辑/校验/质量卡/版本等分区对伴生型是空壳噪声）；普通型维持 spec 中心三分组。
  if (isCompanion) {
    return [
      {
        title: '对话沉淀',
        items: [
          { key: 'graph', label: '沉淀总览' },
          { key: 'companion', label: '伴生候选' },
          { key: 'companion-content', label: '内容清单' },
          { key: 'companion-export', label: '导出 TTL' },
        { key: 'community', label: '社区' },
        ],
      },
      { title: '引用', items: [{ key: 'references', label: '被引用' }] },
    ]
  }
  return [
    {
      title: '内容与编辑',
      items: [
        { key: 'spec', label: 'Spec 编辑' },
        // REQ-363/M163：层级浏览（Protégé 式树形导航）——兼作 52 号 P-5 详情页签分组演进首个落点
        { key: 'hierarchy', label: '层级浏览' },
        { key: 'graph-edit', label: '图形编辑' },
        // REQ-458：关系类型 + 数据属性（两个相邻小分区）→「关系与属性」，内部分段切换
        { key: 'decl', label: '关系与属性' },
        // REQ-458：Spec 公理 + 透传公理 →「公理」，内部分段切换
        { key: 'axioms', label: '公理' },
        { key: 'ingest', label: 'CSV 灌装' },
      ],
    },
    {
      title: '质量与演进',
      items: [
        // REQ-458：校验 + 质量卡 →「校验与质量」，纵向并置（先跑校验再看报告）
        { key: 'quality', label: '校验与质量' },
        { key: 'versions', label: '版本与源码' },
        // REQ-458：进化 + 影响分析 →「演化与影响」，纵向并置
        { key: 'evolution', label: '演化与影响' },
      ],
    },
    {
      title: '消费与引用',
      items: [
        { key: 'graph', label: '可视化' },
        // REQ-458：被引用 + 产物 + TTL 导出（三个相邻小分区）→「引用与产物」，纵向并置
        { key: 'outputs', label: '引用与产物' },
      ],
    },
  ]
}

export default function AssetsPage() {
  const { showToast } = useUI()
  const [tabKey, setTabKey] = useState<string | null>(
    // REQ-470：hash 分区直达（#/onto/assets/{id}/{section}）——惰性初始化（重置效应跳过首跑故可存活）
    () => (parseOntoHash()?.col === 'assets' ? parseOntoHash()!.section ?? null : null),
  )
  // REQ-458：合并分区内部分段视图（关系与属性 / 公理）——切本体回落默认段
  const [declView, setDeclView] = useState<'rel' | 'prop'>('rel')
  const [axView, setAxView] = useState<'spec' | 'raw'>('spec')
  // REQ-467/M267：本体清单单源 hook（7+ 处散拉收敛——创建/删除/重命名经 invalidateOntos 广播即时同步）
  const { ontos, err: listErr, reload: reloadOntoList } = useOntologies()
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  // REQ-326③：列表/详情双态（列表独占整页 ↔ 详情页级工作区）；深链 eino.onto.asset.focus 直接进详情
  // REQ-470：hash 直达（#/onto/assets/{id}/{section}）——一次性深链键优先级更高（跨模块 handoff 消费即清）
  const [view, setView] = useState<'list' | 'detail'>(() => {
    if (localStorage.getItem(ASSET_FOCUS_KEY)) return 'detail'
    const r = parseOntoHash()
    return r?.col === 'assets' && r.assetId ? 'detail' : 'list'
  })
  const [activeId, setActiveId] = useState<string | null>(
    () => localStorage.getItem(ASSET_FOCUS_KEY) ?? (parseOntoHash()?.col === 'assets' ? parseOntoHash()!.assetId ?? null : null),
  )
  // 列表态状态提升：返回列表不丢列表态（REQ-326③ 验收项）
  const [listQ, setListQ] = useState('')

  const [spec, setSpec] = useState<Spec | null>(null)
  const [specErr, setSpecErr] = useState<string | null>(null)
  const [specLoading, setSpecLoading] = useState(false)
  const [specTick, setSpecTick] = useState(0)
  // REQ-466/M266：Spec 编辑脏标记（SpecEditorPane 上报）——离开防护（栏位切换/返回列表/上下条）
  const [specDirty, setSpecDirty] = useState(false)
  // REQ-363 E/REQ-253①：跨视图选择单源（详情域总线）——层级浏览树/描述面板与可视化 2D
  // 共同消费：一处选中处处联动（59 号 P1 设计的实现形态=选择状态提升至详情页级单源）
  const [ontoSel, setOntoSel] = useState<OntoSelection | null>(null)

  const [validations, setValidations] = useState<Record<string, ValidationState>>({})
  const [mergeOpen, setMergeOpen] = useState(false) // REQ-157 导入合并向导
  const [renameOpen, setRenameOpen] = useState(false)
  const [publishName, setPublishName] = useState('') // REQ-239/M65 发布命名
  const [forkName, setForkName] = useState('')
  // REQ-240③/M66：资产卡能力雷达数据（缓存质量报告静默拉取；无报告不显示）
  // REQ-326③：列表态整页雷达——缓存扩为全量渐进拉取（详情态同口径 ensure）
  const [qualityCache, setQualityCache] = useState<Record<string, QualityReport | null>>({})
  /** REQ-467/M267：质量报告 ensure 单源（详情态选中与列表态渐进预取共用——原两份同逻辑收敛；
   *  缓存报告命中直存；无报告/读取失败 → save=false 静默快评兜底〔qualitygate 规则检查非 LLM、零副作用〕，
   *  失败置 null）——经 ref 供 effect 内消费，避免依赖数组膨胀 */
  const ensureQualityRef = useRef((id: string, alive: () => boolean): Promise<void> => {
    const set = (v: QualityReport | null) => {
      if (alive()) setQualityCache((m) => (m[id] === undefined ? { ...m, [id]: v } : m))
    }
    const quick = () =>
      api.qualityRun(id, false, false, false)
        .then((r2) => set(r2.report))
        .catch(() => set(null))
    return api
      .qualityReport(id)
      .then((r) => (r.report ? set(r.report) : quick()))
      .catch(quick)
  })
  const [forkBusy, setForkBusy] = useState(false)
  const [forkErr, setForkErr] = useState<string | null>(null)

  // 深链消费（挂载一次）：handoff 置空，选中即进详情；hash 直达带分区（REQ-470）
  useEffect(() => {
    const f = localStorage.getItem(ASSET_FOCUS_KEY)
    if (f) {
      localStorage.removeItem(ASSET_FOCUS_KEY)
      setActiveId(f)
      setView('detail')
      return
    }
    const r = parseOntoHash()
    if (r?.col === 'assets' && r.assetId && r.section) setTabKey(r.section) // 惰性初始化兜底（深链键路径等异步置入场景）
  }, [])

  // REQ-470：状态→hash（replace 细化不灌栈；页级跳转的 push 在 openAsset/backToList 处理器内）
  useEffect(() => {
    const cur = parseOntoHash()
    if (cur && cur.col !== 'assets') return // 防御：他栏路由不抢写
    writeOntoHash(
      {
        col: 'assets',
        assetId: view === 'detail' && activeId ? activeId : undefined,
        section: view === 'detail' && tabKey ? tabKey : undefined,
      },
      true,
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps -- 仅三态联动写 hash；secKey 派生默认段不单列依赖
  }, [view, activeId, tabKey])

  // REQ-470：后退/前进按 hash 恢复（脏态经 leaveGuard 确认——取消则回写当前状态 hash 保持 URL 一致）
  useEffect(() => {
    const stateRoute = () => ({
      col: 'assets' as const,
      assetId: view === 'detail' && activeId ? activeId : undefined,
      section: view === 'detail' && tabKey ? tabKey : undefined,
    })
    const onPop = () => {
      const r = parseOntoHash()
      if (!r || r.col !== 'assets') return
      const apply = () => {
        if (r.assetId) {
          if (r.assetId !== activeId) setActiveId(r.assetId)
          setView('detail')
          setTabKey(r.section ?? null)
        } else {
          setView('list')
        }
      }
      guardLeave(apply, () => writeOntoHash(stateRoute(), true))
    }
    window.addEventListener('popstate', onPop)
    window.addEventListener('hashchange', onPop) // REQ-470：同文档 hash 导航同应用（脏态同经 leaveGuard）
    return () => {
      window.removeEventListener('popstate', onPop)
      window.removeEventListener('hashchange', onPop)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- popstate 闭包取最新三态
  }, [view, activeId, tabKey])

  // REQ-467：单源薄包装（保持既有调用点签名；需选中新 id 的场景等清单到位再选中——防详情回落列表竞态）
  const reloadOntos = (selectId?: string) => {
    if (selectId) reloadOntoList().then(() => setActiveId(selectId)).catch(() => {})
    else reloadOntoList()
  }

  // REQ-285⑤：切换本体重置分区——分型后两套模板键不相交，残留键（如 outputs）落空渲染
  // REQ-363：切本体同步清空跨视图选择（旧实体身份不跨资产）
  // REQ-458：合并分区内部分段视图同步回落（原两处 setTabKey(null) effect 并此一处）
  // REQ-470：前值 ref 比对替代无脑重置——挂载首跑（含 StrictMode 双跑）不重置，hash 惰性初始化的分区直达可存活
  const prevActiveRef = useRef<string | null>(activeId)
  useEffect(() => {
    if (prevActiveRef.current === activeId) return
    prevActiveRef.current = activeId
    setTabKey(null)
    setOntoSel(null)
    setDeclView('rel')
    setAxView('spec')
  }, [activeId])

  // REQ-466/M266：详情态 Spec 未保存 → 挂离开防护（OntologyModule 栏位切换与页内返回/上下条共用）
  useEffect(() => {
    if (view === 'detail' && specDirty) {
      setLeaveGuard('asset-spec-dirty', '「Spec 编辑」分区有未保存的本地修改，离开（切栏位/回列表/切本体）会丢失。可先保存，或复制备份后再离开。')
    } else {
      clearLeaveGuard('asset-spec-dirty')
    }
    return () => clearLeaveGuard('asset-spec-dirty')
  }, [view, specDirty])

  // REQ-216⑦：伴生绑定本体清单（详情页「伴生候选」页签「对话生长」徽标判定）
  const [boundIds, setBoundIds] = useState<Set<string>>(new Set())
  const reloadBound = () => {
    companionApi
      .boundOntologies()
      .then((r) => setBoundIds(new Set(r.ontology_ids ?? [])))
      .catch(() => setBoundIds(new Set()))
  }

  // REQ-285②：伴生型详情头数据状态（伴生图计数/绑定者/待确认候选）——派生与加载在 active 声明后
  const [compGraph, setCompGraph] = useState<CompanionGraph | null>(null)
  const reloadCompGraphRef = useRef<() => void>(() => {}) // REQ-286 C1：编辑后重拉伴生图
  const [compAgents, setCompAgents] = useState<Agent[]>([])
  const [compPending, setCompPending] = useState(0)
  const reloadProfiles = () => {
    api
      .listRuntimeProfiles()
      .then(setProfiles)
      .catch(() => setProfiles([]))
  }

  useEffect(() => {
    reloadOntos()
    reloadProfiles()
    reloadBound()
  }, [])

  // 选中本体 → 拉取 Spec（404 视为尚未保存，不算错误）
  // REQ-285：伴生容器无 spec 数据面——分型后跳过 spec/质量拉取（404 控制台噪声与空跑评分一并消除）
  useEffect(() => {
    if (!activeId) {
      setSpec(null)
      setSpecErr(null)
      return
    }
    if (boundIds.has(activeId)) {
      setSpec(null)
      setSpecErr(null)
      setSpecLoading(false)
      return
    }
    let alive = true
    setSpecLoading(true)
    setSpecErr(null)
    api
      .getSpec(activeId)
      .then((s) => {
        if (alive) setSpec(s)
      })
      .catch((e: any) => {
        if (!alive) return
        setSpec(null)
        if (!(e instanceof ApiError && e.status === 404)) setSpecErr(e?.message ?? '加载失败')
      })
      .finally(() => {
        if (alive) setSpecLoading(false)
      })
    // REQ-240③/M66 + REQ-264：能力雷达全量支持——先读缓存报告；未生成过则
    // save=false 静默跑分（qualitygate 规则检查非 LLM、零副作用不落产物），详情雷达人人有
    if (qualityCache[activeId] === undefined) ensureQualityRef.current(activeId, () => alive)
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- boundIds 到位后重跑一次即被伴生守卫拦截
  }, [activeId, specTick, boundIds])

  const active = useMemo(() => ontos.find((o) => o.id === activeId) ?? null, [ontos, activeId])

  // REQ-326③：详情意图但本体不存在（深链失效/被删）→ 回落列表态
  useEffect(() => {
    if (view === 'detail' && ontos.length > 0 && !active) setView('list')
  }, [view, ontos, active])

  // REQ-326③：列表态质量雷达渐进预取（并发 3；缓存命中跳过；失败静默 null 与详情态同口径）
  useEffect(() => {
    if (view !== 'list') return
    let alive = true
    const pending = ontos.filter((o) => qualityCache[o.id] === undefined).map((o) => o.id)
    let cursor = 0
    // REQ-467：ensure 单源复用（完成即 pump 维持恒定并发 3）
    const ensureOne = (id: string) => {
      ensureQualityRef.current(id, () => alive).finally(() => {
        if (alive) pump()
      })
    }
    const pump = () => {
      if (alive && cursor < pending.length) ensureOne(pending[cursor++]) // 每次完成补发一个：恒定并发 3
    }
    // 启动并发窗 3 路
    pump()
    pump()
    pump()
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- 渐进预取：仅列表态触发，缓存命中由 ensureOne 内部判断
  }, [view, ontos])

  // REQ-326③：详情内上一条/下一条（与列表同一排序源：分组序 + 组内既有排序）
  const flatIds = useMemo(
    () => orderedGroups(ontos, boundIds, '').flatMap(([, items]) => items.map((o) => o.id)),
    [ontos, boundIds],
  )
  const curIdx = activeId ? flatIds.indexOf(activeId) : -1
  const prevId = curIdx > 0 ? flatIds[curIdx - 1] : null
  const nextId = curIdx >= 0 && curIdx < flatIds.length - 1 ? flatIds[curIdx + 1] : null
  const jumpTo = (id: string | null) => {
    if (!id) return
    guardLeave(() => {
      // REQ-466：脏态切本体先确认（Spec 编辑常驻保活不护跨本体切换）；REQ-470：相邻切换 replace 不灌栈
      writeOntoHash({ col: 'assets', assetId: id }, true)
      setActiveId(id)
      setTabKey(null)
    })
  }

  // REQ-285②：伴生型详情头数据加载（伴生图计数/绑定者/待确认候选）——分型模板统计行数据源；
  // 经首个绑定 agent 解析伴生子图（多 agent 共享同一子图，任一视角等价，与成长图同口径）
  const isCompActive = !!(active && boundIds.has(active.id))
  useEffect(() => {
    if (!activeId || !isCompActive) {
      setCompGraph(null)
      setCompAgents([])
      setCompPending(0)
      return
    }
    let alive = true
    reloadCompGraphRef.current = () => {
      companionApi
        .listOntologyAgents(activeId)
        .then(async (ls) => {
          if (!alive) return
          setCompAgents(ls ?? [])
          if (ls && ls.length > 0) {
            try {
              const g = await companionApi.graph(ls[0].id)
              if (alive) setCompGraph(g)
            } catch { /* 引擎不可达：统计行诚实按 0 计 */ }
          }
        })
        .catch(() => {})
    }
    reloadCompGraphRef.current()
    companionApi
      .listCandidatesByOntology(activeId, 'pending')
      .then((ls) => {
        if (alive) setCompPending(ls?.length ?? 0)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [activeId, isCompActive])
  const validation = activeId ? validations[activeId] : undefined
  // 详情分区键（REQ-230② 规则保留：伴生型本体默认打开「伴生候选」分区，其余默认 Spec 编辑）
  // REQ-285⑤：伴生型默认分区=沉淀总览（REQ-230②「默认候选」口径变更——分型后总览为先）
  const secKey = tabKey ?? (active && boundIds.has(active.id) ? 'graph' : 'spec')

  /** 被 N 套方案引用（只读徽标，增强正交可见性） */
  const refCount = (o: Ontology) => profiles.filter((p) => (p.ontology_ids ?? []).includes(o.id)).length

  // REQ-233③：删除确认引用预检（打开确认框时拉一次三源聚合；running/伴生绑定=服务端将 409，前端禁用删除）
  const [delRefs, setDelRefs] = useState<OntologyReferences | null>(null)
  const delRunning = (delRefs?.runtime_plans ?? []).filter((p) => p.status === 'running')
  const delStopped = (delRefs?.runtime_plans ?? []).filter((p) => p.status !== 'running')
  const delBlocking = delRunning.length > 0 || (delRefs?.companion_agents.length ?? 0) > 0

  const removeActive = async () => {
    if (!active) return
    try {
      await api.deleteOntology(active.id)
      showToast('已删除本体')
      setActiveId(null)
      setView('list') // REQ-326③：删除后回列表
      reloadOntos()
      reloadProfiles()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  /** REQ-83：fork 为独立新本体（forked_from=源 id，version 重置 1），并选中新本体 */
  const doFork = async () => {
    if (!active) return
    setForkBusy(true)
    setForkErr(null)
    try {
      const o = await api.forkOntology(active.id, forkName.trim() ? { name: forkName.trim() } : {})
      showToast(`已 Fork 为「${o.name}」`)
      setForkName('')
      reloadOntos(o.id)
      reloadProfiles()
    } catch (e: any) {
      setForkErr(e?.message ?? 'Fork 失败')
    } finally {
      setForkBusy(false)
    }
  }

  const refreshAfterSave = () => {
    reloadOntos()
    setSpecTick((t) => t + 1)
    // 内容已变：能力雷达缓存失效（移除键 → 下次选中重拉）
    if (activeId) setQualityCache(({ [activeId]: _drop, ...rest }) => rest)
  }

  const groups = sectionGroups(!!active && boundIds.has(active.id))

  /** REQ-326③：点行进详情页级工作区 */
  const openAsset = (id: string) => {
    guardLeave(() => {
      // REQ-470：列表→详情进历史栈（后退回列表）
      writeOntoHash({ col: 'assets', assetId: id })
      setActiveId(id)
      setTabKey(null)
      setView('detail')
    })
  }
  const backToList = () =>
    guardLeave(() => {
      // REQ-466：Spec 编辑常驻保活不护回列表卸载——脏态先确认；REQ-470：详情→列表进历史栈
      writeOntoHash({ col: 'assets' })
      setView('list')
    })

  // REQ-326③：列表态——独占整页（搜索/来源分组/校验徽标/质量雷达全宽承载）
  const renderList = () => (
    <div className="work-main">
      <div className="work-head">
        <div className="work-head-text">
          <div className="work-head-title">
            <Typography.Title level={4} style={{ margin: 0 }}>
              本体资产
            </Typography.Title>
            <Tag color="blue" style={{ margin: 0 }}>统一管理</Tag>
            <Tag style={{ margin: 0 }}>{ontos.length}</Tag>
          </div>
          <p className="work-head-desc">
            已构建本体的统一仓库（REQ-60）：新建请到「本体构建」选择路径；点击本体进详情工作区，负责编辑、校验、版本、产物、可视化与 TTL 导出。
          </p>
        </div>
      </div>
      <div className="work-toolbar">
        <AssetSearchBox q={listQ} onQ={setListQ} />
      </div>
      {listErr ? (
        <div className="work-empty">
          <Result
            status="warning"
            title="本体平面服务未启动（BUILD_SVC_URL/:8091）"
            subTitle={listErr}
            extra={<Button onClick={() => reloadOntos()}>重试</Button>}
          />
        </div>
      ) : (
        <AssetListPage
          ontos={ontos}
          profiles={profiles}
          q={listQ}
          onOpen={openAsset}
          validations={validations}
          qualityCache={qualityCache}
          bound={boundIds}
        />
      )}
    </div>
  )

  // REQ-326③：详情态——页级工作区（返回列表 + 面包屑 + 上一条/下一条轻切换）
  const renderDetail = () => {
    if (!active) return null // 调用侧已保证 view==='detail' && active；此处兼作 TS 窄化
    return (
    <>
      {/* 界面优化批（2026-10-16）：原 detail-topbar 与 work-head 两段合并压缩——行1=返回+面包屑+名称+徽标（右：位置/上下条），
          行2=描述·元信息一行（右：动作按钮 small 化）；头部雷达移除（「质量卡」分区已有 186px 大图），关键内容区不再被挤压 */}
      <div className="work-head" style={{ flexWrap: 'wrap', rowGap: 6, alignItems: 'center' }} data-testid="asset-detail-topbar">
        <div className="work-head-text" style={{ minWidth: 0, flex: 1 }}>
          <div className="work-head-title">
            <Button size="small" icon={<ArrowLeftOutlined />} onClick={backToList} data-testid="asset-back">
              返回列表
            </Button>
            <span className="detail-crumb" style={{ flex: 'none' }}>
              本体资产 <span className="detail-crumb-sep">/</span>
            </span>
            <Typography.Title level={4} style={{ margin: 0 }}>
              {active.name}
            </Typography.Title>
            <Tag color="geekblue" style={{ margin: 0 }}>v{active.version ?? '—'}</Tag>
            {/* REQ-239/M65：发布态徽标（published=命名快照终态；draft=可编辑工作态） */}
            {active.status === 'published' ? (
              <Tooltip title={`已发布命名版本「${active.version_name || `v${active.version}`}」——内容变更将自动回 draft`}>
                <Tag color="green" style={{ margin: 0 }}>Published{active.version_name ? ` · ${active.version_name}` : ''}</Tag>
              </Tooltip>
            ) : (
              <Tag style={{ margin: 0 }}>Draft</Tag>
            )}
            <Tag color={sourceTag(active).color} style={{ margin: 0 }}>{sourceTag(active).text}</Tag>
            {/* REQ-253④（59 号 P4）：表达性签名徽标（规则映射非推理器） */}
            {(() => {
              const ex = expressivityOf(spec)
              return ex.sig ? (
                <Tooltip title={`表达性签名（简化 DL 记号）：${ex.feats.join('、')}`}>
                  <Tag color="cyan" style={{ margin: 0, fontFamily: 'monospace' }} data-testid="expressivity-tag">{ex.sig}</Tag>
                </Tooltip>
              ) : null
            })()}
            <span style={{ flex: 1 }} />
            <span className="detail-nav-pos">
              {curIdx + 1} / {flatIds.length}
            </span>
            <Tooltip title={prevId ? `上一条：${ontos.find((o) => o.id === prevId)?.name ?? ''}` : '已是第一个'}>
              <Button size="small" icon={<LeftOutlined />} disabled={!prevId} onClick={() => jumpTo(prevId)} />
            </Tooltip>
            <Tooltip title={nextId ? `下一条：${ontos.find((o) => o.id === nextId)?.name ?? ''}` : '已是最后一个'}>
              <Button size="small" icon={<RightOutlined />} disabled={!nextId} onClick={() => jumpTo(nextId)} />
            </Tooltip>
          </div>
          {/* 行2：描述（截断）与规模/引用元信息并一行——头部高度收敛 */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '4px 0 0', overflow: 'hidden', whiteSpace: 'nowrap' }}>
            <Tooltip title={active.description || undefined} placement="topLeft">
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 460, color: 'var(--c-ink-2)', fontSize: 13 }}>
                {active.description || '未填写描述'}
              </span>
            </Tooltip>
            <span style={{ color: 'var(--c-ink-3)' }}>·</span>
            {isCompActive ? (
              <>
                {/* REQ-285②：伴生型统计行=伴生图计数（spec 恒空，n_concepts 恒 0 是误导） */}
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>概念 {compGraph?.nodes.filter((n) => n.kind === 'Concept').length ?? 0}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>事件 {compGraph?.nodes.filter((n) => n.kind === 'Event').length ?? 0}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>关系 {compGraph?.edges.length ?? 0}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>待确认候选 {compPending}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <Tooltip title={compAgents.length ? `沉淀者（绑定该容器的智能体）：${compAgents.map((a) => a.name).join('、')}` : '无智能体绑定'}>
                  <span style={{ cursor: 'default', fontSize: 12, color: 'var(--c-ink-2)' }}>沉淀者 {compAgents.length}</span>
                </Tooltip>
              </>
            ) : (
              <>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>概念 {active.n_concepts ?? 0}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>关系 {active.n_relations ?? 0}</span>
                <span style={{ color: 'var(--c-ink-3)' }}>·</span>
                <span style={{ fontSize: 12, color: 'var(--c-ink-2)' }}>实例 {active.n_instances ?? 0}</span>
              </>
            )}
            <span style={{ color: 'var(--c-ink-3)' }}>·</span>
            <Tooltip title="被 N 套运行方案引用（只读；启停操作在「本体运行」栏）">
              <span style={{ cursor: 'default', fontSize: 12, color: 'var(--c-ink-2)' }}>被 {refCount(active)} 套方案引用</span>
            </Tooltip>
            <span style={{ flex: 1 }} />
            {/* 动作按钮（small 化；雷达见「质量卡」分区 186px 大图） */}
            <Space size={4} wrap style={{ flex: 'none' }}>
            {!isCompActive && (active.status === 'published' ? (
              <Popconfirm
                icon={null}
                title="撤回发布？"
                description="本体回 draft 工作态，命名版本随之清除（版本历史保留）。运行方案下次启动装载时将给出 draft 警示。"
                okText="撤回发布"
                cancelText="取消"
                onConfirm={async () => {
                  try {
                    await api.unpublishOntology(active.id)
                    showToast('已撤回发布（draft）')
                    reloadOntos()
                  } catch (e: any) {
                    showToast(e.message, 'err')
                  }
                }}
              >
                <Button size="small" icon={<CloudDownloadOutlined />}>撤回发布</Button>
              </Popconfirm>
            ) : (
              <Popconfirm
                icon={null}
                title={`发布 v${active.version ?? '—'} 为命名版本`}
                description={
                  <Space direction="vertical" style={{ width: 300 }} size={8}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      发布即快照终态：运行向导优先装载已发布版本；后续编辑将自动回 draft，历史快照可恢复为新版本。
                    </Typography.Text>
                    <Input
                      value={publishName}
                      onChange={(e) => setPublishName(e.target.value)}
                      placeholder={`命名（可选，默认 v${active.version ?? '—'}，如 v${active.version ?? '1'}-k8s-baseline）`}
                    />
                  </Space>
                }
                okText="发布"
                cancelText="取消"
                onOpenChange={(o) => {
                  if (o) setPublishName('')
                }}
                onConfirm={async () => {
                  if (!active) return
                  try {
                    const o = await api.publishOntology(active.id, publishName.trim())
                    showToast(`已发布${o.version_name ? ` · ${o.version_name}` : ''}`)
                    reloadOntos()
                  } catch (e: any) {
                    showToast(e.message, 'err')
                  }
                }}
              >
                <Button size="small" type="primary" ghost icon={<CloudUploadOutlined />}>发布</Button>
              </Popconfirm>
            ))}
            {!isCompActive && (
            <Popconfirm
              icon={null}
              title="Fork 为新本体"
              description={
                <Space direction="vertical" style={{ width: 300 }} size={8}>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    复制该本体的全部 Spec 与产物为新本体（forked_from 记录来源，版本重置为 1）。
                  </Typography.Text>
                  <Input
                    value={forkName}
                    onChange={(e) => setForkName(e.target.value)}
                    placeholder={`新名称（可选，默认「${active.name} 副本」）`}
                  />
                </Space>
              }
              okText="Fork"
              cancelText="取消"
              okButtonProps={{ loading: forkBusy }}
              onOpenChange={(o) => {
                if (o) {
                  setForkName('')
                  setForkErr(null)
                }
              }}
              onConfirm={doFork}
            >
              <Button size="small" icon={<BranchesOutlined />}>Fork 本体</Button>
            </Popconfirm>
            )}
            {/* REQ-301 口径修订：重命名对伴生型恢复（name 为表级展示属性，改名不影响绑定/伴生子图；
                宿主方案名保持创建时快照）——导入合并维持伴生隐藏（spec 恒空壳） */}
            <Button size="small" icon={<EditOutlined />} onClick={() => setRenameOpen(true)}>
              重命名
            </Button>
            {!isCompActive && (
            <Button size="small" icon={<ImportOutlined />} onClick={() => setMergeOpen(true)}>
              导入合并
            </Button>
            )}
            {/* REQ-339：演化动作——携本体 id 跳转 OntoExtend 流程（ODP 扩展/词表补全，buildPath 透传） */}
            {!isCompActive && (
            <Button
              size="small"
              icon={<RocketOutlined />}
              data-testid="asset-evolve-btn"
              onClick={() =>
                guardLeave(() => {
                  // REQ-466：演化跳构建栏会卸载详情（含未保存 Spec）——脏态先确认
                  localStorage.setItem('eino.onto.buildPath', 'ontoextend')
                  localStorage.setItem('eino.onto.asset.extend', active.id)
                  localStorage.setItem(ONTO_SIDEBAR_KEY, 'build')
                  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
                })
              }
            >
              演化
            </Button>
            )}
            <Popconfirm
              title={`删除本体「${active.name}」？`}
              description={
                <Space direction="vertical" size={4} style={{ maxWidth: 320 }}>
                  <span style={{ fontSize: 12 }}>级联删除其 Spec、产物与引用；已启动的运行方案不受影响（REQ-87）。</span>
                  {delRunning.length > 0 && (
                    <span style={{ fontSize: 12, color: '#cf1322' }}>
                      被 {delRunning.length} 个运行中方案挂载（{delRunning.map((p) => p.name || p.id).join('、')}）——须先停止再删除。
                    </span>
                  )}
                  {(delRefs?.companion_agents.length ?? 0) > 0 && (
                    <span style={{ fontSize: 12, color: '#cf1322' }}>
                      被 {delRefs!.companion_agents.length} 个智能体绑定为伴生归属——须先解绑/换绑。
                    </span>
                  )}
                  {delStopped.length > 0 && (
                    <span style={{ fontSize: 12, color: '#d46b08' }}>
                      仍被 {delStopped.length} 个未运行方案配置引用，删除后这些方案启动将失败。
                    </span>
                  )}
                  {(delRefs?.kb_vocabs.length ?? 0) > 0 && (
                    <span style={{ fontSize: 12, color: '#d46b08' }}>
                      仍被 {delRefs!.kb_vocabs.length} 个知识库用作约束词表，删除后其 KG 约束抽取将降级为自由抽取。
                    </span>
                  )}
                </Space>
              }
              okText="删除"
              okButtonProps={{ danger: true, disabled: delBlocking }}
              cancelText="取消"
              onOpenChange={(o) => {
                if (o && active) {
                  api
                    .ontologyReferences(active.id)
                    .then(setDelRefs)
                    .catch(() => setDelRefs(null))
                } else {
                  setDelRefs(null)
                }
              }}
              onConfirm={removeActive}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
            </Space>
          </div>
        </div>
      </div>

          {mergeOpen && active && (
            <ImportMergeWizard
              open={mergeOpen}
              onClose={() => setMergeOpen(false)}
              ontologyId={active.id}
              targetSpecText={spec ? JSON.stringify(spec, null, 2) : ''}
              onApplied={() => {
                reloadOntos(active.id)
                setSpecTick((t) => t + 1)
              }}
              onNeedEdit={() => setTabKey('graph-edit')} // REQ-235/H5：有损导入补录直达图形编辑
            />
          )}

          {forkErr && (
            <Alert type="error" showIcon closable title="Fork 失败" description={forkErr} onClose={() => setForkErr(null)} />
          )}

          <Card className="work-card onto-stage-card" size="small">
            <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
              {/* 左锚点分区导航（REQ-183 同款范式：sticky + 常显标签；三簇分组）
                  REQ-253 bugfix：分区多且竖排高于视口时 sticky 底部不可达——限高 + 纵向滚动 */}
              <nav
                aria-label="资产详情分区导航"
                style={{
                  width: 132,
                  flexShrink: 0,
                  position: 'sticky',
                  top: 8,
                  maxHeight: 'calc(100vh - 16px)',
                  overflowY: 'auto',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 10,
                }}
              >
                {groups.map((g) => (
                  <div key={g.title} style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    <span style={{ fontSize: 11, color: 'var(--c-ink-3)', padding: '0 4px' }}>{g.title}</span>
                    {g.items.map((it) => (
                      <button
                        key={it.key}
                        type="button"
                        aria-current={secKey === it.key || undefined}
                        className={`onto-engine-item${secKey === it.key ? ' active' : ''}`}
                        style={{ textAlign: 'left', width: '100%' }}
                        onClick={() => setTabKey(it.key)}
                      >
                        <span className="onto-engine-top">
                          <span className="onto-engine-label">{it.label}</span>
                          {it.key === 'companion' && boundIds.has(active.id) && (
                            <Tooltip title="对话生长：有智能体绑定该本体为伴生归属（REQ-216）">
                              <Tag color="geekblue" style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
                                对话生长
                              </Tag>
                            </Tooltip>
                          )}
                        </span>
                      </button>
                    ))}
                  </div>
                ))}
              </nav>

              {/* 右内容区：Spec 编辑常驻保活（切分区不丢未保存编辑），其余激活挂载 */}
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: secKey === 'spec' ? 'block' : 'none' }}>
                  <SpecEditorPane
                    ontology={active}
                    spec={spec}
                    specLoading={specLoading}
                    specErr={specErr}
                    onReloadSpec={() => setSpecTick((t) => t + 1)}
                    onMetaSaved={() => reloadOntos()}
                    onSpecSaved={refreshAfterSave}
                    onDirtyChange={setSpecDirty}
                  />
                </div>
                {/* REQ-458：校验 + 质量卡 合并为「校验与质量」——先跑校验再看质量报告，纵向并置 */}
                {secKey === 'quality' && (
                  <>
                    <div className="onto-sec" style={{ marginTop: 0 }}>
                      <span className="onto-sec-title">结构校验（JSON Schema + 引用完整性）</span>
                    </div>
                    <ValidatePane
                      ontologyId={active.id}
                      result={validation ?? null}
                      onResult={(r) => setValidations((v) => ({ ...v, [active.id]: r }))}
                    />
                    <div className="onto-sec" style={{ marginTop: 18 }}>
                      <span className="onto-sec-title">质量卡（评估报告与 strict 门禁）</span>
                    </div>
                    <QualityCardPane
                      ontologyId={active.id}
                      onReport={(r) => setQualityCache((m) => ({ ...m, [active.id]: r }))}
                      onGoGraphEdit={() => setTabKey('graph-edit')} // REQ-331②：error finding 定位跳转（沿 REQ-235 onNeedEdit 先例）
                    />
                  </>
                )}
                {secKey === 'versions' && <SourceView ontologyId={active.id} currentVersion={active.version} spec={spec} onRestored={refreshAfterSave} />}
                {secKey === 'graph' && (isCompActive
                  // REQ-285①：伴生型「沉淀总览」直挂伴生成长图（spec 2D/3D/WebVOWL 对空 spec 无意义）
                  // REQ-297 §十九：图级操作入口归位——「挖掘关系」按钮+内联进度在此（完成自动刷新）
                  ? (
                    <div>
                      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginBottom: 8 }}>
                        <MineRelationsButton ontologyId={active.id} onDone={() => reloadCompGraphRef.current()} />
                        <CompanionCheckupButton ontologyId={active.id} onDone={() => reloadCompGraphRef.current()} />
                      </div>
                      <OntologyCompanionGraph key={active.id} ontologyId={active.id} />
                    </div>
                  )
                  : <VizTabs spec={spec} ontologyId={active.id} busFocus={ontoSel} onBusSelect={setOntoSel} />)}
                {/* REQ-363/M163：层级浏览（Protégé 式树形导航；选择总线经 ontoSel 单源联动可视化 2D） */}
                {secKey === 'hierarchy' && !isCompActive && (
                  <HierarchyPane ontology={active} spec={spec} selection={ontoSel} onSelect={setOntoSel} onSaved={refreshAfterSave} />
                )}
                {secKey === 'companion-content' && <CompanionContentPane graph={compGraph} ontologyId={active.id} onChanged={() => reloadCompGraphRef.current()} />}
                {secKey === 'companion-export' && <CompanionExportPane key={active.id} ontologyId={active.id} />}
                {secKey === 'community' && isCompActive && <CommunityPane key={active.id} ontologyId={active.id} />}
                {/* REQ-458：进化 + 影响分析 合并为「演化与影响」——候选裁决与变更波及同区，纵向并置 */}
                {secKey === 'evolution' && (
                  <>
                    <div className="onto-sec" style={{ marginTop: 0 }}>
                      <span className="onto-sec-title">进化候选（诊断 → 配对门控 → 采纳）</span>
                    </div>
                    <EvolutionPane key={active.id} ontologyId={active.id} />
                    <div className="onto-sec" style={{ marginTop: 18 }}>
                      <span className="onto-sec-title">变更影响分析（版本 diff × 引用波及）</span>
                    </div>
                    <ImpactPane ontology={active} />
                  </>
                )}
                {secKey === 'companion' && <OntologyCompanionPane key={active.id} ontologyId={active.id} />}
                {/* 伴生型模板的「被引用」独立分区（普通型已并入「引用与产物」，REQ-458） */}
                {secKey === 'references' && <ReferencesPane key={active.id} ontologyId={active.id} />}
                {/* REQ-458：被引用 + 产物 + TTL 导出 合并为「引用与产物」——三段纵向并置 */}
                {secKey === 'outputs' && !isCompActive && (
                  <>
                    <div className="onto-sec" style={{ marginTop: 0 }}>
                      <span className="onto-sec-title">被引用（运行方案 / KB 词表 / 伴生绑定）</span>
                    </div>
                    <ReferencesPane key={active.id} ontologyId={active.id} />
                    {/* REQ-262/M256：关联本体（跨本体引用登记，external_refs 单源）——被引用之后纵向并置 */}
                    <div style={{ marginTop: 18 }}>
                      <ExternalRefsPane
                        ontologyId={active.id}
                        concepts={(spec?.concepts ?? []).map((c: any) => c.name)}
                        ontos={ontos.map((o) => ({ id: o.id, name: o.name }))}
                      />
                    </div>
                    <div style={{ marginTop: 18 }}>
                      <ArtifactsPane ontologyId={active.id} />
                    </div>
                    <div style={{ marginTop: 18 }}>
                      <ExportPane ontology={active} />
                    </div>
                  </>
                )}
                {secKey === 'graph-edit' && (
                  // REQ-326③：Maximizeable「最大化编辑」页内最大化补丁退役——详情已升页级工作区
                  <GraphEditor ontologyId={active.id} spec={spec} onSpecSaved={refreshAfterSave} baseVersion={active.version} />
                )}
                {/* REQ-458：关系类型 + 数据属性 合并为「关系与属性」——分段切换两个声明编辑器 */}
                {secKey === 'decl' && (
                  <>
                    <Segmented
                      value={declView}
                      onChange={(v) => setDeclView(v as 'rel' | 'prop')}
                      options={[
                        { value: 'rel', label: '关系类型' },
                        { value: 'prop', label: '数据属性' },
                      ]}
                      style={{ marginBottom: 12 }}
                      data-testid="decl-segmented"
                    />
                    {declView === 'rel' ? (
                      <RelationTypesPane spec={spec} ontologyId={active.id} onSaved={refreshAfterSave} />
                    ) : (
                      <DataPropertiesPane spec={spec} ontologyId={active.id} onSaved={refreshAfterSave} />
                    )}
                  </>
                )}
                {/* REQ-458：Spec 公理 + 透传公理 合并为「公理」——分段切换（建模声明 / 导入存档） */}
                {secKey === 'axioms' && (
                  <>
                    <Segmented
                      value={axView}
                      onChange={(v) => setAxView(v as 'spec' | 'raw')}
                      options={[
                        { value: 'spec', label: 'Spec 公理（建模声明）' },
                        { value: 'raw', label: '透传公理（导入存档）' },
                      ]}
                      style={{ marginBottom: 12 }}
                      data-testid="axioms-segmented"
                    />
                    {axView === 'spec' ? (
                      <AxiomsPane spec={spec} />
                    ) : (
                      <AxiomsRawPane key={active.id} ontologyId={active.id} />
                    )}
                  </>
                )}
                {secKey === 'ingest' && (
                  <CsvIngestPane ontologyId={active.id} spec={spec} onIngested={() => refreshAfterSave()} />
                )}
              </div>
            </div>
          </Card>

      {renameOpen && active && (
        <RenameModal
          ontology={active}
          onClose={() => setRenameOpen(false)}
          onSaved={() => {
            setRenameOpen(false)
            reloadOntos()
          }}
        />
      )}
    </>
    )
  }

  // REQ-326③：列表/详情双态页级渲染（列表独占整页 ↔ 详情页级工作区；过渡渲染空等待数据/
  // 深链失效由回落 effect 切回列表态）
  return (
    <div className="main">
      {view === 'detail' && active ? (
        <div className="content-panel asset-detail-scroll" style={{ flex: 1, minWidth: 0, height: '100%', overflowY: 'auto' }}>
          {renderDetail()}
        </div>
      ) : view === 'list' ? (
        <div className="content-panel" style={{ flex: 1, minWidth: 0, height: '100%', overflowY: 'auto' }}>{renderList()}</div>
      ) : null}
    </div>
  )
}
