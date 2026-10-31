-- REQ-550/M357 项目归属与继承 M2（Q-24 分层批③；详设 platform-knowledge/02_智能体/106 号 §二/§五 M2）
-- project.workspace_id：NULL=未归属（存量项目路径逐字节不变——验收口径）。
-- 归属关系列 REFERENCES ON DELETE SET NULL（106 号 mustFix-4 终值）：删工作区项目脱钩、
-- 执行容器字段（目录/Git/团队/文件）零变化——与 conversation.workspace_id 同策略。
ALTER TABLE project ADD COLUMN workspace_id TEXT NULL REFERENCES workspace(id) ON DELETE SET NULL;
