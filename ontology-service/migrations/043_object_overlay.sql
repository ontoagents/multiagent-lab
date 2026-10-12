-- REQ-292/M96 本体编辑三层（base + overlay + audit，67 号 B4/B5）：
-- 机器产出（merge/灌装/LLM 重生成）写 spec（base 语义），人工编辑记 overlay 行，读取/落库即合并后的 effective 文档。
-- overlay 行按确定性身份 logical_id 定位（digest，spec.LogicalID），base_json 保留最近一次机器产出值供 revert。
-- action: edit（改机器对象）| add（人工新增）| delete（人工删除=墓碑，防机器重建复活）。
CREATE TABLE IF NOT EXISTS ontology_object_overlay (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ontology_id TEXT NOT NULL,
    logical_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT 'edit',
    base_json TEXT NOT NULL DEFAULT '{}',
    overlay_json TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 1,
    idem_key TEXT NOT NULL DEFAULT '',
    idem_hash TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    actor TEXT NOT NULL DEFAULT '',
    UNIQUE(ontology_id, logical_id)
);
