import { useEffect, useMemo, useState } from 'react'
import { AutoComplete, Empty, Segmented, Splitter, Tag, Tooltip, Tree, Typography } from 'antd'
import type { DataNode } from 'antd/es/tree'
import type { Ontology, Spec, SpecConcept, SpecDataProperty, SpecInstance, SpecRelation } from '../../../../api/types'
import './hierarchy.css'

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

/** A 类层级树：Concept.Parents DAG → 树投影（多继承跨父复现；悬空父归根+标注；环防护） */
function buildClassTree(spec: Spec) {
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

/** B 实例树：按 Instance.Concept 类型分组；未声明类型/悬空归「未分组」 */
function buildInstanceTree(spec: Spec) {
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
          <NodeTitle name={it.name} extra={<span className="onto-hier-count">属 {it.concept}</span>} />
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
          title: <NodeTitle name={it.name} extra={<span className="onto-hier-count">属 {it.concept || '—'}</span>} />,
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

/** D 实体描述面板：选中实体聚合呈现（Protégé Description view 对应物） */
function DescPanel(props: { ontology: Ontology; spec: Spec; sel: OntoSelection; onSelect: (s: OntoSelection) => void }) {
  const { ontology, spec, sel, onSelect } = props
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
                    <Tag key={p.name} style={{ margin: '0 6px 4px 0', cursor: 'pointer' }} onClick={() => onSelect({ kind: 'concept', name: p.name })}>
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
}

export default function HierarchyPane({ ontology, spec, selection, onSelect }: HierarchyPaneProps) {
  const [mode, setMode] = useState<TreeMode>('class')
  const [q, setQ] = useState('')
  const [expanded, setExpanded] = useState<React.Key[]>([])

  const classTree = useMemo(() => (spec ? buildClassTree(spec) : { data: [], index: new Map<string, NodeMeta>() }), [spec])
  const instTree = useMemo(() => (spec ? buildInstanceTree(spec) : { data: [], index: new Map<string, NodeMeta>(), grouped: 0 }), [spec])
  const propTree = useMemo(() => (spec ? buildPropTree(spec) : { data: [], index: new Map<string, NodeMeta>() }), [spec])
  const cur = mode === 'class' ? classTree : mode === 'instance' ? instTree : propTree

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
    walk(cur.data, 0)
    setExpanded(depth1)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- spec/mode 变化即重置展开态
  }, [spec, mode])

  // E：外部选择（搜索/描述面板/图形视图总线）→ 切对应树 + 展开祖先链 + 高亮
  const selKeys = useMemo(() => {
    if (!selection) return []
    const keys: string[] = []
    for (const [k, m] of cur.index) {
      if (m.sel.kind === selection.kind && m.sel.name === selection.name) keys.push(k)
    }
    return keys
  }, [selection, cur])
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
      k = cur.index.get(k)?.parentKey ?? null
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
          Protégé 式树形导航（REQ-363）：树=导航式查询，与图形视图互补（选择联动）
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
        {mode === 'prop' && (
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            按 Domain 分组呈现——spec_json 无 subPropertyOf，此为分组形态非语义属性层级（真属性层级 P2）
          </Typography.Text>
        )}
      </div>
      <Splitter className="onto-hier-split" orientation="horizontal" style={{ height: 560 }}>
        <Splitter.Panel defaultSize="44%" min="26%" max="72%">
          <div className="onto-hier-tree-wrap">
            {cur.data.length === 0 ? (
              <div className="work-empty" style={{ minHeight: 160 }}>
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={
                    mode === 'class'
                      ? '本 Spec 无概念——在 Spec 编辑或图形编辑中添加后呈现类层级'
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
                treeData={cur.data}
                selectedKeys={selKeys}
                expandedKeys={expanded}
                onExpand={(k) => setExpanded(k)}
                onSelect={(_, info) => {
                  const meta = cur.index.get(String((info.node as any).key))
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
            <DescPanel ontology={ontology} spec={spec} sel={selection} onSelect={onSelect} />
          ) : (
            <div className="work-empty" style={{ minHeight: 160 }}>
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="点击左侧树节点 / 搜索结果查看实体详情（描述面板 + usage 反向引用）" />
            </div>
          )}
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
