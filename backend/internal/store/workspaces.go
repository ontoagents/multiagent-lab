package store

// REQ-548/M355 工作区领域容器存储（Q-24 分层批①；详设 106 号 §二/§五 M1a）。
// workspace=项目之上的新领域实体：只承载 KB 默认库/本体运行方案/默认智能体三项创建时默认值，
// 不插运行期合并链（模型四层/审批/context_mode/伴生四链零改动）。

import (
	"database/sql"
	"errors"
)

func scanWorkspace(row interface{ Scan(...any) error }) (*Workspace, error) {
	var w Workspace
	err := row.Scan(&w.ID, &w.Name, &w.Description, &w.DefaultKBID, &w.DefaultProfileID, &w.DefaultAgentID, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

const workspaceCols = `id, name, description, default_kb_id, default_profile_id, default_agent_id, created_at, updated_at`

// ListWorkspaces 返回全部工作区（创建序）。
func (s *Store) ListWorkspaces() ([]*Workspace, error) {
	rows, err := s.DB.Query(`SELECT ` + workspaceCols + ` FROM workspace ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWorkspace 按 ID 查询。
func (s *Store) GetWorkspace(id string) (*Workspace, error) {
	row := s.DB.QueryRow(`SELECT ` + workspaceCols + ` FROM workspace WHERE id = ?`, id)
	w, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

// CreateWorkspace 新建工作区。
func (s *Store) CreateWorkspace(w *Workspace) (*Workspace, error) {
	if w.ID == "" {
		w.ID = NewID()
	}
	if w.Name == "" {
		return nil, &HTTPError{Status: 400, Msg: "name required"}
	}
	_, err := s.DB.Exec(`INSERT INTO workspace (`+workspaceCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		w.ID, w.Name, w.Description, w.DefaultKBID, w.DefaultProfileID, w.DefaultAgentID, now(), now())
	if err != nil {
		return nil, err
	}
	return s.GetWorkspace(w.ID)
}

// UpdateWorkspace 更新名称/描述/默认值（全量覆盖；updated_at 刷新触发乐观锁基线）。
func (s *Store) UpdateWorkspace(w *Workspace) (*Workspace, error) {
	if w.Name == "" {
		return nil, &HTTPError{Status: 400, Msg: "name required"}
	}
	res, err := s.DB.Exec(`UPDATE workspace SET name=?, description=?, default_kb_id=?, default_profile_id=?, default_agent_id=?, updated_at=? WHERE id=?`,
		w.Name, w.Description, w.DefaultKBID, w.DefaultProfileID, w.DefaultAgentID, now(), w.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetWorkspace(w.ID)
}

// DeleteWorkspace 删除工作区——直挂对话经 FK ON DELETE SET NULL 回落智能体页（消息资产保留，
// Q-24 拍板采建议默认；对照 project CASCADE——对话含消息资产更保守）。
func (s *Store) DeleteWorkspace(id string) error {
	res, err := s.DB.Exec(`DELETE FROM workspace WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
