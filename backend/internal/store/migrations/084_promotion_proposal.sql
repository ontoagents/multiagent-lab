-- REQ-546/M353 learning proposal 暂存区（决策结构化借鉴批③；105 号 P1-5）——
-- 伴生晋升「提案→暂存→人批→入库」显式化：promote 改产 pending 提案（原直接 PUT 退役），
-- approve 裁决执行入库（重取 digest 取交集防伴生图漂移），reject 留痕（decided_note
-- 即 reject route 素材，衔接 REQ-544 decisions[]）；形态沿 companion_candidate
-- （pending|approved|rejected + decided_at 单向审计链）。
-- 迁移编号注：REQ-546 行内原引 080 已被 REQ-495 笔记本批占用（074/075 让位先例）——
-- 本件开工日 ls migrations/ max+1 动态领取=084。
CREATE TABLE IF NOT EXISTS promotion_proposal (
    id TEXT PRIMARY KEY,
    source_ontology_id TEXT NOT NULL,
    target_ontology_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    entity_count INTEGER NOT NULL DEFAULT 0,
    relation_count INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    decided_note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_promotion_proposal_status ON promotion_proposal(status, created_at);
CREATE INDEX IF NOT EXISTS idx_promotion_proposal_source ON promotion_proposal(source_ontology_id);
