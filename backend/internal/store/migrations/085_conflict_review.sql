-- REQ-553/M360 方案分歧结构化识别与人工裁决（结构化协作批 P0；107 号 §四）——
-- conflict_review 提案审计表（识别=平台确定性规则档，裁决=必经人工，执行=复用既有挂起恢复链）：
-- kind=review 点（submit_review 工具体内评估，命中即挂起 state=suspended）| event 点（运行收束后
-- 事件规则命中，无活运行可挂起 → state=pending，裁决后回注消息发起新运行）；resolution 落
-- adopt/third_text/rationale/decision_source=manual 审计对齐 approval.granted。
-- 挂起载体仍是 conversation.interrupt_state 单槽（kind='conflict'+conflict_id 指针），本表存全量审计。
-- 迁移编号注：107 号 §4.4 撰写时点目录最新 078——本件开工日 ls migrations/ max+1 动态领取=085。
CREATE TABLE IF NOT EXISTS conflict_review (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    run_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'review' CHECK (kind IN ('review','event')),
    rule_id TEXT NOT NULL DEFAULT '',
    parties_json TEXT NOT NULL DEFAULT '[]',
    verdicts_json TEXT NOT NULL DEFAULT '[]',
    state TEXT NOT NULL DEFAULT 'suspended' CHECK (state IN ('suspended','pending','resolved','dismissed')),
    resolution_json TEXT NOT NULL DEFAULT '',
    checkpoint_ref TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_conflict_review_conv ON conflict_review(conversation_id, created_at);
-- 项目级冲突规则档（107 号 §4.3②）：''=默认档（review 点 position=disagree → 挂起裁决）；
-- JSON 自定义规则条目两类触发（review 点/event 点），v1 无编辑 UI（直写列/后续轮补面）。
ALTER TABLE project ADD COLUMN conflict_policy TEXT NOT NULL DEFAULT '';
