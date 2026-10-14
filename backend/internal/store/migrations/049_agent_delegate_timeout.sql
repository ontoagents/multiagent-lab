-- REQ-319/M121（78 号《多智能体与子智能体执行支持与调度编排分析》P1 批 A2）：成员委派超时——
-- 本 agent 作为项目成员被委派调用时的单次执行超时秒数（0=不限，存量零变化）；超时错误经委派包装器
-- 降级回喂协调者（chat/delegatetool.go），不再无限占用协调者迭代（78 号 G2）。
ALTER TABLE agent ADD COLUMN delegate_timeout_seconds INTEGER NOT NULL DEFAULT 0;
