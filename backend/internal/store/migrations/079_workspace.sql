-- REQ-548/M355 工作区领域容器 M1a（Q-24 分层批①；详设 platform-knowledge/02_智能体/106 号 §二/§五）
-- 外键策略（106 号 mustFix-4 终值）：
--   config 引用列（default_kb_id/default_profile_id/default_agent_id）裸 TEXT 无 FK——house style
--   同 001_init.sql conversation.kb_id 无 REFERENCES；DSN _pragma=foreign_keys(1) 下加 FK 会让
--   删除被设为默认的库直接 constraint failed；失效兜底=ResolveConvDefaults 求值校验（KB 本地校验，
--   方案走既有 ontology.unavailable 降级路径）。
--   归属关系列 conversation.workspace_id 保留 REFERENCES ON DELETE SET NULL（对照 001 project_id
--   先例）——删工作区对话回落智能体页、消息资产保留（Q-24 拍板采建议默认）。
CREATE TABLE IF NOT EXISTS workspace (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_kb_id TEXT NOT NULL DEFAULT '',
  default_profile_id TEXT NOT NULL DEFAULT '',
  default_agent_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
ALTER TABLE conversation ADD COLUMN workspace_id TEXT NULL REFERENCES workspace(id) ON DELETE SET NULL;
