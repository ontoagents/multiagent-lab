-- 069 本体消费命中增方案归因（REQ-482/M282 查询面组作用域化与消费归因批④）：
-- ontology_hit 增 profile_id（facade 工具结果 scope 块透出的方案 id）——组维度
-- 「一次回答命中哪些本体/方案」聚合数据源；存量行默认空串（不回填，如实口径）。
ALTER TABLE ontology_hit ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ontology_hit_profile ON ontology_hit(profile_id, created_at DESC);
