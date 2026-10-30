-- REQ-540/M347：external_refs 版本 pin（登记级粒度；103 号《本体依赖管理与仓库服务调研》
-- §6.5 步骤一/§7.5-2② 立项裁决）。pin_version=0/空=浮动头版本（OWL 2 系列 IRI/version IRI
-- 双轨语义：本体 IRI 标识本体系列、version IRI 标识具体版本，导入方声明钉住被引版本）；
-- >0=钉住目标本体（to_onto_id）该 spec 版本号（ontology_version 快照，登记时校验快照存在）。
-- 生效面：引用解析点按 pin 解析——构建面悬空检测/登记校验按 pin 版本快照判概念存在；
-- 运行面 FetchTTL 有 pin 取对应版本导出、无 pin 维持头版本（零迁移向后兼容）；
-- 被引方升级后引用方重装载仍取 pin 版本，显式刷新 pin（PUT pin 端点/改登记）才漂移。
ALTER TABLE external_refs ADD COLUMN pin_version INTEGER NOT NULL DEFAULT 0;
