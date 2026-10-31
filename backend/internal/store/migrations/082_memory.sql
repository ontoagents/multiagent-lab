-- REQ-551/M358 工作区记忆作用域 M3（Q-24 分层批④收官；详设 platform-knowledge/02_智能体/106 号 §二/§五 M3；
-- Q-23 衍生② 跨会话个人记忆转正载体）。可审计口径：黑箱记忆不做——每条记忆带来源会话/消息行级溯源，
-- 全量经候选确认（无自动入记忆）。
-- workspace_id 作用域键 REFERENCES ON DELETE CASCADE（106 号「无 FK 或 SET NULL 待定」终值=CASCADE：
-- 记忆无回落宿主——对照 conversation SET NULL 有智能体页回落、笔记本有个人资产语义，记忆离工作区即无义）。
CREATE TABLE IF NOT EXISTS memory (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  source_conversation_id TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate','accepted','rejected')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memory_workspace ON memory(workspace_id, status);
-- 抽取游标（沿伴生 companion_cursor 增量范式：只抽 last_message_id 之后的增量，防重复抽取风暴）
CREATE TABLE IF NOT EXISTS memory_cursor (
  conversation_id TEXT PRIMARY KEY,
  last_message_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
