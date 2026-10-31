package store

import (
	"database/sql"
	"errors"
)

func scanConversation(row interface{ Scan(...any) error }) (*Conversation, error) {
	var c Conversation
	var agentID, projectID, workspaceID, kbID, profileID sql.NullString
	var enableKB, ontoEnabled, enableSkills int
	var toolApproval, modelConnID sql.NullString
	var compOntologyID, contextMode string
	err := row.Scan(&c.ID, &c.Scope, &agentID, &projectID, &workspaceID, &c.Title, &kbID, &enableKB, &profileID, &ontoEnabled, &c.TopK, &c.MinScore, &c.CreatedAt, &c.UpdatedAt, &enableSkills, &c.InterruptState, &toolApproval, &c.ContextState, &c.TodoJSON, &modelConnID, &compOntologyID, &contextMode)
	if err != nil {
		return nil, err
	}
	if toolApproval.Valid {
		ta := toolApproval.String
		c.ToolApproval = &ta
	}
	// REQ-291：会话级模型覆盖（NOT NULL DEFAULT ''，恒非空；''=跟随智能体默认）
	if modelConnID.Valid {
		mc := modelConnID.String
		c.ModelConnID = &mc
	}
	// REQ-302/M106：会话伴生覆盖（恒非空；''=跟随智能体默认）
	c.CompanionOntologyID = &compOntologyID
	// REQ-393/M189：会话级上下文预算档（恒非空；''=跟随 Agent 级）
	c.ContextModeOverride = &contextMode
	if agentID.Valid {
		c.AgentID = &agentID.String
	}
	if projectID.Valid {
		c.ProjectID = &projectID.String
	}
	if workspaceID.Valid {
		c.WorkspaceID = &workspaceID.String
	}
	if kbID.Valid {
		c.KBID = &kbID.String
	}
	if profileID.Valid {
		c.RuntimeProfileID = &profileID.String
	}
	c.EnableKB = enableKB == 1
	c.OntologyEnabled = ontoEnabled == 1
	es := enableSkills == 1
	c.EnableSkills = &es
	return &c, nil
}

const convCols = `id,scope,agent_id,project_id,workspace_id,title,kb_id,enable_kb,runtime_profile_id,ontology_enabled,top_k,min_score,created_at,updated_at,enable_skills,interrupt_state,tool_approval,context_state,todo_json,model_conn_id,companion_ontology_id,context_mode`

// ConversationFilter 会话列表过滤。
type ConversationFilter struct {
	Scope       string
	AgentID     string
	ProjectID   string
	WorkspaceID string // REQ-548/M355：工作区归属过滤（工作区树按归属列会话）
}

// ListConversations 按过滤条件列出对话。
func (s *Store) ListConversations(f ConversationFilter) ([]*Conversation, error) {
	q := `SELECT ` + convCols + ` FROM conversation WHERE 1=1`
	var args []any
	if f.Scope != "" {
		q += ` AND scope = ?`
		args = append(args, f.Scope)
	}
	if f.AgentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, f.AgentID)
	}
	if f.ProjectID != "" {
		q += ` AND project_id = ?`
		args = append(args, f.ProjectID)
	}
	if f.WorkspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, f.WorkspaceID)
	}
	q += ` ORDER BY updated_at DESC, id`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConversation 按 ID 查询。
func (s *Store) GetConversation(id string) (*Conversation, error) {
	row := s.DB.QueryRow(`SELECT `+convCols+` FROM conversation WHERE id = ?`, id)
	c, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// CreateConversation 新建对话；校验 scope 与归属。
func (s *Store) CreateConversation(c *Conversation) (*Conversation, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.Scope != "agent" && c.Scope != "project" {
		return nil, &HTTPError{Status: 400, Msg: "scope must be agent|project"}
	}
	if c.Scope == "agent" && (c.AgentID == nil || *c.AgentID == "") {
		return nil, &HTTPError{Status: 400, Msg: "agent_id required for scope=agent"}
	}
	if c.Scope == "project" && (c.ProjectID == nil || *c.ProjectID == "") {
		return nil, &HTTPError{Status: 400, Msg: "project_id required for scope=project"}
	}
	// REQ-548/M355：project_id/workspace_id 互斥校验（应用层补位——scope CHECK 不动 SQLite 无法
	// ALTER 实测坐实；本函数为唯一写入口，单点守卫覆盖）
	if c.ProjectID != nil && *c.ProjectID != "" && c.WorkspaceID != nil && *c.WorkspaceID != "" {
		return nil, &HTTPError{Status: 400, Msg: "project_id and workspace_id are mutually exclusive"}
	}
	if c.Title == "" {
		c.Title = "新对话"
	}
	if c.TopK == 0 {
		c.TopK = 4
	}
	// enable_skills：未指定时默认开启（前端 full-replace PUT 会显式携带）
	enableSkills := true
	if c.EnableSkills != nil {
		enableSkills = *c.EnableSkills
	}
	// tool_approval：nil → ""（列 NOT NULL DEFAULT ''；内部创建方如 agentd 不传该字段，
	// 显式 nil 插入会违反约束——10a 实测暴露）
	if c.ToolApproval == nil {
		empty := ""
		c.ToolApproval = &empty
	}
	// model_conn_id：nil → ""（REQ-291，同 tool_approval 口径；''=跟随智能体默认）
	if c.ModelConnID == nil {
		empty := ""
		c.ModelConnID = &empty
	}
	// context_mode：nil → ""（REQ-393/M189，同上口径；''=跟随 Agent 级）
	if c.ContextModeOverride == nil {
		empty := ""
		c.ContextModeOverride = &empty
	}
	// REQ-302/M106：会话伴生覆盖（恒非空；''=跟随智能体默认）
	compOID := c.CompanionOntologyID
	if compOID == nil {
		empty := ""
		compOID = &empty
	}
	c.CompanionOntologyID = compOID
	// REQ-548/M355：工作区归属与创建时继承（写时复制——106 号 §三）。互斥通过后：
	// 工作区不存在→400（FK 也会拒绝，显式人话）；KB/方案默认值在默认值填充段一次写入会话行，
	// 运行期装配链/recallKB 单库门禁/retrieval 事件契约零改动。优先级：请求显式参数 > 工作区默认 > 零值
	// （前端建会话从不带 kb_id/profile——显式位为前向兼容，106 号 obj-6 如实标注）。
	// 方案存在性不在 store 层校验（runtime-manager 独立库）——失效走既有 ontology.unavailable 单次降级。
	if c.WorkspaceID != nil && *c.WorkspaceID != "" {
		ws, werr := s.GetWorkspace(*c.WorkspaceID)
		if werr != nil {
			return nil, &HTTPError{Status: 400, Msg: "workspace not found"}
		}
		if c.KBID == nil && ws.DefaultKBID != "" {
			if _, kerr := s.GetKnowledgeBase(ws.DefaultKBID); kerr == nil {
				c.KBID = &ws.DefaultKBID
				c.EnableKB = true // 默认库继承即启用召回（否则默认库无意义）；建后 chip 关闭走 patchConv 正常关闭
			}
			// 默认库已删（config 列无 FK）：静默回落零值——与手动新建无异（106 号降级口径）
		}
		if c.RuntimeProfileID == nil && ws.DefaultProfileID != "" {
			c.RuntimeProfileID = &ws.DefaultProfileID
			c.OntologyEnabled = true
		}
	}
	_, err := s.DB.Exec(`INSERT INTO conversation (`+convCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Scope, c.AgentID, c.ProjectID, c.WorkspaceID, c.Title, c.KBID, boolInt(c.EnableKB), c.RuntimeProfileID, boolInt(c.OntologyEnabled), c.TopK, c.MinScore, now(), now(), boolInt(enableSkills), c.InterruptState, c.ToolApproval, c.ContextState, c.TodoJSON, c.ModelConnID, c.CompanionOntologyID, c.ContextModeOverride)
	if err != nil {
		return nil, err
	}
	return s.GetConversation(c.ID)
}

// UpdateConversation 更新标题与对话级配置（kb / 本体运行方案 / 技能开关 / 工具审批覆盖 / 模型覆盖）。
func (s *Store) UpdateConversation(c *Conversation) (*Conversation, error) {
	// enable_skills / tool_approval / model_conn_id 为 nil 时保留原值（COALESCE），非 nil 时覆盖
	var enableArg any
	if c.EnableSkills != nil {
		enableArg = boolInt(*c.EnableSkills)
	}
	var approvalArg any
	if c.ToolApproval != nil {
		approvalArg = *c.ToolApproval
	}
	var modelArg any
	if c.ModelConnID != nil {
		modelArg = *c.ModelConnID
	}
	// REQ-302/M106：会话伴生覆盖（*string 三态：nil=不改 | ''=清除覆盖跟随智能体默认 | 本体 ID=覆盖）
	var compArg any
	if c.CompanionOntologyID != nil {
		compArg = *c.CompanionOntologyID
	}
	// REQ-393/M189：会话级上下文预算档覆盖（*string 三态同上）
	var ctxModeArg any
	if c.ContextModeOverride != nil {
		ctxModeArg = *c.ContextModeOverride
	}
	res, err := s.DB.Exec(`UPDATE conversation SET title=?,kb_id=?,enable_kb=?,runtime_profile_id=?,ontology_enabled=?,top_k=?,min_score=?,enable_skills=COALESCE(?,enable_skills),tool_approval=COALESCE(?,tool_approval),model_conn_id=COALESCE(?,model_conn_id),companion_ontology_id=COALESCE(?,companion_ontology_id),context_mode=COALESCE(?,context_mode),updated_at=? WHERE id=?`,
		c.Title, c.KBID, boolInt(c.EnableKB), c.RuntimeProfileID, boolInt(c.OntologyEnabled), c.TopK, c.MinScore, enableArg, approvalArg, modelArg, compArg, ctxModeArg, now(), c.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetConversation(c.ID)
}

// DeleteConversation 删除对话（级联消息与事件）。
func (s *Store) DeleteConversation(id string) error {
	res, err := s.DB.Exec(`DELETE FROM conversation WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetConversationInterruptState 更新会话中断挂起状态（"" = 清除；M11 收尾中断恢复）。
// reason 为变更动因（REQ-224① 留痕；如 "interrupt:run_xxx" / "abandon" / "resume"）。
func (s *Store) SetConversationInterruptState(id, stateJSON, reason string) error {
	prev := s.conversationField(id, "interrupt_state")
	res, err := s.DB.Exec(`UPDATE conversation SET interrupt_state=?,updated_at=? WHERE id=?`, stateJSON, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.recordStateChange(id, "interrupt_state", prev, stateJSON, reason)
	return nil
}

// SaveConversationTodo 持久化 todo_write 任务清单（REQ-202/M38：模型自写进度，机器可读侧）。
func (s *Store) SaveConversationTodo(convID, todoJSON string) error {
	prev := s.conversationField(convID, "todo_json")
	if _, err := s.DB.Exec(`UPDATE conversation SET todo_json=?,updated_at=? WHERE id=?`, todoJSON, now(), convID); err != nil {
		return err
	}
	s.recordStateChange(convID, "todo_json", prev, todoJSON, "todo_write")
	return nil
}

// SaveContextState 持久化上下文压缩状态（REQ-201/M37：摘要+覆盖消息 ID；消息只追加，前缀状态长期有效）。
func (s *Store) SaveContextState(convID, stateJSON string) error {
	prev := s.conversationField(convID, "context_state")
	if _, err := s.DB.Exec(`UPDATE conversation SET context_state=?,updated_at=? WHERE id=?`, stateJSON, now(), convID); err != nil {
		return err
	}
	s.recordStateChange(convID, "context_state", prev, stateJSON, "context_compact")
	return nil
}

// conversationField 读会话单列原文（留痕 prev 快照用；列不存在/行不存在返回空）。
func (s *Store) conversationField(id, field string) string {
	var v string
	if err := s.DB.QueryRow(`SELECT `+field+` FROM conversation WHERE id = ?`, id).Scan(&v); err != nil {
		return ""
	}
	return v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetConversationTitle 更新对话标题（REQ-136 对话自动命名：异步提炼后回写；手动改名走 UpdateConversation）。
func (s *Store) SetConversationTitle(id, title string) error {
	res, err := s.DB.Exec(`UPDATE conversation SET title=?,updated_at=? WHERE id=?`, title, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
