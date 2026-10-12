-- REQ-288/M93 伴生社区摘要：伴生图社区分区（对齐 Graphiti community summaries）。
-- Louvain 检测结果 + LLM 摘要缓存（method: llm|skeleton|pending——pending 由对话兜底/
-- 显式按需生成后回填，成本口径沿 KB-O4「按需生成+缓存，不做重建时全量预生成」）。
-- 成员清单 JSON 随行留档：成员不变则重建时保留既有摘要（不重复烧 LLM）。
CREATE TABLE IF NOT EXISTS companion_community (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ontology_id TEXT NOT NULL,
    label TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT 'pending',
    members_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(ontology_id, label)
);
CREATE INDEX IF NOT EXISTS idx_companion_community_ont ON companion_community(ontology_id);
