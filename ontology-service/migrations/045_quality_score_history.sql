-- REQ-331/M131：资产级质量评分历史（评分随版本追加记录——版本×综合分趋势线+草稿→入库→当前三点对照取数源）。
-- source: draft=入库前草稿评分（OntoChat 入库动线）/ ingest=入库评分（saveSpec/mergeApply/ontoChatSave）/ manual=质量卡手动生成。
CREATE TABLE IF NOT EXISTS ontology_quality_score (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ontology_id TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'manual',
  overall REAL NOT NULL,
  completeness REAL NOT NULL,
  consistency REAL NOT NULL,
  maintainability REAL NOT NULL,
  error_count INTEGER NOT NULL DEFAULT 0,
  warning_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oqs_ontology ON ontology_quality_score(ontology_id, created_at);
