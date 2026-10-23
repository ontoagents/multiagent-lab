-- 011_schema_migrations.sql REQ-381①/M177：SQLite 迁移版本表（同 ontology-service 047 口径）。
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
