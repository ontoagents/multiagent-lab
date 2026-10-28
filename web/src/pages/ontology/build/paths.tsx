// REQ-337/M137：构建路径共享常量（自 BuildPage 迁出——六路径页面化的单源定义）。
// 路径记忆沿用 eino.onto.buildPath；首页收敛为路径选择页（'home'），
// 存量路径记忆一次性迁移 'home'（沿 D-O11 learn→build 迁移先例：此后显式选择仍记忆）。
export type BuildPath = 'home' | 'custom' | 'ontochat' | 'kg' | 'ontoextend' | 'kb' | 'catalog'

// REQ-405/M201：六卡按生产频率重排+决策判据文案（有文件→导入 / 有语料→KB / 改已有→演化 /
// 要标准→权威库；示例/空白→自定义 / KG 消费降侧栏）——「选哪条路」一屏可判。
export const PATHS: { key: Exclude<BuildPath, 'home'>; label: string; state: 'ok' | 'partial' | 'guide'; desc: string; when: string; novice?: boolean }[] = [
  // 生产最重场景（80 号 G3/U3）：已交付本体的增量演化比新建更高频
  // REQ-484/M283：3+3 新手主推序（第一行=新手三路径 OntoChat/自定义/知识库；第二行=进阶）；
  // 用户面文案去 REQ/M 内部编号（台账语言归文档，UI 只说人话）。
  { key: 'ontochat', label: 'OntoChat 对话式', state: 'ok', when: '从一段描述访谈成型', desc: '① 用户故事访谈 → ② 能力问题 → ③ 领域补全 → ④ 草稿确认 → ⑤ 入库', novice: true },
  { key: 'custom', label: '自定义构建 / 导入', state: 'ok', when: '有 OWL/TTL 文件或从零开始', desc: '① 来源（导入/AI/空白）→ ② 编辑 → ③ 校验 → ④ 可视化' },
  { key: 'kb', label: '由知识库构建', state: 'partial', when: '知识库已有语料', desc: '从知识库语料生成本体：chunk→LLM 抽取 / KG 直转 / 混合三策略' },
  { key: 'ontoextend', label: 'OntoExtend 演化', state: 'ok', when: '改已有本体', desc: '对已交付本体做增量扩展：ODP 模式或按能力问题生成扩展片段' },
  { key: 'catalog', label: '权威本体库', state: 'ok', when: '要业界标准（W3C/ETSI/DCMI）', desc: '查询并引入业界权威本体，入库即可运行与拓展' },
  { key: 'kg', label: 'KG 消费（引导）', state: 'guide', when: '要看 KG 抽取与消费', desc: '跳转「消费与审计」栏——KG 图谱与 GraphRAG 消费动线' },
]

export const STATE_TAG: Record<Exclude<BuildPath, 'home'>, { color: string; text: string }> = {
  custom: { color: 'green', text: '可用' },
  ontochat: { color: 'green', text: '可用' },
  kb: { color: 'orange', text: '部分可用' },
  kg: { color: 'cyan', text: '引导' },
  ontoextend: { color: 'green', text: '演化路径' },
  catalog: { color: 'green', text: '可用' },
}

export const ONTO_BUILD_PATH_KEY = 'eino.onto.buildPath'
const MIGRATED_KEY = 'eino.onto.buildPath.homeMigrated'

const VALID: BuildPath[] = ['home', 'custom', 'ontochat', 'kb', 'kg', 'ontoextend', 'catalog']

export function readBuildPath(): BuildPath {
  const v = localStorage.getItem(ONTO_BUILD_PATH_KEY)
  // 首页=路径选择页收敛的一次性迁移（沿 D-O11 learn→build 先例）：
  // 存量路径值首读迁 'home'（新首页口径落地一次），此后显式选择仍记忆（select 写回）。
  if (localStorage.getItem(MIGRATED_KEY) !== '1') {
    localStorage.setItem(MIGRATED_KEY, '1')
    localStorage.setItem(ONTO_BUILD_PATH_KEY, 'home')
    return 'home'
  }
  return v && VALID.includes(v as BuildPath) ? (v as BuildPath) : 'home'
}
