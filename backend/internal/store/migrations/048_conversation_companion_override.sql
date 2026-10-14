-- REQ-302/M106 伴生绑定两层化：会话级伴生本体覆盖（''=跟随智能体默认，REQ-291 model_conn_id 同语义）。
-- agent 级绑定保持为默认档；会话显式覆盖后抽取/召回走覆盖本体（跨会话分域）。
ALTER TABLE conversation ADD COLUMN companion_ontology_id TEXT NOT NULL DEFAULT '';
