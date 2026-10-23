-- 055 本体消费贡献度量（REQ-224 H10/60 号搭车件）：onto_* facade 工具调用成功命中逐次落行，
-- 支撑「本体命中次数 / 被引用概念 TopN」聚合（companion.hit 先例推广，入质量卡与消费审计栏）。
CREATE TABLE IF NOT EXISTS ontology_hit (
    id TEXT PRIMARY KEY,
    ontology_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    tool TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    ok INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ontology_hit_oid ON ontology_hit(ontology_id, created_at DESC);
