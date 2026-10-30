package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// 消息与运行事件持久化（历史还原 / 事件时间线）。

// InsertMessage 保存消息。
func (s *Store) InsertMessage(m *Message) (*Message, error) {
	if m.ID == "" {
		m.ID = NewID()
	}
	_, err := s.DB.Exec(`INSERT INTO message (id,conversation_id,role,content,meta,created_at) VALUES (?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.Role, m.Content, m.Meta, now())
	if err != nil {
		return nil, err
	}
	s.InsertMessageFTS(m) // REQ-528/M335：FTS 写穿（辅助索引失败仅告警）
	return m, nil
}

// ListMessages 按时间升序返回对话全部消息（历史还原）。
// ListMessagesPaged REQ-413/M209：长会话消息分页读取（倒数第 page 页，每页 pageSize 条——
// 沿 ListEventsQ 分页先例；页序倒序定位=近期优先打开护栏）。page 从 1 起。
func (s *Store) ListMessagesPaged(convID string, page, pageSize int) ([]*Message, int, error) {
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 200
	}
	if page <= 0 {
		page = 1
	}
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM message WHERE conversation_id=?`, convID).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	rows, err := s.DB.Query(`SELECT id, conversation_id, role, content, COALESCE(meta,''), created_at FROM message
		WHERE conversation_id=? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, convID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Meta, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &m)
	}
	// 倒序取页后正序还原（对话时序）
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, total, rows.Err()
}

func (s *Store) ListMessages(convID string) ([]*Message, error) {
	rows, err := s.DB.Query(`SELECT id,conversation_id,role,content,meta,created_at FROM message WHERE conversation_id = ? ORDER BY created_at, id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		var meta sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &meta, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Meta = meta.String
		out = append(out, &m)
	}
	return out, rows.Err()
}

// ListToolEvents 返回对话的工具调用/结果事件（REQ-201 A1：tool 轮次由 run_event 派生重建，
// 零 schema 变更——历史重建恢复 assistant ToolCalls 与 tool 结果消息）。
func (s *Store) ListToolEvents(convID string) ([]*RunEvent, error) {
	rows, err := s.DB.Query(`SELECT id,conversation_id,run_id,type,data,created_at,COALESCE(schema_version,1) FROM run_event WHERE conversation_id = ? AND type IN ('tool.call','tool.result') ORDER BY created_at, id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RunEvent
	for rows.Next() {
		var e RunEvent
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.ConversationID, &e.RunID, &e.Type, &data, &e.CreatedAt, &e.SchemaVersion); err != nil {
			return nil, err
		}
		e.Data = data.String
		out = append(out, &e)
	}
	return out, rows.Err()
}

// CountMessages 统计对话消息数。
func (s *Store) CountMessages(convID string) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM message WHERE conversation_id = ?`, convID).Scan(&n)
	return n, err
}

// InsertEvent 保存运行事件。
func (s *Store) InsertEvent(e *RunEvent) (*RunEvent, error) {
	if e.ID == "" {
		e.ID = NewID()
	}
	sv := e.SchemaVersion
	if sv == 0 {
		sv = 1 // 存量兼容：调用方未显式盖版本按旧契约计
	}
	_, err := s.DB.Exec(`INSERT INTO run_event (id,conversation_id,run_id,type,data,created_at,schema_version) VALUES (?,?,?,?,?,?,?)`,
		e.ID, e.ConversationID, e.RunID, e.Type, e.Data, now(), sv)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ListEvents 按时间升序返回对话全部运行事件（历史还原事件时间线）。
func (s *Store) ListEvents(convID string) ([]*RunEvent, error) {
	rows, err := s.DB.Query(`SELECT id,conversation_id,run_id,type,data,created_at,COALESCE(schema_version,1) FROM run_event WHERE conversation_id = ? ORDER BY created_at, id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RunEvent
	for rows.Next() {
		var e RunEvent
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.ConversationID, &e.RunID, &e.Type, &data, &e.CreatedAt, &e.SchemaVersion); err != nil {
			return nil, err
		}
		e.Data = data.String
		out = append(out, &e)
	}
	return out, rows.Err()
}

// EventQuery 事件查询过滤（REQ-217③：run_id/type/limit/offset；零值字段不参与过滤）。
type EventQuery struct {
	RunID      string // 按运行过滤（轨迹面板「重放此运行」/单运行视图）
	Type       string // 精确类型过滤（tool.call/tool.result/…）
	TypePrefix string // REQ-281：类型前缀过滤（companion. → LIKE 'companion.%'；转义 %/_）
	Limit      int    // ≤0 = 不限
	Offset     int    // ≥0，配 Limit 分页
	Order      string // REQ-330①：desc=最新优先（收尾轮询增量取数——升序+LIMIT 恒取最老窗，伴生存量>50 的会话新事件恒在窗外=轮询盲区）
}

// ListEventsQ 过滤版事件查询（升序不变；返回命中总数供分页——limit 生效时 out 可能是总数的前窗）。
func (s *Store) ListEventsQ(convID string, q EventQuery) ([]*RunEvent, int, error) {
	where := []string{"conversation_id = ?"}
	args := []any{convID}
	if q.RunID != "" {
		where = append(where, "run_id = ?") // 走 idx_run_event_run（迁移 035）
		args = append(args, q.RunID)
	}
	if q.Type != "" {
		where = append(where, "type = ?")
		args = append(args, q.Type)
	}
	if q.TypePrefix != "" {
		where = append(where, "type LIKE ? ESCAPE '\\'")
		args = append(args, escapeLike(q.TypePrefix)+"%")
	}
	total := 0
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM run_event WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sqlq := `SELECT id,conversation_id,run_id,type,data,created_at,COALESCE(schema_version,1) FROM run_event WHERE ` + strings.Join(where, " AND ")
	if q.Order == "desc" { // REQ-330①：最新优先（取数方自行 reverse 回时序；默认升序不变）
		sqlq += ` ORDER BY created_at DESC, id DESC`
	} else {
		sqlq += ` ORDER BY created_at, id`
	}
	if q.Limit > 0 {
		sqlq += fmt.Sprintf(" LIMIT %d OFFSET %d", q.Limit, max(q.Offset, 0))
	}
	rows, err := s.DB.Query(sqlq, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*RunEvent
	for rows.Next() {
		var e RunEvent
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.ConversationID, &e.RunID, &e.Type, &data, &e.CreatedAt, &e.SchemaVersion); err != nil {
			return nil, 0, err
		}
		e.Data = data.String
		out = append(out, &e)
	}
	return out, total, rows.Err()
}
func (s *Store) TouchConversation(convID string) error {
	_, err := s.DB.Exec(`UPDATE conversation SET updated_at = ? WHERE id = ?`, now(), convID)
	return err
}

// escapeLike LIKE 通配转义（配 SQL 端 ESCAPE '\'；kg.go 同约定，REQ-281 前缀过滤引入共享助手）。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ---- REQ-204/M39 C1：中断检查点持久化 ----

// SetCheckpoint 保存中断检查点（gob blob，按 checkpointID 幂等覆盖）。
func (s *Store) SetCheckpoint(id string, blob []byte) error {
	_, err := s.DB.Exec(`INSERT INTO checkpoint (id,blob,created_at) VALUES (?,?,?)
		ON CONFLICT(id) DO UPDATE SET blob=excluded.blob, created_at=excluded.created_at`, id, blob, now())
	return err
}

// GetCheckpoint 取检查点（不存在返回 ok=false）。
func (s *Store) GetCheckpoint(id string) ([]byte, bool, error) {
	var b []byte
	err := s.DB.QueryRow(`SELECT blob FROM checkpoint WHERE id = ?`, id).Scan(&b)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// DeleteCheckpoint 删除检查点（恢复完成/放弃挂起时清理）。
func (s *Store) DeleteCheckpoint(id string) error {
	_, err := s.DB.Exec(`DELETE FROM checkpoint WHERE id = ?`, id)
	return err
}

// ListMessagesAfterCursor REQ-303 E1：游标行之后的增量消息（O(增量)——替代全量拉取后内存过滤）。
// 游标为空/游标消息已删（会话裁剪）→ 回退全量（保守语义与 remainingAfter 一致）。
func (s *Store) ListMessagesAfterCursor(convID, afterMessageID string) ([]*Message, error) {
	if afterMessageID == "" {
		return s.ListMessages(convID)
	}
	cur := &Message{}
	if err := s.DB.QueryRow(`SELECT id,conversation_id,role,content,meta,created_at FROM message WHERE id = ?`, afterMessageID).
		Scan(&cur.ID, &cur.ConversationID, &cur.Role, &cur.Content, &cur.Meta, &cur.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return s.ListMessages(convID) // 游标消息已删 → 保守全量
		}
		return nil, err
	}
	// REQ-303 E1：created_at 参数归一为空格格式（驱动 Scan 可能回 T/Z 格式，
	// 与列内存格式不一致会导致字符串比较失配——真机实证）
	curCreated := strings.Replace(strings.Replace(cur.CreatedAt, "T", " ", 1), "Z", "", 1)
	rows, err := s.DB.Query(`SELECT id,conversation_id,role,content,meta,created_at FROM message
		WHERE conversation_id = ? AND (created_at > ? OR (created_at = ? AND id > ?))
		ORDER BY created_at, id`, convID, curCreated, curCreated, afterMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Meta, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// HarnessAuditRow 跨会话 Harness 审计行（REQ-311 B2：审批/拦截/验证事件跨会话聚合，附会话标题）。
type HarnessAuditRow struct {
	ID                string `json:"id"`
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	RunID             string `json:"run_id"`
	Type              string `json:"type"`
	Data              string `json:"data,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// harnessAuditTypes 进入跨会话审计聚合的事件族（REQ-224 schema v2 结构化审计 + approval.request 挂起——
// 顺带 55 号开放问题③：挂起中的审批请求在审计历史可见）。
var harnessAuditTypes = []string{"approval.request", "approval.granted", "approval.denied", "hook.denied", "verify.completed", "verify.failed"}

// ListHarnessAudit 按智能体倒序聚合其全部会话的 Harness 审计事件。
// v1 范围=agent 直聊会话（conversation.agent_id）；项目会话的成员执行审计仍在各会话轨迹内
//（71 号 §4.2 B2 诚实边界：项目会话 agent_id 为空，跨成员聚合归属语义留后续）。
func (s *Store) ListHarnessAudit(agentID string, limit int) ([]*HarnessAuditRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	in := make([]string, len(harnessAuditTypes))
	args := make([]any, 0, len(harnessAuditTypes)+2)
	for i, t := range harnessAuditTypes {
		in[i] = "?"
		args = append(args, t)
	}
	typeIn := "(" + strings.Join(in, ",") + ")"
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM run_event e JOIN conversation c ON c.id=e.conversation_id WHERE c.agent_id=? AND e.type IN `+typeIn, append([]any{agentID}, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT e.id,e.conversation_id,IFNULL(c.title,''),e.run_id,e.type,IFNULL(e.data,''),e.created_at
		FROM run_event e JOIN conversation c ON c.id=e.conversation_id
		WHERE c.agent_id=? AND e.type IN ` + typeIn + ` ORDER BY e.created_at DESC, e.id DESC LIMIT ?`
	args = append([]any{agentID}, args...)
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*HarnessAuditRow{}
	for rows.Next() {
		var r HarnessAuditRow
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.ConversationTitle, &r.RunID, &r.Type, &r.Data, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &r)
	}
	return out, total, rows.Err()
}
