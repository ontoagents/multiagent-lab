package tool

// REQ-366/M164 门控面单测：扩展工具清单判定（前缀剥离）、写类审批入 danger 档、
// 既有豁免语义不回退（ontology 查询工具/内置只读免审，mcp 连接器保守全审）。

import "testing"

func TestIsOntoOperationTool(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"前缀命中 CRUD", "ontology__spec_concept_upsert", true},
		{"前缀命中校验", "ontology__spec_validate", true},
		{"前缀命中基准", "ontology__cq_benchmark_run", true},
		{"前缀命中 spec_get", "ontology__spec_get", true},
		{"无前缀命中", "spec_quality_check", true},
		{"既有查询工具不属扩展集", "ontology__get_concept", false},
		{"sparql_query 不属扩展集（挂载即并入）", "ontology__sparql_query", false},
		{"连接器工具不误伤", "myserver__spec_get", true}, // 判定剥前缀不看来源（来源门控在装配层 capability）
		{"内置工具不误伤", "write_file", false},
		{"空串", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOntoOperationTool(tc.in); got != tc.want {
				t.Fatalf("IsOntoOperationTool(%q) = %v, 期望 %v", tc.in, got, tc.want)
			}
		})
	}
	if len(OntoOperationTools) != 13 {
		t.Fatalf("扩展工具清单应 13 件（T2×9+T3×2+T5×2），实际 %d", len(OntoOperationTools))
	}
}

func TestIsDangerousToolOntoWrites(t *testing.T) {
	// 写类 upsert/delete：ontology 前缀但入 danger 档（REQ-366 审批缺口修补）
	for _, n := range []string{
		"ontology__spec_concept_upsert", "ontology__spec_concept_delete",
		"ontology__spec_relation_upsert", "ontology__spec_relation_delete",
		"ontology__spec_attribute_upsert", "ontology__spec_attribute_delete",
		"ontology__spec_instance_upsert", "ontology__spec_instance_delete",
	} {
		if !IsDangerousTool(n) {
			t.Fatalf("%s 应入 danger 档审批", n)
		}
	}
	// 只读扩展工具与既有查询工具：免审语义不变
	for _, n := range []string{
		"ontology__spec_get", "ontology__spec_validate", "ontology__spec_quality_check",
		"ontology__cq_benchmark_datasets", "ontology__cq_benchmark_run",
		"ontology__get_concept", "ontology__sparql_query",
		"oo__anything",
	} {
		if IsDangerousTool(n) {
			t.Fatalf("%s 应免审（只读）", n)
		}
	}
	// mcp 连接器保守全审语义不变
	if !IsDangerousTool("myserver__anything") {
		t.Fatal("mcp 连接器工具应保守全审")
	}
	// 内置写类语义不变
	if !IsDangerousTool("save_file") || !IsDangerousTool("ontochat_save") {
		t.Fatal("内置写类应入 danger 档")
	}
	// 内置只读语义不变
	if IsDangerousTool("grep") || IsDangerousTool("list_files") {
		t.Fatal("内置只读应免审")
	}
}

func TestIsOntoWriteTool(t *testing.T) {
	if !IsOntoWriteTool("ontology__spec_instance_delete") || IsOntoWriteTool("spec_get") {
		t.Fatal("写类判定不符")
	}
}
