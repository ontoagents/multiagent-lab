-- REQ-541/M348：Publish 不可变发布快照（103 号《本体依赖管理与仓库服务调研》§6.4 发布件/
-- §6.5 步骤一——OBO FP-004「发布文件内容 MUST NOT 修改」+ OCI digest 内容寻址思想，
-- 与迁移文件只增不改纪律同构）。版本表即 tag：(ontology_id, version) 唯一定位一份发布快照。
-- 内容列（spec_json/ttl/manifest_json/两 digest/triples/published_at）一经写入不可变——代码层
-- 无其 UPDATE 路径，同版本重发布同内容幂等复用既有行（同 digest），内容漂移 409 拒绝；
-- 仅 version_name 为可变 tag 标签（OCI tag 可变名语义，重发布可改命名）。
-- 快照不做预物化产物（owlrl N²~N³ 膨胀在案观察维持，103 号 R3）——ttl 为发布时 spec 投影+
-- 桥三元组+axioms-raw 余量拼接（REQ-540 诚实边界「版本化导出无 axioms-raw 余量」在发布
-- 版本上经本表解除），不含推断闭包。
CREATE TABLE IF NOT EXISTS ontology_release (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ontology_id   TEXT NOT NULL REFERENCES ontology(id) ON DELETE CASCADE,
    version       INTEGER NOT NULL,
    version_name  TEXT NOT NULL DEFAULT '',
    spec_json     TEXT NOT NULL,
    ttl           TEXT NOT NULL,
    manifest_json TEXT NOT NULL DEFAULT '',
    spec_sha256   TEXT NOT NULL,
    ttl_sha256    TEXT NOT NULL,
    triples       INTEGER NOT NULL DEFAULT -1,  -- 发布时 TTL 精确三元组数；-1=未知（axioms-raw 拼接）
    published_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(ontology_id, version)
);
CREATE INDEX IF NOT EXISTS idx_release_ontology ON ontology_release(ontology_id);
