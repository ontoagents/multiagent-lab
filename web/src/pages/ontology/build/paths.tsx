// REQ-337/M137：构建路径共享常量（自 BuildPage 迁出——六路径页面化的单源定义）。
// 路径记忆沿用 eino.onto.buildPath；首页收敛为路径选择页（'home'），
// 存量路径记忆一次性迁移 'home'（沿 D-O11 learn→build 迁移先例：此后显式选择仍记忆）。
export type BuildPath = 'home' | 'custom' | 'ontochat' | 'kg' | 'ontoextend' | 'kb' | 'catalog'

export const PATHS: { key: Exclude<BuildPath, 'home'>; label: string; state: 'ok' | 'partial' | 'guide'; desc: string }[] = [
  { key: 'custom', label: '自定义构建', state: 'ok', desc: 'S1 来源 → S2 编辑 → S3 校验 → S4 可视化（七阶段前 4 步）' },
  { key: 'ontochat', label: 'OntoChat 流程', state: 'ok', desc: '对话式 CQ 引导 → 逐轮补全 → 草稿入库（REQ-103 模式 A）' },
  { key: 'kb', label: '由知识库构建', state: 'partial', desc: 'KB chunk→LLM / KG→直转 / 混合三策略独立流程页（O13，D-O14/REQ-108）' },
  { key: 'kg', label: 'KG 消费流程', state: 'guide', desc: '入口卡跳转「消费与审计」栏（D-O15 自研 KG，原 semantica 流程改造）' },
  { key: 'ontoextend', label: 'OntoExtend 流程', state: 'partial', desc: 'ODP 模式推荐 + LOV 词表扩展 → 审查入库（M-O14 P2②）' },
  { key: 'catalog', label: '权威本体库', state: 'ok', desc: '查询并引入业界权威本体（W3C/ETSI/DCMI），运行与拓展零门槛（REQ-316）' },
]

export const STATE_TAG: Record<Exclude<BuildPath, 'home'>, { color: string; text: string }> = {
  custom: { color: 'green', text: '可用' },
  ontochat: { color: 'green', text: '可用' },
  kb: { color: 'orange', text: '部分可用' },
  kg: { color: 'cyan', text: '引导' },
  ontoextend: { color: 'gold', text: '部分可用' },
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
