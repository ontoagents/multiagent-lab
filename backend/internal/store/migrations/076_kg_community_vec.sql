-- 076 社区摘要向量缓存（KB-18/M290——全局问答社区检索 embedding 臂）：
-- kg_community_vec（KB 内社区摘要向量化缓存；检索期融合词面 2-gram 臂与余弦相似臂，
-- 词面臂保留为回退与零 embedding 降级路径——KB-18 口径）。label 对齐 kg_community.label。
CREATE TABLE IF NOT EXISTS kg_community_vec (
  kb_id      TEXT    NOT NULL,
  label      TEXT    NOT NULL,
  vec        BLOB    NOT NULL,
  dim        INTEGER NOT NULL,
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (kb_id, label)
);
