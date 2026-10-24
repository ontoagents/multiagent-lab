// REQ-366/M164 MCP 工具化本体操作——backend 侧门控面。
// facade（runtime-manager /mcp）13 件扩展工具（spec CRUD/校验/CQ 基准，T1~T3+T5，T4 P2）
// 的启用门控在平台装配层：仅在 Agent.OntoOps 开启时并入装配清单（默认关，存量零变化——
// 原 6 查询工具挂载即并入的既有语义不变）；写类 upsert/delete 入 danger 档审批（REQ-231）。
package tool

import "strings"

// OntoOperationTools REQ-366：本体 facade 扩展工具名清单（facade 侧原始名，装配层比较时
// 剥 {server}__ 前缀后对照；facade facade_ops.go addOpsTools 同源）。
// REQ-435/M231：运行面三组 12 件并入（facade_runtime.go addRuntimeTools 同源）——
// 方案管理组 7（list 只读+create/update/delete/start/stop/reload 写类）/
// 引擎运维组 3（engines_status、profile_logs 只读+engine_install 写类）/观测组 2（只读）。
var OntoOperationTools = []string{
	"spec_get",
	"spec_concept_upsert", "spec_concept_delete",
	"spec_relation_upsert", "spec_relation_delete",
	"spec_attribute_upsert", "spec_attribute_delete",
	"spec_instance_upsert", "spec_instance_delete",
	"spec_validate", "spec_quality_check",
	"cq_benchmark_datasets", "cq_benchmark_run",
	// REQ-435/M231 运行面组
	"runtime_profile_list",
	"runtime_profile_create", "runtime_profile_update", "runtime_profile_delete",
	"runtime_profile_start", "runtime_profile_stop", "runtime_profile_reload",
	"engines_status", "engine_install", "profile_logs",
	"profile_trace", "route_ambiguities",
}

var ontoOperationSet = func() map[string]bool {
	m := make(map[string]bool, len(OntoOperationTools))
	for _, n := range OntoOperationTools {
		m[n] = true
	}
	return m
}()

// ontoWriteSet 写类子集（spec 资产变更面）——danger 档审批范围。
// REQ-435/M231：运行面写类八件并入（方案 create/update/delete/start/stop/reload+engine_install
// ——方案变更是运行面最重操作，入 danger 档审批）。
var ontoWriteSet = map[string]bool{
	"spec_concept_upsert": true, "spec_concept_delete": true,
	"spec_relation_upsert": true, "spec_relation_delete": true,
	"spec_attribute_upsert": true, "spec_attribute_delete": true,
	"spec_instance_upsert": true, "spec_instance_delete": true,
	// REQ-435 运行面写类
	"runtime_profile_create": true, "runtime_profile_update": true, "runtime_profile_delete": true,
	"runtime_profile_start": true, "runtime_profile_stop": true, "runtime_profile_reload": true,
	"engine_install": true,
}

// stripToolPrefix 剥 {server}__ 前缀（ontology__spec_get → spec_get；无前缀原样）。
func stripToolPrefix(name string) string {
	if i := strings.Index(name, "__"); i >= 0 {
		return name[i+2:]
	}
	return name
}

// IsOntoOperationTool 判定（前缀后）是否 REQ-366 扩展工具。
func IsOntoOperationTool(name string) bool {
	return ontoOperationSet[stripToolPrefix(name)]
}

// IsOntoWriteTool 判定（前缀后）是否本体 spec 写类工具（审批 danger 档口径）。
func IsOntoWriteTool(name string) bool {
	return ontoWriteSet[stripToolPrefix(name)]
}
