import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Card, Input, Popconfirm, Result, Space, Splitter, Tag, Tooltip, Typography } from 'antd'
import { BranchesOutlined, CloudDownloadOutlined, CloudUploadOutlined, DeleteOutlined, EditOutlined, ImportOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../api/client'
import type { QualityReport } from '../../api/client'
import { companionApi } from '../../api/companion'
import type { Ontology, OntologyReferences, RuntimeProfile, Spec } from '../../api/types'
import QualityRadar, { radarDimsOf } from '../../components/QualityRadar'
import { expressivityOf } from './shared/expressivity'
import { useUI } from '../../store/ui'
import { sourceTag, type ValidationState } from './shared'
import CsvIngestPane from './components/CsvIngestPane'
import GraphEditor from './components/GraphEditor'
import SourceView from './components/SourceView'
import SpecEditorPane from './components/assets/SpecEditorPane'
import { ArtifactsPane, ExportPane, ValidatePane } from './components/assets/AssetPanes'
import { RenameModal, VizTabs } from './components/assets/AssetExtras'
import OntologyCompanionPane from './components/companion/OntologyCompanionPane'
import EvolutionPane from './components/assets/EvolutionPane'
import RelationTypesPane from './components/assets/RelationTypesPane'
import AxiomsPane from './components/assets/AxiomsPane'
import DataPropertiesPane from './components/assets/DataPropertiesPane'
import AssetList from './components/assets/AssetList'
import QualityCardPane from './components/assets/QualityCardPane'
import ImportMergeWizard from './components/assets/ImportMergeWizard'
import ReferencesPane from './components/assets/ReferencesPane'
import EmptyGuide from '../../components/EmptyGuide'
import Maximizeable from '../../components/Maximizeable'

// ---------------------------------------------------------------------------
// 本体资产（AssetsPage，REQ-104 ③）：全部已构建本体统一管理
//   REQ-181 v2 重构：平台统一「左列表（来源分组）+ 右主区」两栏（原选择条在上+详情在下）；
//   详情工作区（REQ-237，57 号 F1 整改）：原 12 页签平铺改「左锚点分区导航 + 右内容」——
//   三簇分组（内容与编辑 / 质量与演进 / 消费与引用），加分区不再加宽；
//   Spec 编辑常驻保活（切分区不丢未保存编辑），其余分区激活挂载（沿原 destroyOnHidden 行为）。
//   分区：Spec 编辑 | 校验 | 质量卡 | 版本与源码 | 进化 | 图形编辑 | CSV 灌装 |
//         可视化 | 伴生候选 | 被引用 | 产物 | TTL 导出
//   正交红线：不出现任何引擎、端口、启停配置（运行看本体运行栏）
// B1（REQ-145/M22）：Spec 编辑/校验/产物/导出/选择条/重命名/可视化拆至 components/assets/。
// REQ-233/M60：资产列表搜索 + 空态 EmptyGuide 接入 + 详情「被引用」聚合区块 + 删除确认引用预检
// （running 方案引用服务端拦截 409、前端禁用；stopped/KB 引用警示放行——聚合源 GET /{id}/references）。
// ---------------------------------------------------------------------------

/** 详情工作区分区（三簇；57 号 F1 分组口径）。REQ-240 优化④：伴生分区仅伴生型本体注入。 */
function sectionGroups(isCompanion: boolean): { title: string; items: { key: string; label: string }[] }[] {
  return [
    {
      title: '内容与编辑',
      items: [
        { key: 'spec', label: 'Spec 编辑' },
        { key: 'graph-edit', label: '图形编辑' },
        { key: 'reltypes', label: '关系类型' },
        { key: 'dataprops', label: '数据属性' },
        { key: 'axioms', label: '公理' },
        { key: 'ingest', label: 'CSV 灌装' },
      ],
    },
    {
      title: '质量与演进',
      items: [
        { key: 'validate', label: '校验' },
        { key: 'quality', label: '质量卡' },
        { key: 'versions', label: '版本与源码' },
        { key: 'evolution', label: '进化' },
      ],
    },
    {
      title: '消费与引用',
      items: [
        { key: 'graph', label: '可视化' },
        ...(isCompanion ? [{ key: 'companion', label: '伴生候选' }] : []),
        { key: 'references', label: '被引用' },
        { key: 'artifacts', label: '产物' },
        { key: 'export', label: 'TTL 导出' },
      ],
    },
  ]
}

export default function AssetsPage() {
  const { showToast } = useUI()
  const [tabKey, setTabKey] = useState<string | null>(null) // REQ-230②：显式切换优先；切本体回落默认规则
  const [ontos, setOntos] = useState<Ontology[]>([])
  const [profiles, setProfiles] = useState<RuntimeProfile[]>([])
  const [listErr, setListErr] = useState<string | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)

  const [spec, setSpec] = useState<Spec | null>(null)
  const [specErr, setSpecErr] = useState<string | null>(null)
  const [specLoading, setSpecLoading] = useState(false)
  const [specTick, setSpecTick] = useState(0)

  const [validations, setValidations] = useState<Record<string, ValidationState>>({})
  const [mergeOpen, setMergeOpen] = useState(false) // REQ-157 导入合并向导
  const [renameOpen, setRenameOpen] = useState(false)
  const [publishName, setPublishName] = useState('') // REQ-239/M65 发布命名
  const [forkName, setForkName] = useState('')
  // REQ-240③/M66：资产卡能力雷达数据（缓存质量报告静默拉取；无报告不显示）
  const [qualityCache, setQualityCache] = useState<Record<string, QualityReport | null>>({})
  const [forkBusy, setForkBusy] = useState(false)
  const [forkErr, setForkErr] = useState<string | null>(null)

  const reloadOntos = (selectId?: string) => {
    api
      .listOntologies()
      .then((ls) => {
        setOntos(ls)
        setListErr(null)
        setActiveId((cur) => {
          if (selectId && ls.some((o) => o.id === selectId)) return selectId
          if (cur && ls.some((o) => o.id === cur)) return cur
          return ls[0]?.id ?? null
        })
      })
      .catch((e: any) => {
        setOntos([])
        setListErr(e?.message ?? '加载失败')
      })
  }

  // REQ-216⑦：伴生绑定本体清单（详情页「伴生候选」页签「对话生长」徽标判定）
  const [boundIds, setBoundIds] = useState<Set<string>>(new Set())
  const reloadBound = () => {
    companionApi
      .boundOntologies()
      .then((r) => setBoundIds(new Set(r.ontology_ids ?? [])))
      .catch(() => setBoundIds(new Set()))
  }

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
  useEffect(() => {
    if (!activeId) {
      setSpec(null)
      setSpecErr(null)
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
    if (qualityCache[activeId] === undefined) {
      const ensure = (cached?: QualityReport | null) => {
        if (cached) {
          setQualityCache((m) => ({ ...m, [activeId]: cached }))
          return
        }
        api.qualityRun(activeId, false, false, false)
          .then((r2) => setQualityCache((m) => ({ ...m, [activeId]: r2.report })))
          .catch(() => setQualityCache((m) => ({ ...m, [activeId]: null })))
      }
      api.qualityReport(activeId).then((r) => ensure(r.report)).catch(() => ensure(null))
    }
    return () => {
      alive = false
    }
  }, [activeId, specTick])

  const active = useMemo(() => ontos.find((o) => o.id === activeId) ?? null, [ontos, activeId])
  // 切换选中本体时回落默认页签规则（REQ-230②）
  useEffect(() => {
    setTabKey(null)
  }, [activeId])
  const validation = activeId ? validations[activeId] : undefined
  // 详情分区键（REQ-230② 规则保留：伴生型本体默认打开「伴生候选」分区，其余默认 Spec 编辑）
  const secKey = tabKey ?? (active && boundIds.has(active.id) ? 'companion' : 'spec')

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

  // REQ-240 前端优化①：资产列表栏可收起为图标列（44px；localStorage 记忆）——默认宽 260→220 为详情让空间
  const [listCollapsed, setListCollapsed] = useState(() => localStorage.getItem('eino.assets.collapsed') === '1')
  const toggleList = () => {
    setListCollapsed((v) => {
      localStorage.setItem('eino.assets.collapsed', v ? '0' : '1')
      return !v
    })
  }
  const groups = sectionGroups(!!active && boundIds.has(active.id))

  const listAside = (collapsed: boolean) =>
    collapsed ? (
      <aside className="sidebar" style={{ width: 44, flex: '0 0 44px', maxWidth: 44, alignItems: 'center', padding: '8px 0', gap: 4 }} aria-label="本体列表（已收起）">
        <Button type="text" size="small" icon={<MenuUnfoldOutlined />} aria-label="展开本体列表" onClick={toggleList} />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 4, overflowY: 'auto' }}>
          {ontos.slice(0, 14).map((o) => (
            <Tooltip key={o.id} title={o.name} placement="right">
              <button
                type="button"
                aria-label={`选中 ${o.name}`}
                onClick={() => setActiveId(o.id)}
                style={{
                  width: 28, height: 28, borderRadius: '50%', border: o.id === activeId ? '2px solid var(--c-brand)' : '1px solid var(--c-border)',
                  background: 'var(--c-bg-soft)', color: 'var(--c-ink-2)', fontSize: 12, cursor: 'pointer', lineHeight: 1,
                }}
              >
                {(o.name || '？').slice(0, 1)}
              </button>
            </Tooltip>
          ))}
        </div>
      </aside>
    ) : (
      <aside className="sidebar">
        <div className="side-head">
          <span className="side-title">本体资产</span>
          <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span className="side-count">{ontos.length}</span>
            <Button type="text" size="small" icon={<MenuFoldOutlined />} aria-label="收起本体列表" onClick={toggleList} />
          </span>
        </div>
        <div className="ref-menu" style={{ paddingBottom: 12 }}>
          <AssetList ontos={ontos} profiles={profiles} activeId={activeId} onSelect={setActiveId} validations={validations} />
        </div>
      </aside>
    )


  const renderBody = () => (
    <>
      {listErr ? (
        <div className="work-empty">
          <Result
            status="warning"
            title="本体平面服务未启动（BUILD_SVC_URL/:8091）"
            subTitle={listErr}
            extra={<Button onClick={() => reloadOntos()}>重试</Button>}
          />
        </div>
      ) : !active ? (
        <>
          <div className="work-head">
            <div className="work-head-text">
              <div className="work-head-title">
                <Typography.Title level={4} style={{ margin: 0 }}>
                  本体资产
                </Typography.Title>
                <Tag color="blue" style={{ margin: 0 }}>统一管理</Tag>
              </div>
              <p className="work-head-desc">
                已构建本体的统一仓库（REQ-60）：新建请到「本体构建」选择路径；此处负责编辑、校验、版本、产物、可视化与 TTL 导出。
              </p>
            </div>
          </div>
          <EmptyGuide
            title="暂无本体资产"
            steps={[
              '到「本体构建」栏选择一条构建路径（自定义 / OntoChat / 由知识库构建 / OntoExtend / OO 回流）',
              '构建产物统一进入本栏管理：编辑、校验、质量卡、版本、可视化、导出',
              '在「本体运行」栏创建运行方案装载本体，供智能体对话查询',
            ]}
            actionLabel="前往本体构建"
            onAction={() => {
              localStorage.setItem('eino.onto.sidebar', 'build')
              window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
            }}
          />
        </>
      ) : (
        <>
          <div className="work-head" style={{ flexWrap: 'wrap', rowGap: 8 }}>
            <div className="work-head-text" style={{ minWidth: 0, flex: 1 }}>
              <div className="work-head-title">
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
              </div>
              {/* REQ-264：描述单行截断（Tooltip 全文）——长描述不再多行挤占头部 */}
              <Tooltip title={active.description || undefined} placement="topLeft">
                <p className="work-head-desc" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%', margin: '5px 0 4px' }}>
                  {active.description || '未填写描述'}
                </p>
              </Tooltip>
              {/* 规模与引用元信息行（原标题行内标签迁此，标题行瘦身） */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', fontSize: 12, color: 'var(--c-ink-2)' }}>
                <span>概念 {active.n_concepts ?? 0}</span>
                <span>·</span>
                <span>关系 {active.n_relations ?? 0}</span>
                <span>·</span>
                <span>实例 {active.n_instances ?? 0}</span>
                <span>·</span>
                <Tooltip title="被 N 套运行方案引用（只读；启停操作在「本体运行」栏）">
                  <span style={{ cursor: 'default' }}>被 {refCount(active)} 套方案引用</span>
                </Tooltip>
              </div>
            </div>
            {/* REQ-264 follow-up：右列=动作按钮在上（与本体名同行顶对齐）+ 能力雷达在按钮下方；
                描述/元信息在左列，天然位于雷达左侧 */}
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 8, flexShrink: 0, alignSelf: 'flex-start' }}>
              {qualityCache[active.id] && (
                <Tooltip
                  title={
                    <span>
                      能力雷达：{radarDimsOf(qualityCache[active.id]!.score, qualityCache[active.id]!.stats).map(([k, v]) => `${k} ${Math.round(v)}`).join(' / ')}——详情见「质量卡」分区
                    </span>
                  }
                >
                  <span style={{ display: 'inline-flex', alignItems: 'center', cursor: 'default' }} data-testid="asset-radar">
                    <QualityRadar dims={radarDimsOf(qualityCache[active.id]!.score, qualityCache[active.id]!.stats)} size={92} compact />
                  </span>
                </Tooltip>
              )}
              <Space wrap style={{ order: -1 }}>
              {/* REQ-239/M65：发布（命名快照终态）/ 撤回发布（回 draft） */}
              {active.status === 'published' ? (
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
                  <Button icon={<CloudDownloadOutlined />}>撤回发布</Button>
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
                  <Button type="primary" ghost icon={<CloudUploadOutlined />}>发布</Button>
                </Popconfirm>
              )}
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
                <Button icon={<BranchesOutlined />}>Fork 本体</Button>
              </Popconfirm>
              <Button icon={<EditOutlined />} onClick={() => setRenameOpen(true)}>
                重命名
              </Button>
              <Button icon={<ImportOutlined />} onClick={() => setMergeOpen(true)}>
                导入合并
              </Button>
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
                <Button danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
              </Space>
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
                  />
                </div>
                {secKey === 'validate' && (
                  <ValidatePane
                    ontologyId={active.id}
                    result={validation ?? null}
                    onResult={(r) => setValidations((v) => ({ ...v, [active.id]: r }))}
                  />
                )}
                {secKey === 'quality' && (
                  <QualityCardPane
                    ontologyId={active.id}
                    onReport={(r) => setQualityCache((m) => ({ ...m, [active.id]: r }))}
                  />
                )}
                {secKey === 'versions' && <SourceView ontologyId={active.id} currentVersion={active.version} spec={spec} onRestored={refreshAfterSave} />}
                {secKey === 'artifacts' && <ArtifactsPane ontologyId={active.id} />}
                {secKey === 'graph' && <VizTabs spec={spec} ontologyId={active.id} isCompanion={boundIds.has(active.id)} />}
                {secKey === 'evolution' && <EvolutionPane key={active.id} ontologyId={active.id} />}
                {secKey === 'companion' && <OntologyCompanionPane key={active.id} ontologyId={active.id} />}
                {secKey === 'references' && <ReferencesPane key={active.id} ontologyId={active.id} />}
                {secKey === 'graph-edit' && (
                  <Maximizeable label="最大化编辑">
                    <GraphEditor ontologyId={active.id} spec={spec} onSpecSaved={refreshAfterSave} />
                  </Maximizeable>
                )}
                {secKey === 'reltypes' && <RelationTypesPane spec={spec} />}
                {secKey === 'dataprops' && <DataPropertiesPane spec={spec} />}
                {secKey === 'axioms' && <AxiomsPane spec={spec} />}
                {secKey === 'export' && <ExportPane ontology={active} />}
                {secKey === 'ingest' && (
                  <CsvIngestPane ontologyId={active.id} spec={spec} onIngested={() => refreshAfterSave()} />
                )}
              </div>
            </div>
          </Card>
        </>
      )}

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

  if (listCollapsed) {
    return (
      <div className="main" style={{ display: 'flex', minHeight: 0, flex: 1 }}>
        {listAside(true)}
        <div className="content-panel asset-detail-scroll" style={{ flex: 1, minWidth: 0, height: '100%', overflowY: 'auto' }}>{renderBody()}</div>
      </div>
    )
  }

  return (
    <Splitter className="main sidebar-splitter">
      <Splitter.Panel defaultSize={Number(localStorage.getItem('eino.assets.width')) || 200} min={180} max={420} className="sidebar-panel">
        {listAside(false)}
      </Splitter.Panel>
      <Splitter.Panel className="content-panel asset-detail-scroll" style={{ overflowY: 'auto' }}>{renderBody()}</Splitter.Panel>
    </Splitter>
  )
}
