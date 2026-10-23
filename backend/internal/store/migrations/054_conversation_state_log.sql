-- 054 会话状态变更留痕（REQ-224①/M52 余项）：interrupt_state / context_state / todo_json /
-- schedule 四散字段写入时统一落一行 prev→next 快照（值未变化的写入不落），历史可追溯。
CREATE TABLE IF NOT EXISTS conversation_state_log (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    field TEXT NOT NULL,
    prev TEXT,
    next TEXT,
    reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_conv_state_log_conv ON conversation_state_log(conversation_id, created_at DESC);
