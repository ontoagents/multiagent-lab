-- REQ-310/M114：last_error 发生时间——错误粘性持久化（仅成功启动收敛）之外给「何时失败」。
ALTER TABLE runtime_profile ADD COLUMN last_error_at TEXT;
