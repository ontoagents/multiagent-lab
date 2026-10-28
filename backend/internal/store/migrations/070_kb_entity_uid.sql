-- 070 KB-16/M286 实体稳定 ID 与 chunk-entity 倒排表（KG 存储地基，KG 工程化深化批①）：
-- uid = kb_id || '::' || name 确定性派生（Go 侧 store.KGEntityUID 同式——重建不再换 id；
-- name 保留为展示名+业务键兼容层，无外键约束现状不变 DB 层语义）。
-- 存量按 name 归一回填（关系/claim 的 uid 列经实体表 join 回填）。
ALTER TABLE kg_entity ADD COLUMN uid TEXT NOT NULL DEFAULT '';
UPDATE kg_entity SET uid = kb_id || '::' || name WHERE uid = '';
-- 存量同名多行归一（历史跨 doc 同名重复行：保留最小 rowid 一行，name 引用仍成立）
DELETE FROM kg_entity WHERE uid != '' AND rowid NOT IN (SELECT MIN(rowid) FROM kg_entity GROUP BY uid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_entity_uid ON kg_entity(uid);
ALTER TABLE kg_relationship ADD COLUMN source_uid TEXT NOT NULL DEFAULT '';
ALTER TABLE kg_relationship ADD COLUMN target_uid TEXT NOT NULL DEFAULT '';
UPDATE kg_relationship SET source_uid = (
    SELECT e.kb_id || '::' || e.name FROM kg_entity e
    WHERE e.kb_id = kg_relationship.kb_id AND e.name = kg_relationship.source
) WHERE source_uid = '';
UPDATE kg_relationship SET target_uid = (
    SELECT e.kb_id || '::' || e.name FROM kg_entity e
    WHERE e.kb_id = kg_relationship.kb_id AND e.name = kg_relationship.target
) WHERE target_uid = '';
ALTER TABLE kg_claim ADD COLUMN subject_uid TEXT NOT NULL DEFAULT '';
UPDATE kg_claim SET subject_uid = (
    SELECT e.kb_id || '::' || e.name FROM kg_entity e
    WHERE e.kb_id = kg_claim.kb_id AND e.name = kg_claim.subject
) WHERE subject_uid = '';
-- chunk↔实体倒排表（抽取期落库；检索期替代「KGSubjectsByChunks claim 反查」唯一路径）
CREATE TABLE IF NOT EXISTS kg_chunk_entity (
    kb_id TEXT NOT NULL,
    doc_id TEXT NOT NULL DEFAULT '',
    chunk_id TEXT NOT NULL,
    entity_uid TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (chunk_id, entity_uid)
);
CREATE INDEX IF NOT EXISTS idx_kg_chunk_entity_kb ON kg_chunk_entity(kb_id, chunk_id);
-- 存量回填：approved claims 反查基线（倒排与 claim 反查两路一致的前提）
INSERT OR IGNORE INTO kg_chunk_entity (kb_id, doc_id, chunk_id, entity_uid, name, created_at)
SELECT kb_id, doc_id, chunk_id, subject_uid, subject, created_at FROM kg_claim
WHERE status = 'approved' AND chunk_id != '' AND subject_uid != '';
