-- 046 OntoChat 会话归属审计两列（REQ-354/P2·G4，92 号 §5.6）：
--   source = page | agent_chat（会话来源：页面流程缺省 page，智能体对话经 ontochat_start 建会话落 agent_chat）
--   agent_id = 发起会话的智能体 id（agent_chat 时非空）——审计「哪个智能体产了哪些本体」。
-- 纯增列：旧行按缺省归 page（诚实缺省——历史会话确由页面创建）。
ALTER TABLE ontochat_session ADD COLUMN source TEXT NOT NULL DEFAULT 'page';
ALTER TABLE ontochat_session ADD COLUMN agent_id TEXT NOT NULL DEFAULT '';
