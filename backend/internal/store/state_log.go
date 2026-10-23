package store

// REQ-224①/M52 余项：会话状态族统一收口——变更留痕。
// interrupt_state / context_state / todo_json / conversation_schedule 此前是四个各自迁移、
// 各自写入的散字段，变更无历史。本档统一收口：所有写入经 store 层 setter 时自动比对
// prev→next，值变化即落 conversation_state_log（迁移 054），历史可追溯；no-op 写入不落。

// ConversationStateLog 一条会话状态变更留痕。
type ConversationStateLog struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Field          string `json:"field"` // interrupt_state | context_state | todo_json | schedule
	Prev           string `json:"prev,omitempty"`
	Next           string `json:"next,omitempty"`
	Reason         string `json:"reason,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// recordStateChange setter 内部统一留痕：prev 与 next 相同则跳过（no-op 写入无历史价值）。
// prev/next 为全量快照原文（JSON 或调度摘要）；留痕失败仅日志不阻断业务写入。
func (s *Store) recordStateChange(convID, field, prev, next, reason string) {
	if prev == next {
		return
	}
	_, err := s.DB.Exec(`INSERT INTO conversation_state_log (id, conversation_id, field, prev, next, reason, created_at) VALUES (?,?,?,?,?,?,?)`,
		NewID(), convID, field, prev, next, reason, now())
	if err != nil {
		logStateWriteErr(field, err)
	}
}

// ListConversationStateLog 会话状态变更历史（倒序；limit≤0 取默认 200）。
func (s *Store) ListConversationStateLog(convID string, limit int) ([]*ConversationStateLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT id, conversation_id, field, COALESCE(prev,''), COALESCE(next,''), reason, created_at
		FROM conversation_state_log WHERE conversation_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, convID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConversationStateLog
	for rows.Next() {
		var l ConversationStateLog
		if err := rows.Scan(&l.ID, &l.ConversationID, &l.Field, &l.Prev, &l.Next, &l.Reason, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}
