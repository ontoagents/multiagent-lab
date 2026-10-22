-- REQ-366/M164 MCP 工具化本体操作（03 §2.56，96 号规划档主线 A·O-1）：
-- Agent 能力开关 onto_ops（0=关〔缺省，存量零变化〕| 1=开启）。
-- 开关 on 时装配层并入本体 facade 13 件扩展工具（spec CRUD/校验/CQ 基准；原 6 查询工具
-- 挂载即并入的既有语义不变）——写类 upsert/delete 同时入 danger 档审批（REQ-231 口径：
-- 资产变更面）。默认关：CRUD 是本体资产变更面，严格于 onchat_mode 的内置行默认开先例。
ALTER TABLE agent ADD COLUMN onto_ops INTEGER NOT NULL DEFAULT 0;
