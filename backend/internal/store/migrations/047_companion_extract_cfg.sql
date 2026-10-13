-- REQ-303/M107：伴生抽取高级配置（每 agent 覆盖；空串=env/内置默认）
-- JSON: {"win_msgs":16,"win_chars":8000,"max_windows":3,"temperature":0.2}
ALTER TABLE agent ADD COLUMN companion_extract_cfg TEXT NOT NULL DEFAULT '';
