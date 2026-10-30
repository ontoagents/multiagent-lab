-- 078 本体消费 cited 归因列〔撞号让位注：原编 076→077 连续与并行 KB-18/KB-8①② 撞号（076_kg_community_vec/077_kb_rebuild_job），终顺延 078——三方撞号第 4~5 例，见 102 号冲突分析〕（REQ-530/M337——P13「白挂载不可观测」治理）：
-- ontology_hit 增 cited（0=答案未引用该 target｜1=containment 命中）——归因自「调用了工具」
-- 深化到「答案是否用了该结果」（runner.go 注释自认最小消费信号的深化面）。
ALTER TABLE ontology_hit ADD COLUMN cited INTEGER NOT NULL DEFAULT 0;
