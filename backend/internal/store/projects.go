package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	var constraints string
	var agentIDs string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.CollabMode, &p.WorkflowMode, &constraints, &agentIDs, &p.Coordinator, &p.CreatedAt, &p.UpdatedAt, &p.LocalDir, &p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(agentIDs), &p.AgentIDs)
	if p.AgentIDs == nil {
		p.AgentIDs = []string{}
	}
	p.Constraints = constraints
	return &p, nil
}

const projectCols = `p.id, p.name, p.description, p.collab_mode, p.workflow_mode, p.constraints,
 COALESCE((SELECT json_group_array(agent_id) FROM project_agent pa WHERE pa.project_id = p.id), '[]') AS agent_ids,
 COALESCE((SELECT agent_id FROM project_agent pa WHERE pa.project_id = p.id AND pa.role = 'coordinator' LIMIT 1), '') AS coordinator,
 p.created_at, p.updated_at, p.local_dir, COALESCE(p.workspace_id, '') AS workspace_id`

// ListProjects 返回全部项目。
func (s *Store) ListProjects() ([]*Project, error) {
	rows, err := s.DB.Query(`SELECT ` + projectCols + ` FROM project p ORDER BY p.created_at, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListProjectsByWorkspace REQ-550/M357：按归属工作区过滤项目（工作区侧板 projects 视图数据源；
// 未归属项目不入结果）。
func (s *Store) ListProjectsByWorkspace(workspaceID string) ([]*Project, error) {
	rows, err := s.DB.Query(`SELECT `+projectCols+` FROM project p WHERE p.workspace_id = ? ORDER BY p.created_at, p.id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject 按 ID 查询。
func (s *Store) GetProject(id string) (*Project, error) {
	row := s.DB.QueryRow(`SELECT `+projectCols+` FROM project p WHERE p.id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreateProject 新建项目（成员经 SetProjectAgents 单独设置）。
func (s *Store) CreateProject(p *Project) (*Project, error) {
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.CollabMode == "" {
		p.CollabMode = "agent_as_tool"
	}
	if p.WorkflowMode == "" {
		p.WorkflowMode = "free"
	}
	_, err := s.DB.Exec(`INSERT INTO project (id,name,description,collab_mode,workflow_mode,constraints,local_dir,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Description, p.CollabMode, p.WorkflowMode, p.Constraints, p.LocalDir, now(), now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return s.GetProject(p.ID)
}

// UpdateProject 更新项目基础字段。
func (s *Store) UpdateProject(p *Project) (*Project, error) {
	res, err := s.DB.Exec(`UPDATE project SET name=?,description=?,collab_mode=?,workflow_mode=?,constraints=?,local_dir=?,updated_at=? WHERE id=?`,
		p.Name, p.Description, p.CollabMode, p.WorkflowMode, p.Constraints, p.LocalDir, now(), p.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetProject(p.ID)
}

// DeleteProject 删除项目（级联对话）。
func (s *Store) DeleteProject(id string) error {
	res, err := s.DB.Exec(`DELETE FROM project WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectWorkspace REQ-550/M357：归属（workspaceID 非空）/解除（空）项目到工作区。
// 归属校验工作区存在性（400 人话——FK 兜底但显式报错优先）；项目执行容器字段零变化；
// 已建会话不受影响（继承只在创建时求值——验收口径「归属变更后新建会话才受影响」）。
func (s *Store) SetProjectWorkspace(projectID, workspaceID string) (*Project, error) {
	if workspaceID != "" {
		if _, err := s.GetWorkspace(workspaceID); err != nil {
			return nil, &HTTPError{Status: 400, Msg: "workspace not found"}
		}
	}
	// 解除必须置 NULL 而非 ''（DSN foreign_keys(1)——'' 不在 workspace(id) 会违反 FK）
	var wsVal any
	if workspaceID != "" {
		wsVal = workspaceID
	}
	res, err := s.DB.Exec(`UPDATE project SET workspace_id=?, updated_at=? WHERE id=?`, wsVal, now(), projectID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetProject(projectID)
}

// ApplyWorkspaceDefaultsToProject REQ-550/M357：「应用到现有会话」幂等批量——把当前工作区默认值
// 补齐到项目既有会话（106 号 §三：幂等补齐只覆写仍处继承值的行）。终值口径（无继承溯源列下的
// 可判定集）：kb_id/runtime_profile_id 为空 → 写入当前工作区默认（库需存在，写 kb 同时置
// enable_kb=true；方案写同时置 ontology_enabled=true）；绑定值等于当前工作区默认 → 重申绑定不动
// 开关（显式关 chip 保留）；绑定值偏离（用户显式换绑）→ 不动。返回补齐会话数。
// updated_at 必 bump（106 号 obj-7 口径注记：开着的会话窗下一次 patchConv 409 为预期行为）。
// 幂等：重复执行，第二次零变化。
func (s *Store) ApplyWorkspaceDefaultsToProject(projectID string) (int, error) {
	p, err := s.GetProject(projectID)
	if err != nil {
		return 0, err
	}
	if p.WorkspaceID == "" {
		return 0, &HTTPError{Status: 400, Msg: "project not assigned to a workspace"}
	}
	ws, err := s.GetWorkspace(p.WorkspaceID)
	if err != nil {
		return 0, &HTTPError{Status: 400, Msg: "workspace not found"}
	}
	convs, err := s.ListConversations(ConversationFilter{ProjectID: projectID})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range convs {
		changed := false
		if (c.KBID == nil || *c.KBID == "") && ws.DefaultKBID != "" {
			if _, kerr := s.GetKnowledgeBase(ws.DefaultKBID); kerr == nil {
				if _, uerr := s.DB.Exec(`UPDATE conversation SET kb_id=?, enable_kb=1, updated_at=? WHERE id=?`, ws.DefaultKBID, now(), c.ID); uerr == nil {
					changed = true
				}
			}
		}
		if (c.RuntimeProfileID == nil || *c.RuntimeProfileID == "") && ws.DefaultProfileID != "" {
			if _, uerr := s.DB.Exec(`UPDATE conversation SET runtime_profile_id=?, ontology_enabled=1, updated_at=? WHERE id=?`, ws.DefaultProfileID, now(), c.ID); uerr == nil {
				changed = true
			}
		}
		if changed {
			n++
		}
	}
	return n, nil
}

// ---- 项目文件（M11 §5.2 project_file）----

// InsertProjectFile 写入文件元数据，返回 ID。
func (s *Store) InsertProjectFile(pf *ProjectFile) (string, error) {
	if pf.ID == "" {
		pf.ID = NewID()
	}
	_, err := s.DB.Exec(`INSERT INTO project_file (id,project_id,conversation_id,name,path,size,mime,source,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		pf.ID, pf.ProjectID, pf.ConversationID, pf.Name, pf.Path, pf.Size, pf.Mime, pf.Source, now())
	return pf.ID, err
}

// ListProjectFiles 列出项目文件（产物 + 上传）。
func (s *Store) ListProjectFiles(projectID string) ([]*ProjectFile, error) {
	rows, err := s.DB.Query(`SELECT id,project_id,IFNULL(conversation_id,''),name,path,IFNULL(size,0),IFNULL(mime,''),IFNULL(source,'upload'),IFNULL(created_at,'') FROM project_file WHERE project_id = ? ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ProjectFile
	for rows.Next() {
		pf := &ProjectFile{}
		if err := rows.Scan(&pf.ID, &pf.ProjectID, &pf.ConversationID, &pf.Name, &pf.Path, &pf.Size, &pf.Mime, &pf.Source, &pf.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, pf)
	}
	return out, rows.Err()
}

// GetProjectFile 按 ID 取文件元数据。
func (s *Store) GetProjectFile(id string) (*ProjectFile, error) {
	row := s.DB.QueryRow(`SELECT id,project_id,IFNULL(conversation_id,''),name,path,IFNULL(size,0),IFNULL(mime,''),IFNULL(source,'upload'),IFNULL(created_at,'') FROM project_file WHERE id = ?`, id)
	pf := &ProjectFile{}
	if err := row.Scan(&pf.ID, &pf.ProjectID, &pf.ConversationID, &pf.Name, &pf.Path, &pf.Size, &pf.Mime, &pf.Source, &pf.CreatedAt); err != nil {
		return nil, ErrNotFound
	}
	return pf, nil
}
