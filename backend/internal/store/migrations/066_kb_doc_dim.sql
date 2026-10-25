-- 066_kb_doc_dim.sql REQ-439/M235：doc 级向量维度回写——12 §3.3 既有口径补实现
--（doc 表 dim 列；REQ-54 文档管理字段）。wiki 库 chunksOnly 零 embedding，dim=0 如实零维。
ALTER TABLE knowledge_doc ADD COLUMN dim INTEGER NOT NULL DEFAULT 0;
