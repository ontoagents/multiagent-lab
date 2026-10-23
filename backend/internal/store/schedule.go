package store

import (
	"fmt"
	"time"
)

// REQ-224/M52：C5 定时续跑调度持久化（迁移 038 conversation_schedule）。
// 调度状态出进程入 DB：backend 重启后 LoadSchedules 重新装配定时器（18 v1.91 诚实边界收口）。

// ConversationSchedule 一条会话级定时续跑配置（一会话一条，重设=覆盖）。
type ConversationSchedule struct {
	ConversationID  string `json:"conversation_id"`
	IntervalMinutes int    `json:"interval_minutes"`
	MaxRuns         int    `json:"max_runs"`
	Done            int    `json:"done"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// UpsertSchedule 写入/覆盖调度配置（done 归零——重设即新一轮）。
func (s *Store) UpsertSchedule(convID string, intervalMinutes, maxRuns int) error {
	prev := s.scheduleSnapshot(convID)
	_, err := s.DB.Exec(`INSERT INTO conversation_schedule (conversation_id, interval_minutes, max_runs, done, created_at, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(conversation_id) DO UPDATE SET interval_minutes=excluded.interval_minutes, max_runs=excluded.max_runs, done=0, updated_at=excluded.updated_at`,
		convID, intervalMinutes, maxRuns, 0, now(), now())
	if err == nil {
		// REQ-224①：调度族并入状态变更留痕（field=schedule）
		s.recordStateChange(convID, "schedule", prev, scheduleSnapshotOf(intervalMinutes, maxRuns), "schedule.set")
	}
	return err
}

// GetSchedule 读单条调度；无行返回 nil。
func (s *Store) GetSchedule(convID string) (*ConversationSchedule, error) {
	row := s.DB.QueryRow(`SELECT conversation_id, interval_minutes, max_runs, done, created_at, updated_at FROM conversation_schedule WHERE conversation_id = ?`, convID)
	var sc ConversationSchedule
	var created, updated interface{}
	if err := row.Scan(&sc.ConversationID, &sc.IntervalMinutes, &sc.MaxRuns, &sc.Done, &created, &updated); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	sc.CreatedAt, _ = created.(string)
	sc.UpdatedAt, _ = updated.(string)
	if t, ok := created.(time.Time); ok {
		sc.CreatedAt = t.Format(time.RFC3339)
	}
	return &sc, nil
}

// DeleteSchedule 摘除调度（用户取消 / cap 达到）。REQ-224①：reason 区分动因落留痕。
func (s *Store) DeleteSchedule(convID, reason string) error {
	prev := s.scheduleSnapshot(convID)
	if _, err := s.DB.Exec(`DELETE FROM conversation_schedule WHERE conversation_id = ?`, convID); err != nil {
		return err
	}
	s.recordStateChange(convID, "schedule", prev, "", reason)
	return nil
}

// scheduleSnapshot 读当前调度配置摘要（无行返回空——留痕 prev 用）。
func (s *Store) scheduleSnapshot(convID string) string {
	sc, err := s.GetSchedule(convID)
	if err != nil || sc == nil {
		return ""
	}
	return scheduleSnapshotOf(sc.IntervalMinutes, sc.MaxRuns)
}

func scheduleSnapshotOf(intervalMinutes, maxRuns int) string {
	return fmt.Sprintf("interval=%dm max_runs=%d", intervalMinutes, maxRuns)
}

// IncrementScheduleDone fire 一次：done+1 落库，返回新计数。
func (s *Store) IncrementScheduleDone(convID string) (int, error) {
	_, err := s.DB.Exec(`UPDATE conversation_schedule SET done = done + 1, updated_at = ? WHERE conversation_id = ?`, now(), convID)
	if err != nil {
		return 0, err
	}
	var done int
	if err := s.DB.QueryRow(`SELECT done FROM conversation_schedule WHERE conversation_id = ?`, convID).Scan(&done); err != nil {
		return 0, err
	}
	return done, nil
}

// ListSchedules 全部活跃调度（启动装配用）。
func (s *Store) ListSchedules() ([]*ConversationSchedule, error) {
	rows, err := s.DB.Query(`SELECT conversation_id, interval_minutes, max_runs, done, created_at, updated_at FROM conversation_schedule ORDER BY conversation_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConversationSchedule
	for rows.Next() {
		var sc ConversationSchedule
		if err := rows.Scan(&sc.ConversationID, &sc.IntervalMinutes, &sc.MaxRuns, &sc.Done, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &sc)
	}
	return out, rows.Err()
}
