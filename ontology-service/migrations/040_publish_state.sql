-- 040_publish_state.sql REQ-239/M65（本体版本发布状态机，57 号 V1 治 A-2 线性快照堆无发布态）
-- status：draft | published（保存默认 draft，显式发布动作置 published）；
-- version_name：发布命名版本（如 "v3-k8s-baseline"；空=默认 v{N}），发布态徽标与运行向导透出用。
-- 沿 005 quality_strict 先例（IFNULL 兼容存量行；重放式迁移 ALTER 无法 IF NOT EXISTS，重复启动幂等容忍）。
ALTER TABLE ontology ADD COLUMN status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE ontology ADD COLUMN version_name TEXT NOT NULL DEFAULT '';
