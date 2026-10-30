-- 076 本体消费 cited 归因列（REQ-530/M337——P13「白挂载不可观测」治理）：
-- ontology_hit 增 cited（0=答案未引用该 target｜1=containment 命中）——归因自「调用了工具」
-- 深化到「答案是否用了该结果」（runner.go 注释自认最小消费信号的深化面）。
ALTER TABLE ontology_hit ADD COLUMN cited INTEGER NOT NULL DEFAULT 0;
