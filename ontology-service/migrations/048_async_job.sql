-- 048_async_job.sql REQ-381②/M177：异步任务统一落库——ai-draft/ai-cq-extract/cqrag/
-- evolution diagnose/cqbench run 等进程内 draftJob map（重启丢任务）写穿落库；
-- 对齐 ontochat_job（042）形态。重启后 running 行由启动对账标 error（诚实报错重新发起）。
CREATE TABLE IF NOT EXISTS async_job (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'running',
    progress    TEXT NOT NULL DEFAULT '',
    result_json TEXT,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_async_job_status ON async_job(status, created_at DESC);
