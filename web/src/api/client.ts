import type {
  AgentConfigVersion,
  Agent,
  ConversationDelegate,
  ConversationStateLogRow,
  ConversationStateSnapshot,
  OntoContribution,
  OntoContributionSummaryRow,
  AiDraftJobState,
  AiDraftResult,
  BuildQualitySummary,
  QualityScoreRow,
  CqBenchmarkRun,
  McpServeInfo,
  ArtifactMeta,
  Conversation,
  CsvIngestApplyResult,
  IngestMapping,
  CsvIngestPreview,
  DiffResult,
  DirValidation,
  ForkOntologyInput,
  GitBranch,
  GitCommit,
  GitFileChange,
  GitWorkingFile,
  GuideResponse,
  ImportReport,
  InferenceBackendStatus,
  ChunksToKGResult,
  KBDoc,
  KBSearchResult,
  KGReadResult,
  KGToSpecResult,
  KnowledgeBase,
  LearningExample,
  OntoBuildResult,
  OntoBuildSelectableKB,
  OntoDecision,
  WikiBuildResult,
  WikiPage,
  OntoDecisionInput,
  PipelineCatalogResponse,
  PipelineDetail,
  PipelineProfile,
  PipelineStageSelection,
  Message,
  ModelConnection,
  Ontology,
  OntologyReferences,
  OntoChatDraftVersion, OntoChatSession,
  OntoChatTurnResult,
  OntoChatJob,
  OntoChatPrompt,
  Project,
  ProjectDirListing,
  ProviderModelList,
  RunEventDTO,
  RuntimeProfile,
  Skill,
  SkillImportReport,
  Spec,
  ToolInfo,
  TraceResponse,
  UsageGroupBy,
  UsageStats,
  ValidationError,
  VersionsResponse,
  Connector,
  OverlayRow,
  ImpactReport,
} from './types'

/** 携带 HTTP 状态与校验错误的接口错误（供 UI 区分 404 / 400 validation_errors / 502 不可达） */
export class ApiError extends Error {
  status: number
  validationErrors?: ValidationError[]
  constructor(message: string, status: number, validationErrors?: ValidationError[]) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.validationErrors = validationErrors
  }
}

async function req<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const text = await res.text()
  // 非 JSON 响应（如反代未注册时的 "404 page not found"）不再抛 SyntaxError，
  // 归一为带状态码与响应片段的 ApiError，提示可读（bugfix：此前报 "Unexpected non-whitespace
  // character after JSON at position 4"，无法定位是哪个端点断了）
  let data: any = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    throw new ApiError(`HTTP ${res.status}：响应非 JSON — ${text.slice(0, 140) || '(空)'}`, res.status)
  }
  if (!res.ok) {
    const msg = (data && data.error) || `HTTP ${res.status}`
    throw new ApiError(msg, res.status, data?.validation_errors)
  }
  return data as T
}

/** multipart 上传：不设置 Content-Type（交由浏览器补 boundary） */
async function reqMultipart<T>(url: string, form: FormData): Promise<T> {
  const res = await fetch(url, { method: 'POST', body: form })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const msg = (data && data.error) || `HTTP ${res.status}`
    throw new ApiError(msg, res.status, data?.validation_errors)
  }
  return data as T
}

/** 文本响应请求（版本源码视图等非 JSON 端点；错误仍按 JSON {error} 解析） */
async function reqText(url: string): Promise<string> {
  const res = await fetch(url)
  const text = await res.text()
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const j = JSON.parse(text)
      if (j && j.error) msg = j.error
    } catch { /* 非 JSON 错误体，保留状态码消息 */ }
    throw new ApiError(msg, res.status)
  }
  return text
}

/** SPARQL 工作台请求：Accept 默认 JSON 结果；失败解析 {error} 或透传引擎原文片段 */
async function reqSparql(url: string, query: string, accept?: string, nl?: string): Promise<{ raw: string; json: any }> {
  const headers: Record<string, string> = { 'Content-Type': 'application/sparql-query', Accept: accept ?? 'application/sparql-results+json' }
  if (nl) {
    headers['X-Onto-NL'] = nl // REQ-333：NL 原文随执行落透视（runtime-manager trace_log 翻译对照）
  }
  const res = await fetch(url, { method: 'POST', headers, body: query })
  const text = await res.text()
  let json: any = null
  try {
    json = JSON.parse(text)
  } catch { /* 引擎可能返回非 JSON（如 HTML 错误页） */ }
  if (!res.ok) {
    const msg = (json && json.error) || `引擎返回 HTTP ${res.status}：${(text || '').slice(0, 200)}`
    throw new ApiError(msg, res.status)
  }
  return { raw: text, json }
}

export const api = {
  // agents
  listAgents: () => req<Agent[]>('/api/agents'),
  /** REQ-311 B2/M115：Harness 审计跨会话聚合（审批挂起/决议、hook 拦截、verify 背压；倒序 limit） */
  harnessAudit: (agentId: string, limit = 50) =>
    req<{ events: HarnessAuditEvent[]; total: number }>(`/api/agents/${agentId}/harness-audit?limit=${limit}`),
  createAgent: (a: Partial<Agent>) => req<Agent>('/api/agents', { method: 'POST', body: JSON.stringify(a) }),
  updateAgent: (id: string, a: Partial<Agent>, ifMatch?: string) =>
    req<Agent>(`/api/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(a),
      headers: ifMatch ? { 'If-Match': ifMatch } : undefined,
    }),
  deleteAgent: (id: string) => req<{ deleted: string }>(`/api/agents/${id}`, { method: 'DELETE' }),
  /** REQ-131/M18：对外 MCP 服务化——端点/工具名/调用示例（Token 掩码） */
  getAgentMcpServe: (id: string) => req<McpServeInfo>(`/api/agents/${id}/mcp-serve`),
  // REQ-418/M214：协议面可视化
  listA2aCards: () => req<{ cards: import('./types').A2aCardEntry[]; count: number }>('/api/a2a/cards'),
  listA2aTasks: (limit = 100) => req<{ tasks: import('./types').A2aTaskInfo[]; count: number }>(`/api/a2a/tasks?limit=${limit}`),
  /** REQ-131/M18：重置对外服务 Token */
  resetAgentMcpToken: (id: string) => req<{ token_mask: string }>(`/api/agents/${id}/mcp-serve/reset`, { method: 'POST' }),

  // projects
  listProjects: () => req<Project[]>('/api/projects'),
  createProject: (p: Partial<Project>) => req<Project>('/api/projects', { method: 'POST', body: JSON.stringify(p) }),
  updateProject: (id: string, p: Partial<Project>, ifMatch?: string) =>
    req<Project>(`/api/projects/${id}`, {
      method: 'PUT',
      body: JSON.stringify(p),
      headers: ifMatch ? { 'If-Match': ifMatch } : undefined,
    }),
  setProjectAgents: (id: string, members: { agent_id: string; role: 'coordinator' | 'member' }[]) =>
    req<Project>(`/api/projects/${id}/agents`, { method: 'PUT', body: JSON.stringify(members) }),
  deleteProject: (id: string) => req<{ deleted: string }>(`/api/projects/${id}`, { method: 'DELETE' }),
  /** REQ-101：检测本地目录（REQ-133 分字段直连：format_ok/reachable/exists/is_dir/Git） */
  validateProjectDir: (dir: string) =>
    req<DirValidation>('/api/projects/validate-dir', { method: 'POST', body: JSON.stringify({ dir }) }),
  /** REQ-133：唤起部署主机系统目录选择对话框（仅同机部署可用；远程部署 400 + 提示手输） */
  pickProjectDir: () => req<{ dir: string }>('/api/projects/pick-dir', { method: 'POST' }),
  /** REQ-102：列出绑定目录下的条目（未绑定 → 400）；path 为相对子路径 */
  listProjectDirFiles: (id: string, path?: string) =>
    req<ProjectDirListing>(`/api/projects/${id}/dir-files${path ? `?path=${encodeURIComponent(path)}` : ''}`),
  /** REQ-102：读取目录内文件文本内容（≤1MB；超限 → 400） */
  getProjectDirFile: (id: string, path: string) =>
    reqText(`/api/projects/${id}/dir-file?path=${encodeURIComponent(path)}`),
  // REQ-102 深度版：Git 视图（提交历史 / 分支 / 变更明细）
  gitLog: (id: string, ref?: string, limit = 50) => {
    const qs = new URLSearchParams()
    if (ref) qs.set('ref', ref)
    if (limit !== 50) qs.set('limit', String(limit))
    const s = qs.toString()
    return req<{ commits: GitCommit[] }>(`/api/projects/${id}/git-log${s ? '?' + s : ''}`)
  },
  gitBranches: (id: string) => req<{ branches: GitBranch[] }>(`/api/projects/${id}/git-branches`),
  gitCommitFiles: (id: string, commit: string) =>
    req<{ files: GitFileChange[] }>(`/api/projects/${id}/git-commit-files?commit=${encodeURIComponent(commit)}`),
  gitCommitPatch: (id: string, commit: string, path?: string) => {
    const qs = path ? `&path=${encodeURIComponent(path)}` : ''
    return reqText(`/api/projects/${id}/git-commit-patch?commit=${encodeURIComponent(commit)}${qs}`)
  },
  gitWorking: (id: string) => req<{ files: GitWorkingFile[] }>(`/api/projects/${id}/git-working`),

  // conversations
  listConversations: (q: { scope?: string; agent_id?: string; project_id?: string } = {}) => {
    const params = new URLSearchParams()
    if (q.scope) params.set('scope', q.scope)
    if (q.agent_id) params.set('agent_id', q.agent_id)
    if (q.project_id) params.set('project_id', q.project_id)
    const qs = params.toString()
    return req<Conversation[]>(`/api/conversations${qs ? '?' + qs : ''}`)
  },
  createConversation: (c: Partial<Conversation>) => req<Conversation>('/api/conversations', { method: 'POST', body: JSON.stringify(c) }),
  /** REQ-412/M208：定时任务清单（conversation_schedule 全量） */
  listSchedules: () => req<{ conversation_id: string; interval_minutes: number; max_runs: number; done: number; created_at: string; updated_at: string }[]>(`/api/schedules`),
  /** REQ-412/M208：摘除定时任务（按会话） */
  deleteSchedule: (convId: string) => req<{ status: string }>(`/api/conversations/${convId}/schedule`, { method: 'DELETE' }),
  updateConversation: (id: string, c: Partial<Conversation>, ifMatch?: string) =>
    req<Conversation>(`/api/conversations/${id}`, {
      method: 'PUT',
      body: JSON.stringify(c),
      headers: ifMatch ? { 'If-Match': ifMatch } : undefined,
    }),
  deleteConversation: (id: string) => req<{ deleted: string }>(`/api/conversations/${id}`, { method: 'DELETE' }),
  listMessages: (id: string) => req<Message[]>(`/api/conversations/${id}/messages`),
  // M27/REQ-166：平台助手配置与 AI 内容优化
  assistantConfigGet: () =>
    req<{ system_prompt?: string; model_conn_id?: string; temperature?: number | null }>('/api/assistant/config'),
  /** M-O14 阶段三：L1 提案两段式（查看/应用/忽略） */
  assistantProposalGet: () => req<{ pending: boolean; proposal?: { proposal_id: string; changes: { field: string; from: string; to: string }[]; created_at: string } }>('/api/assistant/proposal'),
  assistantProposalApply: (id: string) =>
    req<{ applied: boolean }>('/api/assistant/proposal/' + id + '/apply', { method: 'POST', body: '{}' }),
  assistantProposalDiscard: (id: string) =>
    req<{ discarded: boolean }>('/api/assistant/proposal/' + id + '/discard', { method: 'POST', body: '{}' }),
  assistantConfigPut: (body: { system_prompt?: string; model_conn_id?: string; temperature?: number | null }) =>
    req<any>('/api/assistant/config', { method: 'PUT', body: JSON.stringify(body) }),
  assistantOptimize: (kind: 'agent_instruction' | 'project_constraints', content: string) =>
    req<{ optimized: string }>('/api/assistant/optimize', { method: 'POST', body: JSON.stringify({ kind, content }) }),
  /** REQ-140：内部方案文档只读查看（限 docs/ 下 .md，fsutil 防越界） */
  docRead: (path: string) =>
    req<{ path: string; title: string; content: string }>('/api/docs/read?path=' + encodeURIComponent(path)),
  /** REQ-136：对话自动命名（首轮用户输入提炼短标题；后台异步，失败回退默认名） */
  autoNameConversation: (id: string, input: string) =>
    req<{ started: boolean; reason?: string }>('/api/conversations/' + id + '/auto-name', { method: 'POST', body: JSON.stringify({ input }) }),
  /** REQ-113①：对话导出 Markdown（events=1 附过程事件附录） */
  exportConversation: (id: string, events = false) =>
    reqText(`/api/conversations/${id}/export${events ? '?events=1' : ''}`),
  listEvents: (id: string, q?: { run_id?: string; type?: string; type_prefix?: string; limit?: number; offset?: number; order?: string }) => {
    const qs = q ? Object.entries(q).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`).join('&') : ''
    return req<RunEventDTO[]>(`/api/conversations/${id}/events${qs ? '?' + qs : ''}`)
  },
  stopConversation: (id: string) => req<{ stopped: boolean }>(`/api/conversations/${id}/stop`, { method: 'POST' }),

  // model connections
  listConnections: () => req<ModelConnection[]>('/api/model-connections'),
  // REQ-231⑤⑥：运行时工具预览（四源合并确定性结果）与 hook 注册真相（Harness 页签数据源）
  agentToolPreview: (id: string) =>
    req<{ tools: { name: string; source: string }[]; masked?: { name: string; source: string }[]; notes?: string[] }>(`/api/agents/${id}/tool-preview`),
  listHooks: (agentId?: string) => req<{ hooks: { name: string; active: boolean; description: string }[] }>(`/api/hooks${agentId ? `?agent_id=${agentId}` : ''}`),
  // REQ-226/M54：配置治理（版本列表/一键回滚）
  listConfigVersions: (id: string) => req<AgentConfigVersion[]>(`/api/agents/${id}/config-versions`),
  rollbackConfig: (id: string, version: number) =>
    req<Agent>(`/api/agents/${id}/config-versions/${version}/rollback`, { method: 'POST', body: '{}' }),
  createConnection: (c: any) => req<ModelConnection>('/api/model-connections', { method: 'POST', body: JSON.stringify(c) }),
  updateConnection: (id: string, c: any) => req<ModelConnection>(`/api/model-connections/${id}`, { method: 'PUT', body: JSON.stringify(c) }),
  deleteConnection: (id: string) => req<{ deleted: string }>(`/api/model-connections/${id}`, { method: 'DELETE' }),
  setDefaultConnection: (id: string) => req<ModelConnection>(`/api/model-connections/${id}/default`, { method: 'PUT' }),
  testConnection: (input: any) => req<{ ok: boolean; error?: string; elapsed_ms: number }>('/api/model-connections/test', { method: 'POST', body: JSON.stringify(input) }),

  // inference backends（M13 §6.16：探测清单，10min TTL 缓存）
  listInferenceBackends: () => req<{ backends: InferenceBackendStatus[] }>('/api/inference-backends'),
  reprobeInferenceBackends: () => req<{ backends: InferenceBackendStatus[] }>('/api/inference-backends/reprobe', { method: 'POST' }),
  /**
   * 自动获取某提供商（锚点连接）的可用模型列表（ASSUMED 契约，接口可能未就绪 → 抛错由 UI 降级）。
   * POST /api/model-connections/{anchorId}/list-models → { models: string[] }
   */
  listProviderModels: (anchorId: string) => req<ProviderModelList>(`/api/model-connections/${anchorId}/list-models`, { method: 'POST' }),

  /**
   * 使用统计（ASSUMED 契约，接口可能未就绪 → 抛错由 UI 降级）。
   * GET /api/stats/usage?group_by=model|agent|project[&from=YYYY-MM-DD&to=YYYY-MM-DD]
   * from/to 可选且含首尾；空值不拼入查询串。
   */
  usageStats: (groupBy: UsageGroupBy, range?: { from?: string; to?: string }) => {
    const params = new URLSearchParams()
    params.set('group_by', groupBy)
    if (range?.from) params.set('from', range.from)
    if (range?.to) params.set('to', range.to)
    return req<UsageStats>(`/api/stats/usage?${params.toString()}`)
  },

  /** REQ-113②：数据量概览（各表行数 + DB 体积 + 对话/项目级联规模） */
  storageOverview: () =>
    req<{
      db_bytes: number
      stats: { conversations: number; messages: number; run_events: number; agents: number; projects: number; skills: number; knowledge_bases: number; project_files: number; model_conns: number }
      conversations: { id: string; title: string; scope: string; messages: number; updated_at: string }[]
      projects: { id: string; name: string; conversations: number }[]
    }>('/api/stats/storage'),

  // ---- M6 知识库（§8：/api/kb 系列） ----
  listKBs: () => req<KnowledgeBase[]>('/api/kb'),
  createKB: (k: Partial<KnowledgeBase>) => req<KnowledgeBase>('/api/kb', { method: 'POST', body: JSON.stringify(k) }),
  updateKB: (id: string, k: Partial<KnowledgeBase>) => req<KnowledgeBase>(`/api/kb/${id}`, { method: 'PUT', body: JSON.stringify(k) }),
  deleteKB: (id: string) => req<{ deleted: string }>(`/api/kb/${id}`, { method: 'DELETE' }),
  listKBDocs: (kbId: string) => req<KBDoc[]>(`/api/kb/${kbId}/docs`),
  uploadKBDoc: (kbId: string, doc: { name: string; content: string; filename?: string; contentB64?: string }) =>
    req<KBDoc>(`/api/kb/${kbId}/docs`, {
      method: 'POST',
      // REQ-439：PDF/DOCX 走 content_b64 二进制通道（file.text() 读二进制必失真）+filename 供后端分发抽取（REQ-414）
      body: JSON.stringify({ title: doc.name, content: doc.content, filename: doc.filename, content_b64: doc.contentB64 }),
    }),
  deleteKBDoc: (kbId: string, docId: string) => req<{ deleted: string }>(`/api/kb/${kbId}/docs/${docId}`, { method: 'DELETE' }),
  reindexKBDoc: (kbId: string, docId: string) => req<KBDoc>(`/api/kb/${kbId}/docs/${docId}/reindex`, { method: 'POST' }),
  searchPreview: (kbId: string, q: string, topK?: number, minScore?: number) =>
    req<KBSearchResult>(`/api/kb/${kbId}/search-preview`, { method: 'POST', body: JSON.stringify({ query: q, top_k: topK, min_score: minScore }) }),
  // M14 D-KB4：GraphRAG 子模块直查（D-O15 自研：KG 无命中返回 degraded:true，不抛错）
  graphragSearchKB: (kbId: string, q: string, maxResults?: number) =>
    req<KBSearchResult>(`/api/kb/${kbId}/graphrag-search`, { method: 'POST', body: JSON.stringify({ query: q, max_results: maxResults }) }),
  /** M16/REQ-128：增强检索（实体聚焦/跳数/关系类型过滤，返回附实体/关系/claims 明细） */
  graphragSearchEnhanced: (kbId: string, body: { query?: string; max_results?: number; entity?: string; hops?: number; relation_types?: string[] }) =>
    req<any>(`/api/kb/${kbId}/graphrag-search`, { method: 'POST', body: JSON.stringify(body) }),
  // REQ-241（M67）：LLM Wiki 类型——页面视图与手动全量重建
  listWikiPages: (kbId: string) => req<{ kb_id: string; pages: WikiPage[] }>(`/api/kb/${kbId}/wiki/pages`),
  rebuildWiki: (kbId: string, connID?: string) =>
    req<{ kb_id: string; result: WikiBuildResult }>(`/api/kb/${kbId}/wiki/rebuild`, {
      method: 'POST',
      body: JSON.stringify({ conn_id: connID || '' }),
    }),
  // M16 阶段一（REQ-127）：图谱浏览与统计
  kgStats: (kbId: string) => req<any>(`/api/kg/${kbId}/stats`),
  kgEntitySearch: (kbId: string, q: string, limit = 20) =>
    req<{ entities: any[] }>(`/api/kg/${kbId}/entities?q=${encodeURIComponent(q)}&limit=${limit}`),
  kgNeighborhood: (kbId: string, entity: string, hops = 1) =>
    req<{ entities: any[]; relationships: any[]; claims: any[] }>(
      `/api/kg/${kbId}/neighborhood?entity=${encodeURIComponent(entity)}&hops=${hops}`,
    ),
  // M16 阶段二（REQ-129）：抽取治理与人工反馈
  kgReview: (kbId: string, kind: 'relationship' | 'claim', id: string, status: 'approved' | 'rejected') =>
    req<any>(`/api/kg/${kbId}/review`, { method: 'POST', body: JSON.stringify({ kind, id, status }) }),
  kgMerge: (kbId: string, keep: string, merge: string[]) =>
    req<any>(`/api/kg/${kbId}/merge`, { method: 'POST', body: JSON.stringify({ keep, merge }) }),
  kgQuality: (kbId: string) =>
    req<{ method_dist: Record<string, number>; orphan_entity: number; top_rel_types: { type: string; count: number }[]; rejected_rels: number; rejected_claims: number; entities: number; relationships: number }>(
      `/api/kg/${kbId}/quality`),
  kgMergeSuggestions: (kbId: string) =>
    req<{
      suggestions: {
        keep: string
        merge: string
        reason: string
        /** M36/KB-7：rule（名称包含）| vector（embedding 相似）；空 = 历史口径 */
        strategy?: string
        similarity?: number
        /** 双嵌入防误并：类型不同时附「慎并」提示 */
        type_warning?: string
      }[]
      vector_degraded?: boolean
    }>(`/api/kg/${kbId}/merge-suggestions`),
  /** M36/KB-7②：实体别名人工标注（分号分隔多别名；空串清除；重建/合并自动保留归并） */
  kgEntityAlias: (kbId: string, name: string, alias: string) =>
    req<{ ok: boolean }>(`/api/kg/${kbId}/entity-alias`, { method: 'PUT', body: JSON.stringify({ name, alias }) }),
  // M16 阶段二（REQ-130）：社区摘要与全局问答
  kgCommunitiesRebuild: (kbId: string) =>
    req<{ ok: boolean; communities: number }>(`/api/kg/${kbId}/communities/rebuild`, { method: 'POST' }),
  kgCommunities: (kbId: string) =>
    req<{ communities: { id: string; label: string; summary: string; method?: string; members: string[] }[] }>(
      `/api/kg/${kbId}/communities`),
  kgGlobalSearch: (kbId: string, query: string) =>
    req<{ degraded: boolean; message?: string; hits: { label: string; summary: string; members: string[]; score: number }[] }>(
      `/api/kb/${kbId}/global-search`, { method: 'POST', body: JSON.stringify({ query }) }),

  // ---- O13 由知识库构建本体（REQ-108；精确路由压过本体反代前缀） ----
  selectableKBsForBuild: () => req<OntoBuildSelectableKB[]>('/api/kbs/selectable-for-ontology-build'),
  buildFromKB: (input: {
    kb_id: string
    strategy: 'chunk-llm' | 'kg-direct' | 'hybrid'
    cq_mode: 'auto' | 'custom' | 'skip'
    custom_cqs?: string[]
  }) => req<OntoBuildResult>('/api/ontologies/build-from-kb', { method: 'POST', body: JSON.stringify(input) }),
  /** M-O14 P2⑤：结构化数据（CSV/JSON）→ 本体骨架映射推导（规则推导不入库；诚实注记关系推导需 LLM 加工） */
  buildFromStructured: (input: { filename: string; content: string; target_ontology_id?: string; mode?: 'instance' | 'template'; hierarchy_columns?: string[] }) =>
    req<{
      source_kind: string
      mode?: string
      main_concept: string
      mapping: { column: string; role: string; infer_type: string; sample?: string; matched_concepts?: string[]; level?: number }[]
      draft_spec: unknown
      quality?: BuildQualitySummary
      notes: string[]
    }>('/api/ontologies/build-from-structured', { method: 'POST', body: JSON.stringify(input) }),
  // D-O15：显式重建自存 KG（原 /api/semantica/chunks-to-kg 退役）
  chunksToKG: (kbId: string) => req<ChunksToKGResult>(`/api/kg/${kbId}/rebuild`, { method: 'POST' }),
  kgToSpecJSON: (kbId: string) =>
    req<KGToSpecResult>('/api/ontologies/kg-to-spec-json', { method: 'POST', body: JSON.stringify({ kb_id: kbId }) }),

  // ---- KG 自存 + 消费/审计（D-O15/REQ-110：主平台自研，零外部进程） ----
  /** 某库全量 KG 子图 + 计数（GET /api/kg/{kbID}） */
  kgRead: (kbId: string) => req<KGReadResult>(`/api/kg/${kbId}`),
  /** 审计决策列表（GET /api/audit/decisions；subject_kind/subject_id/limit 可选） */
  listDecisions: (q: { subject_kind?: string; subject_id?: string; limit?: number } = {}) => {
    const params = new URLSearchParams()
    if (q.subject_kind) params.set('subject_kind', q.subject_kind)
    if (q.subject_id) params.set('subject_id', q.subject_id)
    if (q.limit) params.set('limit', String(q.limit))
    const qs = params.toString()
    return req<OntoDecision[]>(`/api/audit/decisions${qs ? '?' + qs : ''}`)
  },
  /** 手工/系统补录决策留痕（POST /api/audit/decisions）→ 201 */
  createDecision: (d: OntoDecisionInput) =>
    req<OntoDecision>('/api/audit/decisions', { method: 'POST', body: JSON.stringify(d) }),
  /** 溯源链：沿 derived_from 回溯（GET /api/audit/decisions/{id}/chain；32 跳封顶） */
  decisionChain: (id: string) => req<OntoDecision[]>(`/api/audit/decisions/${encodeURIComponent(id)}/chain`),
  /** PROV-O 导出（GET /api/audit/prov-export?kb_id=）→ text/turtle 原文（Go 原生模板，零 rdflib） */
  provExport: (kbId?: string) =>
    reqText(`/api/audit/prov-export${kbId ? `?kb_id=${encodeURIComponent(kbId)}` : ''}`),

  // ---- M7 技能（§8：/api/skills 系列 + 注入预览） ----
  listSkills: () => req<Skill[]>('/api/skills'),
  createSkill: (s: Partial<Skill>) => req<Skill>('/api/skills', { method: 'POST', body: JSON.stringify(s) }),
  updateSkill: (id: string, s: Partial<Skill>) => req<Skill>(`/api/skills/${id}`, { method: 'PUT', body: JSON.stringify(s) }),
  deleteSkill: (id: string) => req<{ deleted: string }>(`/api/skills/${id}`, { method: 'DELETE' }),
  skillPreview: (id: string) => req<{ instruction_block: string; enabled?: boolean }>(`/api/skills/${id}/preview`),
  /** REQ-371：目录式技能包批量导入（zip，multipart 字段 file；同名跳过不覆盖）。
   * REQ-373：collectionName 非空时按包归集——新建件收进一张集合卡（跳过/失败不入集）。 */
  importSkills: (file: File, collectionName?: string) => {
    const fd = new FormData()
    fd.append('file', file)
    if (collectionName && collectionName.trim()) fd.append('collection_name', collectionName.trim())
    return reqMultipart<SkillImportReport>('/api/skills/import', fd)
  },

  // ---- M5 工具注册表（§6.8：前端勾选落 agent.tools） ----
  listTools: () => req<ToolInfo[]>('/api/tools'),

  // ---- M8 本体对接：构建平面 :8091 /api/ontologies*（同源反代，全路径透传） ----
  listOntologies: () => req<Ontology[]>('/api/ontologies'),
  getOntology: (id: string) => req<Ontology>(`/api/ontologies/${id}`),
  createOntology: (o: { name: string; description?: string }) =>
    req<Ontology>('/api/ontologies', { method: 'POST', body: JSON.stringify(o) }),
  updateOntologyMeta: (id: string, m: { name: string; description?: string }) =>
    req<Ontology>(`/api/ontologies/${id}`, { method: 'PUT', body: JSON.stringify(m) }),
  deleteOntology: (id: string) => req<{ deleted?: string }>(`/api/ontologies/${id}`, { method: 'DELETE' }),
  /** REQ-233②/M60：本体「被引用」三源聚合（运行方案挂载/KB 约束词表/伴生绑定；删除确认预检同源） */
  ontologyReferences: (id: string) => req<OntologyReferences>(`/api/ontologies/${id}/references`),
  /** 原始 Spec JSON；从未保存过 → 404（UI 视为空 Spec） */
  getSpec: (id: string) => req<Spec>(`/api/ontologies/${id}/spec`),
  /** 全量保存 Spec（校验门控、递增 version）；400 时错误带 validation_errors；baseVersion 走 If-Match 防陈旧（REQ-293②，不符 409） */
  saveSpec: (id: string, spec: Spec, baseVersion?: number) =>
    req<{ saved: boolean; version: number; overlays_recorded?: number }>(`/api/ontologies/${id}/spec`, {
      method: 'PUT',
      body: JSON.stringify(spec),
      headers: baseVersion
        ? { 'Content-Type': 'application/json', 'If-Match': `v${baseVersion}` }
        : { 'Content-Type': 'application/json' },
    }),
  // REQ-436/M232：元素级端点（构建面一等 REST——Validate/版本门禁服务端单源；REQ-420 手动建模 CRUD 消费）
  elementUpsert: (id: string, kind: 'concept' | 'relation' | 'attribute' | 'instance', body: Record<string, unknown>) =>
    req<{ operation: string; saved: boolean; base_version: number; version: number; detail: string }>(
      `/api/ontologies/${encodeURIComponent(id)}/elements/${kind}`,
      { method: 'POST', body: JSON.stringify(body) },
    ),
  elementDelete: (id: string, kind: 'concept' | 'relation' | 'attribute' | 'instance', name: string) =>
    req<{ operation: string; saved: boolean }>(
      `/api/ontologies/${encodeURIComponent(id)}/elements/${kind}/${encodeURIComponent(name)}`,
      { method: 'DELETE' },
    ),
  /** REQ-292/M96 对象级 overlay（编辑三层）：清单 / 对象编辑（revision 乐观锁+幂等键）/ 撤回到 base */
  listOverlays: (id: string) =>
    req<{ ontology_id: string; count: number; overlays: OverlayRow[] }>(`/api/ontologies/${id}/overlays`),
  putObjectOverlay: (id: string, kind: string, name: string, body: { values: unknown; expected_revision?: number; idempotency_key?: string; actor?: string }) =>
    req<{ saved: boolean; version?: number; logical_id: string; revision: number; idempotent?: boolean }>(
      `/api/ontologies/${id}/objects/${encodeURIComponent(kind)}/${encodeURIComponent(name)}/overlay`,
      { method: 'PUT', body: JSON.stringify(body) },
    ),
  revertOverlay: (id: string, kind: string, name: string) =>
    req<{ reverted: boolean; version: number; note: string }>(
      `/api/ontologies/${id}/objects/${encodeURIComponent(kind)}/${encodeURIComponent(name)}/overlay`,
      { method: 'DELETE' },
    ),
  /** 校验：始终 200，返回 ok + 错误列表 */
  validateOntology: (id: string) =>
    req<{ ok: boolean; validation_errors: ValidationError[] }>(`/api/ontologies/${id}/validate`, { method: 'POST' }),
  listArtifacts: (id: string) => req<ArtifactMeta[]>(`/api/ontologies/${id}/artifacts`),
  // ---- REQ-156/M-O15 质量卡与门禁开关 ----
  qualityRun: (id: string, strict = false, reasoning = false, save = true) =>
    req<{ report: QualityReport; artifact_saved?: boolean; reasoning?: { consistent: boolean | null; violation_count: number; source: string } }>(`/api/ontology/quality/check`, { method: 'POST', body: JSON.stringify({ ontology_id: id, strict, reasoning, save }) }),
  qualityReport: (id: string) =>
    req<{ ontology_id: string; imported_at: string; report: QualityReport | null }>(`/api/ontology/quality/report?ontology_id=${encodeURIComponent(id)}`),
  qualityConfig: (id: string) => req<{ ontology_id: string; strict: boolean; reasoning_check?: boolean }>(`/api/ontologies/${id}/quality-config`),
  /** REQ-224 H10：本体消费贡献画像（onto_* 工具命中次数/被引用概念 TopN） */
  ontologyContribution: (id: string, topN = 10) => req<OntoContribution>(`/api/ontology/${id}/contribution?top_n=${topN}`),
  /** REQ-224 H10：跨本体命中概览（消费审计栏一览） */
  ontologyContributionSummary: (limit = 20) => req<OntoContributionSummaryRow[]>(`/api/ontology/contribution/summary?limit=${limit}`),
  /** REQ-224①：会话状态机读模型（phase 派生 + 状态族全量） */
  conversationState: (id: string) => req<ConversationStateSnapshot>(`/api/conversations/${id}/state`),
  /** REQ-224①：会话状态变更留痕（prev→next 快照，倒序） */
  conversationStateLog: (id: string, limit = 200) => req<ConversationStateLogRow[]>(`/api/conversations/${id}/state-log?limit=${limit}`),
  /** REQ-208：后台委派列表（父会话视角） */
  listDelegates: (convId: string) => req<ConversationDelegate[]>(`/api/conversations/${convId}/delegates`),
  /** REQ-208：委派详情+子会话最终回复全文 */
  getDelegate: (id: string) => req<{ delegate: ConversationDelegate; result: string }>(`/api/delegates/${id}`),
  /** REQ-208：中途引导（运行中→轮界续跑消费；已收束→直接续跑） */
  steerDelegate: (id: string, message: string) =>
    req<ConversationDelegate>(`/api/delegates/${id}/steer`, { method: 'POST', body: JSON.stringify({ message }) }),
  /** REQ-208：打断 */
  interruptDelegate: (id: string) => req<ConversationDelegate>(`/api/delegates/${id}/interrupt`, { method: 'POST' }),
  /** REQ-208：冷恢复/续跑 */
  continueDelegate: (id: string, instruction?: string) =>
    req<ConversationDelegate>(`/api/delegates/${id}/continue`, { method: 'POST', body: JSON.stringify({ instruction: instruction ?? '' }) }),


  setQualityConfig: (id: string, patch: { strict?: boolean; reasoning_check?: boolean }) =>
    req<{ ontology_id: string; strict: boolean; reasoning_check?: boolean }>(`/api/ontologies/${id}/quality-config`, { method: 'PUT', body: JSON.stringify(patch) }),
  /** REQ-255/H2：CQ→SPARQL 翻译（LLM 辅助+人工确认模板；执行走 profileSparql） */
  /** REQ-331①：评分历史（趋势线+草稿→入库→当前三点对照取数源；时间正序） */
  getQualityHistory: (id: string) =>
    req<{ ontology_id: string; history: QualityScoreRow[]; count: number }>(`/api/ontologies/${id}/quality-history`),
  /** REQ-331②：检查项修复指引全量（check_id→「怎么改」人话指引；只读静态可缓存） */
  getQualityRemediations: () => req<Record<string, string>>('/api/ontology/quality/remediations'),
  /** CQ→SPARQL 翻译（REQ-255）；cqs 可选（REQ-342/OC-2 双轨并轨：只翻译存疑项，缺省=全量 spec.CQ） */
  cqSparql: (id: string, cqs?: string[]) =>
    req<{ ontology_id: string; items: { cq: string; sparql: string }[]; count: number }>(`/api/ontologies/${id}/cq-sparql`, {
      method: 'POST',
      body: JSON.stringify({ cqs: cqs ?? [] }),
    }),
  // ---- REQ-157/M-O15 导入合并（审查向导）----
  mergePreview: (id: string, body: MergeIngest) =>
    req<MergePreview>(`/api/ontologies/${id}/merge/preview`, { method: 'POST', body: JSON.stringify(body) }),
  mergeApply: (id: string, body: MergeIngest) =>
    req<{ applied: boolean; version: number; preview: MergePreview }>(`/api/ontologies/${id}/merge/apply`, { method: 'POST', body: JSON.stringify(body) }),
  // ---- REQ-155/M-O15 阶段二：方案生命周期 ----
  lifecyclePlan: () => req<LifecyclePlan>('/api/ontology/lifecycle/plan'),
  lifecycleApply: (actions: LifecycleAction[]) =>
    req<{ applied: number; total: number; results: { profile_id: string; action: string; ok: boolean; error?: string }[] }>('/api/ontology/lifecycle/apply', { method: 'POST', body: JSON.stringify({ actions }) }),
  /** 导入：multipart（file + 可选 name），自动嗅探 ttl/owl/graphml/csv/spec_json */
  importOntologyFile: (file: File, name?: string) => {
    const fd = new FormData()
    fd.append('file', file)
    if (name) fd.append('name', name)
    return reqMultipart<{ ontology: Ontology; report: ImportReport }>('/api/ontologies/import', fd)
  },
  /** 导入：JSON 模式 {filename, content, name?} */
  importOntologyContent: (filename: string, content: string, name?: string) =>
    req<{ ontology: Ontology; report: ImportReport }>('/api/ontologies/import', {
      method: 'POST',
      body: JSON.stringify({ filename, content, name }),
    }),
  /** 内置示例：201 新建（id=onto_k8s_ops）或 200 {id, seeded:false, note} */
  seedSampleOntology: () =>
    req<Ontology & { seeded?: boolean; note?: string }>('/api/ontologies/seed-sample', { method: 'POST' }),
  /** AI 草案：503 表示 LLM 未配置；capabilityQuestions 为能力问题（CQ，REQ §4.8.3） */
  aiDraftOntology: (description: string, extraHint?: string, capabilityQuestions?: string[]) =>
    req<AiDraftResult>('/api/ontologies/ai-draft', {
      method: 'POST',
      body: JSON.stringify({ description, extraHint, capability_questions: capabilityQuestions }),
    }),
  /** AI 草案异步提交（REQ-267/M76）：202 立返 job_id，配 aiDraftJob 轮询取件——去 120s 同步阻塞窗口 */
  aiDraftOntologyAsync: (description: string, extraHint?: string, capabilityQuestions?: string[]) =>
    req<{ job_id: string }>('/api/ontologies/ai-draft-async', {
      method: 'POST',
      body: JSON.stringify({ description, extraHint, capability_questions: capabilityQuestions }),
    }),
  /** 轮询异步草案任务：running | done（result 与同步端点同形）；progress=服务端实测轮次进度（REQ-295③） */
  aiDraftJob: (jobId: string) =>
    req<AiDraftJobState>(
      `/api/ai-draft-jobs/${encodeURIComponent(jobId)}`,
    ),
  /** CQ 抽取候选（REQ-308） */

  /** 读取本体 artifact 内容（REQ-318：axioms-raw 透传公理等；原样返回存储 JSON 文本） */
  getArtifactContent: (id: string, format: string) =>
    req<string>(`/api/ontologies/${encodeURIComponent(id)}/artifacts/${encodeURIComponent(format)}/content`),
  /** 权威本体库来源代理（REQ-316）：只读拉取公网 TTL/RDF → filename+content（SSRF 防护+8MB 上限） */
  catalogFetch: (url: string) =>
    req<{ filename: string; content_type: string; content: string }>(
      `/api/ontology-catalog/fetch?url=${encodeURIComponent(url)}`,
    ),
  /** AI 创建 CQ 候选生成（REQ-308）：描述+澄清轮材料→ExtractCQs（P5 两净化）；202+job 轮询（cqs 字段） */
  aiCQExtract: (description: string, hints: string[]) =>
    req<{ job_id: string }>('/api/ontologies/ai-cq-extract', {
      method: 'POST',
      body: JSON.stringify({ description, hints }),
    }),
  /** AI 创建澄清轮（REQ-306 A1/M110）：生成前基于描述+已补材料输出范围复述+五维缺口清单；同步 RawChat */
  aiClarify: (description: string, hints: string[]) =>
    req<{ reply: string }>('/api/ontologies/ai-clarify', {
      method: 'POST',
      body: JSON.stringify({ description, hints }),
    }),
  /** 导出下载地址（text/turtle attachment） */
  ontologyExportUrl: (id: string, format: string) => `/api/ontologies/${id}/export?format=${encodeURIComponent(format)}`,
  /** 注入指引（经构建平面路由，稳定可用） */
  getOntologyGuide: (id: string) => req<GuideResponse>(`/api/ontologies/${id}/guide`),

  // ---- M8 运行平面 :8090 /api/runtime-profiles* ----
  listRuntimeProfiles: () => req<RuntimeProfile[]>('/api/runtime-profiles'),
  /** REQ-372 P2：知识图谱投影（节点=卡三型、边=prereq） */
  knowledgeGraph: () => req<{ nodes: { id: string; kind: string; domain: string; topic: string; desc?: string; sources?: string[] }[]; edges: { src: string; dst: string; kind: string }[] }>('/api/knowledge/graph'),
  /** REQ-372 P2：学习路径（prereq 闭包拓扑序） */
  knowledgeGraphPath: (target: string) => req<{ target: string; path: string[]; count: number }>(`/api/knowledge/graph/path?target=${encodeURIComponent(target)}`),
  /** REQ-372 P2：重新摄取（全量重建投影+断链检测） */
  knowledgeGraphRefresh: () => req<{ nodes: number; edges: number; broken?: string[] }>('/api/knowledge/graph/refresh', { method: 'POST' }),
  /** REQ-384/M180：多本体路由歧义观测面（resolved=created_at 最早的实际路由目标） */
  routeAmbiguities: () => req<{ ambiguities: { ontology_id: string; profiles: { id: string; name: string; created_at: string }[]; resolved_profile_id: string }[]; count: number; owner_cache_ttl_seconds: number }>('/api/runtime-profiles/route-ambiguities'),
  // REQ-179/M-O16：全局运行配置（执行方式；系统级）
  runtimeConfig: () => req<{ execution_method: 'docker' | 'native' | 'k8s'; docker_available: boolean; options: { value: string; label: string }[] }>('/api/runtime-config'),
  setRuntimeConfig: (execution_method: string) => req<{ execution_method: string }>('/api/runtime-config', { method: 'PUT', body: JSON.stringify({ execution_method }) }),
  // REQ-191/M31：运行环境统一配置（沙箱运行方式 + K8s 访问认证；DB 覆盖 env）
  runtimeEnv: () => req<RuntimeEnvPayload>('/api/runtime-env'),
  setRuntimeEnv: (p: Partial<RuntimeEnvPayload['settings']>) => req<RuntimeEnvPayload>('/api/runtime-env', { method: 'PUT', body: JSON.stringify(p) }),
  testRuntimeEnv: (target: 'docker' | 'k8s') => req<{ target: string; ok: boolean; detail: string }>('/api/runtime-env/test', { method: 'POST', body: JSON.stringify({ target }) }),
  // ---- REQ-214/M46：外部连接器（凭据服务端加密绑定；credentials 仅创建/更新时提交） ----
  listConnectors: () => req<{ connectors: Connector[] }>('/api/connectors'),
  createConnector: (p: { kind: string; name: string; description?: string; config?: Record<string, unknown>; credentials?: Record<string, unknown> }) =>
    req<Connector>('/api/connectors', { method: 'POST', body: JSON.stringify(p) }),
  updateConnector: (id: string, p: { name?: string; description?: string; config?: Record<string, unknown>; credentials?: Record<string, unknown> }) =>
    req<Connector>(`/api/connectors/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(p) }),
  deleteConnector: (id: string) => req<{ ok: boolean }>(`/api/connectors/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  testConnector: (id: string) => req<{ ok: boolean; detail: string; status: string; tools: string[] }>(`/api/connectors/${encodeURIComponent(id)}/test`, { method: 'POST' }),
  // REQ-214 P2④：创建前预检（不落库，按表单探测）
  previewConnector: (p: { kind: string; name?: string; description?: string; config?: Record<string, unknown>; credentials?: Record<string, unknown> }) =>
    req<{ ok: boolean; detail: string; tools: string[] }>('/api/connectors/preview', { method: 'POST', body: JSON.stringify(p) }),
  // ---- REQ-148 供应商分组：多实例与别名（分组标识与 BaseURL 解耦） ----
  listProviderGroups: () => req<ProviderGroupMeta[]>('/api/provider-groups'),
  createProviderGroup: (alias: string) =>
    req<ProviderGroupMeta>('/api/provider-groups', { method: 'POST', body: JSON.stringify({ alias }) }),
  updateProviderGroupAlias: (id: string, alias: string) =>
    req<ProviderGroupMeta>(`/api/provider-groups/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify({ alias }) }),

  /** REQ-146 引擎自检：oxigraph/fuseki 全量呈现（未注册也返回 + 指引） */
  listEngines: () => req<{ engines: EngineStatus[] }>('/api/engines'),
  // ---- M10/10b 沙箱生命周期（per Agent） ----
  sandboxStatus: (id: string) => req<SandboxStatus>(`/api/agents/${encodeURIComponent(id)}/sandbox`),

  // REQ-21804/M49: agent file view (safe root = agent.work_dir; single-level list + 1MB text preview)
  listAgentDirFiles: (agentId: string, path = '') =>
    req<{ path: string; root: string; entries: { name: string; is_dir: boolean; size: number; mod_time: string }[] }>(
      `/api/agents/${agentId}/dir-files?path=${encodeURIComponent(path)}`,
    ),
  getAgentDirFile: (agentId: string, path: string) =>
    reqText(`/api/agents/${agentId}/dir-file?path=${encodeURIComponent(path)}`),  sandboxStart: (id: string) =>
    req<{ endpoint: string }>(`/api/agents/${encodeURIComponent(id)}/sandbox/start`, { method: 'POST' }),
  sandboxStop: (id: string) =>
    req<{ stopped: string }>(`/api/agents/${encodeURIComponent(id)}/sandbox/stop`, { method: 'POST' }),

  /** REQ-146 一键安装（仅 oxigraph；202 异步任务，结果轮询 listEngines） */
  /** 上传发行包安装（REQ-307 手动路径）：使用方自行下载 apache-jena-fuseki tar.gz → 上传解包落位（同步） */
  installEngineFile: (name: string, file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return reqMultipart<{ installed: boolean; files: string[] }>(`/api/engines/${encodeURIComponent(name)}/install-file`, fd)
  },
  installEngine: (name: string) =>
    req<{ started: boolean; engine: string }>(`/api/engines/${encodeURIComponent(name)}/install`, { method: 'POST' }),
  createRuntimeProfile: (p: { name: string; engine?: string; ontology_ids: string[]; config?: string | Record<string, unknown>; port?: number }) =>
    req<RuntimeProfile>('/api/runtime-profiles', { method: 'POST', body: JSON.stringify(p) }),
  updateRuntimeProfile: (id: string, p: { name: string; ontology_ids?: string[]; config?: Record<string, unknown>; port?: number }) =>
    req<RuntimeProfile>(`/api/runtime-profiles/${id}`, { method: 'PUT', body: JSON.stringify(p) }),
  deleteRuntimeProfile: (id: string) => req<{ deleted?: string }>(`/api/runtime-profiles/${id}`, { method: 'DELETE' }),
  startRuntimeProfile: (id: string) => req<RuntimeProfile>(`/api/runtime-profiles/${id}/start`, { method: 'POST' }),
  stopRuntimeProfile: (id: string) => req<RuntimeProfile>(`/api/runtime-profiles/${id}/stop`, { method: 'POST' }),
  reloadRuntimeProfile: (id: string) => req<RuntimeProfile>(`/api/runtime-profiles/${id}/reload`, { method: 'POST' }),
  runtimeProfileLogs: (id: string, tail = 200) => req<{ lines: string[] }>(`/api/runtime-profiles/${id}/logs?tail=${tail}`),

  // ---- P1 尾适配：版本历史（REQ-93）/ 学习示例 / 翻译透视（REQ-94）/ SPARQL 工作台（REQ-92） ----
  /** 版本历史列表（每次保存/导入/灌装留快照） */
  listVersions: (id: string) => req<VersionsResponse>(`/api/ontologies/${id}/versions`),
  /** 某版本导入时的原始源文件（Turtle/RDF-XML/JSON/CSV/GraphML 原文；无原始源 → 404） */
  getVersionOriginal: (id: string, version: number) => reqText(`/api/ontologies/${id}/versions/${version}/original`),
  /** 内置学习示例清单（软件缺陷/组织人员/设备故障） */
  listLearningExamples: () => req<LearningExample[]>('/api/ontologies/seed-learning'),
  /** 灌装学习示例（幂等：已存在 → 200 {seeded:false, note}） */
  seedLearningExample: (key: string) =>
    req<Ontology & { seeded?: boolean; note?: string }>('/api/ontologies/seed-learning', {
      method: 'POST',
      body: JSON.stringify({ key }),
    }),
  /** 工具链候选清单（REQ-75/77，七阶段分组，tools.json 数据驱动） */
  listPipelineCatalog: () => req<PipelineCatalogResponse>('/api/pipelines/catalog'),
  /** 工具链配置列表 */
  listPipelines: () => req<PipelineProfile[]>('/api/pipelines'),
  /** 创建工具链配置（默认模板：每阶段预置 builtin 项） */
  createPipeline: (input: { name: string; ontology_id?: string }) =>
    req<PipelineProfile>('/api/pipelines', { method: 'POST', body: JSON.stringify(input) }),
  /** 配置详情（附 checklist 引导清单视图） */
  getPipeline: (id: string) => req<PipelineDetail>(`/api/pipelines/${id}`),
  /** 更新配置（stages 全量覆盖 / meta 局部） */
  updatePipeline: (id: string, input: Partial<{ name: string; ontology_id: string; runtime_profile_id: string; stages: Record<string, PipelineStageSelection> }>) =>
    req<PipelineProfile>(`/api/pipelines/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  /** 复制为新工具链（checklist 清零） */
  clonePipeline: (id: string, input: { name?: string } = {}) =>
    req<PipelineProfile>(`/api/pipelines/${id}/clone`, { method: 'POST', body: JSON.stringify(input) }),
  /** 删除配置 */
  deletePipeline: (id: string) => req<{ deleted: boolean }>(`/api/pipelines/${id}`, { method: 'DELETE' }),
  /** guided 打卡（tool:<stage>:<tool_id>；toggle 由 done 控制） */
  checkPipeline: (id: string, key: string, done: boolean) =>
    req<{ key: string; done: boolean; checklist: Record<string, unknown> }>(`/api/pipelines/${id}/check`, {
      method: 'POST',
      body: JSON.stringify({ key, done }),
    }),
  /** 翻译透视（最近 N 条，含失败留痕；limit≤200） */
  listTraces: (profileId: string, limit = 50) => req<TraceResponse>(`/api/runtime-profiles/${profileId}/trace?limit=${limit}`),
  /** SPARQL 工作台：POST application/sparql-query；非 running → 409；状态码/错误透传引擎；accept 可覆盖（CONSTRUCT/DESCRIBE 传 text/turtle） */
  runSparql: (profileId: string, query: string, accept?: string, nl?: string) =>
    reqSparql(`/api/runtime-profiles/${profileId}/sparql`, query, accept, nl),
  /** M23①：配置面导出（四域 JSON；key 脱敏不导出）——Blob 下载由调用方处理 */
  settingsExport: () => req<Record<string, unknown>>('/api/settings/export'),
  /** M23①：配置面导入（同形态文件；按名去重保守导入，连接不带 key 需重录） */
  settingsImport: (payload: unknown) =>
    req<{ imported: Record<string, { created: number; skipped: string[]; failed: number; errors?: string[] }> }>('/api/settings/import', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  /** REQ-346/M146：CQ 基准集清单（seeds/evaldata/ontology-cq/ 管理面） */
  cqBenchmarkDatasets: () =>
    req<{ datasets: { name: string; ontology: string; count: number; revision?: string; note?: string }[]; count: number }>(
      '/api/ontology/cq-benchmark/datasets',
    ),
  /** REQ-346/M146：用基准集测我（202 job 轮询 aiDraftJob；报告落 artifact cq-benchmark-report） */
  cqBenchmarkRun: (ontologyId: string, dataset: string) =>
    req<{ job_id: string; total: number }>('/api/ontology/cq-benchmark/run', {
      method: 'POST',
      body: JSON.stringify({ ontology_id: ontologyId, dataset }),
    }),
  /** REQ-346/M146：基准测我报告历史（runs 追加式） */
  cqBenchmarkReport: (id: string) =>
    req<{ ontology_id: string; imported_at: string; report: { runs: CqBenchmarkRun[] } }>(
      `/api/ontologies/${id}/cq-benchmark-report`,
    ),
  /** REQ-333/M133：NL2SPARQL 受控生成（LLM+只读白名单修复环 ≤2 轮；执行由前端携 NL 走工作台代理落透视） */
  nl2Sparql: (ontologyId: string, question: string) =>
    req<{ ontology_id: string; question: string; query: string; rounds: number; history?: string[] }>('/api/ontology/nl2sparql', {
      method: 'POST',
      body: JSON.stringify({ ontology_id: ontologyId, question }),
    }),
  /**
   * SPARQL 工作台端点 URL（REQ-92，Yasgui 自行发起请求，不经 req 封装）。
   * 运行平面反代支持 GET ?query= 与 POST application/sparql-query；非 running → 409。
   */
  sparqlEndpointUrl: (profileId: string) => `/api/runtime-profiles/${profileId}/sparql`,
  /** 某版本原始源文件下载地址（REQ-93；>1MB 时前端提示下载查看而非渲染） */
  versionOriginalUrl: (ontologyId: string, version: number) =>
    `/api/ontologies/${ontologyId}/versions/${version}/original`,

  // ---- P2 本体增量（REQ-95 diff / REQ-96 CSV 灌装 / REQ-83 fork）----
  /** 版本 diff：GET /api/ontologies/{id}/diff?from&to；400 版本无快照 / 404 → ApiError（UI 内联 Alert） */
  diffOntologyVersions: (id: string, fromV: number, toV: number) =>
    req<DiffResult>(`/api/ontologies/${id}/diff?from=${fromV}&to=${toV}`),
  /** 按版本读取 spec_json 快照原文（REQ-145/M22 A3 文本 diff 数据面；404 版本不存在） */
  getVersionSpec: (id: string, version: number) => reqText(`/api/ontologies/${id}/versions/${version}/spec`),
  /** CSV 灌装预览：multipart（csv + concept/key_column/relation_columns/attribute_columns/skip_rows/mode=preview） */
  ingestCsvPreview: (id: string, form: FormData) => reqMultipart<CsvIngestPreview>(`/api/ontologies/${id}/ingest-csv?mode=preview`, form),
  /** CSV 灌装确认入库：同 multipart + ?mode=apply（REQ-295 批次顺修：此前漏传 mode 落 preview 的假成功） */
  ingestCsvApply: (id: string, form: FormData) => reqMultipart<CsvIngestApplyResult>(`/api/ontologies/${id}/ingest-csv?mode=apply`, form),
  /** 映射配置读取（REQ-96 P2b）：GET /api/ontologies/{id}/ingest-mapping；未保存过 → 404 */
  getIngestMapping: (id: string) => req<IngestMapping>(`/api/ontologies/${id}/ingest-mapping`),
  /** 映射配置保存（P2b）：PUT 同路径；结构即灌装配置（concept/key_column/列绑定/类型规则/分隔符/跳行） */
  putIngestMapping: (id: string, mapping: IngestMapping) =>
    req<{ saved: boolean }>(`/api/ontologies/${id}/ingest-mapping`, { method: 'PUT', body: JSON.stringify(mapping) }),
  /** fork 本体：POST /api/ontologies/{id}/fork → 201 新本体（forked_from=源 id，version 重置 1） */
  /** OntoExtend ODP 精选清单（M-O14 P2②；A-7 清扫：原裸 fetch 收编） */
  ontoExtendODPs: () => req<{ odps: unknown[] }>('/api/ontology/ontoextend/odps'),
  /** REQ-343/OE-1：CQ 驱动增量扩展（论文三段式 RAG，新能力承载不复用 ontoextend API 名）——
   *  cqs 缺省用 spec.CQ；profile=structure|restriction；top_k 缺省 20。202 job 轮询 getAiDraftJob（job.rag） */
  cqragDraft: (ontologyId: string, cqs?: string[], profile?: string, topK?: number) =>
    req<{ job_id: string; total: number }>('/api/ontology/cqrag/draft', {
      method: 'POST',
      body: JSON.stringify({ ontology_id: ontologyId, cqs: cqs ?? [], profile: profile ?? 'structure', top_k: topK ?? 20 }),
    }),
  /** LOV 词表搜索（REQ-171 P1；上游不可达 502 优雅降级） */
  vocabSearch: (q: string) => req<{ results?: unknown[]; items?: unknown[] }>(`/api/ontology/vocabularies/search?q=${encodeURIComponent(q)}`),
  forkOntology: (id: string, input: ForkOntologyInput = {}) =>
    req<Ontology>(`/api/ontologies/${id}/fork`, { method: 'POST', body: JSON.stringify(input) }),

  // ---- REQ-239/M65 版本发布状态机 ----
  /** 发布当前版本为命名快照终态（空命名默认 v{N}） */
  publishOntology: (id: string, versionName = '') =>
    req<Ontology>(`/api/ontologies/${id}/publish`, { method: 'POST', body: JSON.stringify({ version_name: versionName }) }),
  /** 撤回发布回 draft（命名清空） */
  unpublishOntology: (id: string) => req<Ontology>(`/api/ontologies/${id}/unpublish`, { method: 'POST' }),
  /** 历史快照回滚：指定版本恢复为新版本（BumpVersion+写版本历史），当前态回 draft */
  /** REQ-278/M103 变更影响分析（闭包波及面+风险分级+B1 疑似蕴含边；from/to 缺省=当前 vs 上一版） */
  impactOntology: (id: string, from?: number, to?: number) => {
    const q = new URLSearchParams()
    if (from) q.set('from', String(from))
    if (to) q.set('to', String(to))
    return req<{ report: ImpactReport }>(`/api/ontologies/${id}/impact?${q}`).then((r) => r.report)
  },
  restoreOntologyVersion: (id: string, version: number) =>
    req<{ restored_from: number; new_version: number; ontology: Ontology }>(`/api/ontologies/${id}/versions/${version}/restore`, { method: 'POST' }),
  /** REQ-240⑥/M66：图布局持久化 artifact（layout_json 派生数据，不写回 spec） */
  saveLayoutArtifact: (id: string, layout: unknown) =>
    req<{ saved: boolean; size: number }>(`/api/ontologies/${id}/artifacts/layout_json`, { method: 'PUT', body: JSON.stringify(layout) }),
  getLayoutArtifact: (id: string) =>
    req<unknown>(`/api/ontologies/${id}/artifacts/layout_json/content`),

  // ---- OntoChat 多轮引导（REQ-103 模式 A；构建平面 /api/ontochat/*）----
  /** REQ-360/M161：agentId 传入时按归属智能体过滤（侧板产出清单数据源）；缺省全量 */
  listOntoChatSessions: (agentId?: string) =>
    req<OntoChatSession[]>(`/api/ontochat/sessions${agentId ? `?agent_id=${encodeURIComponent(agentId)}` : ''}`),
  createOntoChatSession: (title?: string, mode?: 'quick' | 'guided') =>
    req<OntoChatSession>('/api/ontochat/sessions', { method: 'POST', body: JSON.stringify({ title, mode }) }),
  /** REQ-275：访气回退一步 */
  storyBack: (id: string) =>
    req<{ session: OntoChatSession }>(`/api/ontochat/sessions/${id}/story-back`, { method: 'POST' }),
  /** REQ-275：访谈完成——以用户故事为材料抽 CQ 候选（异步 job，阶段 story→cq） */
  storyFinish: (id: string) => req<{ job_id: string }>(`/api/ontochat/sessions/${id}/story-finish`, { method: 'POST' }),
  /** REQ-275：引导卡模板（P3 十条中文适配，只读） */
  storyTemplates: () => req<{ label: string; text: string }[]>('/api/ontochat/story-templates'),
  getOntoChatSession: (id: string) => req<OntoChatSession>(`/api/ontochat/sessions/${id}`),
  deleteOntoChatSession: (id: string) => req<{ deleted: string }>(`/api/ontochat/sessions/${id}`, { method: 'DELETE' }),
  /** 一轮交互：text 用户输入；feedback 非空 = refine 修正轮（意见回喂重新生成）；
   *  import_file = 文件材料导入（REQ-277，纯本地轮零 LLM）。
   *  REQ-271/M80：同步轮（cq/domain 归纳）200 全量结果；生成轮 202 {job_id, session}（轮询 jobs） */
  ontoChatTurn: (id: string, text: string, feedback?: string, importFile?: { name: string; content: string }) =>
    req<OntoChatTurnResult & { job_id?: string }>(`/api/ontochat/sessions/${id}/turn`, {
      method: 'POST',
      body: JSON.stringify({ text, feedback, import_file: importFile }),
    }),
  /** REQ-272：抽取 CQ 候选（异步 job；终态 result={cqs,duplicate_count}） */
  extractOntoChatCQs: (id: string) =>
    req<{ job_id: string }>(`/api/ontochat/sessions/${id}/cq-extract`, { method: 'POST' }),
  /** REQ-272：人工确认 CQ 写入会话（analyze 确认步；生成草稿时回写 spec.CQ） */
  setOntoChatCQs: (id: string, cqs: string[]) =>
    req<{ session: OntoChatSession }>(`/api/ontochat/sessions/${id}/cqs`, {
      method: 'POST',
      body: JSON.stringify({ cqs }),
    }),
  /** REQ-273：CQ 去重与主题聚类（异步 job；max_clusters 0=自动；终态 result={clusters,dedup_count}） */
  analyzeOntoChatCQs: (id: string, maxClusters?: number) =>
    req<{ job_id: string }>(`/api/ontochat/sessions/${id}/cq-analyze`, {
      method: 'POST',
      body: JSON.stringify({ max_clusters: maxClusters ?? 0 }),
    }),
  /** REQ-274：CQ 覆盖测试快筛（异步 job；cqs 缺省用 spec.CQ；默认关=质量卡显式开启动作） */
  coverageCQTest: (id: string, cqs?: string[]) =>
    req<{ job_id: string; total: number }>(`/api/ontologies/${id}/cq-coverage`, {
      method: 'POST',
      body: JSON.stringify({ cqs: cqs ?? [] }),
    }),
  /** REQ-342/OC-2：会话草稿覆盖测试（test 站——draft 后即可测试无需先入库；结果写回会话 context.test_report） */
  sessionCoverageTest: (id: string) =>
    req<{ job_id: string; total: number }>(`/api/ontochat/sessions/${id}/test`, { method: 'POST', body: '{}' }),
  /** REQ-342/OC-2：双轨完整报告落 artifact（快筛段后端已落；SPARQL 精判段并轨后整体覆盖保存） */
  saveCQTestReport: (id: string, report: unknown) =>
    req<{ saved: boolean; size: number }>(`/api/ontologies/${id}/cq-test-report`, { method: 'PUT', body: JSON.stringify(report) }),
  /** REQ-271：生成 job 轮询（1.5s 间隔；done 时 result 含 reply/draft/warning/session） */
  getOntoChatJob: (jobId: string) => req<OntoChatJob>(`/api/ontochat/jobs/${jobId}`),
  /** REQ-271：会话当前活跃任务（无则 job:null；重进会话/刷新后据此恢复轮询） */
  getOntoChatSessionActiveJob: (id: string) =>
    req<{ job: OntoChatJob | null }>(`/api/ontochat/sessions/${id}/job`),
  /** REQ-271：取消生成（运行中即时中止上游调用；错误留痕落会话消息） */
  cancelOntoChatJob: (jobId: string) =>
    req<OntoChatJob>(`/api/ontochat/jobs/${jobId}/cancel`, { method: 'POST' }),
  /** REQ-271⑥：提示词清单（只读，页面显示=运行时注入同一份数据） */
  listOntoChatPrompts: () => req<OntoChatPrompt[]>('/api/ontochat/prompts'),
  /** 草稿入库（预览确认门控，REQ-82）：201 {ontology, session} */
  ontoChatSave: (id: string, name: string, spec?: Spec) =>
    req<{ ontology: Ontology; session: OntoChatSession }>(`/api/ontochat/sessions/${id}/save`, {
      method: 'POST',
      body: JSON.stringify({ name, ...(spec ? { spec } : {}) }), // REQ-338①：增量确认裁剪后的入库形态（缺省=整稿）
    }),
  // REQ-338③：草稿版本链（中断续作/跨设备/审计回放取数口）
  ontoChatDrafts: (id: string) => req<OntoChatDraftVersion[]>(`/api/ontochat/sessions/${id}/drafts`),
}

/**
 * 运行对话并逐事件回调（SSE over fetch，POST 请求）。
 * 返回一个可中断的 AbortController。
 */
/** REQ-19f 对比窗格单项覆盖（空 = 继承对话当前配置；与后端 chat.PaneConfig 对齐） */
export interface ComparePaneConfig {
  agent_id?: string // REQ-143：窗格级智能体（空 = 继承对话配置）
  model_conn_id?: string
  kb_id?: string
  runtime_profile_id?: string
  no_history?: boolean // REQ-143③：不携带对话历史（干净对照）
}

export function runConversation(
  conversationId: string,
  input: string,
  debugLevel: number,
  onEvent: (ev: { event: string; data: any }) => void,
  panes?: ComparePaneConfig[],
  debugPersist?: boolean,
  modelConnId?: string,
): { abort: () => void; done: Promise<void> } {
  const body: Record<string, unknown> = { input, debug_level: debugLevel }
  // REQ-19e/19f 对比模式：≥2 窗格一次提问 N 路（meta 事件回传窗格 run_id 映射）
  if (panes && panes.length >= 2) body.panes = panes
  // REQ-149②：调试事件入库开关（级别≥1 时产生 model.step/装配快照，供历史与回放）
  if (debugPersist) body.debug_persist = true
  // REQ-174：单路模型覆盖（对话输入区模型快捷切换逐次下发；空 = 跟随智能体默认）
  if (modelConnId) body.model_conn_id = modelConnId
  return streamRun(`/api/conversations/${conversationId}/runs`, body, onEvent)
}

/**
 * 外部推理后端权限决议（REQ-206/M41 S2：deepseek-harness-acp 权限转审批）。
 * approve 映射首个 allow 选项；无待决挂起时后端 409（ApiError 文案可懂）。
 */
export function extApproval(conversationId: string, decision: 'approve' | 'deny'): Promise<{ status: string }> {
  return req(`/api/conversations/${conversationId}/ext-approval`, { method: 'POST', body: JSON.stringify({ decision }) })
}

/**
 * 恢复挂起的中断（M11 收尾 · ask_human 中断恢复）：以用户答复定向续跑，SSE 事件流与运行同构。
 */
export function resumeConversation(
  conversationId: string,
  answer: string,
  debugLevel: number,
  onEvent: (ev: { event: string; data: any }) => void,
  debugPersist?: boolean,
): { abort: () => void; done: Promise<void> } {
  return streamRun(`/api/conversations/${conversationId}/resume`, { input: answer, debug_level: debugLevel, ...(debugPersist ? { debug_persist: true } : {}) }, onEvent)
}

/** SSE over fetch 公共流（运行 / 恢复共用；POST JSON → 逐事件回调）。 */
function streamRun(
  url: string,
  body: Record<string, unknown>,
  onEvent: (ev: { event: string; data: any }) => void,
): { abort: () => void; done: Promise<void> } {
  const ctrl = new AbortController()
  const done = (async () => {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: ctrl.signal,
    })
    if (!res.ok || !res.body) {
      let msg = `HTTP ${res.status}`
      try {
        const j = await res.json()
        if (j.error) msg = j.error
      } catch { /* ignore */ }
      onEvent({ event: 'run.error', data: { data: { message: msg } } })
      return
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done: streamDone, value } = await reader.read()
      if (streamDone) break
      buf += decoder.decode(value, { stream: true })
      let idx: number
      // SSE 事件以空行分隔
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const raw = buf.slice(0, idx)
        buf = buf.slice(idx + 2)
        let event = 'message'
        let data = ''
        for (const line of raw.split('\n')) {
          if (line.startsWith('event:')) event = line.slice(6).trim()
          else if (line.startsWith('data:')) data += line.slice(5).trim()
        }
        if (!data) continue
        try {
          onEvent({ event, data: JSON.parse(data) })
        } catch {
          onEvent({ event, data: { raw: data } })
        }
      }
    }
  })()
  return { abort: () => ctrl.abort(), done }
}

/** M10/10b 沙箱状态（per Agent；enabled=false = 平台未配置 SANDBOX_IMAGE） */
export interface SandboxStatus {
  enabled: boolean
  state?: 'running' | 'stopped' | 'error'
  detail?: string
  memory?: string
  cpus?: number
}

/** REQ-191/M31 运行环境配置（settings=DB 原值空=跟随启动环境；effective=DB 覆盖 env 后生效值） */
export interface RuntimeEnvSettings {
  sandbox_mode: '' | 'inprocess' | 'docker' | 'k8s' | 'auto'
  sandbox_image: string
  sandbox_scope: '' | 'agent' | 'run'
  docker_bin: string
  kubectl_bin: string
  k8s_kubeconfig: string
  k8s_context: string
  k8s_namespace: string
  k8s_endpoint_mode: '' | 'port-forward' | 'pod-ip'
  platform_url_in_cluster: string
  platform_url_external: string
  updated_at?: string
}
export interface RuntimeEnvPayload {
  settings: RuntimeEnvSettings
  effective: RuntimeEnvSettings
  defaults: RuntimeEnvSettings
  sandbox_enabled: boolean
}

/** REQ-148 供应商分组元数据（分组 ID + 展示别名；成员连接经 provider_group_id 归属） */
export interface ProviderGroupMeta {
  id: string
  alias: string
  created_at?: string
  updated_at?: string
}

/** REQ-148 连接展示名：有组别名时以别名替换真名的提供商前缀（别名仅展示层，不改真名） */
export function connDisplayName(c: ModelConnection): string {
  if (c.provider_alias) {
    const i = c.name.indexOf('·')
    const model = i > 0 ? c.name.slice(i + 1) : c.model_name
    return `${c.provider_alias}·${model}`
  }
  return c.name.endsWith(`·${c.model_name}`) ? c.name : `${c.name} · ${c.model_name}`
}

/** REQ-146 引擎自检结果（运行平面 GET /api/engines；与后端 engine.EngineStatus 对齐） */
export interface EngineStatus {
  engine: string
  registered: boolean
  installed: boolean
  installing?: boolean
  last_install_error?: string
  binary?: string
  version?: string
  searched?: string[]
  hint?: string
  installable: boolean
}

// ---- REQ-156/157/M-O15：质量报告与导入合并 ----
export interface QualityReport {
  pass: boolean
  strict: boolean
  error_count: number
  warning_count: number
  info_count: number
  findings: { check_id: string; title: string; dimension: string; severity: 'error' | 'warning' | 'info'; count: number; samples: string[] }[]
  score: { overall: number; completeness: number; consistency: number; maintainability: number }
  stats: { concepts: number; relations: number; instances: number }
}
export interface MergeIngest {
  filename?: string
  content?: string
  spec?: unknown
  strategy?: string
  prefix?: string
  /** REQ-293② 防陈旧：预览时点目标版本，apply 回传（不符 409 要求重新预览） */
  base_version?: number
}
export interface MergePreview {
  strategy: string
  prefix: string
  added: string[]
  conflicts: { kind: string; name: string; fields: string[]; incoming: string; current: string; resolution: string; resolved_as?: string }[]
  renamed: string[]
  stats: { concepts_added: number; concepts_updated: number; relations_added: number; relations_updated: number; instances_added: number; instances_updated: number; instances_renamed: number; total_conflicts: number }
  target_name: string
  merged_spec: unknown
  /** REQ-235/H5/M62：原文件解析报告（lossy 清单+warnings；spec 直传时缺省） */
  import_report?: {
    format: string
    lossy: boolean
    warnings?: string[]
    lossy_note?: string
    /** REQ-328/B：导入对账块（unaccounted=0 恒等式+捕获率+推断清单） */
    reconciliation?: {
      triples_in: number
      vocabulary: number
      projected: number
      remainder: number
      unaccounted: number
      inferred_notes?: string[]
      capture_rate?: number
    }
  } | null
  /** REQ-293②：预览时点目标版本（apply 回传）；REQ-295②：人话 notes */
  base_version?: number
  notes?: { kind: string; text: string }[] | null
}

export interface LifecycleAction {
  profile_id: string
  profile_name?: string
  engine?: string
  action: 'start' | 'reload'
  reason: string
}
export interface LifecyclePlan {
  generated_at: string
  profiles: { profile_id: string; profile_name: string; engine: string; status: string; ontologies: string[]; loaded: Record<string, number>; current: Record<string, number>; drifted?: string[]; unknown_drift?: boolean; last_error?: string }[]
  actions: LifecycleAction[]
}

/** REQ-311 B2/M115：Harness 审计跨会话聚合行（run_event 审计族 + 会话标题 JOIN） */
export interface HarnessAuditEvent {
  id: string
  conversation_id: string
  conversation_title: string
  run_id: string
  type: string
  data?: string
  created_at: string
}
