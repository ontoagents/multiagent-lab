-- 047_schema_migrations.sql REQ-381①/M177：SQLite 迁移版本表——显式版本管理替代
-- 「重放+duplicate column 报错容忍」幂等（rename/drop 类迁移防踩坑）。
-- 存量库升级路径：本表建立时既有迁移均已生效但未记录——repo.migrate 对「执行遇
-- duplicate column」的文件视为 legacy 已应用并补记（一次性收敛，此后 rename/drop 安全）。
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
