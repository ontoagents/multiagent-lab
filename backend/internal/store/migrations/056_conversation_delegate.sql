-- 056 可继续子 Agent 后台委派（REQ-208/M42 余项）：协调者对成员的后台委派登记——
-- 子会话（scope=agent 一等会话）承载执行与历史，本表只记委派状态机与归属关系。
-- 状态机：running → completed | failed | stopped；steered 为运行中标记（插话待续跑消费）。
CREATE TABLE IF NOT EXISTS conversation_delegate (
    id TEXT PRIMARY KEY,
    parent_conversation_id TEXT NOT NULL,
    child_conversation_id TEXT NOT NULL,
    member_agent_id TEXT NOT NULL,
    member_name TEXT NOT NULL DEFAULT '',
    task TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    run_id TEXT NOT NULL DEFAULT '',
    steer_note TEXT NOT NULL DEFAULT '',
    result_digest TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_conv_delegate_parent ON conversation_delegate(parent_conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_conv_delegate_child ON conversation_delegate(child_conversation_id);
