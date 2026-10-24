-- 065_connector_card.sql REQ-416/M212：A2A 连接器注册握手产物——agent card JSON 原文落库
--（name/skills/preferredTransport 如实在库，协作面工具描述合成与协议面预览消费）。
ALTER TABLE connector ADD COLUMN card_json TEXT NOT NULL DEFAULT '';
