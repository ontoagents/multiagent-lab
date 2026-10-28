-- REQ-262/M256：跨本体引用登记单源表（68 号 §3.2 数据模型采纳；D-O27 三形态收敛单源）。
-- external_ref 登记（ref_kind=import/alignment/reuse）+ imports 装载侧解析登记 + 导入外引 IRI 登记
-- 三形态统一落本表（ref_kind/predicate 词表区分）。名字引用不 IRI 化（from_concept 裸名）；
-- to_concept 支持外部 IRI（to_onto_id 留空=外部词表/未导入资产，悬空引用 qualitygate 只提醒不阻断）。
CREATE TABLE IF NOT EXISTS external_refs (
    ref_id        TEXT PRIMARY KEY,
    from_onto_id  TEXT NOT NULL,
    from_concept  TEXT NOT NULL,
    to_onto_id    TEXT NOT NULL DEFAULT '',
    to_concept    TEXT NOT NULL,
    ref_kind      TEXT NOT NULL CHECK (ref_kind IN ('import','alignment','reuse')),
    predicate     TEXT NOT NULL DEFAULT '',
    confidence    REAL NOT NULL DEFAULT 1.0,
    justification TEXT NOT NULL DEFAULT 'expert_review',
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    note          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ext_refs_from ON external_refs(from_onto_id);
CREATE INDEX IF NOT EXISTS idx_ext_refs_to ON external_refs(to_onto_id);
