-- 042_ontochat_job.sql REQ-271/M80：OntoChat 生成轮异步 job 持久化（定案口径=与 session 同库 SQLite）。
-- 状态机 queued→running→done|error|cancelled；result_json 存终局载荷（reply/stage/round/warning/draft），
-- progress 存生成-校验环轮次进度（前端轮询展示）。running 超 15min 无更新由 ActiveJobBySession 自愈收敛
-- （进程重启导致的悬挂 running，沿启动对账先例）。
CREATE TABLE IF NOT EXISTS ontochat_job (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'queued',
    error      TEXT NOT NULL DEFAULT '',
    progress   TEXT NOT NULL DEFAULT '',
    result_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ontochat_job_session ON ontochat_job(session_id, status);
