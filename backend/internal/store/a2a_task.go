package store

import (
	"encoding/json"
	"time"
)

// REQ-387/M183：A2A 任务持久化（迁移 062）——进程内 map 仍为热缓存，落库为投影写穿；
// 重启后 tasks/get 回读 DB（终态任务可查，running 行启动对账标 failed）。

// A2ATaskRow a2a_task 表行（payload 承载 artifact/error/history）。
type A2ATaskRow struct {
	ID        string `json:"id"`
	ContextID string `json:"context_id"`
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	Timestamp string `json:"timestamp"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// A2APayload 任务非列字段载荷。
type A2APayload struct {
	ArtifactText string           `json:"artifact_text,omitempty"`
	ErrorText    string           `json:"error_text,omitempty"`
	History      []map[string]any `json:"history,omitempty"`
}

// UpsertA2ATask 写穿一行。
func (s *Store) UpsertA2ATask(r *A2ATaskRow) error {
	if r.UpdatedAt == "" {
		r.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
	}
	if r.CreatedAt == "" {
		r.CreatedAt = r.UpdatedAt
	}
	_, err := s.DB.Exec(`INSERT INTO a2a_task (id, context_id, agent_id, state, timestamp, payload, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET state=excluded.state, timestamp=excluded.timestamp, payload=excluded.payload, updated_at=excluded.updated_at`,
		r.ID, r.ContextID, r.AgentID, r.State, r.Timestamp, r.Payload, r.CreatedAt, r.UpdatedAt)
	return err
}

// GetA2ATask 按 ID 读行（无行 nil）。
func (s *Store) GetA2ATask(id string) (*A2ATaskRow, error) {
	row := s.DB.QueryRow(`SELECT id, context_id, agent_id, state, timestamp, payload, created_at, updated_at FROM a2a_task WHERE id=?`, id)
	var r A2ATaskRow
	if err := row.Scan(&r.ID, &r.ContextID, &r.AgentID, &r.State, &r.Timestamp, &r.Payload, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// MarkOrphanA2ATasks 启动对账：悬挂 working → failed（诚实报错重新委派）。
func (s *Store) MarkOrphanA2ATasks() (int64, error) {
	res, err := s.DB.Exec(`UPDATE a2a_task SET state='failed', payload=json_set(payload, '$.error_text', 'backend 重启中断——请重新委派'), updated_at=? WHERE state IN ('working','submitted')`,
		now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// marshalA2APayload 载荷序列化。
func MarshalA2APayload(p A2APayload) string {
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}
