-- 063_conversation_context_mode.sql REQ-393/M189：上下文预算三档会话级覆盖（54 号开放 4）——
-- 沿 REQ-291 会话级模型覆盖先例：''=跟随 Agent 级，compact/standard/full=覆盖；合并序 会话>Agent>全局。
ALTER TABLE conversation ADD COLUMN context_mode TEXT NOT NULL DEFAULT '';
