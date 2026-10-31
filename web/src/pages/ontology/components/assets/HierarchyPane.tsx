import { useEffect, useMemo, useState } from 'react'
import { AutoComplete, Button, Empty, Form, Input, Modal, Popconfirm, Popover, Segmented, Select, Space, Splitter, Table, Tag, Tooltip, Tree, Typography, message } from 'antd'
import type { DataNode } from 'antd/es/tree'
import { PlusOutlined, RollbackOutlined } from '@ant-design/icons'
import type { Ontology, Spec, SpecConcept, SpecDataProperty, SpecInstance, SpecRelation, ValidationError } from '../../../../api/types'
import { api } from '../../../../api/client'
import './hierarchy.css'
import RestrictionsEditor from './RestrictionsEditor'
import { RenameButton } from './RenameButton'
import InstanceEditPanel from './InstanceEditPanel'
import { useQualityDigest } from './qualityDigest'
import { ERR_COLUMNS } from '../../shared'

// ---------------------------------------------------------------------------
// REQ-363/M163 本体内容层级浏览（Protégé 式树形导航与实体详情）——资产详情「层级浏览」分区。
//   纯前端零后端：数据源=既有 GET /spec 单源派生。六件：
//   A 类层级树（Concept.Parents 多继承 DAG 投影为树：跨父复现+标注；悬空父/环防护诚实投影）
//   B 实例树（按 Instance.Concept 类型分组；未声明类型/悬空归「未分组」诚实呈现）
//   C 属性分组树（关系+数据属性按 Domain 分组——诚实边界：spec_json 无 subPropertyOf，
//     按 Domain 分组是呈现形态非语义层级，真属性层级 P2 单独立项）
//   D 实体描述面板（类型/定义/父与子/关系端点/数据属性/公理/直接实例/urn:o: 确定性 URI
//     ——Protégé Description view 对应物；URI 规则与 spec.go Sanitize/URIPrefix 同口径前端派生）
//   E 全局搜索定位（name/label 子串跨四类元素；选中即树高亮+描述面板联动，经 AssetsPage
//     提升的选择单源广播到可视化 2D——收口 REQ-253① P1 跨视图选择总线挂账〔59 号 P1〕）
//   F 元素级 usage 反向引用（五类口径与 impact.go collectRefs 同口径前端派生：
//     关系 From/To、实例 Concept、子概念、数据属性 Domain、公理主宾；空态诚实）
//   大 spec 防线沿 REQ-304/305：分区懒挂载（本分区激活才挂载）+ antd Tree height 虚拟滚动。
//   树不替代图形视图：SpecGraph/Graph3D/WebVOWL 全保留，选择总线双向联动互补。
// REQ-448/M244（Protégé 手动建模兑现批①，REQ-363 P2③ 脱窗兑现）：树上编辑——HierarchyPane
//   自只读导航升级编辑入口：类树根新增/+子概念〔挂选定父〕/挂父/摘父（DescPanel 父 Tag ✕）/
//   删除；实例树实例删除。全部走构建面元素级端点（kind=concept/instance，Validate+版本门禁
//   服务端单源沿 REQ-436）——「重排 parents」v1 以挂父/摘父达成（拖拽在 DAG 投影复现节点上
//   语义模糊留观察）；被引用删除由校验 400 如实回喂。Protégé 树-画布分工：树管层级，画布管拓扑。
// ---------------------------------------------------------------------------

/** 跨视图选择单源载荷（REQ-253① 总线；kind+name 二元组，元素身份=类型+名） */
export type OntoEntityKind = 'concept' | 'relation' | 'dataprop' | 'instance'
export interface OntoSelection {
  kind: OntoEntityKind
  name: string
}

type TreeMode = 'class' | 'instance' | 'prop'

interface NodeMeta {
  sel: OntoSelection
  parentKey: string | null
}

/**确定性 URN 派生：与 pkg/ontology/spec/spec.go Sanitize 逐字符同口径（保留 Unicode 字母/数字与 _-.，其余替换 _） */
function ontoSanitize(s: string): string {
  let out = ''
  for (const ch of s.trim()) out += /[\p{L}\p{N}_.-]/u.test(ch) ? ch : '_'
  return out
}
function entityUri(ontologyId: string, sel: OntoSelection): string {
  const prefix = `urn:o:${ontoSanitize(ontologyId)}:`
  const n = ontoSanitize(sel.name)
  if (sel.kind === 'concept') return `${prefix}concept:${n}`
  if (sel.kind === 'relation') return `${prefix}relation:${n}`
  if (sel.kind === 'dataprop') return `${prefix}attr:${n}`
  return `${prefix}instance:${n}`
}

/** 树节点 title 公共形态：名 + 次要信息 + 徽标 */
function NodeTitle(props: { name: string; label?: string; def?: string; extra?: React.ReactNode; badge?: React.ReactNode }) {
  return (
    <span className="onto-hier-title">
      <Tooltip title={props.def || undefined} placement="right">
        <span className="onto-hier-name">{props.label || props.name}</span>
      </Tooltip>
      {props.label && props.label !== props.name && <span className="onto-hier-sub">{props.name}</span>}
      {props.extra}
      {props.badge}
    </span>
  )
}

/** A 类层级树：Concept.Parents DAG → 树投影（多继承跨父复现；悬空父归根+标注；环防护）；
 *  REQ-448：ops 回调为选中概念渲染树上快捷操作钮（+子/挂父/删除——hover 显隐） */
function buildClassTree(spec: Spec, ops?: (name: string) => React.ReactNode) {
  const concepts = spec.concepts ?? []
  const byName = new Map(concepts.map((c) => [c.name, c] as const))
  const declared = new Set(byName.keys())
  const childrenOf = new Map<string, string[]>()
  const danglingOf = new Map<string, string[]>()
  const roots: SpecConcept[] = []
  const instCount = new Map<string, number>()
  for (const it of spec.instances ?? []) instCount.set(it.concept, (instCount.get(it.concept) ?? 0) + 1)
  for (const c of concepts) {
    const ps = c.parents ?? []
    const live = ps.filter((p) => declared.has(p))
    const dangling = ps.filter((p) => !declared.has(p))
    if (dangling.length > 0) danglingOf.set(c.name, dangling)
    if (live.length === 0) {
      roots.push(c)
    } else {
      for (const p of live) {
        if (!childrenOf.has(p)) childrenOf.set(p, [])
        childrenOf.get(p)!.push(c.name)
      }
    }
  }
  const keyCounter = new Map<string, number>()
  const index = new Map<string, NodeMeta>()
  const multi = (c: SpecConcept) => (c.parents ?? []).filter((p) => declared.has(p)).length + (danglingOf.get(c.name)?.length ?? 0) > 1
  const build = (c: SpecConcept, parentKey: string | null, ancestors: Set<string>): DataNode | null => {
    if (ancestors.has(c.name)) return null // 环防护：自身祖先不再下钻（DAG 投影即弃该边并诚实断枝）
    const n = keyCounter.get(c.name) ?? 0
    keyCounter.set(c.name, n + 1)
    const key = n === 0 ? `c:${c.name}` : `c:${c.name}#${n}`
    index.set(key, { sel: { kind: 'concept', name: c.name }, parentKey })
    const kids = (childrenOf.get(c.name) ?? [])
      .map((k) => byName.get(k))
      .filter((k): k is SpecConcept => !!k)
      .map((k) => build(k, key, new Set([...ancestors, c.name])))
      .filter((x): x is DataNode => !!x)
    const ic = instCount.get(c.name) ?? 0
    const dup = n > 0
    const dangling = danglingOf.get(c.name) ?? []
    const extra: React.ReactNode[] = []
    if (dup)
      extra.push(
        <Tooltip key="dup" title={`多继承复现：该概念在多个父类下重复呈现（DAG 树投影，非重复定义）`}>
          <Tag color="orange" style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
            复现
          </Tag>
        </Tooltip>,
      )
    if (!dup && multi(c))
      extra.push(
        <Tooltip key="mi" title={`多继承：父类 ${(c.parents ?? []).join('、')}——在各父类下重复呈现（DAG 树投影）`}>
          <Tag color="orange" style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
            多继承
          </Tag>
        </Tooltip>,
      )
    if (dangling.length > 0)
      extra.push(
        <Tooltip key="dg" title={`父概念「${dangling.join('、')}」未在本体中声明——按根/既有父类呈现（诚实投影）`}>
          <Tag color="volcano" style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
            父未声明
          </Tag>
        </Tooltip>,
      )
    return {
      key,
      title: (
        <NodeTitle
          name={c.name}
          label={c.label}
          def={c.definition}
          extra={extra}
          badge={
            <>
              {kids.length > 0 && <span className="onto-hier-count">{kids.length} 子</span>}
              {ic > 0 && (
                <Tooltip title={`直接实例 ${ic} 个`}>
                  <span className="onto-hier-badge">{ic}</span>
                </Tooltip>
              )}
              {ops?.(c.name)}
            </>
          }
        />
      ),
      children: kids.length > 0 ? kids : undefined,
    }
  }
  const data = roots.map((c) => build(c, null, new Set())).filter((x): x is DataNode => !!x)
  return { data, index }
}

/** B 实例树：按 Instance.Concept 类型分组；未声明类型/悬空归「未分组」；REQ-448：实例叶子挂删除钮 */
function buildInstanceTree(spec: Spec, instOps?: (name: string) => React.ReactNode) {
  const byName = new Map((spec.concepts ?? []).map((c) => [c.name, c] as const))
  const declared = new Set(byName.keys())
  const byConcept = new Map<string, SpecInstance[]>()
  const none: SpecInstance[] = []
  for (const it of spec.instances ?? []) {
    if (it.concept && declared.has(it.concept)) {
      if (!byConcept.has(it.concept)) byConcept.set(it.concept, [])
      byConcept.get(it.concept)!.push(it)
    } else {
      none.push(it)
    }
  }
  const index = new Map<string, NodeMeta>()
  const keyCounter = new Map<string, number>()
  const uniqKey = (base: string) => {
    const n = keyCounter.get(base) ?? 0
    keyCounter.set(base, n + 1)
    return n === 0 ? base : `${base}#${n}`
  }
  const data: DataNode[] = []
  for (const [cname, list] of byConcept) {
    const gkey = uniqKey(`g:${cname}`)
    index.set(gkey, { sel: { kind: 'concept', name: cname }, parentKey: null })
    const children = list.map((it) => {
      const k = uniqKey(`i:${it.name}`)
      index.set(k, { sel: { kind: 'instance', name: it.name }, parentKey: gkey })
      return {
        key: k,
        title: (
          <NodeTitle name={it.name} extra={<span className="onto-hier-count">属 {it.concept}</span>} badge={instOps?.(it.name)} />
        ),
      }
    })
    data.push({
      key: gkey,
      title: (
        <NodeTitle name={cname} label={byName.get(cname)?.label} extra={<span className="onto-hier-badge">{list.length}</span>} />
      ),
      children,
    })
  }
  if (none.length > 0) {
    const gkey = uniqKey('g:__none__') // 组节点不进 index：点击组头不产生选择（仅实例叶子可选）
    data.push({
      key: gkey,
      title: (
        <Tooltip title="实例类型未声明或指向未声明概念——诚实归组（REQ-363 B：悬空实例不静默丢弃）">
          <span className="onto-hier-title">
            <span className="onto-hier-name">未分组</span>
            <Tag color="warning" style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>
              类型缺失
            </Tag>
            <span className="onto-hier-badge">{none.length}</span>
          </span>
        </Tooltip>
      ),
      children: none.map((it) => {
        const k = uniqKey(`i:${it.name}`)
        index.set(k, { sel: { kind: 'instance', name: it.name }, parentKey: gkey })
        return {
          key: k,
          title: <NodeTitle name={it.name} extra={<span className="onto-hier-count">属 {it.concept || '—'}</span>} badge={instOps?.(it.name)} />,
        }
      }),
    })
  }
  return { data, index, grouped: byConcept.size + (none.length > 0 ? 1 : 0) }
}

/** C 属性分组树：关系+数据属性按 Domain 分组（呈现形态，非语义层级——spec 无 subPropertyOf） */
function buildPropTree(spec: Spec) {
  const index = new Map<string, NodeMeta>()
  const keyCounter = new Map<string, number>()
  const uniqKey = (base: string) => {
    const n = keyCounter.get(base) ?? 0
    keyCounter.set(base, n + 1)
    return n === 0 ? base : `${base}#${n}`
  }
  const declared = new Set((spec.concepts ?? []).map((c) => c.name))
  const byName = new Map((spec.concepts ?? []).map((c) => [c.name, c] as const))
  const groups = new Map<string, { rels: SpecRelation[]; dps: SpecDataProperty[] }>()
  const order: string[] = []
  const push = (dom: string) => {
    if (!groups.has(dom)) {
      groups.set(dom, { rels: [], dps: [] })
      order.push(dom)
    }
    return groups.get(dom)!
  }
  for (const r of spec.relations ?? []) push(r.from && declared.has(r.from) ? r.from : '__none__').rels.push(r)
  for (const d of spec.data_properties ?? []) push(d.domain && declared.has(d.domain) ? d.domain : '__none__').dps.push(d)
  const data: DataNode[] = order.map((dom) => {
    const g = groups.get(dom)!
    const isNone = dom === '__none__'
    const gkey = uniqKey(`dom:${dom}`)
    if (!isNone) index.set(gkey, { sel: { kind: 'concept', name: dom }, parentKey: null })
    const children: DataNode[] = []
    for (const r of g.rels) {
      const k = uniqKey(`r:${r.name}`)
      index.set(k, { sel: { kind: 'relation', name: r.name }, parentKey: gkey })
      children.push({
        key: k,
        title: (
          <NodeTitle
            name={r.name}
            label={r.label}
            def={r.definition}
            extra={
              <>
                <Tag style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>关系</Tag>
                <span className="onto-hier-count">{r.from || '—'} → {r.to || '—'}</span>
              </>
            }
          />
        ),
      })
    }
    for (const d of g.dps) {
      const k = uniqKey(`d:${d.name}`)
      index.set(k, { sel: { kind: 'dataprop', name: d.name }, parentKey: gkey })
      children.push({
        key: k,
        title: (
          <NodeTitle
            name={d.name}
            label={d.label}
            def={d.definition}
            extra={
              <>
                <Tag color="cyan" style={{ margin: '0 0 0 6px', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>数据属性</Tag>
                <span className="onto-hier-count">{d.range || 'string'}</span>
              </>
            }
          />
        ),
      })
    }
    return {
      key: gkey,
      title: isNone ? (
        <Tooltip title="定义域未声明或指向未声明概念——诚实归组">
          <span className="onto-hier-title">
            <span className="onto-hier-name">未指定定义域</span>
            <span className="onto-hier-badge">{g.rels.length + g.dps.length}</span>
          </span>
        </Tooltip>
      ) : (
        <NodeTitle name={dom} label={byName.get(dom)?.label} extra={<span className="onto-hier-badge">{g.rels.length + g.dps.length}</span>} />
      ),
      children,
    }
  })
  return { data, index }
}

interface UsageGroup {
  cat: string
  items: { label: string; sel?: OntoSelection }[]
}

/** F usage 反向引用：五类口径与 impact.go collectRefs 同口径前端派生 */
function usageOf(spec: Spec | null, sel: OntoSelection | null): UsageGroup[] {
  if (!spec || !sel) return []
  if (sel.kind === 'concept') {
    const relFrom = (spec.relations ?? []).filter((r) => r.from === sel.name)
    const relTo = (spec.relations ?? []).filter((r) => r.to === sel.name)
    const insts = (spec.instances ?? []).filter((i) => i.concept === sel.name)
    const childs = (spec.concepts ?? []).filter((c) => (c.parents ?? []).includes(sel.name))
    const dps = (spec.data_properties ?? []).filter((d) => d.domain === sel.name)
    const axSub = (spec.axioms ?? []).filter((a) => a.subject === sel.name)
    const axObj = (spec.axioms ?? []).filter((a) => (a.targets ?? []).includes(sel.name) && a.subject !== sel.name)
    const out: UsageGroup[] = []
    const relItems = [
      ...relFrom.map((r) => ({ label: `关系「${r.name}」定义域`, sel: { kind: 'relation' as const, name: r.name } })),
      ...relTo.map((r) => ({ label: `关系「${r.name}」值域`, sel: { kind: 'relation' as const, name: r.name } })),
    ]
    if (relItems.length) out.push({ cat: '关系端点', items: relItems })
    if (insts.length) out.push({ cat: '实例', items: insts.map((i) => ({ label: `实例「${i.name}」`, sel: { kind: 'instance' as const, name: i.name } })) })
    if (childs.length) out.push({ cat: '子概念', items: childs.map((c) => ({ label: `子概念「${c.name}」`, sel: { kind: 'concept' as const, name: c.name } })) })
    if (dps.length) out.push({ cat: '数据属性', items: dps.map((d) => ({ label: `数据属性「${d.name}」`, sel: { kind: 'dataprop' as const, name: d.name } })) })
    const axItems = [
      ...axSub.map((a) => ({ label: `公理（${a.type}）主：⇄ [${(a.targets ?? []).join('、')}]` })),
      ...axObj.map((a) => ({ label: `公理（${a.type}）宾：主「${a.subject}」` })),
    ]
    if (axItems.length) out.push({ cat: '公理主宾', items: axItems })
    return out
  }
  if (sel.kind === 'relation') {
    const refs = (spec.instances ?? []).filter((i) => (i.relations ?? []).some((r) => r.rel === sel.name))
    if (!refs.length) return []
    return [{ cat: '实例引用', items: refs.map((i) => ({ label: `实例「${i.name}」`, sel: { kind: 'instance' as const, name: i.name } })) }]
  }
  if (sel.kind === 'dataprop') {
    const refs = (spec.instances ?? []).filter((i) => Object.keys(i.attributes ?? {}).includes(sel.name))
    if (!refs.length) return []
    return [{ cat: '实例引用', items: refs.map((i) => ({ label: `实例「${i.name}」`, sel: { kind: 'instance' as const, name: i.name } })) }]
  }
  return [] // 实例为叶子：无反向引用（诚实空态）
}

function RefTag(props: { label: string; sel?: OntoSelection; onSelect: (s: OntoSelection) => void }) {
  if (!props.sel) return <Tag style={{ margin: '0 6px 4px 0' }}>{props.label}</Tag>
  return (
    <Tag style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => props.onSelect(props.sel!)}>
      {props.label}
    </Tag>
  )
}

/** D 实体描述面板：选中实体聚合呈现（Protégé Description view 对应物）；REQ-448：父概念 Tag ✕=摘父 */
function DescPanel(props: { ontology: Ontology; spec: Spec; sel: OntoSelection; onSelect: (s: OntoSelection) => void; onDetachParent?: (concept: string, parent: string) => void; onChanged?: () => void }) {
  const { ontology, spec, sel, onSelect, onDetachParent, onChanged } = props
  const uri = entityUri(ontology.id, sel)
  const concepts = spec.concepts ?? []
  const byName = new Map(concepts.map((c) => [c.name, c] as const))
  const label = sel.kind === 'concept' ? byName.get(sel.name)?.label : undefined
  const usage = usageOf(spec, sel)
  const usageTotal = usage.reduce((n, g) => n + g.items.length, 0)

  const secTitle = (t: string) => <div className="onto-hier-sec-title">{t}</div>
  const body = (() => {
    if (sel.kind === 'concept') {
      const c = byName.get(sel.name)
      if (!c) return <Typography.Text type="secondary">概念不存在于当前 Spec</Typography.Text>
      const parents = (c.parents ?? []).map((p) => ({ name: p, live: byName.has(p) }))
      const childs = concepts.filter((x) => (x.parents ?? []).includes(c.name))
      const relsOut = (spec.relations ?? []).filter((r) => r.from === c.name)
      const relsIn = (spec.relations ?? []).filter((r) => r.to === c.name)
      const dps = (spec.data_properties ?? []).filter((d) => d.domain === c.name)
      const axioms = (spec.axioms ?? []).filter((a) => a.subject === c.name || (a.targets ?? []).includes(c.name))
      const insts = (spec.instances ?? []).filter((i) => i.concept === c.name)
      return (
        <>
          {secTitle('定义')}
          <p className={`onto-hier-def${c.definition ? '' : ' muted'}`}>{c.definition || '未填写定义'}</p>
          <div className="onto-hier-row">
            <span className="onto-hier-label">父概念</span>
            <span>
              {parents.length > 0 ? (
                parents.map((p) =>
                  p.live ? (
                    <Tag
                      key={p.name}
                      style={{ margin: '0 6px 4px 0', cursor: 'pointer' }}
                      onClick={() => onSelect({ kind: 'concept', name: p.name })}
                      closable={!!onDetachParent}
                      closeIcon={<span data-testid={`hierarchy-detach-${p.name}`} style={{ fontSize: 10 }}>✕</span>}
                      onClose={(e) => {
                        e.preventDefault()
                        onDetachParent?.(sel.name, p.name)
                      }}
                    >
                      {p.name}
                    </Tag>
                  ) : (
                    <Tooltip key={p.name} title="未在本体中声明（悬空引用，诚实呈现）">
                      <Tag color="volcano" style={{ margin: '0 6px 4px 0' }}>{p.name}（未声明）</Tag>
                    </Tooltip>
                  ),
                )
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—（根概念）</Typography.Text>
              )}
            </span>
          </div>
          <div className="onto-hier-row">
            <span className="onto-hier-label">子概念</span>
            <span>
              {childs.length > 0 ? (
                childs.map((x) => (
                  <Tag key={x.name} style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'concept', name: x.name })}>
                    {x.name}
                  </Tag>
                ))
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—（叶子）</Typography.Text>
              )}
            </span>
          </div>
          <div className="onto-hier-row">
            <span className="onto-hier-label">关系端点</span>
            <span>
              {relsOut.length + relsIn.length > 0 ? (
                <>
                  {relsOut.map((r) => (
                    <Tag key={`o:${r.name}`} style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'relation', name: r.name })}>
                      {r.name} → {r.to || '—'}
                    </Tag>
                  ))}
                  {relsIn.map((r) => (
                    <Tag key={`i:${r.name}`} style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'relation', name: r.name })}>
                      {r.from || '—'} → {r.name}
                    </Tag>
                  ))}
                </>
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>
              )}
            </span>
          </div>
          <div className="onto-hier-row">
            <span className="onto-hier-label">数据属性</span>
            <span>
              {dps.length > 0 ? (
                dps.map((d) => (
                  <Tag key={d.name} color="cyan" style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'dataprop', name: d.name })}>
                    {d.name}
                    {d.range ? `:${d.range}` : ''}
                  </Tag>
                ))
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>
              )}
            </span>
          </div>
          {/* REQ-472/M272：限制公理表单化编辑（后端 REQ-376① 五型承载开放编辑面） */}
          {onChanged && <RestrictionsEditor ontologyId={ontology.id} conceptName={c.name} spec={spec} onChanged={onChanged} />}
          {axioms.length > 0 && (
            <div className="onto-hier-row">
              <span className="onto-hier-label">公理</span>
              <span>
                {axioms.map((a, idx) => (
                  <Tag key={idx} color="geekblue" style={{ margin: '0 6px 4px 0' }}>
                    {a.subject === c.name ? `${a.type} ⇄ [${(a.targets ?? []).join('、')}]` : `${a.type}：主「${a.subject}」⇄ 本概念`}
                  </Tag>
                ))}
              </span>
            </div>
          )}
          <div className="onto-hier-row">
            <span className="onto-hier-label">直接实例</span>
            <span>
              {insts.length > 0 ? (
                <>
                  {insts.slice(0, 20).map((i) => (
                    <Tag key={i.name} color="purple" style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'instance', name: i.name })}>
                      {i.name}
                    </Tag>
                  ))}
                  {insts.length > 20 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>等 {insts.length} 个</Typography.Text>}
                </>
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—（无实例）</Typography.Text>
              )}
            </span>
          </div>
        </>
      )
    }
    if (sel.kind === 'relation') {
      const r = (spec.relations ?? []).find((x) => x.name === sel.name)
      if (!r) return <Typography.Text type="secondary">关系不存在于当前 Spec</Typography.Text>
      return (
        <>
          {secTitle('定义')}
          <p className={`onto-hier-def${r.definition ? '' : ' muted'}`}>{r.definition || '未填写定义'}</p>
          <div className="onto-hier-row">
            <span className="onto-hier-label">端点</span>
            <span>
              <Tag style={{ margin: '0 4px 4px 0', cursor: 'pointer' }} onClick={() => r.from && onSelect({ kind: 'concept', name: r.from })}>{r.from || '—'}</Tag>
              <span style={{ color: 'var(--c-ink-3)' }}>→</span>
              <Tag style={{ margin: '0 4px 4px 0', cursor: 'pointer' }} onClick={() => r.to && onSelect({ kind: 'concept', name: r.to })}>{r.to || '—'}</Tag>
            </span>
          </div>
        </>
      )
    }
    if (sel.kind === 'dataprop') {
      const d = (spec.data_properties ?? []).find((x) => x.name === sel.name)
      if (!d) return <Typography.Text type="secondary">数据属性不存在于当前 Spec</Typography.Text>
      return (
        <>
          {secTitle('定义')}
          <p className={`onto-hier-def${d.definition ? '' : ' muted'}`}>{d.definition || '未填写定义'}</p>
          <div className="onto-hier-row">
            <span className="onto-hier-label">定义域</span>
            <span>
              {d.domain ? (
                <Tag style={{ margin: 0, cursor: 'pointer' }} onClick={() => onSelect({ kind: 'concept', name: d.domain! })}>{d.domain}</Tag>
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>—（不限）</Typography.Text>
              )}
            </span>
          </div>
          <div className="onto-hier-row">
            <span className="onto-hier-label">值域类型</span>
            <Tag color="cyan" style={{ margin: 0 }}>{d.range || 'string'}</Tag>
          </div>
        </>
      )
    }
    const it = (spec.instances ?? []).find((x) => x.name === sel.name)
    if (!it) return <Typography.Text type="secondary">实例不存在于当前 Spec</Typography.Text>
    const attrs = Object.entries(it.attributes ?? {})
    return (
      <>
        <div className="onto-hier-row">
          <span className="onto-hier-label">类型</span>
          <Tag color="purple" style={{ margin: 0, cursor: 'pointer' }} onClick={() => it.concept && onSelect({ kind: 'concept', name: it.concept })}>
            {it.concept || '—（未声明）'}
          </Tag>
        </div>
        {/* REQ-476/M276：实例编辑表单化（属性值/断言/换类型——instance upsert 前端首次消费） */}
        {onChanged && <InstanceEditPanel ontologyId={ontology.id} spec={spec} instance={it} onChanged={onChanged} />}
        {attrs.length > 0 && (
          <div className="onto-hier-row">
            <span className="onto-hier-label">属性</span>
            <span>
              {attrs.slice(0, 12).map(([k, v]) => (
                <Tag key={k} style={{ margin: '0 6px 4px 0' }}>{k}={String(v)}</Tag>
              ))}
              {attrs.length > 12 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>等 {attrs.length} 项</Typography.Text>}
            </span>
          </div>
        )}
        {(it.relations ?? []).length > 0 && (
          <div className="onto-hier-row">
            <span className="onto-hier-label">关系</span>
            <span>
              {(it.relations ?? []).map((r, idx) => (
                <Tag key={idx} style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'relation', name: r.rel })}>
                  {r.rel} → {r.target}
                </Tag>
              ))}
            </span>
          </div>
        )}
      </>
    )
  })()

  const kindLabel = { concept: '概念', relation: '关系', dataprop: '数据属性', instance: '实例' }[sel.kind]
  return (
    <div className="onto-hier-desc" data-testid="hierarchy-desc">
      <div className="onto-hier-desc-head">
        <Tag color={sel.kind === 'instance' ? 'purple' : sel.kind === 'dataprop' ? 'cyan' : 'blue'} style={{ margin: 0 }}>
          {kindLabel}
        </Tag>
        {label && label !== sel.name && <Typography.Text strong style={{ fontSize: 14 }}>{label}</Typography.Text>}
        <Typography.Text strong style={{ fontSize: 14 }}>{sel.name}</Typography.Text>
      </div>
      <div className="onto-hier-uri" title={uri} data-testid="hierarchy-uri">
        {uri}
      </div>
      {/* REQ-475/M275：概念/实例重命名（refactor rename——服务端全引用面 rewire） */}
      {onChanged && (sel.kind === 'concept' || sel.kind === 'instance') && (
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 4 }}>
          <RenameButton ontologyId={ontology.id} kind={sel.kind} name={sel.name} label={sel.kind === 'concept' ? '概念' : '实例'} onSaved={onChanged} />
        </div>
      )}
      {body}
      {secTitle('usage 反向引用')}
      <div data-testid="hierarchy-usage">
        {usageTotal === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            无引用——{sel.kind === 'concept' ? '删除不影响其他元素' : sel.kind === 'instance' ? '实例为叶子，无反向引用' : '暂无实例使用'}
          </Typography.Text>
        ) : (
          usage.map((g) => (
            <div key={g.cat} className="onto-hier-usage-group">
              <Tag color="default" style={{ margin: '0 6px 4px 0' }}>{g.cat} · {g.items.length}</Tag>
              {g.items.slice(0, 30).map((x, i) => (
                <RefTag key={i} label={x.label} sel={x.sel} onSelect={onSelect} />
              ))}
              {g.items.length > 30 && <Typography.Text type="secondary" style={{ fontSize: 12 }}>等 {g.items.length} 条</Typography.Text>}
            </div>
          ))
        )}
      </div>
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
        usage 口径与 impact.go collectRefs 同口径前端派生（关系 From/To · 实例类型 · 子概念 · 数据属性域 · 公理主宾）
      </Typography.Text>
    </div>
  )
}

export interface HierarchyPaneProps {
  ontology: Ontology
  spec: Spec | null
  /** 跨视图选择单源（AssetsPage 提升；可视化 2D 消费同源） */
  selection: OntoSelection | null
  onSelect: (s: OntoSelection | null) => void
  /** REQ-448：树上编辑保存后刷新 spec（AssetsPage.refreshAfterSave 同源） */
  onSaved?: () => void
}

export default function HierarchyPane({ ontology, spec, selection, onSelect, onSaved }: HierarchyPaneProps) {
  const [mode, setMode] = useState<TreeMode>('class')
  const [q, setQ] = useState('')
  const [expanded, setExpanded] = useState<React.Key[]>([])
  // REQ-448：树上编辑态——mode=create（新建，parent=选定父/undefined=根）| attach（挂父）；busy=请求在途
  const [elemModal, setElemModal] = useState<{ mode: 'create' | 'attach'; parent?: string; concept?: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [form] = Form.useForm()
  // REQ-502/M310：常驻诊断条（save=false 快评）+元素操作失败结构化明细+回滚入口（快照栈接线）
  const [digestTick, setDigestTick] = useState(0)
  const digest = useQualityDigest(ontology.id, digestTick)
  const [elemErrors, setElemErrors] = useState<ValidationError[] | null>(null)
  const [ontoVersion, setOntoVersion] = useState<number | null>(null)
  const [rollingBack, setRollingBack] = useState(false)

  const propTree = useMemo(() => (spec ? buildPropTree(spec) : { data: [], index: new Map<string, NodeMeta>() }), [spec])

  // ---- REQ-448：元素级操作（Validate+版本门禁服务端单源；400 校验明细如实回喂）----
  const fetchVersion = () => {
    api
      .getOntology(ontology.id)
      .then((o) => setOntoVersion(o.version ?? null))
      .catch(() => {})
  }
  useEffect(() => {
    fetchVersion()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ontology.id])
  const runElem = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    try {
      await fn()
      message.success(ok)
      setElemModal(null)
      form.resetFields()
      setElemErrors(null)
      setDigestTick((t) => t + 1) // REQ-502：保存后自动刷新诊断
      fetchVersion()
      onSaved?.()
    } catch (e: any) {
      // REQ-502：runElem 失败从一行 toast 改 path+message 结构化明细表（对齐 SpecEditorPane 规格）
      if (e instanceof Object && 'validationErrors' in e && Array.isArray((e as { validationErrors: unknown }).validationErrors) && (e as { validationErrors: ValidationError[] }).validationErrors.length) {
        setElemErrors((e as { validationErrors: ValidationError[] }).validationErrors)
      } else {
        setElemErrors([{ path: '-', message: e?.message ?? '操作失败' }])
      }
      message.error('操作失败——结构化明细见下方表')
    } finally {
      setBusy(false)
    }
  }
  const submitElemModal = async () => {
    if (!elemModal) return
    try {
      const v = await form.validateFields()
      if (elemModal.mode === 'create') {
        const parents = elemModal.parent ? [elemModal.parent] : []
        await runElem(
          () => api.elementUpsert(ontology.id, 'concept', { name: v.name.trim(), label: v.label ?? '', definition: v.definition ?? '', parents }),
          elemModal.parent ? `概念「${v.name.trim()}」已创建（父概念：${elemModal.parent}）` : `根概念「${v.name.trim()}」已创建`,
        )
      } else if (elemModal.concept) {
        const c = (spec?.concepts ?? []).find((x) => x.name === elemModal.concept)
        const add = Array.isArray(v.parent) ? v.parent : [v.parent]
        const next = [...new Set([...(c?.parents ?? []), ...add])]
        await runElem(
          () => api.elementUpsert(ontology.id, 'concept', { name: elemModal.concept, parents: next }),
          `「${elemModal.concept}」父集合已更新（共 ${next.length} 个）`,
        )
      }
    } catch (e: any) {
      if (!e?.errorFields) message.error(e?.message ?? '操作失败')
    }
  }
  const detachParent = (concept: string, parent: string) => {
    const c = (spec?.concepts ?? []).find((x) => x.name === concept)
    if (!c) return
    runElem(
      () => api.elementUpsert(ontology.id, 'concept', { name: concept, parents: (c.parents ?? []).filter((p) => p !== parent) }),
      `「${concept}」已摘除父概念「${parent}」`,
    )
  }
  const deleteConcept = (name: string) =>
    runElem(() => api.elementDelete(ontology.id, 'concept', name), `概念「${name}」已删除`)
  const deleteInstance = (name: string) =>
    runElem(() => api.elementDelete(ontology.id, 'instance', name), `实例「${name}」已删除`)

  // 树上快捷操作钮（hover 显隐；DAG 复现节点共享同一组操作——按概念名定位，不按树键）
  const classOps = (name: string) => (
    <span className="onto-hier-ops" data-testid={`hierarchy-ops-${name}`}>
      <Tooltip title={`在「${name}」下新建子概念`}>
        <button type="button" className="onto-hier-op" onClick={(e) => { e.stopPropagation(); setElemModal({ mode: 'create', parent: name }); form.resetFields() }}>＋子</button>
      </Tooltip>
      <Tooltip title={`为「${name}」挂父概念（多继承）`}>
        <button type="button" className="onto-hier-op" onClick={(e) => { e.stopPropagation(); setElemModal({ mode: 'attach', concept: name }); form.resetFields() }}>挂父</button>
      </Tooltip>
      <Popconfirm
        title={`删除概念「${name}」？`}
        description="若被关系/实例/子概念引用，校验会拒绝并给出悬空定位。"
        onConfirm={(e) => { e?.stopPropagation(); deleteConcept(name) }}
        onCancel={(e) => e?.stopPropagation()}
      >
        <button type="button" className="onto-hier-op danger" onClick={(e) => e.stopPropagation()}>删</button>
      </Popconfirm>
    </span>
  )
  const instanceOps = (name: string) => (
    <span className="onto-hier-ops">
      <Popconfirm title={`删除实例「${name}」？`} onConfirm={(e) => { e?.stopPropagation(); deleteInstance(name) }} onCancel={(e) => e?.stopPropagation()}>
        <button type="button" className="onto-hier-op danger" onClick={(e) => e.stopPropagation()}>删</button>
      </Popconfirm>
    </span>
  )
  const classTreeLive = useMemo(
    () => (spec ? buildClassTree(spec, onSaved ? classOps : undefined) : { data: [], index: new Map<string, NodeMeta>() }),
    [spec, onSaved, elemModal, busy], // 操作钮回调闭包含状态——操作态变化即重建（树数据本身派生自 spec）
  )
  const instTreeLive = useMemo(
    () => (spec ? buildInstanceTree(spec, onSaved ? instanceOps : undefined) : { data: [], index: new Map<string, NodeMeta>(), grouped: 0 }),
    [spec, onSaved, elemModal, busy],
  )
  const curLive = mode === 'class' ? classTreeLive : mode === 'instance' ? instTreeLive : propTree

  // 默认展开前两层（大 spec 下不全量展开；虚拟滚动兜底 REQ-304/305）
  useEffect(() => {
    const depth1: React.Key[] = []
    const walk = (ns: DataNode[], d: number) => {
      for (const n of ns) {
        if (d <= 1) {
          depth1.push(n.key)
          if (n.children) walk(n.children as DataNode[], d + 1)
        }
      }
    }
    walk(curLive.data, 0)
    setExpanded(depth1)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- spec/mode 变化即重置展开态
  }, [spec, mode])

  // E：外部选择（搜索/描述面板/图形视图总线）→ 切对应树 + 展开祖先链 + 高亮
  const selKeys = useMemo(() => {
    if (!selection) return []
    const keys: string[] = []
    for (const [k, m] of curLive.index) {
      if (m.sel.kind === selection.kind && m.sel.name === selection.name) keys.push(k)
    }
    return keys
  }, [selection, curLive])
  useEffect(() => {
    if (!selection) return
    const want: TreeMode = selection.kind === 'concept' ? 'class' : selection.kind === 'instance' ? 'instance' : 'prop'
    setMode((m) => (m === want ? m : want))
  }, [selection])
  useEffect(() => {
    if (selKeys.length === 0) return
    const chain: React.Key[] = []
    let k: string | null = selKeys[0]
    while (k) {
      chain.push(k)
      k = curLive.index.get(k)?.parentKey ?? null
    }
    setExpanded((prev) => {
      const s = new Set(prev)
      chain.forEach((x) => s.add(x))
      return [...s]
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- 选择定位时展开祖先
  }, [selKeys.join('|')])

  // E 全局搜索：name/label 子串跨四类元素（大小写不敏感）
  const hits = useMemo(() => {
    const ql = q.trim().toLowerCase()
    if (!spec || !ql) return []
    const out: { key: string; sel: OntoSelection; label: string; sub: string }[] = []
    for (const c of spec.concepts ?? []) {
      if (`${c.name} ${c.label ?? ''}`.toLowerCase().includes(ql))
        out.push({ key: `concept:${c.name}`, sel: { kind: 'concept', name: c.name }, label: c.label || c.name, sub: '概念' })
    }
    for (const r of spec.relations ?? []) {
      if (`${r.name} ${r.label ?? ''}`.toLowerCase().includes(ql))
        out.push({ key: `relation:${r.name}`, sel: { kind: 'relation', name: r.name }, label: r.label || r.name, sub: `关系 ${r.from || '—'}→${r.to || '—'}` })
    }
    for (const d of spec.data_properties ?? []) {
      if (`${d.name} ${d.label ?? ''}`.toLowerCase().includes(ql))
        out.push({ key: `dataprop:${d.name}`, sel: { kind: 'dataprop', name: d.name }, label: d.label || d.name, sub: `数据属性 ${d.range || 'string'}` })
    }
    for (const i of spec.instances ?? []) {
      if (i.name.toLowerCase().includes(ql)) out.push({ key: `instance:${i.name}`, sel: { kind: 'instance', name: i.name }, label: i.name, sub: `实例·属 ${i.concept || '—'}` })
    }
    return out.slice(0, 100)
  }, [spec, q])

  if (!spec) {
    return (
      <div className="work-empty" style={{ minHeight: 180 }}>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本体尚未保存 Spec（加载中或空本体）——保存后此处呈现层级浏览" />
      </div>
    )
  }

  const total = {
    concepts: (spec.concepts ?? []).length,
    relations: (spec.relations ?? []).length,
    dataprops: (spec.data_properties ?? []).length,
    instances: (spec.instances ?? []).length,
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10, flexWrap: 'wrap' }}>
        <Typography.Text strong>层级浏览</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          概念层级（R10 文案人话化）：树形浏览与选择联动，与图形视图互补
        </Typography.Text>
        <span style={{ flex: 1 }} />
        <AutoComplete
          value={q}
          onChange={setQ}
          onSelect={(_, opt: any) => {
            if (opt?.sel) {
              onSelect(opt.sel)
              setQ('')
            }
          }}
          options={hits.map((h) => ({ value: h.key, label: (
            <span>
              <Tag style={{ margin: '0 6px 0 0', fontSize: 10, lineHeight: '16px', padding: '0 4px' }}>{h.sub.split(' ')[0]}</Tag>
              {h.label}
              <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{h.sub}</Typography.Text>
            </span>
          ), sel: h.sel }))}
          style={{ width: 280 }}
          size="small"
          placeholder="全局搜索：概念 / 关系 / 属性 / 实例"
          allowClear
          data-testid="hierarchy-search"
        />
      </div>
      <div style={{ marginBottom: 8, display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
        <Segmented
          size="small"
          value={mode}
          onChange={(v) => setMode(v as TreeMode)}
          options={[
            { value: 'class', label: `类层级（${total.concepts}）` },
            { value: 'instance', label: `实例（${total.instances}）` },
            { value: 'prop', label: `属性（${total.relations + total.dataprops}）` },
          ]}
          data-testid="hierarchy-segment"
        />
        {/* REQ-502：常驻诊断条——save=false 快评摘要（保存后自动刷新），下钻 Findings */}
        {digest && (
          <Popover
            trigger="click"
            placement="bottomLeft"
            title={`质量快评（未保存诊断 · 不落 artifact）——综合 ${digest.score?.overall?.toFixed(1) ?? '—'}`}
            content={
              <div style={{ maxWidth: 420 }}>
                {(digest.findings ?? []).length === 0 ? (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>无 Findings——全部检查通过。</Typography.Text>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4, maxHeight: 320, overflowY: 'auto' }}>
                    {(digest.findings ?? []).map((f, i) => (
                      <div key={i} style={{ fontSize: 12, lineHeight: 1.6 }}>
                        <Tag color={f.severity === 'error' ? 'red' : f.severity === 'warning' ? 'orange' : 'default'} style={{ margin: 0, fontSize: 10 }}>
                          {f.severity}
                        </Tag>
                        {f.title}{f.count > 1 ? `（×${f.count}）` : ''}
                        {f.samples && f.samples.length > 0 && (
                          <Typography.Text type="secondary" style={{ fontSize: 11 }}> — {f.samples.slice(0, 3).join('、')}</Typography.Text>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            }
          >
            <span data-testid="hier-quality-digest" style={{ cursor: 'pointer' }}>
              {digest.error_count > 0 && <Tag color="red" style={{ margin: 0, fontSize: 11 }}>错误 {digest.error_count}</Tag>}
              {digest.warning_count > 0 && <Tag color="orange" style={{ margin: 0, fontSize: 11 }}>警告 {digest.warning_count}</Tag>}
              {digest.error_count === 0 && digest.warning_count === 0 && <Tag style={{ margin: 0, fontSize: 11 }}>诊断通过</Tag>}
            </span>
          </Popover>
        )}
        {/* REQ-502：回滚到上一保存版本（快照栈接线——不新做命令级 undo） */}
        {ontoVersion != null && ontoVersion > 0 && (
          <Popconfirm
            title={`回滚到上一保存版本（v${ontoVersion}）？`}
            description="指定版本内容将恢复为新版本保存。"
            okText="回滚"
            cancelText="取消"
            onConfirm={async () => {
              setRollingBack(true)
              try {
                await api.restoreOntologyVersion(ontology.id, ontoVersion)
                message.success(`已从 v${ontoVersion} 恢复为新版本`)
                setElemErrors(null)
                setDigestTick((t) => t + 1)
                fetchVersion()
                onSaved?.()
              } catch (e: any) {
                message.error(e?.message ?? '回滚失败')
              } finally {
                setRollingBack(false)
              }
            }}
          >
            <Tooltip title="回滚到上一保存版本（快照栈）">
              <Button size="small" type="text" aria-label="回滚到上一保存版本" icon={<RollbackOutlined />} loading={rollingBack} />
            </Tooltip>
          </Popconfirm>
        )}
        {mode === 'prop' && (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            按 Domain 分组呈现——spec_json 无 subPropertyOf，此为分组形态非语义属性层级（真属性层级 P2）
          </Typography.Text>
        )}
        {/* REQ-448：树上编辑入口——根新增（类树）；「重排 parents」以挂父/摘父达成，拖拽留观察 */}
        {mode === 'class' && onSaved && (
          <Button
            size="small"
            icon={<PlusOutlined />}
            data-testid="hierarchy-add-root"
            onClick={() => { setElemModal({ mode: 'create' }); form.resetFields() }}
          >
            根概念
          </Button>
        )}
        {mode === 'class' && onSaved && (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            树上「＋子 / 挂父 / 删」直接编辑（走元素端点，校验门禁同源）；层级重排=挂父/摘父
          </Typography.Text>
        )}
      </div>
      <Splitter className="onto-hier-split" orientation="horizontal" style={{ height: 560 }}>
        <Splitter.Panel defaultSize="44%" min="26%" max="72%">
          <div className="onto-hier-tree-wrap">
            {curLive.data.length === 0 ? (
              <div className="work-empty" style={{ minHeight: 160 }}>
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={
                    mode === 'class'
                      ? (onSaved ? '本 Spec 无概念——点上方「根概念」直接创建第一个概念（树上编辑）' : '本 Spec 无概念——在 Spec 编辑或图形编辑中添加后呈现类层级')
                      : mode === 'instance'
                        ? '本 Spec 无实例——CSV 灌装或构建生成实例后按类型分组呈现'
                        : '本 Spec 无关系与数据属性'
                  }
                />
              </div>
            ) : (
              <Tree
                blockNode
                multiple // DAG 多继承复现节点需同时高亮（controlled selectedKeys 多键，REQ-363 A/E）
                height={540}
                treeData={curLive.data}
                selectedKeys={selKeys}
                expandedKeys={expanded}
                onExpand={(k) => setExpanded(k)}
                onSelect={(_, info) => {
                  const meta = curLive.index.get(String((info.node as any).key))
                  if (meta) onSelect(meta.sel)
                }}
                data-testid="hierarchy-tree"
                className="onto-hier-tree"
              />
            )}
          </div>
        </Splitter.Panel>
        <Splitter.Panel min="28%">
          {selection ? (
            <DescPanel ontology={ontology} spec={spec} sel={selection} onSelect={onSelect} onDetachParent={onSaved ? detachParent : undefined} onChanged={onSaved} />
          ) : (
            <div className="work-empty" style={{ minHeight: 160 }}>
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="点击左侧树节点 / 搜索结果查看实体详情（描述面板 + usage 反向引用）" />
            </div>
          )}
        </Splitter.Panel>
      </Splitter>
      {/* REQ-502：元素操作失败结构化明细表（path+message——对齐 SpecEditorPane 规格） */}
      {elemErrors && elemErrors.length > 0 && (
        <div style={{ marginTop: 10 }} data-testid="hier-elem-errors">
          <Space size={6} style={{ marginBottom: 4 }}>
            <Typography.Text type="danger" style={{ fontSize: 12 }}>元素操作失败（{elemErrors.length} 项）</Typography.Text>
            <Button size="small" type="text" onClick={() => setElemErrors(null)}>收起</Button>
          </Space>
          <Table<ValidationError>
            rowKey={(_, i) => `${i}`}
            columns={ERR_COLUMNS}
            dataSource={elemErrors}
            pagination={false}
            size="small"
          />
        </div>
      )}
      {/* REQ-448：新建（挂选定父/根）与挂父 Modal——走元素端点 kind=concept（presence 语义只传 parents 即改父集） */}
      <Modal
        open={!!elemModal}
        title={
          elemModal?.mode === 'create'
            ? elemModal?.parent
              ? `在「${elemModal.parent}」下新建子概念`
              : '新建根概念'
            : `为「${elemModal?.concept}」挂父概念`
        }
        okText={elemModal?.mode === 'create' ? '创建' : '挂父'}
        okButtonProps={{ loading: busy }}
        onCancel={() => setElemModal(null)}
        cancelText="取消"
        onOk={submitElemModal}
      >
        <Form form={form} layout="vertical">
          {elemModal?.mode === 'create' ? (
            <>
              <Form.Item name="name" label="概念名（唯一标识）" rules={[{ required: true, message: '请输入概念名' }]}>
                <Input placeholder="如 交换机" data-testid="hierarchy-new-name" />
              </Form.Item>
              <Form.Item name="label" label="显示名（可选）">
                <Input />
              </Form.Item>
              <Form.Item name="definition" label="定义（可选）">
                <Input.TextArea rows={2} />
              </Form.Item>
              {elemModal.parent && (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  将挂父概念「{elemModal.parent}」（创建后可在描述面板 ✕ 摘父 / 树上「挂父」增父）
                </Typography.Text>
              )}
            </>
          ) : (
            <>
              <Form.Item name="parent" label="父概念（可多选=多继承）" rules={[{ required: true, message: '请选择父概念' }]}>
                <Select
                  mode="multiple"
                  showSearch
                  optionFilterProp="label"
                  placeholder="选择已有概念"
                  options={(spec?.concepts ?? [])
                    .filter((c) => c.name !== elemModal?.concept)
                    .map((c) => ({ value: c.name, label: c.label || c.name }))}
                />
              </Form.Item>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                将并入该概念既有父集合（去重）；若造成层级环，服务端校验会拒绝并如实回喂。
              </Typography.Text>
            </>
          )}
        </Form>
      </Modal>
    </div>
  )
}
