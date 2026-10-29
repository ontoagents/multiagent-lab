-- 074 出题·批改·掌握度最小闭环（REQ-524/M331〔撞号让位注：原编 073 与并行 KB-20 embed_dim 撞号，顺延 074——交付时点 2026-10-30〕——Q-23 衍生③ 转正领号交付，docs/03 §2.71）：
-- quiz_item 出题记录（本体概念子图出题——概念快照+难度+答案依据）+ mastery_level 概念级掌握度
--（每次批改按题目关联概念聚合更新：correct 计数与加权分数——概念子图掌握度的最小承载，
-- 跨会话记忆（衍生②）后续以此表为数据源）。
CREATE TABLE IF NOT EXISTS quiz_item (
    id            TEXT PRIMARY KEY,
    ontology_id   TEXT NOT NULL,
    concept_names TEXT NOT NULL,            -- JSON 数组：本题关联概念（批改后逐个更新掌握度）
    question      TEXT NOT NULL,
    answer        TEXT NOT NULL,            -- 参考答案+依据（出题时一并生成，批改对照）
    difficulty    TEXT NOT NULL DEFAULT 'basic', -- basic | applied
    created_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_quiz_item_ontology ON quiz_item (ontology_id, created_at);
CREATE TABLE IF NOT EXISTS mastery_level (
    id            TEXT PRIMARY KEY,
    ontology_id   TEXT NOT NULL,
    concept_name  TEXT NOT NULL,
    asked_count   INTEGER NOT NULL DEFAULT 0, -- 累计出题数
    correct_count INTEGER NOT NULL DEFAULT 0, -- 累计答对数
    score         REAL NOT NULL DEFAULT 0,    -- 最近加权分（0~1，批改 score 均值衰减：0.7 旧+0.3 新）
    updated_at    TEXT NOT NULL,
    UNIQUE (ontology_id, concept_name)
);
