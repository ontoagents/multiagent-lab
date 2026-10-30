-- 077 库级重建任务表（KB-8②/M288——整库重建三端点〔kg/communities/wiki rebuild〕异步化）：
-- 单机基线内的库级任务落库（REQ-381 async_job 模式复用）——重启悬挂 running 标 failed
-- （启动对账在 store.Open 外由 kb.NewService 调 MarkStaleRebuildJobs）；进度经列表端点轮询。
CREATE TABLE IF NOT EXISTS kb_rebuild_job (
  id         TEXT PRIMARY KEY,
  kb_id      TEXT NOT NULL,
  kind       TEXT NOT NULL,            -- kg | communities | wiki
  status     TEXT NOT NULL DEFAULT 'running', -- running | success | failed
  error      TEXT NOT NULL DEFAULT '',
  result_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_kb_rebuild_job_kb ON kb_rebuild_job(kb_id, created_at DESC);
