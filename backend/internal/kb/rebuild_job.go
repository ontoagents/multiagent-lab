package kb

// KB-8/M288：库级重建三端点（kg/communities/wiki rebuild）异步化——
// kb_rebuild_job 落库（REQ-381 async_job 模式单机复用）+goroutine 执行+终态回写；
// 进度经 GET rebuild-jobs 列表轮询（REQ-55 状态机同门：running→success|failed）；
// 重启悬挂对账（MarkStaleRebuildJobs）沿 KB-21 分派恢复同门形态。

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// RebuildTimeout 单个库级重建任务上限（沿索引队列 10min 口径）。
const RebuildTimeout = 10 * time.Minute

// EnqueueRebuild 异步执行库级重建：job 行落库（running）→后台 goroutine 执行 fn→终态回写。
// 返回 job 行（HTTP 层 202 + job_id，前端轮询列表端点）。
func (s *Service) EnqueueRebuild(kbID, kind string, fn func(ctx context.Context) (map[string]any, error)) (*store.KBRebuildJob, error) {
	job, err := s.Store.CreateKBRebuildJob(kbID, kind)
	if err != nil {
		return nil, err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), RebuildTimeout)
		defer cancel()
		res, rerr := fn(ctx)
		if rerr != nil {
			_ = s.Store.FinishKBRebuildJob(job.ID, "failed", rerr.Error(), "")
			log.Printf("[kb] rebuild job %s (%s) failed: %v", job.ID, kind, rerr)
			return
		}
		b, _ := json.Marshal(res)
		_ = s.Store.FinishKBRebuildJob(job.ID, "success", "", string(b))
		log.Printf("[kb] rebuild job %s (%s) success", job.ID, kind)
	}()
	return job, nil
}
