// 与后端 §5 模型对齐的前端类型

export interface Agent {
  id: string
  name: string
  description: string
  instruction: string
  model_conn_id: string | null
  /** REQ-452/M248：备援模型连接清单（主连接持续失败时按序切换；空=不启用故障转移） */
  model_failover_conn_ids?: string[] | null
  temperature: number | null
  max_tokens: number | null
  max_iteration: number
  /** REQ-319 A2：作为项目成员被委派调用时的单次执行超时秒数（0=不限）；超时降级回喂协调者 */
  delegate_timeout_seconds?: number
  /** REQ-14：agent 级工具审批策略（''=关闭 | 'danger' 危险档 | 'all' 全部审批；会话级可覆盖）——REQ-219 顺修前端类型缺字段 */
  tool_approval?: string
  /** REQ-231②：审批豁免清单（danger/all 档下免审工具名） */
  approval_exempt?: string[]
  /** REQ-231③：审批挂起超时（小时；0=不限） */
  approval_timeout_hours?: number
  /** REQ-232/M59：agent 级 hooks 配置（fetchGuard 启停 + 自定义 deny 规则；缺省=全默认） */
  hooks?: { fetch_guard_disabled?: boolean; deny_rules?: { name: string; tool_pattern: string; args_pattern?: string; reason?: string }[] }
  tools: string[]
  skills: string[]
  mcp_servers: { name: string; url: string }[]
  runtime_backend: string
  /** M13 §6.16：推理后端（eino-adk 自研默认；claude-code/opencode/aider 外部 CLI） */
  inference_backend: string
  // REQ-137：非内置后端登记的原 logo 图标 URL（未配置回退默认图标）
  logo_url?: string
  /** M10/10b：docker 沙箱资源限制（runtime_backend=docker 时生效；空/0 = 默认 512m/1CPU） */
  sandbox_memory?: string
  sandbox_cpus?: number
  /** REQ-131/M18：对外 MCP 服务化（enabled/token/tool_name；token 为 Agent 级 Bearer） */
  mcp_serve?: { enabled: boolean; token?: string; tool_name?: string }
  /** M28/REQ-170：伴生本体开关（REQ-216 起为派生只读——companion_ontology_id 非空即开启） */
  companion_ontology?: boolean
  /** REQ-216①：伴生本体绑定（可空引用；空=未开启。开启交互=选择/一键创建空本体） */
  companion_ontology_id?: string
  /** REQ-186：内置助手标记（智能体列表分区置顶、内置徽标、无配置入口） */
  is_builtin?: boolean
  /** REQ-187：伴生本体配置增强 */
  companion_extract_hint?: string
  companion_extract_conn_id?: string
  /** REQ-303 C1：抽取高级配置 JSON（win_msgs/win_chars/max_windows/temperature；空=env 默认） */
  companion_extract_cfg?: string
  companion_auto_threshold?: number
  /** REQ-201/M37：上下文预算档位（'' = 标准档；compact 紧凑 / standard 标准 / full 完整不限量） */
  context_mode?: string
  /** REQ-396/M192：上下文压缩策略（空=本智能体模型 | off=仅剪枝 | 连接 id=低成本摘要） */
  compact_policy?: string
  /** REQ-202/M38：工作目录（文件原语安全根，SafeJoin 约束；空=仅项目会话文件工具） */
  work_dir?: string
  /** REQ-202/M38：verify_on_stop 验证命令（空=不验证；失败标记 verify_failed） */
  verify_command?: string
  /** REQ-214/M46：连接器授权白名单（实例 id 数组；agent 级最小权限第一层） */
  connectors?: string[]
  /** REQ-360/M161：对话建本体能力开关（''=关 | 'guided'=访谈 | 'quick'=快速）——
   *  开启时装配自动并入 ontochat_* 四工具+注入内置纪律段；产出清单见侧板分区 */
  onchat_mode?: string
  /** REQ-366/M164：本体操作工具能力开关（false=关 | true=开启）——开启时挂载本体的会话
   *  装配自动并入 facade 13 件扩展工具（spec CRUD/校验/CQ 基准）；写类入 danger 档审批 */
  onto_ops?: boolean
  created_at: string
  updated_at: string
}

/** REQ-214/M46：外部连接器（统一 agent 连接外部能力的产品抽象；凭据服务端加密绑定，永不回传） */
export interface Connector {
  id: string
  kind: 'mcp' | 'kubernetes' | 'ssh' | 'a2a' | 'cli' | 'stdio'
  name: string
  description: string
  config: Record<string, unknown>
  has_credentials: boolean
  status: 'unknown' | 'ok' | 'error'
  status_detail: string
  /** REQ-214 P2：工具名清单（test 落库；授权前知情） */
  tools: string[]
  /** REQ-214 P2：最近一次连接测试时间（状态时效性） */
  tested_at?: string
  /** REQ-416/M212：a2a 注册握手的 agent card / REQ-426：cli 注册探测 cli-probe 载荷（只读预览） */
  card?: Record<string, unknown>
  /** REQ-429/M225：成员能力画像（技能/在环性/成本档——连接器面呈现+协调者路由信号） */
  profile?: Record<string, unknown>
  is_builtin: boolean
  refs: string[]
  created_at: string
  updated_at: string
}

/** REQ-418/M214：协议面——外部 A2A 智能体卡片条目（/api/a2a/cards） */
export interface A2aCardEntry {
  agent_id: string
  name: string
  is_builtin: boolean
  card: Record<string, unknown>
}

/** REQ-418/M214：协议面——A2A 任务行（/api/a2a/tasks，a2a_task 投影+终态产物/错误） */
export interface A2aTaskInfo {
  id: string
  context_id: string
  agent_id: string
  state: string
  timestamp: string
  created_at: string
  updated_at: string
  artifact_text?: string
  error_text?: string
}

/** REQ-131/M18：mcp-serve 管理端点返回（Token 掩码 + 调用示例） */
export interface McpServeInfo {
  enabled: boolean
  tool_name: string
  endpoint: string
  token_mask?: string
  curl?: string
  configured: boolean
}

/** M13 §6.16：推理后端探测状态（GET /api/inference-backends） */
export interface InferenceBackendStatus {
  name: string
  available: boolean
  version?: string
  path?: string
  reason?: string
  default: boolean
  /** REQ-450：安装引导（引导安装非代为安装——平台不下载不托管） */
  install_cmd?: string
  install_docs?: string
  install_note?: string
  capabilities: {
    chat: boolean
    stream: boolean
    skills_mode: 'tools' | 'instruction' | 'none'
    mcp_mode: 'tools' | 'instruction' | 'none'
    agent_as_tool: boolean
    workflow: boolean
    resume: boolean
  }
}

export interface Project {
  id: string
  name: string
  description: string
  collab_mode: string
  workflow_mode: string
  constraints: string
  agent_ids: string[]
  coordinator: string
  /** REQ-101：绑定的本地目录（绝对路径；空串 = 未绑定） */
  local_dir: string
  /** REQ-550/M357 可选归属工作区（空 = 未归属；新会话经项目层转发工作区默认值） */
  workspace_id?: string | null
  created_at: string
  updated_at: string
}

// ---- M12 项目本地目录（REQ-101/102/103）----

/**
 * 目录检测结果（POST /api/projects/validate-dir；REQ-133 分字段直连）。
 * 前端逐字段渲染，不按本机 OS 规则推断路径形态：
 * - format_ok：路径为绝对形态（跨运行时口径）；
 * - reachable：路径形态与部署主机一致——false（远程部署，如 Linux 后端 + Windows 本机目录）时
 *   存在性未校验，exists/is_dir 为 null，不得显示「不存在/非目录」。
 */
export interface DirValidation {
  format_ok: boolean
  reachable: boolean
  exists: boolean | null
  is_dir: boolean | null
  is_git: boolean
  git_branch?: string | null
  git_commit?: string | null
  git_dirty?: boolean | null
  error?: string | null
}

/** 目录条目（GET /api/projects/{id}/dir-files） */
export interface ProjectDirEntry {
  name: string
  is_dir: boolean
  size: number
  mod_time: string
  /** git porcelain 状态（M/A/??/D 或语义词）；非 git 或未跟踪为空 */
  git_status?: string | null
}

export interface ProjectDirListing {
  path: string
  entries: ProjectDirEntry[] | null
}

// ---- M12 REQ-102 深度版：Git 视图（提交历史 / 分支 / 变更明细）----

/** 提交历史条目（GET /api/projects/{id}/git-log） */
export interface GitCommit {
  hash: string
  short: string
  author: string
  date: string
  subject: string
  /** decorations：HEAD -> main / origin/main / tag: x（去掉 HEAD -> 前缀） */
  refs?: string[]
  /** 父提交 ≥2 即合并提交 */
  merge?: boolean
}

/** 分支条目（GET /api/projects/{id}/git-branches） */
export interface GitBranch {
  name: string
  current: boolean
  is_remote: boolean
  short_commit: string
  date: string
}

/** 变更文件（numstat；add/del = -1 表示二进制或合并提交无统计） */
export interface GitFileChange {
  path: string
  add: number
  del: number
}

/** 工作区未提交变更（porcelain 状态码 + numstat） */
export interface GitWorkingFile {
  path: string
  code: string
  staged?: boolean
  add?: number | null
  del?: number | null
}

export interface Conversation {
  id: string
  scope: 'agent' | 'project'
  agent_id: string | null
  project_id: string | null
  /** REQ-548/M355 工作区归属（NULL=无归属存量行为不变；删工作区 SET NULL 回落智能体页） */
  workspace_id?: string | null
  title: string
  kb_id: string | null
  enable_kb: boolean
  runtime_profile_id: string | null
  ontology_enabled: boolean
  /** 会话级技能开关（后端列待跟进；前端按 `?? true` 兼容默认开） */
  enable_skills?: boolean
  /** 中断挂起信息 JSON（M11 收尾：ask_human 等待答复；空 = 无挂起） */
  interrupt_state?: string | null
  /** REQ-135②：对话级工具审批覆盖（空=跟随智能体级 | on | off） */
  tool_approval?: string | null
  /** REQ-291：会话级模型连接覆盖（空=跟随智能体默认；合并序 请求级 > 会话级 > 智能体默认） */
  model_conn_id?: string | null
  /** REQ-393/M189：会话级上下文预算档覆盖（空=跟随 Agent 级 | compact/standard/full；合并序 会话>Agent>全局） */
  context_mode?: string | null
  companion_ontology_id?: string | null
  top_k: number
  min_score: number
  /** REQ-202 todo_write 机器侧清单（[{"title","status"}]；REQ-311 B4 产物卡数据源） */
  todo_json?: string | null
  created_at: string
  updated_at: string
}

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  meta?: string
  created_at: string
}

export interface ModelConnection {
  id: string
  name: string
  conn_type: 'chat' | 'embedding'
  protocol: string
  base_url: string
  model_name: string
  api_key_hint: string
  has_key: boolean
  enabled: boolean
  is_default: boolean
  /** REQ-148 供应商分组 ID（分组标识与 BaseURL 解耦，同供应商可多实例） */
  provider_group_id?: string
  /** REQ-148 组别名快照（List/Get 联查返回，展示层用；仅显示不改真名） */
  provider_alias?: string
  created_at: string
  updated_at: string
}

/**
 * 自动发现模型（ASSUMED 后端契约，接口可能尚未就绪）：
 * POST /api/model-connections/{anchorId}/list-models
 * 后端解密锚点连接的 Key，调用其 OpenAI 兼容的 GET {base_url}/models，返回可用模型 id 列表。
 */
export interface ProviderModelList {
  models: string[]
}

/**
 * 使用统计（ASSUMED 后端契约，接口可能尚未就绪）：
 * GET /api/stats/usage?group_by=model|agent|project
 */
export type UsageGroupBy = 'model' | 'agent' | 'project'

export interface UsageRow {
  key: string
  label: string
  calls: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

export interface UsageStats {
  rows: UsageRow[]
}

/** SSE 平台事件（方案 §7 统一协议） */
export interface PlatformEvent {
  type: string
  run_id?: string
  ts?: string
  data?: any
}

/** run_event 持久化事件（GET /conversations/{id}/events，历史时间线回放） */
export interface RunEventDTO {
  id: string
  conversation_id: string
  run_id: string
  type: string
  data?: string
  created_at: string
}

// ---- M6 知识库（02 文档 §5.1/§6.9/§8） ----

export interface KnowledgeBase {
  id: string
  name: string
  description?: string
  /** REQ-451：平台官方库标注（platform-knowledge 平台知识库，后端按名称单源判定） */
  official?: boolean
  /** M14 D-KB4 双子模块 + REQ-246 第三类型：rag | graphrag | wiki（KB-11 起为展示页签默认；老数据缺省 = rag） */
  mode?: 'rag' | 'graphrag' | 'wiki'
  /** KB-11（M35/D2）：检索能力开关（解除 mode 互斥；两者皆空由后端按 mode 派生） */
  kb_vector?: boolean
  kb_graph?: boolean
  /** @deprecated 后端已移除该字段，仅新建表单兼容保留 */
  store_backend?: 'qdrant' | 'sqlite'
  top_k: number
  min_score: number
  /** M16/REQ-129①：库级 KG 抽取模型连接（空 = 默认 chat）与提示词覆写 */
  kg_conn_id?: string
  kg_prompt?: string
  /** M36/KB-6③：本体约束抽取挂载（本体侧只读词表注入 prompt 白名单；空 = 自由抽取） */
  kg_ontology_id?: string
  /** M36/KB-6①：库级抽取语料预算 chunks（0 = 默认 200） */
  kg_max_chunks?: number
  doc_count?: number
  chunk_count?: number
  created_at: string
  updated_at: string
}

/** graphrag 文档索引后的 KG 抽取结果（M14 ②⑥ 建制；D-O15 起自研抽取，degraded = 非阻断降级） */
export interface KBGraphragInfo {
  ok?: boolean
  method?: 'llm' | 'lightweight'
  chunks?: number
  entities?: number
  relationships?: number
  degraded?: boolean
  error?: string
  warnings?: string[]
}

export interface KBDoc {
  id: string
  kb_id: string
  title: string
  chunk_count?: number
  /** 向量维度（REQ-439/REQ-54；wiki 库零 embedding 为 0 如实零维） */
  dim?: number
  status: 'pending' | 'indexing' | 'success' | 'failed'
  error?: string
  graphrag?: KBGraphragInfo
  created_at: string
  updated_at?: string
}

/** 检索试运行命中（§6.9 retrieval 事件 hits 结构；B1 起附命中区间高亮） */
export interface KBHit {
  doc: string
  /** REQ-523/M330：溯源最后一跳——命中可下钻原文（graphrag 实体/claims 无文档为空） */
  doc_id?: string
  seq: number
  score: number
  excerpt: string
  strategy?: string
  /** B1 引用溯源：excerpt 内命中区间（rune 偏移，start 含 / end 不含） */
  spans?: { start: number; end: number }[]
  /** REQ-246：wiki 臂命中页溯源（chunk_id 解析为 doc 标题 + 序号，回答可下钻原文；非 wiki 臂为空） */
  sources?: { doc?: string; doc_id?: string; seq: number }[]
}

/** REQ-524/M331：学习自测出题（概念子图——answer 作答后才展示） */
export interface QuizItem {
  id: string
  ontology_id: string
  concept_names: string[]
  question: string
  answer: string
  difficulty: string
}

/** REQ-246（M70）：wiki 页面（LLM 摄取生成的互链 Markdown 页面层） */
export interface WikiPage {
  id: string
  kb_id: string
  page_type: 'summary' | 'entity' | 'concept' | 'topic' | 'synthesis' | 'index'
  title: string
  content_md: string
  /** 溯源 chunk_id 数组（页 → chunk 池下钻原文） */
  sources: string[]
  created_at: string
  updated_at: string
}

/** REQ-246：wiki 重建结果（成本可见：实际 LLM 调用次数 / 未变更跳过数） */
export interface WikiBuildResult {
  ok: boolean
  pages: number
  llm_calls: number
  skipped_docs: number
  degraded?: boolean
  error?: string
  warnings?: string[]
  duration_ms: number
}

/** 检索响应（M14 ③④：mode = 实际生效的检索路径；degraded = KG 无命中/异常回退向量） */
export interface KBSearchResult {
  kb_id: string
  mode?: 'rag' | 'graphrag' | 'wiki'
  degraded?: boolean
  error?: string
  hits: KBHit[]
}

// ---- O13 由知识库构建本体（REQ-108，04 §3.7；主平台 4 端点） ----

/** KB 选择器条目（GET /api/kbs/selectable-for-ontology-build） */
export interface OntoBuildSelectableKB {
  id: string
  name: string
  description?: string
  mode?: 'rag' | 'graphrag'
  doc_count: number
  chunk_count: number
  kg_entities: number
  kg_relationships: number
  kg_ready: boolean
  updated_at?: string
}

/** 生成侧结构校验报告（入库时构建平面 PUT spec 做权威校验） */
export interface OntoBuildSpecReport {
  ok: boolean
  errors: ValidationError[]
  warnings?: string[]
}

/** 构建平面统一质量快评（REQ-267/M76：qualitygate 统一口径 save=false 零副作用）；degraded=构建平面不可达，仅本地结构校验兜底 */
export interface BuildQualitySummary {
  overall: number
  error_count: number
  warning_count: number
  degraded?: boolean
}

/** build-from-kb 结果（spec_json 为构建平面 Spec 同形草稿；cqs = REQ-90 能力问题） */
export interface OntoBuildResult {
  kb_id: string
  kb_name: string
  strategy: 'chunk-llm' | 'kg-direct' | 'hybrid'
  method?: string
  cq_mode: 'auto' | 'custom' | 'skip'
  cqs?: string[]
  rounds: number
  chunks_used: number
  truncated?: boolean
  spec_json: Spec
  validation_report: OntoBuildSpecReport
  quality?: BuildQualitySummary
  warnings?: string[]
}

/** chunks-to-kg 结果（D-O15：POST /api/kg/{id}/rebuild 显式重建自存 KG；graphrag 复用 M14 GraphragInfo） */
export interface ChunksToKGResult {
  kb_id: string
  chunks: number
  graphrag: KBGraphragInfo
}

/** kg-to-spec-json 结果（策略 B 独立入口：KG 薄映射；定位=编程接口无 UI 面，REQ-267③ 收口） */
export interface KGToSpecResult {
  kb_id: string
  method?: string
  kg_entities: number
  kg_relationships: number
  spec_json: Spec
  validation_report: OntoBuildSpecReport
  quality?: BuildQualitySummary
}

// ---- M7 技能（02 文档 §5.2 skill DDL / §6.12） ----

export interface SkillResource {
  name: string
  content: string
}

export interface Skill {
  id: string
  name: string
  description: string
  instruction: string
  tools: string[]
  resources: SkillResource[]
  /** REQ-373：''=普通技能 | 'collection'=技能集合（一卡多子技能，挂载按成员展开注入） */
  kind: string
  children: string[]
  builtin: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

/** REQ-371：目录式技能包导入报告（同名跳过不覆盖；failed 为解析/落库失败明细） */
export interface SkillImportReport {
  created: number
  names?: string[]
  skipped?: { name: string; reason: string }[]
  failed?: { name: string; reason: string }[]
  notes?: { skill: string; notes: string[] }[]
  lists_shrunk?: boolean
  /** REQ-373：按包归集结果（collection_name 非空且存在新建件时返回） */
  collection?: { name: string; members: number }
}

// ---- M5 工具注册表（02 文档 §6.8：id/name/desc，前端勾选落 agent.tools） ----

export interface ToolInfo {
  id: string
  name: string
  description: string
  source?: string
  /** REQ-451：能力分组标签（如「平台知识」，工具选择下拉徽标） */
  group?: string
}

// ---- M8 本体对接（构建平面 :8091 / 运行平面 :8090，同源反代；契约以本体平面源码为准） ----

/** 本体元数据（GET /api/ontologies 为裸数组；无 status/progress，前端按阶段派生） */
/** REQ-262/M256：跨本体引用登记一条（external_refs 单源；to_onto_id 空=外部词表/IRI 登记态）。 */
export interface ExternalRef {
  ref_id: string
  from_onto_id: string
  from_concept: string
  to_onto_id: string
  to_concept: string
  ref_kind: 'import' | 'alignment' | 'reuse'
  predicate?: string
  confidence?: number
  justification?: string
  created_by?: string
  created_at?: string
  note?: string
  dangling?: string
}

export interface Ontology {
  id: string
  name: string
  description?: string
  version?: number
  forked_from?: string | null
  created_at?: string
  updated_at?: string
  n_concepts?: number
  n_relations?: number
  n_instances?: number
  /** REQ-239/M65：版本发布状态机——draft（默认）| published（命名快照终态） */
  status?: 'draft' | 'published'
  version_name?: string
}

/** REQ-233②/M60：本体「被引用」三源聚合（GET /api/ontologies/{id}/references，主后端拼装） */
export interface OntologyPlanRef {
  id: string
  name: string
  engine?: string
  status: string
}
export interface OntologyReferences {
  ontology_id: string
  runtime_plans: OntologyPlanRef[]
  kb_vocabs: { id: string; name: string; mode?: string }[]
  companion_agents: { id: string; name: string }[]
  warnings?: string[]
}

/** REQ-376①/M172：匿名限制公理（后端模型/TTL/校验既有；REQ-472/M272 前端类型补齐承载编辑面） */
export interface SpecRestriction {
  type: 'some_values_from' | 'all_values_from' | 'cardinality' | 'min_cardinality' | 'max_cardinality'
  /** 对象属性名（引用 relations.name） */
  property: string
  /** some/all_values_from 值域概念名 */
  target?: string
  /** 基数（基数型） */
  value?: number
}

/** Spec 概念（S2 编辑对象） */
export interface SpecConcept {
  name: string
  label?: string
  definition?: string
  /** REQ-510/M318：建模备注（skos:note 导出） */
  note?: string
  parents?: string[]
  /** REQ-419/M215：同义词（TBox 一等公民——SKOS altLabel 导入捕获回写/手动编辑；check_answerable 命中面） */
  synonyms?: string[]
  /** REQ-472/M272：限制公理（后端 REQ-376① 既有承载——前端补类型开放编辑） */
  restrictions?: SpecRestriction[]
}

/** Spec 关系（有向 from → to）；evidence 证据分级（REQ-295①，declared>name>values，67 号 A2） */
export interface SpecRelation {
  name: string
  label?: string
  definition?: string
  from: string
  to: string
  evidence?: string
  /** REQ-473/M273：对象属性特性（transitive/symmetric/functional/inverse_functional；TTL 导出附加 rdf:type 行） */
  characteristics?: string[]
  /** REQ-473/M273：逆属性（引用 relations.name；TTL 导出 owl:inverseOf） */
  inverse_of?: string
  /** REQ-510/M318：建模备注（skos:note 导出） */
  note?: string
}

/** Spec 实例关系 */
export interface SpecInstanceRelation {
  rel: string
  target: string
}

/** Spec 实例 */
export interface SpecInstance {
  name: string
  concept: string
  attributes?: Record<string, unknown>
  relations?: SpecInstanceRelation[]
}

/** 本体 Spec（GET/PUT /api/ontologies/{id}/spec；PUT 全量、校验门控、递增 version） */
/** 数据属性声明（REQ-268/M77 表达力升级一期；与实例 attributes 键同名关联，TTL 导出 attr: 命名空间） */
export interface SpecDataProperty {
  name: string
  label?: string
  definition?: string
  /** 定义域概念名（可空=不限） */
  domain?: string
  /** 数据类型短名 string|number|integer|boolean|date（未知形态保留 IRI，空=string） */
  range?: string
}

/** REQ-269/M78：公理保留层（disjoint_with/equivalent_class；导入保真承载，TTL 同 IRI 回写） */
export interface SpecAxiom {
  type: 'disjoint_with' | 'equivalent_class'
  subject: string
  targets: string[]
}

export interface Spec {
  name: string
  description?: string
  concepts: SpecConcept[]
  relations: SpecRelation[]
  /** 数据属性声明层（REQ-268/M77；可选，存量资产零迁移） */
  data_properties?: SpecDataProperty[]
  axioms?: SpecAxiom[] // REQ-269/M78：可选公理保留层
  instances: SpecInstance[]
  /** REQ-248/G2：能力问题（仅 spec 层，不入 TTL/校验） */
  cq?: string[]
}

/** 校验错误（PUT spec 400 / POST validate） */
export interface ValidationError {
  path: string
  message: string
}

/** 构建产物元数据（GET /api/ontologies/{id}/artifacts） */
export interface ArtifactMeta {
  format: string
  size: number
  is_normalized?: boolean
  imported_at?: string
}

/** 导入报告（POST /api/ontologies/import） */
export interface ImportReport {
  format: string
  lossy: boolean
  warnings: string[]
  lossy_note?: string
  unprojected?: string[]
  /** REQ-328/B：导入对账块——恒等式 triples_in = vocabulary + projected + remainder（unaccounted=0） */
  reconciliation?: {
    triples_in: number
    vocabulary: number
    projected: number
    remainder: number
    unaccounted: number
    inferred_notes?: string[]
    capture_rate?: number
  }
}

/** AI 草案结果（POST /api/ontologies/ai-draft） */
export interface ExtractedCQ { cq: string; origin?: string }
/** REQ-548/M355 工作区领域容器（Q-24 实体反转——前端绑定选择器消费） */
export interface Workspace {
  id: string
  name: string
  description: string
  default_kb_id: string
  default_profile_id: string
  default_agent_id: string
  created_at: string
  updated_at: string
}

/** REQ-551/M358 工作区记忆（Q-23 衍生②转正载体——候选/已入/已否三态，行级溯源可审计） */
export interface MemoryRec {
  id: string
  workspace_id: string
  content: string
  source_conversation_id: string
  source_message_id: string
  status: 'candidate' | 'accepted' | 'rejected'
  created_at: string
  updated_at: string
}

/** REQ-495/M304 笔记本（自产成果文档资产层；workspace_id 绑定 REQ-548 workspace 实体一对多） */
export interface Notebook {
  id: string
  name: string
  description: string
  workspace_id?: string
  created_at: string
  updated_at: string
}
/** 笔记本页面（两级树第二级；source=manual|local|git，触碰同步=mtime 基线） */
export interface NotePage {
  id: string
  notebook_id: string
  title: string
  content_md: string
  source: string
  source_ref: string
  source_mtime: string
  position: number
  created_at: string
  updated_at: string
}

/** REQ-508/M316 概念停车场候选（材料→AI 铺清单；origin=材料原句引用，仅停车场呈现面不入 spec——概念级备注留痕归 REQ-510） */
export interface ConceptCandidate {
  name: string
  label?: string
  definition?: string
  origin?: string
}
export interface AiDraftJobState {
  id: string
  status: 'running' | 'done' | 'failed'
  progress?: string
  result?: AiDraftResult
  cqs?: ExtractedCQ[]
  dedup?: number
  /** REQ-508：概念停车场铺候选结果（ai-concept-park） */
  concepts?: ConceptCandidate[]
  topic?: string
  truncated?: number
  error?: string
}
/** 质量门禁报告（REQ-312/M116：完整 Report 透出——三维分+Findings+Stats，qualitygate 同形） */
export interface QualityFinding {
  severity: 'error' | 'warning' | 'info'
  dimension?: string
  message: string
  samples?: string[]
}
export interface QualityScore {
  overall: number
  completeness: number
  consistency: number
  maintainability: number
}
export interface QualityReport {
  pass?: boolean
  strict?: boolean
  error_count: number
  warning_count: number
  info_count?: number
  findings?: QualityFinding[]
  score?: QualityScore
  stats?: { concepts: number; relations: number; instances: number; data_properties?: number; axioms?: number }
}

/** REQ-346/M146：CQ 基准集「测我」单次运行报告（runs 追加式历史；三率+逐条信号） */
export interface CqBenchmarkRun {
  ontology_id: string
  version: number
  dataset: string
  ts: string
  signals: { id: string; cq: string; kind?: string; difficulty?: string; translated: boolean; whitelist_ok: boolean; gold_aligned: boolean; query?: string; error?: string }[]
  translated: number
  whitelist: number
  gold_aligned: number
  total: number
  note?: string
  /** REQ-367/M165：双维分层跑分（difficulty/kind 各档独立三率） */
  by_difficulty?: Record<string, { total: number; translated: number; whitelist: number; gold_aligned: number }>
  by_kind?: Record<string, { total: number; translated: number; whitelist: number; gold_aligned: number }>
  /** REQ-352/M152：口径声明（判定口径/分层口径/数据集版本/评测器版本）——版本对比视图消费防口径漂移误读 */
  metric_definition?: { translated?: string; whitelist?: string; gold_aligned?: string; stratified?: string; dataset_rev?: string; generator_ver?: string }
}

/** REQ-331/M131：资产级质量评分历史行（source=draft|ingest|manual；趋势线+三点对照取数源） */
export interface QualityScoreRow {
  id: number
  ontology_id: string
  version: number
  source: 'draft' | 'ingest' | 'manual'
  overall: number
  completeness: number
  consistency: number
  maintainability: number
  error_count: number
  warning_count: number
  created_at: string
}

export interface AiDraftResult {
  spec: Spec
  rounds: number
  warning?: string
  /** REQ-247/G4 + REQ-312/M116：草案质量报告（完整 qualitygate.Report） */
  quality?: QualityReport
}

// ---- OntoChat 多轮引导（REQ-103 模式 A）----

/** 会话阶段：cq 列 CQ → domain 逐轮补全 → draft/refine 草稿与修正 → done 已入库 */
export type OntoChatStage = 'story' | 'cq' | 'domain' | 'draft' | 'refine' | 'done'

export interface OntoChatMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  ts: string
}

export interface OntoChatContext {
  description: string
  cqs?: string[]
  hints?: string[]
  draft_spec?: unknown
  story_step?: number // REQ-275：访谈当前步（0 起）
  story_answers?: string[]
  draft_story?: string // REQ-275：用户故事（访谈制品）
  test_report?: { // REQ-342/OC-2：会话草稿覆盖测试报告（test 站）
    verdicts: { cq: string; verdict: string; explanation?: string }[]
    passed: number
    total: number
    pass_rate: number
  }
}

export interface OntoChatSession {
  id: string
  title: string
  stage: OntoChatStage
  round: number
  messages?: OntoChatMessage[]
  context?: OntoChatContext
  ontology_id?: string
  /** REQ-354/P2·G4 归属审计两列（REQ-360 D 产出清单消费：agent_id 过滤+来源徽标） */
  source?: 'page' | 'agent_chat'
  agent_id?: string
  created_at: string
  updated_at: string
}

/** REQ-338③：OntoChat 草稿版本链快照（draft/refine 每次生成落全量快照；confirm=增量确认调整后的入库形态）。 */
export interface OntoChatDraftVersion {
  version: number
  source: 'draft' | 'refine' | 'confirm'
  round: number
  materials?: string[]
  spec_json: Spec
  created_at: string
}

/** turn 响应：reply 为 assistant 回复；draft 仅生成轮产出 */
export interface OntoChatTurnResult {
  reply: string
  stage: OntoChatStage
  round: number
  draft?: Spec
  warning?: string
  quality?: QualityReport
  session: OntoChatSession
}

/** REQ-271/M80：生成轮异步 job（202 返回 job_id；GET /api/ontochat/jobs/{id} 轮询终态） */
export interface OntoChatJob {
  id: string
  session_id: string
  status: 'queued' | 'running' | 'done' | 'error' | 'cancelled'
  error?: string
  progress?: string
  result?: {
    reply?: string
    stage?: OntoChatStage
    round?: number
    warning?: string
    quality?: QualityReport
    draft?: Spec
    session?: OntoChatSession
    cqs?: OntoChatExtractedCQ[] // REQ-272：CQ 抽取 job 终态候选
    duplicate_count?: number
    clusters?: OntoChatCluster[] // REQ-273：CQ 分析 job 终态簇
    dedup_count?: number
    merged_groups?: { keep: string; duplicates: string[] }[] // REQ-342/OC-3：paraphrase 去重明细
    dedup_skipped?: boolean // REQ-342/OC-3：去重步降级（只聚类未去重）
    purification?: { compound_found: number; split_out: number; named_found: number; abstracted: number } // REQ-342/OC-1：净化统计
    purification_degraded?: boolean // REQ-342/OC-1：净化轮降级（用抽取原清单）
    verdicts?: OntoChatCQVerdict[] // REQ-274：覆盖测试判定（Yes/No/Unknown）
    passed?: number
    total?: number
    pass_rate?: number
  }
  created_at: string
  updated_at: string
}

/** REQ-272：单条 CQ 抽取候选（origin=抽取|拆分|抽象，两净化算子标注） */
export interface OntoChatExtractedCQ {
  cq: string
  origin?: string
}

/** REQ-274：单条 CQ 覆盖判定（Unknown=单条调用失败如实标注，不静默放行） */
export interface OntoChatCQVerdict {
  cq: string
  verdict: 'Yes' | 'No' | 'Unknown'
  explanation?: string
}

/** REQ-273：CQ 主题簇（去重+聚类结果，人工确认后应用写回） */
export interface OntoChatCluster {
  label: string
  cqs: string[]
}

/** GET /api/ontochat/prompts：提示词只读清单（REQ-271⑥，页面显示=运行时注入同一份数据） */
export interface OntoChatPrompt {
  id: string
  label: string
  purpose: string
  source: string
  text: string
}

/** 注入指引（GET /api/ontologies/{id}/guide） */
export interface GuideResponse {
  ontology_id: string
  guide: string
}

/** 运行方案（运行平面 :8090；status 状态机 created→starting→running⇄stopped→error） */
export interface RuntimeProfile {
  id: string
  name: string
  engine?: string
  ontology_ids: string[]
  config?: Record<string, unknown>
  port?: number
  status: 'created' | 'starting' | 'running' | 'stopped' | 'error'
  pid?: number
  last_error?: string
  last_error_at?: string
  created_at?: string
  updated_at?: string
  /** REQ-234①/M61：装载质量快照 JSON {oid:{overall,error_count,warning_count}}（启动异步快评） */
  loaded_quality?: string
  /** REQ-239/M65：装载发布状态快照 JSON {oid:{status,version_name}}（draft 装载警示数据源） */
  loaded_status?: string
}

// ---- P1 尾适配（REQ-92/93/94 + 学习示例） ----

/**
 * 版本原始源文件格式（VersionMeta.original_format；REQ-93 源码视图按格式选渲染模式）。
 * 与构建平面 SaveVersion 落库口径一致：turtle / owl_rdfxml / spec_json / csv / graphml。
 */
export type OriginalFormat = 'turtle' | 'owl_rdfxml' | 'spec_json' | 'csv' | 'graphml'

/** 版本历史条目（GET /api/ontologies/{id}/versions，REQ-93） */
export interface VersionMeta {
  version: number
  created_at: string
  has_original: boolean
  original_format?: string
  original_size?: number
}

/** REQ-541：版本行并入发布快照 tag（version_name 发布命名随 release 透出） */
export interface VersionRow extends VersionMeta {
  release?: { version_name?: string; published_at?: string }
}

export interface VersionsResponse {
  ontology_id: string
  /** REQ-509/M317：实际返回 VersionRow（含 release.version_name）——保持字段兼容 */
  versions: VersionRow[] | null
}

/** 翻译透视条目（GET /api/runtime-profiles/{id}/trace，REQ-94；失败查询也留痕） */
export interface TraceEntry {
  id?: number
  ts: string
  tool: string
  profile_id: string
  ontology_id: string
  sparql: string
  took_ms: number
  result_count: number
  ok: boolean
  error?: string
  nl?: string // REQ-333：自然语言原问题（NL2SPARQL 翻译对照）
}

export interface TraceResponse {
  profile_id: string
  traces: TraceEntry[] | null
}

/** 内置学习示例（GET /api/ontologies/seed-learning） */
export interface LearningExample {
  key: string
  name: string
  description: string
}

// ---- KG 自存 + 消费/审计（D-O15/REQ-110：去-semantica 化，主平台自研 /api/kg、/api/audit）----

/** KG 实体（GET /api/kg/{kbID}；name 库内业务键，同 KB 跨 doc 同名归一） */
export interface KGEntity {
  id: string
  kb_id: string
  doc_id?: string
  name: string
  type?: string
  description?: string
  /** M36/KB-7②：人工别名（分号分隔；检索/搜索命中别名；重建保留） */
  alias?: string
  created_at?: string
}

/** KG 关系（source/target 引用实体 name；type 为大写短语，如 IS_A/具有/引发） */
export interface KGRelationship {
  id: string
  kb_id: string
  doc_id?: string
  source: string
  target: string
  type?: string
  created_at?: string
}

/** KG claim：实体的一条可溯源陈述（原文句），chunk_id 指向出处片段 */
export interface KGClaim {
  id: string
  kb_id: string
  doc_id?: string
  chunk_id?: string
  subject: string
  text: string
  created_at?: string
}

/** KG 子图读取结果（GET /api/kg/{kbID}；消费页图谱/审计页数据源） */
export interface KGReadResult {
  kb_id: string
  entities: KGEntity[]
  relationships: KGRelationship[]
  claims: KGClaim[]
  counts: { entities: number; relationships: number; claims: number }
}

/** 审计决策留痕（GET/POST /api/audit/decisions；derived_from 指向前置决策构成溯源链） */
export interface OntoDecision {
  id: string
  subject_kind: 'kg' | 'ontology' | 'kb' | 'manual'
  subject_id: string
  title: string
  rationale?: string
  derived_from?: string
  meta_json?: string
  created_at: string
}

/** 审计决策录入入参（POST /api/audit/decisions；id/subject_kind 缺省由后端补） */
export interface OntoDecisionInput {
  subject_kind?: OntoDecision['subject_kind']
  subject_id?: string
  title: string
  rationale?: string
  derived_from?: string
  meta_json?: string
}

// ---- P2 本体增量（REQ-95 版本 diff / REQ-96 CSV 灌装 / REQ-83 fork） ----

/** diff 字段级变化：from → to（REQ-95） */
export interface DiffFieldChange {
  from: unknown
  to: unknown
}

/** diff 变更条目：元素名 + 字段级变化集合 */
export interface DiffChanged {
  name: string
  fields: Record<string, DiffFieldChange>
}

/** diff 元素（三集合之一：概念 / 关系 / 实例，按 id=name 对齐） */
export type DiffItem = SpecConcept | SpecRelation | SpecInstance

/** diff 单集合（concepts / relations / instances 同构） */
export interface DiffCollection {
  added: DiffItem[]
  removed: DiffItem[]
  changed: DiffChanged[]
}

/** 引用影响统计：变更元素被 relations/instances 引用的次数 */
export interface DiffImpact {
  name: string
  referenced_by: number
}

/** 版本 diff（GET /api/ontologies/{id}/diff?from&to；400 版本无快照 / 404） */
export interface DiffResult {
  from_version: number
  to_version: number
  concepts: DiffCollection
  relations: DiffCollection
  instances: DiffCollection
  impact: DiffImpact[]
}

/** CSV 灌装统计（REQ-292/294/295 扩展：B6 归一命中数/B2 活动流判据/实际物化数） */
export interface CsvIngestStats {
  rows_read: number
  instances_generated: number
  skipped_empty_key: number
  case_insensitive_matches?: number
  activity_hint?: boolean
  instances_materialized?: number
}

/** 构建产出人话 note（REQ-295②，67 号 A3 四类：merged/unmatched/case/unlinked，带具体数字） */
export interface BuildNote {
  kind: string
  text: string
}

/** CSV 灌装预览（mode=preview）；base_version 供 apply 回传防陈旧（REQ-293②） */
export interface CsvIngestPreview {
  stats: CsvIngestStats
  warnings: string[] | null
  draft: SpecInstance[] | null
  notes?: BuildNote[] | null
  base_version?: number
}

/** CSV 灌装确认入库（mode=apply）；overlays_* = 重放后的人工编辑保留计数（REQ-292①） */
export interface CsvIngestApplyResult {
  saved: boolean
  version: number
  stats: CsvIngestStats
  warnings?: string[] | null
  notes?: BuildNote[] | null
  overlays_applied?: number
  overlays_skipped?: number
}

/** CSV 灌装映射配置（REQ-96 P2b；GET/PUT /api/ontologies/{id}/ingest-mapping） */
export interface IngestMapping {
  concept: string
  key_column: string
  relation_columns?: string[]
  attribute_columns?: string[]
  skip_rows?: number
  /** 列名 → 转换类型（int/number/date/bool；缺省 string 原样） */
  type_rules?: Record<string, string>
  /** 关系列多值分隔符（空 = 整格单值） */
  multi_value_sep?: string
}

/** fork 入参（POST /api/ontologies/{id}/fork；REQ-83） */
export interface ForkOntologyInput {
  name?: string
  description?: string
}

// ---------------------------------------------------------------------------
// 工具链配置（REQ-75/76，GET/POST /api/pipelines 等；04 §4.6）
// ---------------------------------------------------------------------------

/** 七阶段候选工具（tools.json 数据驱动，REQ-77 开放性） */
export interface PipelineStageTool {
  id: string
  stage: string
  name: string
  license?: string
  mode: 'builtin' | 'guided' | 'managed'
  eats?: string[]
  gives?: string[]
  guide?: { install?: string; handoff?: string; entry?: string }
  learning?: string
}

/** 单阶段选择（pipeline_profile.stages 的值） */
export interface PipelineStageSelection {
  tool: string
  mode: string
  params?: Record<string, unknown>
}

/** 工具链配置（pipeline_profile 行） */
export interface PipelineProfile {
  id: string
  name: string
  ontology_id?: string
  runtime_profile_id?: string
  stages: Record<string, PipelineStageSelection>
  checklist: Record<string, unknown>
  created_at?: string
  updated_at?: string
}

/** 引导清单条目（guided 阶段聚合；key = tool:<stage>:<tool_id>） */
export interface PipelineChecklistItem {
  key: string
  kind: 'tool' | 'task'
  stage: string
  title: string
  detail?: string
  entry_url?: string
}

/** catalog 响应（七阶段固定顺序 + 分组清单） */
export interface PipelineCatalogResponse {
  stages: string[]
  catalog: Record<string, PipelineStageTool[]>
}

/** 配置详情响应（profile + checklist 视图） */
export interface PipelineDetail {
  profile: PipelineProfile
  checklist: PipelineChecklistItem[]
}

/** REQ-226/M54：Agent 配置版本快照（保存即版本；一键回滚数据面） */
export interface AgentConfigVersion {
  id: string
  agent_id: string
  version: number
  config_json: string
  note: string
  created_at: string
}

// ---------------------------------------------------------------------------
// REQ-292/M96 对象级 overlay（编辑三层 base+overlay+audit；67 号 B4/B5）
// ---------------------------------------------------------------------------

/** REQ-278/M103 影响分析报告（GET /api/ontologies/{id}/impact） */
export interface ImpactItem {
  kind: string
  name: string
  action: string
  risk: 'high' | 'medium' | 'low'
  reasons?: string[]
}
export interface ImpactReport {
  changes: Record<string, { added?: string[]; removed?: string[]; changed?: string[] }>
  blast: ImpactItem[]
  implied: string[]
  summary: { high: number; medium: number; low: number }
}

/** REQ-542/M349 构建面只读闭包预览（GET /api/ontologies/{id}/closure-preview；三出料集合） */
export interface ClosureNode {
  onto_id: string
  name?: string
  resolution: 'root' | 'head' | 'pin' | 'missing'
  version: number
  concepts: number
  spec_sha256?: string
}
export interface ClosureEdge {
  from_onto_id: string
  to_onto_id: string
  ref_kind: 'import' | 'reuse'
  from_concept?: string
  to_concept?: string
  via_imports?: boolean
  dangling?: string
}
export interface ClosureSchemaSection {
  onto_id: string
  resolution: 'root' | 'head' | 'pin'
  version: number
  concepts: { name: string; label?: string; definition?: string; parents?: string[]; synonyms?: string[] }[]
  relations: { name: string; from: string; to: string; label?: string }[]
  data_properties?: { name: string; domain?: string; range?: string }[]
}
export interface ClosurePreview {
  root_id: string
  generated_at: string
  nodes: ClosureNode[]
  edges: ClosureEdge[]
  remote_skipped?: string[]
  dangling?: { onto_id: string; ref_id: string; code: string; detail: string }[]
  pins?: Record<string, number>
  pin_conflicts?: Record<string, string>
  schema: {
    sections: ClosureSchemaSection[]
    concept_names: string[]
    name_conflicts?: { name: string; onto_ids: string[] }[]
    total_concepts: number
  }
  cycles: string[][]
}

/** overlay 行（GET /api/ontologies/{id}/overlays；effective/missing 为服务端实时计算） */
export interface OverlayRow {
  ontology_id: string
  logical_id: string
  kind: 'concept' | 'relation' | 'data_property' | 'instance'
  name: string
  action: 'edit' | 'add' | 'delete'
  base_json: string
  overlay_json: string
  revision: number
  updated_at: string
  actor?: string
  effective?: boolean
  missing?: boolean
  kind_label?: string
}

// REQ-224 H10/60 号搭车件：本体消费贡献度量（onto_* facade 工具命中统计）
export interface OntoContributionTarget {
  target: string
  count: number
}
export interface OntoContribution {
  ontology_id: string
  total_hits: number
  ok_hits: number
  first_hit_at?: string
  last_hit_at?: string
  by_tool?: { tool: string; count: number }[]
  top_targets?: OntoContributionTarget[]
  recent?: { id: string; tool: string; target?: string; ok: boolean; created_at: string; run_id?: string; conversation_id?: string }[]
}
export interface OntoContributionSummaryRow {
  ontology_id: string
  total_hits: number
  ok_hits: number
  last_hit_at?: string
  last_tool?: string
}
/** REQ-482/M282④：组维度——一次回答（run）命中的本体集合与方案归因 */
export interface OntoRunGroup {
  run_id: string
  profile_id?: string
  hits: number
  last_hit_at?: string
  ontologies: string[]
}
// REQ-224①：会话状态机读模型与变更留痕
export interface ConversationStateSnapshot {
  conversation_id: string
  phase: 'waiting_approval' | 'waiting_human' | 'running' | 'scheduled' | 'idle'
  running: boolean
  interrupt_state?: Record<string, unknown>
  scheduled: boolean
  schedule?: { interval_minutes: number; max_runs: number; done: number } | null
  todo_present: boolean
  todo_head?: string
  context_state: boolean
  updated_at: string
}
export interface ConversationStateLogRow {
  id: string
  conversation_id: string
  field: 'interrupt_state' | 'context_state' | 'todo_json' | 'schedule'
  prev?: string
  next?: string
  reason?: string
  created_at: string
}

// REQ-208/M42 余项：可继续子 Agent（continuation）后台委派
export interface ConversationDelegate {
  id: string
  parent_conversation_id: string
  child_conversation_id: string
  member_agent_id: string
  member_name?: string
  task: string
  status: 'running' | 'completed' | 'failed' | 'stopped'
  run_id?: string
  steer_note?: string
  result_digest?: string
  error?: string
  created_at: string
  finished_at?: string
}
