package store

// 领域模型（与 §5 表清单一一对应；JSON 字段以文本存储）。

// Agent 智能体配置实体。
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instruction  string   `json:"instruction"`
	ModelConnID  *string  `json:"model_conn_id"` // 空 = 跟随全局默认（P1）
	Temperature  *float64 `json:"temperature"`
	MaxTokens    *int     `json:"max_tokens"`
	MaxIteration int      `json:"max_iteration"`
	// REQ-319 A2：本 agent 作为项目成员被委派调用时的单次执行超时秒数（0=不限）；
	// 超时经委派包装器降级回喂协调者（chat/delegatetool.go），不终止整个 run
	DelegateTimeoutSeconds int `json:"delegate_timeout_seconds"`
	// REQ-385/M181：成员委派异步化开关（0=同步阻塞〔缺省，存量零变化〕| 1=被委派时转后台
	// 委派底盘 DelegateManager——协调者不阻塞，delegate_status/collect 跟进结果）
	DelegateAsync bool        `json:"delegate_async"`
	Tools         []string    `json:"tools"`
	Skills        []string    `json:"skills"`      // P2 生效
	MCPServers    []MCPServer `json:"mcp_servers"` // P2 生效；REQ-214 起退役为兼容残留（存量迁移后置空，JSON 字段保留供旧导出消费）
	// REQ-214/M46：连接器引用（实例 id 数组）——agent 级连接白名单（最小权限第一层）。
	Connectors       []string `json:"connectors"`
	RuntimeBackend   string   `json:"runtime_backend"`
	InferenceBackend string   `json:"inference_backend"`  // M13 §6.16：空 = eino-adk 自研默认
	LogoURL          string   `json:"logo_url,omitempty"` // REQ-137：非内置后端登记的原 logo 图标 URL
	ToolApproval     string   `json:"tool_approval"`      // REQ-14 恢复②：工具调用人工审批（""=off | "danger"危险档 | "all"；REQ-231①）
	// REQ-231②：审批豁免清单（JSON 数组；danger/all 档下清单内工具直接放行——个工具覆盖档位）
	ApprovalExempt []string `json:"approval_exempt"`
	// REQ-231③：审批挂起超时（小时；0=不限——恢复重入时超时自动 deny 附超时语义）
	ApprovalTimeoutHours float64 `json:"approval_timeout_hours"`
	// M10/10b：docker 沙箱资源限制（runtime_backend=docker 时生效；空/0 = 默认 512m/1CPU）
	SandboxMemory string   `json:"sandbox_memory,omitempty"`
	SandboxCPUs   float64  `json:"sandbox_cpus,omitempty"`
	McpServe      McpServe `json:"mcp_serve"` // REQ-131/M18：对外 MCP 服务化（enabled/token/tool_name）
	// REQ-216/M47①：伴生本体绑定（可空引用——伴生产物归属容器化；空=未开启）。
	// REQ-170 bool 开关退役为派生：CompanionOntology 在 scan 时按本字段非空回填（读侧兼容），
	// 写侧以 companion_ontology_id 为准。
	CompanionOntologyID string `json:"companion_ontology_id"`
	// REQ-170/M28：伴生本体开关（REQ-216 起为派生只读——companion_ontology_id 非空即开启；
	// 保留 JSON 字段供旧消费方零破坏）
	CompanionOntology bool `json:"companion_ontology"`
	// REQ-186：内置助手标记（1=平台助手内置行——列表分区展示、不可编辑删除；行为上仍内置隔离）
	IsBuiltin bool `json:"is_builtin"`
	// REQ-187：伴生本体配置增强（默认空/空/0 = 现行为零回归）
	CompanionExtractHint   string  `json:"companion_extract_hint"`
	CompanionExtractConnID string  `json:"companion_extract_conn_id"`
	CompanionExtractCfg    string  `json:"companion_extract_cfg"`
	CompanionAutoThreshold float64 `json:"companion_auto_threshold"`
	// REQ-201/M37：上下文预算档位（'' = 标准档；compact 紧凑 / standard 标准 / full 完整不限量=存量行为）
	ContextMode string `json:"context_mode"`
	// CompactPolicy REQ-396/M192：上下文压缩策略（''=默认复用本 agent 模型连接八段式摘要 |
	// off=关闭摘要压缩仅剪枝 | <模型连接 id>=用指定（低成本）连接生成摘要）
	CompactPolicy string `json:"compact_policy"`
	// REQ-202/M38：工作目录（读写/grep/glob 的安全根，fsutil.SafeJoin 约束；空=仅项目会话文件工具）
	WorkDir string `json:"work_dir"`
	// REQ-202/M38：结束前验证命令（verify_on_stop 背压；空=不验证；失败不标记 completed）
	VerifyCommand string `json:"verify_command"`
	// REQ-232/M59：agent 级 hooks 配置（fetchGuard 启停 + 自定义 deny 规则；零值=全默认）
	Hooks AgentHooks `json:"hooks"`
	// REQ-360/M161：对话建本体能力开关（''=关〔缺省，存量零变化〕| guided=访谈 | quick=快速）。
	// 开启时装配层自动并入 ontochat_* 四工具（与 Tools 手勾取并集）+ 注入内置访谈纪律段
	//（instruction segment source=onchat）——引导段从「用户手工粘贴」退役为系统内置（03 §2.54 A）。
	OnchatMode string `json:"onchat_mode"`
	// REQ-366/M164：本体操作工具能力开关（false=关〔缺省，存量零变化〕| true=开启）。
	// 开启时会话挂载本体时装配层并入 facade 13 件扩展工具（spec CRUD/校验/CQ 基准）；
	// 原 6 查询工具挂载即并入的既有语义不变。写类 upsert/delete 入 danger 档审批。
	OntoOps bool `json:"onto_ops"`
	// REQ-452/M248：备援模型连接清单（JSON 数组）——主连接持续失败时按序切换
	// （adk ModelFailoverConfig；空数组=不启用，存量零回归）。切换即 run.warning 诚实标注。
	ModelFailoverConnIDs []string `json:"model_failover_conn_ids"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

// AgentHookDenyRule 自定义 deny 规则（REQ-232②）：工具名 glob 模式 + 参数子串（大小写不敏感），
// 命中即拒绝（hook.denied 结构化事件，guard=custom:<name>）。
type AgentHookDenyRule struct {
	Name        string `json:"name"`                   // 规则名（审计展示用）
	ToolPattern string `json:"tool_pattern"`           // 工具名 glob（path.Match 语义，如 http_fetch / mcp__* / web_*）
	ArgsPattern string `json:"args_pattern,omitempty"` // 参数 JSON 子串匹配（大小写不敏感；空=仅按工具名）
	Reason      string `json:"reason,omitempty"`       // 拒绝原因（回喂模型）
}

// AgentHooks agent 级 hooks 配置（REQ-232①②；零值=fetchGuard 启用+无自定义规则=存量行为）。
type AgentHooks struct {
	FetchGuardDisabled bool                `json:"fetch_guard_disabled"`
	DenyRules          []AgentHookDenyRule `json:"deny_rules"`
}

// McpServe Agent 对外服务配置（REQ-131/M18）：开启后经平台 /mcp 端点以 agent_{id} 工具暴露。
type McpServe struct {
	Enabled  bool   `json:"enabled"`
	Token    string `json:"token,omitempty"`     // Agent 级 Bearer Token（开启时自动生成，可重置）
	ToolName string `json:"tool_name,omitempty"` // 覆盖默认工具名 agent_{id}
}

// MCPServer Agent 级 MCP 端点（P2）。
// REQ-214 起产品层退役为「自定义 MCP 连接器」（connector kind=mcp），本结构保留作迁移输入与旧导出兼容。
type MCPServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Connector 外部连接器（REQ-214/M46：统一 agent 连接外部能力的产品抽象）。
// 两轴 = 连接对象（Kind）× 交付驱动（mcp 直通 / 平台托管插件服务）；
// 凭据服务端绑定（CredentialsEncrypted），不进 LLM 上下文不进工具参数。
type Connector struct {
	ID                   string         `json:"id"`
	Kind                 string         `json:"kind"` // mcp | kubernetes | ssh | a2a
	Name                 string         `json:"name"` // 实例名（装配前缀槽位 {name}__{tool}）
	Description          string         `json:"description"`
	Config               map[string]any `json:"config"`          // kind 专属非敏感配置
	CredentialsEncrypted []byte         `json:"-"`               // AES-256-GCM 密文（secrets.Box）
	HasCredentials       bool           `json:"has_credentials"` // 读侧派生（凭据永不回传明文）
	Status               string         `json:"status"`          // unknown | ok | error（连接测试回写）
	StatusDetail         string         `json:"status_detail"`
	Tools                []string       `json:"tools"`     // REQ-214 P2：工具名清单（test 成功落库——授权前知情）
	TestedAt             string         `json:"tested_at"` // REQ-214 P2：最近一次连接测试时间（状态时效性）
	CardJSON             string         `json:"-"`         // REQ-416/M212：a2a 注册握手 agent card 原文（出参经 connectorOut.card 解析透出）
	IsBuiltin            bool           `json:"is_builtin"`
	CreatedAt            string         `json:"created_at"`
	UpdatedAt            string         `json:"updated_at"`
}

// ProjectFile 项目文件与对话产物元数据（M11 §5.2 project_file）。
type ProjectFile struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	Name           string `json:"name"`
	Path           string `json:"path"` // 项目目录内相对路径
	Size           int64  `json:"size"`
	Mime           string `json:"mime,omitempty"`
	Source         string `json:"source"` // upload | artifact
	CreatedAt      string `json:"created_at,omitempty"`
}

// Project 多 Agent 项目。
type Project struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	CollabMode   string   `json:"collab_mode"`   // single | agent_as_tool | transfer
	WorkflowMode string   `json:"workflow_mode"` // free | sequential | parallel | loop (P1)
	Constraints  string   `json:"constraints"`   // 项目级统一约束（P1）
	LocalDir     string   `json:"local_dir"`     // 绑定的本地目录绝对路径（REQ-101 v0.17；空=未绑定）
	// WorkspaceID REQ-550/M357 可选归属工作区（NULL=未归属存量行为不变；REFERENCES SET NULL——
	// 删工作区项目脱钩；会话创建时经 ResolveConvDefaults 项目层转发工作区默认值，项目无独立覆盖面）
	WorkspaceID string   `json:"workspace_id"`
	AgentIDs    []string `json:"agent_ids"`   // 成员 Agent
	Coordinator string   `json:"coordinator"` // 主 Agent（role=coordinator）
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// Workspace 工作区领域容器（REQ-548/M355，Q-24 实体反转——项目之上的新领域实体，项目保留执行层）。
// config 引用列（default_kb_id/default_profile_id/default_agent_id）裸 TEXT 无 FK（106 号 mustFix-4：
// 失效兜底=ResolveConvDefaults 求值校验，方案走 ontology.unavailable 降级路径）。
type Workspace struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	DefaultKBID      string `json:"default_kb_id"`
	DefaultProfileID string `json:"default_profile_id"`
	DefaultAgentID   string `json:"default_agent_id"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// Notebook 笔记本（REQ-495/M304：自产成果文档资产层容器——Q-23 定位「自产成果（沉淀）」载体）。
// workspace_id 绑定新 workspace 实体一对多（Q-24 实体反转宿主端；SET NULL=删工作区回落个人资产）。
type Notebook struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	WorkspaceID *string `json:"workspace_id"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// NotePage 笔记本页面（两级树第二级；markdown 单源）。
// source=manual|local|git（MVP 两源=本地目录+git 本地克隆路径；触碰同步=mtime 基线）。
type NotePage struct {
	ID         string `json:"id"`
	NotebookID string `json:"notebook_id"`
	Title      string `json:"title"`
	ContentMD  string `json:"content_md"`
	Source     string `json:"source"`
	SourceRef  string `json:"source_ref"`
	SourceMtime string `json:"source_mtime"`
	Position   int    `json:"position"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// KBNotebookSource KB 回流引用登记（三拍板③：引用语义+触碰同步——笔记本单源不拷贝）。
type KBNotebookSource struct {
	ID         string `json:"id"`
	KBID       string `json:"kb_id"`
	NotebookID string `json:"notebook_id"`
	CreatedAt  string `json:"created_at"`
}

// Conversation 对话（scope=agent 直聊 / scope=project 项目对话）。
// WorkspaceID 工作区归属（REQ-548/M355：NULL=无归属存量行为不变；REFERENCES workspace SET NULL；
// 与 ProjectID 互斥——store 层应用校验补位，scope CHECK 不动 SQLite 无法 ALTER）。
type Conversation struct {
	ID               string  `json:"id"`
	Scope            string  `json:"scope"`
	AgentID          *string `json:"agent_id"`
	ProjectID        *string `json:"project_id"`
	WorkspaceID      *string `json:"workspace_id"`
	Title            string  `json:"title"`
	KBID             *string `json:"kb_id"`
	EnableKB         bool    `json:"enable_kb"`
	RuntimeProfileID *string `json:"runtime_profile_id"` // 本体运行方案（外部引用，O-6）
	OntologyEnabled  bool    `json:"ontology_enabled"`
	// EnableSkills 会话级技能开关（nil=未指定：创建默认开、更新保留原值）
	EnableSkills *bool `json:"enable_skills,omitempty"`
	// InterruptState 中断挂起信息 JSON（ask_human 等 HIL 中断；空=无。M11 收尾）
	InterruptState string `json:"interrupt_state,omitempty"`
	// ToolApproval 对话级工具审批覆盖（REQ-135②：nil=不改 | ''=跟随 Agent 级 | on | off）
	ToolApproval *string `json:"tool_approval,omitempty"`
	// ModelConnID 会话级模型连接覆盖（REQ-291：nil=不改 | ''=跟随智能体默认 | 连接 ID；
	// 合并序 请求级 > 会话级 > 智能体默认 > 全局默认，与 REQ-135② 同构）
	ModelConnID *string `json:"model_conn_id,omitempty"`
	// CompanionOntologyID 会话级伴生本体覆盖（REQ-302/M106：nil=不改 | ''=清除覆盖跟随智能体默认 | 本体 ID；
	// 显式覆盖后抽取/召回/入图归属走覆盖本体，跨会话分域；沉淀图仍本体级共享 REQ-211）
	CompanionOntologyID *string `json:"companion_ontology_id,omitempty"`
	// ContextState 上下文压缩状态（REQ-201/M37：JSON 摘要+覆盖消息 ID；空=未压缩）
	ContextState string `json:"context_state,omitempty"`
	// TodoJSON todo_write 任务清单（REQ-202/M38：模型自写进度，机器可读侧；空=未写）
	TodoJSON string `json:"todo_json,omitempty"`
	// ContextModeOverride 会话级上下文预算档覆盖（REQ-393/M189：nil=不改 | ''=跟随 Agent 级 |
	// compact/standard/full=覆盖；合并序 会话>Agent>全局默认，与 REQ-291 同构）
	ContextModeOverride *string `json:"context_mode,omitempty"`
	TopK                int     `json:"top_k"`
	MinScore            float64 `json:"min_score"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// Message 消息（role: user/assistant/system/tool）。
type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	Meta           string `json:"meta,omitempty"` // JSON 字符串：agent 名、usage 等
	CreatedAt      string `json:"created_at"`
}

// RunEvent 过程事件（SSE 事件持久化，用于历史还原）。
type RunEvent struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	Type           string `json:"type"`
	Data           string `json:"data,omitempty"` // JSON 字符串
	CreatedAt      string `json:"created_at"`
	// SchemaVersion 事件契约版本（REQ-224/M52，只增不改）：1=存量（M0~M51），2=结构化审计事件
	// （approval.granted|denied/hook.denied/verify.completed|failed/connector.degraded）+
	// tool.result 截断标记 + message.delta 门控落库。零值=存量行兼容读。
	SchemaVersion int `json:"schema_version,omitempty"`
}

// EventSchemaVersion 当前事件契约版本（写入侧盖戳；消费方按版本兼容读）。
const EventSchemaVersion = 2

// ModelConnection 模型连接（chat / embedding）。
type ModelConnection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ConnType   string `json:"conn_type"` // chat | embedding
	Protocol   string `json:"protocol"`  // openai_compat | anthropic（REQ-172）
	BaseURL    string `json:"base_url"`
	ModelName  string `json:"model_name"`
	APIKeyHint string `json:"api_key_hint"` // 掩码，如 sk-****ab12
	HasKey     bool   `json:"has_key"`      // 是否已存 key（不回传明文）
	Enabled    bool   `json:"enabled"`
	IsDefault  bool   `json:"is_default"`
	// ProviderGroupID 供应商分组（REQ-148）：分组标识与 BaseURL 解耦，同一供应商可多实例
	ProviderGroupID string `json:"provider_group_id,omitempty"`
	// ProviderAlias 组别名的连接级快照（List/Get 联查 provider_group 计算返回，展示层用；请求携带会被忽略）
	ProviderAlias string `json:"provider_alias,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	// 请求体携带、不落库不回显
	// ---- write-only 字段（请求可携带，响应不回传明文） ----
	APIKey string `json:"api_key,omitempty"`
	// CopyKeyFrom 指定源连接 ID：创建/更新时若未携带明文 api_key，则复用源连接已存密文。
	// 支撑「供应商 → 多模型」语义：Key 归属供应商，组内模型连接经此共享同一密文。
	CopyKeyFrom string `json:"copy_key_from,omitempty"`
}
