-- 075 claim 结构化溯源 span（KB-17/M289——claim 溯源健壮化）：
-- kg_claim 增 span_start/span_end（chunk 内容内 rune 起止偏移，-1=未定位）——
-- 替代「首 24 rune Contains、未命中即丢溯源」的弱定位：抽取后对齐算法落
-- 原文精确/归一化窗口两级定位，溯源失败率可量化（span_start=-1 计数）。
ALTER TABLE kg_claim ADD COLUMN span_start INTEGER NOT NULL DEFAULT -1;
ALTER TABLE kg_claim ADD COLUMN span_end INTEGER NOT NULL DEFAULT -1;
