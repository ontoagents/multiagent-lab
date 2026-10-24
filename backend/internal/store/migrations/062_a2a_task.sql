-- 062_a2a_task.sql REQ-387/M183：A2A 任务落库——进程内存表退役为缓存（重启 TaskNotFound
-- 伤对外契约治理）。历史 history/artifact 随 JSON 载荷落库；重启后 running 行由启动对账
-- 标 failed（goroutine 消失诚实报错，外部客户端重新委派）。
CREATE TABLE IF NOT EXISTS a2a_task (
    id          TEXT PRIMARY KEY,
    context_id  TEXT NOT NULL DEFAULT '',
    agent_id    TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'working',
    timestamp   TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL DEFAULT '{}', -- artifact_text/error_text/history JSON
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_a2a_task_ctx ON a2a_task(context_id);
