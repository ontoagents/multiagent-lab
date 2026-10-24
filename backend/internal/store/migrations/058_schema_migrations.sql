-- 058_schema_migrations.sql REQ-391/M187：迁移版本表统一（复用 REQ-381 已交付模式——
-- schema_migrations(version TEXT PRIMARY KEY)+已记录跳过+duplicate column 视为 legacy 补记）。
-- 旧 schema_migration（单数，name 主键）记录全量转入新表（存量库零重放）；
-- 编号重复治理（018/026/027 各重复两次=并行无锁实证）：重命名文件的新版本号随旧记录映射转入。
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
-- 旧单数表（fresh 库补空壳承载 SELECT；存量库已有数据）
CREATE TABLE IF NOT EXISTS schema_migration (
    name TEXT PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_migrations (version, applied_at)
    SELECT name, CURRENT_TIMESTAMP FROM schema_migration;
-- 编号重复治理映射（重命名三件 018→018b/026→026b/027→027b——排序紧随原号保依赖序；存量库旧记录在位即补记不重放）
INSERT OR IGNORE INTO schema_migrations (version, applied_at)
    SELECT '018b_agent_mcp_serve.sql', CURRENT_TIMESTAMP FROM schema_migration WHERE name = '018_agent_mcp_serve.sql';
INSERT OR IGNORE INTO schema_migrations (version, applied_at)
    SELECT '026b_companion_alignment.sql', CURRENT_TIMESTAMP FROM schema_migration WHERE name = '026_companion_alignment.sql';
INSERT OR IGNORE INTO schema_migrations (version, applied_at)
    SELECT '027b_kb_parent_capabilities.sql', CURRENT_TIMESTAMP FROM schema_migration WHERE name = '027_kb_parent_capabilities.sql';
