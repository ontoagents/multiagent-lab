-- REQ-287/M92 伴生检索纵深：伴生实体 FTS5 兜底臂（BM25）投影表。
-- 伴生实体本体在 Oxigraph named graph（ont-{id}），SQLite 侧无投影——本表为检索专用的
-- label+别名+定义文本投影（trigram 分词，BM25 排序），检索时惰性回填、图写入失效后
-- 下次检索重建（与 invalidateLabelCache 同一失效序列，缓存冷启动口径）。
CREATE VIRTUAL TABLE IF NOT EXISTS companion_fts USING fts5(
    content,
    label UNINDEXED,
    ontology_id UNINDEXED,
    tokenize='trigram'
);
