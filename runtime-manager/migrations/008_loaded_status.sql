-- 008_loaded_status.sql REQ-239/M65：装载发布状态快照（Start 拉本体元数据时顺带取
-- status/version_name，JSON {oid:{status,version_name}}；构建平面不可达/旧字段缺失跳过不阻断启动）。
-- 沿 007 loaded_quality / 003 loaded_versions 先例（IFNULL 兼容存量行）。
ALTER TABLE runtime_profile ADD COLUMN loaded_status TEXT;
