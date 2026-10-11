-- REQ-291/M95：会话级模型覆盖（68 号《智能体与会话配置分层分析》P1 影子层转正）
-- ''=跟随智能体默认（与 tool_approval「''=跟随 Agent 级」REQ-135② 同款语义）；非空=本会话模型连接覆盖。
-- 合并序：请求级（RunInput.model_conn_id）> 会话级 > 智能体默认 > 全局默认。
ALTER TABLE conversation ADD COLUMN model_conn_id TEXT NOT NULL DEFAULT '';
