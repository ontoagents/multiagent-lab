-- REQ-565/M372 WF-④ run 级 DAG 快照+节点状态表（动态工作流编排批；109 号 §WF-④）——
-- workflow_run_snapshot：每 run 的 DAG schema 快照+变量表+节点状态持久化（冷恢复/rerun 数据源）；
-- 节点状态并入快照行 JSON（单表形态——节点状态与快照同生命周期，不单拆表；107 号 §三「节点状态表」
-- 素材以 JSON 列承载）。rerun 通道与恢复端点随 WF-⑥ 装配接线（Runner chat 接线同轮）。
-- 迁移编号注：109 号撰写时点 max=082 已过时——本件开工日 ls migrations/ max+1 动态领取=087。
CREATE TABLE IF NOT EXISTS workflow_run_snapshot (
    run_id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL DEFAULT '',
    spec_json TEXT NOT NULL,
    vars_json TEXT NOT NULL DEFAULT '{}',
    node_states_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','paused','completed','failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_wf_snapshot_conv ON workflow_run_snapshot(conversation_id, created_at);
