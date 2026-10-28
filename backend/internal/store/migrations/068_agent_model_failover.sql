-- REQ-452/M248：模型调用故障转移——agent 级备援模型连接清单（JSON 字符串数组）。
-- 主连接（model_conn_id，空=全局默认）持续失败时按序切换；空数组=不启用故障转移（存量零回归）。
ALTER TABLE agent ADD COLUMN model_failover_conn_ids TEXT NOT NULL DEFAULT '[]';
