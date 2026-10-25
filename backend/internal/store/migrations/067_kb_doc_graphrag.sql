-- 067_kb_doc_graphrag.sql REQ-440/M236：GraphragInfo 落库回读——KG 抽取结果（ok/method/degraded/规模）
-- 由「仅导入响应当次回显」改为持久可见（KG 徽标刷新不丢；REQ-55 KG 失败原因同样落库可见）。
ALTER TABLE knowledge_doc ADD COLUMN graphrag_json TEXT NOT NULL DEFAULT '';
