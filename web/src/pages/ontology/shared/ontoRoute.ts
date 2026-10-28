// REQ-470/M265：本体模块 hash 路由——栏位/构建路径/资产详情与分区/运行引擎/学习子模块进 URL
// （`/ontology#/onto/...`）。localStorage 降级为「记忆」而非唯一载体：后退/前进在模块内可用、
// 刷新保持详情分区、URL 可分享；既有一次性深链键（eino.onto.asset.focus 等）语义不破（消费后
// 各页 effect 会回写 hash）。写入约定：页级跳转 push（进历史栈）、页内细化 replace（不灌栈）。
import { useUI } from '../../../store/ui'
export type OntoCol = 'learn' | 'build' | 'assets' | 'runtime' | 'audit'

export type OntoRoute = {
  col: OntoCol
  /** build：路径键（home/custom/ontochat/kb/ontoextend/catalog/kg） */
  path?: string
  /** assets：详情态本体 id（缺省=列表态） */
  assetId?: string
  /** assets：详情分区键（spec/hierarchy/graph-edit/decl/axioms/ingest/quality/versions/evolution/graph/outputs…） */
  section?: string
  /** runtime：引擎键（oxigraph/fuseki/cayley） */
  engine?: string
  /** learn：子模块键（path/build-paths/runtime-paths/examples/pipeline） */
  learn?: string
}

const COLS: OntoCol[] = ['learn', 'build', 'assets', 'runtime', 'audit']
const BUILD_PATHS = ['home', 'custom', 'ontochat', 'kg', 'ontoextend', 'kb', 'catalog']
const ENGINES = ['oxigraph', 'fuseki', 'cayley']
const LEARN_SECTIONS = ['path', 'build-paths', 'runtime-paths', 'examples', 'pipeline']

/** 解析当前（或给定）hash 为路由；非本模块 hash 返回 null。id 类段容忍字母数字-_。 */
export function parseOntoHash(hash?: string): OntoRoute | null {
  const h = hash ?? (typeof window !== 'undefined' ? window.location.hash : '')
  const m = h.match(/^#\/onto\/([a-z]+)(?:\/([A-Za-z0-9_-]+))?(?:\/([A-Za-z0-9_-]+))?/)
  if (!m) return null
  const col = m[1] as OntoCol
  if (!COLS.includes(col)) return null
  const r: OntoRoute = { col }
  if (col === 'build' && m[2] && BUILD_PATHS.includes(m[2])) r.path = m[2]
  if (col === 'assets') {
    if (m[2]) r.assetId = m[2]
    if (m[3]) r.section = m[3]
  }
  if (col === 'runtime' && m[2] && ENGINES.includes(m[2])) r.engine = m[2]
  if (col === 'learn' && m[2] && LEARN_SECTIONS.includes(m[2])) r.learn = m[2]
  return r
}

/** 路由 → hash 字符串（不写历史）。 */
export function ontoHash(r: OntoRoute): string {
  const seg = ['onto', r.col]
  if (r.col === 'build' && r.path) seg.push(r.path)
  if (r.col === 'assets' && r.assetId) {
    seg.push(r.assetId)
    if (r.section) seg.push(r.section)
  }
  if (r.col === 'runtime' && r.engine) seg.push(r.engine)
  if (r.col === 'learn' && r.learn) seg.push(r.learn)
  return '#/' + seg.join('/')
}

/** 写 hash（replace=不进历史栈）。同 hash 幂等不写。 */
export function writeOntoHash(r: OntoRoute, replace = false) {
  if (typeof window === 'undefined') return
  const h = ontoHash(r)
  if (window.location.hash === h) return
  if (replace) history.replaceState(null, '', h)
  else history.pushState(null, '', h)
}

/** 当前 hash 是否本模块路由（用于各页 effect 防误抢他栏 hash）。 */
export function isOntoHashActive(): boolean {
  return parseOntoHash() !== null
}

/**
 * 跨模块深链（REQ-470 收敛通道）：legacy 键照写（既有语义不破）+ setPage 后补写 hash
 * （setPage 的 pushState 仅带路径会清 hash，故必须在其后 replaceState 合并写回）。
 */
export function deepLinkOnto(r: OntoRoute, legacyPairs: Array<[string, string]> = [], sidebarKey = 'eino.onto.sidebar') {
  for (const [k, v] of legacyPairs) {
    try { localStorage.setItem(k, v) } catch { /* 忽略 */ }
  }
  try {
    localStorage.setItem(sidebarKey, r.col)
    if (r.col === 'build' && r.path) {
      localStorage.setItem('eino.onto.buildPath', r.path)
      localStorage.setItem('eino.onto.buildPath.homeMigrated', '1') // 防 REQ-337 一次性迁移吃掉显式值（沿 REQ-360 顺修）
    }
  } catch { /* 忽略 */ }
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  useUI.getState().setPage('ontology')
  history.replaceState(null, '', '/ontology' + ontoHash(r))
}
