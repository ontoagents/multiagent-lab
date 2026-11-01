/**
 * 平台知识/文档互引的仓库内 .md 路径解析（REQ-161/169/188）。
 * 从 ReferencePage 抽出共享：正文相对引用点击（主题页与 DocViewerModal 抽屉内）共用同一套解析口径。
 */


/** 浏览器环境无 node:path，内联 basename */
function baseName(p: string): string {
  return p.slice(p.lastIndexOf('/') + 1)
}

/**
 * docs/ 根编号档 → 仓库相对路径（硬编码：docs/ 根是编号唯一事实源，路径稳定）。
 * docs/research 内的编号档不在此维护——KB_BY_NO 构建期自动派生（REQ-188），
 * 档案移动/改名互引不再断链。REQ-570 例外：'04' 原档 docs/04_本体_方案设计 已迁
 * docs/handbook/04_本体/92 号（2026-11-01 批次1.5），编号 04 互引指到现状档
 * （'12' 账本壳 /'14' 已退役仍留守 docs 根，指路不变）。
 */
const DOCS_BY_NO: Record<string, string> = {
  '01': 'docs/01_智能体_需求文档_PRD.md',
  '02': 'docs/02_智能体_技术方案设计.md',
  '03': 'docs/03_本体_需求文档.md',
  '04': 'docs/handbook/04_本体/92_本体_方案设计_架构现状.md',
  '11': 'docs/11_知识库_需求文档.md',
  '12': 'docs/12_知识库_方案设计.md',
  '14': 'docs/14_本体_前端改造方案.md',
  '15': 'docs/15_开源项目及论文登记簿.md',
  '18': 'docs/18_REQ编号注册表.md',
  '20': 'docs/20_回归冒烟清单.md',
}

/** docs/research 编号档自动派生（REQ-570 四区收敛：派生源由 platform-knowledge/** 收窄为调研区；
 *  handbook/learning 档不派生）：只取 glob 的键（路径），不加载内容（?url 资产化，避免 md 被 JS 解析）；
 *  同号取排序首条（目录间同号并存时取路径序首条，防御性兜底） */
const KB_KEYS = Object.keys(
  import.meta.glob('../../../docs/research/**/*.md', { query: '?url', import: 'default' }),
).sort()
const KB_BY_NO: Record<string, string> = (() => {
  const out: Record<string, string> = {}
  for (const k of KB_KEYS) {
    const i = k.indexOf('docs/research/')
    if (i < 0) continue
    const base = baseName(k)
    const m = /^(\d+)[_-]/.exec(base)
    if (m && !(m[1] in out)) out[m[1]] = k.slice(i)
  }
  return out
})()

/** 文档编号 → 仓库相对路径（frontmatter docs/decisions 指针用）；docs/ 编号优先 */
export const DOC_FILE_BY_NO: Record<string, string> = { ...KB_BY_NO, ...DOCS_BY_NO }

/** 百分号编码防御性解码（XMarkdown 部分渲染路径会对中文 href 做 encodeURI；畸形序列原样返回） */
function safeDecode(s: string): string {
  if (!s.includes('%')) return s
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

/** 相对引用解析（REQ-161 补充要求：文档互引用相对路径）——按所在仓库目录解析，返回仓库相对 .md 路径或 null */
export function resolveRef(href: string, base: string): string | null {
  const h = safeDecode(href.trim())
  if (!h || h.startsWith('http://') || h.startsWith('https://') || h.startsWith('#') || h.startsWith('mailto:')) return null
  if (h.startsWith('/')) return null
  const joined = /^(platform-knowledge|docs|research|seeds)\//.test(h) ? h : `${base}/${h}`
  const parts: string[] = []
  for (const seg of joined.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') parts.pop()
    else parts.push(seg)
  }
  const out = parts.join('/')
  return out.endsWith('.md') ? out : null
}

export function docFileOf(ptr: string): string | null {
  const s = ptr.trim()
  // 完整路径直传（docs/ 下含 handbook/research/learning 三子区前缀天然覆盖；research/ 与
  // platform-knowledge/ 旧前缀保留直传——旧文档历史链接点击后由后端退役指路 400 承接
  // （REQ-184 收尾 2026-10-22 + REQ-570 2026-11-01），不静默映射错文件）
  if (s.endsWith('.md') && (s.startsWith('docs/') || s.startsWith('research/') || s.startsWith('platform-knowledge/'))) return s
  // 编号指针（如 "02" / "02 §6.4" / "17 §2.2"）：取前两位编号映射
  const no = s.slice(0, 2)
  return DOC_FILE_BY_NO[no] ?? null
}
