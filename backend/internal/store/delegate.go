package store

import "database/sql"

// REQ-208/M42 余项：可继续子 Agent（continuation）——后台委派登记面。
// agent_as_tool 协调者经 delegate_background 把任务委派给成员后在子会话（一等会话，
// scope=agent）后台执行：协调者不阻塞，可 steer 插话 / interrupt 打断 / collect 收取结果；
// 冷恢复=子会话历史持久化，重开即接续上下文（续跑走同一会话多轮）。

// Delegate 一条后台委派（子会话状态机宿主）。
type Delegate struct {
	ID                   string  `json:"id"`
	ParentConversationID string  `json:"parent_conversation_id"`
	ChildConversationID  string  `json:"child_conversation_id"`
	MemberAgentID        string  `json:"member_agent_id"`
	MemberName           string  `json:"member_name,omitempty"`
	Task                 string  `json:"task"`
	Status               string  `json:"status"` // running | completed | failed | stopped
	RunID                string  `json:"run_id,omitempty"`
	SteerNote            string  `json:"steer_note,omitempty"` // 运行中收到的插话（续跑轮消费后清空）
	ResultDigest         string  `json:"result_digest,omitempty"`
	Error                string  `json:"error,omitempty"`
	CreatedAt            string  `json:"created_at"`
	FinishedAt           *string `json:"finished_at,omitempty"`
}

// CreateDelegate 落一条委派登记。
func (s *Store) CreateDelegate(d *Delegate) error {
	if d.ID == "" {
		d.ID = NewID()
	}
	if d.Status == "" {
		d.Status = "running"
	}
	_, err := s.DB.Exec(`INSERT INTO conversation_delegate (id, parent_conversation_id, child_conversation_id, member_agent_id, member_name, task, status, run_id, steer_note, result_digest, error, created_at, finished_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.ParentConversationID, d.ChildConversationID, d.MemberAgentID, d.MemberName, d.Task, d.Status, d.RunID, d.SteerNote, d.ResultDigest, d.Error, now(), d.FinishedAt)
	return err
}

// GetDelegate 按 ID 查询。
func (s *Store) GetDelegate(id string) (*Delegate, error) {
	row := s.DB.QueryRow(`SELECT id, parent_conversation_id, child_conversation_id, member_agent_id, COALESCE(member_name,''), task, status, COALESCE(run_id,''), COALESCE(steer_note,''), COALESCE(result_digest,''), COALESCE(error,''), created_at, finished_at FROM conversation_delegate WHERE id = ?`, id)
	return scanDelegate(row)
}

// ListDelegatesByParent 父会话的委派列表（新→旧）。
func (s *Store) ListDelegatesByParent(convID string) ([]*Delegate, error) {
	return s.listDelegates(`WHERE parent_conversation_id = ? ORDER BY created_at DESC, id DESC`, convID)
}

// ListRunningDelegates 活跃委派（重启孤儿清点用）。
func (s *Store) ListRunningDelegates() ([]*Delegate, error) {
	return s.listDelegates(`WHERE status = 'running' ORDER BY created_at`, "")
}

func (s *Store) listDelegates(where, arg string) ([]*Delegate, error) {
	rows, err := s.DB.Query(`SELECT id, parent_conversation_id, child_conversation_id, member_agent_id, COALESCE(member_name,''), task, status, COALESCE(run_id,''), COALESCE(steer_note,''), COALESCE(result_digest,''), COALESCE(error,''), created_at, finished_at FROM conversation_delegate `+where, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Delegate
	for rows.Next() {
		d, err := scanDelegate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanDelegate(row interface{ Scan(...any) error }) (*Delegate, error) {
	d := &Delegate{}
	var finished sql.NullString
	if err := row.Scan(&d.ID, &d.ParentConversationID, &d.ChildConversationID, &d.MemberAgentID, &d.MemberName, &d.Task, &d.Status, &d.RunID, &d.SteerNote, &d.ResultDigest, &d.Error, &d.CreatedAt, &finished); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if finished.Valid {
		d.FinishedAt = &finished.String
	}
	return d, nil
}

// UpdateDelegateStatus 状态迁移（completed/failed 附结果摘要或错误；stopped 附原因）。
func (s *Store) UpdateDelegateStatus(id, status, resultDigest, errMsg string) error {
	res, err := s.DB.Exec(`UPDATE conversation_delegate SET status=?, result_digest=?, error=?, finished_at=CASE WHEN ? IN ('completed','failed','stopped') THEN ? ELSE finished_at END WHERE id=?`,
		status, resultDigest, errMsg, status, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDelegateRun 运行轮信息回写（run_id/状态回 running——steer 续跑轮复用）。
func (s *Store) SetDelegateRun(id, runID string) error {
	_, err := s.DB.Exec(`UPDATE conversation_delegate SET run_id=?, status='running', error='' WHERE id=?`, runID, id)
	return err
}

// SetDelegateSteer 记插话（返回当前累计值；并发插话追加而非覆盖）。
func (s *Store) SetDelegateSteer(id, message string) (string, error) {
	var cur string
	if err := s.DB.QueryRow(`SELECT COALESCE(steer_note,'') FROM conversation_delegate WHERE id=?`, id).Scan(&cur); err != nil {
		return "", err
	}
	merged := message
	if cur != "" {
		merged = cur + "\n" + message
	}
	_, err := s.DB.Exec(`UPDATE conversation_delegate SET steer_note=? WHERE id=?`, merged, id)
	return merged, err
}

// TakeDelegateSteer 取走插话（原子清空——续跑轮消费）。
func (s *Store) TakeDelegateSteer(id string) (string, error) {
	var cur string
	if err := s.DB.QueryRow(`SELECT COALESCE(steer_note,'') FROM conversation_delegate WHERE id=?`, id).Scan(&cur); err != nil {
		return "", err
	}
	if cur != "" {
		if _, err := s.DB.Exec(`UPDATE conversation_delegate SET steer_note='' WHERE id=?`, id); err != nil {
			return "", err
		}
	}
	return cur, nil
}

// MarkOrphanDelegates 重启清点：进程内不再存活的 running 委派标记 stopped
// （可冷恢复——子会话历史完整，续跑即接续）。
func (s *Store) MarkOrphanDelegates() (int64, error) {
	res, err := s.DB.Exec(`UPDATE conversation_delegate SET status='stopped', error='backend 重启中断——子会话历史完整，可从子会话续跑（冷恢复）', finished_at=? WHERE status='running'`, now())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
