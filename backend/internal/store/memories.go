package store

// REQ-551/M358 工作区记忆作用域存储（Q-24 分层批④收官；Q-23 衍生②转正载体）。
// 可审计口径：每条记忆带来源会话/消息行级溯源；全量经候选确认（无自动入记忆——黑箱记忆不做）。
// 与伴生（agent 隐式抽取入图）/笔记本（显式文档沉淀）三线正交：记忆=工作区级显式确认文本，不入 KG。

import (
	"database/sql"
	"errors"
	"strings"
)

// Memory 工作区记忆行（status: candidate|accepted|rejected）。
type Memory struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspace_id"`
	Content              string `json:"content"`
	SourceConversationID string `json:"source_conversation_id"` // 行级溯源：来源会话（空=手工录入）
	SourceMessageID      string `json:"source_message_id"`      // 行级溯源：来源消息（空=手工录入/整轮汇总）
	Status               string `json:"status"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

const memoryCols = `id, workspace_id, content, source_conversation_id, source_message_id, status, created_at, updated_at`

func scanMemory(row interface{ Scan(...any) error }) (*Memory, error) {
	var m Memory
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.Content, &m.SourceConversationID, &m.SourceMessageID, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMemories 按工作区列记忆（status 空=全量；创建序倒序——新候选在前）。
func (s *Store) ListMemories(workspaceID, status string) ([]*Memory, error) {
	q := `SELECT ` + memoryCols + ` FROM memory WHERE workspace_id = ?`
	args := []any{workspaceID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMemory 新增记忆（候选生成/手工录入共用；同工作区同内容去重——精确匹配候选∪已入，
// 重复抽取风暴天然幂等）。返回落库行与是否实际新增。
func (s *Store) CreateMemory(m *Memory) (*Memory, bool, error) {
	m.Content = strings.TrimSpace(m.Content)
	if m.Content == "" {
		return nil, false, &HTTPError{Status: 400, Msg: "content required"}
	}
	if m.WorkspaceID == "" {
		return nil, false, &HTTPError{Status: 400, Msg: "workspace_id required"}
	}
	if m.Status == "" {
		m.Status = "candidate"
	}
	var exists string
	if err := s.DB.QueryRow(`SELECT id FROM memory WHERE workspace_id=? AND content=? LIMIT 1`, m.WorkspaceID, m.Content).Scan(&exists); err == nil {
		return nil, false, nil // 去重命中：不重复落库（幂等）
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if m.ID == "" {
		m.ID = NewID()
	}
	_, err := s.DB.Exec(`INSERT INTO memory (`+memoryCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		m.ID, m.WorkspaceID, m.Content, m.SourceConversationID, m.SourceMessageID, m.Status, now(), now())
	if err != nil {
		return nil, false, err
	}
	created, gerr := s.GetMemory(m.ID)
	return created, gerr == nil, gerr
}

// GetMemory 按 ID 查询。
func (s *Store) GetMemory(id string) (*Memory, error) {
	row := s.DB.QueryRow(`SELECT `+memoryCols+` FROM memory WHERE id = ?`, id)
	m, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// DecideMemory 候选裁决（沿伴生三桶范式：candidate → accepted/rejected；终态不可再改状态，
// 误裁决走删除重建——保持审计链单向）。返回是否发生状态变更。
func (s *Store) DecideMemory(id string, accept bool) (bool, error) {
	target := "accepted"
	if !accept {
		target = "rejected"
	}
	res, err := s.DB.Exec(`UPDATE memory SET status=?, updated_at=? WHERE id=? AND status='candidate'`, target, now(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// 区分「不存在」与「已裁决」
		if _, err := s.GetMemory(id); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// DeleteMemory 删除记忆（任意状态可删——清理权在用户）。
func (s *Store) DeleteMemory(id string) error {
	res, err := s.DB.Exec(`DELETE FROM memory WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptedMemories 召回读侧：工作区已入记忆（创建序——早事实在前，注入文本稳定）。
func (s *Store) AcceptedMemories(workspaceID string) ([]*Memory, error) {
	rows, err := s.DB.Query(`SELECT `+memoryCols+` FROM memory WHERE workspace_id=? AND status='accepted' ORDER BY created_at, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMemoryCursor 读抽取游标（无记录返回空游标；沿伴生 GetCompanionCursor 范式）。
func (s *Store) GetMemoryCursor(convID string) (string, error) {
	var last string
	err := s.DB.QueryRow(`SELECT last_message_id FROM memory_cursor WHERE conversation_id = ?`, convID).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return last, err
}

// AdvanceMemoryCursor 推进抽取游标（upsert）。
func (s *Store) AdvanceMemoryCursor(convID, lastMessageID string) error {
	_, err := s.DB.Exec(`INSERT INTO memory_cursor (conversation_id,last_message_id,updated_at) VALUES (?,?,?)
		ON CONFLICT(conversation_id) DO UPDATE SET last_message_id=excluded.last_message_id, updated_at=excluded.updated_at`,
		convID, lastMessageID, now())
	return err
}
