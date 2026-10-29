-- 071 KB-4②③/M287 实体向量缓存（实体「名称+描述」embedding，实体向量召回双入口的存储面）：
-- PK=entity_uid（KB-16/M286 稳定 id 红利：uid=kb_id::name 跨重建稳定，向量缓存随实体行生命周期
-- 走——ReplaceKGForDoc/MergeKGEntities/deleteKG 的实体删除以孤儿清理同式回收，无悬挂缓存）。
-- fingerprint=name+desc 内容指纹（未变跳过重算）；dim 逐行登记（KB-20 维度漂移检测的承载列）。
-- 缓存语义：检索期全库读入内存余弦（教学规模实体数百~千级，35 号 KB-4 草案口径
-- 「向量存 SQLite BLOB 余弦——沿用 vector.go 现成模式」）。
CREATE TABLE IF NOT EXISTS kg_entity_vec (
    kb_id TEXT NOT NULL,
    entity_uid TEXT NOT NULL,
    dim INTEGER NOT NULL,
    vec BLOB NOT NULL,
    fingerprint TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (entity_uid)
);
CREATE INDEX IF NOT EXISTS idx_kg_entity_vec_kb ON kg_entity_vec(kb_id);
