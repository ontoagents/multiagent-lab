-- REQ-388/M387 C4 Evaluator——运行质量评估落档（复用 REQ-223 Judge 契约，55 号既定口径）。
-- 每次评估一条（run 终态门禁后触发；goal/answer 为评估时点对话末轮快照）。
-- 迁移编号注：本件开工日 ls migrations/ max+1=089（088_quiz_mastery_loop 在案）。
CREATE TABLE run_evaluation (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	conversation_id TEXT NOT NULL,
	goal TEXT NOT NULL DEFAULT '',
	answer TEXT NOT NULL DEFAULT '',
	score INTEGER NOT NULL,
	verdict TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	tools TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL
);
CREATE INDEX idx_run_evaluation_run ON run_evaluation(run_id, created_at);
