-- 041_reasoning_check.sql REQ-255②（60 号 H3 推理级检查档）：本体级 owlrl 推理一致性检查开关
-- （默认关=0，沿低侵入原则②；开启后 quality/check 导出 TTL 经 sidecar reason 跑 OWL 2 RL 闭包
-- 一致性检测，命中错误级入报告 findings 独立检查项 reasoning_owlrl）。
-- 沿 005 quality_strict 先例（IFNULL 兼容存量行）。
ALTER TABLE ontology ADD COLUMN reasoning_check INTEGER NOT NULL DEFAULT 0;
