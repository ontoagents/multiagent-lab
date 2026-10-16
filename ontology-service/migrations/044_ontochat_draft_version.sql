-- 044 OntoChat 草稿版本链（REQ-338③，80 号建议项 X/A3）：draft/refine 每次生成落全量快照——
-- 会话恢复从 {round,draft} 布尔升为版本链：中断续作/跨设备续作/审计回放三诉求一并满足。
-- 文件材料引用清单随快照入库（Context.Hints 的「[文件导入 name]」前缀提取，溯源材料→草稿）。
CREATE TABLE IF NOT EXISTS ontochat_draft_version (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    version    INTEGER NOT NULL,
    spec_json  TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT 'draft', -- draft 首稿 | refine 修正稿 | confirm 增量确认调整后的入库形态
    round      INTEGER NOT NULL DEFAULT 0,
    materials  TEXT NOT NULL DEFAULT '[]',    -- ["a.md","b.txt"] 文件材料引用
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(session_id, version)
);
CREATE INDEX IF NOT EXISTS idx_ontochat_draftver_session ON ontochat_draft_version(session_id, version);
