-- REQ-241 知识库 LLM Wiki 类型（M67，2026-10-02）：写时合成互链 Markdown 页面层。
-- kb.mode 扩第三值 'wiki'（TEXT 列无 CHECK 约束，直接可用；新库可选、存量库不动）。
-- wiki_page：LLM 摄取生成的页面（summary/entity/concept/topic/synthesis/index 五类），
-- sources_json=溯源 chunk_id 数组（页 → chunk 池下钻）；重建全量替换（沿 kg_community 语义）。
CREATE TABLE IF NOT EXISTS wiki_page (
  id TEXT PRIMARY KEY,
  kb_id TEXT NOT NULL,
  page_type TEXT NOT NULL,
  title TEXT NOT NULL,
  content_md TEXT NOT NULL DEFAULT '',
  sources_json TEXT NOT NULL DEFAULT '[]',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_wiki_page_kb ON wiki_page(kb_id);
-- 文档级摄取指纹（内容未变更的文档重建时跳过 summary 生成，省 LLM 调用）
ALTER TABLE knowledge_doc ADD COLUMN wiki_hash TEXT NOT NULL DEFAULT '';
