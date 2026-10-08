// job.go OntoChat 生成轮异步 job 持久化（REQ-271/M80，定案口径=与 session 同库 SQLite）。
// 生命周期：queued（202 立返）→ running → done|error|cancelled。result_json 存终局载荷
// {reply,stage,round,warning,draft?}，前端经 GET /api/ontochat/jobs/{id} 轮询。
package ontochat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

// jobStaleRunning running 态超过该时长视为进程重启悬挂（自愈收敛，沿启动对账先例——
// 最长生成链路 3 轮 × 300s 超时 + 校验开销 < 16min，取 15min 安全界）。
const jobStaleRunning = 15 * time.Minute

// Job 异步生成任务（与会话同库持久化，重启不丢状态）。
type Job struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Status    string          `json:"status"` // queued|running|done|error|cancelled
	Error     string          `json:"error,omitempty"`
	Progress  string          `json:"progress,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// JobActive 判定 job 是否仍占用会话（同会话同时至多一个生成任务）。
func JobActive(j *Job) bool {
	return j != nil && (j.Status == "queued" || j.Status == "running")
}

// CreateJob 建 queued 任务。
func (s *Store) CreateJob(id, sessionID string) (*Job, error) {
	ts := now()
	_, err := s.db.Exec(
		`INSERT INTO ontochat_job (id, session_id, status, created_at, updated_at) VALUES (?, ?, 'queued', ?, ?)`,
		id, sessionID, ts, ts)
	if err != nil {
		return nil, err
	}
	return s.GetJob(id)
}

// GetJob 读取任务；不存在 → repo.ErrNotFound。
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, session_id, status, error, progress, result_json, created_at, updated_at
		 FROM ontochat_job WHERE id = ?`, id)
	var j Job
	var result sql.NullString
	if err := row.Scan(&j.ID, &j.SessionID, &j.Status, &j.Error, &j.Progress, &result, &j.CreatedAt, &j.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, err
	}
	if result.Valid {
		j.Result = json.RawMessage(result.String)
	}
	return &j, nil
}

// ActiveJobBySession 会话当前未完成任务；running 悬挂（updated_at 超 jobStaleRunning，
// 进程重启遗留）自愈收敛为 error 后视为无活跃任务。
func (s *Store) ActiveJobBySession(sessionID string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id FROM ontochat_job WHERE session_id = ? AND status IN ('queued','running')
		 ORDER BY created_at DESC LIMIT 1`, sessionID)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	j, err := s.GetJob(id)
	if err != nil {
		return nil, err
	}
	if j.Status == "running" || j.Status == "queued" {
		if t, perr := time.Parse(time.RFC3339, j.UpdatedAt); perr == nil && time.Since(t) > jobStaleRunning {
			_ = s.UpdateJobStatus(j.ID, "error", "服务重启导致任务中断，请重新发起生成")
			j.Status = "error"
			j.Error = "服务重启导致任务中断，请重新发起生成"
		}
	}
	if JobActive(j) {
		return j, nil
	}
	return nil, nil
}

// UpdateJobStatus 状态流转（error 时附错误摘要；cancelled 同）。
func (s *Store) UpdateJobStatus(id, status, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE ontochat_job SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, errMsg, now(), id)
	return err
}

// UpdateJobProgress 进度透出（生成-校验环轮次）。
func (s *Store) UpdateJobProgress(id, progress string) error {
	_, err := s.db.Exec(`UPDATE ontochat_job SET progress = ?, updated_at = ? WHERE id = ?`, progress, now(), id)
	return err
}

// SetJobResult 终局载荷（done：{reply,stage,round,warning,draft?}）。
func (s *Store) SetJobResult(id string, result []byte) error {
	_, err := s.db.Exec(
		`UPDATE ontochat_job SET result_json = ?, updated_at = ? WHERE id = ?`, result, now(), id)
	return err
}
