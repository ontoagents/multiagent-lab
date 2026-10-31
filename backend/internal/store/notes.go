package store

// REQ-495/M304 笔记本 Notes 存储层（自产成果文档资产层；详设 11 §2 REQ-495 行+三拍板）。
// 容器=笔记本→页面两级；源=manual|local|git（MVP 两源=本地目录+git 本地克隆路径，触碰同步=mtime）；
// 工作区绑定=一对多（notebook.workspace_id REFERENCES workspace SET NULL，Q-24 实体反转宿主端）；
// KB 回流=引用语义（kb_notebook_source 登记表，笔记本单源不拷贝）。

import (
	"database/sql"
	"errors"
)

// ---- 笔记本 ----

func scanNotebook(row interface{ Scan(...any) error }) (*Notebook, error) {
	var n Notebook
	var wsID sql.NullString
	err := row.Scan(&n.ID, &n.Name, &n.Description, &wsID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if wsID.Valid {
		n.WorkspaceID = &wsID.String
	}
	return &n, nil
}

const notebookCols = `id, name, description, workspace_id, created_at, updated_at`

// ListNotebooks 返回全部笔记本（可选工作区过滤）。
func (s *Store) ListNotebooks(workspaceID string) ([]*Notebook, error) {
	q := `SELECT ` + notebookCols + ` FROM notebook WHERE 1=1`
	var args []any
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	q += ` ORDER BY created_at, id`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Notebook
	for rows.Next() {
		n, err := scanNotebook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNotebook 按 ID 查询。
func (s *Store) GetNotebook(id string) (*Notebook, error) {
	row := s.DB.QueryRow(`SELECT ` + notebookCols + ` FROM notebook WHERE id = ?`, id)
	n, err := scanNotebook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// CreateNotebook 新建笔记本（workspace_id 可空=个人资产不绑工作区；一对多绑定）。
func (s *Store) CreateNotebook(n *Notebook) (*Notebook, error) {
	if n.ID == "" {
		n.ID = NewID()
	}
	if n.Name == "" {
		return nil, &HTTPError{Status: 400, Msg: "name required"}
	}
	if n.WorkspaceID != nil && *n.WorkspaceID != "" {
		if _, err := s.GetWorkspace(*n.WorkspaceID); err != nil {
			return nil, &HTTPError{Status: 400, Msg: "workspace not found"}
		}
	}
	_, err := s.DB.Exec(`INSERT INTO notebook (`+notebookCols+`) VALUES (?,?,?,?,?,?)`,
		n.ID, n.Name, n.Description, n.WorkspaceID, now(), now())
	if err != nil {
		return nil, err
	}
	return s.GetNotebook(n.ID)
}

// UpdateNotebook 更新名称/描述/工作区绑定（触碰同步语义：改名不改页）。
func (s *Store) UpdateNotebook(n *Notebook) (*Notebook, error) {
	if n.Name == "" {
		return nil, &HTTPError{Status: 400, Msg: "name required"}
	}
	if n.WorkspaceID != nil && *n.WorkspaceID != "" {
		if _, err := s.GetWorkspace(*n.WorkspaceID); err != nil {
			return nil, &HTTPError{Status: 400, Msg: "workspace not found"}
		}
	}
	res, err := s.DB.Exec(`UPDATE notebook SET name=?, description=?, workspace_id=?, updated_at=? WHERE id=?`,
		n.Name, n.Description, n.WorkspaceID, now(), n.ID)
	if err != nil {
		return nil, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetNotebook(n.ID)
}

// DeleteNotebook 删除笔记本（页面级联 CASCADE；引用 KB 的语料登记由 handler 先行清理重索引）。
func (s *Store) DeleteNotebook(id string) error {
	res, err := s.DB.Exec(`DELETE FROM notebook WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- 页面 ----

func scanNotePage(row interface{ Scan(...any) error }) (*NotePage, error) {
	var p NotePage
	err := row.Scan(&p.ID, &p.NotebookID, &p.Title, &p.ContentMD, &p.Source, &p.SourceRef, &p.SourceMtime, &p.Position, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

const notePageCols = `id, notebook_id, title, content_md, source, source_ref, source_mtime, position, created_at, updated_at`

// ListNotePages 笔记本内页面（position 序）。
func (s *Store) ListNotePages(notebookID string) ([]*NotePage, error) {
	rows, err := s.DB.Query(`SELECT ` + notePageCols + ` FROM note_page WHERE notebook_id = ? ORDER BY position, created_at, id`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NotePage
	for rows.Next() {
		p, err := scanNotePage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetNotePage 按 ID 查询。
func (s *Store) GetNotePage(id string) (*NotePage, error) {
	row := s.DB.QueryRow(`SELECT ` + notePageCols + ` FROM note_page WHERE id = ?`, id)
	p, err := scanNotePage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreateNotePage 新建页面（追加到尾部）。
func (s *Store) CreateNotePage(p *NotePage) (*NotePage, error) {
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.NotebookID == "" {
		return nil, &HTTPError{Status: 400, Msg: "notebook_id required"}
	}
	if p.Title == "" {
		return nil, &HTTPError{Status: 400, Msg: "title required"}
	}
	if _, err := s.GetNotebook(p.NotebookID); err != nil {
		return nil, &HTTPError{Status: 400, Msg: "notebook not found"}
	}
	var pos int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(position),0)+1 FROM note_page WHERE notebook_id = ?`, p.NotebookID).Scan(&pos)
	p.Position = pos
	if p.Source == "" {
		p.Source = "manual"
	}
	_, err := s.DB.Exec(`INSERT INTO note_page (`+notePageCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.NotebookID, p.Title, p.ContentMD, p.Source, p.SourceRef, p.SourceMtime, p.Position, now(), now())
	if err != nil {
		return nil, err
	}
	return s.GetNotePage(p.ID)
}

// UpdateNotePage 更新页面（标题/内容/源基线；updated_at=触碰时刻）。
func (s *Store) UpdateNotePage(p *NotePage) (*NotePage, error) {
	if p.Title == "" {
		return nil, &HTTPError{Status: 400, Msg: "title required"}
	}
	res, err := s.DB.Exec(`UPDATE note_page SET title=?, content_md=?, source_ref=?, source_mtime=?, updated_at=? WHERE id=?`,
		p.Title, p.ContentMD, p.SourceRef, p.SourceMtime, now(), p.ID)
	if err != nil {
		return nil, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetNotePage(p.ID)
}

// DeleteNotePage 删除页面。
func (s *Store) DeleteNotePage(id string) error {
	res, err := s.DB.Exec(`DELETE FROM note_page WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- KB 回流引用登记（三拍板③：引用语义+触碰同步） ----

// ListNotebookSources 笔记本被哪些 KB 登记为语料源。
func (s *Store) ListNotebookSources(notebookID string) ([]*KBNotebookSource, error) {
	rows, err := s.DB.Query(`SELECT id, kb_id, notebook_id, created_at FROM kb_notebook_source WHERE notebook_id = ? ORDER BY created_at`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KBNotebookSource
	for rows.Next() {
		var r KBNotebookSource
		if err := rows.Scan(&r.ID, &r.KBID, &r.NotebookID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ListNotebookSourceKBs 反查：某 KB 登记的全部笔记本（触碰同步时 handler 侧按需）。
func (s *Store) ListNotebookSourceKBs(notebookID string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT kb_id FROM kb_notebook_source WHERE notebook_id = ?`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateNotebookSource 登记引用（重复登记幂等）。
func (s *Store) CreateNotebookSource(kbID, notebookID string) (*KBNotebookSource, error) {
	if _, err := s.GetKnowledgeBase(kbID); err != nil {
		return nil, &HTTPError{Status: 400, Msg: "kb not found"}
	}
	if _, err := s.GetNotebook(notebookID); err != nil {
		return nil, &HTTPError{Status: 400, Msg: "notebook not found"}
	}
	existing, _ := s.ListNotebookSourceKBs(notebookID)
	for _, id := range existing {
		if id == kbID {
			return &KBNotebookSource{KBID: kbID, NotebookID: notebookID}, nil
		}
	}
	id := NewID()
	if _, err := s.DB.Exec(`INSERT INTO kb_notebook_source (id, kb_id, notebook_id, created_at) VALUES (?,?,?,?)`, id, kbID, notebookID, now()); err != nil {
		return nil, err
	}
	return &KBNotebookSource{ID: id, KBID: kbID, NotebookID: notebookID}, nil
}

// DeleteNotebookSource 解除登记。
func (s *Store) DeleteNotebookSource(kbID, notebookID string) error {
	res, err := s.DB.Exec(`DELETE FROM kb_notebook_source WHERE kb_id = ? AND notebook_id = ?`, kbID, notebookID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}
