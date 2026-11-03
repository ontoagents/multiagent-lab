// REQ-337/M137：构建路径共享常量（自 BuildPage 迁出——路径页面化的单源定义）。
// REQ-598/M409 Hub+独占路径页：路径记忆 eino.onto.buildPath 语义微调为「最后访问的
// 路径页」（hub 不写此键——含 deepLinkOnto 的 hub 形态），刷新/重进构建栏恢复到
// 最后访问的路径页，无记录落 Hub；REQ-337 一次性迁移逻辑退役（hub 成为缺省，存量
// 路径值首读直接落对应页）。kg 退役为重定向（不再是路径——#3.3-⑧，旧深链不断链）。
export type BuildPath = 'home' | 'custom' | 'ontochat' | 'kb' | 'structured' | 'parking' | 'ontoextend' | 'catalog' | 'datasource'

// 生产最重场景（80 号 G3/U3）：已交付本体的增量演化比新建更高频。
// 用户面文案去 REQ/M 内部编号（台账语言归文档，UI 只说人话）。
export const PATHS: { key: Exclude<BuildPath, 'home'>; label: string; state: 'ok' | 'partial'; desc: string; when: string; novice?: boolean }[] = [
  // REQ-484/M283：新手最常见「一段描述」置顶（Hub 区块 B 分组首行）
  { key: 'ontochat', label: 'OntoChat 对话式', state: 'ok', when: '从一段描述访谈成型', desc: '① 用户故事访谈 → ② 能力问题 → ③ 领域补全 → ④ 草稿确认 → ⑤ 入库' },
  { key: 'custom', label: '自定义构建 / 导入', state: 'ok', when: '有 OWL/TTL 文件或从零开始', desc: '① 来源（导入/AI/空白）→ ② 编辑 → ③ 校验 → ④ 可视化' },
  { key: 'kb', label: '由知识库构建', state: 'partial', when: '知识库已有语料', desc: '从知识库语料生成本体：chunk→LLM 抽取 / KG 直转 / 混合三策略（KG 直转需先建 KG）' },
  { key: 'structured', label: 'CSV / JSON 表格映射', state: 'ok', when: '有 CSV / JSON 表格数据', desc: '表格数据直接映射本体骨架：首列→实例、其余列→属性（类型推断），或低基数列→概念层级链' },
  { key: 'parking', label: '概念停车场', state: 'ok', when: '有学习材料（笔记/文档节选）', desc: '粘贴/上传学习材料 → AI 铺候选概念 → 勾选成骨架（AI 补层级关系+可同步建知识库）' },
  { key: 'ontoextend', label: 'OntoExtend 演化', state: 'ok', when: '改已有本体', desc: '对已交付本体做增量演化扩展：ODP 模式或按能力问题生成扩展片段' },
  { key: 'catalog', label: '权威本体库', state: 'ok', when: '要业界标准（W3C/ETSI/DCMI）', desc: '查询并引入业界权威本体，入库即可运行与拓展' },
  { key: 'datasource', label: '由数据源构建', state: 'partial', when: '已有结构化数据', desc: '连接 SQLite 只读内省：表→概念、列→属性、外键→关系，AI 补显示名与描述，人审后落库' },
]

// REQ-598：诚实状态徽标文案体系修正——可用 / 部分可用（前置条件见一句话描述）/ 一期范围。
// 「演化路径」类别混入状态的旧口径退役（演化属性归一句话描述）。
export const STATE_TAG: Record<Exclude<BuildPath, 'home'>, { color: string; text: string }> = {
  custom: { color: 'green', text: '可用' },
  ontochat: { color: 'green', text: '可用' },
  structured: { color: 'green', text: '可用' },
  parking: { color: 'green', text: '可用' },
  kb: { color: 'orange', text: '部分可用' },
  ontoextend: { color: 'green', text: '可用' },
  catalog: { color: 'green', text: '可用' },
  datasource: { color: 'orange', text: '一期仅 SQLite' },
}

export const ONTO_BUILD_PATH_KEY = 'eino.onto.buildPath'

const VALID: Exclude<BuildPath, 'home'>[] = ['custom', 'ontochat', 'kb', 'structured', 'parking', 'ontoextend', 'catalog', 'datasource']

/** 路径记忆读取：有效路径页值 → 原值；'home'/无效/无记录 → Hub（'home' 为内部哨兵，
 *  对外 hash 形态=无 path 段的 #/onto/build）。hub 与 deepLinkOnto 的 hub 形态均不写此键。 */
export function readBuildPath(): BuildPath {
  const v = localStorage.getItem(ONTO_BUILD_PATH_KEY)
  return v && VALID.includes(v as Exclude<BuildPath, 'home'>) ? (v as BuildPath) : 'home'
}
