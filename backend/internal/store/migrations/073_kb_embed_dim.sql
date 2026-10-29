-- 073 库级 embedding 维度登记（KB-20/M292 维度漂移检测与告警——P1 bug 级根治）：
-- knowledge_base 增 embed_dim（0=未登记）——首库索引时登记当前 embedding 维度，
-- 写入/检索双端校验：维度不匹配拒绝写/返回可懂报错+重建指引（SQLite 路径原
-- len(v)!=len(vec) 静默跳过丢召回的根治，KB-O3 库级维度锁定工程侧落地）。
ALTER TABLE knowledge_base ADD COLUMN embed_dim INTEGER NOT NULL DEFAULT 0;
