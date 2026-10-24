-- 064_agent_compact_policy.sql REQ-396/M192：上下文压缩策略可选——''=默认（复用本 agent
-- 模型连接八段式摘要）| off=关闭压缩仅剪枝 | <模型连接 id>=用指定（低成本）连接生成摘要。
ALTER TABLE agent ADD COLUMN compact_policy TEXT NOT NULL DEFAULT '';
