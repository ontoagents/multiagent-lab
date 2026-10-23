package repo

// REQ-381②/M177：异步任务统一落库——draftJob 进程内 map（重启丢任务）写穿 async_job 表
// （迁移 048，对齐 ontochat_job 042 形态）。轮询侧 map 未命中回读 DB（重启后 done/failed
// 结果仍可查）；启动对账将悬挂 running 标 error（诚实报错重新发起）。

import (
	"database/sql"
	"encoding/json"
	"time"
)

// AsyncJobRow async_job 表行（与 rest.draftJob 字段同构——result_json 承载整个 job JSON）。
type AsyncJobRow struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Progress   string `json:"progress"`
	ResultJSON string `json:"result_json,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// UpsertAsyncJob 写穿一行（幂等覆盖）。
func (s *Store) UpsertAsyncJob(r *AsyncJobRow) error {
	if r.UpdatedAt == "" {
		r.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if r.CreatedAt == "" {
		r.CreatedAt = r.UpdatedAt
	}
	_, err := s.db.Exec(`INSERT INTO async_job (id, kind, status, progress, result_json, error, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET kind=excluded.kind, status=excluded.status, progress=excluded.progress,
		result_json=excluded.result_json, error=excluded.error, updated_at=excluded.updated_at`,
		r.ID, r.Kind, r.Status, r.Progress, r.ResultJSON, r.Error, r.CreatedAt, r.UpdatedAt)
	return err
}

// GetAsyncJob 按 ID 读行（无行返回 nil）。
func (s *Store) GetAsyncJob(id string) (*AsyncJobRow, error) {
	row := s.db.QueryRow(`SELECT id, kind, status, progress, COALESCE(result_json,''), error, created_at, updated_at FROM async_job WHERE id=?`, id)
	var r AsyncJobRow
	if err := row.Scan(&r.ID, &r.Kind, &r.Status, &r.Progress, &r.ResultJSON, &r.Error, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// MarkOrphanAsyncJobs 启动对账：悬挂 running → error（进程重启 goroutine 消失，诚实报错重新发起）。
func (s *Store) MarkOrphanAsyncJobs() (int64, error) {
	res, err := s.db.Exec(`UPDATE async_job SET status='error', error='backend 重启中断——请重新发起任务', updated_at=? WHERE status IN ('running','queued')`,
		time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// marshalAsyncJob 任意 job 结构 → JSON 串（写穿载荷）。
func MarshalAsyncJob(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
