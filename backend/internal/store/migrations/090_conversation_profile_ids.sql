-- REQ-619/M427：会话本体运行方案多选挂载（runtime_profile_ids JSON 数组）。
-- NULL=未显式设置（运行期回落 runtime_profile_id 单值列——工作区默认继承链继续写单值，零改动）；
-- '[]'=显式清空（关闭本体增强）；非空数组=多选集合（装配期逐方案取指引分段拼接）。
ALTER TABLE conversation ADD COLUMN runtime_profile_ids TEXT;
