-- REQ-572/M380 出题·批改·掌握度闭环（Q-23 衍生③ 转正，01 §3.24）——
-- ①quiz_item 题型扩展：question_type（short_answer 简答=主观 LLM 批改 | choice 单选 | judge 判断
--   =客观确定性判定）+ options（客观题选项 JSON 数组，作答前可见；正确答案仍存 answer 列不前显）。
-- ②quiz_attempt 批改落档（可审计——沿本体审计叙事）：每次提交批改一条（题目/作答/裁决/分数/
--   点评/关联概念快照），掌握度影响可回溯。
-- 迁移编号注：本件开工日 ls migrations/ max+1=088（087_workflow_snapshot 在案）。
ALTER TABLE quiz_item ADD COLUMN question_type TEXT NOT NULL DEFAULT 'short_answer';
ALTER TABLE quiz_item ADD COLUMN options TEXT NOT NULL DEFAULT '[]';
CREATE TABLE quiz_attempt (
	id TEXT PRIMARY KEY,
	ontology_id TEXT NOT NULL,
	quiz_item_id TEXT NOT NULL,
	question_type TEXT NOT NULL DEFAULT 'short_answer',
	answer TEXT NOT NULL,
	score REAL NOT NULL,
	verdict TEXT NOT NULL,
	feedback TEXT NOT NULL DEFAULT '',
	concept_names TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL
);
CREATE INDEX idx_quiz_attempt_onto ON quiz_attempt(ontology_id, created_at);
