import { Empty, Input, Pagination, Tag } from 'antd'
import { SearchOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { ontoStatus, stageDoneFlags, type ValidationState } from '../../shared'
import type { Ontology, RuntimeProfile } from '../../../../api/types'
import type { QualityReport } from '../../../../api/client'
import QualityRadar, { radarDimsOf } from '../../../../components/QualityRadar'
import EmptyGuide from '../../../../components/EmptyGuide'

// ---------------------------------------------------------------------------
// REQ-181/M-O17：资产列表——来源分组（自建 / 对话生长 / 种子 / fork）
// + fork 徽标（forked_from 语义，v1 零迁移派生：original 工件 / 语义 ID / forked_from 字段）
// + 构建段完成度 dots / 版本 / 运行状态标注。
// REQ-216⑦：来源分组扩「对话生长」——有智能体绑定该本体为伴生归属（companion_ontology_id
// 指向它）即归入（作为一个类型；伴生产物归属容器化的资产可见性落点）。
// REQ-233①/M60：列表搜索（名称/描述/ID/来源分组名子串过滤，前端过滤零接口变更）
// + 空态 EmptyGuide 接入（跳「本体构建」动线，治纯文案空态）。
// REQ-326③：列表自左侧窄栏（200px）升格独占整页——全宽承载搜索/来源分组/校验徽标/
// 质量雷达/引用数；点击行进详情页级工作区（onOpen）。
// ---------------------------------------------------------------------------

/** 来源分组（v1 零迁移派生口径，D-O21；REQ-216 增 grown 组，优先于既有判定）：
 *  绑定为伴生归属 → 对话生长；seed_* 语义 ID → 种子；forked_from 非空 → fork（徽标）；其余 → 自建。 */
export function groupOf(o: Ontology, bound: Set<string>): 'seed' | 'fork' | 'built' | 'grown' {
  if (bound.has(o.id)) return 'grown'
  if (/^onto_(seed|k8s_ops|med_common|gene_core)/.test(o.id) || o.forked_from === '') {
    if (o.id.startsWith('onto_seed') || ['onto_k8s_ops', 'onto_med_common', 'onto_gene_core'].includes(o.id)) return 'seed'
  }
  if (o.forked_from) return 'fork'
  return 'built'
}

export const GROUP_META: Record<string, { label: string; order: number }> = {
  built: { label: '自建', order: 0 },
  grown: { label: '对话生长', order: 1 },
  seed: { label: '种子', order: 2 },
  fork: { label: 'Fork', order: 3 },
}

/** 分组 + 过滤（列表页与详情页 prev/next 共用的单一排序源：组序 + 组内既有排序保持） */
export function orderedGroups(
  ontos: Ontology[],
  bound: Set<string>,
  q: string,
): [string, Ontology[]][] {
  const ql = q.trim().toLowerCase()
  const hit = (o: Ontology, g: string) =>
    !ql || [o.name, o.description ?? '', o.id, GROUP_META[g].label].some((s) => s.toLowerCase().includes(ql))
  const groups = new Map<string, Ontology[]>()
  for (const o of ontos) {
    const g = groupOf(o, bound)
    if (!hit(o, g)) continue
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g)!.push(o)
  }
  return [...groups.entries()].sort((a, b) => GROUP_META[a[0]].order - GROUP_META[b[0]].order)
}

// REQ-468：列表分页口径——flat 序全局分页（组头随页内首次出现渲染），pageSize 20（全站列表长清单口径）
const PAGE_SIZE = 20
// REQ-468：S1~S4 语义标签（与 stageDoneFlags 前 4 位对应；标题供 dots 悬停提示）
const STAGE_LABELS = ['① 来源', '② 编辑', '③ 校验', '④ 可视化']

export default function AssetListPage({
  ontos,
  profiles,
  q,
  onOpen,
  validations,
  qualityCache,
  bound,
}: {
  ontos: Ontology[]
  profiles: RuntimeProfile[]
  q: string
  onOpen: (id: string) => void
  validations: Record<string, ValidationState>
  /** 质量报告缓存（列表态渐进拉取；无报告不显示雷达） */
  qualityCache: Record<string, QualityReport | null>
  /** 伴生绑定本体 id（分组判定；AssetsPage 已拉取，透传免重复请求） */
  bound: Set<string>
}) {
  const [page, setPage] = useState(1)
  useEffect(() => {
    // REQ-468：搜索词/清单变化回第 1 页（防空页滞留）
    setPage(1)
  }, [q, ontos.length])

  const goBuild = () => {
    localStorage.setItem('eino.onto.sidebar', 'build')
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  }

  if (ontos.length === 0) {
    return (
      <EmptyGuide
        title="暂无本体"
        steps={['到「本体构建」栏选择一条构建路径（自定义 / OntoChat / 由知识库构建等）', '构建产物统一进入本资产列表管理']}
        actionLabel="前往本体构建"
        onAction={goBuild}
      />
    )
  }

  const ordered = orderedGroups(ontos, bound, q)
  const refCount = (o: Ontology) => profiles.filter((p) => (p.ontology_ids ?? []).includes(o.id)).length
  // REQ-468：全局分页（pageSize 20）——组结构保留：先按 flat 序切页，再在页内重组分组
  const flat = ordered.flatMap(([g, items]) => items.map((o) => [g, o] as const))
  const total = flat.length
  const pageItems = flat.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
  const pageGroups: [string, Ontology[]][] = []
  for (const [g, o] of pageItems) {
    const last = pageGroups[pageGroups.length - 1]
    if (last && last[0] === g) last[1].push(o)
    else pageGroups.push([g, [o]])
  }
  const groupTotal = (g: string) => ordered.find(([kg]) => kg === g)?.[1].length ?? 0

  return (
    <div className="asset-page-list" data-testid="asset-page-list">
      {ordered.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`无匹配「${q.trim()}」的本体`} style={{ marginTop: 40 }} />
      ) : (
        pageGroups.map(([g, items]) => (
          <div key={g} className="asset-group">
            <div className="asset-group-title">
              {GROUP_META[g].label} <span className="side-count">{groupTotal(g)}</span>
            </div>
            <div className="asset-page-rows">
              {items.map((o) => {
                const f = stageDoneFlags(o, validations[o.id], profiles, null, false)
                const st = ontoStatus(o, profiles)
                const qr = qualityCache[o.id]
                const val = validations[o.id]
                const dotsTitle = `构建段完成度 ${f.slice(0, 4).filter(Boolean).length}/4：${STAGE_LABELS.map((s, i) => `${s} ${f[i] ? '✓' : '—'}`).join(' · ')}`
                return (
                  <button
                    key={o.id}
                    type="button"
                    className="asset-row"
                    data-testid="asset-row"
                    onClick={() => onOpen(o.id)}
                    title={`${o.name} · v${o.version ?? '—'} · ${st.text}${g === 'grown' ? ' · 对话生长（伴生归属）' : ''}`}
                  >
                    <span className="asset-row-main">
                      <span className="asset-row-title">
                        <span className="asset-item-name" title={o.name}>{o.name}</span>
                        {/* REQ-490：列表行 4 彩 Tag 圆点化灰字（仅校验有误留红） */}
                        {o.status === 'published' && (
                          <span className="asset-row-dot" title={`已发布${o.version_name ? ` · ${o.version_name}` : ''}`}>
                            <i className="dot dot-ok" />已发布
                          </span>
                        )}
                        {g === 'fork' && <span className="asset-row-dot">fork</span>}
                        {g === 'grown' && <span className="asset-row-dot">对话生长</span>}
                        {val && (
                          val.ok ? (
                            <span className="asset-row-dot"><i className="dot dot-ok" />校验通过</span>
                          ) : (
                            <Tag color="red" style={{ margin: 0, fontSize: 10, lineHeight: '15px', padding: '0 4px' }}>校验有误</Tag>
                          )
                        )}
                      </span>
                      <span className="asset-row-desc">{o.description || '未填写描述'}</span>
                    </span>
                    <span className="asset-row-meta">
                      <span className="onto-dots" title={dotsTitle}>
                        {f.slice(0, 4).map((done, i) => (
                          <i key={i} className={`onto-dot${done ? ' on' : ''}`} />
                        ))}
                      </span>
                      <span style={{ fontSize: 11, color: 'var(--c-ink-3)' }}>v{o.version ?? '—'}</span>
                      <span
                        className="onto-dot"
                        style={{ background: st.color === 'green' ? '#16a34a' : st.color === 'default' ? '#c3c8da' : st.color, opacity: st.text.includes('运行中') ? 1 : 0.5 }}
                        title={st.text}
                      />
                      <span style={{ fontSize: 11, color: 'var(--c-ink-3)' }} title="被 N 套运行方案引用">引用 {refCount(o)}</span>
                      {qr ? (
                        <span
                          className="asset-row-radar"
                          data-testid="asset-row-radar"
                          title={`能力雷达：${radarDimsOf(qr.score, qr.stats).map(([k, v]) => `${k} ${Math.round(v)}`).join(' / ')}`}
                        >
                          <QualityRadar dims={radarDimsOf(qr.score, qr.stats)} size={44} compact />
                        </span>
                      ) : (
                        <span className="asset-row-radar asset-row-radar-empty" title="尚无质量报告（详情页质量卡可生成）" />
                      )}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>
        ))
      )}
      {total > PAGE_SIZE && (
        <Pagination
          size="small"
          current={page}
          pageSize={PAGE_SIZE}
          total={total}
          showTotal={(n) => `共 ${n} 个本体`}
          onChange={setPage}
          style={{ marginTop: 16, justifyContent: 'flex-end' }}
          data-testid="asset-list-pagination"
        />
      )}
    </div>
  )
}

/** 列表搜索框（工具条用；状态提升至 AssetsPage——返回列表不丢列表态） */
export function AssetSearchBox({ q, onQ }: { q: string; onQ: (v: string) => void }) {
  return (
    <Input
      allowClear
      size="small"
      prefix={<SearchOutlined style={{ color: 'var(--c-ink-3)' }} />}
      placeholder="搜索名称 / 描述 / 来源"
      value={q}
      onChange={(e) => onQ(e.target.value)}
      style={{ width: 260 }}
      data-testid="asset-search"
    />
  )
}
