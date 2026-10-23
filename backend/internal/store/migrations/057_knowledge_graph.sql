-- 057 知识体系 L2 图谱层（REQ-372 P2）：知识卡 → 图谱投影（卡为事实源单源，KG 为投影——
-- 每次摄取全量重建）。节点=卡（三型 kind + domain + 主题），边=prereq（相对路径解析为卡 id）。
CREATE TABLE IF NOT EXISTS knowledge_node (
    id      TEXT PRIMARY KEY, -- 仓库相对路径（如 platform-knowledge/09_知识体系/01_本体线/cards/01_x.md）
    kind    TEXT NOT NULL DEFAULT '',
    domain  TEXT NOT NULL DEFAULT '',
    topic   TEXT NOT NULL DEFAULT '',
    desc    TEXT NOT NULL DEFAULT '',
    sources TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS knowledge_edge (
    src TEXT NOT NULL,
    dst TEXT NOT NULL, -- prereq 目标卡 id
    kind TEXT NOT NULL DEFAULT 'prereq',
    PRIMARY KEY (src, dst, kind)
);
CREATE INDEX IF NOT EXISTS idx_kg_edge_dst ON knowledge_edge(dst);
