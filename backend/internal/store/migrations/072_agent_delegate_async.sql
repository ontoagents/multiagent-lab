-- 072 成员委派异步化开关（REQ-385/M181 C3 LongRun——agent_as_tool 子调用异步化汇流）：
-- Agent 能力开关 delegate_async（0=同步阻塞〔缺省，存量零变化〕| 1=被委派时转后台委派底盘
-- （DelegateManager delegate_background 同一编排层——协调者不阻塞，delegate_status/collect 跟进））。
ALTER TABLE agent ADD COLUMN delegate_async INTEGER NOT NULL DEFAULT 0;
